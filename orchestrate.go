package routeros

import (
	"context"
	"time"

	"github.com/quiqxiq/goros/v4/gate"
	"github.com/quiqxiq/goros/v4/schema"
	"github.com/quiqxiq/goros/v4/transport"
)

// ensureRun builds — once per session — the validation pipeline: it runs the
// capability probes (ProbeInspect/ProbeParse), then wires the schema store
// and both gates over the client's own transport seam. Built lazily on first
// Validate/RunStructured/Inspect so sessions that only use the legacy Run*
// API pay no probe round-trips (D-013). Non-trap probe failures are real
// errors and surface from the calling facade method.
func (c *Client) ensureRun(ctx context.Context) error {
	rs := c.runState()
	rs.initMu.Lock()
	defer rs.initMu.Unlock()
	if rs.ready {
		return nil
	}
	if err := c.ProbeInspect(ctx); err != nil {
		return err
	}
	if err := c.ProbeParse(ctx); err != nil {
		return err
	}
	t := clientTransport{c: c}
	rs.t = t
	rs.store = schema.NewStore(t)
	rs.g1 = &gate.Gate1{Transport: t, SupportsParse: c.SupportsParse}
	rs.g2 = &gate.Gate2{Schema: rs.store, SupportsInspect: c.SupportsInspect}
	rs.ready = true
	return nil
}

// validate runs the applicable gates for cmd without executing anything
// (PLAN.md §10 routing): a free-form Script command is checked by Gate 1
// (the device's own :parse — syntax); a structured command skips Gate 1 (its
// rendering is deterministic and well-formed) and is checked by Gate 2
// (attribute schema — semantics). Gates that cannot apply (e.g. v6 sessions)
// skip silently by design (D-009).
func (c *Client) validate(ctx context.Context, cmd *transport.Command) error {
	rs := c.runState()
	// Fase 10 metric: record the applicable gate's latency. Routing: Script
	// -> Gate 1 (:parse), structured -> Gate 2 (schema) — see PLAN.md §10.
	start := time.Now()
	if cmd.Script != "" {
		err := rs.g1.Run(ctx, cmd)
		rs.g1LatencyNs.Store(int64(time.Since(start)))
		return err
	}
	err := rs.g2.Validate(ctx, cmd)
	rs.g2LatencyNs.Store(int64(time.Since(start)))
	return err
}

// Validate is the dry-run entry point (PLAN.md §10, DESIGN.md §2.8): it runs
// the applicable validation gates for cmd and NEVER executes the command
// itself, so it is safe to call repeatedly — including for action commands.
// It returns nil when the gates pass (or silently skip, e.g. on v6), or a
// structured roserr error (validation/syntax, validation/unknown-attribute).
func (c *Client) Validate(ctx context.Context, cmd *transport.Command) error {
	if err := c.ensureRun(ctx); err != nil {
		return err
	}
	return c.validate(ctx, cmd)
}

// Inspect discovers the CommandSchema for (path, verb) — pure discovery, no
// execution and no command-specific validation. On sessions without
// /console/inspect the returned schema carries CategoryUnknown instead of an
// error (the documented v6 degradation, D-009/D-010).
func (c *Client) Inspect(ctx context.Context, path, verb string) (*schema.CommandSchema, error) {
	if err := c.ensureRun(ctx); err != nil {
		return nil, err
	}
	return c.runState().store.Discover(ctx, path, verb)
}

// RunStructured validates cmd with the applicable gates and — only when they
// pass — executes it as its own sentence (path+verb as the command word,
// never wrapped in /execute; /execute is reserved for Script commands and
// Gate 1). It returns the terminal canonical reply (!done, or the
// !trap/!fatal error translated into the roserr taxonomy).
func (c *Client) RunStructured(ctx context.Context, cmd *transport.Command) (*transport.Reply, error) {
	if err := c.ensureRun(ctx); err != nil {
		return nil, err
	}
	if err := c.validate(ctx, cmd); err != nil {
		return nil, err
	}
	return c.Command(ctx, cmd)
}
