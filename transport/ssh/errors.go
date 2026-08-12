package ssh

import (
	"errors"
	"regexp"

	"github.com/quiqxiq/goros/v4/roserr"
)

// Error-message classes for mapping SSH connection errors onto the roserr
// taxonomy. Ported from mapSshConnectError in the centrs reference project
// (src/protocols/ssh.ts L94), adapted to the error text produced by
// golang.org/x/crypto/ssh.
var (
	hostKeyMismatchRe   = regexp.MustCompile(`(?i)host key (verification failed|mismatch)`)
	authFailedRe        = regexp.MustCompile(`(?i)(unable to authenticate|permission denied|authentication failed|no supported methods remain|no such identity|too many authentication failures|failed to login)`)
	connectionRefusedRe = regexp.MustCompile(`(?i)connection refused`)
	timeoutRe           = regexp.MustCompile(`(?i)(i/o timeout|connection timed out|timed out|deadline exceeded)`)
	dnsRe               = regexp.MustCompile(`(?i)(no such host|could not resolve|name or service not known|server misbehaving|no route to host)`)
)

// mapSshError classifies an SSH-layer error (dial, session, channel) into the
// roserr taxonomy. An error already in the taxonomy (e.g. returned by the
// host-key callback) is returned unchanged; anything unrecognized falls back
// to transport/network. Device command errors are NOT classified here — they
// arrive in-band as console output (see Client.Run).
func mapSshError(err error, host string, port int, op string) error {
	if err == nil {
		return nil
	}
	var rosErr *roserr.Error
	if errors.As(err, &rosErr) {
		return err
	}
	ctx := roserr.Context{Via: "ssh", Host: host, Port: port}
	if op != "" {
		ctx.Extra = map[string]any{"op": op}
	}
	msg := err.Error()
	switch {
	case hostKeyMismatchRe.MatchString(msg):
		return roserr.New(
			roserr.CodeHostKeyMismatch,
			msg,
			roserr.WithRemediation("confirm the device identity; pin the key via WithKnownHosts, or opt out explicitly with WithHostKeyPolicy(ssh.HostKeyInsecure)"),
			roserr.WithContext(ctx),
			roserr.WithCause(err),
		)
	case authFailedRe.MatchString(msg):
		return roserr.New(
			roserr.CodeAuthFailed,
			msg,
			roserr.WithRemediation("check the username/password or private key. Note: RouterOS refuses password login once an SSH key is set for the user."),
			roserr.WithContext(ctx),
			roserr.WithCause(err),
		)
	case connectionRefusedRe.MatchString(msg):
		return roserr.New(
			roserr.CodeConnectionRefused,
			msg,
			roserr.WithRemediation("confirm the RouterOS SSH service is enabled (ip service ssh) and reachable on port 22"),
			roserr.WithContext(ctx),
			roserr.WithCause(err),
		)
	case timeoutRe.MatchString(msg):
		return roserr.New(
			roserr.CodeTimeout,
			msg,
			roserr.WithRemediation("check reachability and latency; raise WithTimeout if the device is slow"),
			roserr.WithContext(ctx),
			roserr.WithCause(err),
		)
	case dnsRe.MatchString(msg):
		return roserr.New(
			roserr.CodeDNS,
			msg,
			roserr.WithRemediation("check the hostname/address"),
			roserr.WithContext(ctx),
			roserr.WithCause(err),
		)
	default:
		return roserr.New(roserr.CodeNetwork, msg, roserr.WithContext(ctx), roserr.WithCause(err))
	}
}
