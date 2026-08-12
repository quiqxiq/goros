// Package nativeapi adapts the existing RouterOS native-api client (root
// package `routeros`, files client.go/run.go/reply.go) to the transport
// contract without touching its wire behavior: `proto/` is not modified and
// every command still goes through the same Client machinery as before.
//
// Since Fase 6 (D-012) the canonical implementation of the transport seam —
// Command/List/InspectNodes translation and the session probes — lives on
// *routeros.Client itself (import cycle: nativeapi imports the root package,
// so the root package cannot import nativeapi). This adapter is therefore a
// thin, documented wrapper that exposes that canonical implementation as
// transport.StructuredTransport. Legacy behavior and errors remain
// reachable: a translated device error keeps the original *routeros.DeviceError
// as its cause, so errors.As still works.
//
// Reference: centrs NativeApiAdapter (src/protocols/adapter.ts) and
// executeScript (src/protocols/native-api.ts).
package nativeapi

import (
	"context"

	"github.com/quiqxiq/goros/v4"
	"github.com/quiqxiq/goros/v4/transport"
)

// Adapter wraps a connected, logged-in *routeros.Client and implements
// transport.StructuredTransport. It is safe to use after Client has been
// created and logged in (e.g. via routeros.Dial). All methods delegate to
// the canonical seam on *routeros.Client (D-012).
//
// Session capability flags (SupportsInspect, SupportsParse) are probed once
// after login and remembered by the underlying Client; both follow the same
// policy: a device trap (the feature is absent on this build) resolves to
// false silently, any other failure is returned as a real error.
type Adapter struct {
	c *routeros.Client
}

var (
	_ transport.Transport           = (*Adapter)(nil)
	_ transport.StructuredTransport = (*Adapter)(nil)
)

// New returns an Adapter over an already-connected, logged-in Client.
func New(c *routeros.Client) *Adapter {
	return &Adapter{c: c}
}

// Capabilities reports the seams native-api speaks: structured commands and
// /console/inspect are supported, console lines are not. (Per-session
// *probed* results live in SupportsInspect/SupportsParse; Capabilities is the
// static seam matrix.)
func (a *Adapter) Capabilities() transport.Capabilities {
	return transport.Capabilities{Structured: true, Inspect: true}
}

// Close closes the underlying client connection (idempotent).
func (a *Adapter) Close() error {
	return a.c.Close()
}

// Command sends one structured command and returns the terminal canonical
// reply (!done, or the !trap/!fatal error translated into the roserr
// taxonomy). A Script command is sent via /execute with as-string="" so its
// output arrives in the "ret" attribute.
func (a *Adapter) Command(ctx context.Context, cmd *transport.Command) (*transport.Reply, error) {
	return a.c.Command(ctx, cmd)
}

// List sends one structured command and returns every reply sentence as a
// canonical Reply: the !re data rows followed by the terminal !done. This is
// the row-returning form of Command, used for print-style commands and for
// reading /execute output (the "ret" attribute of the !re rows).
func (a *Adapter) List(ctx context.Context, cmd *transport.Command) ([]*transport.Reply, error) {
	return a.c.List(ctx, cmd)
}

// Inspect issues a /console/inspect probe (request=child|completion) with
// the path in comma-joined form and returns the parsed node rows. Traps
// (e.g. unknown path) surface as roserr errors; swallowing them is the
// schema layer's job.
func (a *Adapter) Inspect(ctx context.Context, request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
	return a.c.InspectNodes(ctx, request, path)
}

// ProbeInspect, SupportsInspect, ProbeParse, and SupportsParse delegate to
// the canonical session probes on *routeros.Client (D-009/D-012): probed
// once after login; device traps resolve to false silently; non-trap
// failures are returned as real errors.
func (a *Adapter) ProbeInspect(ctx context.Context) error { return a.c.ProbeInspect(ctx) }
func (a *Adapter) SupportsInspect() bool                  { return a.c.SupportsInspect() }
func (a *Adapter) ProbeParse(ctx context.Context) error   { return a.c.ProbeParse(ctx) }
func (a *Adapter) SupportsParse() bool                    { return a.c.SupportsParse() }
