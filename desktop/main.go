package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/usbridge/usbridge/desktop/internal/exclusivenet"
	"github.com/usbridge/usbridge/desktop/internal/updater"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if handled, exitCode := updater.RunApplyIfRequested(os.Args[1:]); handled {
		os.Exit(exitCode)
	}
	if handled, exitCode := exclusivenet.RunHelperIfRequested(os.Args[1:]); handled {
		os.Exit(exitCode)
	}

	application := NewApp()
	err := wails.Run(&options.App{
		Title:            "USBridge",
		Width:            1120,
		Height:           760,
		MinWidth:         880,
		MinHeight:        620,
		DisableResize:    false,
		Frameless:        false,
		BackgroundColour: &options.RGBA{R: 255, G: 247, B: 255, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        application.startup,
		OnShutdown:       application.shutdown,
		Bind:             []interface{}{application},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "USBridge UI:", err)
	}
}
