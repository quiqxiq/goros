package transport

import (
	"reflect"
	"testing"
)

func TestCommandPathTokens(t *testing.T) {
	c := &Command{Path: "/ip/address"}
	if got := c.PathTokens(); !reflect.DeepEqual(got, []string{"ip", "address"}) {
		t.Errorf("PathTokens() = %v, want [ip address]", got)
	}
	c = &Command{Path: "/"}
	if got := c.PathTokens(); len(got) != 0 {
		t.Errorf("PathTokens() for \"/\" = %v, want empty", got)
	}
}

func TestCommandCLI(t *testing.T) {
	c := &Command{
		Path:       "/ip/address",
		Verb:       "add",
		Attributes: map[string]string{"address": "192.168.1.1", "interface": "ether1"},
	}
	// Attributes must render in sorted key order (deterministic).
	want := "/ip/address/add address=192.168.1.1 interface=ether1"
	if got := c.CLI(); got != want {
		t.Errorf("CLI() = %q, want %q", got, want)
	}
}

func TestCommandCLIQuoting(t *testing.T) {
	c := &Command{
		Path:       "/ip/address",
		Verb:       "add",
		Attributes: map[string]string{"comment": "has space", "empty": "", "quote": `a"b`},
	}
	want := `/ip/address/add comment="has space" empty= quote="a\"b"`
	if got := c.CLI(); got != want {
		t.Errorf("CLI() = %q, want %q", got, want)
	}
}

func TestCommandCLIQueriesAndProplist(t *testing.T) {
	c := &Command{
		Path:     "/ip/address",
		Verb:     "print",
		Queries:  []string{"type=ether"},
		Proplist: []string{"address", "interface"},
	}
	want := "/ip/address/print ?type=ether .proplist=address,interface"
	if got := c.CLI(); got != want {
		t.Errorf("CLI() = %q, want %q", got, want)
	}
}

func TestCommandWords(t *testing.T) {
	c := &Command{
		Path:       "/ip/address",
		Verb:       "print",
		Attributes: map[string]string{"detail": "", "type": "ether"},
		Queries:    []string{"type=ether"},
		Proplist:   []string{"address", "interface"},
	}
	want := []string{"/ip/address/print", "=detail=", "=type=ether", "?type=ether", ".proplist=address,interface"}
	if got := c.Words(); !reflect.DeepEqual(got, want) {
		t.Errorf("Words() = %q, want %q", got, want)
	}
}

func TestCommandWordsNoVerb(t *testing.T) {
	c := &Command{Path: "/ip/address"}
	if got := c.Words(); !reflect.DeepEqual(got, []string{"/ip/address"}) {
		t.Errorf("Words() without verb = %q, want [\"/ip/address\"]", got)
	}
}

func TestCommandWordsScript(t *testing.T) {
	c := &Command{Script: "/ping 1.1.1.1 count=2"}
	want := []string{"/execute", "=script=/ping 1.1.1.1 count=2", "=as-string="}
	if got := c.Words(); !reflect.DeepEqual(got, want) {
		t.Errorf("Words() for script = %q, want %q", got, want)
	}
}

func TestCommandCLIScriptOverride(t *testing.T) {
	c := &Command{
		Path:   "/ip/address",
		Verb:   "add",
		Script: "/ping 1.1.1.1 count=2",
	}
	if got := c.CLI(); got != c.Script {
		t.Errorf("CLI() = %q, want raw script %q", got, c.Script)
	}
}

func TestCommandConsoleCLI(t *testing.T) {
	c := &Command{
		Path:       "/ip/address",
		Verb:       "add",
		Attributes: map[string]string{"address": "192.168.1.1", "interface": "ether1"},
	}
	// Space-separated path form (R12): "ip address" joined with spaces,
	// not slash-joined "ip/address". Attributes in sorted order.
	want := "/ip address add address=192.168.1.1 interface=ether1"
	if got := c.ConsoleCLI(); got != want {
		t.Errorf("ConsoleCLI() = %q, want %q", got, want)
	}
}

func TestCommandConsoleCLIScriptOverride(t *testing.T) {
	c := &Command{
		Path:   "/ip/address",
		Verb:   "add",
		Script: "/ping 1.1.1.1 count=2",
	}
	if got := c.ConsoleCLI(); got != c.Script {
		t.Errorf("ConsoleCLI() = %q, want raw script %q", got, c.Script)
	}
}

func TestCommandConsoleCLIQuoting(t *testing.T) {
	c := &Command{
		Path:       "/ip/address",
		Verb:       "add",
		Attributes: map[string]string{"comment": "has space", "quote": `a"b`},
	}
	want := `/ip address add comment="has space" quote="a\"b"`
	if got := c.ConsoleCLI(); got != want {
		t.Errorf("ConsoleCLI() = %q, want %q", got, want)
	}
}

func TestCommandConsoleCLIQueriesAndProplist(t *testing.T) {
	c := &Command{
		Path:     "/ip/address",
		Verb:     "print",
		Queries:  []string{"type=ether"},
		Proplist: []string{"address", "interface"},
	}
	want := "/ip address print ?type=ether .proplist=address,interface"
	if got := c.ConsoleCLI(); got != want {
		t.Errorf("ConsoleCLI() = %q, want %q", got, want)
	}
}

func TestReplyRetAndMessage(t *testing.T) {
	r := &Reply{Type: ReplyDone, Attributes: map[string]string{"ret": "abc", "message": "boom"}}
	if r.Ret() != "abc" {
		t.Errorf("Ret() = %q, want abc", r.Ret())
	}
	if r.Message() != "boom" {
		t.Errorf("Message() = %q, want boom", r.Message())
	}
	empty := &Reply{Type: ReplyRe}
	if empty.Ret() != "" || empty.Message() != "" {
		t.Errorf("Ret()/Message() on empty attributes = %q/%q, want empty", empty.Ret(), empty.Message())
	}
}

func TestReplyTypes(t *testing.T) {
	types := []ReplyType{ReplyRe, ReplyDone, ReplyTrap, ReplyFatal, ReplyEmpty}
	wants := []string{"!re", "!done", "!trap", "!fatal", "!empty"}
	for i, rt := range types {
		if string(rt) != wants[i] {
			t.Errorf("ReplyType %d = %q, want %q", i, rt, wants[i])
		}
	}
}
