//go:build windows

package main

// main.go — Wails v2 entry point (Windows).
//
// The window is frameless: the title bar, its buttons and the tray live in the
// Svelte frontend + tray_windows.go. Xray runs inside this process; nothing is
// spawned and nothing is written outside config.json.

import (
	"context"
	"embed"
	"fmt"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var appIconICO []byte

//go:embed build/windows/tray-connected.ico
var trayConnectedICO []byte

//go:embed build/windows/tray-idle.ico
var trayIdleICO []byte

func main() {
	// A CLI escape hatch is handy for debugging on a machine without a display.
	if len(os.Args) > 1 && runCLI(os.Args[1:]) {
		return
	}

	app, cleanup, err := NewApp("")
	if err != nil {
		// config.json unreadable is not fatal: run with an in-memory fallback.
		fmt.Fprintln(os.Stderr, "warning:", err)
	}
	if app == nil {
		fmt.Fprintln(os.Stderr, "fatal: could not start:", err)
		os.Exit(1)
	}

	var (
		ctx      context.Context
		tr       *tray
		cancelfn context.CancelFunc
	)

	gui := &wailsWindow{get: func() context.Context { return ctx }}

	err = wails.Run(&options.App{
		Title:     appName + " — lightweight Xray client",
		Width:     1000,
		Height:    680,
		MinWidth:  760,
		MinHeight: 520,
		Frameless: true,

		// No taskbar flicker on relaunch: the tray keeps the process alive.
		HideWindowOnClose: false,
		BackgroundColour:  options.NewRGBA(16, 17, 21, 255),

		AssetServer: &assetserver.Options{Assets: assets},
		// Wails' own chatter goes into the same buffer the UI log panel reads,
		// because a -H windowsgui build has no console to write to.
		Logger: newWailsLogger(app),

		Windows: &windows.Options{
			Theme:                             windows.Dark,
			WebviewIsTransparent:              false,
			WindowIsTranslucent:               false,
			DisableWindowIcon:                 false,
			DisableFramelessWindowDecorations: false,
			WebviewUserDataPath:               "",
			ResizeDebounceMS:                  8,
		},

		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "lightvpn.xray.singleinstance." + appVersion,
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				// Launching the EXE again focuses the running window instead of
				// opening a second core.
				if ctx != nil {
					wruntime.WindowShow(ctx)
					wruntime.WindowUnminimise(ctx)
				}
			},
		},

		OnStartup: func(startCtx context.Context) {
			ctx, cancelfn = context.WithCancel(startCtx)
			app.AttachGUI(wailsNotifier{ctx: ctx}, gui, trayAdapter{get: func() *tray { return tr }})

			tr = startTray(app, trayIdleICO, trayConnectedICO, gui)

			// The webview needs a beat before it can receive events.
			go func() {
				time.Sleep(120 * time.Millisecond)
				app.Startup()
			}()
		},

		OnShutdown: func(context.Context) {
			if cancelfn != nil {
				cancelfn()
			}
			if tr != nil {
				tr.stop()
			}
			cleanup()
		},

		OnBeforeClose: func(context.Context) (prevent bool) {
			// Closing the window hides it to the tray unless the user turned that
			// off; the core is only stopped on a real quit (tray → Exit).
			if app.store.Snapshot().Settings.MinimizeToTray && !app.isQuitting() {
				return true
			}
			return false
		},

		Bind: []any{app},
		Menu: nil,
	})

	if err != nil {
		// If Wails could not even start, make sure we never leave the machine
		// pointing at a dead proxy.
		cleanup()
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// ---- wails adapters ----------------------------------------------------------

// wailsUILogger forwards the framework's messages into the app log so that a
// webview or binding problem is visible in the UI instead of being lost.
type wailsUILogger struct{ app *App }

func newWailsLogger(a *App) logger.Logger { return &wailsUILogger{app: a} }

func (l *wailsUILogger) log(level, msg string) {
	if l.app != nil {
		l.app.log(level, "wails: "+msg)
		return
	}
	fmt.Fprintln(os.Stderr, level+":", msg)
}

func (l *wailsUILogger) Print(m string)   { l.log("info", m) }
func (l *wailsUILogger) Trace(m string)   { l.log("debug", m) }
func (l *wailsUILogger) Debug(m string)   { l.log("debug", m) }
func (l *wailsUILogger) Info(m string)    { l.log("info", m) }
func (l *wailsUILogger) Warning(m string) { l.log("warning", m) }
func (l *wailsUILogger) Error(m string)   { l.log("error", m) }
func (l *wailsUILogger) Fatal(m string)   { l.log("error", m) }

// wailsNotifier forwards backend events into the webview.
type wailsNotifier struct{ ctx context.Context }

func (n wailsNotifier) Emit(event string, data any) {
	if n.ctx == nil {
		return
	}
	wruntime.EventsEmit(n.ctx, event, data)
}

// wailsWindow implements WindowController on top of the Wails runtime.
type wailsWindow struct {
	get func() context.Context
}

func (w *wailsWindow) ctx() context.Context {
	if w.get == nil {
		return nil
	}
	return w.get()
}

func (w *wailsWindow) Minimise() {
	if c := w.ctx(); c != nil {
		wruntime.WindowMinimise(c)
	}
}

func (w *wailsWindow) ToggleMaximise() {
	if c := w.ctx(); c != nil {
		wruntime.WindowToggleMaximise(c)
	}
}

func (w *wailsWindow) Hide() {
	if c := w.ctx(); c != nil {
		wruntime.WindowHide(c)
	}
}

func (w *wailsWindow) Show() {
	if c := w.ctx(); c != nil {
		wruntime.WindowShow(c)
		wruntime.WindowUnminimise(c)
	}
}

func (w *wailsWindow) Quit() {
	if c := w.ctx(); c != nil {
		wruntime.Quit(c)
		return
	}
	os.Exit(0)
}

func (w *wailsWindow) SetTitle(title string) {
	if c := w.ctx(); c != nil {
		wruntime.WindowSetTitle(c, title)
	}
}

// trayAdapter keeps the tray menu in sync without the backend importing systray.
type trayAdapter struct{ get func() *tray }

func (t trayAdapter) SetState(connected bool, nodeName string) {
	if tr := t.get(); tr != nil {
		tr.SetState(connected, nodeName)
	}
}
