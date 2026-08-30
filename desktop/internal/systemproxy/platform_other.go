//go:build !windows

package systemproxy

func platformSupported() bool      { return false }
func platformStore() settingsStore { return nil }
func platformNotifier() notifier   { return nil }
