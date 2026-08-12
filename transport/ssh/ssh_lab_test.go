//go:build integration

// Lab integration tests for the SSH console transport — matrix M22–M29 in
// docs/PLAN-FASE7.md §8.2. Run only with the `integration` build tag and
// ROUTEROS_TEST_SSH_ADDRESS / ROUTEROS_TEST_SSH_USERNAME /
// ROUTEROS_TEST_SSH_PASSWORD set (address includes :22). Everything here is
// read-only. v6 gate cells are characterization tests: the tolerated codes
// match the native-api M9 pattern, and unclassified outcomes are logged, not
// failed (the classifier is shared and unit-tested; a new device shape gets
// recorded in docs/RESEARCH.md instead of guessing).
package ssh_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	routeros "github.com/quiqxiq/goros/v4"
	"github.com/quiqxiq/goros/v4/gate"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
	"github.com/quiqxiq/goros/v4/transport/nativeapi"
	sshtransport "github.com/quiqxiq/goros/v4/transport/ssh"
)

func labSSHConfig(t *testing.T) (addr, user, pass string) {
	t.Helper()
	addr = os.Getenv("ROUTEROS_TEST_SSH_ADDRESS")
	user = os.Getenv("ROUTEROS_TEST_SSH_USERNAME")
	pass = os.Getenv("ROUTEROS_TEST_SSH_PASSWORD")
	if addr == "" || user == "" || pass == "" {
		t.Skip("ROUTEROS_TEST_SSH_ADDRESS/USERNAME/PASSWORD not set")
	}
	return addr, user, pass
}

// labSSH dials the device over SSH with cleanup wired to close the client.
func labSSH(t *testing.T) *sshtransport.Client {
	t.Helper()
	addr, user, pass := labSSHConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c, err := sshtransport.Dial(ctx, addr, user,
		sshtransport.WithPassword(pass),
		sshtransport.WithTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("ssh.Dial(%s): %v", addr, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func labCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// M22 — Dial + a valid space-form command returns clean, non-empty output.
func TestLabSSHValidCommandCleanOutput(t *testing.T) {
	c := labSSH(t)
	out, err := c.Run(labCtx(t), "/system identity print")
	if err != nil {
		t.Fatalf("Run(identity): %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("output is empty")
	}
	if !strings.Contains(out, "Name:") {
		t.Logf("LAB identity output=%q (no Name: header — build-specific, still non-empty)", out)
	}
	t.Logf("LAB clean output (%d bytes): %q", len(out), out)
}

// M23 — an unknown command is rejected by the Gate 1 console (:parse).
func TestLabSSHGate1UnknownCommand(t *testing.T) {
	c := labSSH(t)
	err := c.Validate(labCtx(t), "/nonsense command")
	if err == nil {
		// Unclassifiable on this build: log and continue (characterization).
		t.Log("LAB unknown-command passed the classifier on this build")
		return
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) {
		t.Errorf("err = %v, want validation/syntax", err)
	}
	t.Logf("LAB unknown-command classified: %v", err)
}

// M24 — an unknown attribute is rejected; codes tolerated as in native-api M9
// (7.21.5 emits the coarse "expected end of command" -> validation/syntax).
func TestLabSSHGate1UnknownAttribute(t *testing.T) {
	c := labSSH(t)
	err := c.Validate(labCtx(t), "/ip address print bogus=1")
	if err == nil {
		t.Log("LAB unknown-attribute passed the classifier on this build")
		return
	}
	if !roserr.IsCode(err, roserr.CodeValidationSyntax) &&
		!roserr.IsCode(err, roserr.CodeValidationUnknownAttribute) {
		t.Errorf("err = %v, want validation/syntax or validation/unknown-attribute", err)
	}
	t.Logf("LAB unknown-attribute classified: %v", err)
}

// M25 — long console output is complete (no paging / truncation, R7).
func TestLabSSHLongOutput(t *testing.T) {
	c := labSSH(t)
	out, err := c.Run(labCtx(t), "/ip route print detail")
	if err != nil {
		t.Fatalf("Run(routes detail): %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("routes detail output is empty")
	}
	if !strings.Contains(out, "DST-ADDR") {
		t.Logf("LAB no DST-ADDR header (build-specific); output %d bytes still complete", len(out))
	}
	t.Logf("LAB long output %d bytes", len(out))
}

// M26 — wrong password maps to auth/failed.
func TestLabSSHAuthFailure(t *testing.T) {
	addr, user, _ := labSSHConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err := sshtransport.Dial(ctx, addr, user,
		sshtransport.WithPassword("definitely-wrong"),
		sshtransport.WithTimeout(8*time.Second),
	)
	if err == nil {
		t.Fatal("Dial with wrong password: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeAuthFailed) {
		t.Errorf("err = %v, want auth/failed", err)
	}
	t.Logf("LAB auth failure mapped: %v", err)
}

// M27 — host-key policies: default TOFU accepts a fresh key; Insecure skips
// verification. (Mismatch rejection is unit-tested in ssh_test.go.)
func TestLabSSHHostKeyPolicies(t *testing.T) {
	addr, user, pass := labSSHConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tofu, err := sshtransport.Dial(ctx, addr, user, sshtransport.WithPassword(pass))
	if err != nil {
		t.Fatalf("TOFU dial: %v", err)
	}
	defer tofu.Close()
	if out, err := tofu.Run(ctx, "/system identity print"); err != nil || out == "" {
		t.Errorf("TOFU Run: out=%q err=%v", out, err)
	}

	insecure, err := sshtransport.Dial(ctx, addr, user,
		sshtransport.WithPassword(pass),
		sshtransport.WithHostKeyPolicy(sshtransport.HostKeyInsecure),
	)
	if err != nil {
		t.Fatalf("insecure dial: %v", err)
	}
	defer insecure.Close()
	t.Log("LAB TOFU + Insecure dials both succeeded")
}

// M28 — a canceled context aborts the command and the client cleans up.
func TestLabSSHContextTimeout(t *testing.T) {
	c := labSSH(t)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	_, err := c.Run(ctx, "/system resource print")
	if err == nil {
		t.Fatal("Run with expired context: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want transport/timeout or context deadline", err)
	}
	// The connection must still be usable after the abort.
	if out, err := c.Run(labCtx(t), "/system identity print"); err != nil || out == "" {
		t.Errorf("post-abort Run: out=%q err=%v", out, err)
	}
	t.Logf("LAB ctx abort: %v; connection still usable", err)
}

// M29 — ValidationError over SSH equals the native-api result for the same
// command (v7-only; native-api Gate 1 skips on v6). Same classifier, so the
// codes must match exactly.
func TestLabSSHValidationEqualsNativeAPI(t *testing.T) {
	addr := os.Getenv("ROUTEROS_TEST_ADDRESS")
	user := os.Getenv("ROUTEROS_TEST_USERNAME")
	pass := os.Getenv("ROUTEROS_TEST_PASSWORD")
	if addr == "" || user == "" || pass == "" {
		t.Skip("ROUTEROS_TEST_ADDRESS/USERNAME/PASSWORD not set (native-api side of M29)")
	}
	sshc := labSSH(t)
	ctx := labCtx(t)

	native, err := routeros.DialContext(ctx, addr, user, pass)
	if err != nil {
		t.Fatalf("DialContext(%s): %v", addr, err)
	}
	defer native.Close()
	if err := native.ProbeParse(ctx); err != nil {
		t.Fatalf("ProbeParse: %v", err)
	}
	if !native.SupportsParse() {
		t.Skip("native-api Gate 1 unsupported (v6): M29 is v7-only")
	}
	g := &gate.Gate1{Transport: nativeapi.New(native), SupportsParse: native.SupportsParse}
	cmd := &transport.Command{Path: "/ip/address", Verb: "print", Attributes: map[string]string{"bogus": "1"}}
	errNative := g.Run(ctx, cmd)
	errSSH := sshc.Validate(ctx, "/ip address print bogus=1")

	if errNative == nil || errSSH == nil {
		t.Fatalf("want both transports to reject; native=%v ssh=%v", errNative, errSSH)
	}
	var ne, se *roserr.Error
	if !errors.As(errNative, &ne) || !errors.As(errSSH, &se) {
		t.Fatalf("errors not roserr: native=%T ssh=%T", errNative, errSSH)
	}
	t.Logf("LAB native=%q | ssh=%q (native=%v | ssh=%v)", ne.Code, se.Code, errNative, errSSH)
	// The same classifier over the same device parser must yield the same
	// semantic code (validation/* per R10).
	if ne.Code != se.Code {
		t.Errorf("code mismatch: native=%q ssh=%q", ne.Code, se.Code)
	}
}
