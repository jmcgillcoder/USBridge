package httpproxy

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/proxy"
)

type Server struct {
	dialer      proxy.ContextDialer
	logger      *slog.Logger
	transport   *http.Transport
	credentials *proxy.Credentials
}

func New(dialer proxy.ContextDialer, logger *slog.Logger) *Server {
	return newServer(dialer, logger, nil)
}

func NewAuthenticated(dialer proxy.ContextDialer, logger *slog.Logger, credentials proxy.Credentials) (*Server, error) {
	if err := credentials.Validate(); err != nil {
		return nil, err
	}
	return newServer(dialer, logger, &credentials), nil
}

func newServer(dialer proxy.ContextDialer, logger *slog.Logger, credentials *proxy.Credentials) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		dialer:      dialer,
		logger:      logger,
		credentials: credentials,
		transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     false,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	httpServer := &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	shutdownFinished := make(chan struct{})
	go func() {
		defer close(shutdownFinished)
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownContext)
	}()

	err := httpServer.Serve(listener)
	s.transport.CloseIdleConnections()
	if ctx.Err() != nil {
		<-shutdownFinished
	}
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (s *Server) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	if s.credentials != nil && !authorized(request.Header.Get("Proxy-Authorization"), *s.credentials) {
		responseWriter.Header().Set("Proxy-Authenticate", `Basic realm="USBridge"`)
		responseWriter.Header().Set("Connection", "close")
		http.Error(responseWriter, "Proxy authentication required", http.StatusProxyAuthRequired)
		return
	}
	if request.Method == http.MethodConnect {
		s.serveConnect(responseWriter, request)
		return
	}
	s.serveForward(responseWriter, request)
}

func authorized(header string, credentials proxy.Credentials) bool {
	scheme, encoded, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Basic") {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return false
	}
	username, password, found := strings.Cut(string(decoded), ":")
	return found && credentials.Matches(username, password)
}

func (s *Server) CloseIdleConnections() {
	if s != nil && s.transport != nil {
		s.transport.CloseIdleConnections()
	}
}

func (s *Server) serveForward(responseWriter http.ResponseWriter, request *http.Request) {
	outbound := request.Clone(request.Context())
	outbound.RequestURI = ""
	outbound.Header = request.Header.Clone()
	removeHopHeaders(outbound.Header)
	outbound.Header.Del("Proxy-Authorization")
	outbound.Header.Del("Forwarded")
	outbound.Header.Del("X-Forwarded-For")
	if outbound.URL.Scheme == "" {
		outbound.URL.Scheme = "http"
	}
	if outbound.URL.Host == "" {
		outbound.URL.Host = request.Host
	}

	response, err := s.transport.RoundTrip(outbound)
	if err != nil {
		s.logger.Warn("HTTP proxy request failed", "host", outbound.URL.Host, "error", err)
		http.Error(responseWriter, "USBridge could not reach the destination through the USB adapter", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	removeHopHeaders(response.Header)
	copyHeaders(responseWriter.Header(), response.Header)
	responseWriter.WriteHeader(response.StatusCode)
	_, _ = io.Copy(responseWriter, response.Body)
}

func (s *Server) serveConnect(responseWriter http.ResponseWriter, request *http.Request) {
	target := request.Host
	if _, _, err := net.SplitHostPort(target); err != nil {
		target = net.JoinHostPort(target, "443")
	}
	upstream, err := s.dialer.DialContext(request.Context(), "tcp", target)
	if err != nil {
		s.logger.Warn("HTTP CONNECT failed", "target", target, "error", err)
		http.Error(responseWriter, "USBridge could not open the tunnel through the USB adapter", http.StatusBadGateway)
		return
	}

	hijacker, ok := responseWriter.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(responseWriter, "HTTP tunneling is unavailable", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	defer client.Close()
	defer upstream.Close()
	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\nProxy-Agent: USBridge\r\n\r\n"); err != nil {
		return
	}
	if err := buffered.Flush(); err != nil {
		return
	}
	proxy.Relay(client, upstream)
}

func removeHopHeaders(header http.Header) {
	for _, connectionHeader := range header.Values("Connection") {
		for _, token := range strings.Split(connectionHeader, ",") {
			header.Del(strings.TrimSpace(token))
		}
	}
	for _, name := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"TE",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(name)
	}
}

func copyHeaders(destination, source http.Header) {
	for name, values := range source {
		for _, value := range values {
			destination.Add(name, value)
		}
	}
}

func (s *Server) String() string {
	return fmt.Sprintf("HTTP proxy (%T)", s.dialer)
}
