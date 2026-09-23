package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestStoreCreatesFileOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("precondition: file must not exist")
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config.json was not created: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("created file is not valid JSON: %v", err)
	}
	if doc["version"].(float64) != dataVersion {
		t.Errorf("version = %v, want %d", doc["version"], dataVersion)
	}
	if s.Snapshot().Settings.SocksPort != 10808 || s.Snapshot().Settings.HTTPPort != 10809 {
		t.Errorf("defaults wrong: %+v", s.Snapshot().Settings)
	}
	if s.Snapshot().Configs == nil {
		t.Error("a fresh store must have an (empty) node list")
	}
}

func TestStoreRoundTripAndDedupe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	links, perrs := ParseShareLinks(strings.Join([]string{vlessWS, trojanTCP, vlessReality}, "\n"))
	if len(perrs) != 0 {
		t.Fatalf("unexpected parse errors: %v", perrs)
	}
	if got := s.AddLinks(links); len(got) != 3 {
		t.Fatalf("added %d, want 3", len(got))
	}
	if got := s.AddLinks(links); len(got) != 0 {
		t.Errorf("re-adding the same nodes must be a no-op, added %d", len(got))
	}
	// re-importing with a fresh certificate pin refreshes it in place
	withPin := links[0].Clone()
	withPin.PinnedCertSHA256 = strings.Repeat("cd", 32)
	withPin.CertNote = "pinned on first use"
	if got := s.AddLinks([]*Link{withPin}); len(got) != 0 {
		t.Error("a known node must not be duplicated just because its pin changed")
	}
	if cur := s.Snapshot().Configs[0]; cur.PinnedCertSHA256 != strings.Repeat("cd", 32) || cur.CertNote == "" {
		t.Errorf("the newer pin must win: %+v", cur)
	}
	if s.Snapshot().SelectedID == "" {
		t.Error("the first imported node must become the selected one")
	}

	// reopen from disk
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.Snapshot()
	if len(got.Configs) != 3 {
		t.Fatalf("after reload: %d nodes, want 3", len(got.Configs))
	}
	if got.Configs[0].Raw != vlessWS {
		t.Errorf("raw link lost: %q", got.Configs[0].Raw)
	}
	if got.Configs[0].ID != links[0].ID {
		t.Error("ids must survive the round trip")
	}

	// delete + select fallback
	if !s2.RemoveLink(got.Configs[1].ID) {
		t.Error("RemoveLink reported nothing removed")
	}
	if len(s2.Snapshot().Configs) != 2 {
		t.Errorf("2 nodes expected after delete, got %d", len(s2.Snapshot().Configs))
	}
	if s2.RemoveLink("nope") {
		t.Error("removing an unknown id must report false")
	}
	if err := s2.Rename(got.Configs[0].ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if s2.Snapshot().Configs[0].Name != "Renamed" {
		t.Error("rename did not stick")
	}
}

func TestStoreAppliesPingAndDebounces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, _ := OpenStore(path)
	links, _ := ParseShareLinks(vlessWS)
	_ = s.AddLinks(links)
	s.ApplyPing(PingResult{ID: links[0].ID, OK: true, MS: 12.5, At: 999})

	// debounce: not yet on disk, but in memory
	if s.Snapshot().Configs[0].PingMS != 12.5 {
		t.Fatal("ping result not applied in memory")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	c := s2.Snapshot().Configs[0]
	if c.PingMS != 12.5 || c.PingAt != 999 {
		t.Errorf("ping result not persisted: %v / %v", c.PingMS, c.PingAt)
	}

	// a failed ping clears the old number
	s2.ApplyPing(PingResult{ID: c.ID, OK: false, Error: "timeout"})
	if got := s2.Snapshot().Configs[0]; got.PingMS != 0 || got.PingError != "timeout" {
		t.Errorf("failed ping must reset ms: %+v", got)
	}
}

func TestStoreRepairsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if s == nil {
		t.Fatalf("a corrupt file must still yield a usable store: %v", err)
	}
	if err == nil {
		t.Error("expected a warning about the unreadable file")
	} else if !strings.Contains(err.Error(), "kept as") {
		t.Errorf("unexpected error text: %v", err)
	}
	var backups []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && strings.Contains(filepath.Base(p), "broken-") {
			backups = append(backups, filepath.Base(p))
		}
		return nil
	})
	if len(backups) != 1 {
		t.Errorf("the unreadable file should have been preserved, found %v", backups)
	}
	if got := s.Snapshot(); len(got.Configs) != 0 || got.Settings.SocksPort != 10808 {
		t.Error("defaults must be restored next to the backup")
	}
}

func TestStoreMergesNewSettingsFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Simulate a config.json written by an older build.
	old := map[string]any{
		"version": dataVersion,
		"configs": []any{},
		"settings": map[string]any{
			"socksPort": 12345, "httpPort": 12346, "minimizeToTray": false,
		},
	}
	b, _ := json.Marshal(old)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	st := s.Snapshot().Settings
	if st.SocksPort != 12345 || st.HTTPPort != 12346 {
		t.Errorf("saved ports lost: %+v", st)
	}
	if st.MinimizeToTray {
		t.Error("an explicit false must be honoured")
	}
	if st.PingTimeoutMS != DefaultSettings().PingTimeoutMS || st.DNSServers != DefaultSettings().DNSServers {
		t.Errorf("missing fields must fall back to defaults: %+v", st)
	}
	if !st.SystemProxy || !st.BypassLocal {
		t.Error("absent booleans must default to the documented values")
	}
}

func TestStoreConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, _ := OpenStore(path)
	links, _ := ParseShareLinks(strings.Join([]string{vlessWS, trojanTCP, vlessReality}, "\n"))
	_ = s.AddLinks(links)

	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.ApplyPing(PingResult{ID: links[i%len(links)].ID, OK: true, MS: float64(i)})
			if i%7 == 0 {
				_ = s.Save()
			}
		}(i)
	}
	wg.Wait()
	if err := s.Save(); err != nil {
		t.Fatalf("save after concurrent updates: %v", err)
	}
	// The file must still be valid JSON afterwards.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d Data
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("config.json corrupted under load: %v\n%s", err, raw)
	}
	if _, tmp := os.Stat(path + ".tmp"); tmp == nil {
		t.Error("the temp file must be renamed away, not left behind")
	}
}
