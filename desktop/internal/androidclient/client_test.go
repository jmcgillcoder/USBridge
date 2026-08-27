package androidclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usbridge/usbridge/desktop/internal/adapter"
)

func TestClientContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-USBridge-Client") != "desktop" {
			t.Fatalf("missing desktop control header")
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/status":
			_ = json.NewEncoder(responseWriter).Encode(StatusResponse{
				Version: "0.1.0",
				Root:    RootStatus{Granted: true, Implementation: "KernelSU"},
				IPMode:  "auto",
			})
		case "/v1/ip-mode":
			if request.Method != http.MethodPut {
				t.Fatalf("method = %s", request.Method)
			}
			var body map[string]string
			_ = json.NewDecoder(request.Body).Decode(&body)
			if body["mode"] != "ipv6" {
				t.Fatalf("mode = %q", body["mode"])
			}
			_ = json.NewEncoder(responseWriter).Encode(OperationResponse{OK: true, Message: "saved"})
		case "/v1/mobile/reconnect":
			changed := false
			commandSucceeded := true
			_ = json.NewEncoder(responseWriter).Encode(OperationResponse{
				OK:               true,
				Message:          "reconnected with the same address",
				Before:           &PublicIPStatus{IPv4: "203.0.113.4"},
				After:            &PublicIPStatus{IPv4: "203.0.113.4"},
				CommandSucceeded: &commandSucceeded,
				IPChanged:        &changed,
			})
		case "/v1/upstream/cellular":
			if request.Method != http.MethodPost {
				t.Fatalf("method = %s", request.Method)
			}
			_ = json.NewEncoder(responseWriter).Encode(OperationResponse{OK: true, Message: "cellular upstream ready"})
		case "/v1/public-ip/refresh":
			_ = json.NewEncoder(responseWriter).Encode(OperationResponse{
				OK:      true,
				Message: "public IP refreshed",
				After:   &PublicIPStatus{IPv4: "203.0.113.8"},
			})
		default:
			http.NotFound(responseWriter, request)
		}
	}))
	defer server.Close()

	client := New(StaticEndpoint(server.URL), "")
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Root.Granted || status.Root.Implementation != "KernelSU" {
		t.Fatalf("unexpected status: %+v", status)
	}
	operation, err := client.SetIPMode(context.Background(), "ipv6")
	if err != nil {
		t.Fatal(err)
	}
	if !operation.OK {
		t.Fatalf("operation = %+v", operation)
	}
	reconnect, err := client.ReconnectMobile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reconnect.IPChanged == nil || *reconnect.IPChanged || reconnect.Before == nil {
		t.Fatalf("reconnect = %+v", reconnect)
	}
	upstream, err := client.ForceCellularUpstream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !upstream.OK {
		t.Fatalf("upstream = %+v", upstream)
	}
	refreshed, err := client.RefreshPublicIP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !refreshed.OK || refreshed.After == nil {
		t.Fatalf("refreshed = %+v", refreshed)
	}
}

func TestEndpointFromAdapterUsesIPv4GatewayFirst(t *testing.T) {
	endpoint, err := EndpointFromAdapter(adapter.Adapter{
		InterfaceIndex: 17,
		Gateways:       []string{"fe80::1", "10.179.36.200"},
	}, 17890)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "http://10.179.36.200:17890" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}

func TestEndpointFromAdapterScopesLinkLocalIPv6Gateway(t *testing.T) {
	endpoint, err := EndpointFromAdapter(adapter.Adapter{
		InterfaceIndex: 17,
		Gateways:       []string{"fe80::1"},
	}, 17890)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "http://[fe80::1%2517]:17890" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}

func TestEndpointFromAdapterKeepsExistingIPv6Zone(t *testing.T) {
	endpoint, err := EndpointFromAdapter(adapter.Adapter{
		InterfaceIndex: 17,
		Gateways:       []string{"fe80::1%Ethernet 2"},
	}, 17890)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "http://[fe80::1%25Ethernet%202]:17890" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}
