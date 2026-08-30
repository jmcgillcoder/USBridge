package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/adapter"
	"github.com/usbridge/usbridge/desktop/internal/androidclient"
	"github.com/usbridge/usbridge/desktop/internal/controlapi"
	"github.com/usbridge/usbridge/desktop/internal/desktopconfig"
	"github.com/usbridge/usbridge/desktop/internal/exclusivenet"
	"github.com/usbridge/usbridge/desktop/internal/proxy"
	"github.com/usbridge/usbridge/desktop/internal/proxy/multiplex"
	"github.com/usbridge/usbridge/desktop/internal/routing"
	"github.com/usbridge/usbridge/desktop/internal/service"
	"github.com/usbridge/usbridge/desktop/internal/systemproxy"
	"github.com/usbridge/usbridge/desktop/internal/traffic"
	"github.com/usbridge/usbridge/desktop/internal/updater"
	"github.com/usbridge/usbridge/desktop/internal/version"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	defaultProxyListen              = "127.0.0.1:18080"
	defaultAuthenticatedProxyListen = "127.0.0.1:18081"
	defaultControlListen            = "127.0.0.1:18082"
	defaultAndroidPort              = 17890
	reconnectOperationTimeout       = 210 * time.Second
	publicIPOperationTimeout        = 45 * time.Second
	cellularUpstreamTimeout         = 75 * time.Second
	startTetheringOperationTimeout  = 90 * time.Second
	stopTetheringOperationTimeout   = 60 * time.Second
	ipModeOperationTimeout          = 15 * time.Second
)

type App struct {
	mu                   sync.RWMutex
	phoneOperationMu     sync.Mutex
	exclusiveOperationMu sync.Mutex
	updateOperationMu    sync.Mutex

	ctx          context.Context
	cancel       context.CancelFunc
	registry     *adapter.Registry
	policy       *routing.Policy
	controller   *service.Controller
	meter        *traffic.Meter
	logger       *slog.Logger
	exclusive    exclusivenet.Controller
	systemProxy  systemproxy.Controller
	updateClient *updater.Client
	latestUpdate *updater.Info

	proxyListener   net.Listener
	proxyServer     *multiplex.Server
	proxyRunning    bool
	networkChanging bool
	lastError       string

	authenticatedProxyListener    net.Listener
	authenticatedProxyServer      *multiplex.Server
	authenticatedProxyCredentials *proxy.Credentials
	authenticatedProxyRunning     bool
	authenticatedProxyError       string

	controlListener net.Listener
	controlServer   *http.Server
	controlRunning  bool
	controlError    string
}

type DesktopSnapshot struct {
	service.Snapshot
	ProxyRunning              bool   `json:"proxyRunning"`
	NetworkChanging           bool   `json:"networkChanging"`
	LastError                 string `json:"lastError,omitempty"`
	ControlListen             string `json:"controlListen"`
	ControlRunning            bool   `json:"controlRunning"`
	ControlError              string `json:"controlError,omitempty"`
	AuthenticatedProxyListen  string `json:"authenticatedProxyListen"`
	AuthenticatedProxyRunning bool   `json:"authenticatedProxyRunning"`
	AuthenticatedProxyError   string `json:"authenticatedProxyError,omitempty"`
	ExclusiveModeSupported    bool   `json:"exclusiveModeSupported"`
	ExclusiveModeEnabled      bool   `json:"exclusiveModeEnabled"`
	ExclusiveModeActive       bool   `json:"exclusiveModeActive"`
	ExclusiveModeInterface    string `json:"exclusiveModeInterface,omitempty"`
	ExclusiveModeError        string `json:"exclusiveModeError,omitempty"`
	SystemProxyActive         bool   `json:"systemProxyActive"`
	SystemProxyError          string `json:"systemProxyError,omitempty"`
}

type AuthenticatedProxyAccess struct {
	Listen    string `json:"listen"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	HTTPURL   string `json:"httpUrl"`
	SOCKS5URL string `json:"socks5Url"`
}

func NewApp() *App {
	return &App{logger: slog.New(slog.NewTextHandler(os.Stdout, nil))}
}

func (a *App) startup(wailsContext context.Context) {
	a.mu.Lock()
	a.ctx, a.cancel = context.WithCancel(wailsContext)
	a.registry = adapter.NewRegistry(adapter.NewProvider())
	a.policy = routing.NewPolicy(routing.IPModeAuto)
	a.meter = traffic.NewMeter()
	a.exclusive = exclusivenet.New(a.logger)
	a.systemProxy = systemproxy.New()
	a.updateClient = updater.NewClient()
	a.mu.Unlock()
	if err := a.systemProxy.Restore(); err != nil {
		a.logger.Warn("failed to recover Windows system proxy", "error", err)
	}

	refreshContext, cancel := context.WithTimeout(a.ctx, 12*time.Second)
	_, err := a.registry.Refresh(refreshContext)
	cancel()
	if err != nil {
		a.setError(fmt.Sprintf("网卡扫描失败：%v", err))
	}

	endpoint := func() (string, error) {
		selected, ok := a.registry.Selected()
		if !ok {
			return "", routing.ErrNoUSBAdapter
		}
		return androidclient.EndpointFromAdapter(selected, defaultAndroidPort)
	}
	androidControl := androidclient.New(endpoint, os.Getenv("USBRIDGE_TOKEN"))
	controller := &service.Controller{
		Adapters:    a.registry,
		Routing:     a.policy,
		Android:     androidControl,
		AndroidPort: defaultAndroidPort,
		ProxyListen: defaultProxyListen,
		Meter:       a.meter,
	}
	a.mu.Lock()
	a.controller = controller
	a.mu.Unlock()

	settings, settingsErr := desktopconfig.LoadOrCreateSettings()
	if settingsErr != nil {
		a.setAuthenticatedProxyError(fmt.Sprintf("账密代理配置不可用：%v", settingsErr))
	} else {
		a.mu.Lock()
		a.authenticatedProxyCredentials = &settings.AuthenticatedProxy
		a.authenticatedProxyError = ""
		a.mu.Unlock()
		a.exclusive.Configure(settings.ExclusiveModeEnabled)
	}

	a.startProxyServers()
	a.startControlServer()
	go a.registry.Run(a.ctx, 10*time.Second, func(selected *adapter.Adapter) {
		if selected == nil {
			a.logger.Info("waiting for USB tethering adapter")
			a.resetProxyConnections("USB tethering adapter disconnected")
			a.reconcileExclusiveMode(nil)
			return
		}
		a.logger.Info("USB tethering adapter selected", "name", selected.Name)
		a.resetProxyConnections("USB tethering adapter changed")
		a.reconcileExclusiveMode(selected)
	})
	go a.monitorAndroidStatus()
	go a.monitorExclusiveMode()
}

func (a *App) monitorAndroidStatus() {
	check := func() {
		ctx, cancel := context.WithTimeout(a.context(), 2500*time.Millisecond)
		defer cancel()
		_ = a.controller.RefreshAndroidStatus(ctx)
	}
	check()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.context().Done():
			return
		case <-ticker.C:
			check()
		}
	}
}

func (a *App) monitorExclusiveMode() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.context().Done():
			return
		case <-ticker.C:
			a.exclusiveOperationMu.Lock()
			a.mu.RLock()
			manager := a.exclusive
			systemProxy := a.systemProxy
			a.mu.RUnlock()
			if manager == nil {
				a.exclusiveOperationMu.Unlock()
				continue
			}
			ctx, cancel := context.WithTimeout(a.context(), 2*time.Second)
			manager.Check(ctx)
			cancel()
			if systemProxy != nil {
				if manager.Status().Active {
					if err := systemProxy.Enable(); err != nil {
						cleanupContext, cleanupCancel := context.WithTimeout(a.context(), 5*time.Second)
						_ = manager.Reconcile(cleanupContext, nil)
						cleanupCancel()
						a.logger.Warn("Windows system proxy takeover stopped", "error", err)
					}
				} else if err := systemProxy.Restore(); err != nil {
					a.logger.Warn("failed to restore Windows system proxy", "error", err)
				}
			}
			a.exclusiveOperationMu.Unlock()
		}
	}
}

func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	cancel := a.cancel
	proxyListener := a.proxyListener
	authenticatedProxyListener := a.authenticatedProxyListener
	controlServer := a.controlServer
	controlListener := a.controlListener
	exclusive := a.exclusive
	systemProxy := a.systemProxy
	a.proxyRunning = false
	a.authenticatedProxyRunning = false
	a.controlRunning = false
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	a.exclusiveOperationMu.Lock()
	if systemProxy != nil {
		if err := systemProxy.Restore(); err != nil {
			a.logger.Warn("failed to restore Windows system proxy during shutdown", "error", err)
		}
	}
	if exclusive != nil {
		exclusive.Close()
	}
	a.exclusiveOperationMu.Unlock()
	if proxyListener != nil {
		_ = proxyListener.Close()
	}
	if authenticatedProxyListener != nil {
		_ = authenticatedProxyListener.Close()
	}
	if controlServer != nil {
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = controlServer.Shutdown(shutdownContext)
		shutdownCancel()
	} else if controlListener != nil {
		_ = controlListener.Close()
	}
}

func (a *App) startProxyServers() {
	dialer := routing.NewBoundDialer(a.registry, a.policy)
	dialer.Gate = a.requireProxyReady
	a.startOpenProxyServer(dialer)

	a.mu.RLock()
	credentials := a.authenticatedProxyCredentials
	a.mu.RUnlock()
	if credentials != nil {
		a.startAuthenticatedProxyServer(dialer, *credentials)
	}
}

func (a *App) startOpenProxyServer(dialer proxy.ContextDialer) {
	proxyListener, err := net.Listen("tcp", defaultProxyListen)
	if err != nil {
		a.setError(fmt.Sprintf("统一代理端口不可用：%v", err))
		return
	}

	a.mu.Lock()
	a.proxyListener = proxyListener
	a.proxyRunning = true
	a.lastError = ""
	a.mu.Unlock()

	proxyServer := multiplex.New(dialer, a.meter, a.logger)
	a.mu.Lock()
	a.proxyServer = proxyServer
	a.mu.Unlock()
	go func() {
		if serveErr := proxyServer.Serve(a.ctx, proxyListener); serveErr != nil {
			a.mu.Lock()
			a.proxyRunning = false
			a.mu.Unlock()
			a.setError(fmt.Sprintf("统一代理服务已停止：%v", serveErr))
		}
	}()
}

func (a *App) startAuthenticatedProxyServer(dialer proxy.ContextDialer, credentials proxy.Credentials) {
	proxyServer, err := multiplex.NewAuthenticated(dialer, a.meter, a.logger, credentials)
	if err != nil {
		a.setAuthenticatedProxyError(fmt.Sprintf("账密代理配置不可用：%v", err))
		return
	}
	listener, err := net.Listen("tcp", defaultAuthenticatedProxyListen)
	if err != nil {
		a.setAuthenticatedProxyError(fmt.Sprintf("账密代理端口不可用：%v", err))
		return
	}

	a.mu.Lock()
	a.authenticatedProxyListener = listener
	a.authenticatedProxyServer = proxyServer
	a.authenticatedProxyRunning = true
	a.authenticatedProxyError = ""
	a.mu.Unlock()

	go func() {
		if serveErr := proxyServer.Serve(a.ctx, listener); serveErr != nil {
			a.mu.Lock()
			a.authenticatedProxyRunning = false
			a.mu.Unlock()
			a.setAuthenticatedProxyError(fmt.Sprintf("账密代理服务已停止：%v", serveErr))
		}
	}()
}

func (a *App) startControlServer() {
	listener, err := net.Listen("tcp4", defaultControlListen)
	if err != nil {
		a.setControlError(fmt.Sprintf("本机控制接口不可用：%v", err))
		return
	}

	api := controlapi.New(controlapi.Dependencies{
		Status: func() any { return a.GetSnapshot() },
		Traffic: func() traffic.Snapshot {
			a.mu.RLock()
			meter := a.meter
			a.mu.RUnlock()
			if meter == nil {
				return traffic.Snapshot{}
			}
			return meter.Snapshot()
		},
		ReconnectMobile: func(requestContext context.Context) (androidclient.OperationResponse, error) {
			operationContext, cancel := context.WithTimeout(requestContext, reconnectOperationTimeout)
			defer cancel()
			return a.reconnectMobile(operationContext)
		},
		RefreshPublicIP: func(requestContext context.Context) (androidclient.OperationResponse, error) {
			operationContext, cancel := context.WithTimeout(requestContext, publicIPOperationTimeout)
			defer cancel()
			return a.refreshPublicIP(operationContext)
		},
		ForceCellularUpstream: func(requestContext context.Context) (androidclient.OperationResponse, error) {
			operationContext, cancel := context.WithTimeout(requestContext, cellularUpstreamTimeout)
			defer cancel()
			return a.forceCellularUpstream(operationContext)
		},
		StartTethering: func(requestContext context.Context) (androidclient.OperationResponse, error) {
			operationContext, cancel := context.WithTimeout(requestContext, startTetheringOperationTimeout)
			defer cancel()
			return a.startTethering(operationContext)
		},
		StopTethering: func(requestContext context.Context) (androidclient.OperationResponse, error) {
			operationContext, cancel := context.WithTimeout(requestContext, stopTetheringOperationTimeout)
			defer cancel()
			return a.stopTethering(operationContext)
		},
		SetIPMode: func(requestContext context.Context, value string) (androidclient.OperationResponse, error) {
			operationContext, cancel := context.WithTimeout(requestContext, ipModeOperationTimeout)
			defer cancel()
			return a.setIPMode(operationContext, value)
		},
	})
	server := controlapi.NewHTTPServer(api)

	a.mu.Lock()
	a.controlListener = listener
	a.controlServer = server
	a.controlRunning = true
	a.controlError = ""
	a.mu.Unlock()

	go func() {
		serveErr := server.Serve(listener)
		a.mu.Lock()
		a.controlRunning = false
		stopping := a.ctx != nil && a.ctx.Err() != nil
		a.mu.Unlock()
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && !errors.Is(serveErr, net.ErrClosed) && !stopping {
			a.setControlError(fmt.Sprintf("本机控制接口已停止：%v", serveErr))
		}
	}()
}

func (a *App) GetSnapshot() DesktopSnapshot {
	a.mu.RLock()
	controller := a.controller
	running := a.proxyRunning
	networkChanging := a.networkChanging
	lastError := a.lastError
	controlRunning := a.controlRunning
	controlError := a.controlError
	authenticatedProxyRunning := a.authenticatedProxyRunning
	authenticatedProxyError := a.authenticatedProxyError
	exclusive := a.exclusive
	systemProxy := a.systemProxy
	a.mu.RUnlock()
	exclusiveStatus := exclusivenet.Status{}
	if exclusive != nil {
		exclusiveStatus = exclusive.Status()
	}
	systemProxyStatus := systemproxy.Status{}
	if systemProxy != nil {
		systemProxyStatus = systemProxy.Status()
	}
	if controller == nil {
		return DesktopSnapshot{
			ProxyRunning:              false,
			NetworkChanging:           networkChanging,
			LastError:                 "服务正在启动",
			ControlListen:             defaultControlListen,
			ControlRunning:            controlRunning,
			ControlError:              controlError,
			AuthenticatedProxyListen:  defaultAuthenticatedProxyListen,
			AuthenticatedProxyRunning: authenticatedProxyRunning,
			AuthenticatedProxyError:   authenticatedProxyError,
			ExclusiveModeSupported:    exclusiveStatus.Supported,
			ExclusiveModeEnabled:      exclusiveStatus.Enabled,
			ExclusiveModeActive:       exclusiveStatus.Active,
			ExclusiveModeInterface:    exclusiveStatus.InterfaceName,
			ExclusiveModeError:        exclusiveStatus.Error,
			SystemProxyActive:         systemProxyStatus.Active,
			SystemProxyError:          systemProxyStatus.Error,
		}
	}
	return DesktopSnapshot{
		Snapshot:                  controller.Snapshot(),
		ProxyRunning:              running,
		NetworkChanging:           networkChanging,
		LastError:                 lastError,
		ControlListen:             defaultControlListen,
		ControlRunning:            controlRunning,
		ControlError:              controlError,
		AuthenticatedProxyListen:  defaultAuthenticatedProxyListen,
		AuthenticatedProxyRunning: authenticatedProxyRunning,
		AuthenticatedProxyError:   authenticatedProxyError,
		ExclusiveModeSupported:    exclusiveStatus.Supported,
		ExclusiveModeEnabled:      exclusiveStatus.Enabled,
		ExclusiveModeActive:       exclusiveStatus.Active,
		ExclusiveModeInterface:    exclusiveStatus.InterfaceName,
		ExclusiveModeError:        exclusiveStatus.Error,
		SystemProxyActive:         systemProxyStatus.Active,
		SystemProxyError:          systemProxyStatus.Error,
	}
}

func (a *App) GetAuthenticatedProxyAccess() (AuthenticatedProxyAccess, error) {
	a.mu.RLock()
	credentials := a.authenticatedProxyCredentials
	a.mu.RUnlock()
	if credentials == nil {
		return AuthenticatedProxyAccess{}, errors.New("账密代理尚未准备好")
	}
	httpURL := &url.URL{
		Scheme: "http",
		Host:   defaultAuthenticatedProxyListen,
		User:   url.UserPassword(credentials.Username, credentials.Password),
	}
	socks5URL := &url.URL{
		Scheme: "socks5",
		Host:   defaultAuthenticatedProxyListen,
		User:   url.UserPassword(credentials.Username, credentials.Password),
	}
	return AuthenticatedProxyAccess{
		Listen:    defaultAuthenticatedProxyListen,
		Username:  credentials.Username,
		Password:  credentials.Password,
		HTTPURL:   httpURL.String(),
		SOCKS5URL: socks5URL.String(),
	}, nil
}

func (a *App) CheckForUpdates() (updater.Info, error) {
	a.updateOperationMu.Lock()
	defer a.updateOperationMu.Unlock()
	a.mu.RLock()
	client := a.updateClient
	a.mu.RUnlock()
	if client == nil {
		return updater.Info{}, errors.New("更新服务正在准备")
	}
	ctx, cancel := context.WithTimeout(a.context(), 35*time.Second)
	defer cancel()
	info, err := client.Check(ctx, version.Version)
	if err != nil {
		return updater.Info{}, err
	}
	a.mu.Lock()
	if info.Available {
		copy := info
		a.latestUpdate = &copy
	} else {
		a.latestUpdate = nil
	}
	a.mu.Unlock()
	return info, nil
}

func (a *App) InstallUpdate() error {
	a.updateOperationMu.Lock()
	defer a.updateOperationMu.Unlock()
	a.mu.RLock()
	client := a.updateClient
	info := a.latestUpdate
	a.mu.RUnlock()
	if client == nil || info == nil || !info.Available {
		return errors.New("请先检查新版本")
	}
	ctx, cancel := context.WithTimeout(a.context(), 10*time.Minute)
	path, err := client.Download(ctx, *info)
	cancel()
	if err != nil {
		return err
	}
	if err := updater.LaunchApply(path); err != nil {
		_ = os.RemoveAll(filepath.Dir(path))
		return err
	}
	wailsruntime.Quit(a.context())
	return nil
}

func (a *App) OpenProjectPage() {
	wailsruntime.BrowserOpenURL(a.context(), updater.RepositoryURL)
}

func (a *App) RefreshAdapters() error {
	ctx, cancel := context.WithTimeout(a.context(), 12*time.Second)
	defer cancel()
	_, err := a.registry.Refresh(ctx)
	if err == nil {
		a.resetProxyConnections("USB tethering adapter refreshed")
		selected, ok := a.registry.Selected()
		if ok {
			a.reconcileExclusiveMode(&selected)
		} else {
			a.reconcileExclusiveMode(nil)
		}
	}
	return err
}

func (a *App) SelectAdapter(selector string) error {
	err := a.controller.SelectAdapter(selector)
	if err == nil {
		a.resetProxyConnections("USB tethering adapter selection changed")
		selected, ok := a.registry.Selected()
		if ok {
			a.reconcileExclusiveMode(&selected)
		} else {
			a.reconcileExclusiveMode(nil)
		}
	}
	return err
}

func (a *App) SetExclusiveMode(enabled bool) error {
	a.exclusiveOperationMu.Lock()
	defer a.exclusiveOperationMu.Unlock()

	a.mu.RLock()
	manager := a.exclusive
	registry := a.registry
	systemProxy := a.systemProxy
	a.mu.RUnlock()
	if manager == nil || registry == nil || systemProxy == nil {
		return errors.New("严格代理模式正在准备")
	}

	previous := manager.Status().Enabled
	manager.Configure(enabled)
	selected, selectedOK := registry.Selected()
	var target *exclusivenet.Target
	if selectedOK {
		value := exclusiveTarget(selected)
		target = &value
	}
	ctx, cancel := context.WithTimeout(a.context(), 130*time.Second)
	err := manager.Reconcile(ctx, target)
	cancel()
	if err != nil {
		_ = systemProxy.Restore()
		if enabled && !previous {
			manager.Configure(false)
			cleanupContext, cleanupCancel := context.WithTimeout(a.context(), 5*time.Second)
			_ = manager.Reconcile(cleanupContext, nil)
			cleanupCancel()
		}
		return err
	}

	var systemProxyErr error
	if enabled && manager.Status().Active {
		if err := systemProxy.Enable(); err != nil {
			// Never leave the phone adapter blocked when Windows could not route
			// compatible applications through the local proxy.
			cleanupContext, cleanupCancel := context.WithTimeout(a.context(), 10*time.Second)
			_ = manager.Reconcile(cleanupContext, nil)
			cleanupCancel()
			systemProxyErr = err
		}
	} else if err := systemProxy.Restore(); err != nil {
		systemProxyErr = err
	}

	if err := desktopconfig.SaveExclusiveMode(enabled); err != nil {
		_ = systemProxy.Restore()
		manager.Configure(previous)
		rollbackContext, rollbackCancel := context.WithTimeout(a.context(), 10*time.Second)
		_ = manager.Reconcile(rollbackContext, target)
		rollbackCancel()
		if previous && manager.Status().Active {
			_ = systemProxy.Enable()
		}
		return fmt.Errorf("保存严格代理模式设置失败：%w", err)
	}
	if systemProxyErr != nil {
		return systemProxyErr
	}
	return nil
}

func (a *App) SetIPMode(value string) error {
	ctx, cancel := context.WithTimeout(a.context(), ipModeOperationTimeout)
	defer cancel()
	_, err := a.setIPMode(ctx, value)
	return err
}

func (a *App) ResetTraffic() {
	a.mu.RLock()
	meter := a.meter
	a.mu.RUnlock()
	if meter != nil {
		meter.Reset()
	}
}

func (a *App) ReconnectMobile() (androidclient.OperationResponse, error) {
	ctx, cancel := context.WithTimeout(a.context(), reconnectOperationTimeout)
	defer cancel()
	return a.reconnectMobile(ctx)
}

func (a *App) RefreshPublicIP() (androidclient.OperationResponse, error) {
	ctx, cancel := context.WithTimeout(a.context(), publicIPOperationTimeout)
	defer cancel()
	return a.refreshPublicIP(ctx)
}

func (a *App) refreshPublicIP(ctx context.Context) (androidclient.OperationResponse, error) {
	a.mu.RLock()
	controller := a.controller
	a.mu.RUnlock()
	if controller == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Windows service is still starting")
	}
	result, err := controller.RefreshPublicIP(ctx)
	if err == nil {
		statusContext, statusCancel := context.WithTimeout(a.context(), 4*time.Second)
		_ = controller.RefreshAndroidStatus(statusContext)
		statusCancel()
	}
	return result, err
}

func (a *App) reconnectMobile(ctx context.Context) (androidclient.OperationResponse, error) {
	a.phoneOperationMu.Lock()
	defer a.phoneOperationMu.Unlock()
	a.beginNetworkChange()
	defer a.endNetworkChange()

	a.mu.RLock()
	controller := a.controller
	a.mu.RUnlock()
	if controller == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Windows service is still starting")
	}
	result, err := controller.ReconnectMobile(ctx)
	a.refreshAdaptersAfterNetworkChange()
	if err == nil {
		statusContext, statusCancel := context.WithTimeout(a.context(), 4*time.Second)
		_ = controller.RefreshAndroidStatus(statusContext)
		statusCancel()
	}
	return result, err
}

func (a *App) StartTethering() (androidclient.OperationResponse, error) {
	ctx, cancel := context.WithTimeout(a.context(), startTetheringOperationTimeout)
	defer cancel()
	return a.startTethering(ctx)
}

func (a *App) startTethering(ctx context.Context) (androidclient.OperationResponse, error) {
	a.phoneOperationMu.Lock()
	defer a.phoneOperationMu.Unlock()
	a.beginNetworkChange()
	defer a.endNetworkChange()

	a.mu.RLock()
	controller := a.controller
	a.mu.RUnlock()
	if controller == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Windows service is still starting")
	}
	result, err := controller.StartTethering(ctx)
	a.refreshAdaptersAfterNetworkChange()
	if err == nil {
		statusContext, statusCancel := context.WithTimeout(a.context(), 4*time.Second)
		_ = controller.RefreshAndroidStatus(statusContext)
		statusCancel()
	}
	return result, err
}

func (a *App) StopTethering() (androidclient.OperationResponse, error) {
	ctx, cancel := context.WithTimeout(a.context(), stopTetheringOperationTimeout)
	defer cancel()
	return a.stopTethering(ctx)
}

func (a *App) stopTethering(ctx context.Context) (androidclient.OperationResponse, error) {
	a.phoneOperationMu.Lock()
	defer a.phoneOperationMu.Unlock()
	a.beginNetworkChange()
	defer a.endNetworkChange()

	a.mu.RLock()
	controller := a.controller
	a.mu.RUnlock()
	if controller == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Windows service is still starting")
	}
	result, err := controller.StopTethering(ctx)
	a.refreshAdaptersAfterNetworkChange()
	return result, err
}

func (a *App) forceCellularUpstream(ctx context.Context) (androidclient.OperationResponse, error) {
	a.phoneOperationMu.Lock()
	defer a.phoneOperationMu.Unlock()
	a.beginNetworkChange()
	defer a.endNetworkChange()

	a.mu.RLock()
	controller := a.controller
	a.mu.RUnlock()
	if controller == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Windows service is still starting")
	}
	result, err := controller.ForceCellularUpstream(ctx)
	a.refreshAdaptersAfterNetworkChange()
	if err == nil {
		statusContext, statusCancel := context.WithTimeout(a.context(), 4*time.Second)
		_ = controller.RefreshAndroidStatus(statusContext)
		statusCancel()
	}
	return result, err
}

func (a *App) setIPMode(ctx context.Context, value string) (androidclient.OperationResponse, error) {
	a.phoneOperationMu.Lock()
	defer a.phoneOperationMu.Unlock()
	a.beginNetworkChange()
	defer a.endNetworkChange()

	a.mu.RLock()
	controller := a.controller
	a.mu.RUnlock()
	if controller == nil {
		return androidclient.OperationResponse{}, fmt.Errorf("Windows service is still starting")
	}
	return controller.SetIPMode(ctx, value)
}

func (a *App) requireProxyReady() error {
	a.mu.RLock()
	changing := a.networkChanging
	controller := a.controller
	a.mu.RUnlock()
	if changing {
		return errors.New("mobile network change in progress")
	}
	if controller == nil {
		return errors.New("Android control service is not ready")
	}
	return controller.RequireCellularUpstream()
}

func (a *App) beginNetworkChange() {
	a.mu.Lock()
	a.networkChanging = true
	a.mu.Unlock()
	a.resetProxyConnections("mobile network change")
}

func (a *App) endNetworkChange() {
	a.mu.Lock()
	a.networkChanging = false
	a.mu.Unlock()
}

func (a *App) refreshAdaptersAfterNetworkChange() {
	a.mu.RLock()
	registry := a.registry
	a.mu.RUnlock()
	if registry == nil {
		return
	}
	ctx, cancel := context.WithTimeout(a.context(), 12*time.Second)
	defer cancel()
	if _, err := registry.Refresh(ctx); err != nil {
		a.logger.Warn("failed to refresh USB adapter after network change", "error", err)
		return
	}
	if selected, ok := registry.Selected(); ok {
		a.reconcileExclusiveMode(&selected)
	} else {
		a.reconcileExclusiveMode(nil)
	}
}

func (a *App) reconcileExclusiveMode(selected *adapter.Adapter) {
	a.exclusiveOperationMu.Lock()
	defer a.exclusiveOperationMu.Unlock()

	a.mu.RLock()
	manager := a.exclusive
	systemProxy := a.systemProxy
	a.mu.RUnlock()
	if manager == nil {
		return
	}
	var target *exclusivenet.Target
	if selected != nil {
		value := exclusiveTarget(*selected)
		target = &value
	}
	ctx, cancel := context.WithTimeout(a.context(), 130*time.Second)
	defer cancel()
	if err := manager.Reconcile(ctx, target); err != nil {
		if systemProxy != nil {
			_ = systemProxy.Restore()
		}
		a.logger.Warn("exclusive phone USB policy is not active", "error", err)
		return
	}
	if systemProxy == nil {
		return
	}
	if manager.Status().Active {
		if err := systemProxy.Enable(); err != nil {
			cleanupContext, cleanupCancel := context.WithTimeout(a.context(), 10*time.Second)
			_ = manager.Reconcile(cleanupContext, nil)
			cleanupCancel()
			a.logger.Warn("Windows system proxy takeover is not active", "error", err)
		}
	} else if err := systemProxy.Restore(); err != nil {
		a.logger.Warn("failed to restore Windows system proxy", "error", err)
	}
}

func exclusiveTarget(selected adapter.Adapter) exclusivenet.Target {
	return exclusivenet.Target{
		ID:             selected.ID,
		Name:           selected.Name,
		InterfaceIndex: selected.InterfaceIndex,
	}
}

func (a *App) resetProxyConnections(reason string) {
	a.mu.RLock()
	server := a.proxyServer
	authenticatedServer := a.authenticatedProxyServer
	a.mu.RUnlock()
	closed := 0
	if server != nil {
		closed += server.ResetConnections()
	}
	if authenticatedServer != nil {
		closed += authenticatedServer.ResetConnections()
	}
	if closed > 0 {
		a.logger.Info("closed proxy connections", "reason", reason, "connections", closed)
	}
}

func (a *App) context() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.ctx == nil {
		return context.Background()
	}
	return a.ctx
}

func (a *App) setError(message string) {
	a.mu.Lock()
	a.lastError = message
	a.mu.Unlock()
	a.logger.Error(message)
}

func (a *App) setControlError(message string) {
	a.mu.Lock()
	a.controlRunning = false
	a.controlError = message
	a.mu.Unlock()
	a.logger.Error(message)
}

func (a *App) setAuthenticatedProxyError(message string) {
	a.mu.Lock()
	a.authenticatedProxyRunning = false
	a.authenticatedProxyError = message
	a.mu.Unlock()
	a.logger.Error(message)
}
