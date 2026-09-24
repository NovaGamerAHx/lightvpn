package main

// parser.go — converts vless:// and trojan:// share links into a normalised Link
// structure that the rest of the app (Xray config generation, ping, UI) uses.
//
// Supported URL shapes:
//
//	vless://<uuid>@<addr>:<port>?encryption=none&security=tls&sni=..&fp=..&type=ws&host=..&path=..#name
//	vless://<uuid>@<addr>:<port>?...&security=reality&pbk=..&sid=..&spx=..&flow=xtls-rprx-vision#name
//	trojan://<password>@<addr>:<port>?security=tls&sni=..&fp=chrome&type=ws&host=..&path=..#name
//	trojan://<password>@<addr>:<port>?security=tls&sni=..&fp=chrome&type=tcp#name
//
// The legacy parameter style (some panels emit "a=b;c=d") is accepted as well.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Protocols understood by the parser.
const (
	ProtoVLESS  = "vless"
	ProtoTrojan = "trojan"
)

// Transport and security names.
const (
	NetTCP         = "tcp"
	NetWS          = "ws"
	NetGRPC        = "grpc"
	NetHTTPUpgrade = "httpupgrade"
	NetXHTTP       = "xhttp"

	SecNone    = "none"
	SecTLS     = "tls"
	SecReality = "reality"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Link is one imported node. It is persisted verbatim inside config.json, so
// every field carries a json tag.
type Link struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Raw      string `json:"raw"`
	Protocol string `json:"protocol"`

	Address string `json:"address"`
	Port    int    `json:"port"`

	// Credentials: UUID for VLESS, password for Trojan.
	UserID     string `json:"userId"`
	Encryption string `json:"encryption,omitempty"`
	Flow       string `json:"flow,omitempty"`

	Security string `json:"security"`
	SNI      string `json:"sni,omitempty"`
	Hostname string `json:"host,omitempty"`
	Path     string `json:"path,omitempty"`

	Network    string   `json:"network"`
	HeaderType string   `json:"headerType,omitempty"`
	ALPN       []string `json:"alpn,omitempty"`

	Fingerprint   string `json:"fp,omitempty"`
	AllowInsecure bool   `json:"allowInsecure,omitempty"`

	// REALITY
	PublicKey string `json:"pbk,omitempty"`
	ShortID   string `json:"sid,omitempty"`
	SpiderX   string `json:"spx,omitempty"`

	// gRPC
	ServiceName string `json:"serviceName,omitempty"`
	GRPCMode    string `json:"grpcMode,omitempty"` // "" (gun) | "multi"
	// XHTTP / other
	Mode  string            `json:"mode,omitempty"`
	Extra map[string]string `json:"extra,omitempty"`

	// Notes are non-fatal normalisations the parser applied ("flow ignored for
	// trojan", …). Shown in the UI so a silently-dropped option is not a mystery.
	Notes []string `json:"notes,omitempty"`

	// PinnedCertSHA256 is Xray's replacement for allowInsecure ("pinnedPeerCert-
	// Sha256"). It comes from the pcs= link parameter or from a trust-on-first-use
	// probe done when connecting.
	PinnedCertSHA256 string `json:"pcs,omitempty"`
	CertNote         string `json:"certNote,omitempty"`

	// Measured state (persisted so the list looks warm right after startup).
	PingMS    float64 `json:"pingMs,omitempty"`
	PingError string  `json:"pingError,omitempty"`
	PingAt    int64   `json:"pingAt,omitempty"`
	AddedAt   int64   `json:"addedAt,omitempty"`
}

// Clone returns a deep copy so callers can never mutate stored state by accident.
func (l *Link) Clone() *Link {
	if l == nil {
		return nil
	}
	b, err := json.Marshal(l)
	if err != nil {
		cp := *l
		return &cp
	}
	var out Link
	if err := json.Unmarshal(b, &out); err != nil {
		cp := *l
		return &cp
	}
	out.Extra = l.Extra
	return &out
}

// Endpoint is the host:port used for dialing and for ping.
func (l *Link) Endpoint() string {
	host := l.Address
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return host + ":" + strconv.Itoa(l.Port)
}

// TransportLabel is the compact description used by the UI badges,
// e.g. "WS · TLS" or "TCP · REALITY".
func (l *Link) TransportLabel() string {
	network := strings.ToUpper(normalizeNetwork(l.Network))
	security := strings.ToUpper(normalizeSecurity(l.Security))
	if security == SecNone || security == "" {
		return network
	}
	return network + " · " + security
}

// HasFlow reports whether an XTLS-Vision style flow is active.
func (l *Link) HasFlow() bool { return l.Flow != "" && l.Flow != "none" }

// Validate checks everything the core needs in order to build a working config.
func (l *Link) Validate() error {
	switch l.Protocol {
	case ProtoVLESS:
		if !uuidRe.MatchString(l.UserID) {
			return fmt.Errorf("invalid VLESS id %q — expected a UUID", clip(l.UserID, 24))
		}
	case ProtoTrojan:
		if l.UserID == "" {
			return fmt.Errorf("trojan password is empty")
		}
	default:
		return fmt.Errorf("unsupported protocol %q (only vless and trojan)", l.Protocol)
	}
	if strings.TrimSpace(l.Address) == "" {
		return fmt.Errorf("server address is empty")
	}
	if l.Port < 1 || l.Port > 65535 {
		return fmt.Errorf("invalid port %d", l.Port)
	}
	switch normalizeNetwork(l.Network) {
	case NetTCP, NetWS, NetGRPC, NetHTTPUpgrade, NetXHTTP:
	default:
		return fmt.Errorf("unsupported transport %q (want tcp, ws, grpc, httpupgrade or xhttp)", l.Network)
	}
	switch normalizeSecurity(l.Security) {
	case SecNone:
	case SecTLS:
	case SecReality:
		n := normalizeNetwork(l.Network)
		if n != NetTCP && n != NetGRPC && n != NetXHTTP {
			return fmt.Errorf("REALITY requires type=tcp, type=grpc or type=xhttp (got type=%s)", l.Network)
		}
		if l.PublicKey == "" {
			return fmt.Errorf("REALITY is missing the public key (pbk)")
		}
	default:
		return fmt.Errorf("unsupported security %q (want none, tls or reality)", l.Security)
	}
	if l.HasFlow() && normalizeSecurity(l.Security) == SecNone {
		return fmt.Errorf("flow=%s requires security=tls or security=reality", l.Flow)
	}
	return nil
}

// Summary is one line of text that is safe to log (credentials redacted).
func (l *Link) Summary() string {
	return fmt.Sprintf("%s://%s@%s type=%s security=%s", l.Protocol,
		redact(l.UserID), l.Endpoint(), normalizeNetwork(l.Network), normalizeSecurity(l.Security))
}

func redact(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "…" + s[len(s)-4:]
}

// ParseError reports why a single line could not be imported.
type ParseError struct {
	Line int    `json:"line"`
	Text string `json:"text"` // offending input, clipped
	Err  string `json:"error"`
}

func (e ParseError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Err) }

// ParseShareLinks parses a blob with one share link per line. Blank lines and
// comment lines are ignored, which makes pasting a whole subscription file work.
// Duplicates (same connection parameters) are reported instead of re-added.
func ParseShareLinks(blob string) (links []*Link, perrs []ParseError) {
	trimmed := strings.TrimSpace(blob)
	if !strings.Contains(trimmed, "://") && len(trimmed) > 16 {
		clean := strings.ReplaceAll(strings.ReplaceAll(trimmed, "\r", ""), "\n", "")
		clean = strings.TrimSpace(clean)
		if dec, err := base64.StdEncoding.DecodeString(clean); err == nil && strings.Contains(string(dec), "://") {
			blob = string(dec)
		} else if dec, err := base64.RawStdEncoding.DecodeString(clean); err == nil && strings.Contains(string(dec), "://") {
			blob = string(dec)
		} else if dec, err := base64.URLEncoding.DecodeString(clean); err == nil && strings.Contains(string(dec), "://") {
			blob = string(dec)
		} else if dec, err := base64.RawURLEncoding.DecodeString(clean); err == nil && strings.Contains(string(dec), "://") {
			blob = string(dec)
		}
	}

	seen := map[string]string{}
	for i, line := range strings.Split(strings.ReplaceAll(blob, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "\"'"))
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "#") && !strings.Contains(line, "://") {
			continue // subscription "# comment"
		}
		if !strings.Contains(line, "://") {
			perrs = append(perrs, ParseError{Line: i + 1, Text: clip(line, 60),
				Err: "not a share link — expected vless:// or trojan://"})
			continue
		}
		l, err := ParseShareLink(line)
		if err != nil {
			perrs = append(perrs, ParseError{Line: i + 1, Text: clip(line, 60), Err: err.Error()})
			continue
		}
		if prev, dup := seen[l.ID]; dup {
			perrs = append(perrs, ParseError{Line: i + 1, Text: clip(l.Name, 40),
				Err: "duplicate of “" + prev + "”"})
			continue
		}
		seen[l.ID] = l.Name
		links = append(links, l)
	}
	return links, perrs
}

// ParseShareLink parses exactly one share link.
func ParseShareLink(raw string) (*Link, error) {
	s := strings.TrimSpace(strings.Trim(strings.TrimSpace(raw), "\"'"))
	if s == "" {
		return nil, fmt.Errorf("empty link")
	}

	var protocol string
	switch {
	case hasPrefixFold(s, "vless://"):
		protocol = ProtoVLESS
	case hasPrefixFold(s, "trojan://"):
		protocol = ProtoTrojan
	case hasPrefixFold(s, "vmess://"):
		return nil, fmt.Errorf("vmess:// links are not supported by this client")
	case hasPrefixFold(s, "ss://"):
		return nil, fmt.Errorf("ss:// (shadowsocks) links are not supported by this client")
	case hasPrefixFold(s, "hysteria://"), hasPrefixFold(s, "tuic://"), hasPrefixFold(s, "shadowsocks://"):
		return nil, fmt.Errorf("%s links are not supported by this client", strings.SplitN(s, ":", 2)[0])
	default:
		return nil, fmt.Errorf("unknown scheme — expected vless:// or trojan://")
	}

	rest := s[len(protocol)+3:]

	// ---- fragment => display name --------------------------------------------
	var name string
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		name = unescape(rest[i+1:])
		rest = rest[:i]
	}

	// ---- query / legacy params -----------------------------------------------
	var query string
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		query, rest = rest[i+1:], rest[:i]
	}
	params, err := parseParams(query)
	if err != nil {
		return nil, err
	}

	// ---- userinfo@host:port -----------------------------------------------------
	var userinfo, hostport string
	if i := strings.LastIndexByte(rest, '@'); i < 0 {
		return nil, fmt.Errorf("missing credentials — expected %s://<id>@<server>:<port>", protocol)
	} else {
		userinfo, hostport = rest[:i], rest[i+1:]
	}

	address, port, err := splitHostPort(hostport)
	if err != nil {
		return nil, fmt.Errorf("%w (in %s://…@%s)", err, protocol, clip(rest, 60))
	}

	l := &Link{
		Protocol:      protocol,
		Name:          strings.TrimSpace(name),
		Raw:           s,
		Address:       address,
		Port:          port,
		UserID:        strings.TrimSpace(unescape(userinfo)),
		Encryption:    strings.ToLower(param(params, "encryption")),
		Flow:          strings.ToLower(param(params, "flow")),
		Security:      normalizeSecurity(param(params, "security")),
		SNI:           param(params, "sni", "peer", "servername"),
		Hostname:      param(params, "host"),
		Path:          param(params, "path"),
		Network:       normalizeNetwork(param(params, "type", "network")),
		Fingerprint:   param(params, "fp", "fingerprint"),
		PublicKey:     param(params, "pbk", "publickey", "public-key", "realitypbk"),
		ShortID:       param(params, "sid", "shortid", "short-id", "realitysid"),
		SpiderX:       param(params, "spx", "spiderx", "realityspx"),
		AllowInsecure: isTruthy(param(params, "allowinsecure", "insecure")),
		HeaderType:    strings.ToLower(param(params, "headertype", "header-type")),
		AddedAt:       time.Now().UnixMilli(),
	}
	if l.HeaderType == "none" {
		l.HeaderType = ""
	}

	// Certificate pinning (the supported replacement for the removed
	// allowInsecure). Some panels ship pcs=… in the share link.
	if fp, err := NormalizeCertFingerprint(param(params, "pcs", "pinnedpeercertsha256", "certfingerprint")); err != nil {
		return nil, fmt.Errorf("invalid pcs (certificate fingerprint): %w", err)
	} else {
		l.PinnedCertSHA256 = fp
	}

	// REALITY keys are url-safe base64 without padding, but panels have been
	// known to emit the padded standard alphabet. Normalise it here so the same
	// node always produces the same id (and Xray always accepts it).
	if l.PublicKey != "" {
		if k, err := normalizeRealityKey(l.PublicKey); err == nil {
			l.PublicKey = k
		}
	}

	// ALPN is comma separated ("h2,http/1.1"); a single value is fine too.
	for _, a := range strings.Split(param(params, "alpn"), ",") {
		if a = strings.TrimSpace(a); a != "" {
			l.ALPN = append(l.ALPN, a)
		}
	}

	// "mode" is overloaded: gRPC uses gun|multi, XHTTP uses auto|packet-override|...
	mode := strings.ToLower(param(params, "mode"))
	if l.Network == NetGRPC {
		l.GRPCMode = mode
	} else {
		l.Mode = mode
	}
	l.ServiceName = param(params, "servicename", "service-name", "grpcserviceName")

	// WebSocket early data ("ed") belongs into the path for Xray's ws settings.
	if ed := param(params, "ed"); ed != "" && l.Path != "" && !strings.Contains(l.Path, "?") {
		l.Path += "?ed=" + ed
	}

	// Preserve everything we did not explicitly consume (shown in the UI details).
	known := map[string]bool{}
	for _, k := range []string{
		"encryption", "flow", "security", "sni", "peer", "servername", "host", "path",
		"type", "network", "fp", "fingerprint", "pbk", "publickey", "public-key", "realitypbk",
		"sid", "shortid", "short-id", "realitysid", "spx", "spiderx", "realityspx",
		"alpn", "headertype", "header-type", "allowinsecure", "insecure", "servicename",
		"service-name", "grpcservicename", "mode", "ed", "seed", "mux", "host",
	} {
		known[k] = true
	}
	for k, v := range params {
		if !known[k] && v != "" {
			if l.Extra == nil {
				l.Extra = map[string]string{}
			}
			l.Extra[k] = v
		}
	}

	// ---- defaults mirroring V2RayN's import behaviour ---------------------------
	if l.Name == "" {
		l.Name = fmt.Sprintf("%s %s:%d", protocol, address, port)
	}
	if l.Protocol == ProtoVLESS && l.Encryption == "" {
		l.Encryption = "none"
	}
	if l.Network == "" {
		l.Network = NetTCP
	}
	if l.Security == "" {
		l.Security = SecNone
	}
	if l.SNI == "" && l.Security != SecNone {
		l.SNI = address // SNI defaults to the server address
	}
	if l.Hostname == "" && (l.Network == NetWS || l.Network == NetHTTPUpgrade) && l.Security != SecNone {
		l.Hostname = address // WS/HTTPUpgrade Host header defaults to the address
	}
	if l.Path == "" && l.Network == NetWS {
		l.Path = "/"
	}
	if l.ServiceName == "" && l.Network == NetGRPC {
		l.ServiceName = "grpc"
	}
	// Current Xray removed both "flow" and "encryption" for Trojan: drop them
	// (the node still works) but tell the user, instead of refusing the link.
	if l.Protocol == ProtoTrojan {
		if l.HasFlow() {
			l.Notes = append(l.Notes, "flow="+l.Flow+" ignored: Xray no longer supports flow for Trojan")
			l.Flow = ""
		}
		if l.Encryption != "" {
			l.Encryption = ""
		}
	}

	if err := l.Validate(); err != nil {
		return nil, err
	}
	l.ID = linkID(l)
	return l, nil
}

// linkID derives a stable id from the connection parameters only, so importing
// the same node twice (even with a different #name) is recognised as a duplicate.
func linkID(l *Link) string {
	key := strings.Join([]string{
		l.Protocol, l.UserID, l.Address, strconv.Itoa(l.Port), l.Network, l.Security,
		l.SNI, l.Hostname, l.Path, l.Flow, l.PublicKey, l.ShortID, l.SpiderX,
		l.HeaderType, l.ServiceName, l.GRPCMode, strings.Join(l.ALPN, ","),
		strconv.FormatBool(l.AllowInsecure), l.Fingerprint,
	}, "|")
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:12]
}

// ---- helpers ---------------------------------------------------------------

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func splitHostPort(hp string) (string, int, error) {
	hp = strings.TrimRight(strings.TrimSpace(hp), "/")
	if hp == "" {
		return "", 0, fmt.Errorf("missing server address")
	}
	var host, portStr string
	switch {
	case strings.HasPrefix(hp, "["): // IPv6 literal
		end := strings.IndexByte(hp, ']')
		if end < 0 {
			return "", 0, fmt.Errorf("unbalanced brackets in IPv6 address")
		}
		host = hp[1:end]
		rest := hp[end+1:]
		if !strings.HasPrefix(rest, ":") {
			return "", 0, fmt.Errorf("missing port after the IPv6 address")
		}
		portStr = rest[1:]
	default:
		i := strings.LastIndexByte(hp, ':')
		if i < 0 {
			return "", 0, fmt.Errorf("missing port (expected host:port)")
		}
		if strings.Contains(hp[i+1:], ":") || strings.Contains(hp, "@") {
			return "", 0, fmt.Errorf("malformed server address")
		}
		host, portStr = hp[:i], hp[i+1:]
	}
	host = strings.TrimSpace(unescape(host))
	if host == "" {
		return "", 0, fmt.Errorf("empty server address")
	}
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q", clip(portStr, 16))
	}
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("port %d is out of range", port)
	}
	return host, port, nil
}

// parseParams accepts "?a=b&c=d" as well as the legacy "?a=b;c=d".
func parseParams(q string) (map[string]string, error) {
	out := map[string]string{}
	q = strings.TrimSpace(q)
	if q == "" {
		return out, nil
	}
	sep := "&"
	if !strings.Contains(q, "&") && strings.Contains(q, ";") {
		sep = ";"
	}
	for _, kv := range strings.Split(q, sep) {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		k, v := kv, ""
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k, v = kv[:i], kv[i+1:]
		}
		// Deliberately percent-decode only: url.QueryUnescape would turn a raw
		// "+" into a space, which corrupts base64 values such as pbk=…/pcs=…
		// that panels paste in unencoded.
		key := strings.ToLower(strings.TrimSpace(queryUnescape(k)))
		val := strings.TrimSpace(queryUnescape(v))
		if key == "" {
			return nil, fmt.Errorf("malformed parameter %q", clip(kv, 40))
		}
		out[key] = val
	}
	return out, nil
}

// param returns the first non-empty value among the given (lowercase) keys.
func param(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[strings.ToLower(k)]; ok && v != "" {
			return v
		}
	}
	return ""
}

func unescape(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	if dec, err := url.PathUnescape(s); err == nil {
		return dec
	}
	return s
}

// queryUnescape percent-decodes a query key or value.
//
// It must NOT use url.QueryUnescape: that form-data decoder turns a literal "+"
// into a space, which silently corrupts the base64 in pbk= (REALITY public key)
// and pcs= (certificate fingerprint) — both are frequently pasted unencoded.
func queryUnescape(s string) string {
	if !strings.ContainsAny(s, "%+") {
		return s
	}
	if dec, err := url.PathUnescape(s); err == nil {
		return dec
	}
	return s
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func normalizeNetwork(n string) string {
	switch strings.ToLower(strings.TrimSpace(n)) {
	case "", "tcp", "raw":
		return NetTCP
	case "ws", "websocket":
		return NetWS
	case "grpc", "gRPC", "grpcs":
		return NetGRPC
	case "httpupgrade", "http-upgrade":
		return NetHTTPUpgrade
	case "xhttp", "splithttp":
		return NetXHTTP
	default:
		return strings.ToLower(strings.TrimSpace(n))
	}
}

func normalizeSecurity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none", "plain":
		return SecNone
	case "tls", "https":
		return SecTLS
	case "reality", "xtls-reality":
		return SecReality
	default:
		return strings.ToLower(strings.TrimSpace(s))
	}
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
