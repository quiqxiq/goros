package nativeapi_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"github.com/quiqxiq/goros/v4"
	"github.com/quiqxiq/goros/v4/proto"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
	"github.com/quiqxiq/goros/v4/transport/nativeapi"
)

// newFakeClient wires a *routeros.Client to an in-memory RouterOS server.
// For every received request sentence, onRequest is called with the sent
// words and must return the reply as a list of sentences (each an []string of
// words) — e.g. a !trap must be followed by a !done, mirroring real devices.
func newFakeClient(t *testing.T, onRequest func(words []string) [][]string) *routeros.Client {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	c, err := routeros.NewClient(clientConn)
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

func TestCapabilitiesAndClose(t *testing.T) {
	c := newFakeClient(t, func(words []string) [][]string { return nil })
	a := nativeapi.New(c)

	if got := a.Capabilities(); got != (transport.Capabilities{Structured: true, Console: false, Inspect: true}) {
		t.Errorf("Capabilities() = %+v", got)
	}
	if err := a.Close(); err != nil {
		t.Errorf("Close(): %v", err)
	}
}

func TestCommandStructured(t *testing.T) {
	var got []string
	c := newFakeClient(t, func(words []string) [][]string {
		got = words
		return [][]string{{"!done", "=ret=ok"}}
	})
	a := nativeapi.New(c)

	// Queries and Proplist rendering is covered by the contract test
	// (TestCommandWords); the fake server's proto.Reader cannot parse
	// "?query"/".proplist=" words (real devices can), so this test sticks to
	// attribute words.
	cmd := &transport.Command{
		Path:       "/ip/address",
		Verb:       "print",
		Attributes: map[string]string{"detail": ""},
	}
	rep, err := a.Command(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if !equal(got, cmd.Words()) {
		t.Errorf("sent words = %q, want %q", got, cmd.Words())
	}
	if rep.Type != transport.ReplyDone {
		t.Errorf("reply.Type = %q, want !done", rep.Type)
	}
	if rep.Attributes["ret"] != "ok" {
		t.Errorf("reply.Attributes[ret] = %q, want ok", rep.Attributes["ret"])
	}
}

func TestCommandScript(t *testing.T) {
	var got []string
	c := newFakeClient(t, func(words []string) [][]string {
		got = words
		return [][]string{{"!done", "=ret="}}
	})
	a := nativeapi.New(c)

	rep, err := a.Command(context.Background(), &transport.Command{Script: "/ping 1.1.1.1 count=2"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	want := []string{"/execute", "=script=/ping 1.1.1.1 count=2", "=as-string="}
	if !equal(got, want) {
		t.Errorf("sent words = %q, want %q", got, want)
	}
	if rep.Type != transport.ReplyDone {
		t.Errorf("reply.Type = %q, want !done", rep.Type)
	}
}

func TestCommandTrap(t *testing.T) {
	c := newFakeClient(t, func(words []string) [][]string {
		return [][]string{{"!trap", "=message=bad value for parameter", "=category=3"}, {"!done"}}
	})
	a := nativeapi.New(c)

	_, err := a.Command(context.Background(), &transport.Command{
		Path:       "/ip/address",
		Verb:       "add",
		Attributes: map[string]string{"address": "999.999.999.999"},
	})
	if err == nil {
		t.Fatal("Command: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeCommandFailed) {
		t.Errorf("err = %v, want CodeCommandFailed", err)
	}
	// Backward compat: the original device error stays reachable.
	var devErr *routeros.DeviceError
	if !errors.As(err, &devErr) {
		t.Fatalf("errors.As(*DeviceError) failed for %v", err)
	}
	if devErr.Sentence.Map["message"] != "bad value for parameter" {
		t.Errorf("message = %q, want %q", devErr.Sentence.Map["message"], "bad value for parameter")
	}
	if ctx, ok := roserr.ContextOf(err); !ok || ctx.Via != "native-api" {
		t.Errorf("ContextOf = %+v (ok=%v), want via=native-api", ctx, ok)
	}
}

func TestCommandFatal(t *testing.T) {
	c := newFakeClient(t, func(words []string) [][]string {
		return [][]string{{"!fatal", "=message=session terminated"}}
	})
	a := nativeapi.New(c)

	_, err := a.Command(context.Background(), &transport.Command{Path: "/ip/address", Verb: "print"})
	if err == nil {
		t.Fatal("Command: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeSessionClosed) {
		t.Errorf("err = %v, want CodeSessionClosed", err)
	}
}

func TestListReturnsRowsAndTerminal(t *testing.T) {
	c := newFakeClient(t, func(words []string) [][]string {
		return [][]string{
			{"!re", "=address=1.2.3.4", "=interface=ether1"},
			{"!done", "=ret=x"},
		}
	})
	a := nativeapi.New(c)

	reps, err := a.List(context.Background(), &transport.Command{Path: "/ip/address", Verb: "print"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(reps) != 2 {
		t.Fatalf("List returned %d replies, want 2", len(reps))
	}
	if reps[0].Type != transport.ReplyRe {
		t.Errorf("reps[0].Type = %q, want !re", reps[0].Type)
	}
	if reps[0].Attributes["address"] != "1.2.3.4" || reps[0].Attributes["interface"] != "ether1" {
		t.Errorf("reps[0].Attributes = %v", reps[0].Attributes)
	}
	if reps[1].Type != transport.ReplyDone || reps[1].Attributes["ret"] != "x" {
		t.Errorf("reps[1] = %+v, want !done with ret=x", reps[1])
	}
}

func TestRawWordsAndTag(t *testing.T) {
	c := newFakeClient(t, func(words []string) [][]string {
		return [][]string{{"!re", "=a=b", ".tag=l1"}, {"!done"}}
	})
	a := nativeapi.New(c)

	reps, err := a.List(context.Background(), &transport.Command{Path: "/x", Verb: "print"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	rep := reps[0]
	if rep.Tag != "l1" {
		t.Errorf("reply.Tag = %q, want l1", rep.Tag)
	}
	want := []string{"!re", "=a=b", ".tag=l1"}
	if !equal(rep.RawWords, want) {
		t.Errorf("RawWords = %q, want %q", rep.RawWords, want)
	}
}

func TestInspect(t *testing.T) {
	var got []string
	c := newFakeClient(t, func(words []string) [][]string {
		got = words
		return [][]string{
			{"!re", "=type=arg", "=name=address", "=completion=address"},
			{"!re", "=type=cmd", "=name=print", "=completion=print"},
			{"!done"},
		}
	})
	a := nativeapi.New(c)

	nodes, err := a.Inspect(context.Background(), transport.InspectChild, "ip,address")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	want := []string{"/console/inspect", "=path=ip,address", "=request=child"}
	if !equal(got, want) {
		t.Errorf("sent words = %q, want %q", got, want)
	}
	if len(nodes) != 2 {
		t.Fatalf("Inspect returned %d nodes, want 2", len(nodes))
	}
	if nodes[0].Type != "arg" || nodes[0].Name != "address" || nodes[0].Completion != "address" {
		t.Errorf("nodes[0] = %+v", nodes[0])
	}
	if nodes[1].Type != "cmd" || nodes[1].Name != "print" {
		t.Errorf("nodes[1] = %+v", nodes[1])
	}
}

func TestProbeInspectSupported(t *testing.T) {
	c := newFakeClient(t, func(words []string) [][]string {
		return [][]string{{"!re", "=type=dir", "=name=system"}, {"!done"}}
	})
	a := nativeapi.New(c)

	if err := a.ProbeInspect(context.Background()); err != nil {
		t.Fatalf("ProbeInspect: %v", err)
	}
	if !a.SupportsInspect() {
		t.Error("SupportsInspect() = false, want true (v7 path)")
	}
}

func TestProbeInspectUnsupportedTrap(t *testing.T) {
	c := newFakeClient(t, func(words []string) [][]string {
		return [][]string{{"!trap", "=message=no such command"}, {"!done"}}
	})
	a := nativeapi.New(c)

	// A trap is the documented silent-degradation path, not an error.
	if err := a.ProbeInspect(context.Background()); err != nil {
		t.Fatalf("ProbeInspect: %v", err)
	}
	if a.SupportsInspect() {
		t.Error("SupportsInspect() = true, want false (v6 path)")
	}
}

func TestProbeParseSupported(t *testing.T) {
	var got []string
	c := newFakeClient(t, func(words []string) [][]string {
		got = words
		return [][]string{{"!done", "=ret=probe"}}
	})
	a := nativeapi.New(c)

	if err := a.ProbeParse(context.Background()); err != nil {
		t.Fatalf("ProbeParse: %v", err)
	}
	if !a.SupportsParse() {
		t.Error("SupportsParse() = false, want true")
	}
	want := []string{"/execute", "=script=:put \"probe\"", "=as-string="}
	if !equal(got, want) {
		t.Errorf("sent words = %q, want %q", got, want)
	}
}

func TestProbeParseUnsupportedTrap(t *testing.T) {
	c := newFakeClient(t, func(words []string) [][]string {
		return [][]string{{"!trap", "=message=unknown parameter"}, {"!done"}}
	})
	a := nativeapi.New(c)

	if err := a.ProbeParse(context.Background()); err != nil {
		t.Fatalf("ProbeParse: %v", err)
	}
	if a.SupportsParse() {
		t.Error("SupportsParse() = true, want false (v6 path)")
	}
}

func TestProbeIdempotent(t *testing.T) {
	probes := 0
	c := newFakeClient(t, func(words []string) [][]string {
		probes++
		return [][]string{{"!done", "=ret=probe"}}
	})
	a := nativeapi.New(c)

	if err := a.ProbeParse(context.Background()); err != nil {
		t.Fatalf("ProbeParse: %v", err)
	}
	if err := a.ProbeParse(context.Background()); err != nil {
		t.Fatalf("ProbeParse #2: %v", err)
	}
	if probes != 1 {
		t.Errorf("ProbeParse issued %d requests, want 1 (probe-once semantics)", probes)
	}
}

func TestProbeConcurrentOnce(t *testing.T) {
	probes := 0
	c := newFakeClient(t, func(words []string) [][]string {
		probes++
		return [][]string{{"!done", "=ret=probe"}}
	})
	a := nativeapi.New(c)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = a.ProbeParse(context.Background())
		}()
	}
	wg.Wait()
	if probes != 1 {
		t.Errorf("ProbeParse issued %d requests, want 1 (concurrent probe-once)", probes)
	}
	if !a.SupportsParse() {
		t.Error("SupportsParse() = false, want true")
	}
}

func TestProbeReturnsRealErrors(t *testing.T) {
	c := newFakeClient(t, func(words []string) [][]string { return nil })
	a := nativeapi.New(c)
	// A closed connection is a transport problem, not a capability answer.
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := a.ProbeInspect(context.Background()); err == nil {
		t.Error("ProbeInspect after Close: want error, got nil")
	}
	if err := a.ProbeParse(context.Background()); err == nil {
		t.Error("ProbeParse after Close: want error, got nil")
	}
	// Failed probes must not pin a flag: a later retry can still probe.
	if a.SupportsInspect() || a.SupportsParse() {
		t.Error("flags set after failed probe")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
