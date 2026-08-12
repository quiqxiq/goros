# PLAN REMAINING — Implementasi sisa menuju rilis v4.0.0

> Dokumen eksekusi untuk **semua pekerjaan yang tersisa** sebelum rilis pertama
> `v4.0.0`, sesuai status PLAN.md (2026-08-12). **Fase 8 (MAC-Telnet) dan
> Fase 9 (REST) DI-SKIP** atas keputusan pemilik — keduanya boleh menyusul
> sebagai minor release (§17 PLAN.md). RoMON/Winbox tetap di luar scope (§16).
>
> Prasyarat yang sudah selesai (tidak diulang di sini): migrasi module path
> `v3 → v4` (D-014), probe SSH R6/R7/R6b/R6c/R12 ke MT-1 (v6 6.49.11) &
> MT-2 (v7 7.21.5) — hasil lengkap di `docs/RESEARCH.md` §15.

---

## 0. Status & fakta kunci

| Item | Status | Bukti |
|---|---|---|
| Fase 0–6 (riset, kontrak, native-api, Gate 1/2, schema, orkestrasi) | ✅ selesai | `docs/DESIGN.md`, `docs/DECISIONS.md` D-001..D-015 |
| Migrasi `v4` + go 1.22 + `x/crypto` | ✅ selesai | `go.mod`, seluruh import `quiqxiq/goros/v4` |
| Probe R6/R7/R6b/R6c/R12 (SSH) | ✅ selesai | `docs/RESEARCH.md` §15 |
| `transport/ssh` | ⬜ **belum ada** | `transport/` hanya berisi `contract.go`, `mock/`, `nativeapi/` |
| Adapter gate console (D-016) | ⬜ belum | D-016/D-017 belum tercatat di DECISIONS.md |
| Metrik sesi (Fase 10) | ⬜ belum ada di kode | — |
| CI job integration (§15) | ⬜ belum | `go.yml` hanya lint + build |
| README contoh `Validate`/`Inspect`/`RunStructured` | ⬜ belum | 0 match |
| Tag `v4.0.0` + rilis | ⬜ belum | — |
| Fase 8 (MAC-Telnet) & Fase 9 (REST) | ⛔ di-skip | keputusan pemilik 2026-08-12 |

Fakta lab yang mengikat desain (RESEARCH.md §15):

- **R12:** SSH exec v6 **hanya menerima bentuk console spasi**
  (`/ip address print`), bentuk slash-joined (`/ip/address/print`) gagal.
  v7 menerima keduanya → `transport.ssh` merender CLI bentuk spasi
  (`ConsoleCLI()`) secara universal.
- **R6b v6:** `:put [:parse "<bentuk spasi>"]` di v6 → `(eval …)` (valid).
  **Gate 1 console di v6 VIABLE** via SSH (jalur native-api v6 tetap skip,
  D-009/R4). Classifier `PureSyntaxClassifier` berlaku apa adanya.
- **R6b v7:** hasil `:parse` via SSH **identik native-api** → satu classifier.
- **R6:** output SSH bersih; **R7:** output panjang tanpa paging;
  **R6c:** `unable to authenticate` → `auth/failed`, `connection refused` →
  `transport/connection-refused`, host-key sha256 tercapture.

---

## 1. Workstream A — Fase 7: transport SSH (blokir DoD §17)

### A1. `ConsoleCLI()` di `transport/contract.go` (R12)

- Method baru `func (c *Command) ConsoleCLI() string` — pendamping `CLI()`
  (aditif; `CLI()` yang dipakai native-api/gate tidak berubah).
- Bentuk: `/ip address print interface=ether1` — token `PathTokens()` digabung
  spasi, lalu verb, lalu atribut (sorted, quoting sama dengan `CLI()`).
- `Script` → dikembalikan apa adanya (konsisten `CLI()`).

### A2. Package `transport/ssh` (3 file + unit test)

```
transport/ssh/
  ssh.go       — Dial (password + private-key), host-key policy TOFU/insecure,
                 timeout, Client (ConsoleTransport), Run (mutex + ctx),
                 Validate (D-017)
  errors.go    — mapSshError: 6 kode roserr (host-key-mismatch, auth/failed,
                 connection-refused, timeout, dns, network) + remediasi
  output.go    — CleanConsoleOutput (port centrs ssh.ts L182)
  ssh_test.go  — unit (tanpa jaringan): CleanConsoleOutput, mapSshError,
                 host-key store TOFU (accept-new + mismatch)
  ssh_lab_test.go — integration (tag `integration`), matriks M22–M29
```

- API `Dial(ctx, addr, user string, opts ...DialOption)` dengan opsi fungsional:
  `WithPassword`, `WithPrivateKey`, `WithPrivateKeyFile`, `WithHostKeyPolicy`,
  `WithKnownHosts`, `WithTimeout` (**D-018** — bentuk konkret, semantik sama
  dengan sketsa DialOptions di PLAN-FASE7 §5.2).
- Host-key: default **TOFU accept-new** (store in-memory per Client); `Insecure`
  opt-out eksplisit; `WithKnownHosts` = pin ketat format OpenSSH.
- `Run`: tanpa PTY (fakta centrs + R6); mutex per Client; ctx per-command;
  exit code non-zero dengan output → output dikembalikan (error device
  dilaporkan in-band lewat console, gate yang mengklasifikasi); exit non-zero
  tanpa output → `routeros/command-failed`; error transport → `mapSshError`.
- `Validate(ctx, line)` (D-017) → `gate.ValidateConsole` (lihat A3).

### A3. Gate console — reuse, bukan salin (D-016/D-017)

- **Persempit `Gate1.Transport`** dari `transport.StructuredTransport` ke
  antarmuka minimal baru `gate.CommandTransport` (`Command(ctx, *Command)
  (*Reply, error)`). Gate 1 tidak pernah butuh `Inspect`. Tidak memutus
  native-api (adapter/`clientTransport`/mock semuanya punya `Command`).
- Tambah field `RenderCLI func(*Command) string` di `Gate1` (nil = `CLI()`,
  bentuk slash untuk native-api); console me-wire ke `ConsoleCLI()` (R12).
- `gate/console.go`:
  - `NewConsoleCommand(ct ConsoleTransport) CommandTransport` — adapter
    `Command{Script: s}` → `Run(ctx, s)` → `Reply{Type: !re, ret: out}`.
  - `NewConsoleGate(ct, supportsParse) *Gate1` — pre-wired dengan adapter +
    `RenderCLI: ConsoleCLI`.
  - `ValidateConsole(ctx, ct, line) error` (D-017) — helper satu baris.
- Tidak ada classifier kedua: `PureSyntaxClassifier` tetap satu-satunya.

### A4. Integration test M22–M29

Matriks (detail di §8 PLAN-FASE7.md), env `ROUTEROS_TEST_SSH_*`:

| # | Skenario | v6 | v7 |
|---|---|---|---|
| M22 | Dial + `/system identity print` (spasi) → output bersih | ✅ diuji | ✅ diuji |
| M23 | `/nonsense command` → Gate tolak `validation/syntax` | diuji (toleran) | ✅ |
| M24 | `print bogus=1` → error setara native-api | diuji (toleran) | ✅ |
| M25 | `/ip route print detail` output panjang lengkap | diuji | ✅ |
| M26 | auth salah → `auth/failed` | ✅ | ✅ |
| M27 | host-key: TOFU accept + Insecure | unit test mismatch | unit test |
| M28 | ctx timeout → `transport/timeout` + cleanup | ✅ | ✅ |
| M29 | ValidationError SSH setara native-api (v7) | — | ✅ |

Sel v6 gate: toleransi hasil (kedua kode `validation/*` diterima; bentuk
tak dikenal → catat di RESEARCH, jangan gagal — pola karakterisasi yang sama
dengan M9 native-api). Hasil aktual dicatat ke RESEARCH.md §16 & matriks
PLAN-FASE7 setelah run.

---

## 2. Workstream B — Fase 10 cross-cutting (scope yang dikerjakan)

- **Metrik (D-019):** `runState` + counter atomik — `InspectRoundTrips`
  (inkremen di `InspectNodes`, membuktikan cache schema efektif) dan latensi
  gate 1/2 (diukur di `validate()`). API publik `Client.Metrics() Metrics`
  (snapshot read-only). Unit test: cache hit → `InspectRoundTrips` naik 1×
  untuk dua `Discover` (fixture mock), `Validate`/`RunStructured` tercatat.
- **Audit roserr:** seluruh transport & gate memakai taksonomi — grep
  `errors.New`/`fmt.Errorf` di `transport/`, `gate/`, `schema/`, root; yang
  bukan roserr diperbaiki (atau dibiarkan jika memang error infra Go murni
  seperti `ctx.Err()`, dicatat).
- **Logging tanpa bocor kredensial:** audit panggilan log — tidak ada password/
  MD5 challenge/kunci privat di log. Kode baru SSH tidak mencatat apa pun.
- **godoc:** 100% simbol publik baru ber-doc; audit simbol publik lama yang
  kekurangan doc → tambahkan.

---

## 3. Workstream C — CI (§15): job integration terpisah

- `go.yml`: job `integration` (needs: build; `if: schedule ||
  workflow_dispatch` — manual/terjadwal, pipeline utama tidak boleh gagal saat
  lab off) menjalankan `go test -tags integration -count=1` untuk
  `./transport/nativeapi/ ./transport/ssh/ ./gate/ .` dengan env
  `ROUTEROS_TEST_*` dari secrets.
- `.gitignore`: tambah `.refrences/` dan `.understand-anything/` (referensi
  lokal + tooling; TIDAK boleh ikut ter-commit).

---

## 4. Workstream D — Dokumen & README

- `README.md`: fix bagian Fork (`v3` → `v4`, D-014); tambah bagian
  "Validasi & eksekusi terstruktur": contoh `Validate` (dry-run), `Inspect`
  (discovery schema), `RunStructured` (validate-then-execute), dan contoh
  SSH (`ssh.Dial` + `Run` + `Validate`).
- `DESIGN.md`: §1 module path → `v4`; §3 tree `ssh/` → diimplementasikan;
  §4.6 Gate1 (CommandTransport + RenderCLI); §4.8 baru (transport ssh);
  §4.7 tambah metrik.
- `DECISIONS.md`: D-016 (narrow Gate1), D-017 (ValidateConsole/ssh.Validate),
  D-018 (DialOption fungsional), D-019 (metrik), D-020 (gitignore referensi).
- `RESEARCH.md` §16: hasil aktual M22–M29.
- `PLAN-FASE7.md`: matriks M22–M29 terisi hasil lab.
- `PLAN.md`: status §0 — Fase 7 ✅, Fase 8/9 di-skip (keputusan pemilik),
  Fase 10 ✅, CI ✅, rilis dilakukan.

---

## 5. Rilis v4.0.0 (gh CLI)

1. Validasi penuh terakhir: `gofmt -l` bersih, `go build ./...`,
   `go vet ./...`, `go vet -tags integration ./...`, `go test -race ./...`
   hijau, lab integration v6 & v7 PASS.
2. Commit logis (4):
   - `feat: multi-transport validation core (Fase 0–6) + module v4`
   - `feat(ssh): console transport + gate console adapter (Fase 7)`
   - `feat(metrics): session metrics (Fase 10) + CI integration job`
   - `docs: README, DESIGN, DECISIONS, RESEARCH, PLAN — release prep`
3. `git push origin main`.
4. `git tag v4.0.0` + `git push origin v4.0.0`.
5. `gh release create v4.0.0 --title "goros v4.0.0" --notes ...` (ringkasan
   fitur + catatan breaking: module path v4, D-014).
6. Verifikasi: `gh release view v4.0.0` + CI hijau di GitHub.

---

## 6. Risiko & mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Error shape `:parse` v6 via SSH tidak persis cocok classifier | M23/M24 v6 gagal | Sel v6 toleran + hasil dicatat jujur; fixture v6 ditambah ke unit test classifier bila bentuk baru ditemukan |
| `ssh.ExitError` disalahpahami sebagai transport error | Gate tak melihat teks error device | `Run` mengembalikan output meski exit ≠ 0 (error device in-band); hanya transport-level yang jadi error |
| Push https gagal (remote origin https, gh pakai ssh) | Rilis tertahan | Fallback: set push URL ke `git@github.com:quiqxiq/goros.git` |
| `.refrences/` ikut ter-commit | Repo membengkak + kredensial acuan | `.gitignore` sebelum `git add` (Workstream C) |
| Sesi SSH hang (command tak berakhir) | Goroutine bocor | ctx per-command + `sess.Close()` untuk unblock; test M28 |
| `WithKnownHosts` file tidak ada | Dial error tidak jelas | Dikembalikan sebagai error mapSshError + remediasi |

---

## 7. DoD — checklist rilis v4.0.0

- [ ] `transport/ssh` (Dial/Run/errors/output) + unit test hijau.
- [ ] `ConsoleCLI()` + Gate console adapter (reuse `PureSyntaxClassifier`,
      terbukti lewat test regresi) — tidak ada classifier kedua.
- [ ] M22–M29 PASS (atau hasil karakterisasi jujur dicatat) di v6 & v7.
- [ ] Metrik sesi `Client.Metrics()` + audit roserr/logging/godoc selesai.
- [ ] Job CI integration + `.gitignore` (.refrences/.understand-anything).
- [ ] README/DESIGN/DECISIONS/RESEARCH/PLAN-FASE7/PLAN konsisten, tanpa
      kredensial di repo.
- [ ] Validasi penuh hijau; `v4.0.0` di-tag & di-push; `gh release` dibuat
      dan terverifikasi.
- [ ] Fase 8/9: tercatat di-skip (keputusan pemilik), boleh minor release.
