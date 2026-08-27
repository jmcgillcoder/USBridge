package socks5

import (
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/proxy"
)

func TestSOCKS5Connect(t *testing.T) {
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		connection, acceptErr := target.Accept()
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
	server := New(&net.Dialer{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go func() { _ = server.Serve(ctx, listener) }()

	client, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte{version5, 1, noAuth}); err != nil {
		t.Fatal(err)
	}
	methodResponse := make([]byte, 2)
	if _, err := io.ReadFull(client, methodResponse); err != nil {
		t.Fatal(err)
	}
	if methodResponse[1] != noAuth {
		t.Fatalf("method response = %v", methodResponse)
	}

	targetAddress := target.Addr().(*net.TCPAddr)
	request := []byte{version5, commandConnect, 0, addressIPv4, 127, 0, 0, 1, 0, 0}
	binary.BigEndian.PutUint16(request[8:10], uint16(targetAddress.Port))
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0 {
		t.Fatalf("SOCKS5 reply code = %d", reply[1])
	}

	message := []byte("USBridge SOCKS5")
	if _, err := client.Write(message); err != nil {
		t.Fatal(err)
	}
	echo := make([]byte, len(message))
	if _, err := io.ReadFull(client, echo); err != nil {
		t.Fatal(err)
	}
	if string(echo) != string(message) {
		t.Fatalf("echo = %q", echo)
	}
}

func TestSOCKS5UsernamePasswordAuthentication(t *testing.T) {
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		connection, acceptErr := target.Accept()
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
	dialer := &authenticationCountingDialer{}
	credentials := proxy.Credentials{Username: "usbridge", Password: "proxy-secret"}
	server, err := NewAuthenticated(dialer, slog.New(slog.NewTextHandler(io.Discard, nil)), credentials)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(ctx, listener) }()

	noAuthClient := dialSOCKSClient(t, listener.Addr().String())
	_, _ = noAuthClient.Write([]byte{version5, 1, noAuth})
	assertBytes(t, noAuthClient, []byte{version5, noAcceptable})
	noAuthClient.Close()

	wrongClient := dialSOCKSClient(t, listener.Addr().String())
	_, _ = wrongClient.Write([]byte{version5, 2, noAuth, usernamePassword})
	assertBytes(t, wrongClient, []byte{version5, usernamePassword})
	writeUserPassword(t, wrongClient, credentials.Username, "wrong")
	assertBytes(t, wrongClient, []byte{authVersion, authFailure})
	wrongClient.Close()
	if dialer.calls.Load() != 0 {
		t.Fatal("failed authentication reached the dialer")
	}

	client := dialSOCKSClient(t, listener.Addr().String())
	defer client.Close()
	_, _ = client.Write([]byte{version5, 2, noAuth, usernamePassword})
	assertBytes(t, client, []byte{version5, usernamePassword})
	writeUserPassword(t, client, credentials.Username, credentials.Password)
	assertBytes(t, client, []byte{authVersion, authSuccess})

	targetAddress := target.Addr().(*net.TCPAddr)
	request := []byte{version5, commandConnect, 0, addressIPv4, 127, 0, 0, 1, 0, 0}
	binary.BigEndian.PutUint16(request[8:10], uint16(targetAddress.Port))
	if _, err := client.Write(request); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(client, reply); err != nil {
		t.Fatal(err)
	}
	if reply[0] != version5 || reply[1] != 0 {
		t.Fatalf("SOCKS5 reply = %v", reply)
	}

	message := []byte("authenticated SOCKS5")
	if _, err := client.Write(message); err != nil {
		t.Fatal(err)
	}
	assertBytes(t, client, message)
	if dialer.calls.Load() != 1 {
		t.Fatalf("dial calls = %d", dialer.calls.Load())
	}
}

type authenticationCountingDialer struct {
	calls atomic.Int32
}

func (dialer *authenticationCountingDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer.calls.Add(1)
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

func dialSOCKSClient(t *testing.T, address string) net.Conn {
	t.Helper()
	client, err := net.DialTimeout("tcp4", address, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		client.Close()
		t.Fatal(err)
	}
	return client
}

func writeUserPassword(t *testing.T, writer io.Writer, username, password string) {
	t.Helper()
	request := []byte{authVersion, byte(len(username))}
	request = append(request, username...)
	request = append(request, byte(len(password)))
	request = append(request, password...)
	if _, err := writer.Write(request); err != nil {
		t.Fatal(err)
	}
}

func assertBytes(t *testing.T, reader io.Reader, expected []byte) {
	t.Helper()
	actual := make([]byte, len(expected))
	if _, err := io.ReadFull(reader, actual); err != nil {
		t.Fatal(err)
	}
	for index := range expected {
		if expected[index] != actual[index] {
			t.Fatalf("response = %v, want %v", actual, expected)
		}
	}
}
