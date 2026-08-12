// Unit tests for the SSH console transport — no network. The lab integration
// matrix (M22–M29) lives in ssh_lab_test.go (build tag `integration`).
package ssh

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"math/rand"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/quiqxiq/goros/v4/roserr"
)

func TestCleanConsoleOutput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already clean", "Flg Src  Dst\n  A 1.2.3.4\n", "Flg Src  Dst\n  A 1.2.3.4"},
		{"CRLF", "line1\r\nline2\r\n", "line1\nline2"},
		{"lone CR", "line1\rline2\r", "line1\nline2"},
		{"trailing whitespace per line", "  a   \t\n b  \n", "  a\n b"},
		{"leading indentation preserved", "  ip/address  print\n    ether1\n", "  ip/address  print\n    ether1"},
		{"blank edge lines dropped", "\n\nok\n\n\n", "ok"},
		{"interior blank line kept", "a\n\nb\n", "a\n\nb"},
		{"empty", "", ""},
		{"only blanks", " \n\t\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CleanConsoleOutput(c.in); got != c.want {
				t.Errorf("CleanConsoleOutput(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMapSshError(t *testing.T) {
	cases := []struct {
		msg  string
		code roserr.Code
	}{
		{"host key verification failed", roserr.CodeHostKeyMismatch},
		{"ssh: host key mismatch", roserr.CodeHostKeyMismatch},
		{"unable to authenticate, attempted methods [none password]", roserr.CodeAuthFailed},
		{"ssh: handshake failed: permission denied", roserr.CodeAuthFailed},
		{"too many authentication failures", roserr.CodeAuthFailed},
		{"dial tcp 192.168.1.1:22: connect: connection refused", roserr.CodeConnectionRefused},
		{"dial tcp 192.168.1.1:22: i/o timeout", roserr.CodeTimeout},
		{"dial tcp: lookup router.local: no such host", roserr.CodeDNS},
		{"dial tcp: could not resolve", roserr.CodeDNS},
		{"some unexpected transport error", roserr.CodeNetwork},
	}
	for _, c := range cases {
		t.Run(c.msg, func(t *testing.T) {
			err := mapSshError(sshError(c.msg), "192.168.1.1", 22, "dial")
			if !roserr.IsCode(err, c.code) {
				t.Errorf("mapSshError(%q) = %v, want %q", c.msg, err, c.code)
			}
			ctx, ok := roserr.ContextOf(err)
			if !ok || ctx.Via != "ssh" || ctx.Host != "192.168.1.1" || ctx.Port != 22 {
				t.Errorf("context = %+v, want via=ssh host=192.168.1.1 port=22", ctx)
			}
		})
	}
}

// sshError is a minimal error type so tests don't depend on x/crypto's
// internal error construction.
type sshError string

func (e sshError) Error() string { return string(e) }

func TestMapSshErrorPassesRoserrThrough(t *testing.T) {
	original := roserr.New(roserr.CodeTimeout, "wrapped")
	got := mapSshError(original, "h", 22, "dial")
	if got != original {
		t.Errorf("mapSshError must return an already-mapped roserr error unchanged, got %v", got)
	}
}

// testPublicKey generates a deterministic ECDSA public key (two different
// seeds yield different keys).
func testPublicKey(t *testing.T, seed int64) ssh.PublicKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.New(rand.NewSource(seed)))
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	return pub
}

func TestHostKeyStoreAcceptNew(t *testing.T) {
	s := &hostKeyStore{keys: map[string]ssh.PublicKey{}}
	cb := s.callback()
	k1 := testPublicKey(t, 1)
	// First key for a hostport is accepted and remembered.
	if err := cb("192.168.1.1", &net.TCPAddr{}, k1); err != nil {
		t.Fatalf("first key: %v", err)
	}
	// The same key again is accepted.
	if err := cb("192.168.1.1", &net.TCPAddr{}, k1); err != nil {
		t.Fatalf("same key: %v", err)
	}
	// A different key for the same hostport is rejected with the taxonomy.
	k2 := testPublicKey(t, 2)
	err := cb("192.168.1.1", &net.TCPAddr{}, k2)
	if err == nil {
		t.Fatal("different key: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeHostKeyMismatch) {
		t.Errorf("err = %v, want transport/host-key-mismatch", err)
	}
	// A different hostname is unaffected (fresh accept-new).
	if err := cb("10.0.0.1", &net.TCPAddr{}, k2); err != nil {
		t.Fatalf("fresh hostname: %v", err)
	}
}

func TestDialRequiresAuth(t *testing.T) {
	// Without any auth method, Dial fails with a clear error before any I/O.
	start := time.Now()
	_, err := Dial(context.Background(), "192.0.2.1:22", "admin")
	if err == nil {
		t.Fatal("Dial without auth: want error, got nil")
	}
	if !roserr.IsCode(err, roserr.CodeAuthFailed) {
		t.Errorf("err = %v, want auth/failed", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("no-auth Dial must fail locally without attempting a connection")
	}
}
