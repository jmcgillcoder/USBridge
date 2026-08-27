package androidclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/adapter"
)

type EndpointProvider func() (string, error)

type Client struct {
	endpoint EndpointProvider
	token    string
	http     *http.Client
}

func New(endpoint EndpointProvider, token string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{
		endpoint: endpoint,
		token:    token,
		http: &http.Client{
			Timeout:   4 * time.Minute,
			Transport: transport,
		},
	}
}

func StaticEndpoint(value string) EndpointProvider {
	return func() (string, error) { return value, nil }
}

func EndpointFromAdapter(value adapter.Adapter, port int) (string, error) {
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("invalid Android API port %d", port)
	}
	var fallback net.IP
	var linkLocal net.IP
	var linkLocalZone string
	for _, gateway := range value.Gateways {
		plain := gateway
		zone := ""
		if index := strings.LastIndexByte(plain, '%'); index >= 0 {
			zone = plain[index+1:]
			plain = plain[:index]
		}
		ip := net.ParseIP(plain)
		if ip == nil || ip.IsUnspecified() || ip.IsLoopback() {
			continue
		}
		if ip.To4() != nil {
			return endpointURL(ip, "", port), nil
		}
		if ip.IsLinkLocalUnicast() {
			if linkLocal == nil {
				linkLocal = ip
				linkLocalZone = zone
			}
		} else if fallback == nil {
			fallback = ip
		}
	}
	if fallback != nil {
		return endpointURL(fallback, "", port), nil
	}
	if linkLocal != nil && value.InterfaceIndex > 0 {
		if linkLocalZone == "" {
			linkLocalZone = strconv.Itoa(value.InterfaceIndex)
		}
		return endpointURL(linkLocal, linkLocalZone, port), nil
	}
	return "", errors.New("selected USB adapter has no phone gateway")
}

func endpointURL(ip net.IP, zone string, port int) string {
	host := ip.String()
	if zone != "" {
		host += "%" + zone
	}
	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
	}).String()
}

func (c *Client) Status(ctx context.Context) (StatusResponse, error) {
	var response StatusResponse
	err := c.do(ctx, http.MethodGet, "/v1/status", nil, &response)
	return response, err
}

func (c *Client) ReconnectMobile(ctx context.Context) (OperationResponse, error) {
	return c.operation(ctx, http.MethodPost, "/v1/mobile/reconnect", nil)
}

func (c *Client) RefreshPublicIP(ctx context.Context) (OperationResponse, error) {
	return c.operation(ctx, http.MethodPost, "/v1/public-ip/refresh", nil)
}

func (c *Client) ForceCellularUpstream(ctx context.Context) (OperationResponse, error) {
	return c.operation(ctx, http.MethodPost, "/v1/upstream/cellular", nil)
}

func (c *Client) StartTethering(ctx context.Context) (OperationResponse, error) {
	return c.operation(ctx, http.MethodPost, "/v1/tether/start", nil)
}

func (c *Client) StopTethering(ctx context.Context) (OperationResponse, error) {
	return c.operation(ctx, http.MethodPost, "/v1/tether/stop", nil)
}

func (c *Client) SetIPMode(ctx context.Context, mode string) (OperationResponse, error) {
	return c.operation(ctx, http.MethodPut, "/v1/ip-mode", map[string]string{"mode": mode})
}

func (c *Client) Traffic(ctx context.Context) (TrafficResponse, error) {
	var response TrafficResponse
	err := c.do(ctx, http.MethodGet, "/v1/traffic", nil, &response)
	return response, err
}

func (c *Client) operation(ctx context.Context, method, path string, body any) (OperationResponse, error) {
	var response OperationResponse
	err := c.do(ctx, method, path, body, &response)
	return response, err
}

func (c *Client) do(ctx context.Context, method, path string, requestBody, responseBody any) error {
	endpoint, err := c.endpoint()
	if err != nil {
		return err
	}
	parsedEndpoint, err := url.Parse(strings.TrimRight(endpoint, "/") + path)
	if err != nil || parsedEndpoint.Scheme != "http" || parsedEndpoint.Host == "" {
		return fmt.Errorf("invalid Android API endpoint %q", endpoint)
	}
	var bodyReader io.Reader
	if requestBody == nil && method != http.MethodGet && method != http.MethodHead {
		requestBody = struct{}{}
	}
	if requestBody != nil {
		encoded, encodeErr := json.Marshal(requestBody)
		if encodeErr != nil {
			return encodeErr
		}
		bodyReader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, parsedEndpoint.String(), bodyReader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-USBridge-Client", "desktop")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("X-USBridge-Token", c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call Android API: %w", err)
	}
	defer response.Body.Close()
	limitedBody := io.LimitReader(response.Body, 1<<20)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(limitedBody)
		return fmt.Errorf("Android API returned %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	if responseBody == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(limitedBody).Decode(responseBody); err != nil {
		return fmt.Errorf("decode Android API response: %w", err)
	}
	return nil
}
