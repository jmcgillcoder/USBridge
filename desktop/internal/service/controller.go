package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/adapter"
	"github.com/usbridge/usbridge/desktop/internal/androidclient"
	"github.com/usbridge/usbridge/desktop/internal/routing"
	"github.com/usbridge/usbridge/desktop/internal/traffic"
)

type Controller struct {
	Adapters    *adapter.Registry
	Routing     *routing.Policy
	Android     *androidclient.Client
	AndroidPort int
	ProxyListen string
	Meter       *traffic.Meter

	phoneMu        sync.RWMutex
	phoneStatus    *androidclient.StatusResponse
	phoneError     string
	phoneCheckedAt time.Time
}

type Snapshot struct {
	Adapters         []adapter.Adapter             `json:"adapters"`
	SelectedAdapter  *adapter.Adapter              `json:"selectedAdapter,omitempty"`
	IPMode           string                        `json:"ipMode"`
	AndroidEndpoint  string                        `json:"androidEndpoint,omitempty"`
	AndroidReady     bool                          `json:"androidReady"`
	AndroidStatus    *androidclient.StatusResponse `json:"androidStatus,omitempty"`
	AndroidError     string                        `json:"androidError,omitempty"`
	AndroidCheckedAt time.Time                     `json:"androidCheckedAt,omitempty"`
	ProxyListen      string                        `json:"proxyListen"`
	Traffic          traffic.Snapshot              `json:"traffic"`
}

func (c *Controller) Snapshot() Snapshot {
	result := Snapshot{
		Adapters:    c.Adapters.Snapshot(),
		IPMode:      c.Routing.Mode().String(),
		ProxyListen: c.ProxyListen,
	}
	if c.Meter != nil {
		result.Traffic = c.Meter.Snapshot()
	}
	if selected, ok := c.Adapters.Selected(); ok {
		result.SelectedAdapter = &selected
		result.AndroidEndpoint, _ = androidclient.EndpointFromAdapter(selected, c.AndroidPort)
	}
	c.phoneMu.RLock()
	result.AndroidReady = c.phoneStatus != nil && c.phoneError == ""
	if c.phoneStatus != nil {
		status := *c.phoneStatus
		result.AndroidStatus = &status
	}
	result.AndroidError = c.phoneError
	result.AndroidCheckedAt = c.phoneCheckedAt
	c.phoneMu.RUnlock()
	return result
}

func (c *Controller) RefreshAndroidStatus(ctx context.Context) error {
	if c.Android == nil {
		return c.setPhoneError(fmt.Errorf("Android control client is unavailable"))
	}
	status, err := c.Android.Status(ctx)
	if err == nil && c.Routing != nil && status.IPMode != "" {
		if mode, parseErr := routing.ParseIPMode(status.IPMode); parseErr == nil {
			c.Routing.SetMode(mode)
		}
	}
	c.phoneMu.Lock()
	c.phoneCheckedAt = time.Now()
	if err != nil {
		c.phoneStatus = nil
		c.phoneError = err.Error()
	} else {
		c.phoneStatus = &status
		c.phoneError = ""
	}
	c.phoneMu.Unlock()
	return err
}

func (c *Controller) setPhoneError(err error) error {
	c.phoneMu.Lock()
	c.phoneStatus = nil
	c.phoneError = err.Error()
	c.phoneCheckedAt = time.Now()
	c.phoneMu.Unlock()
	return err
}

func (c *Controller) RequireCellularUpstream() error {
	c.phoneMu.RLock()
	defer c.phoneMu.RUnlock()
	if c.phoneStatus == nil || c.phoneError != "" {
		return fmt.Errorf("Android control service is not ready")
	}
	if !c.phoneStatus.USB.TetheringEnabled {
		return fmt.Errorf("phone USB tethering is not active")
	}
	if !c.phoneStatus.USB.CellularUpstream {
		return fmt.Errorf("%w: current upstream is %s", routing.ErrNoCellularUpstream, c.phoneStatus.USB.Upstream)
	}
	if c.Routing != nil {
		switch c.Routing.Mode() {
		case routing.IPModeIPv6:
			if isUnavailable(
				c.phoneStatus.Mobile.IPv6Available,
				c.phoneStatus.PublicIP.IPv6,
				c.phoneStatus.Mobile.IPv4Available,
				c.phoneStatus.PublicIP.IPv4,
			) {
				return fmt.Errorf("USB 共享出口当前仅提供 IPv4，请切换为自动或 IPv4")
			}
		}
	}
	return nil
}

func isUnavailable(selectedFamily *bool, selectedPublicIP string, otherFamily *bool, otherPublicIP string) bool {
	publicIPKnown := selectedPublicIP != "" || otherPublicIP != ""
	selectedKnown := selectedFamily != nil || publicIPKnown
	otherKnown := otherFamily != nil || publicIPKnown
	selectedAvailable := selectedPublicIP != ""
	if selectedFamily != nil {
		selectedAvailable = *selectedFamily
	}
	otherAvailable := otherPublicIP != ""
	if otherFamily != nil {
		otherAvailable = *otherFamily
	}
	return selectedKnown && !selectedAvailable && otherKnown && otherAvailable
}

func (c *Controller) SelectAdapter(selector string) error {
	if selector == "" || selector == "auto" {
		c.Adapters.UseAutomaticSelection()
		return nil
	}
	return c.Adapters.Select(selector)
}

func (c *Controller) SetIPMode(ctx context.Context, value string) (androidclient.OperationResponse, error) {
	mode, err := routing.ParseIPMode(value)
	if err != nil {
		return androidclient.OperationResponse{}, err
	}
	if c.Android == nil {
		c.Routing.SetMode(mode)
		return androidclient.OperationResponse{OK: true, Message: "desktop routing mode updated"}, nil
	}
	response, err := c.Android.SetIPMode(ctx, mode.String())
	if err != nil {
		return response, err
	}
	if !response.OK {
		return response, fmt.Errorf("phone rejected IP mode: %s", response.Message)
	}
	c.Routing.SetMode(mode)
	return response, nil
}

func (c *Controller) ReconnectMobile(ctx context.Context) (androidclient.OperationResponse, error) {
	if c.Android == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Android control client is unavailable")
	}
	return c.Android.ReconnectMobile(ctx)
}

func (c *Controller) RefreshPublicIP(ctx context.Context) (androidclient.OperationResponse, error) {
	if c.Android == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Android control client is unavailable")
	}
	return c.Android.RefreshPublicIP(ctx)
}

func (c *Controller) ForceCellularUpstream(ctx context.Context) (androidclient.OperationResponse, error) {
	if c.Android == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Android control client is unavailable")
	}
	return c.Android.ForceCellularUpstream(ctx)
}

func (c *Controller) StartTethering(ctx context.Context) (androidclient.OperationResponse, error) {
	if c.Android == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Android control client is unavailable")
	}
	return c.Android.StartTethering(ctx)
}

func (c *Controller) StopTethering(ctx context.Context) (androidclient.OperationResponse, error) {
	if c.Android == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Android control client is unavailable")
	}
	return c.Android.StopTethering(ctx)
}
