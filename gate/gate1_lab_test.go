//go:build integration

// Gate 1 (:parse syntax validation) lab tests — matrix M8–M10 in
// docs/PLAN-FASE3-FASE4.md §5. On RouterOS 7.x the :parse script path works
// (as-string); on 6.x SupportsParse=false and the gate skips by design.
package gate_test

import (
	"testing"

	"github.com/quiqxiq/goros/v4/gate"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
)

// M8 — a valid command parses cleanly. On v6 the gate must skip silently
// (SupportsParse=false), which also passes.
func TestLabGate1Valid(t *testing.T) {
	a := labAdapter(t)
	ctx := labCtx(t)
	if err := a.ProbeParse(ctx); err != nil {
		t.Fatalf("ProbeParse: %v", err)
	}
	t.Logf("LAB SupportsParse=%v", a.SupportsParse())
	g := &gate.Gate1{Transport: a, SupportsParse: a.SupportsParse}
	if err := g.Run(ctx, &transport.Command{Path: "/system/resource", Verb: "print"}); err != nil {
		t.Fatalf("Gate1.Run(valid): %v", err)
	}
}

// M9 — unknown attribute. Per R10 the 7.21.5 message
// ("expected end of command (line X column Y)") is indistinguishable from
// broken syntax, so the coarse validation/syntax result is expected; some
// builds still emit "bad parameter <name>" (validation/unknown-attribute).
// Both are accepted as the characterization outcome.
func TestLabGate1UnknownAttribute(t *testing.T) {
	a := labAdapter(t)
	ctx := labCtx(t)
	if err := a.ProbeParse(ctx); err != nil {
		t.Fatalf("ProbeParse: %v", err)
	}
	if !a.SupportsParse() {
		t.Skip("SupportsParse=false (v6): gate skipped by design")
	}
	g := &gate.Gate1{Transport: a, SupportsParse: a.SupportsParse}
	err := g.Run(ctx, &transport.Command{
		Path: "/ip/address", Verb: "print", Attributes: map[string]string{"bogus": "1"},
	})
	if err == nil {
		t.Fatal("Gate1.Run(bogus attribute): want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) &&
		!roserr.IsCode(err, roserr.CodeValidationUnknownAttribute) {
		t.Errorf("err = %v, want validation/syntax (coarse, R10) or validation/unknown-attribute", err)
	}
	t.Logf("LAB unknown-attribute classified as: %v", err)
}

// M10 — an unknown command is always syntax-invalid, with a position.
func TestLabGate1UnknownCommand(t *testing.T) {
	a := labAdapter(t)
	ctx := labCtx(t)
	if err := a.ProbeParse(ctx); err != nil {
		t.Fatalf("ProbeParse: %v", err)
	}
	if !a.SupportsParse() {
		t.Skip("SupportsParse=false (v6): gate skipped by design")
	}
	g := &gate.Gate1{Transport: a, SupportsParse: a.SupportsParse}
	err := g.Run(ctx, &transport.Command{Path: "/nonsense", Verb: "command"})
	if err == nil {
		t.Fatal("Gate1.Run(unknown command): want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) {
		t.Errorf("err = %v, want CodeValidationSyntax", err)
	}
	cctx, _ := roserr.ContextOf(err)
	if cctx.Extra["line"] == nil || cctx.Extra["col"] == nil {
		t.Errorf("context.Extra = %v, want line/col position", cctx.Extra)
	}
	t.Logf("LAB unknown-command: %v (line=%v col=%v)", err, cctx.Extra["line"], cctx.Extra["col"])
}
