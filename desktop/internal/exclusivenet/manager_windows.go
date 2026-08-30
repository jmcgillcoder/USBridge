//go:build windows

package exclusivenet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const helperStartupTimeout = 2 * time.Minute

type windowsController struct {
	stateMu     sync.RWMutex
	operationMu sync.Mutex
	logger      *slog.Logger

	enabled bool
	active  bool
	closed  bool
	faulted bool
	target  Target
	lastErr string

	connection net.Conn
	encoder    *json.Encoder
	decoder    *json.Decoder
	requestID  uint64
}

func newController(logger *slog.Logger) Controller {
	if logger == nil {
		logger = slog.Default()
	}
	return &windowsController{logger: logger}
}

func (m *windowsController) Configure(enabled bool) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	m.enabled = enabled
	if !enabled {
		m.faulted = false
		m.lastErr = ""
	}
}

func (m *windowsController) Reconcile(ctx context.Context, target *Target) error {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()

	m.stateMu.RLock()
	closed := m.closed
	enabled := m.enabled
	active := m.active
	currentTarget := m.target
	faulted := m.faulted
	faultMessage := m.lastErr
	m.stateMu.RUnlock()
	if closed {
		return errors.New("严格代理模式服务已经停止")
	}

	if !enabled || target == nil || target.InterfaceIndex <= 0 {
		if m.connection != nil {
			command := helperCommandDisable
			if !enabled {
				command = helperCommandExit
			}
			_, err := m.requestLocked(ctx, command, 0)
			if err != nil {
				m.logger.Warn("failed to remove exclusive network policy", "error", err)
				m.dropHelperLocked()
			} else if command == helperCommandExit {
				m.dropHelperLocked()
			}
		}
		m.stateMu.Lock()
		m.active = false
		m.target = Target{}
		if !enabled {
			m.faulted = false
			m.lastErr = ""
		} else if !m.faulted {
			m.lastErr = ""
		}
		m.stateMu.Unlock()
		return nil
	}

	if faulted {
		if faultMessage == "" {
			faultMessage = "独占保护已停止，请关闭后重新开启"
		}
		return errors.New(faultMessage)
	}

	if active && currentTarget.InterfaceIndex == target.InterfaceIndex && strings.EqualFold(currentTarget.ID, target.ID) {
		m.stateMu.Lock()
		m.target = *target
		m.lastErr = ""
		m.stateMu.Unlock()
		return nil
	}

	if m.connection == nil {
		if err := m.startHelperLocked(ctx); err != nil {
			m.setFault(friendlyError(err))
			return err
		}
	}

	response, err := m.requestLocked(ctx, helperCommandApply, target.InterfaceIndex)
	if err != nil {
		// The helper may still hold the previous interface policy. Closing the
		// authenticated channel makes it exit and releases its dynamic WFP session.
		m.dropHelperLocked()
		m.setFault(friendlyError(err))
		return err
	}
	if !response.Active || response.InterfaceIndex != target.InterfaceIndex {
		err = errors.New("Windows 未确认独占规则已经生效")
		m.dropHelperLocked()
		m.setFault(friendlyError(err))
		return err
	}

	m.stateMu.Lock()
	m.active = true
	m.faulted = false
	m.target = *target
	m.lastErr = ""
	m.stateMu.Unlock()
	return nil
}

func (m *windowsController) Status() Status {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return Status{
		Supported:      true,
		Enabled:        m.enabled,
		Active:         m.enabled && m.active,
		InterfaceIndex: m.target.InterfaceIndex,
		InterfaceName:  m.target.Name,
		Error:          m.lastErr,
	}
}

func (m *windowsController) Check(ctx context.Context) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()

	m.stateMu.RLock()
	closed := m.closed
	enabled := m.enabled
	active := m.active
	target := m.target
	m.stateMu.RUnlock()
	if closed || !enabled || !active || m.connection == nil {
		return
	}
	response, err := m.requestLocked(ctx, helperCommandPing, 0)
	if err != nil {
		m.dropHelperLocked()
		m.setFault("独占保护已停止，请关闭后重新开启")
		m.logger.Warn("exclusive network helper stopped", "error", err)
		return
	}
	if !response.Active || response.InterfaceIndex != target.InterfaceIndex {
		m.dropHelperLocked()
		m.setFault("Windows 网络保护规则已失效，请关闭后重新开启")
	}
}

func (m *windowsController) Close() {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()

	m.stateMu.Lock()
	if m.closed {
		m.stateMu.Unlock()
		return
	}
	m.closed = true
	m.active = false
	m.stateMu.Unlock()

	if m.connection != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, _ = m.requestLocked(ctx, helperCommandExit, 0)
		cancel()
	}
	m.dropHelperLocked()
}

func (m *windowsController) setFault(message string) {
	m.stateMu.Lock()
	m.active = false
	m.faulted = true
	m.lastErr = message
	m.stateMu.Unlock()
}

func (m *windowsController) startHelperLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("准备管理员授权通道失败：%w", err)
	}
	defer listener.Close()
	stopCancelWatch := context.AfterFunc(ctx, func() {
		_ = listener.Close()
	})
	defer stopCancelWatch()

	token, err := randomToken()
	if err != nil {
		return fmt.Errorf("创建管理员授权请求失败：%w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("读取程序路径失败：%w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return fmt.Errorf("读取程序路径失败：%w", err)
	}

	arguments := strings.Join([]string{
		windows.EscapeArg("--usbridge-exclusive-helper"),
		windows.EscapeArg("--exclusive-control=" + listener.Addr().String()),
		windows.EscapeArg("--exclusive-token=" + token),
	}, " ")
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(executable)
	parameters, _ := windows.UTF16PtrFromString(arguments)
	directory, _ := windows.UTF16PtrFromString(filepath.Dir(executable))
	if err := windows.ShellExecute(0, verb, file, parameters, directory, windows.SW_HIDE); err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrElevationCanceled
		}
		return fmt.Errorf("请求管理员授权失败：%w", err)
	}

	deadline := time.Now().Add(helperStartupTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if tcpListener, ok := listener.(*net.TCPListener); ok {
		_ = tcpListener.SetDeadline(deadline)
	}

	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("等待严格代理模式授权超时：%w", acceptErr)
		}
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		decoder := json.NewDecoder(connection)
		var hello helperHello
		if decodeErr := decoder.Decode(&hello); decodeErr != nil || hello.Protocol != helperProtocolVersion || hello.Token != token {
			_ = connection.Close()
			continue
		}
		_ = connection.SetDeadline(time.Time{})
		m.connection = connection
		m.encoder = json.NewEncoder(connection)
		m.decoder = decoder
		return nil
	}
}

func (m *windowsController) requestLocked(ctx context.Context, command string, interfaceIndex int) (helperResponse, error) {
	if m.connection == nil || m.encoder == nil || m.decoder == nil {
		return helperResponse{}, errors.New("严格代理模式管理员服务未连接")
	}
	deadline := time.Now().Add(20 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok {
		deadline = contextDeadline
	}
	_ = m.connection.SetDeadline(deadline)
	defer m.connection.SetDeadline(time.Time{})

	m.requestID++
	request := helperRequest{ID: m.requestID, Command: command, InterfaceIndex: interfaceIndex}
	if err := m.encoder.Encode(request); err != nil {
		m.dropHelperLocked()
		return helperResponse{}, fmt.Errorf("发送严格代理模式设置失败：%w", err)
	}
	var response helperResponse
	if err := m.decoder.Decode(&response); err != nil {
		m.dropHelperLocked()
		return helperResponse{}, fmt.Errorf("读取严格代理模式状态失败：%w", err)
	}
	if response.ID != request.ID {
		m.dropHelperLocked()
		return helperResponse{}, errors.New("严格代理模式管理员服务返回了无效状态")
	}
	if !response.OK {
		if response.ErrorCode == helperErrorAdministratorRequired {
			return response, ErrAdministratorRequired
		}
		if response.Error == "" {
			response.Error = "Windows 网络保护规则未生效"
		}
		return response, errors.New(response.Error)
	}
	return response, nil
}

func (m *windowsController) dropHelperLocked() {
	if m.connection != nil {
		_ = m.connection.Close()
	}
	m.connection = nil
	m.encoder = nil
	m.decoder = nil
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrElevationCanceled) {
		return "已取消管理员授权，严格代理模式未开启"
	}
	if errors.Is(err, ErrAdministratorRequired) {
		return ErrAdministratorRequired.Error()
	}
	return fmt.Sprintf("独占保护未生效：%v", err)
}
