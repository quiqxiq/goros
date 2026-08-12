package schema_test

import (
	"context"
	"testing"
	"time"

	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/schema"
	"github.com/quiqxiq/goros/v4/transport"
	"github.com/quiqxiq/goros/v4/transport/mock"
)

func TestPathTokens(t *testing.T) {
	if got := schema.PathTokens("/ip/address"); !equalStrings(got, []string{"ip", "address"}) {
		t.Errorf("PathTokens(/ip/address) = %q", got)
	}
	if got := schema.PathTokens("/"); len(got) != 0 {
		t.Errorf("PathTokens(/) = %q, want empty", got)
	}
	if got := schema.InspectPath([]string{"ip", "address"}); got != "ip,address" {
		t.Errorf("InspectPath = %q, want ip,address (comma form, not slash)", got)
	}
}

func TestNodePredicates(t *testing.T) {
	// 7.21.5 shape: type="child" + node-type carries the kind.
	if !schema.IsArgumentNode(transport.InspectNode{Type: "child", NodeType: "arg", Name: "address"}) {
		t.Error("IsArgumentNode: 7.21.5 node-type=arg must be true")
	}
	// Legacy shape: type carries the kind.
	if !schema.IsArgumentNode(transport.InspectNode{Type: "arg", Name: "address"}) {
		t.Error("IsArgumentNode: legacy type=arg must be true")
	}
	if schema.IsArgumentNode(transport.InspectNode{Type: "child", NodeType: "cmd", Name: "print"}) {
		t.Error("IsArgumentNode: cmd node must be false")
	}
	if !schema.IsCommandNode(transport.InspectNode{Type: "child", NodeType: "cmd", Name: "print"}, "print") {
		t.Error("IsCommandNode: 7.21.5 node-type=cmd must be true")
	}
	if schema.IsCommandNode(transport.InspectNode{Type: "child", NodeType: "cmd", Name: "get"}, "print") {
		t.Error("IsCommandNode: wrong name must be false")
	}
}

func TestExtractCompletionNames(t *testing.T) {
	rows := []transport.InspectNode{
		{Completion: "address", Text: "Local IP address"}, // description must NOT leak
		{Completion: "name=value"},                        // name=value form -> cut at "="
		{Name: "via-name"},
		{Value: "  spaced  "},
		{Text: "only-text-fallback"},
		{Text: ""},
	}
	got := schema.ExtractCompletionNames(rows)
	want := []string{"address", "name", "via-name", "spaced", "only-text-fallback"}
	if !equalStrings(got, want) {
		t.Errorf("ExtractCompletionNames = %q, want %q (no dedup)", got, want)
	}
}

// addressPrintFixture scripts a mock that answers the /ip/address print
// discovery probes in the 7.21.5 shapes (docs/RESEARCH.md §11), dispatching
// on request kind + path (child and completion share the same paths).
func addressPrintFixture() *mock.Transport {
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
				{Type: "child", NodeType: "arg", Name: "from"},
				{Type: "child", NodeType: "arg", Name: "interval"},
				{Type: "child", NodeType: "arg", Name: "proplist"},
				{Type: "child", NodeType: "arg", Name: "where"},
			}, nil
		case request == transport.InspectCompletion && path == "ip,address,print,proplist":
			return []transport.InspectNode{
				{Completion: "address", Text: "Local IP address"},
				{Completion: "interface"},
				{Completion: "comment"},
				{Completion: "disabled"},
				{Completion: "dynamic"},
				{Completion: "network"},
				// Structural garbage that must be filtered:
				{Completion: "["},
				{Completion: "("},
				{Completion: "$"},
				{Completion: `"`},
				{Completion: "*"},
				{Completion: "<value>"},
				{Completion: "about"},
			}, nil
		default:
			// Plain completion ("ip,address,print") returns a single
			// whitespace node — noise.
			return []transport.InspectNode{{Completion: " "}}, nil
		}
	})
	return m
}

func TestDiscoverTable(t *testing.T) {
	s := schema.NewStore(addressPrintFixture())
	sch, err := s.Discover(context.Background(), "/ip/address", "print")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if sch.Category != schema.CategoryTable {
		t.Errorf("Category = %q, want table", sch.Category)
	}
	if sch.Source != "inspect child+completion" {
		t.Errorf("Source = %q", sch.Source)
	}
	// Union of child args (detail, from, interval, proplist, where) and
	// completion fields (address, interface, comment, disabled, dynamic,
	// network), sorted, garbage filtered.
	want := []string{"address", "comment", "detail", "disabled", "dynamic", "from", "interface", "interval", "network", "proplist", "where"}
	got := make([]string, len(sch.Attributes))
	for i, a := range sch.Attributes {
		got[i] = a.Name
	}
	if !equalStrings(got, want) {
		t.Errorf("Attributes = %q, want %q", got, want)
	}
	for _, n := range []string{"[", "(", "$", `"`, "*", "<value>", "about"} {
		if sch.HasAttribute(n) {
			t.Errorf("structural garbage %q leaked into schema", n)
		}
	}
	if !sch.HasAttribute("address") {
		t.Error("HasAttribute(address) = false")
	}
}

func TestDiscoverAction(t *testing.T) {
	m := mock.NewStructured()
	m.SetInspectFn(func(request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
		switch {
		case request == transport.InspectChild && path == "tool":
			return []transport.InspectNode{
				{Type: "child", NodeType: "cmd", Name: "ping"},
				{Type: "child", NodeType: "cmd", Name: "traceroute"},
				{Type: "child", NodeType: "cmd", Name: "bandwidth-test"},
			}, nil
		case request == transport.InspectChild && path == "tool,ping":
			return []transport.InspectNode{
				{Type: "self", NodeType: "cmd", Name: "ping"},
				{Type: "child", NodeType: "arg", Name: "address"},
				{Type: "child", NodeType: "arg", Name: "count"},
				{Type: "child", NodeType: "arg", Name: "interface"},
				{Type: "child", NodeType: "arg", Name: "size"},
			}, nil
		default:
			return []transport.InspectNode{{Completion: " "}}, nil
		}
	})
	s := schema.NewStore(m)
	sch, err := s.Discover(context.Background(), "/tool", "ping")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if sch.Category != schema.CategoryAction {
		t.Errorf("Category = %q, want action (no print/get at /tool)", sch.Category)
	}
	want := []string{"address", "count", "interface", "size"}
	got := make([]string, len(sch.Attributes))
	for i, a := range sch.Attributes {
		got[i] = a.Name
	}
	if !equalStrings(got, want) {
		t.Errorf("Attributes = %q, want %q (input arguments only)", got, want)
	}
	// Action verbs must NOT trigger the .proplist field trick.
	for _, c := range m.Calls() {
		if c.Kind == "inspect" && c.Path == "tool,ping,proplist" {
			t.Errorf("proplist trick used for action verb ping: %+v", c)
		}
	}
}

func TestDiscoverUnknownPath(t *testing.T) {
	m := mock.NewStructured()
	m.SetInspectFn(func(request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
		return nil, roserr.New(roserr.CodeUnknownPath, "no such command prefix")
	})
	s := schema.NewStore(m)
	sch, err := s.Discover(context.Background(), "/nonsense", "foo")
	if err != nil {
		t.Fatalf("Discover: want nil error for unknown path, got %v", err)
	}
	if sch.Category != schema.CategoryUnknown {
		t.Errorf("Category = %q, want unknown", sch.Category)
	}
}

func TestDiscoverPropagatesRealErrors(t *testing.T) {
	m := mock.NewStructured()
	m.SetInspectFn(func(request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
		return nil, roserr.New(roserr.CodeTimeout, "timed out")
	})
	s := schema.NewStore(m)
	_, err := s.Discover(context.Background(), "/ip/address", "print")
	if err == nil {
		t.Fatal("Discover: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeTimeout) {
		t.Errorf("err = %v, want CodeTimeout preserved", err)
	}
}

func TestDiscoverOverride(t *testing.T) {
	s := schema.NewStore(addressPrintFixture())
	s.RegisterCategory("/ip/address", "print", schema.CategoryAction)
	sch, err := s.Discover(context.Background(), "/ip/address", "print")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if sch.Category != schema.CategoryAction {
		t.Errorf("Category = %q, want action (override wins)", sch.Category)
	}
}

func TestOverrideAfterCacheFillEvicts(t *testing.T) {
	s := schema.NewStore(addressPrintFixture())
	if _, err := s.Discover(context.Background(), "/ip/address", "print"); err != nil {
		t.Fatalf("Discover #1: %v", err)
	}
	// An override registered AFTER a cache fill must still win: it evicts
	// the cached entry so the next Discover re-applies the category.
	s.RegisterCategory("/ip/address", "print", schema.CategoryAction)
	sch, err := s.Discover(context.Background(), "/ip/address", "print")
	if err != nil {
		t.Fatalf("Discover #2: %v", err)
	}
	if sch.Category != schema.CategoryAction {
		t.Errorf("Category = %q, want action (override after cache fill)", sch.Category)
	}
}

func TestCacheHitSingleRoundTrip(t *testing.T) {
	m := addressPrintFixture()
	s := schema.NewStore(m)
	first, err := s.Discover(context.Background(), "/ip/address", "print")
	if err != nil {
		t.Fatalf("Discover #1: %v", err)
	}
	second, err := s.Discover(context.Background(), "/ip/address", "print")
	if err != nil {
		t.Fatalf("Discover #2: %v", err)
	}
	if first != second {
		t.Error("second Discover returned a different schema pointer (cache miss)")
	}
	// One discovery does 4 probes (child path+verb, child path, completion
	// path+verb, completion proplist). The second must add none.
	if got := m.InspectCalls(); got != 4 {
		t.Errorf("InspectCalls after 2x Discover = %d, want 4 (cache hit, no second round-trip)", got)
	}
}

func TestCacheInvalidation(t *testing.T) {
	m := addressPrintFixture()
	s := schema.NewStore(m)
	if _, err := s.Discover(context.Background(), "/ip/address", "print"); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	s.Cache.Delete("/ip/address", "print")
	if _, err := s.Discover(context.Background(), "/ip/address", "print"); err != nil {
		t.Fatalf("Discover after Delete: %v", err)
	}
	if got := m.InspectCalls(); got != 8 {
		t.Errorf("InspectCalls after Delete+re-discover = %d, want 8 (re-probed)", got)
	}
}

func TestCacheTTL(t *testing.T) {
	c := schema.NewCacheWithTTL(time.Millisecond)
	s := &schema.CommandSchema{Path: "/ip/address", Verb: "print"}
	c.Put(schema.SchemaKey("/ip/address", "print"), s)
	if c.Get(schema.SchemaKey("/ip/address", "print")) == nil {
		t.Fatal("Get right after Put: want schema, got nil")
	}
	time.Sleep(5 * time.Millisecond)
	if c.Get(schema.SchemaKey("/ip/address", "print")) != nil {
		t.Error("Get after TTL: want nil (expired)")
	}
}

func TestInspectChildrenOrEmpty(t *testing.T) {
	m := mock.NewStructured()
	m.SetInspectFn(func(request transport.InspectRequestKind, path string) ([]transport.InspectNode, error) {
		if path == "nonsense" {
			return nil, roserr.New(roserr.CodeCommandFailed, "no such command prefix")
		}
		return []transport.InspectNode{{Type: "child", NodeType: "dir", Name: "system"}}, nil
	})
	nodes, err := schema.InspectChildrenOrEmpty(context.Background(), m, []string{"nonsense"})
	if err != nil || nodes != nil {
		t.Errorf("InspectChildrenOrEmpty(nonsense) = %v, %v; want nil, nil (swallowed)", nodes, err)
	}
	nodes, err = schema.InspectChildrenOrEmpty(context.Background(), m, []string{"system"})
	if err != nil || len(nodes) != 1 {
		t.Errorf("InspectChildrenOrEmpty(system) = %v, %v; want 1 node, nil", nodes, err)
	}
}

func equalStrings(a, b []string) bool {
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
