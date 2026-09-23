package main

// storage.go — everything the app remembers lives in ONE json file next to the
// EXE (nodes, settings, and the system-proxy backup so a crash can be repaired
// on the next start). No registry keys, no %APPDATA% litter — that is what makes
// the build portable.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Settings are user editable options, persisted with the node list.
type Settings struct {
	SocksPort        int    `json:"socksPort"`
	HTTPPort         int    `json:"httpPort"`
	PingTimeoutMS    int    `json:"pingTimeoutMs"`
	PingSamples      int    `json:"pingSamples"`
	SystemProxy      bool   `json:"systemProxy"`
	ProxyBypass      string `json:"proxyBypass"`
	BypassLocal      bool   `json:"bypassLocal"`
	AllowInsecure    bool   `json:"allowInsecureAll"`
	Sniffing         bool   `json:"sniffing"`
	LogLevel         string `json:"logLevel"`
	DNSServers       string `json:"dnsServers"`
	MinimizeToTray   bool   `json:"minimizeToTray"`
	ConnectOnStartup bool   `json:"connectOnStartup"`
	AutoPingOnStart  bool   `json:"autoPingOnStart"`
}

// DefaultSettings matches the ports the spec requires.
func DefaultSettings() Settings {
	return Settings{
		SocksPort:       10808,
		HTTPPort:        10809,
		PingTimeoutMS:   3000,
		PingSamples:     3,
		SystemProxy:     true,
		ProxyBypass:     "127.0.0.1;localhost;192.168.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.2*.*;172.3*.*;<local>",
		BypassLocal:     true,
		Sniffing:        true,
		LogLevel:        "warning",
		DNSServers:      "223.5.5.5,1.1.1.1,8.8.8.8",
		MinimizeToTray:  true,
		AutoPingOnStart: true,
	}
}

func (s Settings) coreOptions() CoreOptions {
	o := CoreOptions{
		SocksPort:   s.SocksPort,
		HTTPPort:    s.HTTPPort,
		BypassLocal: s.BypassLocal,
		// AllowInsecure (a Settings toggle) is resolved into a certificate pin by
		// App.Connect before the config is built, so it is not passed through here.
		Sniffing: s.Sniffing,
		LogLevel: orDefault(s.LogLevel, "warning"),
	}
	for _, d := range strings.Split(s.DNSServers, ",") {
		if d = strings.TrimSpace(d); d != "" {
			o.DNSServers = append(o.DNSServers, d)
		}
	}
	o.normalise()
	return o
}

func (s Settings) pingOptions() PingOptions {
	return PingOptions{
		Timeout:     time.Duration(maxInt(s.PingTimeoutMS, 200)) * time.Millisecond,
		Samples:     maxInt(s.PingSamples, 1),
		MaxParallel: 32,
	}
}

// Data is the on-disk document.
type Data struct {
	Version         int          `json:"version"`
	Configs         []*Link      `json:"configs"`
	SelectedID      string       `json:"selectedId"`
	LastConnectedID string       `json:"lastConnectedId"`
	Settings        Settings     `json:"settings"`
	ProxyBackup     *ProxyBackup `json:"proxyBackup,omitempty"`
	UpdatedAt       int64        `json:"updatedAt"`
}

const dataVersion = 1

// Store owns config.json.
type Store struct {
	path  string
	mu    sync.Mutex
	data  Data
	timer *time.Timer
}

// DefaultStorePath returns <dir of the exe>/config.json, falling back to the
// user's config dir when the EXE sits somewhere read-only (Program Files).
func DefaultStorePath() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, "config.json")
		if dirWritable(dir) {
			return candidate
		}
	}
	if runtime.GOOS == "windows" {
		if base, err := os.UserConfigDir(); err == nil {
			return filepath.Join(base, "LightVPN", "config.json")
		}
	}
	if base, err := os.UserHomeDir(); err == nil {
		return filepath.Join(base, ".lightvpn", "config.json")
	}
	return "config.json"
}

func dirWritable(dir string) bool {
	probe := filepath.Join(dir, ".lightvpn-write-probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return true
}

// OpenStore loads config.json, creating it with defaults when missing.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}
	s.data = Data{Version: dataVersion, Settings: DefaultSettings()}

	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("cannot create the config folder: %w", err)
		}
		if err := s.Save(); err != nil {
			return nil, err
		}
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}

	var loaded Data
	if err := json.Unmarshal(raw, &loaded); err != nil {
		// Never lose a user's nodes over a formatting glitch.
		bak := path + ".broken-" + time.Now().Format("20060102-150405")
		_ = os.WriteFile(bak, raw, 0o600)
		if err := s.Save(); err != nil {
			return nil, err
		}
		return s, fmt.Errorf("config.json was not valid JSON (the unreadable file was kept as %s); starting with defaults", filepath.Base(bak))
	}
	if loaded.Version == 0 {
		loaded.Version = dataVersion
	}
	for _, c := range loaded.Configs {
		if c.ID == "" {
			c.ID = linkID(c)
		}
	}
	s.data = loaded

	// Settings are merged against the raw JSON object, not the decoded struct:
	// a key that an older build never wrote must fall back to today's default,
	// while an explicit false stays false.
	var disk diskData
	if err := json.Unmarshal(raw, &disk); err == nil {
		s.data.Settings = mergeSettings(disk.Settings, DefaultSettings())
	}
	return s, nil
}

// diskData mirrors Data but keeps "settings" unparsed, so we can tell an absent
// key apart from a zero value.
type diskData struct {
	Version         int             `json:"version"`
	Configs         []*Link         `json:"configs"`
	SelectedID      string          `json:"selectedId"`
	LastConnectedID string          `json:"lastConnectedId"`
	Settings        json.RawMessage `json:"settings"`
	ProxyBackup     *ProxyBackup    `json:"proxyBackup,omitempty"`
	UpdatedAt       int64           `json:"updatedAt"`
}

// mergeSettings overlays the settings that are actually present in config.json on
// top of the current defaults, then clamps nonsense. Adding a new option to
// Settings therefore never silently turns an existing install off.
func mergeSettings(raw json.RawMessage, def Settings) Settings {
	out := def
	if len(raw) == 0 {
		return out
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(raw, &present); err != nil || len(present) == 0 {
		return out
	}
	base, err := json.Marshal(def)
	if err != nil {
		return out
	}
	merged := map[string]json.RawMessage{}
	_ = json.Unmarshal(base, &merged)
	for k, v := range present {
		if _, known := merged[k]; known {
			merged[k] = v
		}
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return out
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return def
	}
	if out.SocksPort < 1 || out.SocksPort > 65535 {
		out.SocksPort = def.SocksPort
	}
	if out.HTTPPort < 1 || out.HTTPPort > 65535 || out.HTTPPort == out.SocksPort {
		out.HTTPPort = def.HTTPPort
	}
	if out.PingTimeoutMS < 200 {
		out.PingTimeoutMS = def.PingTimeoutMS
	}
	if out.PingSamples < 1 {
		out.PingSamples = def.PingSamples
	}
	if strings.TrimSpace(out.ProxyBypass) == "" {
		out.ProxyBypass = def.ProxyBypass
	}
	if strings.TrimSpace(out.LogLevel) == "" {
		out.LogLevel = def.LogLevel
	}
	if strings.TrimSpace(out.DNSServers) == "" {
		out.DNSServers = def.DNSServers
	}
	return out
}

// Path reports where the store persists.
func (s *Store) Path() string { return s.path }

// Snapshot returns a copy of the current data.
func (s *Store) Snapshot() Data {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.data
	out.Configs = make([]*Link, len(s.data.Configs))
	for i, c := range s.data.Configs {
		out.Configs[i] = c.Clone()
	}
	if s.data.ProxyBackup != nil {
		bak := *s.data.ProxyBackup
		out.ProxyBackup = &bak
	}
	return out
}

// Update mutates the data under the lock and saves it.
func (s *Store) Update(fn func(*Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.data); err != nil {
		return err
	}
	s.data.Version = dataVersion
	s.data.UpdatedAt = time.Now().UnixMilli()
	return s.saveLocked()
}

// UpdateNoSave is for hot paths (ping results) where the caller debounces.
func (s *Store) UpdateNoSave(fn func(*Data) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.data); err != nil {
		return err
	}
	s.data.UpdatedAt = time.Now().UnixMilli()
	s.scheduleSave()
	return nil
}

// Save writes config.json atomically.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) scheduleSave() {
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(700*time.Millisecond, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		_ = s.saveLocked()
	})
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return errors.New("no config path")
	}
	raw, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cannot write %s: %w", tmp, err)
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("cannot write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Antivirus tools sometimes hold the destination open for a scan; retry.
	for attempt := 0; attempt < 5; attempt++ {
		if err := os.Rename(tmp, s.path); err == nil {
			return nil
		} else if attempt == 4 {
			_ = os.Remove(tmp)
			return fmt.Errorf("cannot replace %s: %w", s.path, err)
		}
		time.Sleep(120 * time.Millisecond)
	}
	return nil
}

// ---- node helpers ------------------------------------------------------------

// AddLinks appends nodes, skipping ids that already exist. Returns how many
// were actually added.
// AddLinks appends the links whose id is not present yet and returns the ones it
// actually stored (so the caller can report "3 added, 2 already there").
// A re-imported link that carries a fresh certificate pin updates the pin of the
// node it would otherwise duplicate.
func (s *Store) AddLinks(links []*Link) []*Link {
	var added []*Link
	_ = s.Update(func(d *Data) error {
		byID := map[string]*Link{}
		for _, c := range d.Configs {
			byID[c.ID] = c
		}
		for _, l := range links {
			if l == nil {
				continue
			}
			if cur, dup := byID[l.ID]; dup {
				if l.PinnedCertSHA256 != "" && cur.PinnedCertSHA256 != l.PinnedCertSHA256 {
					cur.PinnedCertSHA256 = l.PinnedCertSHA256
					cur.CertNote = l.CertNote
				}
				continue
			}
			clone := l.Clone()
			byID[clone.ID] = clone
			d.Configs = append(d.Configs, clone)
			added = append(added, clone)
			if d.SelectedID == "" {
				d.SelectedID = clone.ID
			}
		}
		return nil
	})
	return added
}

func (s *Store) RemoveLink(id string) bool {
	removed := false
	_ = s.Update(func(d *Data) error {
		out := d.Configs[:0]
		for _, c := range d.Configs {
			if c.ID == id {
				removed = true
				continue
			}
			out = append(out, c)
		}
		d.Configs = out
		if d.SelectedID == id {
			d.SelectedID = ""
			if len(d.Configs) > 0 {
				d.SelectedID = d.Configs[0].ID
			}
		}
		if d.LastConnectedID == id {
			d.LastConnectedID = ""
		}
		return nil
	})
	return removed
}

// ApplyPing stores a measurement on the matching node.
func (s *Store) ApplyPing(r PingResult) {
	_ = s.UpdateNoSave(func(d *Data) error {
		for _, c := range d.Configs {
			if c.ID != r.ID {
				continue
			}
			c.PingMS, c.PingError, c.PingAt = r.MS, r.Error, r.At
			if !r.OK {
				c.PingMS = 0
			}
		}
		return nil
	})
}

// Rename sets a node's display name.
func (s *Store) Rename(id, name string) error {
	return s.Update(func(d *Data) error {
		for _, c := range d.Configs {
			if c.ID == id {
				c.Name = name
				return nil
			}
		}
		return errors.New("node not found")
	})
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
