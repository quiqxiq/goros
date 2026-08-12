//go:build integration

// Probe lab test — "inspect, validate, dan lainnya" on a real RouterOS
// device (env: ROUTEROS_TEST_ADDRESS/USERNAME/PASSWORD). Everything here is
// read-only: the validate battery is a dry-run (never executes), and the
// only executed commands are print-style. It exercises the full facade
// pipeline: session probes (ProbeInspect/ProbeParse), schema discovery
// (Inspect), Gate 1 (:parse syntax) and Gate 2 (attribute schema), and
// RunStructured execution. On v6 both gates degrade silently by design
// (D-009/D-010), which the report makes visible.
package routeros

import (
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/schema"
	"github.com/quiqxiq/goros/v4/transport"
)

// probeReport prints a section header so the -v output reads as a report.
// Prefixed with "probe" to avoid colliding with other integration files in
// this package (the lab helpers use a lab* prefix for the same reason).
func probeReport(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Logf("== "+format+" ==", args...)
}

// TestLabProbeCapabilities — probe the session seams and report what the
// device can serve (inspect & :parse). These are the flags that decide
// which gates apply.
func TestLabProbeCapabilities(t *testing.T) {
	c := labClient(t)
	ctx := labCtx(t)

	if err := c.ProbeInspect(ctx); err != nil {
		t.Fatalf("ProbeInspect: %v", err)
	}
	if err := c.ProbeParse(ctx); err != nil {
		t.Fatalf("ProbeParse: %v", err)
	}
	probeReport(t, "capabilities")
	t.Logf("LAB SupportsInspect=%v SupportsParse=%v", c.SupportsInspect(), c.SupportsParse())
}

// TestLabProbeInspect — /console/inspect discovery on a table command, an
// action command, and an unknown command. Shows what the schema layer learns
// about each (category, attributes/arguments).
func TestLabProbeInspect(t *testing.T) {
	c := labClient(t)
	ctx := labCtx(t)

	if err := c.ProbeInspect(ctx); err != nil {
		t.Fatalf("ProbeInspect: %v", err)
	}
	probeReport(t, "inspect / discover")

	// Table command.
	sch, err := c.Inspect(ctx, "/ip/address", "print")
	if err != nil {
		t.Fatalf("Inspect(/ip/address, print): %v", err)
	}
	t.Logf("LAB /ip/address/print: category=%s attributes=%d source=%s",
		sch.Category, len(sch.Attributes), sch.Source)
	if sch.Category == schema.CategoryTable {
		names := probeAttrNames(sch)
		sort.Strings(names)
		t.Logf("LAB   fields (%d): %v", len(names), names)
	}

	// Action command.
	sch, err = c.Inspect(ctx, "/tool", "ping")
	if err != nil {
		t.Logf("LAB /tool/ping: Inspect error (noted): %v", err)
	} else {
		t.Logf("LAB /tool/ping: category=%s arguments=%d",
			sch.Category, len(sch.Attributes))
		if sch.Category == schema.CategoryAction {
			names := probeAttrNames(sch)
			sort.Strings(names)
			t.Logf("LAB   args: %v", names)
		}
	}

	// Unknown command.
	sch, err = c.Inspect(ctx, "/nonsense", "command")
	if err != nil {
		t.Logf("LAB /nonsense/command: Inspect error: %v", err)
	} else {
		t.Logf("LAB /nonsense/command: category=%s (unknown = skip, not fail)", sch.Category)
	}
}

// TestLabProbeValidateBattery — dry-run validation of a battery of commands.
// Validate must NEVER execute anything. Outcomes:
//
//	ok   — gates passed (or skipped by design, e.g. v6)
//	err  — rejected with a roserr validation code
func TestLabProbeValidateBattery(t *testing.T) {
	c := labClient(t)
	ctx := labCtx(t)

	cases := []struct {
		name string
		cmd  *transport.Command
	}{
		{"identity-print", &transport.Command{Path: "/system/identity", Verb: "print"}},
		{"resource-print", &transport.Command{Path: "/system/resource", Verb: "print"}},
		{"address-print-detail", &transport.Command{
			Path: "/ip/address", Verb: "print", Attributes: map[string]string{"detail": ""},
		}},
		{"ping-action-args", &transport.Command{
			Path: "/tool", Verb: "ping",
			Attributes: map[string]string{"address": "127.0.0.1", "count": "1"},
		}},
		{"address-print-bogus-attr", &transport.Command{
			Path: "/ip/address", Verb: "print", Attributes: map[string]string{"interface1": "ether1"},
		}},
		{"unknown-structured-cmd", &transport.Command{Path: "/nonsense", Verb: "command"}},
		{"script-unbalanced-quotes", &transport.Command{Script: `:put "unclosed`}},
	}

	probeReport(t, "validate battery (dry-run)")
	for _, tc := range cases {
		err := c.Validate(ctx, tc.cmd)
		if err == nil {
			t.Logf("LAB %-26s -> OK (lolos / skip by design)", tc.name)
			continue
		}
		code := probeErrCode(err)
		msg := err.Error()
		if cctx, ok := roserr.ContextOf(err); ok {
			if m, ok := cctx.Extra["missing"].([]string); ok {
				msg = fmt.Sprintf("missing=%v", m)
			}
		}
		t.Logf("LAB %-26s -> %s (%s)", tc.name, code, msg)
	}
}

// TestLabProbeGate1Scripts — Gate 1 syntax validation over the script path
// (only applies when SupportsParse; v6 skips silently). Valid scripts pass;
// an unknown command and a bogus attribute on a script fail with a position.
func TestLabProbeGate1Scripts(t *testing.T) {
	c := labClient(t)
	ctx := labCtx(t)

	probeReport(t, "gate1 (:parse) script battery")

	if err := c.ProbeParse(ctx); err != nil {
		t.Fatalf("ProbeParse: %v", err)
	}
	if !c.SupportsParse() {
		t.Log("LAB gate1 script path: SupportsParse=false (v6) — skipped by design")
		return
	}

	cases := []struct {
		name string
		cmd  *transport.Command
	}{
		{"script-valid-put", &transport.Command{Script: `:put (1+1)`}},
		{"script-valid-if", &transport.Command{Script: `:if (1=1) do={:put "yes"}`}},
		{"script-unknown-cmd", &transport.Command{Script: `/nonsense command`}},
		{"script-bogus-attr", &transport.Command{Script: `/ip address print interface1=ether1`}},
	}

	for _, tc := range cases {
		err := c.Validate(ctx, tc.cmd)
		if err == nil {
			t.Logf("LAB %-22s -> OK (device parser menerima)", tc.name)
			continue
		}
		code := probeErrCode(err)
		pos := ""
		if cctx, ok := roserr.ContextOf(err); ok {
			if l, cok := cctx.Extra["line"].(int); cok {
				col, _ := cctx.Extra["col"].(int)
				pos = fmt.Sprintf(" @line=%d col=%d", l, col)
			}
		}
		t.Logf("LAB %-22s -> %s%s", tc.name, code, pos)
	}
}

// TestLabProbeRunStructuredReadOnly — RunStructured really executes
// (read-only print commands only) and returns the canonical reply.
func TestLabProbeRunStructuredReadOnly(t *testing.T) {
	c := labClient(t)
	ctx := labCtx(t)

	probeReport(t, "run-structured (read-only)")

	// RunStructured really executes the validated command and returns the
	// terminal !done (the !re data rows ride the List seam below — Command
	// returns only the terminal sentence by contract).
	rep, err := c.RunStructured(ctx, &transport.Command{Path: "/system/identity", Verb: "print"})
	if err != nil {
		t.Fatalf("RunStructured(identity): %v", err)
	}
	if rep.Type != transport.ReplyDone {
		t.Errorf("identity reply.Type = %q, want !done", rep.Type)
	}
	t.Logf("LAB identity (RunStructured): type=%s — validated & executed", rep.Type)

	reps, err := c.List(ctx, &transport.Command{Path: "/system/identity", Verb: "print"})
	if err != nil {
		t.Fatalf("List(identity): %v", err)
	}
	for _, r := range reps {
		if r.Type == transport.ReplyRe {
			t.Logf("LAB identity name=%q", r.Attributes["name"])
		}
	}

	reps, err = c.List(ctx, &transport.Command{Path: "/system/resource", Verb: "print"})
	if err != nil {
		t.Fatalf("List(resource): %v", err)
	}
	for _, r := range reps {
		if r.Type == transport.ReplyRe {
			t.Logf("LAB resource uptime=%s cpu=%s%%", r.Attributes["uptime"], r.Attributes["cpu-load"])
		}
	}

	reps, err = c.List(ctx, &transport.Command{Path: "/ip/address", Verb: "print"})
	if err != nil {
		t.Fatalf("List(ip/address): %v", err)
	}
	rows := 0
	for _, r := range reps {
		if r.Type == transport.ReplyRe {
			rows++
		}
	}
	t.Logf("LAB /ip/address rows=%d (via List, non-gated seam)", rows)
}

func probeAttrNames(sch *schema.CommandSchema) []string {
	names := make([]string, len(sch.Attributes))
	for i, a := range sch.Attributes {
		names[i] = a.Name
	}
	return names
}

// probeErrCode extracts the roserr.Code from err, or "?" when err is not a
// roserr error.
func probeErrCode(err error) roserr.Code {
	var re *roserr.Error
	if errors.As(err, &re) {
		return re.Code
	}
	return "?"
}
