// Package roserr provides the structured error taxonomy shared by all goros
// packages: transport, gate, schema, and the root facade.
//
// The taxonomy is inspired by the error catalog of the centrs reference
// project (.refrences/centrs/src/core/routeros-errors.ts and
// error-catalog.ts): every error carries a stable machine-readable code, a
// human summary, an optional remediation hint, and structured context. The
// Code is the semantic identity of an error — compare with errors.Is or
// IsCode, never string-match the rendered message.
//
// Dependency rule: this package must not import any other goros package, so
// wire, transport, gate, and schema can all return the same error types
// without an import cycle.
package roserr

import (
	"errors"
	"fmt"
	"strings"
)

// Code is the stable, machine-readable identity of an error. Values are
// grouped by domain with a slash prefix: "routeros/*" (device-level failures),
// "validation/*" (gate results), "auth/*" (authentication), "transport/*"
// (connection/protocol plumbing).
type Code string

const (
	// RouterOS device-level failures (traps, unknown commands/attributes).
	CodeUnknownPath      Code = "routeros/unknown-path"
	CodeUnknownAttribute Code = "routeros/unknown-attribute"
	CodeInvalidValue     Code = "routeros/invalid-value"
	CodeCommandFailed    Code = "routeros/command-failed"
	CodeSessionClosed    Code = "routeros/session-closed"

	// Validation gate results.
	CodeValidationSyntax           Code = "validation/syntax"
	CodeValidationUnknownAttribute Code = "validation/unknown-attribute"

	// Authentication failures (any transport).
	CodeAuthFailed Code = "auth/failed"

	// Transport-level failures (any transport).
	CodeConnectionRefused     Code = "transport/connection-refused"
	CodeTimeout               Code = "transport/timeout"
	CodeDNS                   Code = "transport/dns"
	CodeNetwork               Code = "transport/network"
	CodeTLSCertificate        Code = "transport/tls-certificate"
	CodeHostKeyMismatch       Code = "transport/host-key-mismatch"
	CodeCapabilityUnsupported Code = "transport/capability-unsupported"
)

// Context carries structured details about where an error occurred. All fields
// are optional; zero values are omitted from the rendered message.
type Context struct {
	Via   string         // transport identifier, e.g. "native-api", "ssh".
	Host  string         // remote host or address.
	Port  int            // remote port.
	Path  string         // RouterOS path the error relates to, if any.
	Extra map[string]any // transport- or phase-specific details (status, exit code, ...).
}

// Error is the structured error used across all goros packages.
type Error struct {
	Code        Code
	Summary     string
	Remediation string
	Context     Context
	Cause       error
}

// Option configures an Error at construction time.
type Option func(*Error)

// WithRemediation sets a human hint for fixing the underlying problem.
func WithRemediation(s string) Option { return func(e *Error) { e.Remediation = s } }

// WithContext attaches structured context (via/host/port/path/extra).
func WithContext(c Context) Option { return func(e *Error) { e.Context = c } }

// WithCause wraps an underlying error, making it reachable via errors.Unwrap.
func WithCause(err error) Option { return func(e *Error) { e.Cause = err } }

// New builds a structured Error. summary must be a complete, standalone
// sentence; remediation, context, and cause are optional.
func New(code Code, summary string, opts ...Option) *Error {
	e := &Error{Code: code, Summary: summary}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Error renders the human-readable form: "code: summary [via=...] [host:port]
// [path=...]; remediation: ...". The Code field remains the machine-readable
// identity; never parse this string.
func (e *Error) Error() string {
	var sb strings.Builder
	sb.WriteString(string(e.Code))
	sb.WriteString(": ")
	sb.WriteString(e.Summary)
	if e.Context.Via != "" {
		sb.WriteString(" [via=")
		sb.WriteString(e.Context.Via)
		sb.WriteString("]")
	}
	if e.Context.Host != "" {
		sb.WriteString(" [host=")
		sb.WriteString(e.Context.Host)
		if e.Context.Port != 0 {
			fmt.Fprintf(&sb, ":%d", e.Context.Port)
		}
		sb.WriteString("]")
	}
	if e.Context.Path != "" {
		sb.WriteString(" [path=")
		sb.WriteString(e.Context.Path)
		sb.WriteString("]")
	}
	if e.Remediation != "" {
		sb.WriteString("; remediation: ")
		sb.WriteString(e.Remediation)
	}
	return sb.String()
}

// Unwrap returns the wrapped cause, if any, so errors.Is/errors.As reach it.
func (e *Error) Unwrap() error { return e.Cause }

// Is reports whether target is a *Error with the same Code. Codes are the
// semantic identity of errors in this taxonomy, so matching is by Code rather
// than by pointer or message text.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// IsCode reports whether err (or any error in its Unwrap chain) is a roserr.Error
// carrying the given code. Built on errors.Is and Error.Is, it walks the whole
// chain — including errors.Join branches — and matches by Code, the semantic
// identity of an error, never by text. This is the safe way to switch on error
// category without parsing the rendered message.
func IsCode(err error, code Code) bool {
	return errors.Is(err, &Error{Code: code})
}

// ContextOf returns the structured Context of the first roserr.Error found in
// err's Unwrap chain, and whether one was found. Callers use it to read
// via/host/port/path details (for remediation or logging) without parsing the
// rendered message.
func ContextOf(err error) (Context, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Context, true
	}
	return Context{}, false
}
