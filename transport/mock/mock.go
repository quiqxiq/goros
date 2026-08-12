// Package mock provides an in-memory transport for testing the gate, schema,
// and orchestration layers without a real RouterOS device. It implements the
// transport.StructuredTransport and transport.ConsoleTransport seams, records
// every call (for call-count assertions such as cache-hit checks), and lets
// tests script replies and failures per command.
package mock

import (
	"context"
	"sync"

	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
)

// Call is one recorded invocation of the mock.
type Call struct {
	Kind    string // "command", "inspect", or "run".
	Command *transport.Command
	Request transport.InspectRequestKind
	Path    string
	Line    string
}

// CommandFn scripts the behavior of Command. Returning an error fails the call.
type CommandFn func(cmd *transport.Command) (*transport.Reply, error)

// InspectFn scripts the behavior of Inspect. Returning an error fails the call.
type InspectFn func(request transport.InspectRequestKind, path string) ([]transport.InspectNode, error)

// RunFn scripts the behavior of Run. Returning an error fails the call.
type RunFn func(line string) (string, error)

// Transport is a scriptable, thread-safe in-memory transport. Create with New,
// NewStructured, or NewConsole, then override behavior with the Set* methods.
type Transport struct {
	mu sync.Mutex

	capabilities transport.Capabilities

	commandFn CommandFn
	inspectFn InspectFn
	runFn     RunFn

	calls []Call
	// Call counters for quick assertions without walking Calls.
	commandCalls int
	inspectCalls int
	runCalls     int

	closed   bool
	closeErr error
}

var (
	_ transport.Transport           = (*Transport)(nil)
	_ transport.StructuredTransport = (*Transport)(nil)
	_ transport.ConsoleTransport    = (*Transport)(nil)
)

// New returns a mock that supports every seam (structured + console + inspect)
// and answers calls with benign defaults: Command returns an empty !done
// reply, Inspect returns no rows, Run returns an empty string. Override with
// SetCommandFn / SetInspectFn / SetRunFn.
func New() *Transport {
	return &Transport{
		capabilities: transport.Capabilities{Structured: true, Console: true, Inspect: true},
		commandFn: func(*transport.Command) (*transport.Reply, error) {
			return &transport.Reply{Type: transport.ReplyDone, Attributes: map[string]string{}}, nil
		},
		inspectFn: func(transport.InspectRequestKind, string) ([]transport.InspectNode, error) {
			return nil, nil
		},
		runFn: func(string) (string, error) { return "", nil },
	}
}

// NewStructured returns a mock with only the structured seams (like native-api
// minus console): Structured and Inspect true, Console false.
func NewStructured() *Transport {
	t := New()
	t.capabilities.Console = false
	return t
}

// NewConsole returns a mock with only the console seam (like ssh and
// mac-telnet): Console true, Structured and Inspect false.
func NewConsole() *Transport {
	t := New()
	t.capabilities.Structured = false
	t.capabilities.Inspect = false
	return t
}

// SetCommandFn overrides the scripted behavior of Command.
func (t *Transport) SetCommandFn(fn CommandFn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.commandFn = fn
}

// SetInspectFn overrides the scripted behavior of Inspect.
func (t *Transport) SetInspectFn(fn InspectFn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inspectFn = fn
}

// SetRunFn overrides the scripted behavior of Run.
func (t *Transport) SetRunFn(fn RunFn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.runFn = fn
}

// SetCloseError injects an error that the next Close call returns.
func (t *Transport) SetCloseError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeErr = err
}

// Capabilities implements transport.Transport.
func (t *Transport) Capabilities() transport.Capabilities {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.capabilities
}

// Close implements transport.Transport. Idempotent.
func (t *Transport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return t.closeErr
}

// Closed reports whether Close has been called.
func (t *Transport) Closed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// Command implements transport.StructuredTransport.
func (t *Transport) Command(ctx context.Context, cmd *transport.Command) (*transport.Reply, error) {
	t.mu.Lock()
	if !t.capabilities.Structured {
		t.mu.Unlock()
		return nil, roserr.New(
			roserr.CodeCapabilityUnsupported,
			"mock transport does not support structured commands",
		)
	}
	t.commandCalls++
	t.calls = append(t.calls, Call{Kind: "command", Command: cmd})
	fn := t.commandFn
	t.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return fn(cmd)
}

// Inspect implements transport.StructuredTransport.
func (t *Transport) Inspect(ctx context.Context, request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
	t.mu.Lock()
	if !t.capabilities.Inspect {
		t.mu.Unlock()
		return nil, roserr.New(
			roserr.CodeCapabilityUnsupported,
			"mock transport does not support /console/inspect",
		)
	}
	t.inspectCalls++
	t.calls = append(t.calls, Call{Kind: "inspect", Request: request, Path: path})
	fn := t.inspectFn
	t.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return fn(request, path)
}

// Run implements transport.ConsoleTransport.
func (t *Transport) Run(ctx context.Context, line string) (string, error) {
	t.mu.Lock()
	if !t.capabilities.Console {
		t.mu.Unlock()
		return "", roserr.New(
			roserr.CodeCapabilityUnsupported,
			"mock transport does not support console lines",
		)
	}
	t.runCalls++
	t.calls = append(t.calls, Call{Kind: "run", Line: line})
	fn := t.runFn
	t.mu.Unlock()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	return fn(line)
}

// CommandCalls returns how many times Command has been invoked.
func (t *Transport) CommandCalls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.commandCalls
}

// InspectCalls returns how many times Inspect has been invoked.
func (t *Transport) InspectCalls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inspectCalls
}

// RunCalls returns how many times Run has been invoked.
func (t *Transport) RunCalls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.runCalls
}

// TotalCalls returns the total number of recorded calls.
func (t *Transport) TotalCalls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.calls)
}

// Calls returns a copy of all recorded calls, in order.
func (t *Transport) Calls() []Call {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Call(nil), t.calls...)
}
