package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	core "github.com/xtls/xray-core/core"
)

// loadThroughXray feeds a generated config to Xray's own JSON loader. This is the
// strongest check available without touching the network: every field name and
// value must be understood by the exact xray-core version we ship with.
func loadThroughXray(t *testing.T, cfg []byte) {
	t.Helper()
	if _, err := core.LoadConfig("json", bytes.NewReader(cfg)); err != nil {
		t.Fatalf("xray rejected the generated config: %v\n%s", err, string(cfg))
	}
}

func TestAllRequiredFormatsAreAcceptedByXray(t *testing.T) {
	links := []string{
		vlessWS,
		vlessReality,
		trojanWS,
		trojanTCP,
		"vless://" + testUUID + "@host.example:443?encryption=none&security=reality&sni=host.example&fp=chrome&pbk=" + testPubKey + "&sid=00&spx=%2F&type=grpc&serviceName=x&mode=multi#grpc-reality",
		"vless://" + testUUID + "@host.example:443?encryption=none&security=tls&type=tcp&headerType=none#plain-tls",
		"vless://" + testUUID + "@host.example:8443?encryption=none&flow=xtls-rprx-vision&security=tls&sni=host.example&fp=firefox&type=tcp#vision-over-tls",
		"trojan://pw@host.example:443?security=tls&type=httpupgrade&path=%2Fx#httpupgrade",
		"vless://" + testUUID + "@host.example:443?encryption=none&security=none&type=tcp#no-tls",
	}
	for _, raw := range links {
		l, err := ParseShareLink(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", clip(raw, 60), err)
		}
		cfg, err := BuildXrayConfig(l, DefaultCoreOptions())
		if err != nil {
			t.Fatalf("build %q: %v", l.Name, err)
		}
		t.Run(l.Name, func(t *testing.T) { loadThroughXray(t, cfg) })
	}
}

func TestConfigShape(t *testing.T) {
	l, err := ParseShareLink(vlessWS)
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultCoreOptions()
	cfg, err := BuildXrayConfig(l, opts)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(cfg, &doc); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}

	inb, ok := doc["inbounds"].([]any)
	if !ok || len(inb) != 2 {
		t.Fatalf("expected 2 inbounds, got %v", doc["inbounds"])
	}
	socks := inb[0].(map[string]any)
	httpIn := inb[1].(map[string]any)
	if socks["protocol"] != "socks" || int(socks["port"].(float64)) != 10808 || socks["listen"] != "127.0.0.1" {
		t.Errorf("socks inbound wrong: %v", socks)
	}
	if socks["settings"].(map[string]any)["udp"] != true {
		t.Error("socks inbound must allow UDP")
	}
	if httpIn["protocol"] != "http" || int(httpIn["port"].(float64)) != 10809 || httpIn["listen"] != "127.0.0.1" {
		t.Errorf("http inbound wrong: %v", httpIn)
	}

	outb := doc["outbounds"].([]any)
	if len(outb) != 3 {
		t.Fatalf("expected proxy+direct+block outbounds, got %d", len(outb))
	}
	first := outb[0].(map[string]any)
	if first["tag"] != "proxy" || first["protocol"] != "vless" {
		t.Errorf("the first outbound must be the tunnel (it is the default route): %v", first)
	}
	vnext := first["settings"].(map[string]any)["vnext"].([]any)[0].(map[string]any)
	if vnext["address"] != "example.fr" || int(vnext["port"].(float64)) != 443 {
		t.Errorf("vnext wrong: %v", vnext)
	}
	user := vnext["users"].([]any)[0].(map[string]any)
	if user["id"] != testUUID || user["encryption"] != "none" {
		t.Errorf("vless user wrong: %v", user)
	}
	if _, has := user["flow"]; has {
		t.Error("flow must be absent when the link has none")
	}

	stream := first["streamSettings"].(map[string]any)
	if stream["network"] != "ws" || stream["security"] != "tls" {
		t.Errorf("stream wrong: %v", stream)
	}
	ws := stream["wsSettings"].(map[string]any)
	if ws["path"] != "/v2rayws" || ws["host"] != "example.fr" {
		t.Errorf("wsSettings wrong: %v", ws)
	}
	tlsS := stream["tlsSettings"].(map[string]any)
	if tlsS["serverName"] != "example.fr" || tlsS["fingerprint"] != "chrome" || tlsS["alpn"] != "http/1.1" {
		t.Errorf("tlsSettings wrong: %v", tlsS)
	}

	rules := doc["routing"].(map[string]any)["rules"].([]any)
	if len(rules) == 0 {
		t.Fatal("routing rules missing")
	}
	var directBlob strings.Builder
	for _, r := range rules {
		m := r.(map[string]any)
		if m["outboundTag"] == "direct" {
			b, _ := json.Marshal(m)
			directBlob.Write(b)
		}
	}
	direct := directBlob.String()
	if direct == "" {
		t.Fatal("expected a direct route for local traffic")
	}
	for _, want := range []string{"192.168.0.0/16", "127.0.0.0/8", "10.0.0.0/8", "::1/128"} {
		if !strings.Contains(direct, want) {
			t.Errorf("private range %s must be bypassed, direct rules: %s", want, direct)
		}
	}
	if _, ok := doc["dns"]; !ok {
		t.Error("dns section missing")
	}

	// Bypass off => no direct rules at all.
	opts.BypassLocal = false
	cfg2, err := BuildXrayConfig(l, opts)
	if err != nil {
		t.Fatal(err)
	}
	var doc2 map[string]any
	_ = json.Unmarshal(cfg2, &doc2)
	for _, r := range doc2["routing"].(map[string]any)["rules"].([]any) {
		if r.(map[string]any)["outboundTag"] == "direct" {
			t.Error("BypassLocal=false must not emit direct rules")
		}
	}
}

func TestRealitySettingsMapping(t *testing.T) {
	l, err := ParseShareLink(vlessReality)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := BuildXrayConfig(l, DefaultCoreOptions())
	if err != nil {
		t.Fatal(err)
	}
	loadThroughXray(t, cfg)

	var doc map[string]any
	_ = json.Unmarshal(cfg, &doc)
	stream := doc["outbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)
	if stream["security"] != "reality" {
		t.Fatalf("security = %v", stream["security"])
	}
	rs := stream["realitySettings"].(map[string]any)
	if rs["publicKey"] != testPubKey {
		t.Errorf("pbk = %v, want %v", rs["publicKey"], testPubKey)
	}
	if rs["shortId"] != "0123abcd" {
		t.Errorf("sid = %v", rs["shortId"])
	}
	if rs["spiderX"] != "/search" {
		t.Errorf("spx = %v (must keep the leading slash)", rs["spiderX"])
	}
	if rs["serverName"] != "www.microsoft.com" {
		t.Errorf("sni = %v", rs["serverName"])
	}
	if rs["fingerprint"] != "chrome" {
		t.Errorf("fp = %v", rs["fingerprint"])
	}
	if _, has := stream["tlsSettings"]; has {
		t.Error("reality must not also emit tlsSettings")
	}
	// flow belongs on the vless user
	user := doc["outbounds"].([]any)[0].(map[string]any)["settings"].(map[string]any)["vnext"].([]any)[0].(map[string]any)["users"].([]any)[0].(map[string]any)
	if user["flow"] != "xtls-rprx-vision" {
		t.Errorf("flow = %v", user["flow"])
	}
}

func TestRealityKeyNormalisation(t *testing.T) {
	// A panel that emitted standard base64 (with + / and padding) must still work.
	link := "vless://" + testUUID + "@h.example:443?encryption=none&security=reality&sni=h.example&pbk=" + testPubKeyURL + "&type=tcp#k"
	std := strings.ReplaceAll(link, testPubKeyURL, testPubKeyStd)

	a, err := ParseShareLink(link)
	if err != nil {
		t.Fatalf("url-safe form: %v", err)
	}
	b, err := ParseShareLink(std)
	if err != nil {
		t.Fatalf("standard form: %v", err)
	}
	if a.ID != b.ID {
		t.Errorf("the two encodings must describe the same node: %s vs %s", a.ID, b.ID)
	}
	na, err := normalizeRealityKey(testPubKeyStd)
	if err != nil {
		t.Fatal(err)
	}
	if na != testPubKeyURL {
		t.Errorf("normalise() = %q, want %q", na, testPubKeyURL)
	}
	if _, err := normalizeRealityKey("not-a-key"); err == nil {
		t.Error("garbage keys must be rejected")
	}
}

func TestRealityGuards(t *testing.T) {
	mk := func(q string) (*Link, error) {
		return ParseShareLink("vless://" + testUUID + "@h.example:443?encryption=none&security=reality&pbk=" + testPubKey + "&" + q + "#g")
	}
	// bad short id
	l, err := mk("sid=zzzz&type=tcp")
	if err == nil {
		cfg, berr := BuildXrayConfig(l, DefaultCoreOptions())
		if berr == nil {
			t.Error("non-hex sid must be rejected, got " + string(cfg))
		}
	} else if !strings.Contains(err.Error(), "REALITY") {
		t.Errorf("unexpected parse error: %v", err)
	}
	// unknown fingerprint (xray itself would refuse it)
	l, err = mk("fp=netscape&type=tcp")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := BuildXrayConfig(l, DefaultCoreOptions()); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Errorf("unknown fingerprint must produce a clear error, got %v", err)
	}
	// missing fingerprint defaults to chrome and stays valid
	l, err = mk("type=tcp")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := BuildXrayConfig(l, DefaultCoreOptions())
	if err != nil {
		t.Fatalf("default fingerprint: %v", err)
	}
	loadThroughXray(t, cfg)
	if !strings.Contains(string(cfg), `"fingerprint": "chrome"`) {
		t.Error("expected the chrome default fingerprint in the config")
	}
}

// Xray removed "allowInsecure" outright (config error since 2026-06-01). A link
// carrying it must still import, and the generated config must express the same
// intent through the supported mechanism instead.
func TestAllowInsecureIsNeverEmitted(t *testing.T) {
	l, err := ParseShareLink("trojan://pw@h.example:443?security=tls&sni=h.example&alpn=h2%2Chttp%2F1.1&allowInsecure=1&type=ws&path=%2Fx#x")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.ALPN) != 2 || l.ALPN[0] != "h2" || l.ALPN[1] != "http/1.1" {
		t.Errorf("alpn = %v", l.ALPN)
	}
	if !l.AllowInsecure {
		t.Fatal("allowInsecure=1 must be remembered on the link")
	}
	if _, err := BuildXrayConfig(l, DefaultCoreOptions()); err == nil {
		t.Error("without a pin the config must refuse to be built, not emit a removed option")
	} else if !strings.Contains(err.Error(), "pin") {
		t.Errorf("the error must say what to do, got: %v", err)
	}

	l.PinnedCertSHA256 = strings.Repeat("ab", 32)
	cfg, err := BuildXrayConfig(l, DefaultCoreOptions())
	if err != nil {
		t.Fatal(err)
	}
	loadThroughXray(t, cfg)
	if strings.Contains(string(cfg), "allowInsecure") {
		t.Error("the removed allowInsecure key must never be emitted")
	}
	if !strings.Contains(string(cfg), `"pinnedPeerCertSha256": "`+strings.Repeat("ab", 32)+`"`) {
		t.Errorf("pin missing:\n%s", cfg)
	}
}

func TestPCSParamFromLink(t *testing.T) {
	fp := strings.Repeat("0A", 32)
	l, err := ParseShareLink("trojan://pw@h.example:443?security=tls&sni=h.example&pcs=" + fp + "&type=tcp#pinned")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.PinnedCertSHA256) != 64 || l.PinnedCertSHA256 != strings.ToLower(fp) {
		t.Errorf("pcs = %q", l.PinnedCertSHA256)
	}
	cfg, err := BuildXrayConfig(l, DefaultCoreOptions())
	if err != nil {
		t.Fatal(err)
	}
	loadThroughXray(t, cfg)
	if !strings.Contains(string(cfg), strings.ToLower(fp)) {
		t.Error("the pinned fingerprint from the link must reach the config (normalised to lowercase)")
	}
	// a fingerprint written with colons (OpenSSL style) is equally acceptable
	l2, err := ParseShareLink("trojan://pw@h.example:443?security=tls&pcs=" + strings.Join(strings.Split(strings.Repeat("0a", 32), ""), ":") + "&type=tcp#c")
	if err != nil {
		t.Fatalf("colon form: %v", err)
	}
	if l2.PinnedCertSHA256 != strings.ToLower(fp) {
		t.Errorf("colon separators were not stripped: %q", l2.PinnedCertSHA256)
	}
	if _, err := ParseShareLink("trojan://pw@h.example:443?security=tls&pcs=zz&type=tcp#bad"); err == nil {
		t.Error("a malformed pcs must be rejected at import time")
	}
}

func TestConfigPreviewIsPretty(t *testing.T) {
	l, err := ParseShareLink(vlessWS)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ConfigJSON(l, DefaultCoreOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, "\n  \"inbounds\"") || !strings.Contains(s, "  \"streamSettings\"") {
		t.Error("preview must be indented JSON")
	}
	if strings.Contains(s, testUUID) == false {
		t.Error("preview must contain the node id (it is the config we run)")
	}
}
