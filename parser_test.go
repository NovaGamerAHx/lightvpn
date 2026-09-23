package main

import (
	"strings"
	"testing"
)

// The four link shapes from the spec, verbatim (with %2F-encoded alpn/path).

const (
	vlessWS = "vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@example.fr:443" +
		"?encryption=none&security=tls&sni=example.fr&fp=chrome&alpn=http%2F1.1" +
		"&type=ws&host=example.fr&path=%2Fv2rayws#DE-Frankfurt-01"

	vlessReality = "vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@203.0.113.9:8443" +
		"?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.microsoft.com&fp=chrome" +
		"&pbk=" + testPubKey + "&sid=0123abcd&spx=%2Fsearch&type=tcp#NL-Reality-Vision"

	trojanWS = "trojan://S3cr3t-Pa55w0rd@fr.example.net:2053" +
		"?security=tls&sni=fr.example.net&fp=chrome&alpn=http%2F1.1&type=ws&host=fr.example.net&path=%2Ftrojan#FR-Paris"

	trojanTCP = "trojan://S3cr3t-Pa55w0rd@fr.example.net:443?security=tls&sni=fr.example.net&fp=chrome&type=tcp#FR-direct"
)

func TestParseVLESSWebSocketTLS(t *testing.T) {
	l, err := ParseShareLink(vlessWS)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := map[string]any{
		"Protocol": "vless", "UserID": "1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d",
		"Address": "example.fr", "Port": 443, "Encryption": "none",
		"Security": "tls", "SNI": "example.fr", "Hostname": "example.fr",
		"Path": "/v2rayws", "Network": "ws", "Fingerprint": "chrome", "Name": "DE-Frankfurt-01",
	}
	check(t, l, want)
	if len(l.ALPN) != 1 || l.ALPN[0] != "http/1.1" {
		t.Errorf("alpn = %v, want [http/1.1]", l.ALPN)
	}
	if l.AllowInsecure {
		t.Error("allowInsecure must default to false")
	}
	if l.TransportLabel() != "WS · TLS" {
		t.Errorf("transport label = %q", l.TransportLabel())
	}
	if l.Endpoint() != "example.fr:443" {
		t.Errorf("endpoint = %q", l.Endpoint())
	}
	if l.ID == "" {
		t.Error("id must be filled in")
	}
}

func TestParseVLESSRealityVision(t *testing.T) {
	l, err := ParseShareLink(vlessReality)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	check(t, l, map[string]any{
		"Protocol": "vless", "Address": "203.0.113.9", "Port": 8443,
		"Security": "reality", "SNI": "www.microsoft.com", "Network": "tcp",
		"Flow": "xtls-rprx-vision", "PublicKey": testPubKey, "ShortID": "0123abcd",
		"SpiderX": "/search", "Fingerprint": "chrome", "Name": "NL-Reality-Vision",
	})
	if !l.HasFlow() {
		t.Error("HasFlow() must be true for xtls-rprx-vision")
	}
	if l.TransportLabel() != "TCP · REALITY" {
		t.Errorf("transport label = %q", l.TransportLabel())
	}
}

func TestParseTrojanWebSocketTLS(t *testing.T) {
	l, err := ParseShareLink(trojanWS)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	check(t, l, map[string]any{
		"Protocol": "trojan", "UserID": "S3cr3t-Pa55w0rd", "Address": "fr.example.net",
		"Port": 2053, "Security": "tls", "SNI": "fr.example.net", "Hostname": "fr.example.net",
		"Path": "/trojan", "Network": "ws", "Name": "FR-Paris",
	})
	if l.Flow != "" {
		t.Errorf("trojan must not carry a flow, got %q", l.Flow)
	}
}

func TestParseTrojanTCPTLS(t *testing.T) {
	l, err := ParseShareLink(trojanTCP)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	check(t, l, map[string]any{
		"Protocol": "trojan", "Address": "fr.example.net", "Port": 443,
		"Security": "tls", "Network": "tcp", "Name": "FR-direct",
	})
}

func TestParseGRPCAndSecurities(t *testing.T) {
	cases := []struct {
		link string
		want map[string]any
	}{
		{
			link: "vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@a.example:443?encryption=none&security=reality" +
				"&sni=a.example&fp=chrome&pbk=" + testPubKey + "&type=grpc&serviceName=rpc&mode=multi#grpc",
			want: map[string]any{"Network": "grpc", "ServiceName": "rpc", "GRPCMode": "multi", "Security": "reality"},
		},
		{
			link: "trojan://pw@a.example:443?security=tls&sni=a.example&type=httpupgrade&path=%2Fup#httpupgrade",
			want: map[string]any{"Network": "httpupgrade", "Path": "/up"},
		},
		{
			link: "vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@a.example:80?encryption=none&security=none&type=tcp&headerType=srtp#plain",
			want: map[string]any{"Security": "none", "HeaderType": "srtp", "SNI": ""},
		},
		{
			link: "vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@a.example:443?encryption=none&security=tls&allowInsecure=1&type=tcp#insecure",
			want: map[string]any{"AllowInsecure": true, "SNI": "a.example"},
		},
	}
	for _, c := range cases {
		l, err := ParseShareLink(c.link)
		if err != nil {
			t.Fatalf("parse %s: %v", c.want, err)
		}
		check(t, l, c.want)
	}
}

func TestParseLegacySemicolonParams(t *testing.T) {
	link := "vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@1.2.3.4:443" +
		"?encryption=none;security=tls;type=ws;host=cdn.example.com;path=/ray;ed=2048;sni=cdn.example.com;fp=firefox#legacy"
	l, err := ParseShareLink(link)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if l.Network != "ws" || l.Hostname != "cdn.example.com" {
		t.Errorf("legacy params not consumed: %+v", l)
	}
	if l.Path != "/ray?ed=2048" {
		t.Errorf("path = %q, want /ray?ed=2048", l.Path)
	}
	if l.Fingerprint != "firefox" {
		t.Errorf("fp = %q", l.Fingerprint)
	}
}

func TestParseIPv6AndQuirks(t *testing.T) {
	l, err := ParseShareLink("vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@[2001:db8::1]:2087?encryption=none&security=tls&type=tcp#v6")
	if err != nil {
		t.Fatalf("ipv6: %v", err)
	}
	if l.Address != "2001:db8::1" || l.Port != 2087 {
		t.Errorf("got %s:%d", l.Address, l.Port)
	}
	if l.Endpoint() != "[2001:db8::1]:2087" {
		t.Errorf("endpoint = %q, want [2001:db8::1]:2087", l.Endpoint())
	}

	// percent-encoded userinfo, uppercase scheme, trailing slash, surrounding quotes
	l2, err := ParseShareLink(` TROJAN://user%40name%3Apass@Host.Example:443/?security=tls&type=ws&path=%2F%3Fed%3D2048#✨%20node `)
	if err != nil {
		t.Fatalf("quirks: %v", err)
	}
	if l2.UserID != "user@name:pass" {
		t.Errorf("userinfo not decoded: %q", l2.UserID)
	}
	if l2.Name != "✨ node" {
		t.Errorf("fragment not decoded: %q", l2.Name)
	}
	if l2.Path != "/?ed=2048" {
		t.Errorf("path = %q", l2.Path)
	}
	if l2.Protocol != "trojan" {
		t.Errorf("scheme should be normalised to lowercase: %q", l2.Protocol)
	}

	// a flow on a trojan link is dropped with a note, not rejected (Xray removed it)
	lflow, err := ParseShareLink("trojan://pw@host.example:443?security=tls&type=tcp&flow=xtls-rprx-vision#flow")
	if err != nil {
		t.Fatalf("trojan flow must be tolerated: %v", err)
	}
	if lflow.Flow != "" {
		t.Errorf("trojan flow must be cleared, got %q", lflow.Flow)
	}
	if len(lflow.Notes) == 0 || !strings.Contains(lflow.Notes[0], "flow=") {
		t.Errorf("dropping an option must be reported in Notes, got %v", lflow.Notes)
	}

	// no fragment -> generated name
	l3, err := ParseShareLink("trojan://pw@host.example:443?security=tls&type=tcp")
	if err != nil {
		t.Fatalf("no-fragment: %v", err)
	}
	if l3.Name != "trojan host.example:443" {
		t.Errorf("generated name = %q", l3.Name)
	}
}

func TestParseRejectsBadLinks(t *testing.T) {
	bad := []struct {
		link string
		want string
	}{
		{"", "empty"},
		{"not a link at all", "expected vless:// or trojan://"},
		{"vmess://abcd", "vmess"},
		{"ss://abcd", "shadowsocks"},
		{"vless://example.fr:443?security=tls", "missing credentials"},
		{"vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@example.fr?security=tls", "missing port"},
		{"vless://no-a-uuid@example.fr:443?encryption=none", "invalid VLESS id"},
		{"trojan://@example.fr:443?security=tls", "password is empty"},
		{"vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@example.fr:443?security=reality&type=ws", "REALITY requires"},
		{"vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@example.fr:443?security=reality&type=tcp", "public key"},
		{"vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@example.fr:0?security=none", "port"},
		{"vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@example.fr:443?security=quic", "unsupported security"},
		{"vless://1a2b3c4d-5e6f-4a1b-9c8d-7e6f5a4b3c2d@example.fr:443?type=hysteria2", "unsupported transport"},
	}
	for _, c := range bad {
		_, err := ParseShareLink(c.link)
		if err == nil {
			t.Errorf("%q: expected an error mentioning %q, got none", c.link, c.want)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: error %q does not mention %q", c.link, err.Error(), c.want)
		}
	}
	// and the multi-line variant reports the line number
	_, perrs := ParseShareLinks("vless://" + testUUID + "@h:443?security=tls\ngarbage-line")
	if len(perrs) != 1 || perrs[0].Line != 2 {
		t.Errorf("expected one error on line 2, got %+v", perrs)
	}
}

func TestParseShareLinksBlob(t *testing.T) {
	blob := strings.Join([]string{
		"# a comment line from a subscription file",
		"",
		"   " + vlessWS + "   ",
		trojanTCP,
		vlessReality,
		vlessWS, // duplicate of line 3 -> reported, not added
		"totally-not-a-link",
	}, "\n")

	links, errs := ParseShareLinks(blob)
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d (%v)", len(links), links)
	}
	if len(errs) != 2 {
		t.Fatalf("expected 2 problems (dup + junk), got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Err, "duplicate") {
		t.Errorf("first problem should be the duplicate: %+v", errs[0])
	}
	ids := map[string]bool{}
	for _, l := range links {
		if ids[l.ID] {
			t.Errorf("duplicate id in output: %s", l.ID)
		}
		ids[l.ID] = true
	}
	// ids must be stable across parses
	again, _ := ParseShareLinks(blob)
	for i, l := range again {
		if l.ID != links[i].ID {
			t.Errorf("ids are not stable")
		}
	}
}

func TestSummaryRedactsSecrets(t *testing.T) {
	l, err := ParseShareLink(trojanWS)
	if err != nil {
		t.Fatal(err)
	}
	s := l.Summary()
	if strings.Contains(s, "S3cr3t-Pa55w0rd") {
		t.Errorf("Summary leaked the password: %s", s)
	}
	if !strings.Contains(s, "fr.example.net:2053") {
		t.Errorf("Summary lost the endpoint: %s", s)
	}
}

func check(t *testing.T, l *Link, want map[string]any) {
	t.Helper()
	for k, v := range want {
		switch k {
		case "Protocol":
			if l.Protocol != v {
				t.Errorf("Protocol = %v, want %v", l.Protocol, v)
			}
		case "Name":
			if l.Name != v {
				t.Errorf("Name = %q, want %q", l.Name, v)
			}
		case "Address":
			if l.Address != v {
				t.Errorf("Address = %v, want %v", l.Address, v)
			}
		case "Port":
			if l.Port != v {
				t.Errorf("Port = %v, want %v", l.Port, v)
			}
		case "UserID":
			if l.UserID != v {
				t.Errorf("UserID = %v, want %v", l.UserID, v)
			}
		case "Encryption":
			if l.Encryption != v {
				t.Errorf("Encryption = %v, want %v", l.Encryption, v)
			}
		case "Security":
			if l.Security != v {
				t.Errorf("Security = %v, want %v", l.Security, v)
			}
		case "SNI":
			if l.SNI != v {
				t.Errorf("SNI = %v, want %v", l.SNI, v)
			}
		case "Host":
		case "Hostname":
			if l.Hostname != v {
				t.Errorf("Hostname = %v, want %v", l.Hostname, v)
			}
		case "Path":
			if l.Path != v {
				t.Errorf("Path = %q, want %q", l.Path, v)
			}
		case "Network":
			if l.Network != v {
				t.Errorf("Network = %v, want %v", l.Network, v)
			}
		case "Flow":
			if l.Flow != v {
				t.Errorf("Flow = %v, want %v", l.Flow, v)
			}
		case "Fingerprint":
			if l.Fingerprint != v {
				t.Errorf("Fingerprint = %v, want %v", l.Fingerprint, v)
			}
		case "PublicKey":
			if l.PublicKey != v {
				t.Errorf("PublicKey = %v, want %v", l.PublicKey, v)
			}
		case "ShortID":
			if l.ShortID != v {
				t.Errorf("ShortID = %v, want %v", l.ShortID, v)
			}
		case "SpiderX":
			if l.SpiderX != v {
				t.Errorf("SpiderX = %v, want %v", l.SpiderX, v)
			}
		case "ServiceName":
			if l.ServiceName != v {
				t.Errorf("ServiceName = %v, want %v", l.ServiceName, v)
			}
		case "GRPCMode":
			if l.GRPCMode != v {
				t.Errorf("GRPCMode = %v, want %v", l.GRPCMode, v)
			}
		case "HeaderType":
			if l.HeaderType != v {
				t.Errorf("HeaderType = %v, want %v", l.HeaderType, v)
			}
		case "AllowInsecure":
			if l.AllowInsecure != v {
				t.Errorf("AllowInsecure = %v, want %v", l.AllowInsecure, v)
			}
		default:
			t.Fatalf("check(): unhandled key %q", k)
		}
	}
}
