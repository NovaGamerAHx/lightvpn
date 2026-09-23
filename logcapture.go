package main

// logCapture.go — Xray writes its console log to os.Stdout. A GUI EXE has no
// console, so those messages would simply vanish. We temporarily redirect
// stdout/stderr into a pipe and keep the last N lines in memory; the UI shows
// them in the "core log" drawer, which makes "why did this node fail"
// answerable without any external log file.

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"time"
)

// LogEntry is one captured core log line.
type LogEntry struct {
	TS    int64  `json:"ts"`
	Time  string `json:"time"`
	Level string `json:"level"`
	Text  string `json:"text"`
}

type logCapture struct {
	mu      sync.Mutex
	running bool
	lines   []LogEntry
	max     int

	origOut *os.File
	origErr *os.File
	write   *os.File
	done    chan struct{}
	onLine  func(level, msg string)
}

func newLogCapture(max int, onLine func(level, msg string)) *logCapture {
	if max <= 0 {
		max = 200
	}
	return &logCapture{max: max, onLine: onLine}
}

// Start redirects os.Stdout/os.Stderr. Safe to call repeatedly.
func (c *logCapture) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil
	}
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	c.origOut, c.origErr = os.Stdout, os.Stderr
	c.write = w
	c.done = make(chan struct{})
	os.Stdout, os.Stderr = w, w
	c.running = true

	go func() {
		defer close(c.done)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			level, text := splitLogLevel(line)
			c.record(LogEntry{TS: time.Now().UnixMilli(), Time: time.Now().Format("15:04:05"), Level: level, Text: text})
			if c.onLine != nil {
				c.onLine(level, text)
			}
		}
	}()
	return nil
}

// Stop restores the original streams and drains the reader.
func (c *logCapture) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	out, err2, w, done := c.origOut, c.origErr, c.write, c.done
	c.origOut, c.origErr, c.write = nil, nil, nil
	c.mu.Unlock()

	_ = w.Close()
	if done != nil {
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
	}
	if out != nil {
		os.Stdout = out
	}
	if err2 != nil {
		os.Stderr = err2
	}
}

func (c *logCapture) record(e LogEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, e)
	if len(c.lines) > c.max {
		c.lines = c.lines[len(c.lines)-c.max:]
	}
}

// Snapshot returns up to n most recent entries, oldest first.
func (c *logCapture) Snapshot(n int) []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n <= 0 || n > len(c.lines) {
		n = len(c.lines)
	}
	out := make([]LogEntry, n)
	copy(out, c.lines[len(c.lines)-n:])
	return out
}

// splitLogLevel understands Xray's console format:
//
//	[2026-01-02 03:04:05] Info: Xray 26.1.1 started
//	[2026-01-02 03:04:05] WARNING: node test-server: connection rejected: invalid user
func splitLogLevel(line string) (level, text string) {
	level = "info"
	text = line
	if i := strings.Index(line, "] "); i > 0 && strings.HasPrefix(line, "[") {
		text = strings.TrimSpace(line[i+2:])
	}
	low := strings.ToLower(text)
	switch {
	case strings.HasPrefix(low, "error:"), strings.Contains(low, "error: "):
		level, text = "error", strings.TrimSpace(text[strings.Index(strings.ToLower(text), "error:")+6:])
	case strings.HasPrefix(low, "warning:"), strings.HasPrefix(low, "warn:"):
		level = "warn"
		text = strings.TrimSpace(text[strings.Index(low, ":")+1:])
	case strings.HasPrefix(low, "info:"):
		level = "info"
		text = strings.TrimSpace(text[5:])
	case strings.HasPrefix(low, "debug:"):
		level = "debug"
		text = strings.TrimSpace(text[6:])
	}
	return level, strings.TrimSpace(text)
}
