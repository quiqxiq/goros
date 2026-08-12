package schema

import (
	"context"
	"strings"

	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
)

// This file ports the transport-agnostic /console/inspect primitives from
// centrs src/core/inspect.ts 1:1 — pure functions, no I/O. Grounding: the
// request modes mirror tikoci/lsp-routeros-ts; the wire facts (comma-joined
// path, node-type field) are confirmed on CHR 7.23.1 (centrs) and lab 7.21.5
// (docs/RESEARCH.md §11).

// PathTokens splits a slash RouterOS path into menu tokens, dropping the
// leading slash and empties: "/ip/address" -> ["ip", "address"].
func PathTokens(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
}

// InspectPath joins menu tokens into the COMMA form /console/inspect expects
// for its path argument: ["ip", "address"] -> "ip,address". The inspect path
// argument is internally a RouterOS array: a comma string is :toarray-split
// into the menu-walk tokens, whereas a "/"-prefixed command string matches no
// menu (confirmed on CHR 7.23.1 and lab 7.21.5). Callers must pass tokens
// joined by comma, never a slash command.
func InspectPath(tokens []string) string {
	return strings.Join(tokens, ",")
}

// IsArgumentNode reports whether n is an argument (arg) node from
// request=child. RouterOS builds differ in which field carries the node kind
// (older builds: type="arg"; 7.21.5: type="child" + node-type="arg"), so both
// are checked.
func IsArgumentNode(n transport.InspectNode) bool {
	return n.Type == "arg" || n.NodeType == "arg"
}

// IsCommandNode reports whether n is a command (cmd) node with the given
// name (e.g. "print", "get").
func IsCommandNode(n transport.InspectNode, name string) bool {
	return n.Name == name && (n.Type == "cmd" || n.NodeType == "cmd")
}

// ExtractCompletionNames flattens completion rows into candidate names.
// Within one row the name-bearing fields (completion/name/value) are read
// first; "text" is used only as a fallback when none of them is populated,
// because on 7.21.5 "text" carries the human-readable description (e.g.
// "Local IP address") while the token name lives in "completion" (lab
// grounding, RESEARCH.md §11). Values are stripped from the first "=" onward
// (the name=value completion form), trimmed, and blanks dropped. Names are
// returned in row order WITHOUT de-duplication or sorting — callers that need
// a stable set wrap with mergeNames (which uniques) plus sort.
func ExtractCompletionNames(rows []transport.InspectNode) []string {
	var out []string
	for _, r := range rows {
		var picked []string
		for _, v := range []string{r.Completion, r.Name, r.Value} {
			if v != "" {
				picked = append(picked, v)
			}
		}
		if len(picked) == 0 && r.Text != "" {
			picked = append(picked, r.Text)
		}
		for _, v := range picked {
			if eq := strings.IndexByte(v, '='); eq != -1 {
				v = v[:eq]
			}
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// InspectChildrenOrEmpty issues request=child for a token path and swallows
// the grounded not-found failures (unknown-path, command-failed) to an empty
// list, so the caller can classify the absence itself. Use it for existence
// probes only — NOT for attribute discovery, where a trap is a real failure
// and Discover surfaces it.
func InspectChildrenOrEmpty(ctx context.Context, t transport.StructuredTransport, tokens []string) ([]transport.InspectNode, error) {
	nodes, err := t.Inspect(ctx, transport.InspectChild, InspectPath(tokens))
	if err != nil {
		if isSwallowableInspectErr(err) {
			return nil, nil
		}
		return nil, err
	}
	return nodes, nil
}

// isSwallowableInspectErr reports whether an Inspect error means "feature or
// path absent" rather than a real failure. Only the grounded not-found codes
// are swallowed — unknown-path (classified) and command-failed (legacy
// catch-all) — mirroring centrs inspectChildrenOrEmpty.
func isSwallowableInspectErr(err error) bool {
	return roserr.IsCode(err, roserr.CodeUnknownPath) ||
		roserr.IsCode(err, roserr.CodeCommandFailed)
}
