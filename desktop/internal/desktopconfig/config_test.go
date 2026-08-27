package desktopconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateProxyCredentialsPersistsPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "USBridge", "config.json")
	first, err := LoadOrCreateProxyCredentialsAt(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateProxyCredentialsAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("credentials changed after reload: first=%+v second=%+v", first, second)
	}
	if first.Username != proxyUsername || first.Password != proxyPassword {
		t.Fatalf("unexpected generated credentials: %+v", first)
	}
}

func TestLoadOrCreateProxyCredentialsRejectsDamagedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"authenticatedProxy":{"username":"","password":""}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateProxyCredentialsAt(path); err == nil {
		t.Fatal("expected damaged configuration to be rejected")
	}
}

func TestSaveExclusiveModePreservesProxyCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "USBridge", "config.json")
	initial, err := LoadOrCreateSettingsAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if initial.ExclusiveModeEnabled {
		t.Fatal("exclusive mode should be opt-in for a new configuration")
	}
	if err := SaveExclusiveModeAt(path, true); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOrCreateSettingsAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.ExclusiveModeEnabled {
		t.Fatal("exclusive mode setting was not persisted")
	}
	if loaded.AuthenticatedProxy != initial.AuthenticatedProxy {
		t.Fatalf("proxy credentials changed: before=%+v after=%+v", initial.AuthenticatedProxy, loaded.AuthenticatedProxy)
	}
}

func TestLoadSettingsMigratesConfigurationWithoutExclusiveMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	contents := []byte(`{"authenticatedProxy":{"username":"usbridge","password":"usbridge_pw"}}`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOrCreateSettingsAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ExclusiveModeEnabled {
		t.Fatal("legacy configuration unexpectedly enabled exclusive mode")
	}
}
