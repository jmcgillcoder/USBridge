package systemproxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	proxyServer   = "http=127.0.0.1:18080;https=127.0.0.1:18080"
	proxyOverride = "<local>;localhost;127.*;[::1]"
)

type OptionalInteger struct {
	Present bool   `json:"present"`
	Value   uint32 `json:"value,omitempty"`
}

type OptionalString struct {
	Present bool   `json:"present"`
	Value   string `json:"value,omitempty"`
}

type Settings struct {
	ProxyEnable   OptionalInteger `json:"proxyEnable"`
	ProxyServer   OptionalString  `json:"proxyServer"`
	ProxyOverride OptionalString  `json:"proxyOverride"`
}

type Status struct {
	Supported bool
	Active    bool
	Error     string
}

type Controller interface {
	Enable() error
	Restore() error
	Status() Status
}

type settingsStore interface {
	Read() (Settings, error)
	Write(Settings) error
}

type notifier interface {
	Notify() error
}

type recoveryRecord struct {
	Original Settings `json:"original"`
	Applied  Settings `json:"applied"`
}

type controller struct {
	mu           sync.Mutex
	store        settingsStore
	notifier     notifier
	recoveryPath string
	supported    bool
	active       bool
	lastErr      string
}

func New() Controller {
	directory, err := os.UserConfigDir()
	if err != nil {
		return &controller{supported: platformSupported(), lastErr: fmt.Sprintf("读取 Windows 代理设置失败：%v", err)}
	}
	return newController(platformStore(), platformNotifier(), filepath.Join(directory, "USBridge", "system-proxy-recovery.json"), platformSupported())
}

func newController(store settingsStore, notifier notifier, recoveryPath string, supported bool) *controller {
	return &controller{store: store, notifier: notifier, recoveryPath: recoveryPath, supported: supported}
}

func desiredSettings() Settings {
	return Settings{
		ProxyEnable:   OptionalInteger{Present: true, Value: 1},
		ProxyServer:   OptionalString{Present: true, Value: proxyServer},
		ProxyOverride: OptionalString{Present: true, Value: proxyOverride},
	}
}

func (m *controller) Enable() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.supported {
		return m.fail(errors.New("系统代理接管仅支持 Windows"))
	}
	if m.store == nil || m.notifier == nil || m.recoveryPath == "" {
		return m.fail(errors.New("Windows 系统代理尚未准备好"))
	}

	current, err := m.store.Read()
	if err != nil {
		return m.fail(fmt.Errorf("读取 Windows 系统代理失败：%w", err))
	}
	desired := desiredSettings()
	record, recordErr := m.loadRecovery()
	if recordErr == nil && current == record.Applied {
		if err := m.notifier.Notify(); err != nil {
			return m.fail(fmt.Errorf("刷新 Windows 系统代理失败：%w", err))
		}
		m.active = true
		m.lastErr = ""
		return nil
	}
	if recordErr != nil && !errors.Is(recordErr, os.ErrNotExist) {
		return m.fail(fmt.Errorf("读取系统代理恢复记录失败：%w", recordErr))
	}
	// A changed current value means another program took ownership after an
	// earlier crash. Preserve that value as the new restore point.
	record = recoveryRecord{Original: current, Applied: desired}
	if err := m.saveRecovery(record); err != nil {
		return m.fail(fmt.Errorf("保存系统代理恢复记录失败：%w", err))
	}
	if err := m.store.Write(desired); err != nil {
		m.forceRestore(record)
		return m.fail(fmt.Errorf("设置 Windows 系统代理失败：%w", err))
	}
	if err := m.notifier.Notify(); err != nil {
		m.forceRestore(record)
		return m.fail(fmt.Errorf("刷新 Windows 系统代理失败：%w", err))
	}
	m.active = true
	m.lastErr = ""
	return nil
}

func (m *controller) Restore() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.supported || m.store == nil || m.recoveryPath == "" {
		m.active = false
		return nil
	}
	record, err := m.loadRecovery()
	if errors.Is(err, os.ErrNotExist) {
		m.active = false
		m.lastErr = ""
		return nil
	}
	if err != nil {
		return m.fail(fmt.Errorf("读取系统代理恢复记录失败：%w", err))
	}
	current, err := m.store.Read()
	if err != nil {
		return m.fail(fmt.Errorf("读取 Windows 系统代理失败：%w", err))
	}
	if current == record.Applied {
		if err := m.store.Write(record.Original); err != nil {
			return m.fail(fmt.Errorf("恢复 Windows 系统代理失败：%w", err))
		}
	}
	if m.notifier != nil {
		if err := m.notifier.Notify(); err != nil {
			return m.fail(fmt.Errorf("刷新 Windows 系统代理失败：%w", err))
		}
	}
	if err := os.Remove(m.recoveryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return m.fail(fmt.Errorf("清理系统代理恢复记录失败：%w", err))
	}
	m.active = false
	m.lastErr = ""
	return nil
}

func (m *controller) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{Supported: m.supported, Active: m.active, Error: m.lastErr}
}

func (m *controller) fail(err error) error {
	m.active = false
	m.lastErr = err.Error()
	return err
}

func (m *controller) forceRestore(record recoveryRecord) {
	if m.store.Write(record.Original) == nil && (m.notifier == nil || m.notifier.Notify() == nil) {
		_ = os.Remove(m.recoveryPath)
	}
	m.active = false
}

func (m *controller) loadRecovery() (recoveryRecord, error) {
	contents, err := os.ReadFile(m.recoveryPath)
	if err != nil {
		return recoveryRecord{}, err
	}
	var record recoveryRecord
	if err := json.Unmarshal(contents, &record); err != nil {
		return recoveryRecord{}, err
	}
	return record, nil
}

func (m *controller) saveRecovery(record recoveryRecord) error {
	contents, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(filepath.Dir(m.recoveryPath), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(m.recoveryPath), "system-proxy-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, m.recoveryPath)
}
