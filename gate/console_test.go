package gate_test

import (
	"context"
	"strings"
	"testing"

	"github.com/quiqxiq/goros/v4/gate"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
	"github.com/quiqxiq/goros/v4/transport/mock"
)

// consoleGate returns a mock console transport whose Run returns ret, plus
// the captured line.
func consoleGate(ret string) (*mock.Transport, *string) {
	m := mock.NewConsole()
	var gotLine string
	m.SetRunFn(func(line string) (string, error) {
		gotLine = line
		return ret, nil
	})
	return m, &gotLine
}

func TestValidateConsoleValid(t *testing.T) {
	m, line := consoleGate(`(evl /system/resource/print)`)
	if err := gate.ValidateConsole(context.Background(), m, "/system/resource/print"); err != nil {
		t.Fatalf("ValidateConsole(valid): %v", err)
	}
	if m.RunCalls() != 1 {
		t.Errorf("RunCalls = %d, want 1", m.RunCalls())
	}
	// The :parse script must embed the line as a RouterOS string literal.
	want := `:put [:parse "/system/resource/print"]`
	if *line != want {
		t.Errorf("run line = %q, want %q", *line, want)
	}
}

func TestValidateConsoleSyntaxError(t *testing.T) {
	m, _ := consoleGate(`syntax error (line 1 column 10)`)
	err := gate.ValidateConsole(context.Background(), m, "/nonsense command")
	if err == nil {
		t.Fatal("ValidateConsole(bad syntax): want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) {
		t.Errorf("err = %v, want validation/syntax", err)
	}
}

func TestValidateConsoleUnknownAttribute(t *testing.T) {
	// Console transports echo the wrapped form "(evl bad parameter ...)".
	m, _ := consoleGate(`(evl bad parameter bogus (line 1 column 19) /ip/address/print)`)
	err := gate.ValidateConsole(context.Background(), m, "/ip address print bogus=1")
	if err == nil {
		t.Fatal("ValidateConsole(bogus): want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationUnknownAttribute) {
		t.Errorf("err = %v, want validation/unknown-attribute", err)
	}
	ctx, _ := roserr.ContextOf(err)
	if ctx.Extra["attribute"] != "bogus" {
		t.Errorf("context.Extra[attribute] = %v, want bogus", ctx.Extra["attribute"])
	}
}

func TestValidateConsoleUnbalancedQuotesFailsLocally(t *testing.T) {
	m, _ := consoleGate(`(evl ...)`)
	err := gate.ValidateConsole(context.Background(), m, `/ip address print comment="unclosed`)
	if err == nil {
		t.Fatal("ValidateConsole(unbalanced): want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) {
		t.Errorf("err = %v, want validation/syntax", err)
	}
	if m.RunCalls() != 0 {
		t.Errorf("RunCalls = %d, want 0 (local preflight, no round-trip)", m.RunCalls())
	}
}

func TestNewConsoleGateUsesConsoleCLI(t *testing.T) {
	// A structured command through the console gate must render the
	// space-separated path form (R12) inside the :parse script — the form
	// RouterOS 6 SSH exec accepts.
	m, line := consoleGate(`(evl /ip address print)`)
	g := gate.NewConsoleGate(m, nil)
	err := g.Run(context.Background(), &transport.Command{
		Path: "/ip/address", Verb: "print", Attributes: map[string]string{"interface": "ether1"},
	})
	if err != nil {
		t.Fatalf("Gate1.Run(structured over console): %v", err)
	}
	want := `:put [:parse "/ip address print interface=ether1"]`
	if *line != want {
		t.Errorf("run line = %q, want %q", *line, want)
	}
}

func TestValidateConsoleTransportErrorPassesThrough(t *testing.T) {
	m := mock.NewConsole()
	transportErr := roserr.New(roserr.CodeConnectionRefused, "ssh: connection refused")
	m.SetRunFn(func(string) (string, error) { return "", transportErr })
	err := gate.ValidateConsole(context.Background(), m, "/system identity print")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	// A transport failure must NOT be relabeled as validation.
	if roserr.IsCode(err, roserr.CodeValidationSyntax) || roserr.IsCode(err, roserr.CodeValidationUnknownAttribute) {
		t.Errorf("err = %v, must not be a validation error", err)
	}
	if !roserr.IsCode(err, roserr.CodeConnectionRefused) {
		t.Errorf("err = %v, want original CodeConnectionRefused preserved", err)
	}
}

func TestNewConsoleCommandScriptVsStructured(t *testing.T) {
	// The adapter runs cmd.Script verbatim; without Script it falls back to
	// ConsoleCLI.
	m := mock.NewConsole()
	var got []string
	m.SetRunFn(func(line string) (string, error) {
		got = append(got, line)
		return "(evl ok)", nil
	})
	a := gate.NewConsoleCommand(m)
	if _, err := a.Command(context.Background(), &transport.Command{Script: "/ping 1.1.1.1 count=2"}); err != nil {
		t.Fatalf("Command(script): %v", err)
	}
	if _, err := a.Command(context.Background(), &transport.Command{Path: "/system", Verb: "identity", Attributes: map[string]string{"name": "x y"}}); err != nil {
		t.Fatalf("Command(structured): %v", err)
	}
	want := []string{"/ping 1.1.1.1 count=2", "/system identity name=\"x y\""}
	if len(got) != 2 || strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("run lines = %q, want %q", got, want)
	}
}
