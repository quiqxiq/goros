// Package schema discovers and caches the RouterOS command schema — which
// attributes a command accepts and whether it is a table (print/get) or an
// action command — via /console/inspect (RouterOS 7+). Gate 2 (Fase 4) uses
// it to validate command attributes; the discovery strategy mirrors centrs
// (src/execute.ts inspectExecuteAttributes and src/retrieve.ts). Dependencies:
// transport and roserr only.
package schema

// Category classifies a command's output/validation shape. Explicitly three
// values so a caller can always tell what discovery concluded — an empty
// Attributes slice is not silently treated as "no fields".
type Category string

const (
	// CategoryTable marks print/get-style commands whose fields can be
	// discovered statically (fields are valid attribute names).
	CategoryTable Category = "table"
	// CategoryAction marks commands with input arguments (e.g. /tool/ping);
	// the discovered Attributes are the input arguments. Output exists only
	// when the command runs.
	CategoryAction Category = "action"
	// CategoryUnknown marks commands inspect could not resolve (no inspect
	// support, unknown path, or no children found). Callers skip validation
	// with a capability note rather than fail.
	CategoryUnknown Category = "unknown"
)

// Attribute is one accepted attribute/field of a command. Kept minimal on
// purpose: value types, defaults, and flags are future extensions.
type Attribute struct {
	Name string
}

// CommandSchema is the discovered schema of one RouterOS command
// (Path+Verb). Immutable once stored in a cache.
type CommandSchema struct {
	Path       string
	Verb       string
	Category   Category
	Attributes []Attribute // sorted by Name.
	Source     string      // provenance, e.g. "inspect child+completion", "override".
}

// HasAttribute reports whether name is among the schema's attributes.
func (s *CommandSchema) HasAttribute(name string) bool {
	for _, a := range s.Attributes {
		if a.Name == name {
			return true
		}
	}
	return false
}
