package main

// ping.go — TCP connect latency, run concurrently for every node.
//
// Xray's own observatory needs a full tunnel handshake; for "is this server
// alive and how far away is it" a plain TCP connect is faster, needs no
// credentials and works even when the node is broken. We take the best of a few
// samples so one retransmission does not make a good node look bad.

import (
	"context"
	"net"
	"sync"
	"time"
)

// PingResult is one measurement for one node.
type PingResult struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Endpoint string  `json:"endpoint"`
	OK       bool    `json:"ok"`
	MS       float64 `json:"ms"`
	JitterMS float64 `json:"jitterMs,omitempty"`
	Samples  int     `json:"samples,omitempty"`
	Error    string  `json:"error,omitempty"`
	At       int64   `json:"at"`
}

// PingOptions tune the measurement.
type PingOptions struct {
	Timeout     time.Duration
	Samples     int
	MaxParallel int
}

func (p PingOptions) normalise() PingOptions {
	if p.Timeout <= 0 {
		p.Timeout = 3 * time.Second
	}
	if p.Samples <= 0 {
		p.Samples = 3
	}
	if p.Samples > 10 {
		p.Samples = 10
	}
	if p.MaxParallel <= 0 {
		p.MaxParallel = 32
	}
	return p
}

// PingEndpoint measures the TCP connect round trip to addr.
func PingEndpoint(ctx context.Context, addr string, timeout time.Duration, samples int) (best time.Duration, jitter time.Duration, err error) {
	opts := PingOptions{Timeout: timeout, Samples: samples}.normalise()
	d := net.Dialer{Timeout: opts.Timeout, KeepAlive: 0}

	var times []time.Duration
	var lastErr error
	for i := 0; i < opts.Samples; i++ {
		if ctx.Err() != nil {
			return 0, 0, ctx.Err()
		}
		start := time.Now()
		conn, derr := d.DialContext(ctx, "tcp", addr)
		elapsed := time.Since(start)
		if derr != nil {
			lastErr = derr
			break // a dead server does not need three more attempts
		}
		_ = conn.Close()
		if elapsed <= 0 {
			elapsed = time.Microsecond
		}
		times = append(times, elapsed)
		if best == 0 || elapsed < best {
			best = elapsed
		}
		if i+1 < opts.Samples {
			select {
			case <-ctx.Done():
				return best, 0, ctx.Err()
			case <-time.After(60 * time.Millisecond):
			}
		}
	}
	if len(times) == 0 {
		return 0, 0, lastErr
	}
	var worst time.Duration
	for _, t := range times {
		if t > worst {
			worst = t
		}
	}
	return best, worst - best, nil
}

// PingAll pings every node concurrently and streams results through onResult as
// soon as each node finishes, so the list fills in progressively instead of
// freezing until the slowest server times out.
func PingAll(ctx context.Context, links []*Link, o PingOptions, onResult func(PingResult)) []PingResult {
	o = o.normalise()
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		all []PingResult
	)
	sem := make(chan struct{}, o.MaxParallel)

	for _, l := range links {
		if l == nil {
			continue
		}
		wg.Add(1)
		go func(l *Link) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			res := PingResult{ID: l.ID, Name: l.Name, Endpoint: l.Endpoint(), At: time.Now().UnixMilli(), Samples: o.Samples}
			best, jitter, err := PingEndpoint(ctx, res.Endpoint, o.Timeout, o.Samples)
			switch {
			case err != nil && ctx.Err() != nil:
				return // cancelled: report nothing
			case err != nil:
				res.Error = friendlyDialError(err)
			default:
				res.OK = true
				res.MS = roundMS(best)
				res.JitterMS = roundMS(jitter)
			}
			mu.Lock()
			all = append(all, res)
			mu.Unlock()
			if onResult != nil {
				onResult(res)
			}
		}(l)
	}
	wg.Wait()
	return all
}

func roundMS(d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64((d+50*time.Microsecond)/(100*time.Microsecond)) / 10.0 // 0.1 ms resolution
}