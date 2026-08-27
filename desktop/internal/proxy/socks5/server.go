package socks5

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/proxy"
)

const (
	version5         = 0x05
	noAuth           = 0x00
	usernamePassword = 0x02
	noAcceptable     = 0xFF
	authVersion      = 0x01
	authSuccess      = 0x00
	authFailure      = 0x01
	commandConnect   = 0x01
	addressIPv4      = 0x01
	addressDomain    = 0x03
	addressIPv6      = 0x04
)

type Server struct {
	Dialer           proxy.ContextDialer
	Logger           *slog.Logger
	HandshakeTimeout time.Duration
	Credentials      *proxy.Credentials
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
		Dialer:           dialer,
		Logger:           logger,
		HandshakeTimeout: 10 * time.Second,
		Credentials:      credentials,
	}
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		connection, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			if temporary, ok := err.(net.Error); ok && temporary.Temporary() {
				continue
			}
			return err
		}
		go s.handle(ctx, connection)
	}
}

func (s *Server) handle(ctx context.Context, client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(s.HandshakeTimeout))
	reader := bufio.NewReader(client)
	if err := negotiate(reader, client, s.Credentials); err != nil {
		return
	}
	target, err := readRequest(reader)
	if err != nil {
		_ = writeReply(client, 0x08, nil)
		return
	}
	upstream, err := s.Dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		s.Logger.Warn("SOCKS5 CONNECT failed", "target", target, "error", err)
		_ = writeReply(client, 0x04, nil)
		return
	}
	defer upstream.Close()
	if err := writeReply(client, 0x00, upstream.LocalAddr()); err != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})
	proxy.Relay(client, upstream)
}

func negotiate(reader *bufio.Reader, writer io.Writer, credentials *proxy.Credentials) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	if header[0] != version5 {
		return fmt.Errorf("unsupported SOCKS version %d", header[0])
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(reader, methods); err != nil {
		return err
	}
	requiredMethod := byte(noAuth)
	if credentials != nil {
		requiredMethod = usernamePassword
	}
	selected := byte(noAcceptable)
	for _, method := range methods {
		if method == requiredMethod {
			selected = requiredMethod
			break
		}
	}
	if _, err := writer.Write([]byte{version5, selected}); err != nil {
		return err
	}
	if selected == noAcceptable {
		return fmt.Errorf("client did not offer required SOCKS5 authentication method %d", requiredMethod)
	}
	if credentials != nil {
		return authenticateUsernamePassword(reader, writer, *credentials)
	}
	return nil
}

func authenticateUsernamePassword(reader *bufio.Reader, writer io.Writer, credentials proxy.Credentials) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	if header[0] != authVersion || header[1] == 0 {
		_, _ = writer.Write([]byte{authVersion, authFailure})
		return errors.New("invalid SOCKS5 username/password authentication header")
	}
	username := make([]byte, int(header[1]))
	if _, err := io.ReadFull(reader, username); err != nil {
		return err
	}
	passwordLength, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if passwordLength == 0 {
		_, _ = writer.Write([]byte{authVersion, authFailure})
		return errors.New("empty SOCKS5 password")
	}
	password := make([]byte, int(passwordLength))
	if _, err := io.ReadFull(reader, password); err != nil {
		return err
	}
	status := byte(authFailure)
	if credentials.Matches(string(username), string(password)) {
		status = authSuccess
	}
	if _, err := writer.Write([]byte{authVersion, status}); err != nil {
		return err
	}
	if status != authSuccess {
		return errors.New("SOCKS5 username/password authentication failed")
	}
	return nil
}

func readRequest(reader *bufio.Reader) (string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return "", err
	}
	if header[0] != version5 || header[1] != commandConnect || header[2] != 0x00 {
		return "", errors.New("unsupported SOCKS5 request")
	}
	var host string
	switch header[3] {
	case addressIPv4:
		value := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(reader, value); err != nil {
			return "", err
		}
		host = net.IP(value).String()
	case addressIPv6:
		value := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(reader, value); err != nil {
			return "", err
		}
		host = net.IP(value).String()
	case addressDomain:
		length, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		value := make([]byte, int(length))
		if _, err := io.ReadFull(reader, value); err != nil {
			return "", err
		}
		host = string(value)
	default:
		return "", errors.New("unsupported SOCKS5 address type")
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBytes); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBytes)))), nil
}

func writeReply(writer io.Writer, reply byte, address net.Addr) error {
	response := []byte{version5, reply, 0x00, addressIPv4, 0, 0, 0, 0, 0, 0}
	if tcpAddress, ok := address.(*net.TCPAddr); ok && tcpAddress != nil {
		if ipv4 := tcpAddress.IP.To4(); ipv4 != nil {
			copy(response[4:8], ipv4)
			binary.BigEndian.PutUint16(response[8:10], uint16(tcpAddress.Port))
		} else if ipv6 := tcpAddress.IP.To16(); ipv6 != nil {
			response = make([]byte, 4+net.IPv6len+2)
			response[0], response[1], response[2], response[3] = version5, reply, 0x00, addressIPv6
			copy(response[4:20], ipv6)
			binary.BigEndian.PutUint16(response[20:22], uint16(tcpAddress.Port))
		}
	}
	_, err := writer.Write(response)
	return err
}
