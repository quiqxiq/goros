package gate_test

import (
	"context"
	"testing"

	"github.com/quiqxiq/goros/v4/gate"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
	"github.com/quiqxiq/goros/v4/transport/mock"
)

func TestStringLiteral(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`/system/resource/print`, `"/system/resource/print"`},
		{`/ip/address/print interface="eth 1"`, `"/ip/address/print interface=\"eth 1\""`},
		{`a\b`, `"a\\b"`}, // backslash escaped before quote, so \" stays a single escaped quote
		{``, `""`},
	}
	for _, c := range cases {
		if got := gate.StringLiteral(c.in); got != c.want {
			t.Errorf("StringLiteral(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHasUnbalancedQuotes(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`/ip/address/print`, false},
		{`/ip/address/print comment="a b"`, false},
		{`/ip/address/print comment="a b`, true}, // unbalanced double quote
		{`/ip/address/print comment='a b`, true}, // unbalanced single quote
		{`/x say 'it''s fine'`, false},           // even quote count
		{`/x \"escaped\"`, false},                // escaped quotes don't count
		{`/x \`, false},                          // trailing backslash: nothing to escape
	}
	for _, c := range cases {
		if got := gate.HasUnbalancedQuotes(c.in); got != c.want {
			t.Errorf("HasUnbalancedQuotes(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestClassifierFixtures(t *testing.T) {
	cases := []struct {
		name      string
		ret       string
		valid     bool
		code      roserr.Code
		attribute string
		line, col int
	}{
		{
			name:  "valid 7.21.5 evl",
			ret:   `(evl /system/resource/print)`,
			valid: true,
		},
		{
			name: "bad parameter centrs corpus",
			ret:  `bad parameter interface (line 1 column 24)`,
			code: roserr.CodeValidationUnknownAttribute, attribute: "interface", line: 1, col: 24,
		},
		{
			name: "expected end of command 7.21.5",
			ret:  `expected end of command (line 1 column 24)`,
			code: roserr.CodeValidationSyntax, line: 1, col: 24,
		},
		{
			name: "syntax error 7.21.5",
			ret:  `syntax error (line 1 column 10)`,
			code: roserr.CodeValidationSyntax, line: 1, col: 10,
		},
		{
			name:      "wrapped bad parameter (console echo)",
			ret:       `(evl bad parameter bogus (line 1 column 19) /ip/address/print)`,
			code:      roserr.CodeValidationUnknownAttribute,
			attribute: "bogus",
			line:      1, col: 19,
		},
		{
			name:  "unrecognized text is defensive pass",
			ret:   `some future message format`,
			valid: true,
		},
		{
			name:  "empty output passes",
			ret:   ``,
			valid: true,
		},
		{
			name: "command not found variant",
			ret:  `bad command name (line 1 column 10)`,
			code: roserr.CodeValidationSyntax,
			line: 1, col: 10,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := (gate.PureSyntaxClassifier{}).Classify(c.ret)
			if res.Valid != c.valid {
				t.Errorf("Classify(%q).Valid = %v, want %v", c.ret, res.Valid, c.valid)
			}
			if !c.valid {
				if res.Code != c.code {
					t.Errorf("Classify(%q).Code = %q, want %q", c.ret, res.Code, c.code)
				}
				if res.Attribute != c.attribute {
					t.Errorf("Classify(%q).Attribute = %q, want %q", c.ret, res.Attribute, c.attribute)
				}
				if res.Line != c.line || res.Col != c.col {
					t.Errorf("Classify(%q) position = %d:%d, want %d:%d", c.ret, res.Line, res.Col, c.line, c.col)
				}
			}
		})
	}
}

// newGate1Mock returns a mock transport plus a Gate1 wired to it.
func newGate1Mock(ret string) (*mock.Transport, *gate.Gate1) {
	m := mock.NewStructured()
	m.SetCommandFn(func(cmd *transport.Command) (*transport.Reply, error) {
		return &transport.Reply{Type: transport.ReplyDone, Attributes: map[string]string{"ret": ret}}, nil
	})
	g := &gate.Gate1{Transport: m}
	return m, g
}

func TestGate1Valid(t *testing.T) {
	m, g := newGate1Mock(`(evl /system/resource/print)`)
	err := g.Run(context.Background(), &transport.Command{Path: "/system/resource", Verb: "print"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if m.CommandCalls() != 1 {
		t.Errorf("CommandCalls = %d, want 1", m.CommandCalls())
	}
	// The script must embed the deterministic CLI rendering.
	call := m.Calls()[0]
	want := "/execute"
	got := call.Command.Words()
	if len(got) == 0 || got[0] != want {
		t.Fatalf("sent words = %q, want first word %q", got, want)
	}
	if got[1] != `=script=:put [:parse "/system/resource/print"]` {
		t.Errorf("script word = %q", got[1])
	}
}

func TestGate1SyntaxError(t *testing.T) {
	_, g := newGate1Mock(`syntax error (line 1 column 10)`)
	err := g.Run(context.Background(), &transport.Command{Path: "/nonsense", Verb: "command"})
	if err == nil {
		t.Fatal("Run: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) {
		t.Errorf("err = %v, want CodeValidationSyntax", err)
	}
	ctx, _ := roserr.ContextOf(err)
	if ctx.Extra["line"] != 1 || ctx.Extra["col"] != 10 {
		t.Errorf("context.Extra = %v, want line=1 col=10", ctx.Extra)
	}
	if ctx.Path != "/nonsense" {
		t.Errorf("context.Path = %q, want /nonsense", ctx.Path)
	}
}

func TestGate1UnknownAttribute(t *testing.T) {
	_, g := newGate1Mock(`bad parameter interface (line 1 column 24)`)
	err := g.Run(context.Background(), &transport.Command{
		Path: "/ip/address", Verb: "print", Attributes: map[string]string{"interface1": "x"},
	})
	if err == nil {
		t.Fatal("Run: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationUnknownAttribute) {
		t.Errorf("err = %v, want CodeValidationUnknownAttribute", err)
	}
	ctx, _ := roserr.ContextOf(err)
	if ctx.Extra["attribute"] != "interface" {
		t.Errorf("context.Extra[attribute] = %v, want interface", ctx.Extra["attribute"])
	}
}

func TestGate1PreflightUnbalancedQuotes(t *testing.T) {
	m := mock.NewStructured()
	g := &gate.Gate1{Transport: m}
	// A raw script line is passed through CLI() unescaped, so an unbalanced
	// quote in it must fail locally (structured attributes are already
	// protected by Command.CLI()'s value quoting).
	err := g.Run(context.Background(), &transport.Command{
		Script: `/ip/address/print comment="unclosed`,
	})
	if err == nil {
		t.Fatal("Run: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) {
		t.Errorf("err = %v, want CodeValidationSyntax", err)
	}
	if m.CommandCalls() != 0 {
		t.Errorf("CommandCalls = %d, want 0 (local preflight, no round-trip)", m.CommandCalls())
	}
}

func TestGate1TransportErrorPassesThrough(t *testing.T) {
	m := mock.NewStructured()
	transportErr := roserr.New(roserr.CodeCommandFailed, "device rejected the script")
	m.SetCommandFn(func(cmd *transport.Command) (*transport.Reply, error) {
		return nil, transportErr
	})
	g := &gate.Gate1{Transport: m}
	err := g.Run(context.Background(), &transport.Command{Path: "/ip/address", Verb: "print"})
	if err == nil {
		t.Fatal("Run: want error, got nil")
	}
	// A script/transport failure must NOT be relabeled as validation.
	if roserr.IsCode(err, roserr.CodeValidationSyntax) || roserr.IsCode(err, roserr.CodeValidationUnknownAttribute) {
		t.Errorf("err = %v, must not be a validation error", err)
	}
	if !roserr.IsCode(err, roserr.CodeCommandFailed) {
		t.Errorf("err = %v, want original CodeCommandFailed preserved", err)
	}
}

func TestGate1SkipWhenParseUnsupported(t *testing.T) {
	m := mock.NewStructured()
	m.SetCommandFn(func(cmd *transport.Command) (*transport.Reply, error) {
		t.Fatal("Command must not be called on a skipped gate")
		return nil, nil
	})
	g := &gate.Gate1{Transport: m, SupportsParse: func() bool { return false }}
	err := g.Run(context.Background(), &transport.Command{Path: "/ip/address", Verb: "print"})
	if err != nil {
		t.Fatalf("Run: want nil (skip), got %v", err)
	}
	if m.CommandCalls() != 0 {
		t.Errorf("CommandCalls = %d, want 0", m.CommandCalls())
	}
}
