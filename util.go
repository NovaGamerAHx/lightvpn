package main

// util.go — small, pure helper functions used across the app.
// Kept separate so parser and xrayconfig do not bloat with repetitive logic.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"
)

// generateID returns a random hex string suitable for link and log IDs.
func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// clamp keeps an integer within [min, max].
func clamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// clip trims a string if it exceeds max runes, appending an ellipsis.
func clip(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

// formatBytes formats byte counts in human readable units (B, KB, MB, GB).
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// nowMillis returns the current unix timestamp in milliseconds.
func nowMillis() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
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
	case strings.Contains(low, "connection refused"), strings.Contains(low, "refused"):
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