// Package transport defines the canonical contract every goros transport
// speaks: the structured and console seams, the canonical Reply and Command
// types, and the capability matrix. Validation, discovery, and schema layers
// depend on this package — never on a concrete transport — so a gate written
// once works across native-api, ssh, mac-telnet, and rest.
//
// The shape follows the ProtocolAdapter seam of the centrs reference project
// (.refrences/centrs/src/protocols/adapter.ts), adapted to Go idioms. Unlike
// centrs, which distinguishes adapter *capabilities* at runtime, goros splits
// the contract into two typed seams — StructuredTransport and
// ConsoleTransport — and reports capabilities for the orchestrator.
package transport

import (
	"context"
	"sort"
	"strings"
)

// ReplyType identifies the type of a RouterOS reply sentence. Values mirror
// the sentence words already defined by the native-api implementation.
type ReplyType string

const (
	ReplyRe    ReplyType = "!re"    // a data record.
	ReplyDone  ReplyType = "!done"  // command completed successfully.
	ReplyTrap  ReplyType = "!trap"  // command failed; device returned an error.
	ReplyFatal ReplyType = "!fatal" // session is dead; all pending commands fail.
	ReplyEmpty ReplyType = "!empty" // RouterOS 7.18+; no data (e.g. empty listen stream).
)

// Reply is the canonical, transport-agnostic form of one RouterOS reply. Every
// transport must translate its raw result into this shape before returning it
// to callers.
type Reply struct {
	Type       ReplyType
	Attributes map[string]string // =key=value pairs.
	Tag        string            // echo of the request tag; may be empty.
	RawWords   []string          // raw sentence words, for debug; may be empty.
}

// Ret returns the value of the "ret" attribute (used, among others, for
// `:parse` results and login challenges), or "" when absent.
func (r *Reply) Ret() string { return r.Attributes["ret"] }

// Message returns the value of the "message" attribute (device error text on
// !trap/!fatal), or "" when absent.
func (r *Reply) Message() string { return r.Attributes["message"] }

// Command is the canonical, transport-agnostic form of a RouterOS command.
//
// For structured transports (native-api, rest) the command is sent as its own
// sentence: Path+Verb as the command word, Attributes as =key=value words,
// Queries as ?-words, Proplist as =.proplist=. For console transports (ssh,
// mac-telnet) the transport renders CLI() and runs it as one console line.
type Command struct {
	Path       string            // slash-prefixed menu path, e.g. "/ip/address".
	Verb       string            // command verb, e.g. "print", "add", "set".
	Attributes map[string]string // command attributes (=key=value).
	Queries    []string          // RouterOS query words, WITHOUT the leading "?".
	Proplist   []string          // field projection (.proplist).
	Script     string            // raw CLI line; when set, Path/Verb/Attributes are ignored.
}

// PathTokens splits Path into menu tokens, dropping the leading slash and
// empties: "/ip/address" -> ["ip", "address"].
func (c *Command) PathTokens() []string {
	return strings.FieldsFunc(c.Path, func(r rune) bool { return r == '/' })
}

// CLI renders the canonical RouterOS console line for this command. It is used
// by console transports and by the Gate 1 `:parse` script. Values containing
// whitespace or a double quote are double-quoted (inner quotes escaped);
// precise escaping for embedding inside a RouterOS *string literal* is handled
// by the gate layer. Attributes are rendered in sorted key order so the output
// is deterministic.
func (c *Command) CLI() string {
	if c.Script != "" {
		return c.Script
	}
	var sb strings.Builder
	sb.WriteString(c.Path)
	if c.Verb != "" {
		sb.WriteString("/")
		sb.WriteString(c.Verb)
	}
	for _, k := range sortedKeys(c.Attributes) {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(quoteCLIValue(c.Attributes[k]))
	}
	for _, q := range c.Queries {
		sb.WriteString(" ?")
		sb.WriteString(q)
	}
	if len(c.Proplist) > 0 {
		sb.WriteString(" .proplist=")
		sb.WriteString(strings.Join(c.Proplist, ","))
	}
	return sb.String()
}

// ConsoleCLI renders the space-separated console form of the command —
// "/ip address print interface=ether1" — which every RouterOS build accepts
// over SSH exec. It differs from CLI() only in the path form: CLI()
// slash-joins the menu tokens ("/ip/address/print"), while ConsoleCLI joins
// them with spaces. RouterOS 6 SSH exec rejects the slash-joined form and
// accepts only the space form; RouterOS 7 accepts both (verified in the lab,
// R12 — docs/RESEARCH.md §15). Attributes render exactly like CLI(): sorted
// keys, values quoted when they contain whitespace or a quote. A Script
// command is returned verbatim, like CLI().
func (c *Command) ConsoleCLI() string {
	if c.Script != "" {
		return c.Script
	}
	var sb strings.Builder
	sb.WriteString("/")
	sb.WriteString(strings.Join(c.PathTokens(), " "))
	if c.Verb != "" {
		sb.WriteString(" ")
		sb.WriteString(c.Verb)
	}
	for _, k := range sortedKeys(c.Attributes) {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(quoteCLIValue(c.Attributes[k]))
	}
	for _, q := range c.Queries {
		sb.WriteString(" ?")
		sb.WriteString(q)
	}
	if len(c.Proplist) > 0 {
		sb.WriteString(" .proplist=")
		sb.WriteString(strings.Join(c.Proplist, ","))
	}
	return sb.String()
}

// Words renders the structured native-api sentence words for this command:
// the command word (Path/Verb), then =key=value attribute words in sorted key
// order, then ?query words, then the .proplist=... word. A Script command
// renders the /execute form — "/execute", "=script=<script>", "=as-string="
// (empty value) — matching how the centrs reference project executes scripts
// (executeScript in src/protocols/native-api.ts). Structured transports
// (native-api, rest) send exactly these words as their own sentence.
func (c *Command) Words() []string {
	if c.Script != "" {
		return []string{"/execute", "=script=" + c.Script, "=as-string="}
	}
	cmdWord := c.Path
	if c.Verb != "" {
		cmdWord = strings.TrimRight(c.Path, "/") + "/" + c.Verb
	}
	words := []string{cmdWord}
	for _, k := range sortedKeys(c.Attributes) {
		words = append(words, "="+k+"="+c.Attributes[k])
	}
	for _, q := range c.Queries {
		words = append(words, "?"+q)
	}
	if len(c.Proplist) > 0 {
		words = append(words, ".proplist="+strings.Join(c.Proplist, ","))
	}
	return words
}

// sortedKeys returns the keys of m in sorted order for deterministic output.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// quoteCLIValue wraps a value in double quotes when it contains whitespace or
// a quote character, escaping inner quotes with a backslash.
func quoteCLIValue(v string) string {
	if !strings.ContainsAny(v, " \t\"") {
		return v
	}
	return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
}

// Capabilities describes which seams a transport supports. Validation layers
// use it to decide which gates apply and how; a transport must refuse
// unsupported operations with CodeCapabilityUnsupported rather than fail
// silently.
type Capabilities struct {
	Structured bool // sends structured commands (native-api, rest).
	Console    bool // runs raw console lines (ssh, mac-telnet).
	Inspect    bool // supports /console/inspect (native-api, rest).
}

// Transport is the common seam: capability reporting and resource release.
// Every concrete transport implements it plus StructuredTransport and/or
// ConsoleTransport, depending on its capabilities.
type Transport interface {
	// Capabilities reports which seams this transport supports.
	Capabilities() Capabilities
	// Close releases the underlying connection. Safe to call when never
	// connected; idempotent.
	Close() error
}

// StructuredTransport sends structured commands and /console/inspect probes.
// Implemented by native-api and rest.
type StructuredTransport interface {
	Transport
	// Command sends one structured command and returns the canonical reply
	// (the terminal !done, or the !trap/!fatal error). Honors ctx
	// cancellation and timeout.
	Command(ctx context.Context, cmd *Command) (*Reply, error)
	// Inspect issues a /console/inspect probe (request=child|completion) and
	// returns the raw node rows. Path must be in comma-joined form (see
	// schema.InspectPath); transports must not reinterpret it.
	Inspect(ctx context.Context, request InspectRequestKind, path string) ([]InspectNode, error)
}

// ConsoleTransport runs raw RouterOS console lines and returns the captured
// text output. Implemented by ssh and mac-telnet. Validation on console
// transports rides the single ":put [:parse ...]" gate.
type ConsoleTransport interface {
	Transport
	// Run executes one console line and returns the device's text output.
	Run(ctx context.Context, line string) (string, error)
}

// InspectRequestKind is the /console/inspect request mode. Only "child" and
// "completion" are wired; "highlight" and "syntax" are a documented extension
// point (out of scope).
type InspectRequestKind string

const (
	InspectChild      InspectRequestKind = "child"
	InspectCompletion InspectRequestKind = "completion"
)

// InspectNode is one raw row returned by /console/inspect. Fields mirror the
// shapes RouterOS fills for request=child and request=completion; the schema
// layer (Fase 4) interprets them.
type InspectNode struct {
	Type       string // "arg", "cmd", ...
	NodeType   string // alias field some builds populate ("node-type").
	Name       string // node name.
	Completion string // completion-form value (request=completion).
	Value      string // value-form value (request=completion).
	Text       string // text-form value (request=completion).
}
