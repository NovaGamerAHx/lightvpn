package main

// app.go — the API surface the Svelte frontend calls. It is deliberately free of
// any Wails import: the GUI layer injects a Notifier (event push) and a
// WindowController (title-bar buttons), which keeps all of this testable on any
// OS while the shipped Windows binary wires it to Wails.

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

const appVersion = "1.0.0"
const appName = "LightVPN"

// Notifier pushes events to the webview ("state", "nodes", "ping", "log").
type Notifier interface {
	Emit(event string, data any)
}

type noopNotifier struct{}

func (noopNotifier) Emit(string, any) {}

// WindowController is implemented by the GUI layer (see main_windows.go).
type WindowController interface {
	Minimise()
	ToggleMaximise()
	Hide()
	Show()
	Quit()
	SetTitle(string)
}

// TrayController lets the backend keep the tray menu in sync with the state.
type TrayController interface {
	SetState(connected bool, nodeName string)
}

// NodeView is one row in the UI list.
type NodeView struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Protocol      string   `json:"protocol"`
	Address       string   `json:"address"`
	Port          int      `json:"port"`
	Endpoint      string   `json:"endpoint"`
	Transport     string   `json:"transport"`
	Network       string   `json:"network"`
	Security      string   `json:"security"`
	SNI           string   `json:"sni,omitempty"`
	Host          string   `json:"host,omitempty"`
	Path          string   `json:"path,omitempty"`
	Flow          string   `json:"flow,omitempty"`
	FPR           string   `json:"fp,omitempty"`
	PBK           string   `json:"pbk,omitempty"`
	SID           string   `json:"sid,omitempty"`
	SPX           string   `json:"spx,omitempty"`
	HeaderType    string   `json:"headerType,omitempty"`
	ALPN          string   `json:"alpn,omitempty"`
	Encryption    string   `json:"encryption,omitempty"`
	AllowInsecure bool     `json:"allowInsecure"`
	CertPin       string   `json:"certPin,omitempty"`
	CertNote      string   `json:"certNote,omitempty"`
	Notes         []string `json:"notes,omitempty"`
	PingMS        float64  `json:"pingMs"`
	PingError     string   `json:"pingError,omitempty"`
	PingAt        int64    `json:"pingAt,omitempty"`
	AddedAt       int64    `json:"addedAt,omitempty"`
	Selected      bool     `json:"selected"`
	Connected     bool     `json:"connected"`
	Pinging       bool     `json:"pinging"`
}

// StateView is everything the UI needs to paint one screen.
type StateView struct {
	AppName     string      `json:"appName"`
	AppVersion  string      `json:"appVersion"`
	CoreVersion string      `json:"coreVersion"`
	Connected   bool        `json:"connected"`
	Connecting  bool        `json:"connecting"`
	ElapsedMS   int64       `json:"elapsedMs"`
	StartedAt   int64       `json:"startedAtMs"`
	Current     *NodeView   `json:"current"`
	Status      CoreStatus  `json:"status"`
	Proxy       ProxyStatus `json:"proxy"`
	Settings    Settings    `json:"settings"`
	PingRunning bool        `json:"pingRunning"`
	Platform    string      `json:"platform"`
	StorePath   string      `json:"storePath"`
	Error       string      `json:"error,omitempty"`
	Warnings    []string    `json:"warnings,omitempty"`
}

// ImportResult reports what a paste of share links produced.
type ImportResult struct {
	Added    []NodeView   `json:"added"`
	Errors   []ParseError `json:"errors"`
	Total    int          `json:"total"`
	Existing int          `json:"existing"` // already in the list, skipped
	Message  string       `json:"message"`
}

// App is the bound object (window.go.main.App.* in the frontend).
type App struct {
	store *Store
	mgr   *Manager
	proxy *proxyBackend
	notif Notifier
	win   WindowController
	tray  TrayController

	cap *logCapture

	mu             sync.Mutex
	connectingFlag bool
	warnings       []string
	pingCtx        context.Context
	cancelPing     context.CancelFunc
	pinging        map[string]bool
	lastErr        string
	quitting       bool
}

// NewApp wires the app together. path may be "" to use config.json next to the EXE.
func NewApp(path string) (*App, func(), error) {
	if path == "" {
		path = DefaultStorePath()
	}
	store, err := OpenStore(path)
	if err != nil && store == nil {
		return nil, nil, err
	}
	var warnings []string
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	data := store.Snapshot()

	app := &App{
		store:    store,
		proxy:    newProxyBackend(),
		notif:    noopNotifier{},
		pinging:  map[string]bool{},
		warnings: warnings,
	}
	app.cap = newLogCapture(400, app.onCoreLine)
	app.mgr = NewManager(data.Settings.coreOptions(), app.log)
	app.mgr.AttachCapture(app.cap)

	// Repair an interrupted session: if a previous run died while the system
	// proxy was pointed at us, put the old settings back before doing anything.
	if data.ProxyBackup != nil {
		if err := app.proxy.Restore(data.ProxyBackup); err != nil {
			app.warn("could not restore the system proxy left by a previous run: " + err.Error())
		} else {
			app.warn("a previous session ended unexpectedly — the system proxy was restored")
			_ = store.Update(func(d *Data) error { d.ProxyBackup = nil; return nil })
		}
	}

	cleanup := func() { app.Shutdown() }
	return app, cleanup, nil
}

// AttachGUI is called by the Wails layer once the window exists.
func (a *App) AttachGUI(n Notifier, w WindowController, t TrayController) {
	a.mu.Lock()
	if n != nil {
		a.notif = n
	}
	a.win, a.tray = w, t
	a.mu.Unlock()
}

// ---- state ------------------------------------------------------------------

// GetState returns the full UI state.
func (a *App) GetState() StateView { return a.State() }

func (a *App) State() StateView {
	data := a.store.Snapshot()
	st := a.mgr.Status()
	sv := StateView{
		AppName:     appName,
		AppVersion:  appVersion,
		CoreVersion: xrayVersionString(),
		Connected:   st.Running,
		Connecting:  a.isConnecting(),
		ElapsedMS:   st.UptimeMS,
		StartedAt:   st.StartedAt,
		Status:      st,
		Settings:    data.Settings,
		PingRunning: a.pingRunning(),
		Platform:    runtime.GOOS + "/" + runtime.GOARCH,
		StorePath:   a.store.Path(),
		Error:       st.LastError,
	}
	sv.Proxy = a.currentProxyStatus(data)
	// "Current" is the node the hero panel talks about: the running one, or the
	// selected one while idle, so the UI never shows an empty state at rest.
	want := st.NodeID
	if want == "" {
		want = data.SelectedID
	}
	for _, c := range data.Configs {
		if c.ID == want {
			v := a.nodeView(c, data)
			sv.Current = &v
			break
		}
	}
	a.mu.Lock()
	sv.Warnings = append(sv.Warnings, a.warnings...)
	a.mu.Unlock()
	return sv
}

// ListNodes returns all imported nodes.
func (a *App) ListNodes() []NodeView {
	data := a.store.Snapshot()
	out := make([]NodeView, 0, len(data.Configs))
	for _, c := range data.Configs {
		out = append(out, a.nodeView(c, data))
	}
	return out
}

// ListConfigs is an alias kept for symmetry with the spec's wording.
func (a *App) ListConfigs() []NodeView { return a.ListNodes() }

func (a *App) nodeView(l *Link, d Data) NodeView {
	v := NodeView{
		ID: l.ID, Name: l.Name, Protocol: l.Protocol,
		Address: l.Address, Port: l.Port, Endpoint: l.Endpoint(),
		Transport: l.TransportLabel(),
		Network:   normalizeNetwork(l.Network), Security: normalizeSecurity(l.Security),
		SNI: l.SNI, Host: l.Hostname, Path: l.Path, Flow: l.Flow,
		FPR: l.Fingerprint, PBK: l.PublicKey, SID: l.ShortID, SPX: l.SpiderX,
		HeaderType: l.HeaderType, ALPN: alpnJoin(l.ALPN), Encryption: l.Encryption,
		AllowInsecure: l.AllowInsecure || d.Settings.AllowInsecure,
		CertPin:       l.PinnedCertSHA256,
		CertNote:      l.CertNote,
		Notes:         append([]string(nil), l.Notes...),
		PingMS:        l.PingMS, PingError: l.PingError, PingAt: l.PingAt, AddedAt: l.AddedAt,
		Selected:  d.SelectedID == l.ID,
		Connected: a.mgr.NodeID() == l.ID,
		Pinging:   a.isPinging(l.ID),
	}
	return v
}

func alpnJoin(v []string) string { return strings.Join(v, ", ") }

// ---- import / edit -----------------------------------------------------------

// ImportLinks parses and stores one-or-many share links (one per line).
func (a *App) ImportLinks(blob string) (ImportResult, error) {
	links, perrs := ParseShareLinks(blob)
	res := ImportResult{Errors: perrs, Total: len(links) + len(perrs)}

	data := a.store.Snapshot()
	added := a.store.AddLinks(links)
	for _, l := range added {
		res.Added = append(res.Added, a.nodeView(l, data))
	}
	res.Existing = len(links) - len(added)
	if len(added) > 0 || res.Existing > 0 {
		a.log("info", fmt.Sprintf("imported %d node(s), %d already present, %d line(s) rejected",
			len(added), res.Existing, len(perrs)))
	}

	parts := []string{}
	switch {
	case len(added) > 0:
		parts = append(parts, fmt.Sprintf("Imported %d node%s", len(added), plural(len(added))))
	case res.Existing > 0:
		parts = append(parts, fmt.Sprintf("%d node%s already in the list", res.Existing, plural(res.Existing)))
	default:
		parts = append(parts, "No share link found in that text")
	}
	if res.Existing > 0 && len(added) > 0 {
		parts = append(parts, fmt.Sprintf("%d already there", res.Existing))
	}
	if len(perrs) > 0 {
		parts = append(parts, fmt.Sprintf("%d line%s rejected", len(perrs), plural(len(perrs))))
	}
	res.Message = strings.Join(parts, ", ")
	if len(perrs) > 0 && len(added) == 0 {
		res.Message = "Nothing was imported — see the errors below"
	}

	a.emit("nodes", a.ListNodes())
	a.emitState()
	return res, nil
}

// DeleteNode removes a node (and disconnects first if it is the active one).
func (a *App) DeleteNode(id string) error {
	data := a.store.Snapshot()
	var target *Link
	for _, c := range data.Configs {
		if c.ID == id {
			l := c
			target = l
		}
	}
	if target == nil {
		return fmt.Errorf("node not found")
	}
	if a.mgr.NodeID() == id {
		if err := a.Disconnect(); err != nil {
			return err
		}
	}
	if !a.store.RemoveLink(id) {
		return fmt.Errorf("node not found")
	}
	a.log("info", "deleted node "+target.Name)
	a.emit("nodes", a.ListNodes())
	a.emitState()
	return nil
}

// RenameNode changes a node's display name.
func (a *App) RenameNode(id, name string) error {
	name = clip(strings.TrimSpace(name), 80)
	if name == "" {
		return fmt.Errorf("the name cannot be empty")
	}
	if err := a.store.Rename(id, name); err != nil {
		return err
	}
	a.emit("nodes", a.ListNodes())
	a.emitState()
	return nil
}

// CopyLink returns the original share link for the clipboard.
func (a *App) CopyLink(id string) (string, error) {
	for _, c := range a.store.Snapshot().Configs {
		if c.ID == id {
			return c.Raw, nil
		}
	}
	return "", fmt.Errorf("node not found")
}

// SelectNode marks the row the connect button should act on.
func (a *App) SelectNode(id string) error {
	return a.store.Update(func(d *Data) error {
		for _, c := range d.Configs {
			if c.ID == id {
				d.SelectedID = id
				return nil
			}
		}
		return fmt.Errorf("node not found")
	})
}

// NodeConfigPreview returns the exact JSON handed to Xray for a node.
func (a *App) NodeConfigPreview(id string) (string, error) {
	l, err := a.linkByID(id)
	if err != nil {
		return "", err
	}
	cfg, err := BuildXrayConfig(l, a.store.Snapshot().Settings.coreOptions())
	if err != nil {
		return "", err
	}
	return string(cfg), nil
}

func (a *App) linkByID(id string) (*Link, error) {
	data := a.store.Snapshot()
	if id == "" {
		id = data.SelectedID
	}
	for _, c := range data.Configs {
		if c.ID == id {
			return c, nil
		}
	}
	if id == "" && len(data.Configs) > 0 {
		return data.Configs[0], nil
	}
	return nil, fmt.Errorf("no node selected — import a share link first")
}

// ---- ping -------------------------------------------------------------------

// PingNode measures one node. id == "" means the selected node.
func (a *App) PingNode(id string) (PingResult, error) {
	l, err := a.linkByID(id)
	if err != nil {
		return PingResult{}, err
	}
	data := a.store.Snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), data.Settings.pingOptions().Timeout+time.Second)
	defer cancel()

	a.setPinging(l.ID, true)
	defer a.setPinging(l.ID, false)

	best, jitter, perr := PingEndpoint(ctx, l.Endpoint(), data.Settings.pingOptions().Timeout, data.Settings.pingOptions().Samples)
	res := PingResult{ID: l.ID, Name: l.Name, Endpoint: l.Endpoint(), At: time.Now().UnixMilli()}
	if perr != nil {
		res.Error = friendlyDialError(perr)
	} else {
		res.OK, res.MS, res.JitterMS = true, roundMS(best), roundMS(jitter)
	}
	a.applyPing(res)
	return res, nil
}

// PingAll pings every node; results also stream through the "ping" event.
func (a *App) PingAll() ([]PingResult, error) {
	links := a.store.Snapshot().Configs
	if len(links) == 0 {
		return nil, fmt.Errorf("no nodes imported yet")
	}
	opts := a.store.Snapshot().Settings.pingOptions()

	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	if a.cancelPing != nil {
		a.cancelPing()
	}
	a.pingCtx, a.cancelPing = ctx, cancel
	a.mu.Unlock()

	defer func() {
		cancel()
		a.mu.Lock()
		a.pingCtx, a.cancelPing = nil, nil
		a.mu.Unlock()
		a.emit("ping:done", map[string]any{"count": len(links)})
	}()

	for _, l := range links {
		a.setPinging(l.ID, true)
	}
	results := PingAll(ctx, links, opts, func(r PingResult) {
		a.setPinging(r.ID, false)
		a.applyPing(r)
		a.emit("ping", r)
	})
	for _, l := range links {
		a.setPinging(l.ID, false)
	}
	a.emit("nodes", a.ListNodes())
	return results, nil
}

// CancelPing aborts a running "test all".
func (a *App) CancelPing() {
	a.mu.Lock()
	if a.cancelPing != nil {
		a.cancelPing()
	}
	a.mu.Unlock()
}

func (a *App) applyPing(r PingResult) {
	a.store.ApplyPing(r)
}

func (a *App) setPinging(id string, on bool) {
	a.mu.Lock()
	if on {
		a.pinging[id] = true
	} else {
		delete(a.pinging, id)
	}
	a.mu.Unlock()
}

func (a *App) isPinging(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pinging[id]
}

func (a *App) pingRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pingCtx != nil
}

// ---- connect ----------------------------------------------------------------

// Connect starts Xray for a node (id == "" uses the selected node) and applies
// the system proxy when enabled.
func (a *App) Connect(id string) (StateView, error) {
	l, err := a.linkByID(id)
	if err != nil {
		return a.State(), err
	}
	data := a.store.Snapshot()

	// Re-point an existing connection instead of failing.
	if a.mgr.IsRunning() {
		if err := a.down(false); err != nil {
			return a.State(), err
		}
	}

	a.setConnecting(true)
	defer a.setConnecting(false)

	_ = a.store.Update(func(d *Data) error { d.SelectedID = l.ID; return nil })

	// Xray dropped "allowInsecure": nodes that carry it need their certificate
	// pinned instead. Do it now (once) so the generated config is accepted.
	l, err = a.ensureCertPin(l, data.Settings)
	if err != nil {
		a.setError(err.Error())
		a.emitState()
		return a.State(), err
	}

	a.log("info", fmt.Sprintf("connecting to %s …", l.Name))
	if err := a.mgr.Start(l); err != nil {
		a.setError(err.Error())
		a.emitState()
		return a.State(), err
	}

	if data.Settings.SystemProxy {
		if bak, err := a.proxy.Enable(data.Settings.HTTPPort, data.Settings.ProxyBypass); err != nil {
			a.warn("core is running but the system proxy was not set: " + err.Error())
			a.log("warn", "system proxy not applied: "+err.Error())
		} else {
			_ = a.store.Update(func(d *Data) error { d.ProxyBackup = bak; return nil })
		}
	}
	_ = a.store.Update(func(d *Data) error { d.LastConnectedID = l.ID; return nil })

	a.clearError()
	a.log("info", "connected")
	a.trayState(true, l.Name)
	a.emitState()
	a.emit("nodes", a.ListNodes())
	return a.State(), nil
}

// ensureCertPin resolves (and persists) the certificate pin a node needs in
// place of the removed allowInsecure option. It returns the link to build the
// config from — a copy when the pin had to be discovered.
func (a *App) ensureCertPin(l *Link, st Settings) (*Link, error) {
	if l.Security != "tls" {
		return l, nil
	}
	if strings.TrimSpace(l.PinnedCertSHA256) != "" {
		return l, nil // already pinned (from the link itself or a previous connect)
	}
	if !l.AllowInsecure && !st.AllowInsecure {
		return l, nil // trusted certificate: normal verification, nothing to pin
	}

	a.log("info", "checking the server certificate …")
	res, err := ResolveCertPin(context.Background(), l)
	if err != nil {
		// The probe failed, most often because the server is unreachable — which
		// the tunnel would notice anyway. Let the user try: the core will report
		// the TLS error itself, and that error is more specific than ours.
		a.warn("could not pin the certificate: " + err.Error() + " — starting anyway")
		return l, nil
	}
	if res.Verified {
		a.log("info", "certificate validated normally, no pinning needed")
		return l, nil
	}
	if res.Fingerprint == "" {
		return l, nil
	}

	pin, note := res.Fingerprint, res.Note()
	if err := a.store.Update(func(d *Data) error {
		for _, c := range d.Configs {
			if c.ID == l.ID {
				c.PinnedCertSHA256 = pin
				c.CertNote = note
				return nil
			}
		}
		return fmt.Errorf("node disappeared while pinning its certificate")
	}); err != nil {
		return nil, err
	}
	out := l.Clone()
	out.PinnedCertSHA256 = pin
	out.CertNote = note
	a.log("info", "pinned certificate sha256:"+pin[:16]+"… ("+note+")")
	return out, nil
}

// ClearCertPin forgets a pin so the next connect re-measures the certificate
// (use this after the server rotated its certificate).
func (a *App) ClearCertPin(id string) error {
	if id == "" {
		id = a.store.Snapshot().SelectedID
	}
	return a.store.Update(func(d *Data) error {
		for _, c := range d.Configs {
			if c.ID == id {
				c.PinnedCertSHA256 = ""
				c.CertNote = ""
				return nil
			}
		}
		return fmt.Errorf("node not found")
	})
}

// Disconnect stops Xray and restores the system proxy.
func (a *App) Disconnect() error {
	if !a.mgr.IsRunning() && !a.proxyActive() {
		return nil
	}
	return a.down(true)
}

// Toggle is what the big button calls.
func (a *App) Toggle(id string) (StateView, error) {
	if a.mgr.IsRunning() {
		return a.State(), a.Disconnect()
	}
	return a.Connect(id)
}

func (a *App) down(log bool) error {
	data := a.store.Snapshot()
	var proxyErr string

	if data.ProxyBackup != nil || data.Settings.SystemProxy {
		if err := a.proxy.Restore(data.ProxyBackup); err != nil {
			proxyErr = err.Error()
			a.warn("system proxy could not be restored: " + err.Error())
		} else if err := a.store.Update(func(d *Data) error { d.ProxyBackup = nil; return nil }); err != nil {
			a.log("warn", "could not clear the proxy backup: "+err.Error())
		}
	}

	coreErr := a.mgr.Stop()
	if log {
		a.log("info", "disconnected")
	}
	a.trayState(false, "")
	a.emitState()
	a.emit("nodes", a.ListNodes())
	if coreErr != nil && proxyErr != "" {
		return fmt.Errorf("%w (and the system proxy could not be restored: %s)", coreErr, proxyErr)
	}
	if coreErr != nil {
		return coreErr
	}
	if proxyErr != "" {
		return fmt.Errorf("core stopped, but the system proxy was not restored: %s", proxyErr)
	}
	return nil
}

func (a *App) proxyActive() bool {
	st, err := a.proxy.Current()
	if err != nil {
		return false
	}
	return st.ProxyEnable
}

// SetSystemProxy toggles the "apply Windows proxy while connected" behaviour and
// can enable/disable it on a live connection.
func (a *App) SetSystemProxy(on bool) (StateView, error) {
	data := a.store.Snapshot()
	if err := a.store.Update(func(d *Data) error { d.Settings.SystemProxy = on; return nil }); err != nil {
		return a.State(), err
	}
	switch {
	case on && a.mgr.IsRunning():
		bak, err := a.proxy.Enable(data.Settings.HTTPPort, data.Settings.ProxyBypass)
		if err != nil {
			return a.State(), err
		}
		_ = a.store.Update(func(d *Data) error { d.ProxyBackup = bak; return nil })
	case !on:
		if err := a.proxy.Restore(data.ProxyBackup); err != nil && data.ProxyBackup != nil {
			return a.State(), err
		}
		_ = a.store.Update(func(d *Data) error { d.ProxyBackup = nil; return nil })
	}
	a.log("info", fmt.Sprintf("system proxy %s", map[bool]string{true: "enabled", false: "disabled"}[on]))
	a.emitState()
	return a.State(), nil
}

// ProxyStatus re-reads the registry (the UI can verify nothing was left behind).
func (a *App) ProxyStatus() ProxyStatus { return a.currentProxyStatus(a.store.Snapshot()) }

func (a *App) currentProxyStatus(d Data) ProxyStatus {
	st, err := a.proxy.Current()
	if err != nil {
		st.Error = err.Error()
	}
	st.EnabledByApp = d.ProxyBackup != nil
	return st
}

// ---- settings ---------------------------------------------------------------

// UpdateSettings stores new options; ports take effect on the next connect.
func (a *App) UpdateSettings(s Settings) (StateView, error) {
	if s.SocksPort < 1 || s.SocksPort > 65535 {
		return a.State(), fmt.Errorf("SOCKS port must be between 1 and 65535")
	}
	if s.HTTPPort < 1 || s.HTTPPort > 65535 {
		return a.State(), fmt.Errorf("HTTP port must be between 1 and 65535")
	}
	if s.SocksPort == s.HTTPPort {
		return a.State(), fmt.Errorf("the SOCKS and HTTP ports must differ")
	}
	if s.PingTimeoutMS < 200 {
		s.PingTimeoutMS = 200
	}
	if s.PingSamples < 1 {
		s.PingSamples = 1
	}
	if s.LogLevel == "" {
		s.LogLevel = "warning"
	}

	wasRunning := a.mgr.IsRunning()
	if err := a.store.Update(func(d *Data) error { d.Settings = s; return nil }); err != nil {
		return a.State(), err
	}
	a.mgr.SetOptions(s.coreOptions())
	a.log("info", "settings saved")
	if wasRunning {
		a.log("info", "restart the connection to apply the new ports")
	}
	a.emitState()
	return a.State(), nil
}

// GetSettings returns the persisted settings.
func (a *App) GetSettings() Settings { return a.store.Snapshot().Settings }

// ConfigPath tells the user where their data lives (and creates the parent dir
// on demand so the reveal action never fails).
func (a *App) ConfigPath() string { return a.store.Path() }

// GetLogs returns the last n core log lines.
func (a *App) GetLogs(n int) []LogEntry {
	if a.cap == nil {
		return nil
	}
	return a.cap.Snapshot(n)
}

// ---- lifecycle --------------------------------------------------------------

// Startup runs the post-window-open work: auto-connect and auto-ping.
func (a *App) Startup() {
	d := a.store.Snapshot()
	if d.Settings.ConnectOnStartup && d.LastConnectedID != "" {
		id := d.LastConnectedID
		go func() {
			time.Sleep(400 * time.Millisecond)
			if _, err := a.Connect(id); err != nil {
				a.log("warn", "auto-connect failed: "+err.Error())
			}
		}()
	}
	if d.Settings.AutoPingOnStart && len(d.Configs) > 0 {
		go func() {
			time.Sleep(250 * time.Millisecond)
			if _, err := a.PingAll(); err != nil {
				a.log("debug", "startup ping skipped: "+err.Error())
			}
		}()
	}
	a.emitState()
}

// Shutdown stops everything: core, ping, system proxy, and the file handle.
func (a *App) Shutdown() {
	a.mu.Lock()
	if a.quitting {
		a.mu.Unlock()
		return
	}
	a.quitting = true
	a.mu.Unlock()

	if a.cancelPing != nil {
		a.cancelPing()
	}
	if d := a.store.Snapshot(); d.ProxyBackup != nil || a.mgr.IsRunning() {
		if err := a.down(false); err != nil {
			a.log("error", "shutdown: "+err.Error())
		}
	}
	_ = a.mgr.Stop()
	_ = a.store.Save()
}

// Quit closes the app (tray menu / window X when minimize-to-tray is off).
func (a *App) Quit() {
	a.Shutdown()
	a.mu.Lock()
	w := a.win
	a.mu.Unlock()
	if w != nil {
		w.Quit()
		return
	}
	os.Exit(0) // headless fallback (tests / --run-on-cli)
}

// ---- window controls (bound, used by the custom title bar) -------------------

func (a *App) WindowMinimise() {
	if w := a.window(); w != nil {
		w.Minimise()
	}
}

func (a *App) WindowToggleMaximise() {
	if w := a.window(); w != nil {
		w.ToggleMaximise()
	}
}

func (a *App) WindowHide() {
	if w := a.window(); w != nil {
		w.Hide()
	}
}

func (a *App) WindowShow() {
	if w := a.window(); w != nil {
		w.Show()
	}
}

// WindowClose implements the X button: hide to tray when configured, quit otherwise.
func (a *App) WindowClose() {
	if a.store.Snapshot().Settings.MinimizeToTray {
		if w := a.window(); w != nil {
			w.Hide()
			a.log("info", "minimised to the tray — Xray keeps running")
			return
		}
	}
	a.Quit()
}

func (a *App) window() WindowController {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.win
}

// ---- plumbing ----------------------------------------------------------------

func (a *App) setConnecting(on bool) {
	a.mu.Lock()
	a.connectingFlag = on
	a.mu.Unlock()
	a.emitState()
}

func (a *App) isConnecting() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connectingFlag
}

func (a *App) setError(msg string) {
	a.mu.Lock()
	a.lastErr = msg
	a.mu.Unlock()
}

func (a *App) clearError() { a.setError("") }

// isQuitting lets OnBeforeClose distinguish "user closed the window" (hide to
// tray) from "the app is going away" (really close, after cleanup).
func (a *App) isQuitting() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.quitting
}

func (a *App) warn(msg string) {
	a.mu.Lock()
	a.warnings = append(a.warnings, msg)
	a.mu.Unlock()
	a.log("warn", msg)
}

// log records an app-level line (buffer + UI). Lines captured from Xray's own
// console go through onCoreLine, which only emits — otherwise they would be
// appended twice.
func (a *App) log(level, msg string) {
	e := LogEntry{
		TS:    time.Now().UnixMilli(),
		Time:  time.Now().Format("15:04:05"),
		Level: level,
		Text:  msg,
	}
	if a.cap != nil {
		a.cap.record(e)
	}
	a.emit("log", e)
}

func (a *App) onCoreLine(level, msg string) {
	a.emit("log", LogEntry{
		TS: time.Now().UnixMilli(), Time: time.Now().Format("15:04:05"),
		Level: level, Text: msg,
	})
}

func (a *App) emit(event string, data any) {
	a.mu.Lock()
	n := a.notif
	a.mu.Unlock()
	if n != nil {
		n.Emit(event, data)
	}
}

func (a *App) emitState() { a.emit("state", a.State()) }

func (a *App) trayState(connected bool, name string) {
	a.mu.Lock()
	t := a.tray
	a.mu.Unlock()
	if t != nil {
		t.SetState(connected, name)
	}
}

// beginLogCapture is called by the manager right before the core starts so that
// Xray's own console output lands in our ring buffer.
func (a *App) beginLogCapture() {
	if a.cap != nil {
		_ = a.cap.Start()
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
