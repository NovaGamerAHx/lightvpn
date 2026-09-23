//go:build windows

package main

// proxy_windows.go — sets the Windows system (WinINet) proxy by writing
// HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings and then
// telling WinINet/WinHTTP to reload it, exactly like V2RayN does.
//
// Notes:
//   * HKCU needs no elevation, so the app stays portable and admin-free.
//   * The previous values are snapshotted first and restored on disconnect, so
//     a pre-existing corporate proxy survives a connect/disconnect cycle.
//   * InternetSetOptionW(SETTINGS_CHANGED + REFRESH) is what makes already-open
//     browsers pick the setting up without a restart.

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	wininet           = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOp = wininet.NewProc("InternetSetOptionW")
)

const (
	internetOptionSettingsChanged = 39
	internetOptionRefresh         = 37
)

type proxyBackend struct{}

func newProxyBackend() *proxyBackend { return &proxyBackend{} }

func (p *proxyBackend) Supported() bool { return true }

func (p *proxyBackend) Current() (ProxyStatus, error) {
	st := ProxyStatus{Supported: true, Key: "HKCU\\" + proxyRegistryKey}
	k, err := registry.OpenKey(registry.CURRENT_USER, proxyRegistryKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return st, fmt.Errorf("cannot open %s: %w", st.Key, err)
	}
	defer k.Close()

	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		st.ProxyEnable = v != 0
	}
	st.ProxyServer, _, _ = k.GetStringValue("ProxyServer")
	st.ProxyOverride, _, _ = k.GetStringValue("ProxyOverride")
	return st, nil
}

// Enable points the system proxy at the local HTTP inbound and returns the
// snapshot needed to undo it later.
func (p *proxyBackend) Enable(httpPort int, bypass string) (*ProxyBackup, error) {
	if httpPort <= 0 || httpPort > 65535 {
		return nil, fmt.Errorf("invalid HTTP proxy port %d", httpPort)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, proxyRegistryKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil, fmt.Errorf("cannot open HKCU\\%s: %w", proxyRegistryKey, err)
	}
	defer k.Close()

	backup := &ProxyBackup{SavedAt: time.Now().UnixMilli()}
	if v, _, err := k.GetIntegerValue("ProxyEnable"); err == nil {
		backup.ProxyEnable = fmt.Sprintf("%d", v)
	} else {
		backup.ProxyEnable = "0"
	}
	backup.ProxyServer, _, _ = k.GetStringValue("ProxyServer")
	backup.ProxyOverride, _, _ = k.GetStringValue("ProxyOverride")
	if v, _, err := k.GetIntegerValue("AutoDetect"); err == nil {
		backup.AutoDetect = fmt.Sprintf("%d", v)
	}

	server := fmt.Sprintf("127.0.0.1:%d", httpPort)
	if err := k.SetStringValue("ProxyServer", server); err != nil {
		return nil, friendlyRegistryError(err, "ProxyServer")
	}
	if strings.TrimSpace(bypass) != "" {
		if err := k.SetStringValue("ProxyOverride", strings.TrimSpace(bypass)); err != nil {
			return nil, friendlyRegistryError(err, "ProxyOverride")
		}
	}
	// Turning off automatic configuration stops a PAC file overriding us.
	_ = k.SetDWordValue("AutoDetect", 0)
	if err := k.SetDWordValue("ProxyEnable", 1); err != nil {
		return nil, friendlyRegistryError(err, "ProxyEnable")
	}

	// No explicit flush: registry writes are durable once the key handle is
	// closed (registry.Key.Close does that), and the broadcast below is what
	// makes running browsers pick the change up without a restart.
	notifyProxyChange()
	return backup, nil
}

// Restore puts back whatever the system had before Enable (or simply turns the
// proxy off when there was nothing to restore).
func (p *proxyBackend) Restore(b *ProxyBackup) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, proxyRegistryKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("cannot open HKCU\\%s: %w", proxyRegistryKey, err)
	}
	defer k.Close()

	bak := ProxyBackup{ProxyEnable: "0"}
	if b != nil {
		bak = *b
	}
	if err := k.SetDWordValue("ProxyEnable", parseDWORD(bak.ProxyEnable)); err != nil {
		return friendlyRegistryError(err, "ProxyEnable")
	}
	if bak.ProxyServer != "" {
		if err := k.SetStringValue("ProxyServer", bak.ProxyServer); err != nil {
			return friendlyRegistryError(err, "ProxyServer")
		}
	}
	if err := k.SetStringValue("ProxyOverride", bak.ProxyOverride); err != nil {
		return friendlyRegistryError(err, "ProxyOverride")
	}
	if bak.AutoDetect != "" {
		_ = k.SetDWordValue("AutoDetect", parseDWORD(bak.AutoDetect))
	}

	notifyProxyChange()
	return nil
}

func notifyProxyChange() {
	if procInternetSetOp == nil || procInternetSetOp.Find() != nil {
		return
	}
	// Both calls ignore their buffer; run on a locked thread so the DLL sees a
	// stable stack while the syscall executes.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, opt := range []uintptr{internetOptionSettingsChanged, internetOptionRefresh} {
		syscall.SyscallN(procInternetSetOp.Addr(), 0, opt, 0, 0)
	}
}

func parseDWORD(s string) uint32 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var n uint32
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + uint32(r-'0')
		if n > 1 {
			break
		}
	}
	return n
}

func friendlyRegistryError(err error, value string) error {
	switch err {
	case syscall.ERROR_ACCESS_DENIED:
		return fmt.Errorf("access denied writing %s — the key is protected by policy", value)
	case syscall.ERROR_FILE_NOT_FOUND, registry.ErrNotExist:
		return fmt.Errorf("the Internet Settings key is missing (%w)", err)
	}
	return fmt.Errorf("cannot write %s: %w", value, err)
}

var _ = unsafe.Pointer(nil)
