package main

import (
	"context"
	"crypto/tls"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// tlsTestServer answers a single handshake with a self-signed certificate and
// reports the fingerprint the client should end up pinning.
func tlsTestServer(t *testing.T) (addr, fingerprint string) {
	t.Helper()
	certPEM, keyPEM, err := selfSignedCert()
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		t.Fatalf("test cert is not usable: %v", err)
	}
	sum := fingerprintDER(pair.Certificate[0])
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{pair},
		NextProtos:   []string{"h2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					continue
				}
			}
			_ = c.(*tls.Conn).Handshake()
			_ = c.Close()
		}
	}()
	t.Cleanup(func() { close(done); _ = ln.Close() })
	return ln.Addr().String(), sum
}

func TestResolveCertPinProbesTheServer(t *testing.T) {
	addr, want := tlsTestServer(t)
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	l := &Link{
		Protocol: ProtoTrojan, Address: host, Port: port,
		Security: "tls", SNI: "localhost", ALPN: []string{"h2"},
		AllowInsecure: true, UserID: "pw",
	}
	res, err := ResolveCertPin(context.Background(), l)
	if err != nil {
		t.Fatalf("ResolveCertPin: %v", err)
	}
	if res.Verified {
		t.Error("a self-signed certificate must not be reported as verified")
	}
	if !strings.EqualFold(res.Fingerprint, want) {
		t.Errorf("pinned %s, want the leaf fingerprint %s", res.Fingerprint, want)
	}
	if res.Subject != "lightvpn-selftest" {
		t.Errorf("subject = %q", res.Subject)
	}
	if res.Note() == "" {
		t.Error("the details panel needs a note explaining the pin")
	}

	// Applying the pin is what lets the config be built at all.
	if _, err := PinNodeForConnection(context.Background(), l); err != nil {
		t.Fatalf("PinNodeForConnection: %v", err)
	}
	if !strings.EqualFold(l.PinnedCertSHA256, want) {
		t.Fatalf("the link must carry the pin after connecting once: %q", l.PinnedCertSHA256)
	}
	cfg, err := BuildXrayConfig(l, DefaultCoreOptions())
	if err != nil {
		t.Fatalf("build after pinning: %v", err)
	}
	if !strings.Contains(string(cfg), want) {
		t.Error("the fingerprint must be in the generated config")
	}
	loadThroughXray(t, cfg)

	// And with a trusted/unreachable-but-pinned link no probe is needed at all.
	l2 := *l
	l2.AllowInsecure = false
	l2.PinnedCertSHA256 = ""
	l2.PinnedCertSHA256 = want
	res2, err := ResolveCertPin(context.Background(), &l2)
	if err != nil || res2.Fingerprint != want {
		t.Errorf("an explicit fingerprint must be used as-is: %v (%q)", err, res2.Fingerprint)
	}

	l3 := *l
	l3.Address = "127.0.0.1"
	l3.Port = closedPort(t)
	l3.PinnedCertSHA256 = ""
	if _, err := ResolveCertPin(context.Background(), &l3); err == nil {
		t.Error("an unreachable server must produce a clear error, not a silent empty pin")
	}
}

func TestNormalizeCertFingerprint(t *testing.T) {
	fp := strings.Repeat("ab", 32)
	for _, in := range []string{fp, strings.ToUpper(fp), " " + fp + " ", strings.Join(strings.Split(fp, ""), ":")} {
		got, err := NormalizeCertFingerprint(in)
		if err != nil || got != fp {
			t.Errorf("NormalizeCertFingerprint(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "abc", strings.Repeat("zz", 32), strings.Repeat("ab", 31)} {
		if _, err := NormalizeCertFingerprint(bad); err == nil && strings.TrimSpace(bad) != "" {
			t.Errorf("%q must be rejected", bad)
		}
	}
	if got, err := NormalizeCertFingerprint(""); err != nil || got != "" {
		t.Errorf("the empty fingerprint means \"nothing pinned\": %q, %v", got, err)
	}
}

func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	time.Sleep(10 * time.Millisecond)
	return ln.Addr().(*net.TCPAddr).Port
}
