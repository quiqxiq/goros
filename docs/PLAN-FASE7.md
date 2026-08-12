# PLAN Fase 7 — Transport SSH (console)

> Melanjutkan `PLAN.md` §11 (Fase 7) dan matriks §15. Fase 0–6 **selesai**
> (lihat status di `docs/PLAN-FASE3-FASE4.md` dan `docs/RESEARCH.md`).
> Dokumen ini adalah rencana eksekusi Fase 7: transport kedua, bukti klaim
> "multi-transport" (DoD global §17).
>
> **Keputusan baru yang mengikat dokumen ini (dictatat 2026-08-12):**
> - **D-014** — pemilik memilih **bump v4**: module path
>   `github.com/quiqxiq/goros/v4`, tag rilis `v4.x.y`. Semua import/README
>   bermigrasi (prasyarat, §2).
> - **D-015 + R4** — spike Gate 1 di v6 (MT-1) selesai: **tidak ada mekanisme
>   viable** untuk membaca output script di v6 → degradasi v6 tetap (D-009),
>   kini berbasis fakta (`docs/RESEARCH.md` §14).
>
> Acuan implementasi: `.refrences/centrs/src/protocols/ssh.ts` (fakta
> perilaku + `mapSshConnectError` + `cleanConsoleOutput`),
> `.refrences/centrs/src/execute.ts` (`validateConsoleParseCommand`).

---

## 0. Status & fakta yang sudah dimiliki

| Item | Status | Bukti |
|---|---|---|
| Fase 0–6 (riset, kontrak, native-api, Gate 1/2, schema, orkestrasi) | ✅ selesai | `RESEARCH.md`, `DECISIONS.md` D-001..D-015 |
| Seam `transport.ConsoleTransport` (kontrak) | ✅ sudah ada | `transport/contract.go` — `Run(ctx, line) (string, error)` |
| Classifier Gate 1 (fungsi murni, siap dipakai console) | ✅ sudah ada | `gate/gate1.go` — `PureSyntaxClassifier` + `StringLiteral` + `HasUnbalancedQuotes` (D-008) |
| **R6** SSH tanpa PTY → output bersih (v6/v7) | ✅ **v7 bersih**; v6 hanya menerima bentuk spasi (R12) | `RESEARCH.md` §15 |
| **R7** paging console output panjang via SSH | ✅ v7: 1983 B lengkap, tanpa paging | `RESEARCH.md` §15 |
| **R6b** `:parse` via SSH = native-api? v6 bentuk apa? | ✅ v7 identik native-api (classifier sama berlaku); v6 bentuk spasi → `(eval …)` valid | `RESEARCH.md` §15 |
| **R6c** bentuk error connect/auth SSH | ✅ `unable to authenticate`→`auth/failed`; `connection refused`→`transport/connection-refused`; host-key tercapture | `RESEARCH.md` §15 |
| Mode auth SSH device lab (password? key?) | ✅ password jalan di keduanya (probe R6) | §15 |
| Port 22 v6 & v7 | ✅ OPEN | `RESEARCH.md` inventaris lab |

**Fakta grounded centrs (CHR 7.23.1) yang dipakai sebagai hipotesis untuk
R6/R7** (`ssh.ts` header): RouterOS SSH **tidak memberi pseudo-tty**, tapi
satu baris `ssh user@host "<command>"` **berjalan di console dan mengembalikan
output bersih** — tanpa prompt, tanpa ANSI, tanpa echo. Satu-satunya
post-processing: trim padding kolom console (`cleanConsoleOutput`). Gate
validasi memakai `:put [:parse …]` yang sama persis dengan mac-telnet — lewat
SSH mengembalikan string `(evl …)` / `bad parameter <name>` yang sama.

---

## 1. Tujuan & kriteria selesai

**Tujuan:** transport kedua (console-based) yang membuktikan klaim
"multi-transport" (PLAN.md §17): gate & schema ditulis sekali, berjalan
identik di native-api (structured) dan SSH (console). SSH juga menjadi pola
untuk transport console lain (mac-telnet, Fase 8).

**Kriteria selesai (PLAN.md §11, diuji ke device nyata v6 & v7):**
1. `transport/ssh` terimplementasi penuh: `Dial`, `ConsoleTransport.Run`,
   error mapping, `cleanConsoleOutput`, host-key policy, auth password + key.
2. Gate console bekerja: `:put [:parse "<line>"]` → `PureSyntaxClassifier`
   (fungsi murni yang **sama persis** dengan Fase 3, tidak disalin).
3. Integration test SSH ke CHR/device nyata: **command valid** → output benar;
   **command salah syntax** → `validation/syntax`; **command atribut salah** →
   error dengan kode & makna **setara** hasil native-api untuk kesalahan yang
   sama (v7). Di v6: degradasi seperti desain (skip gate bila `:parse` console
   tidak didukung — hasil probe R6 menentukan).
4. DoD global §17 "minimal satu transport baru selesai penuh" tercapai.

---

## 2. Prasyarat (wajib, mekanis): migrasi module path v3 → v4 (D-014)

D-014 memutuskan bump v4. Semua kode baru Fase 7 harus ditulis dengan module
path `github.com/quiqxiq/goros/v4`. Migrasi dilakukan **sebelum** menulis
kode SSH agar tidak ada double-touch import:

1. `go.mod`: `module github.com/quiqxiq/goros/v4`.
2. Seluruh import internal: `github.com/quiqxiq/goros/v3` →
   `github.com/quiqxiq/goros/v4` (root, `transport/`, `gate/`, `schema/`,
   `roserr/`, `proto/`, `examples/`, `_lab/`).
3. `README.md`: jalur `go get`, contoh kode, link pkg.go.dev.
4. Verifikasi: `go build ./...`, `go vet ./...`, `go test -race ./...` hijau.
5. Catat sebagai bagian eksekusi D-014 di `DECISIONS.md` (tanggal migrasi).

> Catatan: ini perubahan mekanis satu commit; tidak menyentuh perilaku wire.

---

## 3. Riset wajib sebelum kode (R6, R7, auth) — probe ke MT-1 & MT-2

Seperti pola fase sebelumnya: **probe lab dulu, catat ke `docs/RESEARCH.md`,
baru tulis kode.** Target: MT-1 (v6 6.49.11, port 22) & MT-2 (v7 7.21.5,
port 22). Kredensial hanya lewat env var (`M_ADDR`/`M_USER`/`M_PASS`).

**✅ Selesai 2026-08-12 (harness `_lab/probe_ssh`) — hasil lengkap di
`docs/RESEARCH.md` §15.** Ringkasan yang mengubah desain:

1. **R6 v7:** output SSH bersih (tanpa prompt/ANSI/echo) → hanya butuh
   `CleanConsoleOutput` (CRLF→LF, trim trailing per baris, buang baris
   kosong tepi).
2. **R6 v6 (R12):** SSH exec v6 **hanya menerima bentuk console spasi**
   (`/ip address print`), bentuk slash-joined (`/ip/address/print`) gagal
   `expected command name`. Bentuk spasi + slash juga diterima v7 →
   **transport/ssh merender CLI bentuk spasi secara universal** dari
   `PathTokens()` (§5.5).
3. **R6b v7:** `:parse` via SSH **identik native-api** → classifier D-008
   berlaku apa adanya (termasuk pola R11 `(<% …)`).
4. **R6b v6:** `:parse` bentuk spasi di v6 → `(eval …)` (valid, fallthrough
   classifier lolos). **Gate 1 console di v6 VIABLE** (output langsung dari
   session, tanpa `/execute`/`as-string` yang memblokir native-api per R4) —
   membalik kesimpulan R4 untuk jalur console.
5. **R7 v7:** output panjang lengkap tanpa paging.
6. **R6c:** `unable to authenticate` → `auth/failed`; `connection refused` →
   `transport/connection-refused`; host-key sha256 tercapture (TOFU).

**Implikasi desain (diputuskan, D-016):** v6 di matriks berubah dari "skip"
menjadi **bisa PASS** untuk Gate 1 console (bentuk spasi). Bila probe
implementasi menemukan error shape v6 lain (`expected command name`),
tambahkan fixture v6 ke unit test classifier sebelum matriks diisi.

---

## 4. Acuan centrs per bagian implementasi

| Bagian Go | Acuan centrs | Yang diambil |
|---|---|---|
| Dial & opsi koneksi | `ssh.ts` `SshConnectionConfig` + `sshCommonOptions` | Semantik: batch (non-interactive), connect-timeout, host-key TOFU (`accept-new`) vs `insecure`, key path |
| Error mapping | `ssh.ts` `mapSshConnectError` | 6 kode: `transport/host-key-mismatch`, `auth/failed`, `transport/connection-refused`, `transport/timeout`, `transport/dns`, `transport/network` — port 1:1 ke taksonomi `roserr` (Fase 1) |
| Pembersihan output | `ssh.ts` `cleanConsoleOutput` | CRLF→LF, trim trailing whitespace per baris, buang baris kosong tepi, **pertahankan** indentasi leading (isi) |
| Gate console | `execute.ts` `validateConsoleParseCommand` | Preflight kutip lokal (sudah ada `HasUnbalancedQuotes`); satu `:put [:parse …]`; klasifikasi lewat fungsi murni bersama |
| Fakta perilaku | `ssh.ts` header | RouterOS **tanpa pseudo-tty**; satu baris = satu command console; output bersih |

> Catatan: centrs **shell-out** ke host `ssh` (keputusan khusus Bun). Di Go
> gunakan `golang.org/x/crypto/ssh` dengan **perilaku wire yang sama** —
> yang diambil dari centrs adalah fakta RouterOS, bukan implementasinya
> (PLAN.md §3 catatan).

---

## 5. Desain `transport/ssh`

### 5.1 Struktur file

```
transport/ssh/
  ssh.go       — Client: Dial, options (auth, host-key, timeout), ConsoleTransport
  errors.go    — mapSshError (port mapSshConnectError → roserr)
  output.go    — CleanConsoleOutput (port cleanConsoleOutput)
  ssh_test.go  — unit: CleanConsoleOutput, error mapping, dial opsional (skip bila offline)
  ssh_lab_test.go — integration (build tag integration), R6/R7/M22–M29
```

### 5.2 Tipe publik

```go
// DialOptions mengontrol koneksi SSH (zero value = aman & deterministik).
type DialOptions struct {
    HostKey     HostKeyPolicy // ToFU (default) | Insecure (opt-out eksplisit)
    KnownHosts  string        // path file known_hosts (kosong = default TOFU)
    Timeout     time.Duration // connect + per-command timeout
    Auth        []AuthMethod  // Password / PrivateKey (ParsePrivateKey) / Agent
}

// Client mengimplementasikan transport.ConsoleTransport.
type Client struct{ ... }
func Dial(ctx context.Context, addr, user string, opts ...DialOption) (*Client, error)
func (c *Client) Run(ctx context.Context, line string) (string, error)
func (c *Client) Capabilities() transport.Capabilities // {Console: true}
func (c *Client) Close() error
```

- **Auth default:** password (lab memakai password; probe R6c memastikan).
  Dukungan public-key (`ssh.ParsePrivateKey` / agent) wajib per PLAN.md §11.
  Fakta penting dari centrs: **RouterOS menolak login password begitu key
  di-set untuk user tersebut** → dokumentasikan di remediasi `auth/failed`.
- **Host-key policy:** default TOFU (accept-new) — konsisten dengan centrs
  default; `Insecure` sebagai opt-out eksplisit bernama jelas (prinsip
  "escape hatch eksplisit", DESIGN.md §2.1).
- **Concurrency model** (PLAN.md §11): satu koneksi SSH = synchronous
  (command berurutan, tanpa tag multiplexing). Konkurensi = beberapa koneksi
  terpisah. `Run` dilindungi mutex per `Client` (jangan ubah asumsi ini tanpa
  keputusan baru di DECISIONS.md).

### 5.3 Error mapping (`errors.go`)

Port `mapSshConnectError` centrs → `roserr` (taksonomi Fase 1):

| Kondisi stderr/error | Kode roserr |
|---|---|
| `host key verification failed` / host-key mismatch | `transport/host-key-mismatch` |
| `permission denied` / `authentication failed` / `no such identity` / `too many authentication failures` | `auth/failed` |
| `connection refused` | `transport/connection-refused` |
| `timed out` / `connection timed out` | `transport/timeout` |
| `could not resolve` / `name or service not known` | `transport/dns` |
| lainnya | `transport/network` |

Setiap error membawa `roserr.Context{Via: "ssh", Host, Port, Op}` + remediasi
+ cause. **Error command RouterOS TIDAK lewat sini** — channel SSH sukses
(exit 0) dan error device muncul di stdout, diklasifikasikan gate (pola
centrs yang sama).

### 5.4 `CleanConsoleOutput` (`output.go`)

```go
// Port setia cleanConsoleOutput (ssh.ts L182):
// CRLF→LF, trim trailing whitespace tiap baris, buang baris kosong
// leading/trailing. Indentasi leading dipertahankan (isi print).
func CleanConsoleOutput(stdout string) string
```

### 5.5 Render CLI bentuk spasi (R12 — wajib untuk v6, kompatibel v7)

Fakta lab (§15): SSH exec v6 **menolak bentuk slash-joined**
(`/ip/address/print` → `expected command name`) dan hanya menerima bentuk
console spasi (`/ip address print`). v7 menerima keduanya. Karena itu
**transport/ssh merender command sebagai bentuk spasi secara universal**:

```go
// ConsoleCLI renders the space-separated console form accepted by v6 and v7:
// "/ip address print interface=ether1" (tokens dari PathTokens + verb +
// atribut), BUKAN slash-joined "/ip/address/print". Deterministik (sorted
// keys), sama seperti CLI(). Dipakai ssh.Run dan Gate 1 console.
func (c *Command) ConsoleCLI() string
```

- Ditempatkan di `transport/contract.go` sebagai pendamping `CLI()` (aditif,
  tidak mengubah `CLI()` yang dipakai gate/native-api).
- `Gate1` console memakai `ConsoleCLI()` (bukan `CLI()`) saat membangun
  script `:put [:parse ...]` — hasil `:parse` bentuk spasi di v6 = `(eval …)`
  (fallthrough classifier lolos), di v7 = `(evl …)`.
- Atribut & query dirender sama seperti `CLI()` (quoted saat perlu).

---

## 6. Gate console — reuse, bukan salin

**Prinsip (PLAN.md §7, §11): classifier SAMA PERSIS dengan Fase 3, tidak
pernah disalin.**

- `Gate1` di Fase 3 memakai field `Gate1.Transport` bertipe
  `transport.StructuredTransport`, yang mewajibkan `Command` **dan** `Inspect`.
  Console transport hanya punya `Run(ctx, line)`. Solusi tanpa duplikasi,
  **diputuskan eksplisit di sini (D-016)**: **persempit field
  `Gate1.Transport` ke antarmuka minimal** `Command(ctx,
  *transport.Command) (*transport.Reply, error)` — Gate 1 tidak pernah
  membutuhkan `Inspect` (itu tugas Gate 2), jadi mempersempit ke seam
  terkecil sesuai prinsip DESIGN.md §2.9. Refactor ini **tidak memutus
  native-api**: adapter native-api sudah punya `Command`, jadi tetap
  memenuhi antarmuka yang dipersempit. Console adapter (`gate/console.go`)
  mengimplementasikan antarmuka itu dengan memetakan `Command{Script: s}` →
  `Run(ctx, s)` → `Reply{Type: ReplyRe, Attributes: {"ret": out}}`:

```go
// gate/console.go — adapter console → antarmuka minimal Gate1 (D-016).
// Command{Script: s} → Run(ctx, s) → Reply{Type: ReplyRe, Attributes: {"ret": out}}.
// (Alternatif yang ditolak: adapter memalsukan Inspect dengan
//  capability-unsupported — membebani adapter dengan method tak relevan.)
```

- Alur per command (console):
  1. preflight lokal `HasUnbalancedQuotes(cli)` → gagalkan tanpa round-trip
     (sudah ada, dipakai apa adanya);
  2. `script := ":put [:parse " + StringLiteral(cli) + "]"`
  3. `out := client.Run(ctx, script)` → `CleanConsoleOutput`;
  4. `PureSyntaxClassifier.Classify(out)` → `ValidationError` / lolos.
- Di **v7**, hasil `:parse` via SSH diharapkan sama dengan native-api
  (`(evl …)` valid; error sesuai R10/R11) — diverifikasi R6b.
- Di **v6**, hasil probe R6b menentukan: kalau `:parse` console v6
  mengembalikan string yang klasifikabel, Gate 1 console bisa jalan di v6
  (catatan: jalur native-api v6 tetap skip). Kalau tidak → skip (D-009),
  konsisten.

---

## 7. API publik & integrasi orkestrasi

- `transport/ssh` berdiri sendiri: `Dial` + `Run` (console). Gate & schema
  TIDAK menambah dependency ke `transport/ssh` (aturan §2 PLAN.md).
- Facade Fase 6 (`Client.Validate`/`Inspect`/`RunStructured`) berjalan di
  atas `StructuredTransport` (native-api). Untuk console, pemakai memakai
  `ssh.Client.Run` + gate langsung (helper kecil `Validate(ctx, line)` di
  package `ssh` atau `gate` — keputusan terpisah, **D-017**).
- `Capabilities{Console:true}` → Gate 2 (inspect) **tidak applicable** di
  SSH: `Inspect=false` → skip senyap, konsisten dengan desain D-009/D-010.

---

## 8. Struktur test & matriks

### 8.1 Unit (tanpa jaringan)

- `CleanConsoleOutput`: CRLF, trailing space, baris kosong tepi, indentasi
  leading dipertahankan (fixture dari centrs + hasil R6).
- `mapSshError`: 6 kelas pesan stderr → kode roserr tepat + konteks.
- Adapter gate console: fixture teks `(evl …)` / `syntax error (line X col Y)`
  / `(<% …` / `bad parameter <name>` → hasil klasifikasi setara fixture
  Fase 3 (regresi classifier lintas transport).

### 8.2 Integration (build tag `integration`, device nyata)

Matriks M22–M29 (SSH × versi × skenario), mengikuti §15 PLAN.md:

| # | Skenario (via SSH) | v6 | v7 |
|---|---|---|---|
| M22 | Dial + print valid → output bersih (R6; v6 wajib bentuk spasi, R12) | ✅ 14 B (`name: quixiq`) | ✅ 20 B (`name: mikrotik-sim`) |
| M23 | Command salah syntax (mis. `nonsense command`) → Gate tolak | ✅ `validation/syntax` (pola wrapped `(<%` berlaku di v6 juga) | ✅ `validation/syntax` |
| M24 | Atribut salah nama (mis. `print bogus=1`) → error setara native-api | ✅ `validation/syntax` (coarse R10) | ✅ `validation/syntax` |
| M25 | Output panjang (`/ip route print detail`) tidak terpotong (R7) | ✅ 1371 B lengkap | ✅ 1913 B lengkap |
| M26 | Auth gagal (user/password salah) → `auth/failed` | ✅ | ✅ |
| M27 | Host-key tidak dikenal → TOFU accept / `insecure` skip | ✅ TOFU + Insecure dial sukses | ✅ TOFU + Insecure dial sukses |
| M28 | Koneksi putus tengah jalan (ctx timeout) → `transport/timeout`, cleanup bersih | ✅ koneksi tetap usable | ✅ koneksi tetap usable |
| M29 | Kode & makna ValidationError via SSH **setara** native-api (DoD §11) | ⛔ SKIP (native-api Gate 1 v6 skip, D-009) | ✅ kode sama (`validation/syntax`) |

Kredensial hanya lewat env var `ROUTEROS_TEST_*`; tidak ada kredensial di
repo. Hasil aktual lengkap: `docs/RESEARCH.md` §16. **Semua sel PASS** di
kedua device (2026-08-12) — termasuk temuan bahwa pola error `:parse` v6
via SSH identik dengan v7 (`(<% bad command name …)`, `expected end of
command …`) sehingga classifier D-008 berlaku apa adanya tanpa fixture baru.

---

## 9. Urutan eksekusi

1. **Migrasi module path v3→v4** (§2) + verifikasi build/test hijau;
   hapus harness spike v6 (`_lab/spike_v6_gate1{,b,c}`, `_lab/spike_v6_cleanup`)
   — hasil sudah dicatat di RESEARCH.md §14, tidak perlu di-migrasi.
2. **Probe lab R6/R7/R6b/R6c** ke MT-1 & MT-2 → catat ke `docs/RESEARCH.md`.
3. Implementasi `transport/ssh` (errors.go → output.go → ssh.go) + unit test.
4. Adapter gate console (`gate/console.go`, D-016) + unit test classifier
   lintas transport.
5. Integration test M22–M29 → jalankan ke v6 & v7, perbaiki.
6. Update dokumen: `DECISIONS.md` (D-016..; hasil R6/R7), `DESIGN.md`
   (tree + §4.8 SSH), `RESEARCH.md` (R6/R7 status), `PLAN-FASE7.md` (matriks
   terisi), `PLAN.md` status Fase 7.
7. Validasi penuh: `gofmt`, `go build ./...`, `go vet ./...`,
   `go vet -tags integration ./...`, `go test -race ./...`, lab integration
   v6 & v7, code review.

---

## 10. Risiko & mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| R6 gagal: SSH RouterOS mengembalikan output kotor (prompt/ANSI) di 6.49/7.21.5 | Gate console klasifikasi salah | Probe R6 dulu (fakta centrs 7.23.1 sebagai hipotesis); `CleanConsoleOutput` + verifikasi dengan classifier |
| `:parse` console di v6 tidak klasifikabel | Gate 1 console v6 skip | R6b menentukan; skip senyap (D-009), jangan error per-command |
| Host-key TOFU menolak di lab pertama | Dial gagal `host-key-mismatch` | R6c + dokumentasi `Insecure` opt-out; simpan known_hosts untuk test |
| SSH session leak / hang (command tak berakhir) | Goroutine bocor | Timeout per-command via ctx + tutup session di `defer`; test M28 |
| Dep `x/crypto/ssh` baru (dependency baru) | Review keamanan | Gunakan rilis stabil; go.sum di-commit; dokumentasikan |
| Scope creep (PTY, terminal interaktif, sftp) | Molor | Di luar scope Fase 7 — hanya command console satu baris (PLAN.md §11) |

---

## 11. DoD Fase 7 (checklist)

- [ ] Module path v4 ter-migrasi, build/test hijau (D-014).
- [ ] R6/R7/R6b/R6c terjawab di `RESEARCH.md` dengan probe nyata.
- [ ] `transport/ssh`: Dial (password + key + TOFU/insecure), Run, error
      mapping 6 kode, CleanConsoleOutput — unit test hijau.
- [ ] Gate console memakai `PureSyntaxClassifier` yang sama (tidak ada
      classifier kedua) — terbukti lewat test regresi bersama.
- [ ] M22–M29: integration test PASS di v6 & v7 (atau skip terdokumentasi).
- [ ] ValidationError via SSH setara native-api untuk kesalahan yang sama (v7).
- [ ] `docs/DECISIONS.md`, `docs/DESIGN.md`, `docs/RESEARCH.md`,
      `docs/PLAN-FASE7.md` ter-update; tanpa kredensial di repo.
