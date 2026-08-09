package desktopwails

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func Run(assets embed.FS) error {
	container, err := NewContainer()
	if err != nil {
		return fmt.Errorf("construct desktop services: %w", err)
	}
	if err := container.Runtime.startIPC(context.Background()); err != nil {
		return fmt.Errorf("start local IPC: %w", err)
	}
	defer container.Runtime.stopIPC()
	defer container.Runtime.StopLocalGateway()

	var window *application.WebviewWindow
	app := application.New(application.Options{
		Name:        "AI Runtime Gateway",
		Description: "Local AI gateway and expert escalation runtime",
		Services: []application.Service{
			application.NewService(container.Runtime),
			application.NewService(container.Consultations),
			application.NewService(container.Settings),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: desktopAutostartIdentifier,
			OnSecondInstanceLaunch: func(_ application.SecondInstanceData) {
				if window != nil {
					window.Show()
					window.Focus()
				}
			},
		},
		Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	container.Settings.setAutostartService(newWailsAutostartService(app.Autostart))
	if err := container.Settings.reconcileAutostart(context.Background()); err != nil {
		app.Logger.Error("reconcile login autostart", "error", err)
	}

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "main",
		Title:  "AI Runtime Gateway",
		Width:  1180,
		Height: 760,
		URL:    "/",
		Hidden: hasArgument(os.Args[1:], "--background"),
	})
	configureTray(app, window, container.Settings)
	return app.Run()
}

func hasArgument(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if argument == wanted {
			return true
		}
	}
	return false
}
