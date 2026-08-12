# Changelog

All notable changes to goros are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v4.0.1] - 2026-08-12

### Added

- **Integration test suite for listener isolation & multi-connection**
  (`multi_lab_test.go`). Verified against real RouterOS devices (v6 + v7):
  cancelling one listener (via context timeout or manual `Cancel()`) does not
  stop sibling listeners on the same connection; closing one connection does
  not kill other connections; concurrent connections to the same device and
  cross-device concurrent commands all succeed.
- **Lab probe integration tests** (`probe_lab_test.go`). A read-only battery
  exercising the full validation pipeline on a real device: session
  capability probes, `/console/inspect` schema discovery (table vs action
  categories), dry-run `Validate` (Gate 1 `:parse` + Gate 2 attribute schema),
  Gate 1 script validation, and `RunStructured` execution.
- **`docs/USAGE.md`** — a complete usage guide covering installation, `Run`,
  `Listen` (implicit and explicit async), `tab` polling, multi-listen,
  multi-connection, structured validation, TLS, error handling, SSH console,
  and logging, with lab-verified example output and instructions to re-run the
  tests against your own devices.
- **`CHANGELOG.md`** — this file.

### Changed

- **`README.md`** — link to the new `docs/USAGE.md` and to the `multi_listen`
  example.

### Fixed

- **`client_test.go` — `TestTrapHandling` now cleans up after itself.** The
  DNS static entry it creates is auto-removed: the `.id` is looked up by the
  exact `name`+`address` the test uses, and the cleanup is registered before
  the mutation (and ordered before the connection close) so it fires even
  when the test fails mid-way. Previously the entry could be left on the
  device; now verified to leave zero leftovers on both lab devices.

## [v4.0.0] - 2026-08-12

### Added

- **Two transports**: the native-api transport (adapted from the existing
  client, fully backward compatible) and the SSH console transport
  (`transport/ssh`, RouterOS 6 & 7, exec without PTY).
- **Structured command validation** (PLAN.md Fase 0–7):
  - `roserr` — stable error taxonomy (`validation/*`, `routeros/*`,
    `auth/*`, `transport/*` codes).
  - `transport` — canonical `Command`/`Reply` contract + capability matrix.
  - `gate` — Gate 1 (`:parse` syntax, shared classifier across transports)
    and Gate 2 (attribute schema against `/console/inspect`).
  - `schema` — `/console/inspect` discovery (table/action categories) with
    per-session caching.
  - Client facade: `Validate` (dry-run, never executes), `Inspect`,
    `RunStructured`, plus `ProbeInspect`/`ProbeParse` session probes.
- **Session metrics** (gate latencies) on the root client.
- **Module path migrated** to `github.com/quiqxiq/goros/v4` (Go 1.22).
- Documentation: `docs/DESIGN.md`, `docs/DECISIONS.md`, `docs/RESEARCH.md`,
  `docs/PLAN.md` and phase plans, README.

### Changed

- The `go-routeros/routeros/v3` fork now lives under `github.com/quiqxiq/goros/v4`;
  `go.mod` requires `golang.org/x/crypto` (direct) for the SSH transport.

### Fixed

- Multi-listen bug fixes (upstream `go-routeros`), verified on real devices.

### Security

- No credentials are stored in the repository; lab credentials come from
  environment variables only.
