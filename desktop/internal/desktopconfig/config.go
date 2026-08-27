package desktopconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/usbridge/usbridge/desktop/internal/proxy"
)

const (
	proxyUsername = "usbridge"
	proxyPassword = "usbridge_pw"
)

type Settings struct {
	AuthenticatedProxy   proxy.Credentials `json:"authenticatedProxy"`
	ExclusiveModeEnabled bool              `json:"exclusiveModeEnabled"`
}

func LoadOrCreateProxyCredentials() (proxy.Credentials, error) {
	value, err := LoadOrCreateSettings()
	return value.AuthenticatedProxy, err
}

func LoadOrCreateSettings() (Settings, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return Settings{}, fmt.Errorf("locate user configuration directory: %w", err)
	}
	return LoadOrCreateSettingsAt(filepath.Join(configDirectory, "USBridge", "config.json"))
}

func LoadOrCreateProxyCredentialsAt(path string) (proxy.Credentials, error) {
	value, err := LoadOrCreateSettingsAt(path)
	return value.AuthenticatedProxy, err
}

func LoadOrCreateSettingsAt(path string) (Settings, error) {
	value, err := loadSettings(path)
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Settings{}, err
	}

	value = defaultSettings()
	if err := writeSettings(path, value); err != nil {
		// Another process may have created the file while this process generated it.
		if loaded, loadErr := loadSettings(path); loadErr == nil {
			return loaded, nil
		}
		return Settings{}, err
	}
	return value, nil
}

func SaveExclusiveMode(enabled bool) error {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate user configuration directory: %w", err)
	}
	return SaveExclusiveModeAt(filepath.Join(configDirectory, "USBridge", "config.json"), enabled)
}

func SaveExclusiveModeAt(path string, enabled bool) error {
	value, err := LoadOrCreateSettingsAt(path)
	if err != nil {
		return err
	}
	value.ExclusiveModeEnabled = enabled
	return writeSettings(path, value)
}

func loadSettings(path string) (Settings, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, err
	}
	var value Settings
	if err := json.Unmarshal(contents, &value); err != nil {
		return Settings{}, fmt.Errorf("read proxy configuration: %w", err)
	}
	if err := value.AuthenticatedProxy.Validate(); err != nil {
		return Settings{}, fmt.Errorf("read proxy configuration: %w", err)
	}
	return value, nil
}

func defaultProxyCredentials() proxy.Credentials {
	return proxy.Credentials{
		Username: proxyUsername,
		Password: proxyPassword,
	}
}

func defaultSettings() Settings {
	return Settings{AuthenticatedProxy: defaultProxyCredentials()}
}

func writeSettings(path string, value Settings) error {
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode proxy configuration: %w", err)
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create proxy configuration directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary proxy configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("protect proxy configuration: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write proxy configuration: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("flush proxy configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close proxy configuration: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("save proxy configuration: %w", err)
	}
	return nil
}
