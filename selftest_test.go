package main

import (
	"testing"
)

// TestSelfTestEndToEnd is the real "does this client actually tunnel traffic"
// gate: it starts a throwaway Xray server in-process, points the generated
// client config at it over VLESS/Trojan + TLS (TCP and WebSocket), fetches an
// object through the local HTTP and SOCKS5 inbounds, and finally checks that a
// node with wrong credentials is rejected.
func TestSelfTestEndToEnd(t *testing.T) {
	logf := func(level, msg string) {
		t.Logf("core[%s] %s", level, msg)
	}

	rep := RunSelfTest(logf)
	for _, r := range rep.Results {
		if !r.OK {
			t.Errorf("scenario %q failed: %s (%.0f ms)", r.Name, r.Detail, r.MS)
			continue
		}
		t.Logf("scenario %q ok: %s (%.0f ms)", r.Name, r.Detail, r.MS)
	}
	if !rep.OK {
		t.Fatalf("self test failed: %s", rep.Summary)
	}
	if len(rep.Results) < len(selfTestScenarios) {
		t.Fatalf("only %d scenarios ran", len(rep.Results))
	}
}
