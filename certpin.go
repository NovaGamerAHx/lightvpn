package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"
)

// Xray removed "allowInsecure" (it is a hard config error since 2026-06-01), and
// share links still carry allowInsecure=1 for servers with self-signed
// certificates. The supported replacement is pinning the peer certificate:
// "pinnedPeerCertSha256".
//
// So for those nodes we do a trust-on-first-use handshake here:
//
//  1. try a strict handshake first — if the certificate validates normally there
//     is nothing to pin and the config stays as boring as possible;
//  2. otherwise fetch the leaf certificate and return its SHA-256, which the
//     generated config pins.
//
// The result is cached on the node, so reconnects do not re-dial and a rotated
// certificate is detected (the pin simply stops matching and the user is told).

// certPinTimeout bounds the extra handshake done before starting the core.
const certPinTimeout = 5 * time.Second

// CertPin is what ResolveCertPin learned about a server certificate.
type CertPin struct {
	Fingerprint string // hex SHA-256 of the leaf DER ("" when nothing needs pinning)
	Pinned      bool   // the fingerprint came from a real probe of the server
	Verified    bool   // the certificate validated against the system roots
	Subject     string
	Issuer      string
	NotBefore   string // RFC3339, display only
	NotAfter    string // RFC3339, display only
	Expired     bool   // the certificate is past NotAfter
}

// Note is the one-line description shown in the config details panel.
func (c CertPin) Note() string {
	if c.Verified {
		return "certificate validated normally (system roots)"
	}
	if c.Fingerprint == "" {
		return ""
	}
	n := "self-signed certificate pinned on first use"
	if c.Subject != "" {
		n = "pinned certificate " + c.Subject
	}
	if c.NotAfter != "" {
		if c.Expired {
			n += ", certificate EXPIRED " + c.NotAfter[:10]
		} else {
			n += ", valid until " + c.NotAfter[:10]
		}
	}
	return n
}

// ResolveCertPin decides what the generated config needs in order to trust the
// TLS server of l.
func ResolveCertPin(ctx context.Context, l *Link) (CertPin, error) {
	var none CertPin
	if l == nil || l.Security != "tls" {
		return none, nil
	}
	if fp, err := NormalizeCertFingerprint(l.PinnedCertSHA256); err == nil && fp != "" {
		return CertPin{Fingerprint: fp}, nil // the link carried pcs= itself
	} else if strings.TrimSpace(l.PinnedCertSHA256) != "" {
		return none, err // present but malformed: say so instead of silently ignoring
	}
	if !l.AllowInsecure {
		return none, nil // a properly signed certificate: nothing to do
	}

	addr := net.JoinHostPort(strings.Trim(l.Address, "[]"), fmt.Sprint(l.Port))
	sni := l.SNI
	if sni == "" && !isIPAddress(l.Address) {
		sni = l.Address
	}

	alpn := append([]string(nil), l.ALPN...)
	if len(alpn) == 0 {
		alpn = []string{"h2", "http/1.1"}
	}

	// One handshake, then decide locally. Probing with a *failing* strict
	// handshake first would make the server log a TLS error for every connect,
	// which is noise the user should not have to read.
	var lastErr error
	for _, try := range [][]string{alpn, nil} {
		c, cerr := handshake(ctx, addr, sni, try)
		if cerr != nil {
			lastErr = cerr
			continue
		}
		state := c.ConnectionState()
		res := CertPin{}
		if len(state.PeerCertificates) > 0 {
			leaf := state.PeerCertificates[0]
			res.Fingerprint = fingerprintDER(leaf.Raw)
			res.Subject, res.Issuer = certNameOf(leaf), leaf.Issuer.CommonName
			res.NotAfter = leaf.NotAfter.Format(time.RFC3339)
			res.NotBefore = leaf.NotBefore.Format(time.RFC3339)
			res.Expired = time.Now().After(leaf.NotAfter)
			res.Verified = certVerifies(state.PeerCertificates, sni)
		}
		closeQuietly(c) // a clean alert keeps the server's logs quiet
		if len(state.PeerCertificates) == 0 {
			lastErr = fmt.Errorf("the server presented no certificate")
			continue
		}
		if res.Verified {
			res.Fingerprint = "" // a trusted certificate needs no pin
			return res, nil
		}
		res.Pinned = true
		return res, nil
	}
	return none, fmt.Errorf("this node needs its certificate pinned, but the TLS handshake to %s failed: %w", addr, lastErr)
}

// certVerifies answers the question Xray will ask: does this chain validate
// against the system trust store for this name? Intermediates from the handshake
// are used, nothing else is.
func certVerifies(certs []*x509.Certificate, serverName string) bool {
	if len(certs) == 0 {
		return false
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	if serverName == "" {
		serverName = certNameOf(certs[0])
	}
	_, err = certs[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inter,
		DNSName:       serverName,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err == nil
}

// PinNodeForConnection resolves the pin a node needs and records it on the link,
// so the very next BuildXrayConfig produces a config Xray accepts. The caller is
// responsible for persisting l if it wants the pin to survive the session.
func PinNodeForConnection(ctx context.Context, l *Link) (CertPin, error) {
	res, err := ResolveCertPin(ctx, l)
	if err != nil {
		return res, err
	}
	l.PinnedCertSHA256 = res.Fingerprint
	l.CertNote = res.Note()
	return res, nil
}

// handshake performs a TLS handshake that always accepts whatever the server
// presents: the caller decides what to do with the chain. A clean CloseNotify on
// the way out keeps server-side logs free of "EOF" handshake errors.
func handshake(ctx context.Context, addr, sni string, alpn []string) (*tls.Conn, error) {
	d := &net.Dialer{Timeout: certPinTimeout}
	rc, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	tc := tls.Client(rc, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true, // #nosec G407 -- probe only: the result is inspected below and pinned
		NextProtos:         alpn,
		MinVersion:         tls.VersionTLS12,
	})
	_ = tc.SetDeadline(time.Now().Add(certPinTimeout))
	if err := tc.HandshakeContext(ctx); err != nil {
		_ = tc.Close()
		return nil, err
	}
	_ = tc.SetDeadline(time.Time{})
	return tc, nil
}

func closeQuietly(c *tls.Conn) {
	if c == nil {
		return
	}
	_ = c.Close()
}

// fingerprintDER is the SHA-256 of a DER-encoded leaf certificate, which is
// exactly what Xray's pinnedPeerCertSha256 compares against.
func fingerprintDER(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// ValidCertFingerprint reports whether s is a usable pinnedPeerCertSha256 value
// (64 hex chars, colons tolerated the way OpenSSL prints them).
func ValidCertFingerprint(s string) bool {
	h, err := NormalizeCertFingerprint(s)
	return err == nil && len(h) == 64
}

// NormalizeCertFingerprint strips separators and lowercases a fingerprint.
func NormalizeCertFingerprint(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil // "nothing pinned" is not an error
	}
	s = strings.NewReplacer(":", "", "-", "", " ", "", "\n", "", "\r", "").Replace(s)
	if len(s) != 64 {
		return "", fmt.Errorf("a pinned certificate fingerprint must be 64 hex characters (SHA-256), got %d", len(s))
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", fmt.Errorf("the certificate fingerprint is not hexadecimal")
	}
	return strings.ToLower(s), nil
}

// isIPAddress reports whether host is a bare IPv4/IPv6 literal.
func isIPAddress(host string) bool {
	if i := strings.LastIndexByte(host, '%'); i >= 0 {
		host = host[:i] // strip a zone id like fe80::1%eth0
	}
	return net.ParseIP(host) != nil
}

func certNameOf(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}
	if len(cert.IPAddresses) > 0 {
		return cert.IPAddresses[0].String()
	}
	return ""
}
