package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usbridge/usbridge/desktop/internal/androidclient"
	"github.com/usbridge/usbridge/desktop/internal/routing"
)

func TestRefreshAndroidStatusCachesHealthyPhone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/status" {
			http.NotFound(writer, request)
			return
		}
		_ = json.NewEncoder(writer).Encode(androidclient.StatusResponse{
			Version: "0.2.0",
			IPMode:  "ipv6",
			Root: androidclient.RootStatus{
				Granted:        true,
				Implementation: "KernelSU",
			},
			USB: androidclient.USBStatus{
				TetheringEnabled: true,
				Upstream:         "cellular",
				CellularUpstream: true,
			},
		})
	}))
	defer server.Close()

	policy := routing.NewPolicy(routing.IPModeAuto)
	controller := &Controller{
		Android: androidclient.New(androidclient.StaticEndpoint(server.URL), ""),
		Routing: policy,
	}
	if err := controller.RefreshAndroidStatus(context.Background()); err != nil {
		t.Fatal(err)
	}

	controller.phoneMu.RLock()
	defer controller.phoneMu.RUnlock()
	if controller.phoneStatus == nil || !controller.phoneStatus.Root.Granted {
		t.Fatalf("phone status = %+v", controller.phoneStatus)
	}
	if controller.phoneError != "" {
		t.Fatalf("phone error = %q", controller.phoneError)
	}
	if err := controller.RequireCellularUpstream(); err != nil {
		t.Fatalf("cellular upstream gate: %v", err)
	}
	if got := policy.Mode(); got != routing.IPModeIPv6 {
		t.Fatalf("desktop IP mode = %s, want ipv6", got)
	}
}

func TestRequireCellularUpstreamAllowsIPv4ModeThroughNAT64(t *testing.T) {
	controller := &Controller{
		Routing: routing.NewPolicy(routing.IPModeIPv4),
		phoneStatus: &androidclient.StatusResponse{
			USB: androidclient.USBStatus{
				TetheringEnabled: true,
				CellularUpstream: true,
			},
			Mobile: androidclient.MobileStatus{
				IPv4Available: boolPointer(false),
				IPv6Available: boolPointer(true),
			},
		},
	}

	if err := controller.RequireCellularUpstream(); err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestRequireCellularUpstreamAllowsAutoOnIPv6OnlyNetwork(t *testing.T) {
	controller := &Controller{
		Routing: routing.NewPolicy(routing.IPModeAuto),
		phoneStatus: &androidclient.StatusResponse{
			USB: androidclient.USBStatus{
				TetheringEnabled: true,
				CellularUpstream: true,
			},
			Mobile: androidclient.MobileStatus{
				IPv4Available: boolPointer(false),
				IPv6Available: boolPointer(true),
			},
		},
	}

	if err := controller.RequireCellularUpstream(); err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestRequireCellularUpstreamAllowsIPv4ModeWhenPhonePublicIPIsDualStack(t *testing.T) {
	controller := &Controller{
		Routing: routing.NewPolicy(routing.IPModeIPv4),
		phoneStatus: &androidclient.StatusResponse{
			USB: androidclient.USBStatus{
				TetheringEnabled: true,
				CellularUpstream: true,
			},
			Mobile: androidclient.MobileStatus{
				IPv4Available: boolPointer(false),
				IPv6Available: boolPointer(true),
			},
			PublicIP: androidclient.PublicIPStatus{
				IPv4: "198.51.100.8",
				IPv6: "2001:db8::8",
			},
		},
	}

	if err := controller.RequireCellularUpstream(); err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestRequireCellularUpstreamAllowsIPv4ModeForOlderPhoneContract(t *testing.T) {
	controller := &Controller{
		Routing: routing.NewPolicy(routing.IPModeIPv4),
		phoneStatus: &androidclient.StatusResponse{
			USB: androidclient.USBStatus{
				TetheringEnabled: true,
				CellularUpstream: true,
			},
			PublicIP: androidclient.PublicIPStatus{IPv6: "2001:db8::8"},
		},
	}

	if err := controller.RequireCellularUpstream(); err != nil {
		t.Fatalf("error = %v", err)
	}
}

func TestRequireCellularUpstreamRejectsUnavailableIPv6Mode(t *testing.T) {
	controller := &Controller{
		Routing: routing.NewPolicy(routing.IPModeIPv6),
		phoneStatus: &androidclient.StatusResponse{
			USB: androidclient.USBStatus{
				TetheringEnabled: true,
				CellularUpstream: true,
			},
			Mobile: androidclient.MobileStatus{
				IPv4Available: boolPointer(true),
				IPv6Available: boolPointer(false),
			},
		},
	}

	err := controller.RequireCellularUpstream()
	if err == nil || !strings.Contains(err.Error(), "仅提供 IPv4") {
		t.Fatalf("error = %v", err)
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func TestRefreshAndroidStatusCachesFailure(t *testing.T) {
	controller := &Controller{
		Android: androidclient.New(androidclient.StaticEndpoint("http://127.0.0.1:1"), ""),
	}
	if err := controller.RefreshAndroidStatus(context.Background()); err == nil {
		t.Fatal("expected connection error")
	}

	controller.phoneMu.RLock()
	defer controller.phoneMu.RUnlock()
	if controller.phoneStatus != nil || controller.phoneError == "" {
		t.Fatalf("status = %+v, error = %q", controller.phoneStatus, controller.phoneError)
	}
}
