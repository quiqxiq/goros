package gate

import (
	"context"
	"sort"
	"strings"

	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/schema"
	"github.com/quiqxiq/goros/v4/transport"
)

// Gate2 validates a command's attributes against the discovered
// /console/inspect schema (Fase 4). On sessions without inspect support it
// skips silently — the documented v6 degradation, never a per-command error
// (docs/PLAN-FASE3-FASE4.md §3.5).
type Gate2 struct {
	Schema *schema.Store
	// SupportsInspect reports whether this session serves /console/inspect
	// (probed once after login, e.g. adapter.SupportsInspect). Nil means
	// "assume supported". When it returns false, Validate skips.
	SupportsInspect func() bool
}

// Validate checks that every attribute key of cmd is known to the schema for
// (cmd.Path, cmd.Verb). It returns nil when all keys are known, or when
// validation cannot apply (unsupported session, unknown category). Otherwise
// it returns a CodeValidationUnknownAttribute error carrying the missing and
// available names plus the validation source in the context.
func (g *Gate2) Validate(ctx context.Context, cmd *transport.Command) error {
	if g.SupportsInspect != nil && !g.SupportsInspect() {
		// Session cannot inspect (v6): skip, not error.
		return nil
	}
	sch, err := g.Schema.Discover(ctx, cmd.Path, cmd.Verb)
	if err != nil {
		return err
	}
	if sch.Category == schema.CategoryUnknown {
		// Inspect could not resolve the command: skip with a capability
		// note rather than fail.
		return nil
	}

	var missing []string
	for k := range cmd.Attributes {
		if !sch.HasAttribute(k) {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)

	available := make([]string, 0, len(sch.Attributes))
	for _, a := range sch.Attributes {
		available = append(available, a.Name)
	}
	return roserr.New(
		roserr.CodeValidationUnknownAttribute,
		"unknown attribute(s): "+strings.Join(missing, ", "),
		roserr.WithContext(roserr.Context{
			Via:  "gate2",
			Path: cmd.Path,
			Extra: map[string]any{
				"path":             cmd.Path,
				"verb":             cmd.Verb,
				"missing":          missing,
				"available":        available,
				"validationSource": sch.Source,
			},
		}),
	)
}
