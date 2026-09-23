package main

// platform.go — tiny shared helpers (build info + signal waiting), so the CLI
// and the GUI layers stay free of boilerplate.

import (
	"io"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
)

const (
	runtimeGOOS   = runtime.GOOS
	runtimeGOARCH = runtime.GOARCH
)

func readAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

// waitForInterrupt resolves on SIGINT/SIGTERM (Ctrl+C) or WM_CLOSE on Windows.
func waitForInterrupt() <-chan struct{} {
	done := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	var once sync.Once
	go func() {
		for range c {
			once.Do(func() {
				signal.Stop(c)
				close(done)
			})
		}
	}()
	return done
}
