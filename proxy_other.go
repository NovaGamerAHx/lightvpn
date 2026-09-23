//go:build !windows

package main

// proxy_other.go — no-op system proxy backend for non-Windows builds.
// It exists so `go test ./...` can exercise the whole app on Linux/macOS; the
// shipped binary is always Windows and uses proxy_windows.go.

type proxyBackend struct{}

func newProxyBackend() *proxyBackend { return &proxyBackend{} }

func (p *proxyBackend) Supported() bool { return false }

func (p *proxyBackend) Current() (ProxyStatus, error) {
	return ProxyStatus{Supported: false, Error: ErrProxyUnsupported.Error()}, nil
}

func (p *proxyBackend) Enable(httpPort int, bypass string) (*ProxyBackup, error) {
	return nil, ErrProxyUnsupported
}

func (p *proxyBackend) Restore(b *ProxyBackup) error {
	if b == nil {
		return nil
	}
	return ErrProxyUnsupported
}
