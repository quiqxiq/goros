package routeros

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/quiqxiq/goros/v4/proto"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/schema"
	"github.com/quiqxiq/goros/v4/transport"
)

// fakeRouterOS runs an in-memory RouterOS API server over net.Pipe.
// onRequest receives the sent words and must return the reply as a list of
// sentences (each an []string of words) — a !trap must be followed by a
// !done, mirroring real devices.
func fakeRouterOS(t *testing.T, onRequest func(words []string) [][]string) *Client {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	c, err := NewClient(clientConn)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		r := proto.NewReader(serverConn)
		w := proto.NewWriter(serverConn)
		for {
			sen, err := r.ReadSentence()
			if err != nil {
				return
			}
			for _, sentence := range onRequest(sentenceWords(sen)) {
				w.BeginSentence()
				for _, word := range sentence {
					w.WriteWord(word)
				}
				_ = w.EndSentence()
			}
		}
	}()
	// Cleanup order matters (LIFO): close the client first so the server
	// goroutine's ReadSentence unblocks, then wait for the goroutine.
	t.Cleanup(func() { <-done })
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// sentenceWords reconstructs the sent words from a parsed sentence.
func sentenceWords(sen *proto.Sentence) []string {
	words := []string{sen.Word}
	for _, p := range sen.List {
		words = append(words, "="+p.Key+"="+p.Value)
	}
	if sen.Tag != "" {
		words = append(words, ".tag="+sen.Tag)
	}
	return words
}

// attrValue returns the value of the =key= word, or "" when absent.
func attrValue(words []string, key string) string {
	prefix := "=" + key + "="
	for _, w := range words {
		if strings.HasPrefix(w, prefix) {
			return strings.TrimPrefix(w, prefix)
		}
	}
	return ""
}

// fixtureArgRows renders request=child arg rows followed by !done.
func fixtureArgRows(names ...string) [][]string {
	rows := make([][]string, 0, len(names)+1)
	for _, n := range names {
		rows = append(rows, []string{"!re", "=type=child", "=node-type=arg", "=name=" + n, "=completion=" + n})
	}
	rows = append(rows, []string{"!done"})
	return rows
}

// fixtureCmdRows renders request=child cmd rows (path-level verb nodes).
func fixtureCmdRows(names ...string) [][]string {
	rows := make([][]string, 0, len(names)+1)
	for _, n := range names {
		rows = append(rows, []string{"!re", "=type=child", "=node-type=cmd", "=name=" + n})
	}
	rows = append(rows, []string{"!done"})
	return rows
}

// callCounters tracks what reached the fake device.
type callCounters struct {
	executed      atomic.Int64 // the real command reached the device
	inspectProbes atomic.Int64 // ProbeInspect round-trips
	parseProbes   atomic.Int64 // ProbeParse round-trips
	parseScripts  atomic.Int64 // Gate 1 :parse round-trips
}

// v7Fake returns a Client over a fake device that serves /console/inspect and
// /execute =as-string= (RouterOS 7 behavior), with the schema fixtures for
// /ip/address print used by the tests.
func v7Fake(t *testing.T) (*Client, *callCounters) {
	t.Helper()
	cc := &callCounters{}
	c := fakeRouterOS(t, func(words []string) [][]string {
		switch {
		case words[0] == "/console/inspect":
			path, request := attrValue(words, "path"), attrValue(words, "request")
			if request == "child" && path == "system" {
				cc.inspectProbes.Add(1)
			}
			switch {
			case request == "child" && path == "system":
				return [][]string{{"!re", "=type=dir", "=name=system"}, {"!done"}}
			case request == "child" && path == "ip,address,print":
				return fixtureArgRows("address", "comment")
			case request == "child" && path == "ip,address,add":
				return fixtureArgRows("address", "comment", "interface")
			case request == "child" && path == "ip,address":
				return fixtureCmdRows("print", "add")
			case request == "completion" && strings.HasSuffix(path, ",print,proplist"):
				return fixtureArgRows("address", "interface")
			default:
				return [][]string{{"!done"}}
			}
		case words[0] == "/execute":
			script := attrValue(words, "script")
			if strings.Contains(script, `:put "probe"`) {
				cc.parseProbes.Add(1)
				return [][]string{{"!done", "=ret=probe"}}
			}
			if strings.Contains(script, ":parse") {
				cc.parseScripts.Add(1)
				switch {
				case strings.Contains(script, "/nonsense command"):
					// R11 (RESEARCH.md §13): inside a script expression the
					// error comes back wrapped, not line-anchored.
					return [][]string{{"!done", "=ret=(<% bad command name nonsense (line 1 column 2) nonsense;command)"}}
				case strings.Contains(script, "/nonsense/command"):
					return [][]string{{"!done", "=ret=syntax error (line 1 column 10)"}}
				case strings.Contains(script, "bogus"):
					return [][]string{{"!done", "=ret=expected end of command (line 1 column 24)"}}
				}
				return [][]string{{"!done", "=ret=(evl /ip/address/print)"}}
			}
			return [][]string{{"!done", "=ret="}}
		case words[0] == "/ip/address/print":
			cc.executed.Add(1)
			return [][]string{{"!re", "=address=1.2.3.4", "=interface=ether1"}, {"!done"}}
		case words[0] == "/ip/address/add":
			cc.executed.Add(1)
			return [][]string{{"!done"}}
		default:
			return [][]string{{"!done"}}
		}
	})
	return c, cc
}

func TestValidateStructuredRejectsBogusAttribute(t *testing.T) {
	c, cc := v7Fake(t)
	err := c.Validate(context.Background(), &transport.Command{
		Path: "/ip/address", Verb: "print", Attributes: map[string]string{"bogus": "1"},
	})
	if err == nil {
		t.Fatal("Validate(bogus): want error, got nil")
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
	for _, want := range []string{"address", "comment", "interface"} {
		found := false
		for _, a := range avail {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("available = %v, want contains %q", avail, want)
		}
	}
	if src, _ := cctx.Extra["validationSource"].(string); src != "inspect child+completion" {
		t.Errorf("validationSource = %q, want inspect child+completion", src)
	}
	if cc.executed.Load() != 0 {
		t.Errorf("command executed during dry-run: %d executions", cc.executed.Load())
	}
}

func TestValidateStructuredPasses(t *testing.T) {
	c, _ := v7Fake(t)
	if err := c.Validate(context.Background(), &transport.Command{Path: "/ip/address", Verb: "print"}); err != nil {
		t.Fatalf("Validate(print): %v", err)
	}
}

func TestValidateScriptRejectsSyntax(t *testing.T) {
	// R11: a space-form command inside the :parse script comes back wrapped
	// as "(<% bad command name ...)" — the classifier must still reject it.
	c, cc := v7Fake(t)
	err := c.Validate(context.Background(), &transport.Command{Script: "/nonsense command"})
	if err == nil {
		t.Fatal("Validate(nonsense): want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) {
		t.Fatalf("err = %v, want validation/syntax", err)
	}
	cctx, _ := roserr.ContextOf(err)
	if cctx.Extra["line"] != 1 || cctx.Extra["col"] != 2 {
		t.Errorf("position = line %v col %v, want 1/2", cctx.Extra["line"], cctx.Extra["col"])
	}
	if cc.executed.Load() != 0 {
		t.Error("dry-run must not execute the command")
	}
}

func TestValidateScriptAnchoredSyntaxPosition(t *testing.T) {
	// Slash-form unknown command -> bare "syntax error (line 1 column 10)".
	c, _ := v7Fake(t)
	err := c.Validate(context.Background(), &transport.Command{Script: "/nonsense/command"})
	if err == nil {
		t.Fatal("Validate(nonsense/command): want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) {
		t.Fatalf("err = %v, want validation/syntax", err)
	}
	cctx, _ := roserr.ContextOf(err)
	if cctx.Extra["line"] != 1 || cctx.Extra["col"] != 10 {
		t.Errorf("position = line %v col %v, want 1/10", cctx.Extra["line"], cctx.Extra["col"])
	}
}

func TestValidateScriptUnknownAttributeCoarse(t *testing.T) {
	// R10 (docs/RESEARCH.md §10): on 7.21.5 an unknown attribute is
	// indistinguishable from broken syntax -> coarse validation/syntax; the
	// precise Missing/Available diagnosis is Gate 2's job.
	c, _ := v7Fake(t)
	err := c.Validate(context.Background(), &transport.Command{Script: "/ip/address print bogus=1"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) {
		t.Errorf("err = %v, want coarse validation/syntax (R10)", err)
	}
}

func TestValidateScriptUnbalancedQuotesFailsLocally(t *testing.T) {
	c, cc := v7Fake(t)
	err := c.Validate(context.Background(), &transport.Command{Script: `/ip/address print comment="unclosed`})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) {
		t.Errorf("err = %v, want validation/syntax", err)
	}
	if cc.parseScripts.Load() != 0 {
		t.Errorf("unbalanced quotes must fail locally without a :parse round-trip; got %d", cc.parseScripts.Load())
	}
}

func TestRunStructuredExecutesAfterValidation(t *testing.T) {
	c, cc := v7Fake(t)
	rep, err := c.RunStructured(context.Background(), &transport.Command{Path: "/ip/address", Verb: "print"})
	if err != nil {
		t.Fatalf("RunStructured(print): %v", err)
	}
	if rep.Type != transport.ReplyDone {
		t.Errorf("reply.Type = %q, want !done", rep.Type)
	}
	if cc.executed.Load() != 1 {
		t.Errorf("executions = %d, want 1", cc.executed.Load())
	}
}

func TestRunStructuredBlockedByGate2(t *testing.T) {
	c, cc := v7Fake(t)
	_, err := c.RunStructured(context.Background(), &transport.Command{
		Path: "/ip/address", Verb: "add", Attributes: map[string]string{"bogus": "1"},
	})
	if err == nil || !roserr.IsCode(err, roserr.CodeValidationUnknownAttribute) {
		t.Fatalf("err = %v, want validation/unknown-attribute", err)
	}
	if cc.executed.Load() != 0 {
		t.Errorf("executions = %d, want 0 (blocked before execution)", cc.executed.Load())
	}
}

func TestInspectDiscoversSchema(t *testing.T) {
	c, _ := v7Fake(t)
	sch, err := c.Inspect(context.Background(), "/ip/address", "print")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if sch.Category != schema.CategoryTable {
		t.Errorf("Category = %q, want table", sch.Category)
	}
	for _, name := range []string{"address", "comment", "interface"} {
		if !sch.HasAttribute(name) {
			t.Errorf("schema missing attribute %q (attrs=%v)", name, sch.Attributes)
		}
	}
}

func TestProbesRunOnceAcrossCalls(t *testing.T) {
	c, cc := v7Fake(t)
	for i := 0; i < 2; i++ {
		if err := c.Validate(context.Background(), &transport.Command{Path: "/ip/address", Verb: "print"}); err != nil {
			t.Fatalf("Validate #%d: %v", i+1, err)
		}
	}
	if cc.inspectProbes.Load() != 1 {
		t.Errorf("inspect probes = %d, want 1 (probe-once)", cc.inspectProbes.Load())
	}
	if cc.parseProbes.Load() != 1 {
		t.Errorf("parse probes = %d, want 1 (probe-once)", cc.parseProbes.Load())
	}
}

// v6Fake returns a Client over a fake device without /console/inspect and
// without /execute =as-string= (RouterOS 6 behavior): both probes trap.
func v6Fake(t *testing.T) (*Client, *callCounters) {
	t.Helper()
	cc := &callCounters{}
	c := fakeRouterOS(t, func(words []string) [][]string {
		switch {
		case words[0] == "/console/inspect":
			return [][]string{{"!trap", "=message=no such command"}, {"!done"}}
		case words[0] == "/execute":
			if strings.Contains(attrValue(words, "script"), `:put "probe"`) {
				cc.parseProbes.Add(1)
			}
			return [][]string{{"!trap", "=message=unknown parameter"}, {"!done"}}
		case words[0] == "/ip/address/print":
			cc.executed.Add(1)
			return [][]string{{"!re", "=address=1.2.3.4"}, {"!done"}}
		default:
			return [][]string{{"!done"}}
		}
	})
	return c, cc
}

func TestMetricsInspectRoundTripsAndGateLatency(t *testing.T) {
	// Fase 10: Metrics must reflect /console/inspect round-trips (probe +
	// one discovery; the second Discover is a cache hit) and per-gate
	// latency.
	c, _ := v7Fake(t)
	ctx := context.Background()

	// First Inspect: probe (system) + discovery round-trips.
	if _, err := c.Inspect(ctx, "/ip/address", "print"); err != nil {
		t.Fatalf("Inspect #1: %v", err)
	}
	afterFirst := c.Metrics().InspectRoundTrips
	if afterFirst == 0 {
		t.Error("InspectRoundTrips = 0 after first Inspect")
	}
	// Second Inspect for the same path+verb is a cache hit: no new trips.
	if _, err := c.Inspect(ctx, "/ip/address", "print"); err != nil {
		t.Fatalf("Inspect #2: %v", err)
	}
	if got := c.Metrics().InspectRoundTrips; got != afterFirst {
		t.Errorf("InspectRoundTrips after cache hit = %d, want %d (cache effective)", got, afterFirst)
	}

	// Gate latency: a Script command routes through Gate 1, a structured
	// command through Gate 2.
	if err := c.Validate(ctx, &transport.Command{Script: "/system resource print"}); err != nil {
		t.Fatalf("Validate(script): %v", err)
	}
	if err := c.Validate(ctx, &transport.Command{Path: "/ip/address", Verb: "print"}); err != nil {
		t.Fatalf("Validate(structured): %v", err)
	}
	m := c.Metrics()
	if m.Gate1Latency <= 0 {
		t.Errorf("Gate1Latency = %v, want > 0", m.Gate1Latency)
	}
	if m.Gate2Latency <= 0 {
		t.Errorf("Gate2Latency = %v, want > 0", m.Gate2Latency)
	}
}

func TestV6DegradationSkipsGates(t *testing.T) {
	c, cc := v6Fake(t)
	ctx := context.Background()
	// Dry-run on v6: gates skip silently, nothing executes, no error.
	if err := c.Validate(ctx, &transport.Command{Path: "/ip/address", Verb: "print"}); err != nil {
		t.Fatalf("Validate on v6: %v", err)
	}
	if cc.executed.Load() != 0 {
		t.Errorf("dry-run executed: %d", cc.executed.Load())
	}
	// Structured run on v6: gates skip, the command really executes.
	rep, err := c.RunStructured(ctx, &transport.Command{Path: "/ip/address", Verb: "print"})
	if err != nil {
		t.Fatalf("RunStructured on v6: %v", err)
	}
	if rep.Type != transport.ReplyDone || cc.executed.Load() != 1 {
		t.Errorf("reply=%q executed=%d, want !done executed=1", rep.Type, cc.executed.Load())
	}
	if c.SupportsInspect() || c.SupportsParse() {
		t.Error("v6 flags must be false")
	}
}
