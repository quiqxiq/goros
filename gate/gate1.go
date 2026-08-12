// Package gate implements the validation gates prescribed by PLAN.md: Gate 1
// (Fase 3) validates command syntax by running the device's own parser via
// `:put [:parse ...]`; Gate 2 (Fase 4) validates attributes against the
// discovered /console/inspect schema. Gates depend only on transport, schema,
// and roserr — never on a concrete transport — so they run unchanged over
// native-api, rest, ssh, and mac-telnet.
package gate

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
)

// StringLiteral renders s as a RouterOS double-quoted string literal for
// embedding in a script line: the value is wrapped in quotes, backslashes are
// escaped first, then double quotes (order matters). Mirrors
// routerOsStringLiteral in centrs (src/execute.ts L1431).
func StringLiteral(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// HasUnbalancedQuotes reports whether s contains an odd number of unescaped
// double or single quotes — the local preflight that fails a Gate 1 command
// without a round-trip (a quote imbalance would corrupt the script
// embedding). Backslash-escaped quotes are honored. Mirrors
// hasUnbalancedQuotes in centrs (src/execute.ts).
func HasUnbalancedQuotes(s string) bool {
	escaped := false
	dq, sq := 0, 0
	for _, r := range s {
		if escaped {
			escaped = false
			continue
		}
		switch r {
		case '\\':
			escaped = true
		case '"':
			dq++
		case '\'':
			sq++
		}
	}
	return dq%2 == 1 || sq%2 == 1
}

// ClassifyResult is the pure outcome of classifying one :parse output string.
type ClassifyResult struct {
	Valid     bool        // true when the command parsed cleanly (or the message is unrecognized).
	Code      roserr.Code // CodeValidationSyntax or CodeValidationUnknownAttribute when !Valid.
	Attribute string      // attribute name when classified as unknown-attribute.
	Line, Col int         // 1-based error position; 0 when the message carries none.
	Message   string      // the original ret text, verbatim.
}

// Regexes for the Gate 1 classifier, in strict match order (see Classify).
//
// Facts (lab-verified, docs/RESEARCH.md §8/§13): on RouterOS 7.21.5 a
// successful :parse echoes the evaluated expression wrapped as "(evl ...)",
// while parse failures are echoed bare — "syntax error (line 1 column 10)"
// for slash-form unknown commands and "expected end of command (line 1
// column 24)" for unknown attributes — or, when the failing command sits
// inside a script expression, wrapped as "(<% bad command name nonsense
// (line 1 column 2) ...)" (R11, RESEARCH.md §13). The centrs corpus
// additionally shows "bad parameter <name>" both bare (native-api) and
// wrapped inside the "(evl ...)" echo (console). Multiline mode ((?m)) is on
// so console transports (Fase 7) whose output carries a prompt or leading
// lines still match — centrs uses the same flags.
var (
	// Wrapped "bad parameter": console transports echo the :put message
	// inside the evaluated form, e.g. "(evl bad parameter bogus (line ...))".
	// Anchored to a line start: the ret string is the whole :parse output, so
	// "^" (with (?m)) matches the observed shapes, and anchoring prevents a
	// false positive when a VALID command's quoted attribute value contains
	// error-looking text mid-line (the valid echo line starts with "(evl").
	wrappedBadParamRe = regexp.MustCompile(`(?mi)^[ \t]*\(evl\s+bad parameter\s+(\S+)`)
	// Wrapped syntax failures: inside a script expression, :parse returns the
	// error wrapped as "(<% bad command name nonsense (line 1 column 2)
	// nonsense;command)" instead of a bare line-anchored message (R11). The
	// "(<% " prefix is the discriminator — valid :parse output is "(evl ...)".
	// Anchored like the other wrapped pattern, for the same false-positive
	// reason. (A "(<% bad parameter X" wrapped form has not been observed in
	// the lab — centrs shows bad-parameter only as "(evl bad parameter" — so
	// it is intentionally not matched rather than guessed.)
	wrappedSyntaxRe = regexp.MustCompile(`(?mi)^[ \t]*\(<%[^()]*(?:syntax error|bad command name|expected[^()]*)`)
	// Line-anchored "bad parameter <name>".
	badParamRe = regexp.MustCompile(`(?mi)^[ \t]*bad parameter\s+(\S+)`)
	// Line-anchored syntax failures, with or without position. Anchoring
	// before the "(evl" validity fallthrough is what keeps wrapped errors
	// from passing as valid.
	syntaxRe = regexp.MustCompile(`(?mi)^[ \t]*(?:syntax error|bad command name|expected[^()\n]*)`)
	// Position fragment "(line N column M)".
	lineColRe = regexp.MustCompile(`\(line (\d+) column (\d+)\)`)
)

// PureSyntaxClassifier classifies one :parse output string without any I/O.
// It is shared by native-api (Fase 3) and the console transports (Fase 7) —
// never duplicated (PLAN.md §7).
type PureSyntaxClassifier struct{}

// Classify maps ret (the raw :parse result) to a ClassifyResult. Match order
// is mandatory and grounded in the centrs classifyParseResult grammar plus
// the 7.21.5 lab corpus:
//
//  1. "(evl bad parameter <name> ...)" (wrapped)  -> unknown-attribute
//  2. "(<% ... syntax error|bad command name|expected ...)" (wrapped, R11) -> validation/syntax
//  3. "^bad parameter <name>"                     -> unknown-attribute
//  4. "^syntax error | ^bad command name | ^expected ..." -> validation/syntax
//  5. "(evl ...)"                                 -> valid (the fallthrough)
//  6. anything unrecognized                       -> valid (defensive)
//
// The wrapped-error patterns MUST be checked before the generic "(evl ...)"
// validity fallthrough, or a console transport would let bad parameters pass.
func (PureSyntaxClassifier) Classify(ret string) *ClassifyResult {
	res := &ClassifyResult{Message: ret}
	if m := wrappedBadParamRe.FindStringSubmatch(ret); m != nil {
		res.Code = roserr.CodeValidationUnknownAttribute
		res.Attribute = m[1]
		applyPosition(res, ret)
		return res
	}
	if wrappedSyntaxRe.MatchString(ret) {
		res.Code = roserr.CodeValidationSyntax
		applyPosition(res, ret)
		return res
	}
	if m := badParamRe.FindStringSubmatch(ret); m != nil {
		res.Code = roserr.CodeValidationUnknownAttribute
		res.Attribute = m[1]
		applyPosition(res, ret)
		return res
	}
	if syntaxRe.MatchString(ret) {
		res.Code = roserr.CodeValidationSyntax
		applyPosition(res, ret)
		return res
	}
	// "(evl ...)" and any unrecognized text pass. Never false-positive on
	// unfamiliar messages: an unknown format is safer to let through than
	// to reject a valid command.
	res.Valid = true
	return res
}

func applyPosition(res *ClassifyResult, text string) {
	if m := lineColRe.FindStringSubmatch(text); m != nil {
		res.Line, _ = strconv.Atoi(m[1])
		res.Col, _ = strconv.Atoi(m[2])
	}
}

// CommandTransport is the minimal seam Gate 1 needs (D-016): one canonical
// command in, one canonical reply out. Structured transports satisfy it with
// their StructuredTransport.Command; console transports via the console
// adapter (NewConsoleCommand). Gate 1 never inspects, so the seam stops at
// Command — narrowing the field from transport.StructuredTransport keeps the
// gate usable over any transport without faking Inspect.
type CommandTransport interface {
	Command(ctx context.Context, cmd *transport.Command) (*transport.Reply, error)
}

// Gate1 validates command syntax by running the device's own parser:
// `:put [:parse <literal>]` over the transport's script path (/execute with
// =as-string= on native-api). Transport-agnostic: native-api and rest send
// the script via Command{Script: ...}; console transports route it through
// NewConsoleCommand. The classifier is a pure function reused verbatim by
// both seams (Fase 7) — never duplicated.
type Gate1 struct {
	// Transport is the minimal command seam (D-016). Native-api/rest wire
	// their StructuredTransport directly; console transports wire the
	// console adapter (NewConsoleCommand).
	Transport CommandTransport
	// RenderCLI renders the CLI form embedded in the :parse script. Nil
	// means cmd.CLI() (slash-joined path — native-api). Console transports
	// wire this to cmd.ConsoleCLI() (space-separated path — required by
	// RouterOS 6 SSH exec, R12; see NewConsoleGate).
	RenderCLI func(*transport.Command) string
	// SupportsParse reports whether this session can run the :parse script
	// path (probed once after login). Nil means "assume supported". When it
	// returns false, Run skips silently — the documented v6 degradation
	// (docs/PLAN-FASE3-FASE4.md §2.3), never a per-command error.
	SupportsParse func() bool
}

// Run validates cmd and returns nil when the device parser accepts it, or a
// roserr error (validation/syntax or validation/unknown-attribute) when it
// rejects it. A script/transport-level failure is returned unchanged — never
// relabeled as a validation result, mirroring centrs isPreflightTransportError.
func (g *Gate1) Run(ctx context.Context, cmd *transport.Command) error {
	if g.SupportsParse != nil && !g.SupportsParse() {
		// Session cannot run the :parse script path (v6): skip, not error.
		return nil
	}
	cli := cmd.CLI()
	if g.RenderCLI != nil {
		cli = g.RenderCLI(cmd)
	}
	if HasUnbalancedQuotes(cli) {
		return roserr.New(
			roserr.CodeValidationSyntax,
			"command contains unbalanced quotes",
			roserr.WithContext(roserr.Context{Via: "gate1", Path: cmd.Path}),
		)
	}
	script := ":put [:parse " + StringLiteral(cli) + "]"
	rep, err := g.Transport.Command(ctx, &transport.Command{Script: script})
	if err != nil {
		return err
	}
	res := (PureSyntaxClassifier{}).Classify(rep.Ret())
	if res.Valid {
		return nil
	}
	c := roserr.Context{Via: "gate1", Path: cmd.Path}
	extra := map[string]any{}
	if res.Attribute != "" {
		extra["attribute"] = res.Attribute
	}
	if res.Line != 0 || res.Col != 0 {
		extra["line"] = res.Line
		extra["col"] = res.Col
	}
	if len(extra) > 0 {
		c.Extra = extra
	}
	return roserr.New(res.Code, res.Message, roserr.WithContext(c))
}
