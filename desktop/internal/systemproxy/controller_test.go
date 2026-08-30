package systemproxy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeStore struct {
	value      Settings
	writeErr   error
	writeCount int
}

func (s *fakeStore) Read() (Settings, error) { return s.value, nil }
func (s *fakeStore) Write(value Settings) error {
	s.writeCount++
	if s.writeErr != nil {
		return s.writeErr
	}
	s.value = value
	return nil
}

type fakeNotifier struct {
	err   error
	calls int
}

func (n *fakeNotifier) Notify() error {
	n.calls++
	return n.err
}

func TestEnableAndRestorePreservesOriginalSettings(t *testing.T) {
	original := Settings{
		ProxyEnable:   OptionalInteger{Present: true, Value: 1},
		ProxyServer:   OptionalString{Present: true, Value: "existing:8080"},
		ProxyOverride: OptionalString{Present: true, Value: "intranet"},
	}
	store := &fakeStore{value: original}
	notifier := &fakeNotifier{}
	path := filepath.Join(t.TempDir(), "recovery.json")
	manager := newController(store, notifier, path, true)

	if err := manager.Enable(); err != nil {
		t.Fatal(err)
	}
	if store.value != desiredSettings() || !manager.Status().Active {
		t.Fatalf("proxy was not enabled: value=%+v status=%+v", store.value, manager.Status())
	}
	if err := manager.Restore(); err != nil {
		t.Fatal(err)
	}
	if store.value != original || manager.Status().Active {
		t.Fatalf("proxy was not restored: value=%+v status=%+v", store.value, manager.Status())
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery file still exists: %v", err)
	}
}

func TestRestoreRemovesValuesThatWereOriginallyAbsent(t *testing.T) {
	store := &fakeStore{}
	manager := newController(store, &fakeNotifier{}, filepath.Join(t.TempDir(), "recovery.json"), true)
	if err := manager.Enable(); err != nil {
		t.Fatal(err)
	}
	if err := manager.Restore(); err != nil {
		t.Fatal(err)
	}
	if store.value != (Settings{}) {
		t.Fatalf("absent values were not restored: %+v", store.value)
	}
}

func TestEnableReusesCrashRecoveryWithoutOverwritingOriginal(t *testing.T) {
	original := Settings{ProxyServer: OptionalString{Present: true, Value: "existing:8080"}}
	path := filepath.Join(t.TempDir(), "recovery.json")
	firstStore := &fakeStore{value: original}
	first := newController(firstStore, &fakeNotifier{}, path, true)
	if err := first.Enable(); err != nil {
		t.Fatal(err)
	}

	secondStore := &fakeStore{value: desiredSettings()}
	second := newController(secondStore, &fakeNotifier{}, path, true)
	if err := second.Enable(); err != nil {
		t.Fatal(err)
	}
	if err := second.Restore(); err != nil {
		t.Fatal(err)
	}
	if secondStore.value != original {
		t.Fatalf("crash recovery original was overwritten: %+v", secondStore.value)
	}
}

func TestRestoreDoesNotOverwriteProxyChangedByAnotherProgram(t *testing.T) {
	store := &fakeStore{value: Settings{ProxyEnable: OptionalInteger{Present: true}}}
	path := filepath.Join(t.TempDir(), "recovery.json")
	manager := newController(store, &fakeNotifier{}, path, true)
	if err := manager.Enable(); err != nil {
		t.Fatal(err)
	}
	external := Settings{ProxyEnable: OptionalInteger{Present: true, Value: 1}, ProxyServer: OptionalString{Present: true, Value: "other:9000"}}
	store.value = external
	if err := manager.Restore(); err != nil {
		t.Fatal(err)
	}
	if store.value != external {
		t.Fatalf("external proxy setting was overwritten: %+v", store.value)
	}
}

func TestEnableRollsBackWhenNotificationFails(t *testing.T) {
	original := Settings{ProxyServer: OptionalString{Present: true, Value: "existing:8080"}}
	store := &fakeStore{value: original}
	notifier := &fakeNotifier{err: errors.New("notify failed")}
	path := filepath.Join(t.TempDir(), "recovery.json")
	manager := newController(store, notifier, path, true)
	if err := manager.Enable(); err == nil {
		t.Fatal("expected enable failure")
	}
	if store.value != original || manager.Status().Active {
		t.Fatalf("failed enable was not rolled back: value=%+v status=%+v", store.value, manager.Status())
	}
}

func TestRestoreRetriesNotificationBeforeRemovingRecovery(t *testing.T) {
	original := Settings{ProxyServer: OptionalString{Present: true, Value: "existing:8080"}}
	store := &fakeStore{value: original}
	notifier := &fakeNotifier{}
	path := filepath.Join(t.TempDir(), "recovery.json")
	manager := newController(store, notifier, path, true)
	if err := manager.Enable(); err != nil {
		t.Fatal(err)
	}
	notifier.err = errors.New("notify failed")
	if err := manager.Restore(); err == nil {
		t.Fatal("expected restore notification failure")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("recovery file was removed before notification succeeded: %v", err)
	}
	notifier.err = nil
	if err := manager.Restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery file still exists after retry: %v", err)
	}
}
