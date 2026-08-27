package multiplex

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/proxy"
	"github.com/usbridge/usbridge/desktop/internal/traffic"
)

func TestHTTPAndSOCKS5ShareOnePort(t *testing.T) {
	httpTarget := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "http-ok")
	}))
	defer httpTarget.Close()

	echoTarget, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echoTarget.Close()
	go func() {
		connection, acceptErr := echoTarget.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_, _ = io.Copy(connection, connection)
	}()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	meter := traffic.NewMeter()
	server := New(&net.Dialer{}, meter, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go func() { _ = server.Serve(ctx, listener) }()

	proxyURL := &url.URL{Scheme: "http", Host: listener.Addr().String()}
	httpClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	response, err := httpClient.Get(httpTarget.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "http-ok" {
		t.Fatalf("HTTP body = %q", body)
	}

	socksClient, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer socksClient.Close()
	_, _ = socksClient.Write([]byte{5, 1, 0})
	method := make([]byte, 2)
	_, _ = io.ReadFull(socksClient, method)
	if method[0] != 5 || method[1] != 0 {
		t.Fatalf("SOCKS5 method reply = %v", method)
	}
	targetAddress := echoTarget.Addr().(*net.TCPAddr)
	request := []byte{5, 1, 0, 1, 127, 0, 0, 1, 0, 0}
	binary.BigEndian.PutUint16(request[8:10], uint16(targetAddress.Port))
	_, _ = socksClient.Write(request)
	reply := make([]byte, 10)
	_, _ = io.ReadFull(socksClient, reply)
	if reply[1] != 0 {
		t.Fatalf("SOCKS5 reply = %v", reply)
	}
	_, _ = socksClient.Write([]byte("socks-ok"))
	echo := make([]byte, len("socks-ok"))
	_, _ = io.ReadFull(socksClient, echo)
	if string(echo) != "socks-ok" {
		t.Fatalf("SOCKS5 echo = %q", echo)
	}

	snapshot := meter.Snapshot()
	if snapshot.HTTP.Connections != 1 || snapshot.SOCKS5.Connections != 1 {
		t.Fatalf("protocol counters = %+v", snapshot)
	}
}

func TestAuthenticatedHTTPAndSOCKS5ShareOnePort(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "authenticated")
	}))
	defer target.Close()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	credentials := proxy.Credentials{Username: "usbridge", Password: "proxy-secret"}
	server, err := NewAuthenticated(&net.Dialer{}, traffic.NewMeter(), slog.New(slog.NewTextHandler(io.Discard, nil)), credentials)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(ctx, listener) }()

	unauthenticated := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(&url.URL{
			Scheme: "http",
			Host:   listener.Addr().String(),
		})},
	}
	response, err := unauthenticated.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("unauthenticated HTTP status = %d", response.StatusCode)
	}

	authenticatedProxy := &url.URL{
		Scheme: "http",
		Host:   listener.Addr().String(),
		User:   url.UserPassword(credentials.Username, credentials.Password),
	}
	authenticated := &http.Client{
		Timeout:   3 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(authenticatedProxy)},
	}
	response, err = authenticated.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK || string(body) != "authenticated" {
		t.Fatalf("authenticated HTTP response = %d %q", response.StatusCode, body)
	}

	client, err := net.DialTimeout("tcp4", listener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = client.Write([]byte{5, 2, 0, 2})
	method := make([]byte, 2)
	if _, err := io.ReadFull(client, method); err != nil {
		t.Fatal(err)
	}
	if method[0] != 5 || method[1] != 2 {
		t.Fatalf("SOCKS5 method = %v", method)
	}
	authRequest := []byte{1, byte(len(credentials.Username))}
	authRequest = append(authRequest, credentials.Username...)
	authRequest = append(authRequest, byte(len(credentials.Password)))
	authRequest = append(authRequest, credentials.Password...)
	_, _ = client.Write(authRequest)
	authReply := make([]byte, 2)
	if _, err := io.ReadFull(client, authReply); err != nil {
		t.Fatal(err)
	}
	if authReply[0] != 1 || authReply[1] != 0 {
		t.Fatalf("SOCKS5 authentication reply = %v", authReply)
	}
}
