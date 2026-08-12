// Console-transport wiring for Gate 1 (D-016/D-017): lets the exact same
// Gate1 and PureSyntaxClassifier used by native-api run over a
// transport.ConsoleTransport, rendering the space-separated CLI form that
// RouterOS 6 SSH exec requires (R12). No classifier is duplicated — the
// console adapter only translates Run output into the canonical Reply the
// gate reads.
package gate

import (
	"context"

	"github.com/quiqxiq/goros/v4/transport"
)

// consoleCommandTransport adapts a transport.ConsoleTransport to the minimal
// CommandTransport seam Gate 1 needs (D-016): a Command{Script: s} is
// executed via Run(ctx, s) and the output is wrapped as a canonical Reply
// whose "ret" attribute carries the text — exactly what Gate 1's classifier
// reads (rep.Ret()).
type consoleCommandTransport struct {
	ct transport.ConsoleTransport
}

var _ CommandTransport = (*consoleCommandTransport)(nil)

// NewConsoleCommand returns the console adapter for Gate 1 (D-016): one
// canonical command in, one canonical reply out.
func NewConsoleCommand(ct transport.ConsoleTransport) CommandTransport {
	return &consoleCommandTransport{ct: ct}
}

func (a *consoleCommandTransport) Command(ctx context.Context, cmd *transport.Command) (*transport.Reply, error) {
	line := cmd.Script
	if line == "" {
		line = cmd.ConsoleCLI()
	}
	out, err := a.ct.Run(ctx, line)
	if err != nil {
		return nil, err
	}
	return &transport.Reply{Type: transport.ReplyRe, Attributes: map[string]string{"ret": out}}, nil
}

// NewConsoleGate returns a Gate1 pre-wired for a console transport (D-016):
// the console adapter as the transport and ConsoleCLI as the renderer
// (space-separated path — RouterOS 6 SSH exec, R12). supportsParse may be
// nil ("assume supported": the console :parse path works on v6 and v7,
// RESEARCH.md §15).
func NewConsoleGate(ct transport.ConsoleTransport, supportsParse func() bool) *Gate1 {
	return &Gate1{
		Transport:     NewConsoleCommand(ct),
		RenderCLI:     func(cmd *transport.Command) string { return cmd.ConsoleCLI() },
		SupportsParse: supportsParse,
	}
}

// ValidateConsole validates one raw console line over a console transport
// using the device's own parser (D-017): unbalanced quotes fail locally, the
// line is embedded in `:put [:parse "..."]`, run, and classified with the
// same PureSyntaxClassifier as native-api. Returns nil when the device
// accepts the line, or validation/syntax / validation/unknown-attribute.
// Read-only — line is never executed.
func ValidateConsole(ctx context.Context, ct transport.ConsoleTransport, line string) error {
	return NewConsoleGate(ct, nil).Run(ctx, &transport.Command{Script: line})
}
