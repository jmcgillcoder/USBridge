package httpproxy

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/proxy"
)

func TestHTTPAndConnectProxy(t *testing.T) {
	target := httptest.NewTLSServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("X-USBridge-Test", "ok")
		_, _ = io.WriteString(responseWriter, "through-proxy")
	}))
	defer target.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := New(&net.Dialer{}, logger)
	go func() { _ = server.Serve(ctx, listener) }()

	proxyURL := &url.URL{Scheme: "http", Host: listener.Addr().String()}
	client := &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{ // Test server uses an ephemeral certificate.
			InsecureSkipVerify: true,
		},
	}}
	response, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(body)); got != "through-proxy" {
		t.Fatalf("body = %q", got)
	}
	if response.Header.Get("X-USBridge-Test") != "ok" {
		t.Fatal("response did not pass through the test target")
	}
}

func TestHTTPProxyAuthentication(t *testing.T) {
	var leakedProxyAuthorization atomic.Bool
	httpTarget := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Proxy-Authorization") != "" {
			leakedProxyAuthorization.Store(true)
		}
		_, _ = io.WriteString(responseWriter, "authenticated-http")
	}))
	defer httpTarget.Close()
	tlsTarget := httptest.NewTLSServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(responseWriter, "authenticated-connect")
	}))
	defer tlsTarget.Close()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dialer := &countingDialer{}
	credentials := proxy.Credentials{Username: "usbridge", Password: "proxy-secret"}
	server, err := NewAuthenticated(dialer, slog.New(slog.NewTextHandler(io.Discard, nil)), credentials)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(ctx, listener) }()

	unauthenticatedClient := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(&url.URL{
			Scheme: "http",
			Host:   listener.Addr().String(),
		})},
	}
	response, err := unauthenticatedClient.Get(httpTarget.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("status = %d, want 407", response.StatusCode)
	}
	if response.Header.Get("Proxy-Authenticate") != `Basic realm="USBridge"` {
		t.Fatalf("challenge = %q", response.Header.Get("Proxy-Authenticate"))
	}
	if dialer.calls.Load() != 0 {
		t.Fatal("unauthenticated request reached the dialer")
	}

	authenticatedProxyURL := &url.URL{
		Scheme: "http",
		Host:   listener.Addr().String(),
		User:   url.UserPassword(credentials.Username, credentials.Password),
	}
	authenticatedClient := &http.Client{
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(authenticatedProxyURL),
			TLSClientConfig: &tls.Config{ // Test server uses an ephemeral certificate.
				InsecureSkipVerify: true,
			},
		},
	}
	for _, testCase := range []struct {
		url  string
		body string
	}{
		{url: httpTarget.URL, body: "authenticated-http"},
		{url: tlsTarget.URL, body: "authenticated-connect"},
	} {
		response, err := authenticatedClient.Get(testCase.url)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusOK || string(body) != testCase.body {
			t.Fatalf("response = %d %q", response.StatusCode, body)
		}
	}
	if leakedProxyAuthorization.Load() {
		t.Fatal("Proxy-Authorization leaked to the destination")
	}
}

func TestHTTPProxyRejectsMalformedCredentials(t *testing.T) {
	credentials := proxy.Credentials{Username: "usbridge", Password: "proxy-secret"}
	server, err := NewAuthenticated(&countingDialer{}, slog.New(slog.NewTextHandler(io.Discard, nil)), credentials)
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{
		"",
		"Bearer value",
		"Basic !!!",
		"Basic dXNlcg==",
		"Basic dXNicmlkZ2U6d3Jvbmc=",
	} {
		request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
		request.Header.Set("Proxy-Authorization", header)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusProxyAuthRequired {
			t.Fatalf("header %q returned %d", header, response.Code)
		}
	}
}

type countingDialer struct {
	calls atomic.Int32
}

func (dialer *countingDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer.calls.Add(1)
	return (&net.Dialer{}).DialContext(ctx, network, address)
}
