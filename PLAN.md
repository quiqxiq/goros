# Rencana Pengembangan Final: `goros` — Multi-Transport & Validasi Command Terstruktur

> Dokumen ini adalah **gabungan final** dari `adaptasi.md` (strategi & manajemen
> risiko) dan `Go-routeros-planing.md` (spesifikasi teknis & struktur package),
> disesuaikan dengan **kondisi nyata project ini** — fork dari
> `github.com/go-routeros/routeros/v3` yang di-publish di `github.com/quiqxiq/goros`,
> module path `github.com/quiqxiq/goros/v4` (D-001 → revisi D-014: bump v4,
> dikonfirmasi pemilik 2026-08-12) — dan dengan
> **`.refrences/centrs` sebagai acuan implementasi**.
>
> Keputusan besar yang membedakan dokumen ini dari dua dokumen sumber:
> 1. **Strategi: ADAPTASI, bukan rewrite dan bukan module baru.** Kode native-api
>    yang sudah matang (`client.go`, `proto/`, `async.go`, `listen.go`, dst.) dipertahankan
>    dan diadaptasi ke arsitektur baru. API publik existing (Client, Dial*, Run,
>    Listen, RunAsync) tetap berfungsi.
> 2. **Transport: native-api (existing) + SSH. MAC-Telnet opsional (di-spike dulu).**
>    **RoMON dan Winbox DIKELUARKAN dari scope** — tidak dikerjakan, tidak direncanakan.
>    REST tetap opsional (ada acuannya di centrs).
> 3. **Acuan implementasi: `.refrences/centrs`** — setiap fase menyebut file sumber
>    centrs yang relevan sebagai rujukan perilaku/wire behavior yang sudah
>    dibuktikan di CHR (dokumen centrs mencatat fakta grounded pada CHR 7.23.1/7.23.3).
>
> Dokumen ini adalah rencana awal, bukan sumber kebenaran yang membatu — revisi
> wajib dicatat di `docs/DECISIONS.md` (lihat Fase 0).

---

## 0. Keputusan yang harus dikonfirmasi sebelum Fase 1 dimulai

Empat keputusan ini mengikat seluruh fase. **✅ Semua sudah dikonfirmasi oleh
pemilik project pada 2026-08-12 — detail lengkap di `docs/DECISIONS.md`.**

| # | Keputusan | Hasil konfirmasi | Catatan |
|---|---|---|---|
| 1 | **Module path** | `github.com/quiqxiq/goros/v3` → **`github.com/quiqxiq/goros/v4`** | Awalnya v3 (D-001, suffix `/vN` wajib untuk major ≥ 2); **direvisi oleh D-014**: pemilik memilih bump v4 saat API baru final (Fase 6) — migrasi mekanis selesai, tag rilis `v4.0.0`. |
| 2 | **Versi Go minimum** | `go 1.22` (naik dari 1.21) | Diterapkan di Fase 1. Membuka loop-var semantics tanpa membatasi pengguna berlebihan. (D-002) |
| 3 | **Kebijakan versi** | ~~Lanjut `v3.x` backward-compat~~ → **BUMP v4** (D-014) | Keputusan final bump v4 ditunda ke Fase 6 saat API baru final. **2026-08-12: pemilik memilih bump v4** — module path `github.com/quiqxiq/goros/v4`, tag `v4.x.y` (D-014; migrasi mekanis selesai sebagai prasyarat Fase 7 §2; **rilis `v4.0.0` 2026-08-12**). (D-003 → D-014) |

> **Status rilis (2026-08-12):** seluruh scope yang dikerjakan selesai —
> Fase 0–7 ✅ (termasuk transport SSH kedua, DoD §17), Fase 10 cross-cutting
> ✅ (metrik D-019, audit roserr/logging/godoc), CI job integration ✅,
> README/DESIGN/DECISIONS/RESEARCH/PLAN-FASE7 konsisten, lab integration
> M22–M29 PASS di v6 & v7. **Fase 8 (MAC-Telnet) dan Fase 9 (REST)
> di-skip** atas keputusan pemilik (boleh menyusul sebagai minor release,
> §17); RoMON/Winbox tetap di luar scope (§16). Rencana implementasi sisa
> yang sudah dieksekusi: `docs/PLAN-REMAINING.md`; hasil lab:
> `docs/RESEARCH.md` §16.
| 4 | **Lisensi** | Pertahankan MIT (copyright André Luiz dos Santos, 2016) + catatan fork | Tambah baris catatan fork di LICENSE/README. (D-004) |

Keputusan lain yang juga dikonfirmasi dan tercatat: strategi adaptasi (D-005),
scope transport tanpa RoMON/Winbox, MAC-Telnet opsional (D-006).

**Keempat keputusan §0 sudah dikonfirmasi — Fase 1 boleh dimulai.**

---

## 1. Prinsip desain yang tidak bisa dinegosiasikan

Tulis prinsip ini di `docs/DESIGN.md` sejak awal supaya semua fase (termasuk
pengerjaan lintas sesi oleh AI) merujuk dokumen yang sama.

- **Validasi adalah produk utama, bukan fitur tambahan.** Tidak ada jalur di API
  publik untuk "mematikan validasi supaya command lolos" tanpa pemanggil sadar.
  Escape hatch (jika ada) harus eksplisit namanya ("skip validation") dan ditaruh
  di level pembuatan `Client` sekali, bukan parameter per-panggilan.
- **Return selalu terstruktur.** Lapisan terbawah (`wire`, transport) boleh bicara
  byte/string mentah; begitu naik ke `gate` dan `schema`, semuanya tipe Go bernama
  dengan field jelas. Tidak pernah `[]map[string]string` mentah ke pemanggil publik
  untuk hasil validasi/discovery.
- **Transport-agnostic di level atas.** Validasi, discovery, dan tipe data hasil
  tidak tahu transport mana di baliknya. Semua transport mengekspos kontrak yang
  sama (inspirasi: `ProtocolAdapter` di centrs).
- **Jangan pecah kompatibilitas native-api existing.** `client.go`/`proto/` sudah
  matang dan dipakai orang lain. Perkenalkan interface baru, migrasikan
  implementasi lama ke interface itu, pertahankan API publik lama sebagai
  wrapper — jangan tulis ulang dari nol.
- **`context.Context` wajib di setiap operasi jaringan**, semua transport, sejak
  desain pertama — termasuk operasi broadcast/discovery yang rawan hang.
- **Error punya taksonomi konsisten**, bukan `errors.New(...)` bebas. Satu
  package `roserr` (bukan `error.go` tersebar) supaya tidak ada import siklik.
- **Setiap gate harus bisa dijelaskan alasannya**, bukan cuma pass/fail: error
  wajib membawa kode, apa yang hilang/salah, dan apa alternatifnya.
- **Jangan gabungkan "cek" dengan "eksekusi".** Command kategori aksi/komputasi
  (monitor-traffic, ping, torch) punya efek samping. Method dry-run tidak boleh
  pernah mengeksekusi command aslinya; run dan dry-run adalah dua method terpisah,
  bukan satu method dengan flag boolean.
- **Satu paket = satu tanggung jawab** (lihat §2).
- **Setiap fase berakhir testable dan terdokumentasi** — kriteria selesai
  diverifikasi lewat test, bukan opini.

---

## 2. Struktur package (adaptasi in-place, bukan layout baru penuh)

Prinsip adaptasi: **pindahkan sesedikit mungkin, tambahkan sisanya**. Package
yang sudah ada dan matang (`proto/`) dibiarkan sebagai lapisan wire; package baru
dibuat di sampingnya. API publik lama tetap di root package sebagai facade.

```
goros (root package `routeros`)          ← FACADE PUBLIK (dipertahankan)
│   client.go, async.go, listen.go,       ← API existing: Client, Dial*, Run,
│   run.go, reply.go, cancel.go,           RunContext, Listen, RunAsync, Close
│   chan_reply.go, error.go, logger.go     → DIPERTAHANKAN + ditambah method
│                                           Validate / Inspect / RunStructured
│   transport/        ← BARU: kontrak transport (interface) + implementasi
│     contract.go      (StructuredTransport & ConsoleTransport, Reply kanonik)
│     nativeapi/       (adaptasi: membungkus/mengimplementasi di atas Client
│                       existing — bukan menyalin logikanya)
│     ssh/             (BARU: console transport, x/crypto/ssh)
│     mactelnet/       (OPSIONAL: hanya jika spike Fase 8 lolos)
│     rest/            (OPSIONAL: hanya jika Fase 9 dikerjakan)
│   gate/            ← BARU: Gate1 (syntax :parse) + Gate2 (semantik
│                      /console/inspect) + orkestrator
│   schema/          ← BARU: CommandSchema, Attribute, discovery + cache
│   roserr/          ← BARU: taksonomi error terstruktur lintas package
│   proto/           ← EXISTING, TIDAK DIUBAH: wire codec (Reader, Writer,
│                      Sentence) — diperlakukan sebagai lapisan wire
```

> Catatan nama package: module path `github.com/quiqxiq/goros/v4` (D-001 →
> D-014), tapi nama package root **sengaja tetap `routeros`** (di `client.go`)
> demi kompatibilitas kode pemanggil — Go memperbolehkan nama package berbeda
> dari elemen terakhir module path. Jangan "perbaiki" nama ini tanpa keputusan
> di DECISIONS.md.

Aturan ketergantungan (arah import):

- `proto` tidak bergantung ke apa pun di luar dirinya.
- `transport` bergantung ke `proto` dan `roserr`.
- `gate` bergantung ke `transport`, `schema`, `roserr` — **tidak** bergantung ke
  implementasi transport spesifik.
- `schema` bergantung ke `transport` dan `roserr` saja.
- `roserr` tidak bergantung ke package lain mana pun (tempat tipe error bersama).
- Root package bergantung ke semua — ini satu-satunya facade untuk pemakai akhir.

Dokumentasi hidup yang wajib ada sejak Fase 1:
- `docs/DESIGN.md` — prinsip di §1.
- `docs/DECISIONS.md` — log keputusan non-trivial (tanggal, keputusan, alasan,
  alternatif yang ditolak). Krusial karena pengerjaan lintas sesi/pelaksana.

---

## 3. Peta acuan `.refrences/centrs` per fase

Ini peta yang menghubungkan setiap fase ke file sumber centrs yang harus dibaca
dulu sebelum implementasi. centrs mencatat fakta wire behavior yang sudah
dibuktikan di CHR (7.23.1/7.23.3) — jangan mengubah perilaku wire berdasarkan
tebakan bila acuan sudah ada.

| Fase | File acuan di centrs | Yang diambil |
|---|---|---|
| Fase 0 (riset) | `commands/api/AGENTS.md`, `src/core/inspect.ts` (komentar grounding), `src/protocols/ssh.ts` (komentar CHR) | Fakta perilaku `/console/inspect` & SSH console yang sudah diverifikasi |
| Fase 1 (kontrak) | `src/protocols/adapter.ts`, `src/protocols/index.ts`, `src/core/error-catalog.ts`, `src/core/routeros-errors.ts`, `src/errors.ts` | Bentuk interface transport, capabilities, kode error |
| Fase 2 (adaptasi native-api) | `src/protocols/native-api.ts`, `src/protocols/adapter.ts` (class `NativeApiAdapter`) | Pemetaan method talk→inspect/execute, `as-string` di `/execute` |
| Fase 3 (Gate 1) | `src/execute.ts` (`runSyntaxGate`, `classifyParseResult`, `parseScriptFor`, `routerOsStringLiteral`) | Pola `:put [:parse …]`, klasifikasi hasil, preflight escape |
| Fase 4 (Gate 2 + schema) | `src/core/inspect.ts` (primitif), `src/retrieve.ts` + `src/execute.ts` (strategi union `child`+`completion`), `src/explain/` (pelengkap: analisa output console) | Primitif inspect, strategi discovery per kategori command |
| Fase 5 (versi v6/v7) | `src/core/inspect.ts` (`inspectChildrenOrEmpty`) | Probe sekali + degradasi senyap |
| Fase 6 (orkestrasi) | `src/execute.ts`, `src/api.ts` | Alur validate→run, pemisahan dry-run, facade |
| Fase 7 (SSH) | `src/protocols/ssh.ts`, `src/execute.ts` (`validateConsoleParseCommand`) | Eksekusi console tanpa PTY, gate `:parse` tunggal |
| Fase 8 (MAC-Telnet, ops.) | `src/protocols/mac-telnet.ts`, `src/protocols/mac-telnet-console.ts`, `src/protocols/mtwei.ts`, `src/protocols/ec-srp5.ts`; `src/discover.ts` (MNDP, untuk discovery perangkat L2) | Handshake, framing, console reader, gate console |
| Fase 9 (REST, ops.) | `src/protocols/adapter.ts` (class `RestAdapter`), `src/api.ts` | Pemetaan verb→HTTP, `.query`/`.proplist` |
| Fase 10 (cross-cutting) | `src/core/error-catalog.ts` (konsistensi kode), `src/protocols/adapter.ts` (timeout handling) | Pola error konsisten, timeout + teardown |

> Catatan peta: beberapa perilaku di centrs (mis. `ssh host "cmd"` shell-out)
> adalah keputusan khusus ekosistem Bun. Di Go gunakan `golang.org/x/crypto/ssh`
> dengan *perilaku wire yang sama* (tanpa PTY, output bersih). Yang diambil dari
> centrs adalah **fakta perilaku RouterOS**, bukan implementasinya.

---

## 4. Fase 0 — Riset & Validasi Asumsi (wajib sebelum menulis kode)

Tujuan: mengumpulkan fakta di lab (router/CHR sungguhan) supaya fase berikut
tidak menebak. Setiap poin wajib punya jawaban tertulis di dokumen riset internal
(`docs/RESEARCH.md`) sebelum Fase 1.

**Yang wajib diriset & dikonfirmasi di lab:**

1. **SSH:** apakah `ssh user@host "<satu baris command>"` di RouterOS menjalankan
   command console dan mengembalikan output bersih tanpa PTY (centrs mencatat
   "no pseudo-tty, clean output" di CHR 7.23.1 — verifikasi ulang untuk versi
   target). Bagaimana perilaku paging console untuk output panjang.
2. **`:parse`:** verifikasi bahwa `:put [:parse "<command>"]` (a) tidak pernah
   melempar (centrs mencatat GH#230: `:parse` tidak throw di CHR 7.23.3 — hasil
   diagnostiknya muncul sebagai string), (b) mengembalikan `(evl …)` untuk syntax
   error dan `bad parameter <name>` untuk atribut tidak dikenal, (c) hasilnya sama
   persis lewat native-api (`as-string`) vs console (SSH/MAC-Telnet).
3. **`/console/inspect`:** konfirmasi argumen `path` harus bentuk **koma**
   (`ip,address`), bukan garis miring (centrs: bentuk slash tidak cocok dan selalu
   kosong). Konfirmasi `request=child` vs `request=completion` bentuk hasilnya
   (kolom `type`/`name`/`completion`/`value`/`text`) untuk versi target.
4. **Rentang versi target:** satu versi 7.x terbaru sebagai target utama + minimal
   satu 7.x lain (stabilitas format pesan) + satu v6 untuk jalur fallback Gate 2.
5. **MAC-Telnet (hanya jika Fase 8 akan dikerjakan):** konfirmasi ulang skema
   handshake, kebutuhan enkripsi per versi, kebutuhan raw socket/privilege per OS.
   **RoMON & Winbox: TIDAK diriset — di luar scope.**

**Kriteria selesai Fase 0:** `docs/RESEARCH.md` menjawab semua poin di atas
dengan hasil pengujian nyata, plus keputusan tertulis rentang versi yang didukung
penuh vs sebagian. Empat keputusan di §0 sudah dikonfirmasi dan dicatat di
`docs/DECISIONS.md`.

---

## 5. Fase 1 — Fondasi: kontrak transport, Reply kanonik, error taxonomy

**Tujuan:** "slot" transport + tipe data bersama, supaya gate/discovery bisa
ditulis sekali dan bekerja di semua transport.

**Yang perlu dikerjakan (acuan: `centrs/src/protocols/adapter.ts`):**

- Definisikan interface transport di `transport/contract.go`:
  - `StructuredTransport` — kirim command terstruktur (path/verb/atribut), terima
    `Reply` kanonik; panggil `/console/inspect`; tutup koneksi.
  - `ConsoleTransport` — jalankan satu baris teks console, terima teks balasan
    (dipakai SSH/MAC-Telnet; diadaptasi dari semangat `ProtocolAdapter` centrs
    yang membedakan kemampuan `retrieve` vs `execute`).
  - Kemampuan (capabilities) per transport: `Inspect()`, `Structured()`,
    `Console()` — analog `ProtocolAdapterCapabilities`.
- Definisikan **Reply kanonik** di `transport/contract.go`: tipe reply
  (`!re`/`!done`/`!trap`/`!fatal`/`!empty` — `!empty` sudah ada di repo ini),
  atribut, tag, word mentah untuk debug. Transport mana pun wajib menerjemahkan
  hasil mentahnya ke bentuk ini.
- Definisikan **taksonomi error** di package `roserr` (acuan
  `centrs/src/core/routeros-errors.ts` + `error-catalog.ts`):
  - `routeros/unknown-path`, `routeros/unknown-attribute`,
    `routeros/invalid-value`, `routeros/command-failed`, `routeros/session-closed`
  - `validation/syntax`, `validation/unknown-attribute` (hasil gate)
  - `auth/failed`
  - `transport/connection-refused`, `transport/timeout`, `transport/dns`,
    `transport/network`, `transport/tls-certificate`, `transport/host-key-mismatch`,
    `transport/capability-unsupported`
  - Setiap error: kode, summary, remediasi, konteks (via/host/port/path), cause.
- Buat **mock transport** (`transport/mock/`) yang dipakai menguji gate/discovery
  tanpa router sungguhan (pola fixture reply tiruan di centrs).

**Kriteria selesai:** kontrak terdokumentasi di `docs/DESIGN.md`; semua interface
di atas terkompilasi; satu mock transport berfungsi; `roserr` dipakai oleh mock
tersebut. Test: `go test ./transport/...` hijau.

---

## 6. Fase 2 — Adaptasi native-api existing ke kontrak transport

**Tujuan:** membuktikan kontrak Fase 1 masuk akal dengan mengadaptasi kode
native-api yang sudah matang — **tanpa menulis ulang logikanya.**

**Yang perlu dikerjakan (acuan: `centrs/src/protocols/native-api.ts` + class
`NativeApiAdapter` di `adapter.ts`):**

- Petakan kemampuan existing `Client` ke kontrak:
  - kirim command & terima Reply → `Run`/`RunContext` (`run.go`) + `reply.go`
  - eksekusi script mentah → `/execute` dengan `as-string=""` (perlu method baru;
    centrs `executeScript` menunjukkan pola `{script, "as-string": ""}`)
  - deteksi dukungan inspect → probe `/console/inspect` sekali di awal sesi
    (pola centrs `inspectChildrenOrEmpty`)
  - tutup koneksi → `Close` existing
- **Perilaku wire-level tidak boleh berubah.** Perubahan hanya di titik
  masuk/keluar: bagaimana command dikirim dan bagaimana hasil diterjemahkan ke
  Reply kanonik. `proto/` tidak disentuh.
- Implementasikan `transport/nativeapi` sebagai adapter yang **membungkus
  `Client`** (atau method baru di `Client` yang mengimplementasikan interface —
  pilih salah satu dan catat di DECISIONS.md). Login legacy MD5 (`challengeResponse`
  di `client.go`) sudah ada — pertahankan; centrs punya implementasi serupa
  (`challengeResponse` di native-api.ts) sebagai pembanding.
- Cache hasil probe inspect per sesi (analog flag boolean sesi di planing.md Fase 5).
- **Test regresi wajib:** seluruh test existing (`client_test.go`, `proto/*_test.go`)
  tetap hijau; tambah test yang membandingkan perilaku sebelum/sesudah adaptasi
  (command yang sama → hasil setara).

**Kriteria selesai:** native-api berjalan penuh lewat kontrak baru; semua test
existing hijau (`go test ./...`); deteksi inspect berfungsi dan diuji ke ≥2 versi
RouterOS (satu mendukung inspect, satu tidak).

---

## 7. Fase 3 — Gate 1: validasi syntax via `:parse` (native-api)

**Tujuan:** validasi syntax command lewat `:parse`, termasuk membaca isi `ret`
secara eksplisit — nilai tambah nyata yang bahkan centrs akui belum dikerjakan
di jalur native-api-nya.

**Spesifikasi (acuan: `centrs/src/execute.ts` — `runSyntaxGate`,
`classifyParseResult`, `parseScriptFor`):**

- Bangun script: `:put [:parse "<command asli>"]`, command di-escape sebagai
  string literal RouterOS (`routerOsStringLiteral`: escape backslash & kutip
  ganda). Lakukan **preflight kutip lokal** sebelum mengirim (centrs
  `validateConsoleParseCommand` punya "local quote preflight" — kalau kutip
  tidak seimbang, gagalkan lokal tanpa round-trip).
- Kirim lewat `/execute` dengan atribut `script` + `as-string=""`.
- Dua jalur hasil yang dibedakan:
  1. `/execute` sendiri gagal (`!trap`) → kegagalan level script/transport,
     bukan hasil parse command yang divalidasi.
  2. `/execute` sukses → isi `ret` adalah hasil `:parse`. Cocokkan terhadap pola:
     frasa atribut tidak dikenal (ekstrak nama atribut → `ValidationError`),
     frasa syntax rusak (variasi: command salah, nama command tidak dikenal,
     bagian wajib kosong). Tidak ada yang cocok → command lolos syntax.
- Jika pesan error memuat posisi baris/kolom, ekstrak ke field posisi.
- Classifier harus **fungsi murni** yang bisa dipakai ulang dari native-api dan
  console transport (alasan: centrs memakai `classifyParseResult` yang sama di
  kedua jalur).

**Kriteria selesai:** unit test tanpa jaringan dengan fixture teks `ret`
(command valid, atribut tidak dikenal, command tidak dikenal, syntax rusak
dengan/tanpa posisi) → `ValidationError` benar untuk tiap fixture.

---

## 8. Fase 4 — Gate 2: validasi semantik + discovery `CommandSchema`

**Tujuan:** validasi keberadaan path/atribut lewat `/console/inspect` sekaligus
membangun `CommandSchema` terstruktur — nilai tambah utama library ini.

**Spesifikasi (acuan: `centrs/src/core/inspect.ts` — port primitifnya hampir
1:1; `src/retrieve.ts` + `src/execute.ts` — tempat strategi discovery per
kategori hidup menurut komentar header inspect.ts):**

- Port primitif inspect ke Go:
  - `pathTokens` (split `/`, buang kosong), `inspectPath` (**gabung koma** —
    wajib, slash tidak cocok), `isArgumentNode` (`type=="arg"`), `isCommandNode`,
    `extractCompletionNames` (baca `completion`/`name`/`value`/`text`, potong
    mulai `=` pertama), `inspectChildren`, `inspectCompletions`,
    `inspectChildrenOrEmpty` (telan trap unknown-path → daftar kosong).
- `/console/inspect` dikirim sebagai command native-api biasa dengan atribut
  `request` (`child`|`completion`) dan `path` (koma).
- `request=child` → node di bawah path; filter `type=="arg"` = daftar atribut valid.
- `request=completion` → dua kebutuhan digabung (union + dedup):
  1. melengkapi atribut yang tidak muncul di `child`; dan
  2. **trik discovery nama field**: untuk `print`, completion pada `.proplist`;
     untuk singleton `get`, lewat argumen nilai (`value-name`).
- `CommandSchema.Category` eksplisit, tiga nilai: `table` (field bisa didapat
  statis), `action` (ada argumen input, field output hanya "ada" saat dijalankan —
  jangan pernah kembalikan field kosong tanpa menjelaskan lewat Category),
  `unknown` (inspect tidak didukung). Sediakan **override manual** kategori
  (pemetaan path→kategori yang bisa diisi pemanggil).
- Cache: kunci = token path + verb, nilai = `CommandSchema` lengkap, TTL default
  pendek (konstanta yang bisa dikonfigurasi lewat Client), + invalidasi manual.

**Kriteria selesai:** unit test dengan fixture reply tiruan untuk `child` dan
`completion`: union+dedup benar; trik `.proplist`/`value-name` benar; kategori
table vs action sesuai fixture; cache hit tidak round-trip kedua (verifikasi
lewat penghitung panggilan transport tiruan); override manual dipakai.

---

## 9. Fase 5 — Deteksi versi & fallback v6/v7

**Tujuan:** `/console/inspect` adalah fitur v7. Sesi harus tahu sejak awal apakah
fitur ini tersedia supaya Gate 2 berdegradasi rapi, bukan gagal per command.

**Cakupan (acuan: `centrs/src/core/inspect.ts` — `inspectChildrenOrEmpty`):**

- Sekali di awal sesi (setelah login): panggil `request=child` ke path yang pasti
  ada di semua versi (identitas sistem). Simpan hasilnya sebagai flag boolean
  level sesi — jangan probe ulang.
- Tidak didukung → Gate 2 di-skip otomatis untuk sesi itu, Gate 1 tetap jalan
  penuh (`:parse` bukan fitur v7-only). Jangan lempar error ke pemanggil hanya
  karena Gate 2 tidak tersedia — tangani senyap di orkestrasi (Fase 6).
- Fallback discovery v6 opsional (hanya jika pemanggil memilih eksplisit, karena
  ini benar-benar mengeksekusi `print` tanpa filter): union nama field dari hasil.

**Kriteria selesai:** dua skenario test dengan transport tiruan (probe sukses /
probe gagal) → flag sesi benar di kedua kasus; Gate 2 di-skip (bukan gagal) pada
skenario kedua.

---

## 10. Fase 6 — Orkestrasi validasi + API publik

**Tujuan:** menyatukan Gate 1, Gate 2, dan eksekusi jadi satu alur konsisten —
di sinilah API publik utama library terbentuk.

**Cakupan (acuan: `centrs/src/execute.ts` — alur `resolve → validate → run`;
`src/api.ts` — pola facade):**

- Urutan pasti: command bentuk string CLI bebas → Gate 1 dulu; command
  terstruktur (path/verb/atribut) → Gate 1 dilewati. Lalu Gate 2 kalau sesi
  mendukungnya. Hanya kalau gate yang applicable lolos, command asli dikirim —
  **langsung sebagai sentence-nya sendiri** (path+verb sebagai `Cmd`), bukan
  dibungkus `/execute`. `/execute` hanya untuk Gate 1 dan command yang tidak bisa
  distrukturkan (subshell / tidak diawali `/`).
- Tiga method publik yang jelas bedanya (di `Client`):
  1. **dry-run** — hanya jalankan gate, tidak pernah eksekusi command asli
     (aman dipanggil berulang, termasuk command kategori action);
  2. **Inspect** — kembalikan `CommandSchema` untuk suatu path tanpa command
     konkret (murni discovery);
  3. **run (terstruktur)** — validasi lalu eksekusi.
  Dry-run dan run adalah **dua method terpisah**, bukan satu method dengan flag.
- Error publik selalu `roserr` terstruktur — tidak pernah string mentah RouterOS
  tanpa dibungkus.

**Kriteria selesai:** integration test ke CHR sungguhan: satu command tabel valid
(berhasil, hasil benar); satu command atribut salah nama (gagal Gate 2 dengan
daftar Missing/Available benar); satu command syntax rusak (gagal Gate 1); satu
command action dipanggil via dry-run (lolos gate, TIDAK tereksekusi — diverifikasi
lewat efek samping yang seharusnya tidak terjadi); command sama via run
(benar-benar tereksekusi).

---

## 11. Fase 7 — Transport SSH (console)

**Tujuan:** transport kedua sekaligus pola untuk transport console-based lain.

**Cakupan (acuan: `centrs/src/protocols/ssh.ts` — fakta perilaku;
`src/execute.ts` — `validateConsoleParseCommand`):**

- Gunakan `golang.org/x/crypto/ssh` (bukan shell-out seperti centrs). Fakta
  perilaku yang diambil dari centrs: **RouterOS tidak memberi pseudo-tty**, tetapi
  satu baris command di sesi SSH berjalan di console dan mengembalikan output
  bersih (tanpa prompt/ANSI/echo). Validasi ulang di Fase 0.
- Eksekusi: kirim teks command ke sesi console, baca teks balasan. Dukungan
  public-key (`ssh.ParsePrivateKey` / agent) + password; error autentikasi dipetakan
  ke `auth/failed`, host-key mismatch → `transport/host-key-mismatch`
  (referensi kode error: `mapSshConnectError` di ssh.ts).
- **Gate untuk transport ini cuma satu** (bukan dua terpisah): kirim
  `:put [:parse "<command>"]` yang sama seperti Fase 3, hasil dibaca dari teks
  output console, classifier pola **sama persis** (fungsi murni dari Fase 3 —
  jangan disalin). Satu `:parse` console melaporkan syntax error DAN
  `bad parameter <name>` sekaligus (grounded CHR 7.23.1 — komentar
  `validateConsoleParseCommand`).
- Model concurrency: satu koneksi SSH pada dasarnya synchronous (satu command
  selesai dulu sebelum berikutnya; tidak ada tag multiplexing). Kebutuhan konkuren
  = beberapa koneksi terpisah.
- Pembersihan output console: CRLF→LF, trim padding kolom, buang baris kosong
  tepi (port `cleanConsoleOutput`).

**Kriteria selesai:** integration test ke CHR lewat SSH: command valid + command
salah syntax/atribut; `ValidationError` yang dihasilkan punya kode & makna setara
dengan hasil native-api untuk kesalahan yang sama.

---

## 12. Fase 8 — Transport MAC-Telnet (OPSIONAL, wajib spike dulu)

> **RoMON dan Winbox secara eksplisit DI LUAR SCOPE project ini** — fase ini
> hanya membahas MAC-Telnet, dan hanya jika dibutuhkan.

**Sebelum mulai — wajib spike kelayakan (acuan: `centrs/src/protocols/mac-telnet.ts`
(1299 baris), `mac-telnet-console.ts`, `mtwei.ts`, `ec-srp5.ts`):** akses
MAC-Telnet butuh operasi raw socket di layer data-link (bukan TCP/UDP biasa),
privilege khusus per OS, dan perilaku enkripsi yang berubah antar versi RouterOS.
Jangan alokasikan waktu penuh sebelum ada bukti spike bahwa pendekatan yang
dipilih jalan di lingkungan target. Kalau spike gagal/mahal → tunda fase ini ke
backlog, tanpa menghentikan fase lain.

**Cakupan (jika spike lolos):**
- Discovery perangkat L2 (broadcast) sebagai langkah sebelum connect — centrs
  `discover.ts` (MNDP) relevan sebagai acuan mekanisme discovery.
- Handshake sesi (nomor urut, ack, session ID, enkripsi sesuai versi target),
  framing paket — port dari `mac-telnet.ts`/`mtwei.ts`.
- Ujung sesi adalah stream console teks — **pakai ulang parser/gate console dari
  Fase 7** (satu `:parse`, classifier sama). Titik beda MAC-Telnet dari SSH murni
  di pembentukan koneksi/framing, bukan parsing hasil.
- Error yang jelas & actionable untuk kurangnya privilege runtime.

**Kriteria selesai:** koneksi + eksekusi command ke minimal satu perangkat uji
nyata di lab (bukan simulasi), didokumentasikan jujur (versi teruji, kondisi
jaringan, yang belum didukung).

---

## 13. Fase 9 (OPSIONAL) — Transport REST

**Tujuan:** hanya dikerjakan kalau ada kebutuhan konkret setelah native-api solid
— jangan dikerjakan lebih dulu "karena kelihatan gampang".

**Cakupan (acuan: `centrs/src/protocols/adapter.ts` — class `RestAdapter`;
`src/api.ts` — pemetaan verb):**

- Tidak butuh wire protocol custom — HTTP + JSON.
- Karena bentuk input REST sudah terstruktur, Gate 1 dilewati (sama seperti
  command terstruktur di native-api). Gate 2 tetap wajib (`/console/inspect`
  sebagai endpoint biasa).
- Pemetaan verb→HTTP: GET→print, PUT→add, PATCH→set, DELETE→remove, POST→run
  (acuan `mapMethodToVerb` di api.ts). `.query`/`.proplist` sebagai body POST
  `/print` (acuan `restQueryBody`).

**Kriteria selesai:** setara Fase 6 tapi lewat REST — satu command valid dan satu
gagal validasi, hasil konsisten dengan transport lain.

---

## 14. Fase 10 — Cross-cutting: error, context, logging, observability, hardening

Berjalan paralel sepanjang Fase 2–9, plus satu putaran konsolidasi eksplisit
sebelum siap dipakai orang lain (gabungan Fase 8 `adaptasi.md` + Fase 10
`Go-routeros-planing.md`).

**Cakupan:**

- **Error:** audit semua transport & gate — taksonomi `roserr` dipakai konsisten,
  tidak ada yang melempar error generik di luar taksonomi. Referensi konsistensi:
  `centrs/src/core/error-catalog.ts`.
- **Context:** setiap operasi jaringan menghormati cancellation/timeout context,
  termasuk broadcast/discovery. Pola timeout+teardown (tutup koneksi saat timeout
  supaya state reader konsisten) — acuan `withTimeout` di
  `centrs/src/protocols/adapter.ts` dan `TalkCtx` existing.
- **Logging:** lanjutkan pola `slog` existing (`logger.go`, `SetLogHandler`) ke
  semua transport & gate, level bisa diatur dari luar, **tanpa membocorkan
  kredensial** (password, MD5 challenge mentah, kunci privat) dalam kondisi apa pun.
- **Metrik dasar:** jumlah round-trip `/console/inspect` per sesi (membuktikan
  cache Fase 4 efektif) + latensi tiap gate.
- **Hardening:** `go vet` & linter bersih; godoc 100% simbol publik.

**Kriteria selesai:** checklist audit dijalankan terhadap tiap transport & gate
dengan hasil "sesuai" di semua poin, celah diperbaiki (bukan dicatat sebagai
utang), metrik & log bisa diverifikasi manual lewat satu sesi percobaan nyata.

---

## 15. Strategi testing & kriteria akseptasi global

- Tidak ada fase selesai tanpa lulus unit test-nya sendiri. Fase yang bicara ke
  jaringan sungguhan (6, 7, 8, 9) wajib lulus integration test ke router nyata —
  mock saja tidak cukup untuk fase-fase itu.
- `go test -race` wajib untuk package `gate` dan `transport/nativeapi` di tiap CI
  run (rawan race: multiplexing tag, cache).
- Integration test dipisah dari unit test lewat build tag `integration`, dijalankan
  sebagai job CI terpisah (manual/terjadwal) — pipeline utama tidak boleh gagal
  hanya karena lab sedang tidak bisa diakses.
- Matriks pengujian: (transport) × (versi RouterOS target dari Fase 0) ×
  (skenario: command valid, parameter salah, path tidak ada, koneksi putus di
  tengah, autentikasi gagal, output gagal di-parse).
- Target versi: satu 7.x terbaru (utama), satu 7.x lain (stabilitas format
  `:parse` & `/console/inspect`), satu v6 (jalur fallback Fase 5).
- Perbarui `docs/DECISIONS.md` setiap ada keputusan yang menyimpang dari dokumen
  ini.

---

## 16. Di luar scope (sengaja tidak dikerjakan)

- **RoMON** — transport routed management overlay: **tidak dikerjakan**.
- **Winbox** (protokol terminal WinBox): **tidak dikerjakan**.
- **Struct/type generator** (auto-generate struct Go per path RouterOS) — proyek
  terpisah di masa depan yang mengonsumsi `CommandSchema` sebagai input. Library
  ini berhenti di titik "data & skemanya, sudah tervalidasi".
- Request mode `highlight` dan `syntax` di `/console/inspect` — extension point
  untuk nanti (centrs juga belum meng-wire keduanya).
- Integrasi ke sistem/driver/agent lain apa pun — library berdiri sendiri.
- Business logic di atas RouterOS (subscriber management, billing, dsb).

---

## 17. Definition of Done — keseluruhan project

Project siap rilis versi pertama kalau **semua** berikut terpenuhi:

- Fase 0–2 selesai penuh (riset, fondasi, native-api teradaptasi) — baseline
  wajib, tidak bisa dilewati.
- Minimal satu transport baru (SSH, Fase 7) selesai penuh — membuktikan klaim
  "multi-transport", bukan cuma native-api ditata ulang.
- Fase 6 (orkestrasi + API publik) selesai dengan integration test ke CHR.
- Fase 3–5 (Gate 1, Gate 2, versi v6/v7) selesai dan bekerja di minimal dua
  transport (native-api + SSH) — ini inti nilai tambah project.
- Fase 10 (cross-cutting) dan dokumentasi/rilis selesai untuk scope yang
  dikerjakan.
- MAC-Telnet (Fase 8) dan REST (Fase 9) **boleh** dirilis belakangan sebagai
  minor release susulan kalau butuh waktu jauh lebih lama dari perkiraan — ini
  bukan kegagalan project, tapi keputusan prioritas berdasarkan hasil spike/riset.

---

## 18. Risiko utama & mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Adaptasi Fase 2 tidak sengaja mengubah perilaku native-api existing | Regresi yang merugikan pengguna existing | Test regresi wajib di Fase 2 (seluruh test existing + perbandingan perilaku) sebelum lanjut fase mana pun |
| `/console/inspect` tidak konsisten lintas versi RouterOS 7.x | Gate 2 memberi hasil salah | Fase 0 + §15 mewajibkan pengujian di >1 versi 7.x, bukan satu versi acuan |
| `:parse` berperilaku berbeda di console (SSH) vs native-api | Gate 1 tidak konsisten lintas transport | Fase 0 verifikasi eksplisit; classifier dibuat fungsi murni tunggal (Fase 3) yang dipakai kedua jalur |
| MAC-Telnet ternyata jauh lebih rumit (raw socket, enkripsi, privilege) | Molor jadwal | Spike kelayakan wajib sebelum Fase 8; fase ini boleh ditunda ke backlog (lihat §17) |
| Scope creep ke arah struct generator / RoMON / Winbox / business logic | Project tidak pernah selesai | Batasan scope di §16 ditegaskan ulang di setiap tinjauan progres fase |
| Pengerjaan lintas sesi kehilangan konteks keputusan | Keputusan yang sama didebat ulang, desain inkonsisten | `docs/DECISIONS.md` + `docs/DESIGN.md` wajib diupdate di tiap fase; peta acuan centrs di §3 dibaca ulang sebelum tiap fase |

---

## 19. Urutan eksekusi ringkas

```
Fase 0 (riset & keputusan) → Fase 1 (kontrak) → Fase 2 (adaptasi native-api)
→ Fase 3 (Gate 1 :parse) → Fase 4 (Gate 2 + schema) → Fase 5 (v6/v7)
→ Fase 6 (orkestrasi + API publik) → Fase 7 (SSH) → [Fase 8 MAC-Telnet, wajib spike]
→ [Fase 9 REST, opsional] → Fase 10 (cross-cutting) → rilis
```

Gerbang wajib di tiap transisi: kriteria selesai fase sebelumnya terpenuhi dan
lulus test yang relevan, sebelum fase berikutnya dimulai. Referensi
`.refrences/centrs` untuk fase berjalan: lihat peta di §3.
