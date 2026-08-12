//go:build integration

// Multi-listener & multi-connection lab tests against real RouterOS devices.
//
// Answers the two questions every user of Listen*() asks:
//
//  1. When one listener is cancelled (ctx timeout or manual Cancel()), do the
//     other listeners on the same connection keep running?  → Yes: each
//     listener gets its own tag and a per-listener /cancel, so the sibling
//     streams are untouched (verified by TestLabMultiListen*Isolation).
//  2. When one connection is closed, do other connections stay alive?
//     → Yes: connections are fully independent (verified by
//     TestLabCloseOneConnectionOthersAlive).
//
// All tests are read-only: they only monitor traffic on ether1, they never
// create/remove entries. Credentials come from env vars only:
//
//	ROUTEROS_TEST_ADDRESS / _USERNAME / _PASSWORD  (primary device)
//	ROUTEROS_TEST_ADDRESS_2 / _USERNAME_2 / _PASSWORD_2  (optional second device)
package routeros

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// labConfig2 is like labConfig in orchestrate_lab_test.go but returns the raw
// values so tests can open several connections.
func labConfig2(t *testing.T) (addr, user, pass string) {
	t.Helper()
	addr = os.Getenv("ROUTEROS_TEST_ADDRESS")
	user = os.Getenv("ROUTEROS_TEST_USERNAME")
	pass = os.Getenv("ROUTEROS_TEST_PASSWORD")
	if addr == "" || user == "" || pass == "" {
		t.Skip("ROUTEROS_TEST_ADDRESS/USERNAME/PASSWORD not set")
	}
	return addr, user, pass
}

// labSecondConfig returns the optional second device config, or "" when unset.
func labSecondConfig(t *testing.T) (addr, user, pass string) {
	t.Helper()
	addr = os.Getenv("ROUTEROS_TEST_ADDRESS_2")
	user = os.Getenv("ROUTEROS_TEST_USERNAME_2")
	pass = os.Getenv("ROUTEROS_TEST_PASSWORD_2")
	if addr == "" || user == "" || pass == "" {
		t.Log("ROUTEROS_TEST_ADDRESS_2/USERNAME_2/PASSWORD_2 not set — skipping cross-device part")
		return "", "", ""
	}
	return addr, user, pass
}

// labDial opens a client with a 10s dial timeout and wires cleanup.
func labDial(t *testing.T, addr, user, pass string) *Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := DialContext(ctx, addr, user, pass)
	if err != nil {
		t.Fatalf("DialContext(%s): %v", addr, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// labStream wraps a monitor-traffic listener with a drained receive channel
// so tests can observe sentence arrival without blocking the stream.
type labStream struct {
	l        *ListenReply
	received chan struct{} // buffered; every sentence observed as one token
	done     chan struct{} // closed when the listener channel is closed
}

// monitorTraffic starts a monitor-traffic listener on ether1 and returns a
// labStream draining its channel.
func monitorTraffic(t *testing.T, c *Client, ctx context.Context, name string) *labStream {
	t.Helper()
	l, err := c.ListenContext(ctx, "/interface/monitor-traffic", "=interface=ether1")
	if err != nil {
		t.Fatalf("%s: ListenContext(monitor-traffic): %v", name, err)
	}
	s := &labStream{
		l:        l,
		received: make(chan struct{}, 1000),
		done:     make(chan struct{}),
	}
	go func() {
		defer close(s.done)
		for range l.Chan() {
			select {
			case s.received <- struct{}{}:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		select {
		case <-s.done:
			// drain goroutine exited
		case <-time.After(5 * time.Second):
			t.Error("monitorTraffic drain goroutine did not exit within 5s")
		}
	})
	return s
}

// awaitSentence waits up to wait for at least n sentences to arrive.
func awaitSentence(t *testing.T, s *labStream, n int, wait time.Duration) bool {
	t.Helper()
	deadline := time.After(wait)
	count := 0
	for count < n {
		select {
		case <-s.received:
			count++
		case <-deadline:
			return false
		}
	}
	return true
}

// TestLabMultiListenCtxCancelIsolation — cancelling one listener through its
// context must not stop its siblings on the same connection.
func TestLabMultiListenCtxCancelIsolation(t *testing.T) {
	addr, user, pass := labConfig2(t)
	c := labDial(t, addr, user, pass)
	c.Queue = 100

	// Stream 1 is cancelled after 4s via its own context.
	ctx1, cancel1 := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel1()
	s1 := monitorTraffic(t, c, ctx1, "l1")

	// Stream 2 lives as long as the (unbounded) test context.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel2()
	s2 := monitorTraffic(t, c, ctx2, "l2")

	// Let both streams produce at least one sentence.
	if !awaitSentence(t, s1, 1, 8*time.Second) {
		t.Fatal("l1: no monitor-traffic sentence received; is ether1 up?")
	}
	if !awaitSentence(t, s2, 1, 8*time.Second) {
		t.Fatal("l2: no monitor-traffic sentence received; is ether1 up?")
	}

	// Wait for l1's context to expire → l1 is cancelled by the client.
	select {
	case <-ctx1.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("l1 ctx did not expire in time")
	}
	t.Log("l1 context expired, listener 1 cancelled")

	// After l1 dies, l2 must still receive fresh sentences.
	if !awaitSentence(t, s2, 2, 8*time.Second) {
		t.Fatal("l2 stopped after sibling l1 was cancelled: listeners are NOT isolated")
	}
	t.Log("OK: l2 kept receiving after l1 was cancelled")
}

// TestLabMultiListenManualCancelIsolation — same as above but cancelling
// through ListenReply.Cancel() (sends /cancel =tag=...) instead of ctx.
func TestLabMultiListenManualCancelIsolation(t *testing.T) {
	addr, user, pass := labConfig2(t)
	c := labDial(t, addr, user, pass)
	c.Queue = 100

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	s1 := monitorTraffic(t, c, ctx, "l1")
	s2 := monitorTraffic(t, c, ctx, "l2")
	s3 := monitorTraffic(t, c, ctx, "l3")

	for _, s := range []*labStream{s1, s2, s3} {
		if !awaitSentence(t, s, 1, 8*time.Second) {
			t.Fatal("a listener got no sentence; is ether1 up?")
		}
	}

	// Manually cancel l1. Wait for its channel to close (done channel from
	// monitorTraffic closes when the Chan() is drained and closed).
	if _, err := s1.l.Cancel(); err != nil {
		t.Fatalf("l1.Cancel(): %v", err)
	}
	select {
	case <-s1.done:
		t.Log("l1 closed after manual Cancel()")
	case <-time.After(8 * time.Second):
		t.Fatal("l1 did not close after Cancel()")
	}

	// l2 and l3 must still produce fresh sentences.
	for i, s := range []*labStream{s2, s3} {
		if !awaitSentence(t, s, 2, 8*time.Second) {
			t.Fatalf("listener #%d stopped after sibling l1 was cancelled", i+2)
		}
	}
	t.Log("OK: l2 & l3 kept receiving after manual Cancel() of l1")
}

// TestLabMultiConnectionConcurrentRun — many connections to the same device
// running commands concurrently all succeed (no cross-talk between clients).
func TestLabMultiConnectionConcurrentRun(t *testing.T) {
	addr, user, pass := labConfig2(t)

	const n = 5

	// Dial all clients on the test goroutine (t.Fatalf is only legal there),
	// then run commands concurrently.
	clients := make([]*Client, n)
	for i := range clients {
		clients[i] = labDial(t, addr, user, pass)
	}

	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i, c := range clients {
		wg.Add(1)
		go func(id int, c *Client) {
			defer wg.Done()
			defer c.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			r, err := c.RunContext(ctx, "/system/resource/print")
			if err != nil {
				errs <- fmt.Errorf("client %d: %w", id, err)
				return
			}
			if len(r.Re) != 1 {
				errs <- fmt.Errorf("client %d: got %d !re, want 1", id, len(r.Re))
			}
		}(i, c)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	t.Logf("OK: %d concurrent connections to %s all succeeded", n, addr)
}

// TestLabCloseOneConnectionOthersAlive — closing one connection must not kill
// the other connections.
func TestLabCloseOneConnectionOthersAlive(t *testing.T) {
	addr, user, pass := labConfig2(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c1 := labDial(t, addr, user, pass)
	c2 := labDial(t, addr, user, pass)
	c3 := labDial(t, addr, user, pass)

	// Prove all three work.
	for i, c := range []*Client{c1, c2, c3} {
		if _, err := c.RunContext(ctx, "/system/resource/print"); err != nil {
			t.Fatalf("client %d pre-check: %v", i+1, err)
		}
	}

	// Close c2.
	if err := c2.Close(); err != nil {
		t.Fatalf("c2.Close(): %v", err)
	}

	// c1 and c3 must still work.
	for i, c := range []*Client{c1, c3} {
		if _, err := c.RunContext(ctx, "/system/resource/print"); err != nil {
			t.Fatalf("client %d failed after sibling connection closed: %v", i+1, err)
		}
	}
	t.Log("OK: c1 & c3 kept working after c2 was closed")
}

// TestLabCrossDeviceConcurrent — if a second device is configured, run
// commands against both devices concurrently from the same process.
func TestLabCrossDeviceConcurrent(t *testing.T) {
	addr1, user1, pass1 := labConfig2(t)
	addr2, user2, pass2 := labSecondConfig(t)
	if addr2 == "" {
		t.Skip("second device not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c1 := labDial(t, addr1, user1, pass1)
	c2 := labDial(t, addr2, user2, pass2)

	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)

	for i, c := range []*Client{c1, c2} {
		go func(id int, c *Client) {
			defer wg.Done()
			if _, err := c.RunContext(ctx, "/system/resource/print"); err != nil {
				errs <- fmt.Errorf("device %d: %w", id, err)
			}
		}(i, c)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	t.Logf("OK: concurrent commands on %s and %s both succeeded", addr1, addr2)
}
