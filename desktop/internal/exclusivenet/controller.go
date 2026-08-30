package exclusivenet

import (
	"context"
	"errors"
	"log/slog"
)

var (
	ErrAdministratorRequired = errors.New("需要管理员权限才能开启严格代理模式")
	ErrElevationCanceled     = errors.New("已取消管理员授权")
)

// Target identifies the Windows interface protected by exclusive mode.
type Target struct {
	ID             string
	Name           string
	InterfaceIndex int
}

// Status separates the saved preference from the currently installed policy.
type Status struct {
	Supported      bool   `json:"supported"`
	Enabled        bool   `json:"enabled"`
	Active         bool   `json:"active"`
	InterfaceIndex int    `json:"interfaceIndex,omitempty"`
	InterfaceName  string `json:"interfaceName,omitempty"`
	Error          string `json:"error,omitempty"`
}

type Controller interface {
	Configure(enabled bool)
	Reconcile(ctx context.Context, target *Target) error
	Check(ctx context.Context)
	Status() Status
	Close()
}

func New(logger *slog.Logger) Controller {
	return newController(logger)
}

// RunHelperIfRequested handles the private elevated-helper command line.
func RunHelperIfRequested(arguments []string) (handled bool, exitCode int) {
	return runHelperIfRequested(arguments)
}
