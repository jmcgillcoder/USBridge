package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/usbridge/usbridge/desktop/internal/adapter"
	"github.com/usbridge/usbridge/desktop/internal/androidclient"
	"github.com/usbridge/usbridge/desktop/internal/proxy/multiplex"
	"github.com/usbridge/usbridge/desktop/internal/routing"
	"github.com/usbridge/usbridge/desktop/internal/service"
	"github.com/usbridge/usbridge/desktop/internal/traffic"
	"github.com/usbridge/usbridge/desktop/internal/version"
)

type options struct {
	listen      string
	adapter     string
	ipMode      string
	androidPort int
	list        bool
	showVersion bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "USBridge:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	configuration, err := parseOptions(arguments)
	if err != nil {
		return err
	}
	if configuration.showVersion {
		fmt.Printf("USBridge %s (%s)\n", version.Version, version.Commit)
		return nil
	}
	mode, err := routing.ParseIPMode(configuration.ipMode)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	registry := adapter.NewRegistry(adapter.NewProvider())
	refreshContext, refreshCancel := context.WithTimeout(context.Background(), 12*time.Second)
	_, refreshErr := registry.Refresh(refreshContext)
	refreshCancel()
	if refreshErr != nil {
		logger.Warn("initial adapter discovery failed", "error", refreshErr)
	}
	if configuration.adapter != "" {
		if err := registry.Select(configuration.adapter); err != nil {
			return err
		}
	}
	if configuration.list {
		return printAdapters(registry)
	}

	policy := routing.NewPolicy(mode)
	boundDialer := routing.NewBoundDialer(registry, policy)
	meter := traffic.NewMeter()
	endpoint := func() (string, error) {
		selected, ok := registry.Selected()
		if !ok {
			return "", routing.ErrNoUSBAdapter
		}
		return androidclient.EndpointFromAdapter(selected, configuration.androidPort)
	}
	androidControl := androidclient.New(endpoint, os.Getenv("USBRIDGE_TOKEN"))
	controller := &service.Controller{
		Adapters:    registry,
		Routing:     policy,
		Android:     androidControl,
		AndroidPort: configuration.androidPort,
		ProxyListen: configuration.listen,
		Meter:       meter,
	}
	boundDialer.Gate = controller.RequireCellularUpstream

	proxyListener, err := net.Listen("tcp", configuration.listen)
	if err != nil {
		return fmt.Errorf("listen for unified proxy: %w", err)
	}
	defer proxyListener.Close()

	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignals()
	ctx, cancel := context.WithCancel(signalContext)
	defer cancel()
	go monitorAndroidStatus(ctx, controller, logger)

	go registry.Run(ctx, 10*time.Second, func(selected *adapter.Adapter) {
		if selected == nil {
			logger.Warn("no USB tethering adapter selected; proxy traffic is blocked")
			return
		}
		logger.Info(
			"USB tethering adapter selected",
			"name", selected.Name,
			"description", selected.Description,
			"ipv4", selected.IPv4,
			"ipv6", selected.IPv6,
		)
	})

	proxyServer := multiplex.New(boundDialer, meter, logger)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- proxyServer.Serve(ctx, proxyListener) }()

	snapshot := controller.Snapshot()
	logger.Info(
		"unified HTTP and SOCKS5 proxy started",
		"listen", proxyListener.Addr(),
		"ipMode", snapshot.IPMode,
		"failClosed", true,
	)
	if snapshot.SelectedAdapter == nil {
		logger.Warn("connect the phone and enable USB tethering; outbound proxy traffic remains blocked until detection succeeds")
	}

	select {
	case <-ctx.Done():
		logger.Info("proxy services stopped")
		return nil
	case serverErr := <-serverErrors:
		cancel()
		if serverErr == nil || errors.Is(serverErr, net.ErrClosed) {
			return nil
		}
		return serverErr
	}
}

func monitorAndroidStatus(ctx context.Context, controller *service.Controller, logger *slog.Logger) {
	refresh := func() {
		requestContext, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := controller.RefreshAndroidStatus(requestContext); err != nil {
			logger.Debug("Android control status unavailable", "error", err)
		}
	}
	refresh()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		}
	}
}

func parseOptions(arguments []string) (options, error) {
	var result options
	flags := flag.NewFlagSet("usbridge", flag.ContinueOnError)
	flags.StringVar(&result.listen, "listen", "127.0.0.1:18080", "shared HTTP and SOCKS5 proxy listen address")
	flags.StringVar(&result.adapter, "adapter", "", "USB adapter ID, interface index, or name; empty selects automatically")
	flags.StringVar(&result.ipMode, "ip-mode", "auto", "auto, ipv4, or ipv6")
	flags.IntVar(&result.androidPort, "android-port", 17890, "Android control API port")
	flags.BoolVar(&result.list, "list-adapters", false, "print discovered network adapters and exit")
	flags.BoolVar(&result.showVersion, "version", false, "print version and exit")
	if err := flags.Parse(arguments); err != nil {
		return options{}, err
	}
	return result, nil
}

func printAdapters(registry *adapter.Registry) error {
	selected, selectedOK := registry.Selected()
	result := struct {
		Selected *adapter.Adapter  `json:"selected,omitempty"`
		Adapters []adapter.Adapter `json:"adapters"`
	}{Adapters: registry.Snapshot()}
	if selectedOK {
		result.Selected = &selected
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
