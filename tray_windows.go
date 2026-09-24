//go:build windows

package main

// tray_windows.go — system tray icon + menu, kept out of the Wails layer because
// Wails v2 dropped its tray API (it lives in "onhold" upstream). This uses the
// maintained pure-Go systray library the Wails v3 / getlantern lineage uses:
// icon, tooltip, left-click toggle, right-click menu.
//
// The tray needs its own Win32 message loop, so it runs on a dedicated locked
// OS thread inside a goroutine; that keeps the main thread free for Wails.

import (
	"runtime"
	"sync"

	"github.com/energye/systray"
)

type tray struct {
	app    *App
	win    WindowController
	idle   []byte
	active []byte

	mu          sync.Mutex
	ready       bool
	pending     bool
	pendingName string

	itemToggle *systray.MenuItem
	itemShow   *systray.MenuItem
	itemPing   *systray.MenuItem
	itemExit   *systray.MenuItem

	stopOnce sync.Once
}

// startTray spawns the tray loop; it never blocks the caller.
func startTray(a *App, idle, active []byte, w WindowController) *tray {
	t := &tray{app: a, win: w, idle: idle, active: active}
	go t.run()
	return t
}

func (t *tray) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	systray.Run(t.onReady, t.onExit)
}

func (t *tray) onReady() {
	systray.SetIcon(t.idle)
	systray.SetTooltip(appName + " — idle")

	// Left click / double click show or hide the window, right click opens the menu.
	systray.SetOnClick(func(systray.IMenu) { t.toggleWindow() })
	systray.SetOnDClick(func(systray.IMenu) { t.showWindow() })
	systray.SetOnRClick(func(menu systray.IMenu) { _ = menu.ShowMenu() })

	t.itemShow = systray.AddMenuItem("Show / hide window", "Bring the app back to the foreground")
	t.itemShow.Click(func() { go t.toggleWindow() })

	systray.AddSeparator()

	t.itemToggle = systray.AddMenuItem("Connect", "Start the tunnel with the selected node")
	t.itemToggle.Click(func() { go t.toggleConnection() })

	t.itemPing = systray.AddMenuItem("Test ping (all nodes)", "Measure TCP latency to every server")
	t.itemPing.Click(func() { go func() { _, _ = t.app.PingAll() }() })

	systray.AddSeparator()

	t.itemExit = systray.AddMenuItem("Exit", "Disconnect, restore the system proxy and quit")
	t.itemExit.Click(func() {
		go func() {
			t.stop()
			t.app.Quit()
		}()
	})

	t.mu.Lock()
	t.ready = true
	pending, name := t.pending, t.pendingName
	t.mu.Unlock()
	t.apply(pending, name)
}

func (t *tray) onExit() {
	t.mu.Lock()
	t.ready = false
	t.mu.Unlock()
}

// SetState reflects connection state in the icon, tooltip and menu text.
func (t *tray) SetState(connected bool, nodeName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.ready {
		t.pending, t.pendingName = connected, nodeName
		return
	}
	t.apply(connected, nodeName)
}

func (t *tray) apply(connected bool, name string) {
	if connected {
		systray.SetIcon(t.active)
		tip := appName + " — connected"
		if name != "" {
			tip += " · " + name
		}
		systray.SetTooltip(tip)
		if t.itemToggle != nil {
			t.itemToggle.SetTitle("Disconnect")
			t.itemToggle.SetTooltip("Stop the tunnel and restore the system proxy")
		}
		return
	}
	systray.SetIcon(t.idle)
	systray.SetTooltip(appName + " — idle")
	if t.itemToggle != nil {
		t.itemToggle.SetTitle("Connect")
		t.itemToggle.SetTooltip("Start the tunnel with the selected node")
	}
}

func (t *tray) toggleWindow() {
	if t.win == nil {
		return
	}
	t.win.Show() // the window is either visible (no-op) or hidden/minimised
}

func (t *tray) showWindow() {
	if t.win != nil {
		t.win.Show()
	}
}

func (t *tray) toggleConnection() {
	_, _ = t.app.Toggle("")
}

func (t *tray) stop() {
	t.stopOnce.Do(func() { systray.Quit() })
}
