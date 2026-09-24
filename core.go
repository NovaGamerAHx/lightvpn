package main

// core.go — owns the in-process Xray instance.
//
// Xray is embedded as a Go library (github.com/xtls/xray-core/core): we build a
// JSON config, hand it to core.New, then Start()/Close() the instance. There is
// no xray.exe subprocess anywhere in this app.

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	core "github.com/xtls/xray-core/core"
)

// CoreStatus is what the UI renders in the status card.
type CoreStatus struct {
	Running   bool   `json:"running"`
	Starting  bool   `json:"starting"`
	NodeID    string `json:"nodeId"`
	NodeName  string `json:"nodeName"`
	Endpoint  string `json:"endpoint"`
	SocksPort int    `json:"socksPort"`
	HTTPPort  int    `json:"httpPort"`
	StartedAt int64  `json:"startedAtMs"`
	UptimeMS  int64  `json:"uptimeMs"`
	LastError string `json:"lastError"`
	ConfigDir string `json:"-"`
}

// Manager starts and stops the core. All methods are safe for concurrent use.
type Manager struct {
	mu     sync.Mutex
	inst   *core.Instance
	status CoreStatus
	opts   CoreOptions
	config []byte // last generated JSON, kept for the UI preview / debugging
	logf   func(level, msg string)
	cap    *logCapture
}

// AttachCapture lets the app siphon Xray's console output while it runs.
func (m *Manager) AttachCapture(c *logCapture) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cap = c
}

func NewManager(opts CoreOptions, logf func(level, msg string)) *Manager {
	opts.normalise()
	return &Manager{opts: opts, logf: logf}
}

// SetOptions updates the generated config for the next connection.
func (m *Manager) SetOptions(opts CoreOptions) {
	m.mu.Lock()
	defer m.mu.Unlock()
	opts.normalise()
	m.opts = opts
}

// Options returns a copy of the current core options.
func (m *Manager) Options() CoreOptions {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.opts
}

// LastConfig returns the JSON handed to Xray for the current (or last) connection.
func (m *Manager) LastConfig() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte(nil), m.config...)
}

// IsRunning reports whether an instance is currently active.
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inst != nil && m.inst.IsRunning()
}

// NodeID is the id of the node the tunnel is currently running for.
func (m *Manager) NodeID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inst == nil {
		return ""
	}
	return m.status.NodeID
}

// Start brings up Xray for the given node. It is a no-op error if a connection
// is already active — call Stop first (App.Connect does that for you).
func (m *Manager) Start(l *Link) error {
	if l == nil {
		return fmt.Errorf("no node selected")
	}

	m.mu.Lock()
	if m.inst != nil {
		m.mu.Unlock()
		return fmt.Errorf("already connected to %s — disconnect first", m.status.NodeName)
	}
	m.status.Starting = true
	m.status.NodeID = l.ID
	m.status.NodeName = l.Name
	m.status.Endpoint = l.Endpoint()
	m.status.LastError = ""
	opts := m.opts
	m.mu.Unlock()

	cfg, err := BuildXrayConfig(l, opts)
	if err != nil {
		m.fail(l, err)
		return fmt.Errorf("invalid config: %w", err)
	}

	m.log("info", "starting core for "+l.Summary())
	if m.cap != nil { // must happen before core.New: Xray captures os.Stdout at startup
		_ = m.cap.Start()
	}

	var inst *core.Instance
	// A just-closed instance may still hold the listener for a few ms.
	for attempt := 0; attempt < 3; attempt++ {
		inst, err = m.buildInstance(cfg)
		if err == nil || !isAddrInUse(err) {
			break
		}
		m.log("warn", fmt.Sprintf("port busy, retrying (%d/3)", attempt+1))
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		m.fail(l, err)
		return err
	}

	m.mu.Lock()
	m.inst = inst
	m.config = cfg
	m.status.Running = true
	m.status.Starting = false
	m.status.StartedAt = time.Now().UnixMilli()
	m.status.SocksPort = opts.SocksPort
	m.status.HTTPPort = opts.HTTPPort
	m.mu.Unlock()

	m.log("info", fmt.Sprintf("connected: %s (socks %d, http %d)", l.Name, opts.SocksPort, opts.HTTPPort))
	return nil
}

func (m *Manager) buildInstance(cfg []byte) (*core.Instance, error) {
	parsed, err := core.LoadConfig("json", bytes.NewReader(cfg))
	if err != nil {
		return nil, fmt.Errorf("config rejected by xray: %w", err)
	}
	inst, err := core.New(parsed)
	if err != nil {
		if inst != nil {
			_ = inst.Close()
		}
		return nil, friendlyCoreError(err, "core init failed")
	}
	if err := inst.Start(); err != nil {
		_ = inst.Close()
		return nil, friendlyCoreError(err, "core start failed")
	}
	return inst, nil
}

// Stop shuts Xray down and releases both listener ports.
func (m *Manager) Stop() error {
	m.mu.Lock()
	inst := m.inst
	m.inst = nil
	name := m.status.NodeName
	m.status.Running = false
	m.status.Starting = false
	m.status.StartedAt = 0
	m.status.NodeID = ""
	m.status.NodeName = ""
	m.status.Endpoint = ""
	m.mu.Unlock()

	if inst == nil {
		return nil
	}
	m.log("info", "stopping core ("+name+")")
	done := make(chan error, 1)
	go func() { done <- inst.Close() }()
	select {
	case err := <-done:
		if err != nil {
			m.log("warn", "core closed with an error: "+err.Error())
			return friendlyCoreError(err, "core shutdown error")
		}
	case <-time.After(5 * time.Second):
		// Never let a stuck core block window teardown.
		m.log("warn", "core did not close within 5s; continuing")
	}
	return nil
}

// Status snapshots the current state (with a fresh uptime value).
func (m *Manager) Status() CoreStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	running := m.inst != nil && m.inst.IsRunning()
	s.Running = running
	if !running {
		s.StartedAt = 0
		s.UptimeMS = 0
		s.NodeID = ""
		s.NodeName = ""
		s.Endpoint = ""
	}
	s.SocksPort = m.opts.SocksPort
	s.HTTPPort = m.opts.HTTPPort
	if s.Running && s.StartedAt > 0 {
		s.UptimeMS = time.Now().UnixMilli() - s.StartedAt
	}
	return s
}

// ConfigJSON returns the generated config for a node without starting anything
// (the UI "preview config" action).
func ConfigJSON(l *Link, opts CoreOptions) (string, error) {
	cfg, err := BuildXrayConfig(l, opts)
	if err != nil {
		return "", err
	}
	return string(cfg), nil
}

func (m *Manager) fail(l *Link, err error) {
	msg := friendlyCoreError(err, "connection failed").Error()
	m.mu.Lock()
	m.status.Running = false
	m.status.Starting = false
	m.status.NodeID = ""
	m.status.NodeName = ""
	m.status.Endpoint = ""
	m.status.StartedAt = 0
	m.status.UptimeMS = 0
	m.status.LastError = msg
	m.mu.Unlock()
	m.log("error", "could not connect to "+l.Name+": "+msg)
}

func (m *Manager) log(level, msg string) {
	if m.logf != nil {
		m.logf(level, msg)
	}
}

// friendlyCoreError translates Xray's own error output into actionable text.
func friendlyCoreError(err error, prefix string) error {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(err.Error())
	low := strings.ToLower(msg)
	hint := ""
	switch {
	case strings.Contains(low, "no such host"):
		hint = " — the server hostname could not be resolved (check the address, or your DNS)"
	case strings.Contains(low, "connection refused"):
		hint = " — the server refused the connection (wrong port, or the server is down)"
	case strings.Contains(low, "i/o timeout"), strings.Contains(low, "context deadline"):
		hint = " — the server did not answer, it is likely unreachable or the port is blocked"
	case strings.Contains(low, "address already in use"), strings.Contains(low, "only one usage of each socket address"):
		hint = " — port 10808/10809 is occupied by another program; change the ports in Settings"
	case strings.Contains(low, "certificate"), strings.Contains(low, "x509"):
		hint = " — TLS certificate verification failed; enable \"allow insecure\" for self-signed servers"
	case strings.Contains(low, "reality") && strings.Contains(low, "auth"):
		hint = " — REALITY authentication failed (check pbk/sid/spx and the server name)"
	case strings.Contains(low, "invalid") && strings.Contains(low, "uuid"):
		hint = " — the UUID in the link is not valid"
	}
	if isAddrInUse(err) {
		return fmt.Errorf("%s%s", prefix, hint)
	}
	return fmt.Errorf("%s: %s%s", prefix, clip(oneLine(msg), 300), hint)
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	l := strings.ToLower(err.Error())
	return strings.Contains(l, "address already in use") ||
		strings.Contains(l, "only one usage of each socket address")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

// xrayVersionString is provided by the core package itself.
func xrayVersionString() string { return core.Version() }
