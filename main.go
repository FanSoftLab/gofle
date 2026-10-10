package main

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/logging"
	"context"
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// assets embeds the built frontend served by the Wails asset server.
//
//go:embed all:frontend/dist
var assets embed.FS

// appIcon holds the application icon embedded for the OS window/about dialog.
//
//go:embed build/appicon.png
var appIcon []byte

//go:generate wails generate module

func main() {
	os.Exit(run())
}

// windowTitle is the OS window / about-dialog title.
func windowTitle() string {
	return "Go Fast Log Explorer"
}

// run starts the Wails application and returns the process exit code. The
// deferred logging teardown runs when run returns, before os.Exit in main, so
// startup failures and runtime logs are still flushed.
func run() int {
	// Structured logging to stderr + gofle.log beside the config dir (the
	// packaged app's stderr is discarded by .desktop launches, so the file is
	// the only durable sink for startup failures and runtime Warn/Error logs).
	defer logging.Setup(config.DefaultDir())()

	app, err := NewApp()
	if err != nil {
		slog.Error("failed to initialize app", "err", err)
		return 1
	}

	winWidth, winHeight, winMaximized := app.GetWindowGeometry()

	startState := options.Normal
	if winMaximized {
		startState = options.Maximised
	}

	err = wails.Run(&options.App{
		Title:            windowTitle(),
		Width:            winWidth,
		Height:           winHeight,
		WindowStartState: startState,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			app.Startup(ctx)

			if winMaximized {
				wailsRuntime.WindowMaximise(ctx)
			}
		},
		OnBeforeClose: func(ctx context.Context) bool {
			// Ask the frontend to flush the tab session before the webview is
			// torn down. pagehide/beforeunload are unreliable under Wails.
			wailsRuntime.EventsEmit(ctx, "app:before-close")
			// Flushes debounced settings + window geometry to the shared config.
			app.SaveCurrentWindowGeometry(ctx)
			return false
		},
		OnShutdown: app.Shutdown,
		Bind: []any{
			app,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:  true,
			CSSDropProperty: "--wails-drop-target",
			CSSDropValue:    "drop",
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
		Linux: &linux.Options{
			Icon:                appIcon,
			ProgramName:         "GoFLE",
			WindowIsTranslucent: false,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			Appearance:           mac.NSAppearanceNameDarkAqua,
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   windowTitle(),
				Message: "Go Fast Log Explorer",
				Icon:    appIcon,
			},
		},
	})
	if err != nil {
		slog.Error("failed to start Wails application", "err", err)
		return 1
	}

	return 0
}
