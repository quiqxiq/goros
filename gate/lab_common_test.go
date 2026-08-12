//go:build integration

package gate_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/quiqxiq/goros/v4"
	"github.com/quiqxiq/goros/v4/transport/nativeapi"
)

// Shared lab helpers for the gate integration tests (M8–M10, M14–M21 in
// docs/PLAN-FASE3-FASE4.md §5). Everything here is read-only; credentials
// come from env vars only.

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

// labAdapter dials the device, logs in, and returns a nativeapi adapter with
// cleanup wired to close the connection.
func labAdapter(t *testing.T) *nativeapi.Adapter {
	t.Helper()
	addr, user, pass := labConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := routeros.DialContext(ctx, addr, user, pass)
	if err != nil {
		t.Fatalf("DialContext(%s): %v", addr, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return nativeapi.New(c)
}

// labCtx returns a per-test timeout context, cancelled on cleanup.
func labCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}
