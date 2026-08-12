//go:build integration

// Gate 2 (schema validation) + CommandSchema discovery lab tests — matrix
// M14–M21 in docs/PLAN-FASE3-FASE4.md §5. Inspect-based tests run on
// RouterOS 7.x only; on 6.x SupportsInspect=false and Gate 2 skips by design.
package gate_test

import (
	"testing"

	"github.com/quiqxiq/goros/v4/gate"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/schema"
	"github.com/quiqxiq/goros/v4/transport"
)

// M14/M15 — /console/inspect probes on a real path: request=child on
// /ip/address, and the field-discovery trick request=completion on the
// .proplist argument. Skipped on devices without inspect (v6).
func TestLabInspectProbes(t *testing.T) {
	a := labAdapter(t)
	ctx := labCtx(t)
	if err := a.ProbeInspect(ctx); err != nil {
		t.Fatalf("ProbeInspect: %v", err)
	}
	if !a.SupportsInspect() {
		t.Skip("SupportsInspect=false (v6): inspect not served")
	}

	children, err := a.Inspect(ctx, transport.InspectChild, "ip,address")
	if err != nil {
		t.Fatalf("Inspect(child ip,address): %v", err)
	}
	t.Logf("LAB child nodes=%d", len(children))
	if len(children) == 0 {
		t.Error("no child nodes under /ip/address")
	}

	fields, err := a.Inspect(ctx, transport.InspectCompletion, "ip,address,print,proplist")
	if err != nil {
		t.Fatalf("Inspect(completion ip,address,print,proplist): %v", err)
	}
	names := schema.ExtractCompletionNames(fields)
	t.Logf("LAB completion names (%d): %v", len(names), names)
	if len(names) == 0 {
		t.Error("no field names from .proplist completion")
	}
}

// M16 — Discover /ip/address/print: category table with field attributes.
func TestLabDiscoverTable(t *testing.T) {
	a := labAdapter(t)
	ctx := labCtx(t)
	if err := a.ProbeInspect(ctx); err != nil {
		t.Fatalf("ProbeInspect: %v", err)
	}
	if !a.SupportsInspect() {
		t.Skip("SupportsInspect=false (v6): inspect not served")
	}
	s := schema.NewStore(a)
	sch, err := s.Discover(ctx, "/ip/address", "print")
	if err != nil {
		t.Fatalf("Discover(/ip/address, print): %v", err)
	}
	if sch.Category != schema.CategoryTable {
		t.Errorf("Category = %q, want table", sch.Category)
	}
	if len(sch.Attributes) == 0 {
		t.Error("no attributes discovered for /ip/address/print")
	}
	t.Logf("LAB /ip/address/print: category=%s attrs=%v source=%s",
		sch.Category, attributeNames(sch), sch.Source)
	for _, want := range []string{"address", "interface", "comment"} {
		if !sch.HasAttribute(want) {
			t.Errorf("schema missing field %q", want)
		}
	}
}

// M17 — Discover /tool/ping: category action with input arguments.
func TestLabDiscoverAction(t *testing.T) {
	a := labAdapter(t)
	ctx := labCtx(t)
	if err := a.ProbeInspect(ctx); err != nil {
		t.Fatalf("ProbeInspect: %v", err)
	}
	if !a.SupportsInspect() {
		t.Skip("SupportsInspect=false (v6): inspect not served")
	}
	s := schema.NewStore(a)
	sch, err := s.Discover(ctx, "/tool", "ping")
	if err != nil {
		t.Fatalf("Discover(/tool, ping): %v", err)
	}
	if sch.Category != schema.CategoryAction {
		t.Errorf("Category = %q, want action", sch.Category)
	}
	for _, want := range []string{"address", "count"} {
		if !sch.HasAttribute(want) {
			t.Errorf("action schema missing argument %q", want)
		}
	}
	t.Logf("LAB /tool/ping: category=%s attrs=%v", sch.Category, attributeNames(sch))
}

// M18/M19/M21 — Gate 2: valid attributes pass; a bogus attribute is rejected
// with Missing/Available; on v6 (SupportsInspect=false) the gate skips.
func TestLabGate2Validation(t *testing.T) {
	a := labAdapter(t)
	ctx := labCtx(t)
	if err := a.ProbeInspect(ctx); err != nil {
		t.Fatalf("ProbeInspect: %v", err)
	}
	t.Logf("LAB SupportsInspect=%v", a.SupportsInspect())
	s := schema.NewStore(a)
	g2 := &gate.Gate2{Schema: s, SupportsInspect: a.SupportsInspect}

	// M18 — known attributes pass (and on v6 the gate skips, also nil).
	if err := g2.Validate(ctx, &transport.Command{
		Path: "/ip/address", Verb: "print", Attributes: map[string]string{"detail": ""},
	}); err != nil {
		t.Fatalf("Gate2.Validate(known): %v", err)
	}

	if !a.SupportsInspect() {
		t.Skip("SupportsInspect=false (v6): M19/M20 not applicable")
	}

	// M19 — unknown attribute -> Missing/Available in context.
	err := g2.Validate(ctx, &transport.Command{
		Path: "/ip/address", Verb: "print", Attributes: map[string]string{"interface1": "ether1"},
	})
	if err == nil {
		t.Fatal("Gate2.Validate(bogus): want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationUnknownAttribute) {
		t.Errorf("err = %v, want CodeValidationUnknownAttribute", err)
	}
	cctx, _ := roserr.ContextOf(err)
	missing, _ := cctx.Extra["missing"].([]string)
	if len(missing) != 1 || missing[0] != "interface1" {
		t.Errorf("Extra[missing] = %v, want [interface1]", cctx.Extra["missing"])
	}
	avail, ok := cctx.Extra["available"].([]string)
	if !ok || len(avail) == 0 {
		t.Errorf("Extra[available] = %v, want non-empty field list", cctx.Extra["available"])
	}
	t.Logf("LAB gate2 rejection: missing=%v available_count=%d source=%v",
		missing, len(avail), cctx.Extra["validationSource"])
}

func attributeNames(sch *schema.CommandSchema) []string {
	names := make([]string, len(sch.Attributes))
	for i, a := range sch.Attributes {
		names[i] = a.Name
	}
	return names
}
