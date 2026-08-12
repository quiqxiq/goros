//go:build integration

// Fase 6 orchestrator lab tests — the DoD of PLAN.md §10 against a real
// RouterOS device (env: ROUTEROS_TEST_ADDRESS/USERNAME/PASSWORD).
//
// The last two tests touch the device: Validate must never execute, while
// RunStructured really executes. The write test (add + verify + auto-cleanup)
// runs on whatever device ROUTEROS_TEST_ADDRESS points to — per the owner's
// decision, MT-2 (v7, 192.168.230.3) with auto-cleanup. The .id is looked up
// by a unique comment and removed in t.Cleanup, registered before the run so
// it fires even on failure.
package routeros

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
)

func labClient(t *testing.T) *Client {
	t.Helper()
	addr := os.Getenv("ROUTEROS_TEST_ADDRESS")
	user := os.Getenv("ROUTEROS_TEST_USERNAME")
	pass := os.Getenv("ROUTEROS_TEST_PASSWORD")
	if addr == "" || user == "" || pass == "" {
		t.Skip("ROUTEROS_TEST_ADDRESS/USERNAME/PASSWORD not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := DialContext(ctx, addr, user, pass)
	if err != nil {
		t.Fatalf("DialContext(%s): %v", addr, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func labCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestLabRunStructuredTableValid — DoD Fase 6: command tabel valid melalui
// run terstruktur berhasil.
func TestLabRunStructuredTableValid(t *testing.T) {
	c := labClient(t)
	ctx := labCtx(t)
	rep, err := c.RunStructured(ctx, &transport.Command{Path: "/system/resource", Verb: "print"})
	if err != nil {
		t.Fatalf("RunStructured(/system/resource/print): %v", err)
	}
	if rep.Type != transport.ReplyDone {
		t.Errorf("reply.Type = %q, want !done", rep.Type)
	}
	t.Logf("LAB valid table command -> %s", rep.Type)
}

// TestLabRunStructuredGate2RejectsBogus — atribut salah nama gagal Gate 2
// dengan daftar Missing/Available (hanya berlaku saat inspect didukung, v7).
func TestLabRunStructuredGate2RejectsBogus(t *testing.T) {
	c := labClient(t)
	ctx := labCtx(t)
	if err := c.ProbeInspect(ctx); err != nil {
		t.Fatalf("ProbeInspect: %v", err)
	}
	if !c.SupportsInspect() {
		t.Skip("SupportsInspect=false (v6): Gate 2 skipped by design")
	}
	_, err := c.RunStructured(ctx, &transport.Command{
		Path: "/ip/address", Verb: "print", Attributes: map[string]string{"bogus": "1"},
	})
	if err == nil {
		t.Fatal("RunStructured(bogus): want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationUnknownAttribute) {
		t.Fatalf("err = %v, want validation/unknown-attribute", err)
	}
	cctx, ok := roserr.ContextOf(err)
	if !ok {
		t.Fatal("ContextOf failed")
	}
	missing, _ := cctx.Extra["missing"].([]string)
	if len(missing) != 1 || missing[0] != "bogus" {
		t.Errorf("missing = %v, want [bogus]", cctx.Extra["missing"])
	}
	avail, _ := cctx.Extra["available"].([]string)
	if len(avail) == 0 {
		t.Error("available empty, want real field names")
	}
	t.Logf("LAB gate2 reject: missing=%v available=%d source=%v",
		missing, len(avail), cctx.Extra["validationSource"])
}

// TestLabRunStructuredGate1RejectsSyntax — command tidak dikenal via jalur
// script gagal Gate 1 di v7 (validation/syntax), atau trap device di v6 yang
// terdegradasi (command-failed).
func TestLabRunStructuredGate1RejectsSyntax(t *testing.T) {
	c := labClient(t)
	ctx := labCtx(t)
	_, err := c.RunStructured(ctx, &transport.Command{Script: "/nonsense command"})
	if err == nil {
		t.Fatal("RunStructured(nonsense): want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) && !roserr.IsCode(err, roserr.CodeCommandFailed) {
		t.Errorf("err = %v, want validation/syntax (v7) or command-failed (v6 degraded)", err)
	}
	t.Logf("LAB gate1 reject: %v", err)
}

// TestLabValidateDryRunDoesNotExecute — action command via dry-run: lolos
// gate tapi TIDAK tereksekusi (tidak ada entry baru di device).
func TestLabValidateDryRunDoesNotExecute(t *testing.T) {
	c := labClient(t)
	ctx := labCtx(t)
	comment := fmt.Sprintf("goros-f6-dry-%d", time.Now().UnixNano())
	cmd := &transport.Command{
		Path: "/ip/address", Verb: "add",
		Attributes: map[string]string{
			"address": "10.99.99.98/32", "interface": "ether1", "comment": comment,
		},
	}
	if err := c.Validate(ctx, cmd); err != nil {
		t.Fatalf("Validate(add): %v", err)
	}
	if id := labFindAddressByComment(ctx, c, comment); id != "" {
		t.Fatalf("dry-run created entry id=%s; dry-run must never execute", id)
	}
	t.Log("LAB dry-run: gates passed, no entry created")
}

// TestLabRunStructuredExecutesAndCleansUp — DoD Fase 6: run benar-benar
// mengeksekusi (entry dibuat), lalu auto-cleanup menghapusnya. Menulis ke
// device yang ditunjuk ROUTEROS_TEST_ADDRESS (MT-2 v7, persetujuan pemilik).
func TestLabRunStructuredExecutesAndCleansUp(t *testing.T) {
	c := labClient(t)
	ctx := labCtx(t)
	comment := fmt.Sprintf("goros-f6-write-%d", time.Now().UnixNano())
	cmd := &transport.Command{
		Path: "/ip/address", Verb: "add",
		Attributes: map[string]string{
			"address": "10.99.99.99/32", "interface": "ether1", "comment": comment,
		},
	}
	// Auto-cleanup registered BEFORE the run so it fires even on failure.
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		id := labFindAddressByComment(cctx, c, comment)
		if id == "" {
			return // nothing to clean
		}
		if _, err := c.Command(cctx, &transport.Command{
			Path: "/ip/address", Verb: "remove", Attributes: map[string]string{".id": id},
		}); err != nil {
			t.Logf("cleanup remove %s: %v", id, err)
			return
		}
		if still := labFindAddressByComment(cctx, c, comment); still != "" {
			t.Errorf("cleanup failed: entry %s still present", still)
		} else {
			t.Logf("LAB cleanup ok: removed %s", id)
		}
	})

	rep, err := c.RunStructured(ctx, cmd)
	if err != nil {
		t.Fatalf("RunStructured(add): %v", err)
	}
	if rep.Type != transport.ReplyDone {
		t.Errorf("reply.Type = %q, want !done", rep.Type)
	}
	if labFindAddressByComment(ctx, c, comment) == "" {
		t.Fatal("entry not found after run; command was not executed")
	}
	t.Log("LAB run: entry created and verified")
}

// labFindAddressByComment returns the .id of the /ip/address entry whose
// comment matches, or "" when absent. Uses the raw List seam (no gates) so
// cleanup never trips on validation.
func labFindAddressByComment(ctx context.Context, c *Client, comment string) string {
	reps, err := c.List(ctx, &transport.Command{
		Path: "/ip/address", Verb: "print", Queries: []string{"comment=" + comment},
	})
	if err != nil {
		return ""
	}
	for _, rep := range reps {
		if rep.Type == transport.ReplyRe && rep.Attributes["comment"] == comment {
			return rep.Attributes[".id"]
		}
	}
	return ""
}
