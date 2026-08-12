package schema

import (
	"context"
	"sort"

	"github.com/quiqxiq/goros/v4/transport"
)

// Store discovers, categorizes, and caches CommandSchema for one transport
// session. Create with NewStore; register manual category overrides with
// RegisterCategory.
type Store struct {
	Transport transport.StructuredTransport
	Cache     *Cache
	overrides map[string]Category
}

// NewStore returns a Store over t with a fresh cache and no overrides.
func NewStore(t transport.StructuredTransport) *Store {
	return &Store{
		Transport: t,
		Cache:     NewCache(),
		overrides: make(map[string]Category),
	}
}

// RegisterCategory overrides the discovered Category for one (path, verb)
// command; the override wins over discovery (D-010). Callers use it when
// inspect is unavailable or wrong for a specific command. It also evicts any
// cached schema for the command so the override takes effect immediately,
// not after the TTL expires.
func (s *Store) RegisterCategory(path, verb string, c Category) {
	s.overrides[SchemaKey(path, verb)] = c
	s.Cache.Delete(path, verb)
}

// Discover resolves the schema for command (path, verb) with the union
// strategy of centrs (execute.ts inspectExecuteAttributes + retrieve.ts):
//
//  1. request=child on path+verb   -> argument nodes of the command.
//  2. request=child on path        -> command nodes, to detect print/get.
//  3. request=completion on path+verb -> extra argument names (usually noise;
//     the real field list comes from step 4).
//  4. Field discovery trick: when the verb is print, request=completion on
//     path+verb+".proplist"; when it is get, on path+verb+".value-name".
//  5. Union + dedup + sort all names; derive the Category per verb; apply any
//     registered override.
//
// Results are cached by (path, verb); a repeated Discover does not
// round-trip (verified by call-count tests, M20).
//
// Errors: a not-found trap on the child probe yields a CategoryUnknown schema
// without error — that is the documented "inspect could not resolve" state,
// not a failure. Non-trap failures propagate.
func (s *Store) Discover(ctx context.Context, path, verb string) (*CommandSchema, error) {
	key := SchemaKey(path, verb)
	if cached := s.Cache.Get(key); cached != nil {
		return cached, nil
	}

	pathTokens := PathTokens(path)
	// Defensive copy: cmdTokens must never alias pathTokens' backing array
	// (a later append to one would corrupt the other).
	cmdTokens := append(append([]string{}, pathTokens...), verb)

	// 1. Argument nodes of the command itself (path+verb).
	cmdChildren, err := s.Transport.Inspect(ctx, transport.InspectChild, InspectPath(cmdTokens))
	if err != nil {
		if isSwallowableInspectErr(err) {
			return s.put(key, &CommandSchema{
				Path: path, Verb: verb, Category: CategoryUnknown, Source: "inspect-failed",
			}), nil
		}
		return nil, err
	}
	attrs := namesOfArgumentNodes(cmdChildren)

	// 2. Command nodes at the path level (to detect print/get and the verb).
	pathChildren, err := s.Transport.Inspect(ctx, transport.InspectChild, InspectPath(pathTokens))
	if err != nil && !isSwallowableInspectErr(err) {
		return nil, err
	}

	// 3. Completion on path+verb: extra argument names (filtered; on 7.21.5
	//    this row is usually structural noise).
	completionRows, err := s.Transport.Inspect(ctx, transport.InspectCompletion, InspectPath(cmdTokens))
	if err == nil {
		attrs = mergeNames(attrs, usableCompletionNames(ExtractCompletionNames(completionRows)))
	} else if !isSwallowableInspectErr(err) {
		return nil, err
	}

	// 4. Field discovery trick (retrieve.ts): .proplist for print-style
	//    commands, value-name for singleton get-style commands.
	if fieldArg := fieldDiscoveryArgument(verb, pathChildren); fieldArg != "" {
		fieldTokens := append(append([]string{}, pathTokens...), verb, fieldArg)
		rows, err := s.Transport.Inspect(ctx, transport.InspectCompletion, InspectPath(fieldTokens))
		if err == nil {
			attrs = mergeNames(attrs, usableCompletionNames(ExtractCompletionNames(rows)))
		} else if !isSwallowableInspectErr(err) {
			return nil, err
		}
	}

	// 5. Category per verb, then override.
	cat := CategoryUnknown
	if hasCommandNode(pathChildren, verb) {
		switch verb {
		case "print", "get":
			cat = CategoryTable
		default:
			cat = CategoryAction
		}
	}
	if c, ok := s.overrides[key]; ok {
		cat = c
	}

	sch := &CommandSchema{
		Path:       path,
		Verb:       verb,
		Category:   cat,
		Attributes: sortedAttributes(attrs),
		Source:     "inspect child+completion",
	}
	return s.put(key, sch), nil
}

// fieldDiscoveryArgument picks the completion argument that yields field
// names for the given verb: ".proplist" when the verb is a supported print,
// "value-name" when it is a supported get (centrs retrieve.ts L1012). Empty
// for action verbs — their input arguments come from the child probe only.
func fieldDiscoveryArgument(verb string, pathChildren []transport.InspectNode) string {
	switch verb {
	case "print":
		if hasCommandNode(pathChildren, "print") {
			return "proplist"
		}
	case "get":
		if hasCommandNode(pathChildren, "get") {
			return "value-name"
		}
	}
	return ""
}

func (s *Store) put(key string, sch *CommandSchema) *CommandSchema {
	s.Cache.Put(key, sch)
	return sch
}

func hasCommandNode(nodes []transport.InspectNode, name string) bool {
	for _, n := range nodes {
		if IsCommandNode(n, name) {
			return true
		}
	}
	return false
}

// namesOfArgumentNodes collects non-empty names of argument nodes.
func namesOfArgumentNodes(nodes []transport.InspectNode) []string {
	var out []string
	for _, n := range nodes {
		if IsArgumentNode(n) && n.Name != "" {
			out = append(out, n.Name)
		}
	}
	return out
}

// mergeNames appends src names not already present, preserving first-seen
// order (dedup + union in one pass).
func mergeNames(dst, src []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, n := range dst {
		seen[n] = true
	}
	for _, n := range src {
		if !seen[n] {
			seen[n] = true
			dst = append(dst, n)
		}
	}
	return dst
}

// structuralCompletionTokens are the garbage tokens RouterOS 7.21.5 emits in
// request=completion rows alongside real names (lab-verified, RESEARCH.md
// §11): array/macro punctuation, the id-prefix "*", the "<value>" literal
// placeholder, and "about".
var structuralCompletionTokens = map[string]bool{
	"[": true, "]": true, "(": true, ")": true, "$": true,
	`"`: true, "*": true, "<value>": true, "about": true, "`": true,
}

// usableCompletionNames drops the lab-observed structural tokens before a
// completion list is merged into the schema.
func usableCompletionNames(names []string) []string {
	var out []string
	for _, n := range names {
		if !structuralCompletionTokens[n] {
			out = append(out, n)
		}
	}
	return out
}

// sortedAttributes maps a unique name list to sorted Attribute values.
func sortedAttributes(names []string) []Attribute {
	out := make([]Attribute, len(names))
	for i, n := range names {
		out[i] = Attribute{Name: n}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
