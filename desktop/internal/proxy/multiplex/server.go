package multiplex

import (
	"bufio"
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/proxy"
	"github.com/usbridge/usbridge/desktop/internal/proxy/httpproxy"
	"github.com/usbridge/usbridge/desktop/internal/proxy/socks5"
	"github.com/usbridge/usbridge/desktop/internal/traffic"
)

// Server identifies SOCKS5 by its 0x05 version byte and sends all HTTP method
// prefixes to net/http. This lets both proxy protocols share one TCP port.
type Server struct {
	HTTP             *httpproxy.Server
	SOCKS5           *socks5.Server
	Meter            *traffic.Meter
	Connections      *proxy.ConnectionTracker
	Logger           *slog.Logger
	DetectionTimeout time.Duration
}

func New(dialer proxy.ContextDialer, meter *traffic.Meter, logger *slog.Logger) *Server {
	return newServer(dialer, meter, logger, nil)
}

func NewAuthenticated(dialer proxy.ContextDialer, meter *traffic.Meter, logger *slog.Logger, credentials proxy.Credentials) (*Server, error) {
	if err := credentials.Validate(); err != nil {
		return nil, err
	}
	return newServer(dialer, meter, logger, &credentials), nil
}

func newServer(dialer proxy.ContextDialer, meter *traffic.Meter, logger *slog.Logger, credentials *proxy.Credentials) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if meter == nil {
		meter = traffic.NewMeter()
	}
	httpServer := httpproxy.New(dialer, logger)
	socksServer := socks5.New(dialer, logger)
	if credentials != nil {
		httpServer, _ = httpproxy.NewAuthenticated(dialer, logger, *credentials)
		socksServer, _ = socks5.NewAuthenticated(dialer, logger, *credentials)
	}
	return &Server{
		HTTP:             httpServer,
		SOCKS5:           socksServer,
		Meter:            meter,
		Connections:      proxy.NewConnectionTracker(),
		Logger:           logger,
		DetectionTimeout: 8 * time.Second,
	}
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	serverContext, cancel := context.WithCancel(ctx)
	defer cancel()
	httpListener := newDispatchListener(listener.Addr())
	socksListener := newDispatchListener(listener.Addr())
	defer httpListener.Close()
	defer socksListener.Close()

	serverErrors := make(chan error, 2)
	go func() { serverErrors <- s.HTTP.Serve(serverContext, httpListener) }()
	go func() { serverErrors <- s.SOCKS5.Serve(serverContext, socksListener) }()
	go func() {
		<-serverContext.Done()
		_ = listener.Close()
	}()

	for {
		connection, err := listener.Accept()
		if err != nil {
			cancel()
			select {
			case serverErr := <-serverErrors:
				if serverErr != nil {
					return serverErr
				}
			default:
			}
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.dispatch(serverContext, connection, httpListener, socksListener)
	}
}

func (s *Server) dispatch(
	ctx context.Context,
	connection net.Conn,
	httpListener *dispatchListener,
	socksListener *dispatchListener,
) {
	_ = connection.SetReadDeadline(time.Now().Add(s.DetectionTimeout))
	reader := bufio.NewReaderSize(connection, 4096)
	prefix, err := reader.Peek(1)
	if err != nil {
		_ = connection.Close()
		return
	}
	_ = connection.SetReadDeadline(time.Time{})
	buffered := &bufferedConnection{Conn: connection, reader: reader}
	trackedConnection := s.Connections.Wrap(buffered)

	if prefix[0] == 0x05 {
		tracked := s.Meter.Wrap(trackedConnection, traffic.ProtocolSOCKS5)
		if !socksListener.deliver(ctx, tracked) {
			_ = tracked.Close()
		}
		return
	}
	if prefix[0] >= 'A' && prefix[0] <= 'Z' {
		tracked := s.Meter.Wrap(trackedConnection, traffic.ProtocolHTTP)
		if !httpListener.deliver(ctx, tracked) {
			_ = tracked.Close()
		}
		return
	}
	s.Logger.Debug("rejected unknown proxy protocol", "remote", connection.RemoteAddr())
	_ = trackedConnection.Close()
}

func (s *Server) ResetConnections() int {
	if s == nil {
		return 0
	}
	if s.HTTP != nil {
		s.HTTP.CloseIdleConnections()
	}
	return s.Connections.CloseAll()
}

type bufferedConnection struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConnection) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}

type dispatchListener struct {
	address net.Addr
	queue   chan net.Conn
	done    chan struct{}
	close   sync.Once
}

func newDispatchListener(address net.Addr) *dispatchListener {
	return &dispatchListener{
		address: address,
		queue:   make(chan net.Conn),
		done:    make(chan struct{}),
	}
}

func (l *dispatchListener) Accept() (net.Conn, error) {
	select {
	case connection := <-l.queue:
		return connection, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *dispatchListener) Close() error {
	l.close.Do(func() { close(l.done) })
	return nil
}

func (l *dispatchListener) Addr() net.Addr {
	return l.address
}

func (l *dispatchListener) deliver(ctx context.Context, connection net.Conn) bool {
	select {
	case l.queue <- connection:
		return true
	case <-l.done:
		return false
	case <-ctx.Done():
		return false
	}
}
