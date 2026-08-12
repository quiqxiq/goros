//go:build integration

// Lab integration tests against a real RouterOS device. Run only when the
// `integration` build tag is set (per PLAN.md §15) and when
// ROUTEROS_TEST_ADDRESS / ROUTEROS_TEST_USERNAME / ROUTEROS_TEST_PASSWORD are
// set. Every scenario here is read-only; see docs/PLAN-FASE2-FASE3.md §5 for
// the full matrix (M1–M5 map to the tests below).
package nativeapi_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/quiqxiq/goros/v4"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
	"github.com/quiqxiq/goros/v4/transport/nativeapi"
)

func labConfig(t *testing.T) (addr, user, pass string) {
	t.Helper()
	addr = os.Getenv("ROUTEROS_TEST_ADDRESS")
	user = os.Getenv("ROUTEROS_TEST_USERNAME")
	pass = os.Getenv("ROUTEROS_TEST_PASSWORD")
	if addr == "" || user == "" || pass == "" {
		t.Skip("ROUTEROS_TEST_ADDRESS/USERNAME/PASSWORD not set")
	}
	return addr, user, pass
}

func labClient(t *testing.T) *routeros.Client {
	t.Helper()
	addr, user, pass := labConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := routeros.DialContext(ctx, addr, user, pass)
	if err != nil {
		t.Fatalf("DialContext(%s): %v", addr, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// M1 — Dial + login and legacy Run work against a real device.
func TestLabVersion(t *testing.T) {
	c := labClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r, err := c.RunContext(ctx, "/system/resource/print")
	if err != nil {
		t.Fatalf("Run /system/resource/print: %v", err)
	}
	if len(r.Re) == 0 {
		t.Fatal("no rows in reply")
	}
	v := r.Re[0].Map["version"]
	if v == "" {
		t.Fatal("version attribute empty")
	}
	t.Logf("LAB version=%s", v)
}

// M2 — nativeapi adapter Command returns the terminal !done.
func TestLabAdapterCommand(t *testing.T) {
	c := labClient(t)
	a := nativeapi.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rep, err := a.Command(ctx, &transport.Command{Path: "/system/identity", Verb: "print"})
	if err != nil {
		t.Fatalf("adapter Command: %v", err)
	}
	if rep.Type != transport.ReplyDone {
		t.Errorf("reply type = %q, want !done", rep.Type)
	}
}

// M3 — adapter List returns data rows followed by the terminal !done.
func TestLabAdapterList(t *testing.T) {
	c := labClient(t)
	a := nativeapi.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reps, err := a.List(ctx, &transport.Command{Path: "/system/resource", Verb: "print"})
	if err != nil {
		t.Fatalf("adapter List: %v", err)
	}
	if len(reps) < 2 {
		t.Fatalf("List returned %d replies, want rows + !done", len(reps))
	}
	last := reps[len(reps)-1]
	if last.Type != transport.ReplyDone {
		t.Errorf("last reply type = %q, want !done", last.Type)
	}
	t.Logf("LAB rows=%d version=%s", len(reps)-1, reps[0].Attributes["version"])
}

// M4 — unknown command traps: roserr.CodeCommandFailed with the legacy
// *DeviceError still reachable via errors.As (backward compat).
func TestLabTrapMapping(t *testing.T) {
	c := labClient(t)
	a := nativeapi.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := a.Command(ctx, &transport.Command{Path: "/nonsense", Verb: "command"})
	if err == nil {
		t.Fatal("want error for unknown command")
	}
	if !roserr.IsCode(err, roserr.CodeCommandFailed) {
		t.Errorf("err = %v, want CodeCommandFailed", err)
	}
	var devErr *routeros.DeviceError
	if !errors.As(err, &devErr) {
		t.Fatalf("errors.As(*DeviceError) failed for %v", err)
	}
	t.Logf("LAB trap message=%q", devErr.Sentence.Map["message"])
}

// M5 — /console/inspect probe. Characterization test: on RouterOS 6.x it traps
// cleanly (not supported), on 7.x it returns nodes. Both outcomes are valid;
// only unexpected error codes fail.
func TestLabInspectProbe(t *testing.T) {
	c := labClient(t)
	a := nativeapi.New(c)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nodes, err := a.Inspect(ctx, transport.InspectChild, "system")
	if err != nil {
		if !roserr.IsCode(err, roserr.CodeCommandFailed) {
			t.Errorf("inspect error %v: want CodeCommandFailed (unsupported) or success", err)
		}
		t.Logf("LAB inspect: unsupported (err=%v)", err)
		return
	}
	t.Logf("LAB inspect: supported, nodes=%d", len(nodes))
}
