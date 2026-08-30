//go:build windows

package systemproxy

import (
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

type registryStore struct{}

func platformSupported() bool      { return true }
func platformStore() settingsStore { return registryStore{} }
func platformNotifier() notifier   { return winINetNotifier{} }

func (registryStore) Read() (Settings, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		return Settings{}, err
	}
	defer key.Close()
	enabled, err := readInteger(key, "ProxyEnable")
	if err != nil {
		return Settings{}, err
	}
	server, err := readString(key, "ProxyServer")
	if err != nil {
		return Settings{}, err
	}
	override, err := readString(key, "ProxyOverride")
	if err != nil {
		return Settings{}, err
	}
	return Settings{ProxyEnable: enabled, ProxyServer: server, ProxyOverride: override}, nil
}

func (registryStore) Write(value Settings) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := writeInteger(key, "ProxyEnable", value.ProxyEnable); err != nil {
		return err
	}
	if err := writeString(key, "ProxyServer", value.ProxyServer); err != nil {
		return err
	}
	return writeString(key, "ProxyOverride", value.ProxyOverride)
}

func readInteger(key registry.Key, name string) (OptionalInteger, error) {
	value, _, err := key.GetIntegerValue(name)
	if err == registry.ErrNotExist {
		return OptionalInteger{}, nil
	}
	if err != nil {
		return OptionalInteger{}, fmt.Errorf("read %s: %w", name, err)
	}
	return OptionalInteger{Present: true, Value: uint32(value)}, nil
}

func readString(key registry.Key, name string) (OptionalString, error) {
	value, _, err := key.GetStringValue(name)
	if err == registry.ErrNotExist {
		return OptionalString{}, nil
	}
	if err != nil {
		return OptionalString{}, fmt.Errorf("read %s: %w", name, err)
	}
	return OptionalString{Present: true, Value: value}, nil
}

func writeInteger(key registry.Key, name string, value OptionalInteger) error {
	if !value.Present {
		if err := key.DeleteValue(name); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("delete %s: %w", name, err)
		}
		return nil
	}
	if err := key.SetDWordValue(name, value.Value); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func writeString(key registry.Key, name string, value OptionalString) error {
	if !value.Present {
		if err := key.DeleteValue(name); err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("delete %s: %w", name, err)
		}
		return nil
	}
	if err := key.SetStringValue(name, value.Value); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

type winINetNotifier struct{}

var internetSetOption = windows.NewLazySystemDLL("wininet.dll").NewProc("InternetSetOptionW")

func (winINetNotifier) Notify() error {
	for _, option := range []uintptr{39, 37} { // SETTINGS_CHANGED, then REFRESH.
		result, _, callErr := internetSetOption.Call(0, option, 0, 0)
		if result == 0 {
			return callErr
		}
	}
	return nil
}
