package roserr

import (
	"errors"
	"strings"
	"testing"
)

func TestNewAndRender(t *testing.T) {
	err := New(
		CodeUnknownAttribute,
		"attribute does not exist",
		WithRemediation("check the attribute name"),
		WithContext(Context{Via: "native-api", Host: "10.0.0.1", Port: 8728, Path: "/ip/address"}),
	)
	got := err.Error()
	for _, want := range []string{
		"routeros/unknown-attribute",
		"attribute does not exist",
		"[via=native-api]",
		"[host=10.0.0.1:8728]",
		"[path=/ip/address]",
		"remediation: check the attribute name",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, want it to contain %q", got, want)
		}
	}
	if err.Code != CodeUnknownAttribute {
		t.Errorf("Code = %q, want %q", err.Code, CodeUnknownAttribute)
	}
}

func TestIsCodeAndErrorsIs(t *testing.T) {
	base := New(CodeTimeout, "timed out")
	wrapped := New(CodeSessionClosed, "session closed", WithCause(base))

	if !IsCode(base, CodeTimeout) {
		t.Error("IsCode(base, CodeTimeout) = false, want true")
	}
	if IsCode(base, CodeDNS) {
		t.Error("IsCode(base, CodeDNS) = true, want false")
	}
	// errors.Is follows the Unwrap chain (target must be an error, not a Code).
	if !errors.Is(wrapped, base) {
		t.Error("errors.Is(wrapped, base) = false, want true (through cause)")
	}
	if !IsCode(wrapped, CodeTimeout) {
		t.Error("IsCode(wrapped, CodeTimeout) = false, want true (through cause)")
	}
}

func TestIsMatchesByCode(t *testing.T) {
	a := New(CodeAuthFailed, "one context")
	b := New(CodeAuthFailed, "another context")
	if !errors.Is(a, b) {
		t.Error("errors.Is(a, b) = false: two errors with the same code should match")
	}
	c := New(CodeDNS, "dns error")
	if errors.Is(a, c) {
		t.Error("errors.Is(a, c) = true: different codes should not match")
	}
	// Non-roserr errors never match.
	if errors.Is(a, errors.New("plain")) {
		t.Error("errors.Is with a plain error = true, want false")
	}
}

func TestUnwrap(t *testing.T) {
	cause := errors.New("underlying")
	err := New(CodeNetwork, "network failed", WithCause(cause))
	if !errors.Is(err, cause) {
		t.Error("errors.Is(err, cause) = false, want true")
	}
	if errors.Unwrap(err) != cause {
		t.Errorf("Unwrap() = %v, want %v", errors.Unwrap(err), cause)
	}
	// Without a cause, Unwrap returns nil.
	if errors.Unwrap(New(CodeDNS, "dns")) != nil {
		t.Error("Unwrap() without cause = non-nil, want nil")
	}
}

func TestOptionDefaults(t *testing.T) {
	err := New(CodeCommandFailed, "command failed")
	if err.Remediation != "" {
		t.Errorf("default Remediation = %q, want empty", err.Remediation)
	}
	if err.Cause != nil {
		t.Errorf("default Cause = %v, want nil", err.Cause)
	}
	if err.Context.Via != "" || err.Context.Host != "" || err.Context.Port != 0 ||
		err.Context.Path != "" || err.Context.Extra != nil {
		t.Errorf("default Context not zero: %+v", err.Context)
	}
	if !strings.Contains(err.Error(), "routeros/command-failed: command failed") {
		t.Errorf("bare render unexpected: %q", err.Error())
	}
}

func TestContextOf(t *testing.T) {
	base := New(CodeTimeout, "timed out",
		WithContext(Context{Via: "native-api", Host: "10.0.0.1", Port: 8728}))
	wrapped := New(CodeSessionClosed, "session closed", WithCause(base))

	// errors.As semantics: the first *Error in the chain wins, i.e. the
	// outermost error's own context (even when empty).
	ctx, ok := ContextOf(wrapped)
	if !ok {
		t.Fatal("ContextOf(wrapped): not found")
	}
	if ctx.Via != "" {
		t.Errorf("ContextOf(wrapped) = %+v, want the (empty) outermost context", ctx)
	}
	// Reading context on the error that carries it directly works.
	if ctx, ok := ContextOf(base); !ok || ctx.Via != "native-api" || ctx.Host != "10.0.0.1" || ctx.Port != 8728 {
		t.Errorf("ContextOf(base) = %+v (ok=%v), want via/host/port", ctx, ok)
	}
	if _, ok := ContextOf(New(CodeDNS, "dns")); !ok {
		t.Error("ContextOf on bare roserr.Error: not found")
	}
	if _, ok := ContextOf(errors.New("plain")); ok {
		t.Error("ContextOf on plain error: found, want not found")
	}
	if _, ok := ContextOf(nil); ok {
		t.Error("ContextOf(nil): found, want not found")
	}
}

func TestIsCodeNonRoserr(t *testing.T) {
	if IsCode(errors.New("plain"), CodeTimeout) {
		t.Error("IsCode on plain error = true, want false")
	}
	if IsCode(nil, CodeTimeout) {
		t.Error("IsCode(nil) = true, want false")
	}
}
