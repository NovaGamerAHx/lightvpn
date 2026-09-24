package main

import (
	"context"
	"net"
	"strconv"
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
	if best < 0 {
		t.Errorf("rtt must not be negative, got %v", best)
	}
	if jitter < 0 {
		t.Errorf("jitter must not be negative, got %v", jitter)
	}
	if ms := roundMS(best); ms != float64((best+50*time.Microsecond)/(100*time.Microsecond))/10.0 {
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
	if time.Since(start) > 2*time.Second {
		t.Errorf("dead port took %v to fail, must be fast", time.Since(start))
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
	_ = PingAll(ctx, links, PingOptions{Timeout: 10 * time.Second, Samples: 3, MaxParallel: 2}, nil)
	if time.Since(start) > 2*time.Second {
		t.Errorf("PingAll did not honour cancel (ran for %v)", time.Since(start))
	}
}

func TestFriendlyDialErrorMessages(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"dial tcp: lookup nope.example: no such host", "DNS lookup failed for the server address"},
		{"connectex: No connection could be made because the target machine actively refused it.", "port closed — the server refused the connection"},
		{"dial tcp 1.2.3.4:443: i/o timeout", "timed out (unreachable or blocked port)"},
		{"dial tcp: network is unreachable", "network unreachable"},
		{"dial tcp: no route to host", "no route to host"},
	}
	for _, c := range cases {
		got := friendlyDialError(&net.OpError{Err: &customError{c.in}})
		if got != c.want {
			t.Errorf("in=%q\n got=%q\nwant=%q", c.in, got, c.want)
		}
	}
}

type customError struct{ s string }

func (c *customError) Error() string { return c.s }