package main

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// echoListener accepts and immediately closes connections, which is exactly what
// a TCP connect ping measures.
func echoListener(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
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
			_ = c.Close()
		}
	}()
	return ln.Addr().String(), func() { close(done); _ = ln.Close() }
}

func TestPingEndpointMeasuresRTT(t *testing.T) {
	addr, stop := echoListener(t)
	defer stop()

	best, jitter, err := PingEndpoint(context.Background(), addr, 2*time.Second, 3)
	if err != nil {
		t.Fatalf("ping on a live listener failed: %v", err)
	}
	if best <= 0 {
		t.Errorf("rtt must be > 0, got %v", best)
	}
	if jitter < 0 {
		t.Errorf("jitter must not be negative, got %v", jitter)
	}
	if ms := roundMS(best); ms != float64(int64(best/100)/10) {
		t.Errorf("roundMS mismatch: %v", ms)
	}
}

func TestPingEndpointReportsDeadPort(t *testing.T) {
	// Bind then close to obtain a port nothing listens on.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	start := time.Now()
	_, _, err = PingEndpoint(context.Background(), addr, 2*time.Second, 3)
	if err == nil {
		t.Fatal("pinging a closed port must fail")
	}
	if strings.Contains(friendlyDialError(err), "timed out") && time.Since(start) > 1500*time.Millisecond {
		t.Errorf("a refused connection must be reported immediately, took %v", time.Since(start))
	}
	if msg := friendlyDialError(err); !strings.Contains(msg, "refused") {
		t.Errorf("friendly error should mention the refusal, got %q", msg)
	}
}

func TestPingEndpointHonoursCancellation(t *testing.T) {
	// 10.255.255.1 is documentation space: it black-holes packets, so only a
	// cancelled context can end this quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := PingEndpoint(ctx, "10.255.255.1:443", 10*time.Second, 3)
	if err == nil {
		t.Skip("the network answered the black-hole address; nothing to check here")
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("cancellation was ignored (waited %v)", time.Since(start))
	}
}

func TestPingAllStreamsAndStaysConcurrent(t *testing.T) {
	const n = 8
	var addrs []string
	stops := []func(){}
	for i := 0; i < n; i++ {
		a, stop := echoListener(t)
		addrs = append(addrs, a)
		stops = append(stops, stop)
	}
	defer func() {
		for _, s := range stops {
			s()
		}
	}()

	links := make([]*Link, 0, n)
	for i, a := range addrs {
		host, portStr, err := net.SplitHostPort(a)
		if err != nil {
			t.Fatal(err)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			t.Fatal(err)
		}
		links = append(links, &Link{
			ID: string(rune('a' + i)), Name: "node-" + string(rune('a'+i)),
			Protocol: "vless", Address: host, Port: port, Network: "tcp", Security: "none", UserID: testUUID,
		})
	}

	var streamed atomic.Int32
	results := PingAll(context.Background(), links, PingOptions{Timeout: 2 * time.Second, Samples: 1, MaxParallel: 4},
		func(PingResult) { streamed.Add(1) })

	if len(results) != n {
		t.Fatalf("got %d results, want %d", len(results), n)
	}
	if int(streamed.Load()) != n {
		t.Errorf("every result must also be streamed live: %d of %d", streamed.Load(), n)
	}
	for _, r := range results {
		if !r.OK {
			t.Errorf("node %s failed: %s", r.Name, r.Error)
		}
	}
}

func TestPingAllCancelledStopsEarly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	links := []*Link{
		{ID: "1", Name: "blackhole", Protocol: "vless", Address: "10.255.255.1", Port: 443},
		{ID: "2", Name: "blackhole2", Protocol: "trojan", Address: "10.255.255.2", Port: 443},
	}
	go func() { time.Sleep(120 * time.Millisecond); cancel() }()
	start := time.Now()
	_ = PingAll(ctx, links, PingOptions{Timeout: 15 * time.Second, Samples: 3}, nil)
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("PingAll ignored the cancellation (took %v)", d)
	}
}

func TestFriendlyDialErrorMessages(t *testing.T) {
	cases := map[string]string{
		"dial tcp: lookup nope: no such host":               "DNS lookup failed",
		"dial tcp 1.2.3.4:443: connect: connection refused": "refused",
		"dial tcp: i/o timeout":                             "timed out",
		"network is unreachable":                            "network unreachable",
	}
	for in, want := range cases {
		got := friendlyDialError(errString(in))
		if !strings.Contains(got, want) {
			t.Errorf("friendlyDialError(%q) = %q, want it to contain %q", in, got, want)
		}
	}
	if friendlyDialError(nil) != "" {
		t.Error("nil error must map to the empty string")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
