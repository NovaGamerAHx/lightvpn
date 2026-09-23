package main

// selftest.go — an end-to-end proof that this client really tunnels traffic.
//
// It starts a second, throwaway Xray instance in the same process acting as a
// real server (VLESS/Trojan over TLS, TCP and WebSocket transports), points a
// generated config at it, and then fetches an object through the local HTTP and
// SOCKS5 inbounds. Because the generated config routes *everything* through the
// proxy outbound, a successful fetch can only happen if the tunnel is working.
//
// The last scenario uses wrong credentials and must fail, which proves the
// server is actually authenticating (and that failures reach the UI as errors).
//
// Run it from the shipped EXE with:  LightVPN.exe --selftest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	core "github.com/xtls/xray-core/core"
)

// SelfTestResult is one scenario outcome.
type SelfTestResult struct {
	Name   string  `json:"name"`
	OK     bool    `json:"ok"`
	Detail string  `json:"detail"`
	MS     float64 `json:"ms"`
}

// SelfTestReport is the aggregate result.
type SelfTestReport struct {
	OK      bool             `json:"ok"`
	Results []SelfTestResult `json:"results"`
	Summary string           `json:"summary"`
}

type scenario struct {
	name     string
	protocol string // vless | trojan
	network  string // tcp | ws
	badCreds bool
}

var selfTestScenarios = []scenario{
	{name: "VLESS + TCP + TLS", protocol: "vless", network: "tcp"},
	{name: "VLESS + WebSocket + TLS", protocol: "vless", network: "ws"},
	{name: "Trojan + TCP + TLS", protocol: "trojan", network: "tcp"},
	{name: "Trojan + WebSocket + TLS", protocol: "trojan", network: "ws"},
	{name: "VLESS + TCP + TLS — wrong id must be rejected", protocol: "vless", network: "tcp", badCreds: true},
}

// RunSelfTest executes every scenario sequentially.
func RunSelfTest(logf func(level, msg string)) SelfTestReport {
	if logf == nil {
		logf = func(string, string) {}
	}
	rep := SelfTestReport{OK: true}
	for _, sc := range selfTestScenarios {
		start := time.Now()
		res := SelfTestResult{Name: sc.name}
		detail, err := runScenario(sc, logf)
		res.MS = float64(time.Since(start).Microseconds()) / 1000.0
		if err != nil {
			res.OK = false
			res.Detail = err.Error()
			rep.OK = false
			logf("error", sc.name+": "+err.Error())
		} else {
			res.OK = true
			res.Detail = detail
			logf("info", fmt.Sprintf("%s: %s (%.0f ms)", sc.name, detail, res.MS))
		}
		rep.Results = append(rep.Results, res)
	}
	passed := 0
	for _, r := range rep.Results {
		if r.OK {
			passed++
		}
	}
	rep.Summary = fmt.Sprintf("%d/%d scenarios passed", passed, len(rep.Results))
	return rep
}

func runScenario(sc scenario, logf func(string, string)) (string, error) {
	certPEM, keyPEM, err := selfSignedCert()
	if err != nil {
		return "", fmt.Errorf("could not create the test certificate: %w", err)
	}

	serverPort, err := freeTCPPort()
	if err != nil {
		return "", err
	}
	socksPort, err := freeTCPPort()
	if err != nil {
		return "", err
	}
	httpPort, err := freeTCPPort()
	if err != nil {
		return "", err
	}

	secret := randomHex(8) // uuid-ish password / trojan password
	uuid := randomUUID()   // vless id
	nonce := randomHex(12)

	// ---- origin server the tunneled request must reach ----------------------
	origin := &originServer{nonce: nonce, body: "lightvpn-selftest-" + nonce}
	if err := origin.start(); err != nil {
		return "", err
	}
	defer origin.stop()

	// ---- server side Xray ---------------------------------------------------
	serverCfg := buildTestServerConfig(sc, serverPort, uuid, secret, certPEM, keyPEM)
	serverInst, err := startTestCore(serverCfg, logf, "server")
	if err != nil {
		return "", fmt.Errorf("test server did not start: %w", err)
	}
	defer func() { _ = serverInst.Close() }()

	// ---- client side: exactly what App.Connect does -------------------------
	link, err := ParseShareLink(testLinkFor(sc, serverPort, uuid, secret))
	if err != nil {
		return "", fmt.Errorf("the generated test link did not parse: %w", err)
	}
	// allowInsecure no longer exists in Xray: resolve the pin the same way
	// App.Connect does, so the self test covers that code path too.
	if pin, perr := ResolveCertPin(context.Background(), link); perr != nil {
		return "", perr
	} else if pin.Fingerprint != "" {
		link.PinnedCertSHA256 = pin.Fingerprint
	}

	opts := CoreOptions{
		SocksPort:   socksPort,
		HTTPPort:    httpPort,
		BypassLocal: false, // force every destination through the proxy outbound
		DNSServers:  []string{"127.0.0.1"},
		Sniffing:    true,
		LogLevel:    "error",
	}
	clientCfg, err := BuildXrayConfig(link, opts)
	if err != nil {
		return "", fmt.Errorf("config generation failed: %w", err)
	}
	clientInst, err := startTestCore(clientCfg, logf, "client")
	if err != nil {
		return "", fmt.Errorf("the generated client config did not run: %w", err)
	}
	defer func() { _ = clientInst.Close() }()

	// Give the listeners a moment (they bind on core.New, but be kind to CI).
	if err := waitReady("127.0.0.1:"+fmt.Sprint(httpPort), 3*time.Second); err != nil {
		return "", fmt.Errorf("client HTTP inbound never came up: %w", err)
	}

	// ---- 1) HTTP proxy path (this is what the Windows system proxy uses) ----
	// The first request right after a core starts can race with the transport
	// being wired up (Xray answers 503); a client retries, so we do too. The
	// negative control must not retry — we want to see the rejection itself.
	attempts, backoff := 8, 200*time.Millisecond
	if sc.badCreds {
		attempts, backoff = 1, 0
	}
	got, err := retryFetch(attempts, backoff, func() (string, error) {
		return fetchViaHTTPProxy(httpPort, origin.url())
	})
	if sc.badCreds {
		if err == nil {
			return "", fmt.Errorf("a request with wrong credentials succeeded (body %q) — the server is not authenticating", clip(got, 40))
		}
		return "rejected as expected: " + clip(err.Error(), 90), nil
	}
	if err != nil {
		return "", fmt.Errorf("request through the local HTTP proxy failed: %w", err)
	}
	if got != origin.body {
		return "", fmt.Errorf("tunnel returned %q, wanted %q", clip(got, 60), origin.body)
	}

	// ---- 2) SOCKS5 path ------------------------------------------------------
	if err := waitReady("127.0.0.1:"+fmt.Sprint(socksPort), 2*time.Second); err != nil {
		return "", fmt.Errorf("client SOCKS inbound never came up: %w", err)
	}
	gotSOCKS, err := retryFetch(8, 200*time.Millisecond, func() (string, error) {
		return fetchViaSOCKS5(socksPort, origin.url())
	})
	if err != nil {
		return "", fmt.Errorf("request through the local SOCKS5 proxy failed: %w", err)
	}
	if gotSOCKS != origin.body {
		return "", fmt.Errorf("socks tunnel returned %q, wanted %q", clip(gotSOCKS, 60), origin.body)
	}

	// ---- 3) ping path against the same listener ------------------------------
	rtt, _, perr := PingEndpoint(context.Background(), link.Endpoint(), 2*time.Second, 2)
	if perr != nil {
		return "", fmt.Errorf("TCP ping failed even though the server is listening: %w", perr)
	}

	return fmt.Sprintf("fetched %d bytes over HTTP+SOCKS5, ping %.2f ms", len(got), float64(rtt.Microseconds())/1000.0), nil
}

// ---- helpers -----------------------------------------------------------------

// retryFetch runs fn until it succeeds or the attempts are used up, returning the
// last error. Only "the tunnel was not ready yet" style failures are retried.
func retryFetch(attempts int, wait time.Duration, fn func() (string, error)) (string, error) {
	var body string
	var lastErr error
	for i := 0; i < attempts; i++ {
		body, lastErr = fn()
		if lastErr == nil {
			return body, nil
		}
		if !strings.Contains(lastErr.Error(), "503") && !strings.Contains(lastErr.Error(), "EOF") &&
			!strings.Contains(lastErr.Error(), "connection reset") && !strings.Contains(lastErr.Error(), "malformed") {
			return body, lastErr
		}
		if i+1 < attempts {
			time.Sleep(wait)
		}
	}
	return body, lastErr
}

func startTestCore(cfg []byte, logf func(string, string), role string) (*core.Instance, error) {
	parsed, err := core.LoadConfig("json", strings.NewReader(string(cfg)))
	if err != nil {
		return nil, fmt.Errorf("config rejected: %w", err)
	}
	inst, err := core.New(parsed)
	if err != nil {
		if inst != nil {
			_ = inst.Close()
		}
		return nil, fmt.Errorf("%s init failed: %w", role, err)
	}
	if err := inst.Start(); err != nil {
		_ = inst.Close()
		return nil, fmt.Errorf("%s start failed: %w", role, err)
	}
	logf("debug", fmt.Sprintf("selftest %s core started", role))
	return inst, nil
}

func buildTestServerConfig(sc scenario, port int, uuid, password, certPEM, keyPEM string) []byte {
	settings := map[string]any{}
	switch sc.protocol {
	case "trojan":
		settings["clients"] = []any{map[string]any{"password": password}}
	default:
		settings["clients"] = []any{map[string]any{"id": uuid}}
		settings["decryption"] = "none"
	}

	stream := map[string]any{
		"network":  sc.network,
		"security": "tls",
		"tlsSettings": map[string]any{
			"certificates": []any{map[string]any{
				"certificate": []string{certPEM},
				"key":         []string{keyPEM},
			}},
		},
	}
	if sc.network == "ws" {
		stream["wsSettings"] = map[string]any{"path": "/lightvpn-test"}
	}

	cfg := map[string]any{
		"log":       map[string]any{"loglevel": "error", "access": "none", "error": ""},
		"inbounds":  []any{map[string]any{"port": port, "listen": "127.0.0.1", "protocol": sc.protocol, "settings": settings, "streamSettings": stream}},
		"outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{}}},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return b
}

func testLinkFor(sc scenario, port int, uuid, password string) string {
	cred, name := uuid, "VLESS"
	if sc.protocol == "trojan" {
		cred, name = password, "Trojan"
	}
	if sc.badCreds {
		cred = randomUUID() // a valid but unknown id/password
	}
	q := "encryption=none&security=tls&sni=localhost&fp=chrome&allowInsecure=1"
	if sc.protocol != "trojan" {
		q = "encryption=none&" + q
	} else {
		q = "security=tls&sni=localhost&fp=chrome&allowInsecure=1"
	}
	if sc.network == "ws" {
		q += "&type=ws&host=localhost&path=%2Flightvpn-test"
	} else {
		q += "&type=tcp"
	}
	return fmt.Sprintf("%s://%s@127.0.0.1:%d?%s#Selftest%%20%s", sc.protocol, cred, port, q, name)
}

type originServer struct {
	nonce string
	body  string
	srv   *http.Server
	ln    net.Listener
}

func (o *originServer) start() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	o.ln = ln
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+o.nonce {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(o.body))
	})
	o.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = o.srv.Serve(ln) }()
	return nil
}

func (o *originServer) stop() {
	if o.srv != nil {
		_ = o.srv.Close()
	}
}

func (o *originServer) url() string {
	return "http://" + o.ln.Addr().String() + "/" + o.nonce
}

func fetchViaHTTPProxy(port int, target string) (string, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}),
		},
	}
	resp, err := client.Get(target)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return string(b), fmt.Errorf("origin returned HTTP %d", resp.StatusCode)
	}
	return string(b), nil
}

// fetchViaSOCKS5 performs a minimal SOCKS5 (no-auth) CONNECT by hand, so the
// test does not need a third-party socks client.
func fetchViaSOCKS5(port int, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		host, portStr = u.Host, "80"
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	// greeting: version 5, one method (no auth)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return "", err
	}
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return "", err
	}
	if hdr[0] != 0x05 || hdr[1] != 0x00 {
		return "", fmt.Errorf("socks5 handshake refused (version %d method %d)", hdr[0], hdr[1])
	}

	ip := net.ParseIP(host)
	var addr []byte
	if ip != nil && ip.To4() != nil {
		addr = append([]byte{0x01}, ip.To4()...)
	} else if ip6 := ip; ip6 != nil && ip6.To16() != nil {
		addr = append([]byte{0x04}, ip6.To16()...)
	} else { // domain
		addr = append([]byte{0x03, byte(len(host))}, host...)
	}
	p, err := parsePort(portStr)
	if err != nil {
		return "", err
	}
	req := append([]byte{0x05, 0x01, 0x00}, addr...)
	req = append(req, byte(p>>8), byte(p&0xff))
	if _, err := conn.Write(req); err != nil {
		return "", err
	}
	resp := make([]byte, 4)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return "", err
	}
	if resp[1] != 0x00 {
		return "", fmt.Errorf("socks5 connect refused: status %d", resp[1])
	}
	switch resp[3] {
	case 0x01:
		_, err = io.ReadFull(conn, make([]byte, 4+2))
	case 0x03: // domain bound address: 1 length byte + domain + 2 port bytes
		lb := make([]byte, 1)
		if _, err = io.ReadFull(conn, lb); err == nil {
			_, err = io.ReadFull(conn, make([]byte, int(lb[0])+2))
		}
	case 0x04:
		_, err = io.ReadFull(conn, make([]byte, 16+2))
	}
	if err != nil {
		return "", fmt.Errorf("socks5 reply truncated: %w", err)
	}

	// origin-form request target ("/path"), with the authority in the Host
	// header — that is what a plain HTTP server expects. Sending
	// "GET host:port/path" (neither origin- nor absolute-form) gets a 400.
	path := "/"
	if i := strings.Index(rawURL, "://"); i >= 0 {
		if j := strings.Index(rawURL[i+3:], "/"); j >= 0 {
			path = rawURL[i+3+j:]
		}
	}
	hostHdr := host
	if portStr != "" && portStr != "80" {
		hostHdr = net.JoinHostPort(host, portStr)
	}
	reqText := "GET " + path + " HTTP/1.1\r\nHost: " + hostHdr + "\r\nConnection: close\r\nX-Selftest: 1\r\n\r\n"
	if _, err := conn.Write([]byte(reqText)); err != nil {
		return "", err
	}
	body, err := io.ReadAll(conn)
	if err != nil {
		return "", err
	}
	text := string(body)
	split := strings.SplitN(text, "\r\n\r\n", 2)
	if len(split) != 2 {
		return "", fmt.Errorf("malformed HTTP response through socks: %q", clip(text, 80))
	}
	if !strings.Contains(split[0], " 200 ") {
		return split[1], fmt.Errorf("origin responded %s", clip(strings.SplitN(split[0], "\r\n", 2)[0], 60))
	}
	return split[1], nil
}

// ---- tiny utilities -----------------------------------------------------------

func freeTCPPort() (int, error) {
	for i := 0; i < 20; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		port := l.Addr().(*net.TCPAddr).Port
		if err := l.Close(); err != nil {
			return 0, err
		}
		// Make sure the port is not immediately reused by another listener.
		d, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			_ = d.Close()
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("could not find a free local port")
}

func waitReady(addr string, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 400*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return nil
		}
		time.Sleep(80 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("ab", n)
	}
	return fmt.Sprintf("%x", b)[:2*n]
}

func randomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// selfSignedCert makes a throwaway ECDSA cert for 127.0.0.1/localhost.
func selfSignedCert() (certPEM, keyPEM string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "lightvpn-selftest", Organization: []string{appName}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", err
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM, nil
}

func parsePort(s string) (int, error) {
	var p int
	if _, err := fmt.Sscanf(s, "%d", &p); err != nil {
		return 0, err
	}
	if p <= 0 || p > 65535 {
		return 0, fmt.Errorf("bad port")
	}
	return p, nil
}
