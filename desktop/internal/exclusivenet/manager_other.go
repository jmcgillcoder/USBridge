//go:build !windows

package exclusivenet

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

type unsupportedController struct {
	mu      sync.Mutex
	enabled bool
}

func newController(*slog.Logger) Controller { return &unsupportedController{} }

func (m *unsupportedController) Configure(enabled bool) {
	m.mu.Lock()
	m.enabled = enabled
	m.mu.Unlock()
}

func (m *unsupportedController) Reconcile(context.Context, *Target) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabled {
		return errors.New("严格代理模式仅支持 Windows")
	}
	return nil
}

func (m *unsupportedController) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{Supported: false, Enabled: m.enabled}
}

func (*unsupportedController) Check(context.Context) {}

func (*unsupportedController) Close() {}

func runHelperIfRequested([]string) (bool, int) { return false, 0 }
