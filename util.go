package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

// marshalIndent produces stable, human readable JSON (the same bytes the UI
// previews and the core receives).
func marshalIndent(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func prettyJSON(raw []byte) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(b)
}

// xrayVersion reads the version of the vendored xray-core module straight out
// of the binary's build info, so the UI can show what core it embeds.
func xrayVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, d := range bi.Deps {
		if d.Path == "github.com/xtls/xray-core" {
			v := strings.TrimPrefix(d.Version, "v")
			if i := strings.Index(v, "-"); i > 0 {
				v = v[:i]
			}
			return v
		}
	}
	return "unknown"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func clampMS(d time.Duration) float64 {
	if d < 0 {
		return 0
	}
	return float64(d.Microseconds()) / 1000.0
}

// friendlyDialError turns Go's raw dial errors into something a user can act on.
func friendlyDialError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "no such host"):
		return "DNS lookup failed for the server address"
	case strings.Contains(low, "connection refused"):
		return "port closed — the server refused the connection"
	case strings.Contains(low, "i/o timeout"), strings.Contains(low, "timeout"):
		return "timed out (unreachable or blocked port)"
	case strings.Contains(low, "network is unreachable"):
		return "network unreachable"
	case strings.Contains(low, "no route to host"):
		return "no route to host"
	case strings.Contains(low, "only support tcp"):
		return "REALITY only supports type=tcp, grpc or xhttp"
	}
	return clip(msg, 160)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprint(err)
}
