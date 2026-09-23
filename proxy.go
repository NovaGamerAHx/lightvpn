package main

// proxy.go — Windows system proxy handling (shared types).
//
// The real implementation is in proxy_windows.go (registry + wininet refresh).
// On other operating systems the same API exists as a stub so the rest of the
// app — and the tests — compile and run everywhere.

import "errors"

// ErrProxyUnsupported is returned on non-Windows builds.
var ErrProxyUnsupported = errors.New("setting the system proxy is only supported on Windows")

// ProxyBackup is the snapshot of HKCU Internet Settings taken before we touch it.
// It is persisted in config.json so a crash or a power loss cannot leave the
// machine pointing at a dead 127.0.0.1 port.
type ProxyBackup struct {
	ProxyEnable   string `json:"proxyEnable"` // "0"/"1" as stored in the registry
	ProxyServer   string `json:"proxyServer"`
	ProxyOverride string `json:"proxyOverride"`
	AutoDetect    string `json:"autoDetect"`
	SavedAt       int64  `json:"savedAt"`
}

// ProxyStatus is reported to the UI.
type ProxyStatus struct {
	Supported     bool   `json:"supported"`
	EnabledByApp  bool   `json:"enabledByApp"`
	ProxyEnable   bool   `json:"proxyEnable"`
	ProxyServer   string `json:"proxyServer"`
	ProxyOverride string `json:"proxyOverride"`
	Key           string `json:"key"`
	Error         string `json:"error,omitempty"`
}

const proxyRegistryKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
