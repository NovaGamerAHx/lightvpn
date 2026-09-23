package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	app, cleanup, err := NewApp(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	t.Cleanup(cleanup)
	return app
}

func TestAppImportStateDeleteFlow(t *testing.T) {
	app := newTestApp(t)

	st := app.State()
	if st.AppName != appName || st.AppVersion != appVersion {
		t.Errorf("app identity missing: %+v", st)
	}
	if st.CoreVersion == "" || st.CoreVersion == "unknown" {
		t.Errorf("the UI shows the embedded xray version, got %q", st.CoreVersion)
	}
	if len(app.ListNodes()) != 0 {
		t.Fatal("a fresh app must have no nodes")
	}

	blob := strings.Join([]string{vlessWS, trojanTCP, vlessReality, "not-a-link"}, "\n")
	res, err := app.ImportLinks(blob)
	if err != nil {
		t.Fatalf("ImportLinks: %v", err)
	}
	if len(res.Added) != 3 {
		t.Errorf("expected 3 added, got %d (%s)", len(res.Added), res.Message)
	}
	if len(res.Errors) != 1 {
		t.Errorf("expected 1 rejected line, got %d", len(res.Errors))
	}
	nodes := app.ListNodes()
	if len(nodes) != 3 {
		t.Fatalf("ListNodes = %d, want 3", len(nodes))
	}
	if !nodes[0].Selected {
		t.Error("the first imported node should be selected")
	}
	if nodes[0].Transport != "WS · TLS" || nodes[0].Protocol != "vless" {
		t.Errorf("node view wrong: %+v", nodes[0])
	}

	// GetState mirrors the list and carries the settings the UI binds to.
	st = app.State()
	if st.Settings.SocksPort != 10808 || st.Settings.HTTPPort != 10809 {
		t.Errorf("state settings: %+v", st.Settings)
	}
	if st.Current == nil || st.Current.ID != nodes[0].ID {
		t.Errorf("state.current must point at the selected node, got %+v", st.Current)
	}

	// Everything must have hit disk already (the UI never waits on I/O).
	raw, err := os.ReadFile(app.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	var d Data
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("config.json invalid after import: %v", err)
	}
	if len(d.Configs) != 3 {
		t.Errorf("config.json holds %d nodes, want 3", len(d.Configs))
	}

	// re-importing the same blob must add nothing
	res2, _ := app.ImportLinks(blob)
	if len(res2.Added) != 0 {
		t.Errorf("re-import added %d duplicate nodes", len(res2.Added))
	}

	if err := app.RenameNode(nodes[1].ID, "  Renamed node  "); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := app.ListNodes()[1].Name; got != "Renamed node" {
		t.Errorf("name = %q (must be trimmed)", got)
	}
	if err := app.RenameNode(nodes[1].ID, "   "); err == nil {
		t.Error("an empty name must be rejected")
	}

	link, err := app.CopyLink(nodes[0].ID)
	if err != nil || link != vlessWS {
		t.Errorf("CopyLink = %q, %v — must return the original link verbatim", clip(link, 40), err)
	}

	if err := app.DeleteNode(nodes[2].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(app.ListNodes()) != 2 {
		t.Errorf("node still present after delete")
	}
	if err := app.DeleteNode("does-not-exist"); err == nil {
		t.Error("deleting an unknown node must error")
	}
	if _, err := app.NodeConfigPreview("does-not-exist"); err == nil {
		t.Error("preview of an unknown node must error")
	}
}

func TestAppConnectLifecycle(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.ImportLinks(strings.Join([]string{vlessWS, trojanTCP}, "\n")); err != nil {
		t.Fatal(err)
	}
	nodes := app.ListNodes()

	// Connect must not pretend to work without a node.
	if _, err := app.Connect("nope"); err == nil {
		t.Error("connecting to an unknown node must fail")
	}

	st, err := app.Connect(nodes[0].ID)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !st.Connected || st.Current == nil || st.Current.ID != nodes[0].ID {
		t.Fatalf("state after connect: %+v", st)
	}
	if !st.Status.Running || st.Status.SocksPort != 10808 || st.Status.HTTPPort != 10809 {
		t.Errorf("core status: %+v", st.Status)
	}
	if st.StartedAt == 0 {
		t.Error("the UI needs startedAtMs to render elapsed time")
	}
	if st.Current.Connected != true {
		t.Error("the connected row must be marked as active")
	}

	// The listeners are really up (this is what the system proxy points at).
	if err := waitReady("127.0.0.1:10808", 3*time.Second); err != nil {
		t.Errorf("SOCKS inbound: %v", err)
	}
	if err := waitReady("127.0.0.1:10809", 3*time.Second); err != nil {
		t.Errorf("HTTP inbound: %v", err)
	}

	// Connecting again must switch nodes, not error out.
	st2, err := app.Connect(nodes[1].ID)
	if err != nil {
		t.Fatalf("switch node: %v", err)
	}
	if st2.Current.Protocol != "trojan" {
		t.Errorf("expected the tunnel to move to the trojan node, got %+v", st2.Current)
	}

	// Deleting the live node disconnects first.
	if err := app.DeleteNode(nodes[1].ID); err != nil {
		t.Fatalf("delete live node: %v", err)
	}
	if app.State().Connected {
		t.Error("deleting the active node must tear the core down")
	}

	if err := app.Disconnect(); err != nil {
		t.Errorf("Disconnect when idle must be a no-op, got %v", err)
	}

	// Ports must be free again after the shutdown.
	if err := waitReady("127.0.0.1:10808", 100*time.Millisecond); err == nil {
		t.Error("the SOCKS port is still bound after disconnect")
	}
}

func TestAppConnectPersistsSelectionAndHistory(t *testing.T) {
	app := newTestApp(t)
	if _, err := app.ImportLinks(vlessWS); err != nil {
		t.Fatal(err)
	}
	id := app.ListNodes()[0].ID
	if _, err := app.Connect(id); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = app.Disconnect() }()

	d := app.store.Snapshot()
	if d.SelectedID != id {
		t.Errorf("SelectedID = %q", d.SelectedID)
	}
	if d.LastConnectedID != id {
		t.Errorf("LastConnectedID = %q (needed by \"reconnect on startup\")", d.LastConnectedID)
	}
}

func TestAppSettingsValidation(t *testing.T) {
	app := newTestApp(t)
	s := DefaultSettings()
	if _, err := app.UpdateSettings(s); err != nil {
		t.Fatalf("baseline settings must be accepted: %v", err)
	}

	bad := []struct {
		name   string
		mutate func(*Settings)
		want   string
	}{
		{"socks port too small", func(x *Settings) { x.SocksPort = 0 }, "SOCKS port"},
		{"http port too big", func(x *Settings) { x.HTTPPort = 70000 }, "HTTP port"},
		{"same port twice", func(x *Settings) { x.HTTPPort = x.SocksPort }, "must differ"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			s := DefaultSettings()
			c.mutate(&s)
			if _, err := app.UpdateSettings(s); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want an error containing %q", err, c.want)
			}
		})
	}

	// clamped instead of rejected, so a typo cannot brick the ping settings
	s.PingTimeoutMS = 1
	s.PingSamples = 0
	if _, err := app.UpdateSettings(s); err != nil {
		t.Fatal(err)
	}
	got := app.GetSettings()
	if got.PingTimeoutMS != 200 || got.PingSamples != 1 {
		t.Errorf("settings were not clamped: %+v", got)
	}
}

func TestAppLogsSurfaceErrors(t *testing.T) {
	app := newTestApp(t)
	seen := map[string]string{}
	app.AttachGUI(collector{t: t, seen: seen}, nil, nil)

	if _, err := app.ImportLinks(vlessWS); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("the frontend must receive events")
	}
	if _, ok := seen["nodes"]; !ok {
		t.Errorf("expected a 'nodes' event, saw %v", keys(seen))
	}
	if _, ok := seen["state"]; !ok {
		t.Errorf("expected a 'state' event, saw %v", keys(seen))
	}
	if len(app.GetLogs(50)) == 0 {
		t.Error("app-level log lines must be recorded")
	}
}

func TestAppHeadlessWindowCallsAreSafe(t *testing.T) {
	app := newTestApp(t)
	// No WindowController attached (CLI/tests): these must not panic.
	app.WindowMinimise()
	app.WindowToggleMaximise()
	app.WindowHide()
	app.WindowShow()
	if err := app.Disconnect(); err != nil {
		t.Errorf("disconnect while idle: %v", err)
	}
}

type collector struct {
	t    *testing.T
	seen map[string]string
}

func (c collector) Emit(event string, data any) { c.seen[event] = event }

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
