package gate_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/quiqxiq/goros/v4/gate"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/schema"
	"github.com/quiqxiq/goros/v4/transport"
	"github.com/quiqxiq/goros/v4/transport/mock"
)

// gate2Fixture returns a mock answering the /ip/address print discovery in
// the 7.21.5 shapes, a schema Store over it, and a Gate2 wired to both.
func gate2Fixture(t *testing.T) (*mock.Transport, *gate.Gate2) {
	t.Helper()
	m := mock.NewStructured()
	m.SetInspectFn(func(request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
		switch {
		case request == transport.InspectChild && path == "ip,address":
			return []transport.InspectNode{
				{Type: "child", NodeType: "cmd", Name: "print"},
				{Type: "child", NodeType: "cmd", Name: "get"},
				{Type: "child", NodeType: "cmd", Name: "add"},
			}, nil
		case request == transport.InspectChild && path == "ip,address,print":
			return []transport.InspectNode{
				{Type: "child", NodeType: "arg", Name: "detail"},
				{Type: "child", NodeType: "arg", Name: "proplist"},
			}, nil
		case request == transport.InspectCompletion && path == "ip,address,print,proplist":
			return []transport.InspectNode{
				{Completion: "address"},
				{Completion: "interface"},
				{Completion: "comment"},
			}, nil
		default:
			return []transport.InspectNode{{Completion: " "}}, nil
		}
	})
	s := schema.NewStore(m)
	g := &gate.Gate2{Schema: s}
	return m, g
}

func TestGate2ValidAttributes(t *testing.T) {
	_, g := gate2Fixture(t)
	err := g.Validate(context.Background(), &transport.Command{
		Path:       "/ip/address",
		Verb:       "print",
		Attributes: map[string]string{"detail": "", "interface": "ether1"},
	})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestGate2RejectsUnknownAttribute(t *testing.T) {
	_, g := gate2Fixture(t)
	err := g.Validate(context.Background(), &transport.Command{
		Path:       "/ip/address",
		Verb:       "print",
		Attributes: map[string]string{"interface1": "ether1"},
	})
	if err == nil {
		t.Fatal("Validate: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeValidationUnknownAttribute) {
		t.Errorf("err = %v, want CodeValidationUnknownAttribute", err)
	}
	ctx, _ := roserr.ContextOf(err)
	extra := ctx.Extra
	if !reflect.DeepEqual(extra["missing"], []string{"interface1"}) {
		t.Errorf("Extra[missing] = %v, want [interface1]", extra["missing"])
	}
	avail, ok := extra["available"].([]string)
	if !ok {
		t.Fatalf("Extra[available] not a []string: %T", extra["available"])
	}
	found := false
	for _, a := range avail {
		if a == "interface" {
			found = true
		}
	}
	if !found {
		t.Errorf("Extra[available] = %v, want to contain interface", avail)
	}
	if extra["validationSource"] != "inspect child+completion" {
		t.Errorf("Extra[validationSource] = %v", extra["validationSource"])
	}
	if extra["path"] != "/ip/address" || extra["verb"] != "print" {
		t.Errorf("Extra path/verb = %v / %v", extra["path"], extra["verb"])
	}
}

func TestGate2SkipWhenInspectUnsupported(t *testing.T) {
	m, g := gate2Fixture(t)
	g.SupportsInspect = func() bool { return false }
	err := g.Validate(context.Background(), &transport.Command{
		Path: "/ip/address", Verb: "print", Attributes: map[string]string{"interface1": "x"},
	})
	if err != nil {
		t.Fatalf("Validate: want nil (skip on v6), got %v", err)
	}
	if m.InspectCalls() != 0 {
		t.Errorf("InspectCalls = %d, want 0 (skipped before discovery)", m.InspectCalls())
	}
}

func TestGate2SkipUnknownCategory(t *testing.T) {
	m := mock.NewStructured()
	m.SetInspectFn(func(request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
		return nil, roserr.New(roserr.CodeUnknownPath, "no such command prefix")
	})
	g := &gate.Gate2{Schema: schema.NewStore(m)}
	err := g.Validate(context.Background(), &transport.Command{
		Path: "/nonsense", Verb: "foo", Attributes: map[string]string{"x": "y"},
	})
	if err != nil {
		t.Fatalf("Validate: want nil (unknown category skips), got %v", err)
	}
}

func TestGate2UsesCache(t *testing.T) {
	m, g := gate2Fixture(t)
	for i := 0; i < 2; i++ {
		if err := g.Validate(context.Background(), &transport.Command{
			Path: "/ip/address", Verb: "print", Attributes: map[string]string{"detail": ""},
		}); err != nil {
			t.Fatalf("Validate #%d: %v", i+1, err)
		}
	}
	if got := m.InspectCalls(); got != 4 {
		t.Errorf("InspectCalls after 2x Validate = %d, want 4 (one discovery, cache hit)", got)
	}
}
