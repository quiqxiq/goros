package mock

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
)

func TestNewDefaults(t *testing.T) {
	m := New()

	reply, err := m.Command(context.Background(), &transport.Command{Path: "/ip/address", Verb: "print"})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if reply.Type != transport.ReplyDone {
		t.Errorf("reply.Type = %q, want !done", reply.Type)
	}

	nodes, err := m.Inspect(context.Background(), transport.InspectChild, "ip,address")
	if err != nil || len(nodes) != 0 {
		t.Errorf("Inspect = %v, %v; want empty, nil", nodes, err)
	}

	out, err := m.Run(context.Background(), "/ip/address/print")
	if err != nil || out != "" {
		t.Errorf("Run = %q, %v; want empty, nil", out, err)
	}
}

func TestCapabilityVariants(t *testing.T) {
	s := NewStructured()
	if s.Capabilities() != (transport.Capabilities{Structured: true, Console: false, Inspect: true}) {
		t.Errorf("NewStructured capabilities = %+v", s.Capabilities())
	}
	if _, err := s.Run(context.Background(), "/x"); !roserr.IsCode(err, roserr.CodeCapabilityUnsupported) {
		t.Errorf("Run on structured-only mock: err = %v, want CodeCapabilityUnsupported", err)
	}

	c := NewConsole()
	if c.Capabilities() != (transport.Capabilities{Structured: false, Console: true, Inspect: false}) {
		t.Errorf("NewConsole capabilities = %+v", c.Capabilities())
	}
	if _, err := c.Command(context.Background(), &transport.Command{Path: "/x"}); !roserr.IsCode(err, roserr.CodeCapabilityUnsupported) {
		t.Errorf("Command on console-only mock: err = %v, want CodeCapabilityUnsupported", err)
	}
	if _, err := c.Inspect(context.Background(), transport.InspectChild, "x"); !roserr.IsCode(err, roserr.CodeCapabilityUnsupported) {
		t.Errorf("Inspect on console-only mock: err = %v, want CodeCapabilityUnsupported", err)
	}
}

func TestCallRecordingAndCounters(t *testing.T) {
	m := New()
	cmd := &transport.Command{Path: "/ip/address", Verb: "print"}

	if _, err := m.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Inspect(context.Background(), transport.InspectChild, "ip,address"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Run(context.Background(), "/ip/address/print"); err != nil {
		t.Fatal(err)
	}

	if m.CommandCalls() != 1 || m.InspectCalls() != 1 || m.RunCalls() != 1 || m.TotalCalls() != 3 {
		t.Fatalf("counters = cmd%d insp%d run%d total%d, want 1/1/1/3",
			m.CommandCalls(), m.InspectCalls(), m.RunCalls(), m.TotalCalls())
	}

	calls := m.Calls()
	if len(calls) != 3 {
		t.Fatalf("Calls() len = %d, want 3", len(calls))
	}
	if calls[0].Kind != "command" || calls[0].Command != cmd {
		t.Errorf("calls[0] = %+v, want recorded command", calls[0])
	}
	if calls[1].Kind != "inspect" || calls[1].Request != transport.InspectChild || calls[1].Path != "ip,address" {
		t.Errorf("calls[1] = %+v, want recorded inspect", calls[1])
	}
	if calls[2].Kind != "run" || calls[2].Line != "/ip/address/print" {
		t.Errorf("calls[2] = %+v, want recorded run", calls[2])
	}
}

func TestHandlers(t *testing.T) {
	m := New()
	m.SetCommandFn(func(cmd *transport.Command) (*transport.Reply, error) {
		return &transport.Reply{Type: transport.ReplyTrap, Attributes: map[string]string{"message": "boom"}}, nil
	})
	m.SetInspectFn(func(request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
		return []transport.InspectNode{{Type: "arg", Name: "address"}}, nil
	})
	m.SetRunFn(func(line string) (string, error) { return "output", nil })

	reply, err := m.Command(context.Background(), &transport.Command{Path: "/ip/address", Verb: "add"})
	if err != nil || reply.Type != transport.ReplyTrap || reply.Message() != "boom" {
		t.Errorf("Command = %+v, %v; want trap/boom", reply, err)
	}
	nodes, err := m.Inspect(context.Background(), transport.InspectChild, "ip,address")
	if err != nil || len(nodes) != 1 || nodes[0].Name != "address" {
		t.Errorf("Inspect = %+v, %v; want one arg node", nodes, err)
	}
	out, err := m.Run(context.Background(), "/x")
	if err != nil || out != "output" {
		t.Errorf("Run = %q, %v; want output", out, err)
	}
}

func TestHandlerErrorPropagates(t *testing.T) {
	m := New()
	want := roserr.New(roserr.CodeValidationSyntax, "bad syntax")
	m.SetCommandFn(func(*transport.Command) (*transport.Reply, error) { return nil, want })

	_, err := m.Command(context.Background(), &transport.Command{Path: "/x"})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want the handler error", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	m := New()
	if err := m.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if !m.Closed() {
		t.Error("Closed() = false after Close")
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close: %v, want nil (idempotent)", err)
	}
}

func TestCloseError(t *testing.T) {
	m := New()
	want := errors.New("close failed")
	m.SetCloseError(want)
	if err := m.Close(); !errors.Is(err, want) {
		t.Errorf("Close err = %v, want %v", err, want)
	}
}

func TestContextCancellation(t *testing.T) {
	m := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := m.Command(ctx, &transport.Command{Path: "/x"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Command with cancelled ctx: err = %v, want context.Canceled", err)
	}
	if _, err := m.Inspect(ctx, transport.InspectChild, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("Inspect with cancelled ctx: err = %v, want context.Canceled", err)
	}
	if _, err := m.Run(ctx, "/x"); !errors.Is(err, context.Canceled) {
		t.Errorf("Run with cancelled ctx: err = %v, want context.Canceled", err)
	}
}

// TestConcurrentCalls exercises the mock under the race detector.
func TestConcurrentCalls(t *testing.T) {
	m := New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.Command(context.Background(), &transport.Command{Path: "/ip/address", Verb: "print"})
			_, _ = m.Inspect(context.Background(), transport.InspectChild, "ip,address")
			_, _ = m.Run(context.Background(), "/ip/address/print")
		}()
	}
	wg.Wait()
	if got := m.TotalCalls(); got != 150 {
		t.Errorf("TotalCalls() = %d, want 150", got)
	}
}
