package routeros

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quiqxiq/goros/v4/gate"
	"github.com/quiqxiq/goros/v4/proto"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/schema"
	"github.com/quiqxiq/goros/v4/transport"
)

// runState carries the Fase 6 orchestrator state for one Client session: the
// session capability probes (SupportsInspect/SupportsParse) and the lazily
// built gate/schema pipeline. It lives in its own struct — one field on
// Client — so client.go stays untouched apart from that field (D-012).
type runState struct {
	// probeMu guards the probe flags and is held across a probe, so two
	// concurrent first calls cannot double-probe (probe-once is airtight);
	// Supports*/readers simply wait for the probe to finish.
	probeMu         sync.Mutex
	inspectProbed   bool
	supportsInspect bool
	parseProbed     bool
	supportsParse   bool

	// initMu guards lazy construction of the pipeline (probes -> store ->
	// gates). Built once on first Validate/RunStructured/Inspect (D-013).
	initMu sync.Mutex
	ready  bool
	t      transport.StructuredTransport
	store  *schema.Store
	g1     *gate.Gate1
	g2     *gate.Gate2

	// Fase 10 session metrics (Client.Metrics). Atomic so they are safe to
	// read concurrently with command traffic.
	inspectTrips atomic.Int64 // /console/inspect round-trips this session
	g1LatencyNs  atomic.Int64 // most recent Gate 1 (:parse) duration
	g2LatencyNs  atomic.Int64 // most recent Gate 2 (schema) duration
}

// runState returns the orchestrator state for this session, creating it on
// first use (defensive: Clients not built via NewClient).
func (c *Client) runState() *runState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.run == nil {
		c.run = &runState{}
	}
	return c.run
}

// ProbeInspect determines once whether the device serves /console/inspect
// (RouterOS 7+). A successful probe sets SupportsInspect()=true; a device
// trap ("no such command" on v6) resolves to false and is NOT an error — the
// documented silent-degradation path (D-009). Any other failure (timeout,
// network) is returned as-is: it says nothing about capability. The mutex is
// held across the probe, so two concurrent first calls cannot double-probe.
func (c *Client) ProbeInspect(ctx context.Context) error {
	rs := c.runState()
	rs.probeMu.Lock()
	defer rs.probeMu.Unlock()
	if rs.inspectProbed {
		return nil
	}
	if _, err := c.InspectNodes(ctx, transport.InspectChild, "system"); err != nil {
		var devErr *DeviceError
		if errors.As(err, &devErr) {
			// Device rejected the probe: /console/inspect is not served
			// on this build (verified: v6 -> trap).
			rs.inspectProbed = true
			return nil
		}
		return err
	}
	rs.inspectProbed = true
	rs.supportsInspect = true
	return nil
}

// SupportsInspect reports whether ProbeInspect determined that the device
// serves /console/inspect. False when not yet probed.
func (c *Client) SupportsInspect() bool {
	rs := c.runState()
	rs.probeMu.Lock()
	defer rs.probeMu.Unlock()
	return rs.inspectProbed && rs.supportsInspect
}

// ProbeParse determines once whether /execute with =as-string= works on this
// device (the Gate 1 script path; RouterOS 7+). It sends a trivial :put
// script and verifies its output arrives in the "ret" attribute. A trap
// ("unknown parameter" for as-string on v6) resolves to false silently; any
// other failure is returned as-is. Like ProbeInspect, the mutex is held
// across the probe so concurrent first calls cannot double-probe.
func (c *Client) ProbeParse(ctx context.Context) error {
	rs := c.runState()
	rs.probeMu.Lock()
	defer rs.probeMu.Unlock()
	if rs.parseProbed {
		return nil
	}
	rep, err := c.Command(ctx, &transport.Command{Script: `:put "probe"`})
	if err != nil {
		var devErr *DeviceError
		if errors.As(err, &devErr) {
			// Device rejected the script: as-string not supported
			// (verified: v6 -> trap "unknown parameter").
			rs.parseProbed = true
			return nil
		}
		return err
	}
	// as-string accepted; the :put output must arrive in "ret".
	rs.parseProbed = true
	rs.supportsParse = rep != nil && rep.Ret() != ""
	return nil
}

// SupportsParse reports whether ProbeParse determined that /execute with
// =as-string= works (i.e. the Gate 1 script path is usable). False when not
// yet probed.
func (c *Client) SupportsParse() bool {
	rs := c.runState()
	rs.probeMu.Lock()
	defer rs.probeMu.Unlock()
	return rs.parseProbed && rs.supportsParse
}

// TranslateError maps a legacy device error into the roserr taxonomy while
// keeping the original error reachable via errors.As/errors.Is (cause).
// Canonical implementation shared by the root seam and transport/nativeapi
// (D-012) — never duplicated.
func TranslateError(err error) error {
	var devErr *DeviceError
	if !errors.As(err, &devErr) {
		return err
	}
	code := roserr.CodeCommandFailed
	summary := devErr.Sentence.Map["message"]
	if summary == "" {
		summary = devErr.Error()
	}
	if devErr.Sentence.Word == "!fatal" {
		code = roserr.CodeSessionClosed
	}
	return roserr.New(
		code,
		summary,
		roserr.WithCause(err),
		roserr.WithContext(roserr.Context{Via: "native-api"}),
	)
}

// TranslateReply converts a legacy Reply into the canonical reply list,
// preserving sentence order: every !re row first, then the terminal sentence.
func TranslateReply(r *Reply) []*transport.Reply {
	n := len(r.Re)
	if r.Done != nil {
		n++
	}
	out := make([]*transport.Reply, 0, n)
	for _, sen := range r.Re {
		out = append(out, translateSentence(sen))
	}
	if r.Done != nil {
		out = append(out, translateSentence(r.Done))
	}
	return out
}

// translateSentence converts one wire sentence into the canonical Reply form.
func translateSentence(sen *proto.Sentence) *transport.Reply {
	return &transport.Reply{
		Type:       transport.ReplyType(sen.Word),
		Attributes: sen.Map,
		Tag:        sen.Tag,
		RawWords:   rawSentenceWords(sen),
	}
}

// rawSentenceWords reconstructs the original sentence words for debug
// purposes: the word, the =key=value pairs in wire order, then the .tag=
// word when present.
func rawSentenceWords(sen *proto.Sentence) []string {
	words := make([]string, 0, len(sen.List)+2)
	words = append(words, sen.Word)
	for _, p := range sen.List {
		words = append(words, "="+p.Key+"="+p.Value)
	}
	if sen.Tag != "" {
		words = append(words, ".tag="+sen.Tag)
	}
	return words
}

// Command sends one structured command and returns the terminal canonical
// reply (!done, or the !trap/!fatal error translated into the roserr
// taxonomy). A Script command is sent via /execute with as-string="" so its
// output arrives in the "ret" attribute. This is the canonical seam
// implementation of the native-api transport (D-012); transport/nativeapi
// delegates here.
func (c *Client) Command(ctx context.Context, cmd *transport.Command) (*transport.Reply, error) {
	replies, err := c.List(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if len(replies) == 0 {
		return nil, roserr.New(
			roserr.CodeCommandFailed,
			"no reply received from device",
			roserr.WithContext(roserr.Context{Via: "native-api"}),
		)
	}
	return replies[len(replies)-1], nil
}

// List sends one structured command and returns every reply sentence as a
// canonical Reply: the !re data rows followed by the terminal !done. This is
// the row-returning form of Command, used for print-style commands and for
// reading /execute output (the "ret" attribute of the !re rows).
func (c *Client) List(ctx context.Context, cmd *transport.Command) ([]*transport.Reply, error) {
	r, err := c.RunArgsContext(ctx, cmd.Words())
	if err != nil {
		return nil, TranslateError(err)
	}
	return TranslateReply(r), nil
}

// InspectNodes issues a /console/inspect probe (request=child|completion)
// with the path in comma-joined form and returns the parsed node rows. It is
// named InspectNodes (not Inspect) so the public schema-discovery method
// Inspect(path, verb) keeps the name the plan prescribes (D-013). Traps
// (e.g. unknown path) surface as roserr errors; swallowing them is the
// schema layer's job.
func (c *Client) InspectNodes(ctx context.Context, request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
	// Fase 10 metric: one round-trip per /console/inspect probe, including
	// the session probe and every schema-discovery trip (the schema cache's
	// effectiveness is visible as a flat counter across repeated Inspect
	// calls).
	c.runState().inspectTrips.Add(1)
	replies, err := c.List(ctx, &transport.Command{
		Path:       "/console",
		Verb:       "inspect",
		Attributes: map[string]string{"request": string(request), "path": path},
	})
	if err != nil {
		return nil, err
	}
	nodes := make([]transport.InspectNode, 0, len(replies))
	for _, rep := range replies {
		if rep.Type != transport.ReplyRe {
			continue
		}
		nodes = append(nodes, transport.InspectNode{
			Type:       rep.Attributes["type"],
			NodeType:   rep.Attributes["node-type"],
			Name:       rep.Attributes["name"],
			Completion: rep.Attributes["completion"],
			Value:      rep.Attributes["value"],
			Text:       rep.Attributes["text"],
		})
	}
	return nodes, nil
}

// clientTransport adapts Client's seam methods to transport.StructuredTransport
// for the gate/schema layers. The /console/inspect seam is named InspectNodes
// on Client to leave the name "Inspect" for the public discovery method
// (D-013); this wrapper restores the interface name.
type clientTransport struct{ c *Client }

func (t clientTransport) Capabilities() transport.Capabilities {
	return transport.Capabilities{Structured: true, Inspect: true}
}

func (t clientTransport) Close() error { return t.c.Close() }

func (t clientTransport) Command(ctx context.Context, cmd *transport.Command) (*transport.Reply, error) {
	return t.c.Command(ctx, cmd)
}

func (t clientTransport) Inspect(ctx context.Context, request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
	return t.c.InspectNodes(ctx, request, path)
}

// Metrics is a snapshot of cumulative session metrics (Fase 10).
type Metrics struct {
	// InspectRoundTrips is how many /console/inspect probes this session
	// issued. It grows on schema discovery; a second Discover for the same
	// path+verb must NOT grow it (the schema cache is effective).
	InspectRoundTrips int64
	// Gate1Latency is the most recent Gate 1 (:parse) validation duration.
	Gate1Latency time.Duration
	// Gate2Latency is the most recent Gate 2 (schema) validation duration.
	Gate2Latency time.Duration
}

// Metrics returns the current session metrics (Fase 10): /console/inspect
// round-trips and per-gate latency. All values are zero before the first
// Validate/Inspect/RunStructured. Read-only snapshot; safe to call
// concurrently with command traffic.
func (c *Client) Metrics() Metrics {
	rs := c.runState()
	return Metrics{
		InspectRoundTrips: rs.inspectTrips.Load(),
		Gate1Latency:      time.Duration(rs.g1LatencyNs.Load()),
		Gate2Latency:      time.Duration(rs.g2LatencyNs.Load()),
	}
}
