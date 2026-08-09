package desktopwails

import (
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type desktopLifecycle struct {
	quitting atomic.Bool
}

func configureTray(app *application.App, window *application.WebviewWindow, settings *SettingsService) {
	lifecycle := &desktopLifecycle{}
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if lifecycle.quitting.Load() {
			return
		}
		current, err := settings.Get()
		if err != nil {
			app.Logger.Error("read close behaviour setting", "error", err)
			window.Hide()
			event.Cancel()
			return
		}
		if current.MinimizeToTray {
			window.Hide()
			event.Cancel()
			return
		}
		lifecycle.quitting.Store(true)
		event.Cancel()
		app.Quit()
	})

	tray := app.SystemTray.New()
	tray.SetLabel("AI Runtime")

	menu := app.Menu.New()
	menu.Add("Open").OnClick(func(*application.Context) {
		window.Show()
		window.Focus()
	})
	menu.Add("Quit").OnClick(func(*application.Context) {
		lifecycle.quitting.Store(true)
		app.Quit()
	})
	tray.SetMenu(menu)
	tray.OnClick(func() {
		window.Show()
		window.Focus()
	})
}
