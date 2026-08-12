# DESIGN — Prinsip & Arsitektur `goros`

> **Dokumen hidup.** Berisi prinsip desain dan struktur package yang stabil —
> tidak berubah kecuali ada alasan kuat, dan perubahan wajib tercatat di
> `docs/DECISIONS.md`.
>
> Referensi:
> - Rencana eksekusi lengkap (fase, spesifikasi, kriteria selesai): `PLAN.md`
> - Acuan wire behavior yang sudah dibuktikan di CHR: `.refrences/centrs`
> - Log keputusan: `docs/DECISIONS.md`
>
> **Status:** v1, disetujui 2026-08-12. Berlaku mulai Fase 1.

---

## 1. Identitas project

- Library Go untuk mengakses perangkat Mikrotik/RouterOS: **multi-transport +
  validasi dua gate + return terstruktur**.
- Module path: `github.com/quiqxiq/goros/v4` (D-001 kemudian direvisi oleh
  D-014 — bump v4 dikonfirmasi pemilik 2026-08-12). Nama package root **tetap
  `routeros`** (bukan `goros`) demi kompatibilitas kode pemanggil.
- Strategi: **adaptasi** — kode native-api existing (`client.go`, `proto/`)
  dipertahankan dan diadaptasi ke arsitektur baru, bukan ditulis ulang.
- Scope transport: native-api (adaptasi) + SSH (wajib). MAC-Telnet opsional
  (spike dulu). **RoMON & Winbox: di luar scope.**

---

## 2. Prinsip desain yang tidak bisa dinegosiasikan

Prinsip ini berlaku di semua fase dan semua package. Siapa pun (manusia atau AI)
yang mengerjakan fase mana pun wajib merujuk ke sini.

1. **Validasi adalah produk utama, bukan fitur tambahan.**
   Tidak ada jalur di API publik untuk "mematikan validasi supaya command lolos"
   tanpa pemanggil sadar. Escape hatch (jika ada) harus eksplisit namanya
   ("skip validation") dan ditaruh di level pembuatan `Client` — sekali, terlihat
   di kode inisialisasi — bukan sebagai parameter yang dikirim ulang di tiap
   pemanggilan method.

2. **Return selalu terstruktur.**
   Lapisan terbawah (`proto`, transport) boleh bicara byte/string mentah karena
   itu memang levelnya. Begitu naik ke `gate` dan `schema`, semuanya tipe Go
   bernama dengan field jelas. Hasil validasi/discovery tidak pernah
   `[]map[string]string` mentah ke pemanggil publik.

3. **Transport-agnostic di level atas.**
   Validasi, discovery, dan tipe data hasil tidak tahu (dan tidak peduli) apakah
   di baliknya native-api, SSH, MAC-Telnet, atau REST. Semua transport mengekspos
   kontrak yang sama ke lapisan atas (inspirasi: `ProtocolAdapter` di centrs).

4. **Jangan pecah kompatibilitas native-api existing.**
   `client.go`/`proto/` sudah matang dan dipakai orang lain. Perkenalkan
   interface baru, migrasikan implementasi lama ke interface itu, pertahankan
   API publik lama sebagai wrapper — jangan tulis ulang dari nol.

5. **`context.Context` wajib di setiap operasi jaringan.**
   Di semua transport, sejak desain interface pertama kali — bukan ditambahkan
   belakangan. Termasuk operasi broadcast/discovery yang rawan hang.

6. **Error punya taksonomi konsisten.**
   Bukan `errors.New(...)` bebas di banyak tempat. Satu package `roserr`
   menyediakan tipe error terstruktur (kode, summary, remediasi, konteks, cause)
   yang dipakai semua package lain — sekaligus menghindari import siklik.

7. **Setiap gate bisa dijelaskan alasannya, bukan cuma pass/fail.**
   Error dari gate mana pun wajib membawa kode, apa yang hilang/salah, dan apa
   yang tersedia sebagai alternatif — supaya pemanggil bisa memperbaiki sendiri.

8. **Jangan gabungkan "cek" dengan "eksekusi".**
   Command kategori aksi/komputasi (monitor-traffic, ping, torch) punya efek
   samping nyata. Method dry-run tidak boleh pernah mengeksekusi command aslinya.
   Dry-run dan run adalah **dua method terpisah**, bukan satu method dengan flag
   boolean (flag itu akan disalahgunakan sebagai "skip validation").

9. **Satu paket = satu tanggung jawab.**
   `proto` cuma codec byte. `transport` cuma koneksi + kontrak. `gate` cuma
   keputusan valid/tidak. `schema` cuma bentuk data command. Kalau satu fungsi
   butuh tahu detail lebih dari satu tanggung jawab ini, itu tanda salah taruh,
   bukan tanda perlu digabung.

10. **Setiap fase berakhir testable dan terdokumentasi.**
    Kriteria selesai diverifikasi lewat test, bukan opini "kelihatannya sudah
    benar". Lihat kriteria per fase di `PLAN.md`.

---

## 3. Struktur package & aturan dependensi

```
goros (root package `routeros`)          ← FACADE PUBLIK (dipertahankan)
│   client.go, async.go, listen.go,       ← API existing: Client, Dial*, Run,
│   run.go, reply.go, cancel.go,           RunContext, Listen, RunAsync, Close
│   chan_reply.go, error.go, logger.go     → DIPERTAHANKAN + ditambah method
│                                           Validate / Inspect / RunStructured
│   transport/        ← BARU: kontrak transport (interface) + implementasi
│     contract.go      (StructuredTransport & ConsoleTransport, Reply kanonik,
│                       Command.ConsoleCLI — bentuk spasi, R12)
│     nativeapi/       (adaptasi: wrapper tipis di atas Client existing —
│                       mendelegasikan ke seam kanonik di root, D-012)
│     ssh/             (console transport, x/crypto/ssh — Fase 7 SELESAI)
│     mactelnet/       (OPSIONAL: hanya jika spike Fase 8 lolos)
│     rest/            (OPSIONAL: hanya jika Fase 9 dikerjakan)
│   gate/            ← BARU: Gate1 (syntax :parse) + Gate2 (semantik
│                      /console/inspect) + adapter console (D-016/D-017)
│   schema/          ← BARU: CommandSchema, Attribute, discovery + cache
│   roserr/          ← BARU: taksonomi error terstruktur lintas package
│   proto/           ← EXISTING, TIDAK DIUBAH: wire codec (Reader, Writer,
│                      Sentence) — diperlakukan sebagai lapisan wire
```

Aturan arah import (dipatuhi ketat, dicek di review):

- `proto` → tidak bergantung ke apa pun di luar dirinya.
- `transport` → `proto`, `roserr`.
- `gate` → `transport`, `schema`, `roserr`. **Tidak** bergantung ke implementasi
  transport spesifik.
- `schema` → `transport`, `roserr`.
- `roserr` → tidak bergantung ke package lain mana pun.
- Root package → boleh bergantung ke semua (satu-satunya facade untuk pemakai
  akhir).

Catatan pengecualian yang disengaja pada aturan "transport → proto, roserr":

- `transport/nativeapi` mengimpor facade root `routeros` karena membungkus
  `*routeros.Client` (D-007). Sejak **Fase 6** implementasi kanonik seam &
  probe hidup di root (`client_transport.go`, D-012) — root tidak bisa
  mengimpor nativeapi (import cycle), adapter mendelegasikan 1:1 supaya
  translasi tidak disalin.
- `transport/ssh` mengimpor `gate` untuk `Client.Validate` →
  `gate.ValidateConsole` (D-017): helper validasi console hidup di `gate`
  (satu sumber alur `:parse`), dan `ssh` mendelegasikan ke sana. Bukan import
  cycle (gate tidak mengimpor `transport/ssh`), tapi melintasi lapisan —
  dipertahankan karena `Validate` adalah convenience publik yang tidak boleh
  menduplikasi alur gate.

---

## 4. Kontrak inti (diimplementasikan di Fase 1)

Bentuk final mengikuti `ProtocolAdapter` di
`.refrences/centrs/src/protocols/adapter.ts`, diadaptasi ke idiom Go. Berlokasi:
`transport/contract.go`, `roserr/roserr.go`, `transport/mock/`.

### 4.1 Transport (di `transport/contract.go`)

- `Transport` — seam bersama: `Capabilities() Capabilities` + `Close() error`
  (idempotent).
- `StructuredTransport` (native-api, REST) — `Command(ctx, *Command) (*Reply,
  error)` dan `Inspect(ctx, InspectRequestKind, path) ([]InspectNode, error)`.
- `ConsoleTransport` (SSH, MAC-Telnet) — `Run(ctx, line string) (string,
  error)`, satu baris console → teks balasan.
- `Capabilities{Structured, Console, Inspect bool}` — analog
  `ProtocolAdapterCapabilities`. Transport yang tidak punya kemampuan tertentu
  menolak dengan `roserr.CodeCapabilityUnsupported`, bukan gagal diam-diam.

### 4.2 Reply & Command kanonik (di `transport/contract.go`)

- `Reply{Type, Attributes, Tag, RawWords}` — `Type` ∈ `!re`/`!done`/`!trap`/
  `!fatal`/`!empty`; helper `Ret()` (`=ret=`) dan `Message()` (`=message=`).
- `Command{Path, Verb, Attributes, Queries, Proplist, Script}` — bentuk
  terstruktur; `PathTokens()` (split `/`), `CLI()` (bentuk baris console
  deterministik, dipakai Gate 1 `:parse`; nilai ber-spasi di-quote, atribut
  urut sorted), dan **`ConsoleCLI()`** (pendamping `CLI()`: token path
  digabung **spasi** — `/ip address print interface=ether1` — bentuk yang
  wajib dipakai SSH exec v6 (R12) dan juga diterima v7).
- Semua transport wajib menerjemahkan hasil mentahnya ke bentuk ini sebelum
  dikembalikan ke pemanggil.

### 4.3 Taksonomi error (package `roserr`)

Kode final (acuan `.refrences/centrs/src/core/routeros-errors.ts` +
`error-catalog.ts`):

| Grup | Kode |
|---|---|
| RouterOS | `routeros/unknown-path`, `routeros/unknown-attribute`, `routeros/invalid-value`, `routeros/command-failed`, `routeros/session-closed` |
| Validasi | `validation/syntax`, `validation/unknown-attribute` |
| Autentikasi | `auth/failed` |
| Transport | `transport/connection-refused`, `transport/timeout`, `transport/dns`, `transport/network`, `transport/tls-certificate`, `transport/host-key-mismatch`, `transport/capability-unsupported` |

`roserr.Error{Code, Summary, Remediation, Context{Via,Host,Port,Path,Extra},
Cause}` — `Code` adalah identitas semantik: bandingkan dengan `errors.Is` (Is
mencocokkan Code) atau helper `roserr.IsCode(err, code)`, jangan string-match
pesan. `WithRemediation`/`WithContext`/`WithCause` sebagai opsi konstruktor.

### 4.4 Mock transport (di `transport/mock/`)

`mock.Transport` mengimplementasikan kedua seam, thread-safe, merekam semua
panggilan (`Calls()`, penghitung `CommandCalls`/`InspectCalls`/`RunCalls` untuk
uji cache), dan bisa di-script lewat `SetCommandFn`/`SetInspectFn`/`SetRunFn`.
Varian `NewStructured()` (native-api) dan `NewConsole()` (SSH/MAC-Telnet) untuk
menguji perilaku per kemampuan.

### 4.5 Adapter native-api (di `transport/nativeapi/`)

`nativeapi.Adapter` membungkus `*routeros.Client` existing dan
mengimplementasikan `transport.StructuredTransport` **tanpa mengubah perilaku
wire** — `proto/` dan `client.go` tidak disentuh (PLAN.md Fase 2; D-007).
Translasi dua arah di titik masuk/keluar:
- `transport.Command` → sentence terstruktur via `Command.Words()`;
  command `Script` dikirim sebagai `/execute =script= =as-string=` (pola
  `executeScript` centrs) — jalur Gate 1 (Fase 3).
- `routeros.Reply` / `proto.Sentence` → `transport.Reply` kanonik;
  `!trap` → `roserr.CodeCommandFailed`, `!fatal` → `roserr.CodeSessionClosed`;
  `*routeros.DeviceError` asli tetap terjangkau via `errors.As` (cause).
- `List()` → semua sentence (baris `!re` + terminal); `Command()` → sentence
  terminal; `Inspect()` → `/console/inspect`, mem-parse `InspectNode`.
- **Flag sesi** (Fase 3/4, D-009): `ProbeInspect`/`SupportsInspect` dan
  `ProbeParse`/`SupportsParse` — di-probe sekali setelah login; trap device
  → `false` diam-diam (degradasi v6), error non-trap → dikembalikan.

Sejak **Fase 6 (D-012)** implementasi kanonik seam (Command/List/InspectNodes),
translasi (`TranslateError`/`TranslateReply`), dan probe hidup di
`*routeros.Client` (root, `client_transport.go`); adapter mendelegasikan 1:1
ke sana — tidak ada logika yang disalin. Alasan: root tidak bisa mengimpor
`transport/nativeapi` (import cycle), sementara PLAN.md §10 mewajibkan API
publik di `Client`.

### 4.6 Gate & schema (di `gate/` dan `schema/`, Fase 3–4)

- **`gate.Gate1`** — validasi syntax via `:put [:parse <literal>]` pada jalur
  script transport (`/execute =as-string=` di native-api, atau `Run` console
  via adapter `gate/console.go` — D-016). Preflight lokal `HasUnbalancedQuotes`
  (kutip tak seimbang → gagal tanpa round-trip); input = `Command.CLI()`
  (native-api) atau `Command.ConsoleCLI()` bentuk spasi (console, R12 —
  disuntikkan lewat field `RenderCLI`, D-016). Hasil parse diklasifikasi
  fungsi murni **`PureSyntaxClassifier`** (multi-pattern, urutan matching
  wajib, D-008), termasuk bentuk error terbungkus
  `(<% bad command name … (line X column Y))` dari command bentuk spasi
  (R11, RESEARCH.md §13); kegagalan level script/transport **tidak** direlabel
  jadi `validation/*`. `SupportsParse=false` (native-api v6) → skip senyap;
  console v6 **viable** (R6b/R12 — `:parse` bentuk spasi berjalan).
  Field `Gate1.Transport` bertipe antarmuka minimal `gate.CommandTransport`
  (hanya `Command`) — console transport bisa memenuhinya lewat adapter
  `NewConsoleCommand`, tanpa `Inspect` palsu (D-016).
- **`schema.Store`** — discovery `CommandSchema` per `(path, verb)` via
  `/console/inspect`: union `request=child` (argumen) + `request=completion`
  + trik field `.proplist`/`value-name` (acuan centrs `retrieve.ts`); sampah
  struktural completion (`[ ( $ " * <value> about`) difilter (fakta lab 7.21.5,
  RESEARCH.md §11); primitif murni di `schema/inspect.go` (port 1:1
  `centrs/src/core/inspect.ts`). Kategori eksplisit `table`/`action`/`unknown`
  (per verb) + override `RegisterCategory` (D-010). Cache TTL pendek (30 s,
  D-011) — dua `Discover` → 1× round-trip (diverifikasi call-count).
- **`gate.Gate2`** — validasi atribut: `requested ∉ schema.Attributes` →
  `validation/unknown-attribute` dengan `missing`/`available`/`validationSource`
  di context; kategori `unknown` atau `SupportsInspect=false` (v6) → skip
  senyap. Dipanggil orkestrator (Fase 6).
- Aturan dependensi (PLAN.md §2, DESIGN §3): `schema` → `transport`, `roserr`;
  `gate` → `transport`, `schema`, `roserr`; tidak ada import cycle.

### 4.7 Orkestrasi & API publik (Fase 6, di package root; D-012/D-013)

- **Seam kanonik di `Client`** (D-012): `Command`, `List`, `InspectNodes`,
  translasi error/reply (`TranslateError`, `TranslateReply`), dan probe sesi
  (`ProbeInspect`/`SupportsInspect`, `ProbeParse`/`SupportsParse`) hidup di
  root package — bukan di `transport/nativeapi` — karena nativeapi mengimpor
  root (import cycle: root tidak bisa mengimpor nativeapi).
  `nativeapi.Adapter` mendelegasikan 1:1; tidak ada duplikasi translasi.
- **Tiga method publik** (D-013, PLAN.md §10):
  1. `Validate(ctx, cmd) error` — dry-run: gate yang applicable dijalankan,
     command **TIDAK pernah dieksekusi** (aman untuk command action).
  2. `Inspect(ctx, path, verb) (*schema.CommandSchema, error)` — discovery
     murni, tanpa command konkret; v6 → `CategoryUnknown`, bukan error.
  3. `RunStructured(ctx, cmd) (*transport.Reply, error)` — validasi lalu
     eksekusi command sebagai sentence-nya sendiri (bukan dibungkus
     `/execute`).
- **Routing gate** (PLAN.md §10): command `Script` (CLI bebas) → Gate 1
  (`:parse`) saja; command terstruktur → Gate 1 dilewati, Gate 2 (atribut)
  saja. Sesi tanpa dukungan (v6) → gate skip senyap (D-009).
- **Pipeline lazy**: probes → `schema.Store` → gate1 → gate2 dibangun sekali
  di first-use (`ensureRun`, mutex-protected). Pengguna API legacy `Run*`
  tidak terpengaruh (tidak ada probe tambahan saat Dial).
- **Metrik sesi (Fase 10, D-019):** `Client.Metrics() Metrics` — snapshot
  read-only berisi `InspectRoundTrips` (counter round-trip `/console/inspect`,
  di-inkremen di `InspectNodes`; membuktikan cache schema D-011 efektif) dan
  latensi Gate 1/2 (diukur di `validate()`).

### 4.8 Transport SSH console (Fase 7, di `transport/ssh/` — SELESAI)

- `ssh.Dial(ctx, addr, user, opts ...DialOption)` — functional options
  (D-018): `WithPassword`, `WithPrivateKey(File)`, `WithHostKeyPolicy`
  (TOFU accept-new default / `HostKeyInsecure` opt-out eksplisit),
  `WithKnownHosts` (pin ketat format OpenSSH), `WithTimeout`. Minimal satu
  metode auth wajib; kunci privat di-parse saat Dial (error nyata, bukan
  senyap).
- `Client` mengimplementasikan `transport.ConsoleTransport`: `Run(ctx, line)`
  tanpa PTY, output dibersihkan `CleanConsoleOutput` (CRLF→LF, trim trailing
  per baris, buang baris kosong tepi — port `cleanConsoleOutput` centrs).
  Error transport dipetakan ke taksonomi `roserr` (`mapSshError`: 6 kode —
  `auth/failed`, `transport/host-key-mismatch`, `transport/connection-refused`,
  `transport/timeout`, `transport/dns`, `transport/network` — port
  `mapSshConnectError` centrs). Error device **in-band** via output console
  (exit ≠ 0 dengan output → output dikembalikan, gate yang mengklasifikasi),
  bukan transport error.
- Concurrency: satu koneksi = synchronous (mutex per `Client`); konkuren =
  beberapa koneksi. `Close` idempotent.
- `Client.Validate(ctx, line)` (D-017) → `gate.ValidateConsole` — Gate 1
  `:parse` yang **sama persis** dengan native-api (classifier tunggal, D-008).
- Render CLI bentuk **spasi** (`Command.ConsoleCLI`) di mana pun — wajib
  untuk SSH exec v6 (R12), juga diterima v7.

---

## 5. Perubahan dokumen ini

Perubahan prinsip di §2 atau struktur di §3 **wajib** dicatat di
`docs/DECISIONS.md` sebelum diterapkan — dokumen ini bukan sumber kebenaran yang
membatu, tapi revisinya harus terlacak.
