// Package ssh implements the RouterOS console transport over SSH (Fase 7).
//
// Behavior follows the facts verified in the lab (docs/RESEARCH.md §15) and
// the centrs reference project (src/protocols/ssh.ts): RouterOS grants no
// pseudo-tty, a single-line command runs on the console and returns clean
// output (no prompt, ANSI, or echo), and RouterOS 6 SSH exec only accepts the
// space-separated console form of a command — so this package runs
// Command.ConsoleCLI() (R12) and cleans output with CleanConsoleOutput.
// Error mapping ports mapSshConnectError onto the roserr taxonomy.
//
// The Gate 1 (:parse) validator rides the same :put script used by
// native-api, classified by the same PureSyntaxClassifier — no classifier is
// duplicated (see gate.NewConsoleGate and Client.Validate, D-016/D-017).
package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/quiqxiq/goros/v4/gate"
	"github.com/quiqxiq/goros/v4/roserr"
	"github.com/quiqxiq/goros/v4/transport"
)

// HostKeyPolicy selects how the client verifies the device's SSH host key.
type HostKeyPolicy int

const (
	// HostKeyToFU is the default: the first host key seen for an address is
	// accepted and remembered for the lifetime of the Client; a different key
	// for the same address is rejected with transport/host-key-mismatch
	// (OpenSSH "accept-new" semantics).
	HostKeyToFU HostKeyPolicy = iota
	// HostKeyInsecure skips host-key verification entirely. Explicit opt-out
	// for lab/trusted networks only; never the default.
	HostKeyInsecure
)

// Client is one SSH connection to a RouterOS device, implementing
// transport.ConsoleTransport. Commands run synchronously: a single connection
// executes one console line at a time (no tag multiplexing); concurrency is
// several separate Clients (PLAN.md §11). Run is mutex-protected.
type Client struct {
	mu   sync.Mutex
	conn *ssh.Client
	host string
	port int
}

var (
	_ transport.Transport        = (*Client)(nil)
	_ transport.ConsoleTransport = (*Client)(nil)
)

// DialOption configures Dial (D-018). Functional options; at least one auth
// method (WithPassword / WithPrivateKey / WithPrivateKeyFile) is required.
type DialOption func(*dialConfig)

type dialConfig struct {
	auth       []ssh.AuthMethod
	pemKeys    [][]byte
	keyFiles   []string
	hostKey    HostKeyPolicy
	knownHosts string
	timeout    time.Duration
}

// WithPassword adds username/password authentication.
func WithPassword(pass string) DialOption {
	return func(c *dialConfig) { c.auth = append(c.auth, ssh.Password(pass)) }
}

// WithPrivateKey adds public-key authentication from PEM bytes (unencrypted
// keys only; the library does not support passphrase-protected keys, matching
// the reference project's scope). The key is parsed during Dial so a malformed
// key surfaces as a real error there, not silently.
func WithPrivateKey(pem []byte) DialOption {
	return func(c *dialConfig) { c.pemKeys = append(c.pemKeys, pem) }
}

// WithPrivateKeyFile adds public-key auth from a PEM file on disk. The file is
// read and parsed during Dial.
func WithPrivateKeyFile(path string) DialOption {
	return func(c *dialConfig) { c.keyFiles = append(c.keyFiles, path) }
}

// WithHostKeyPolicy overrides host-key verification (default HostKeyToFU).
func WithHostKeyPolicy(p HostKeyPolicy) DialOption {
	return func(c *dialConfig) { c.hostKey = p }
}

// WithKnownHosts pins the device's host key against an OpenSSH known_hosts
// file (strict: the key must already be listed). When unset, TOFU accept-new
// semantics apply (HostKeyToFU).
func WithKnownHosts(path string) DialOption {
	return func(c *dialConfig) { c.knownHosts = path }
}

// WithTimeout sets the connect and per-command timeout (default 10s).
func WithTimeout(d time.Duration) DialOption {
	return func(c *dialConfig) { c.timeout = d }
}

// Dial connects to addr (host:port) and authenticates as user. At least one
// auth method is required. Returns a Client implementing
// transport.ConsoleTransport, or an error mapped onto the roserr taxonomy
// (auth/failed, transport/connection-refused, transport/timeout,
// transport/dns, transport/network, transport/host-key-mismatch).
func Dial(ctx context.Context, addr, user string, opts ...DialOption) (*Client, error) {
	cfg := &dialConfig{hostKey: HostKeyToFU, timeout: 10 * time.Second}
	for _, o := range opts {
		o(cfg)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, mapSshError(err, addr, 0, "dial")
	}
	port, _ := strconv.Atoi(portStr)

	auth, err := buildAuth(cfg, host, port)
	if err != nil {
		return nil, err
	}
	cb, err := hostKeyCallback(cfg, host, port)
	if err != nil {
		return nil, err
	}
	clientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: cb,
		Timeout:         cfg.timeout,
	}
	conn, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		return nil, mapSshError(err, host, port, "dial")
	}
	return &Client{conn: conn, host: host, port: port}, nil
}

// buildAuth assembles the ssh.AuthMethod list from the configured methods,
// parsing any private keys so failures surface as errors.
func buildAuth(cfg *dialConfig, host string, port int) ([]ssh.AuthMethod, error) {
	ctx := roserr.Context{Via: "ssh", Host: host, Port: port}
	auth := append([]ssh.AuthMethod(nil), cfg.auth...)
	addKey := func(pem []byte, src string) error {
		signer, err := ssh.ParsePrivateKey(pem)
		if err != nil {
			return roserr.New(
				roserr.CodeAuthFailed,
				"cannot parse private key: "+src,
				roserr.WithRemediation("provide an unencrypted PEM private key"),
				roserr.WithContext(ctx),
				roserr.WithCause(err),
			)
		}
		auth = append(auth, ssh.PublicKeys(signer))
		return nil
	}
	for _, pem := range cfg.pemKeys {
		if err := addKey(pem, "WithPrivateKey"); err != nil {
			return nil, err
		}
	}
	for _, path := range cfg.keyFiles {
		pem, err := os.ReadFile(path)
		if err != nil {
			return nil, mapSshError(err, host, port, "read-key")
		}
		if err := addKey(pem, path); err != nil {
			return nil, err
		}
	}
	if len(auth) == 0 {
		return nil, roserr.New(
			roserr.CodeAuthFailed,
			"no authentication method configured",
			roserr.WithRemediation("provide WithPassword, WithPrivateKey, or WithPrivateKeyFile"),
			roserr.WithContext(ctx),
		)
	}
	return auth, nil
}

// hostKeyCallback builds the ssh.HostKeyCallback for cfg: TOFU accept-new
// over an in-memory store (default), a strict known_hosts pin, or no
// verification (HostKeyInsecure).
func hostKeyCallback(cfg *dialConfig, host string, port int) (ssh.HostKeyCallback, error) {
	ctx := roserr.Context{Via: "ssh", Host: host, Port: port}
	if cfg.hostKey == HostKeyInsecure {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	if cfg.knownHosts != "" {
		cb, err := knownhosts.New(cfg.knownHosts)
		if err != nil {
			return nil, roserr.New(
				roserr.CodeNetwork,
				"cannot read known_hosts file",
				roserr.WithRemediation("check the path given to WithKnownHosts"),
				roserr.WithContext(ctx),
				roserr.WithCause(err),
			)
		}
		return cb, nil
	}
	store := &hostKeyStore{keys: map[string]ssh.PublicKey{}}
	return store.callback(), nil
}

// hostKeyStore implements OpenSSH "accept-new": the first key seen for a
// hostport is trusted; a later, different key for the same hostport is
// rejected.
type hostKeyStore struct {
	mu   sync.Mutex
	keys map[string]ssh.PublicKey
}

// callback returns the ssh.HostKeyCallback for one Client. Keys are keyed by
// the hostname the SSH library passes (the host part of the dial address).
func (s *hostKeyStore) callback() ssh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		existing, ok := s.keys[hostname]
		if !ok {
			s.keys[hostname] = key
			return nil
		}
		if !bytes.Equal(existing.Marshal(), key.Marshal()) {
			return roserr.New(
				roserr.CodeHostKeyMismatch,
				fmt.Sprintf("host key mismatch for %s: the device presented a different key than previously trusted", hostname),
				roserr.WithRemediation("confirm the device identity before trusting the new key"),
				roserr.WithContext(roserr.Context{Via: "ssh", Host: hostname}),
			)
		}
		return nil
	}
}

// Run executes one console line (no pseudo-tty) and returns the device's
// cleaned text output. RouterOS reports command errors in-band via the
// console output, so a non-zero SSH exit with non-empty output still returns
// that output (the gate classifies it); only transport-level failures return
// errors. A context timeout/cancel aborts the command and releases the
// session.
func (c *Client) Run(ctx context.Context, line string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	sess, err := c.conn.NewSession()
	if err != nil {
		return "", mapSshError(err, c.host, c.port, "open-session")
	}
	defer sess.Close()

	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := sess.Output(line)
		done <- result{out, err}
	}()
	select {
	case <-ctx.Done():
		// Release the session to unblock Output, then reap the goroutine.
		_ = sess.Close()
		<-done
		if ctx.Err() == context.DeadlineExceeded {
			return "", roserr.New(
				roserr.CodeTimeout,
				"command timed out",
				roserr.WithCause(ctx.Err()),
				roserr.WithContext(roserr.Context{Via: "ssh", Host: c.host, Port: c.port}),
			)
		}
		return "", ctx.Err()
	case res := <-done:
		out := CleanConsoleOutput(string(res.out))
		if res.err == nil {
			return out, nil
		}
		var exitErr *ssh.ExitError
		if errors.As(res.err, &exitErr) {
			if strings.TrimSpace(out) == "" {
				return "", roserr.New(
					roserr.CodeCommandFailed,
					"device exited without console output",
					roserr.WithCause(res.err),
					roserr.WithContext(roserr.Context{Via: "ssh", Host: c.host, Port: c.port}),
				)
			}
			// Device error reported in-band via console output: return the
			// text; the caller (gate) classifies it.
			return out, nil
		}
		return out, mapSshError(res.err, c.host, c.port, "run")
	}
}

// Capabilities implements transport.Transport: console yes, structured and
// inspect no.
func (c *Client) Capabilities() transport.Capabilities {
	return transport.Capabilities{Console: true}
}

// Close closes the SSH connection. Idempotent.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Validate runs Gate 1 (the device's own :parse parser) over this SSH
// connection for one raw console line (D-017): syntax and unknown-attribute
// errors come back as validation/syntax / validation/unknown-attribute; nil
// when the device parser accepts the line. Read-only — line is never
// executed. Same classifier as native-api (gate.PureSyntaxClassifier).
func (c *Client) Validate(ctx context.Context, line string) error {
	return gate.ValidateConsole(ctx, c, line)
}
