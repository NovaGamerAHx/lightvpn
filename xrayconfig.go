package main

// xrayconfig.go — turns a parsed Link into a complete Xray-core JSON config:
// SOCKS5 + HTTP inbounds, the tunnel outbound, DNS and routing.
//
// The JSON is deliberately written against the stable public Xray config
// schema (infra/conf), so it is also readable/debuggable by hand — and the UI
// can show exactly what will be handed to the core.

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// CoreOptions control how the config for a node is generated.
type CoreOptions struct {
	SocksPort   int      `json:"socksPort"`
	HTTPPort    int      `json:"httpPort"`
	BypassLocal bool     `json:"bypassLocal"` // direct for RFC1918/loopback
	DNSServers  []string `json:"dnsServers"`
	Sniffing    bool     `json:"sniffing"`
	LogLevel    string   `json:"logLevel"` // "none" | "error" | "warning" | "info" | "debug"
	Tag         string   `json:"-"`
}

// DefaultCoreOptions matches the ports required by the spec.
func DefaultCoreOptions() CoreOptions {
	return CoreOptions{
		SocksPort:   10808,
		HTTPPort:    10809,
		BypassLocal: true,
		DNSServers:  []string{"223.5.5.5", "1.1.1.1", "8.8.8.8"},
		Sniffing:    true,
		LogLevel:    "warning",
	}
}

func (o *CoreOptions) normalise() {
	if o.SocksPort <= 0 {
		o.SocksPort = 10808
	}
	if o.HTTPPort <= 0 {
		o.HTTPPort = 10809
	}
	if len(o.DNSServers) == 0 {
		o.DNSServers = DefaultCoreOptions().DNSServers
	}
	if o.LogLevel == "" {
		o.LogLevel = "warning"
	}
	if o.Tag == "" {
		o.Tag = "proxy"
	}
}

// privateCIDRs are routed direct when BypassLocal is on. GeoIP files are not
// bundled (single EXE, no external dependencies), so this list is hard coded.
var privateCIDRs = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/128",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
}

// BuildXrayConfig returns the JSON config for one node.
func BuildXrayConfig(l *Link, opts CoreOptions) ([]byte, error) {
	if l == nil {
		return nil, fmt.Errorf("no config selected")
	}
	opts.normalise()
	if err := l.Validate(); err != nil {
		return nil, err
	}

	outbound, err := buildOutbound(l, opts.Tag)
	if err != nil {
		return nil, err
	}

	cfg := map[string]any{
		"log": map[string]any{
			"loglevel": opts.LogLevel,
			"access":   "none",
			"error":    "",
		},
		"dns": map[string]any{
			"servers":             dnsServers(opts.DNSServers),
			"queryStrategy":       "useip",
			"disableFallback":     false,
			"enableParallelQuery": true,
		},
		"inbounds": []any{
			map[string]any{
				"tag":      "socks-in",
				"port":     opts.SocksPort,
				"listen":   "127.0.0.1",
				"protocol": "socks",
				// No "timeout": SocksServerConfig has no such field (it is a
				// legacy V2Ray key Xray ignores).
				"settings": map[string]any{
					"auth": "noauth",
					"udp":  true,
					"ip":   "127.0.0.1",
				},
				"sniffing": sniffing(opts.Sniffing),
			},
			map[string]any{
				"tag":      "http-in",
				"port":     opts.HTTPPort,
				"listen":   "127.0.0.1",
				"protocol": "http",
				"settings": map[string]any{
					"allowTransparent": true,
				},
				"sniffing": sniffing(opts.Sniffing),
			},
		},
		"outbounds": []any{
			outbound,
			map[string]any{
				"tag":      "direct",
				"protocol": "freedom",
				"settings": map[string]any{"domainStrategy": "AsIs"},
			},
			map[string]any{
				"tag":      "block",
				"protocol": "blackhole",
				"settings": map[string]any{"response": map[string]any{"type": "http"}},
			},
		},
		"routing": buildRouting(opts),
	}

	return marshalIndent(cfg)
}

func sniffing(on bool) map[string]any {
	return map[string]any{
		"enabled":      on,
		"destOverride": []string{"http", "tls", "quic"},
		"metadataOnly": false,
		"routeOnly":    false,
	}
}

func dnsServers(list []string) []any {
	out := make([]any, 0, len(list))
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, map[string]any{"address": s, "port": 53})
	}
	if len(out) == 0 {
		out = append(out, map[string]any{"address": "1.1.1.1", "port": 53})
	}
	return out
}

func buildRouting(o CoreOptions) map[string]any {
	rules := []any{}

	// Ad-block-ish noise: never let bittorrent saturate the tunnel.
	rules = append(rules, map[string]any{
		"type": "field", "protocol": []string{"bittorrent"}, "outboundTag": "block",
	})

	if o.BypassLocal {
		rules = append(rules,
			map[string]any{
				"type": "field",
				"domain": []string{
					"full:localhost", "domain:localhost", "keyword:.local",
					"domain:lan", "domain:internal", "domain:local",
				},
				"outboundTag": "direct",
			},
			map[string]any{
				"type":        "field",
				"ip":          privateCIDRs,
				"outboundTag": "direct",
			},
			// Private network printer/NAS discovery ports stay local.
			map[string]any{
				"type":        "field",
				"port":        "53,137-139,445,1900,5353,5355",
				"network":     "udp",
				"outboundTag": "direct",
			},
		)
	}

	return map[string]any{
		"domainStrategy": "AsIs",
		// Anything not matched by a rule above uses the first outbound == the proxy.
		"rules": rules,
	}
}

// buildOutbound produces the Xray outbound object for this node.
func buildOutbound(l *Link, tag string) (map[string]any, error) {
	stream, err := buildStreamSettings(l)
	if err != nil {
		return nil, err
	}

	out := map[string]any{
		"tag":            tag,
		"protocol":       l.Protocol,
		"streamSettings": stream,
		"mux":            map[string]any{"enabled": false},
	}

	switch l.Protocol {
	case ProtoVLESS:
		user := map[string]any{"id": strings.ToLower(l.UserID), "encryption": orDefault(l.Encryption, "none")}
		if l.HasFlow() {
			user["flow"] = l.Flow
		}
		out["settings"] = map[string]any{
			"vnext": []any{map[string]any{
				"address": l.Address,
				"port":    l.Port,
				"users":   []any{user},
			}},
		}
	case ProtoTrojan:
		out["settings"] = map[string]any{
			"servers": []any{map[string]any{
				"address":  l.Address,
				"port":     l.Port,
				"password": l.UserID,
				"level":    0,
			}},
		}
	default:
		return nil, fmt.Errorf("unsupported protocol %q", l.Protocol)
	}
	return out, nil
}

func buildStreamSettings(l *Link) (map[string]any, error) {
	stream := map[string]any{"network": normalizeNetwork(l.Network), "security": l.Security}

	switch normalizeNetwork(l.Network) {
	case NetTCP:
		ss := map[string]any{}
		if l.HeaderType != "" {
			ss["header"] = map[string]any{"type": l.HeaderType}
		}
		stream["tcpSettings"] = ss
	case NetWS:
		ws := map[string]any{}
		if l.Path != "" {
			ws["path"] = l.Path
		}
		if l.Hostname != "" {
			ws["host"] = l.Hostname
		}
		stream["wsSettings"] = ws
	case NetHTTPUpgrade:
		hs := map[string]any{}
		if l.Path != "" {
			hs["path"] = l.Path
		}
		if l.Hostname != "" {
			hs["host"] = l.Hostname
		}
		stream["httpupgradeSettings"] = hs
	case NetGRPC:
		g := map[string]any{"serviceName": orDefault(l.ServiceName, "grpc")}
		if l.GRPCMode == "multi" {
			g["multiMode"] = true
		}
		if l.Hostname != "" {
			g["authority"] = l.Hostname
		} else if l.SNI != "" {
			g["authority"] = l.SNI
		}
		stream["grpcSettings"] = g
	case NetXHTTP:
		x := map[string]any{}
		if l.Path != "" {
			x["path"] = l.Path
		}
		if l.Hostname != "" {
			x["host"] = l.Hostname
		}
		if m := orDefault(l.Mode, "auto"); m != "" {
			x["mode"] = m
		}
		stream["xhttpSettings"] = x
	}

	switch normalizeSecurity(l.Security) {
	case SecNone:
	case SecTLS:
		tls := map[string]any{}
		if l.SNI != "" {
			tls["serverName"] = l.SNI
		}
		if len(l.ALPN) > 0 {
			tls["alpn"] = strings.Join(l.ALPN, ",")
		}
		if fp := l.Fingerprint; fp != "" {
			tls["fingerprint"] = fp
		}
		// "allowInsecure" is a *removed* Xray feature (hard config error since
		// 2026-06-01) even though share links still carry it. Its supported
		// equivalent is pinning the peer certificate; ResolveCertPin computes
		// that fingerprint for allowInsecure nodes before we start the core, so
		// all this function has to do is translate l.PinnedCertSHA256.
		if pin, err := NormalizeCertFingerprint(l.PinnedCertSHA256); err != nil {
			return nil, err
		} else if pin != "" {
			tls["pinnedPeerCertSha256"] = pin
		} else if l.AllowInsecure {
			return nil, fmt.Errorf("this node asks to skip certificate verification, which Xray no longer supports — connect once to pin its certificate automatically (Settings → Allow insecure does this for every node)")
		}
		stream["tlsSettings"] = tls
	case SecReality:
		rs, err := buildRealitySettings(l)
		if err != nil {
			return nil, err
		}
		stream["realitySettings"] = rs
		// REALITY implies a TLS-shaped handshake; a flow may still be set.
	default:
		return nil, fmt.Errorf("unsupported security %q", l.Security)
	}
	return stream, nil
}

func buildRealitySettings(l *Link) (map[string]any, error) {
	pbk, err := normalizeRealityKey(l.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid pbk (public key): %w", err)
	}
	sid := strings.ToLower(strings.TrimSpace(l.ShortID))
	if sid != "" {
		if len(sid) > 16 || len(sid)%2 == 1 {
			return nil, fmt.Errorf("invalid sid (short id) %q: expected 0-16 hex chars", l.ShortID)
		}
		if _, err := hex.DecodeString(sid); err != nil {
			return nil, fmt.Errorf("invalid sid (short id) %q: not hexadecimal", l.ShortID)
		}
	}
	spx := l.SpiderX
	if spx == "" {
		spx = "/"
	}
	if !strings.HasPrefix(spx, "/") {
		spx = "/" + spx
	}
	fp := strings.ToLower(orDefault(l.Fingerprint, "chrome"))
	switch fp {
	case "chrome", "firefox", "safari", "edge", "ios", "android", "qq", "360", "random", " randomized", "tlsauto":
	default:
		return nil, fmt.Errorf("unknown fp (fingerprint) %q — use chrome, firefox, safari, edge, ios, android, qq, 360 or random", l.Fingerprint)
	}
	rs := map[string]any{
		"fingerprint": fp,
		"publicKey":   pbk,
		"spiderX":     spx,
	}
	if l.SNI != "" {
		rs["serverName"] = l.SNI
	}
	if sid != "" {
		rs["shortId"] = sid
	}
	// NOTE: realitySettings has no allowInsecure in Xray at all — REALITY needs no
	// certificate verification, so a stray allowInsecure=1 in the link is dropped.
	return rs, nil
}

// normalizeRealityKey accepts both URL-safe and standard base64 and always
// returns the padded-free URL-safe form that Xray expects.
func normalizeRealityKey(k string) (string, error) {
	k = strings.TrimSpace(k)
	if k == "" {
		return "", fmt.Errorf("empty key")
	}
	k = strings.TrimRight(k, "=")
	// Try url-safe first (the usual form in share links), then std base64.
	if raw, err := base64.RawURLEncoding.DecodeString(k); err == nil && len(raw) == 32 {
		return base64.RawURLEncoding.EncodeToString(raw), nil
	}
	std := strings.NewReplacer("-", "+", "_", "/").Replace(k)
	if raw, err := base64.RawStdEncoding.DecodeString(std); err == nil && len(raw) == 32 {
		return base64.RawURLEncoding.EncodeToString(raw), nil
	}
	if raw, err := base64.StdEncoding.DecodeString(k); err == nil && len(raw) == 32 {
		return base64.RawURLEncoding.EncodeToString(raw), nil
	}
	return "", fmt.Errorf("%q is not a 32 byte base64 key", clip(k, 24))
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// ConfigSummary is what the UI shows in the "generated config" preview.
func ConfigSummary(l *Link, o CoreOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s · %s · %s\n", l.Name, strings.ToUpper(l.Protocol), l.TransportLabel())
	fmt.Fprintf(&b, "server %s\n", l.Endpoint())
	if l.SNI != "" {
		fmt.Fprintf(&b, "sni %s", l.SNI)
	}
	if l.Fingerprint != "" {
		fmt.Fprintf(&b, "  fp %s", l.Fingerprint)
	}
	if l.Flow != "" {
		fmt.Fprintf(&b, "\nflow %s", l.Flow)
	}
	fmt.Fprintf(&b, "\nsocks 127.0.0.1:%d   http 127.0.0.1:%d", o.SocksPort, o.HTTPPort)
	if o.BypassLocal {
		b.WriteString("\nrouting local traffic direct")
	}
	return b.String()
}

// sortedKeys is used by tests and the details view for deterministic output.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
