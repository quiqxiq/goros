# PLAN EKSEKUSI — Fase 3 (Gate 1: `:parse`) + Fase 4 (Gate 2: `/console/inspect` + `CommandSchema`) — Lab v6/v7

> Dokumen eksekusi yang **grounded di fakta lab nyata** (probe 2026-08-12
> terhadap v6 6.49.11 dan v7 7.21.5) — bukan rencana dari asumsi. Fakta
> lengkap: `docs/RESEARCH.md`. Induk rencana fase: `PLAN.md` (§7 = Fase 3,
> §8 = Fase 4). Log keputusan: `docs/DECISIONS.md`.
> Dokumen pendahulu: `docs/PLAN-FASE2-FASE3.md` (sisa Fase 2 + Gate 1
> riset) — plan ini melanjutkan dan memperdalam bagian Fase 3 di sana.
>
> **Keputusan pemilik project (2026-08-12, melanjutkan pola sesi lalu):**
> 1. Satu dokumen gabungan Fase 3 + Fase 4 (keduanya berbagi infrastruktur:
>    classifier murni, probe sesi, cache — dikerjakan bersama agar konsisten).
> 2. Kedua device lab ✅ reachable: MT-1 v6 (192.168.233.1) & MT-2 v7
>    (192.168.230.3). Matriks v7 M1–M7 + probe `:parse` sudah terverifikasi.
> 3. Skenario read-only terhadap kedua device — **eksekusi menyusul** setelah
>    dokumen ini direview (plan dokumen = deliverable sesi ini).

---

## 0. Ringkasan status

| Fase | Status implementasi | Status riset/validasi lab |
|---|---|---|
| Fase 1 — fondasi (kontrak, roserr, mock) | ✅ selesai | n/a (unit test) |
| Fase 2 — adaptasi native-api (`transport/nativeapi`) | ✅ selesai (D-007) | ✅ M1–M5 PASS di v6 & v7; inspect v6=false, v7=true |
| Fase 3 — Gate 1 (`:parse`) | ✅ selesai (D-008, D-009) | ✅ M8–M10 PASS di v7; v6 skip via `SupportsParse=false` |
| Fase 4 — Gate 2 (`/console/inspect` + `CommandSchema`) | ✅ selesai (D-009..D-011) | ✅ M14–M19 PASS di v7 (child=13 node, proplist=19 nama field, table/action, Missing/Available); v6 skip via `SupportsInspect=false` |

**Prasyarat yang sudah ada di kode:**
- `transport.Command.Script` + `Words()` → `/execute =script= =as-string=` ✅
  (dibangun di Fase 2 — jalur Gate 1 v7 siap dipakai).
- `transport.InspectNode`, `InspectRequestKind{Child,Completion}`,
  `StructuredTransport.Inspect` ✅ (kontrak Fase 1 + adapter Fase 2).
- `roserr.CodeValidationSyntax`, `CodeValidationUnknownAttribute`,
  `CodeUnknownPath`, `CodeUnknownAttribute` ✅ (taksonomi Fase 1).
- Prasyarat sesi **belum** diimplementasi: probe inspect + flag sesi
  (`ProbeInspect`/`SupportsInspect`, sisa Fase 2) — masuk ke Fase 4 di bawah.

---

## 1. Inventaris lab & aturan main

Lihat `docs/RESEARCH.md` §Inventaris. Ringkas:

| Alias | IP | Versi | Akses | Peran dalam test |
|---|---|---|---|---|
| MT-1 | `192.168.233.1` | 6.49.11 (stable) | ✅ | v6: **tanpa** inspect, `/execute as-string` gagal, `/execute` tanpa output → jalur degradasi |
| MT-2 | `192.168.230.3` | 7.21.5 (long-term) | ✅ | v7: **dengan** inspect (`nodes=34`), `as-string` OK, `:parse` tidak throw → jalur penuh |

**Aturan main (wajib):**
1. **Hanya command read-only** (print/identity/resource/inspect/parse).
   Tidak ada `add/set/remove` dalam testing fase ini.
2. Kredensial hanya lewat env var; jangan commit.
3. Frekuensi rendah (v6 CPU tinggi) — `-count=1`, tanpa loop berlebihan.
4. Setiap fakta baru → catat di `docs/RESEARCH.md`.

---

## 2. Fase 3 — Gate 1: validasi syntax via `:parse`

### 2.1 Spesifikasi (PLAN.md §7 + acuan centrs)

Acuan: `.refrences/centrs/src/execute.ts` — `runSyntaxGate` (L803),
`classifyParseResult` (dari `mac-telnet-console.ts` L606+),
`routerOsStringLiteral` (L1431), `hasUnbalancedQuotes`; dan
`.refrences/centrs/src/protocols/mac-telnet-console.ts` — grammar
`classifyParseResult`.

**Alur Gate 1 (native-api):**

1. **Preflight kutip lokal** (`hasUnbalancedQuotes`) — jika kutip `"`/`'`
   tidak seimbang → gagalkan lokal dengan `validation/syntax` **tanpa**
   round-trip ke device. (Escaped `\` dihormati.)
2. Bangun script: `:put [:parse <literal>]` di mana `<literal>` =
   `routerOsStringLiteral(command)` — command dibungkus `"..."`, escape
   `\` → `\\` dan `"` → `\"`.
3. Kirim via `transport.Command{Script: script}` → adapter mengirim
   `/execute =script=... =as-string=` (jalur v7, sudah terverifikasi).
4. **Dua jalur hasil yang dibedakan:**
   - `/execute` gagal (`!trap`) → kegagalan level script/transport, **bukan**
     hasil parse. Bedakan: `roserr.CodeCommandFailed` (device menolak script
     itu sendiri, mis. `unknown parameter` di v6) vs error transport. Jangan
     relabel jadi `validation/*` — pola `isPreflightTransportError` centrs.
   - `/execute` sukses → baca `ret` (hasil `:parse`) → **classifier murni**
     (lihat 2.2).
5. Hasil classifier → `nil` (lolos) atau `roserr.Error`:
   - `validation/unknown-attribute` + nama atribut (jika pola cocok)
   - `validation/syntax` + posisi baris/kolom bila tersedia
   - lain → lolos.

### 2.2 Classifier — fungsi murni, multi-pattern (temuan lab v7!)

`PureSyntaxClassifier.Classify(ret string) *ClassifyResult` — `{Valid bool,
Code roserr.Code, Attribute string, Line, Col int, Message string}`. **Tanpa
I/O**, dipakai native-api (Fase 3) DAN console transport (Fase 7) — jangan
disalin (PLAN §7).

**Urutan matching WAJIB (grounded centrs `classifyParseResult` + fakta
lab 7.21.5):**

| # | Pola | Contoh terverifikasi (7.21.5) | Hasil |
|---|---|---|---|
| 1 | `(evl bad parameter <name> …)` — bentuk terbungkus (console/echo) | (corpus centrs; cek di v7 bila muncul) | `unknown-attribute` name=... |
| 2 | `^bad parameter <name>` (opsional `(line X column Y)`) | centrs fixture | `unknown-attribute` |
| 3 | `^(?:syntax error\|bad command name\|expected end of command\|expected …)` (+ `(line X column Y)`) | **`syntax error (line 1 column 10)`** — `/nonsense/command`; **`expected end of command (line 1 column 24)`** — `/system/resource/print bogus=1` | `validation/syntax` + posisi |
| 4 | Diawali `(evl …)` → command **valid** (parse sukses) | **`(evl /system/resource/print)`** | lolos |
| 5 | Tidak cocok pola error → **lolos** (defensive: jangan false-positive) | — | lolos |

**Temuan kunci yang mengubah desain vs centrs:**
- **Pola `bad parameter <name>` TIDAK muncul di 7.21.5** — kasus atribut tak
  dikenal menghasilkan `expected end of command (line 1 column 24)` (pola #3).
  Konsekuensi desain: di 7.21.5, Gate 1 hanya bisa memberi sinyal **coarse**
  `validation/syntax` untuk kasus itu. Identifikasi **presisi** atribut
  (Missing/Available) bukan tugas Gate 1 — itu **tugas Gate 2** (Fase 4,
  `/console/inspect`), yang memang lebih tepat. (Lihat catatan M9 + R10.)
- `:parse` **tidak pernah throw** (GH#230 terkonfirmasi) — hasil selalu string
  di `ret`; kode lama yang hanya menunggu error tidak akan menolak apa pun.
- Posisi `(line X column Y)` konsisten → ekstrak Line/Col untuk lokasi error.
- `:parse` menerima `/system/resource print` (path terpisah spasi) dan
  menormalkannya → **jangan** preflight-menyalahkan spasi; preflight lokal
  HANYA soal keseimbangan kutip.
- **Input Gate 1 = `cmd.CLI()`**: command terstruktur (path/verb/attrs)
  dirender dulu ke string CLI deterministik (`Command.CLI()`, quoting
  otomatis), lalu dibungkus `:put [:parse StringLiteral(cli)]`. Keputusan
  "command terstruktur boleh dilewati Gate 1" adalah tanggung jawab
  orkestrator (PLAN §10, Fase 6), bukan Gate 1 sendiri.

**Escape helper (`routerOsStringLiteral`)**: fungsi murni `StringLiteral(s
string) string` di package gate — dipakai ulang oleh Gate 1 dan Fase 7.

### 2.3 Jalur v7 vs v6 — degradasi eksplisit (keputusan, perlu D-008)

| | v7 (MT-2) | v6 (MT-1) |
|---|---|---|
| `/execute` + `=as-string=` | ✅ `ret` = string hasil | ❌ `unknown parameter` |
| `/execute` tanpa `as-string` | ✅ tapi `ret="*45"` (ref, bukan string) | ✅ tapi `rows=0` (output tak kembali) |
| Kesimpulan | jalur penuh | **degradasi eksplisit** |

Desain (konsisten dengan pola `SupportsInspect`): flag sesi
**`SupportsParse`** (di `transport/nativeapi`):
- Di-probe sekali setelah login dengan `:put "probe"` via `/execute
  =as-string=`; sukses baca `ret` → `true`; trap `unknown parameter` → `false`.
- `false` → Gate 1 di-**skip** dengan catatan kapabilitas (pemanggil sadar),
  bukan error per-command. Escape hatch eksplisit (DESIGN.md §2.1).
- Spike mekanisme v6 (variabel global / script run) = **di luar scope sesi
  ini**; ditunda sampai Gate 1 v7 selesai dan pemilik setuju (tulis script =
  operasi tulis).

### 2.4 Struktur kode

```
gate/
  gate1.go        — Gate1{transport transport.StructuredTransport}
                     Run(ctx, cmd *transport.Command) error  (script-mode)
                     + PureSyntaxClassifier (fungsi murni)
                     + StringLiteral (escape) + HasUnbalancedQuotes
  gate1_test.go   — unit, fixture teks ret (no network):
                     valid (evl), bad parameter centrs, expected end of command
                     7.21.5, syntax error 7.21.5, wrapped (evl bad parameter),
                     kutip tidak seimbang (preflight), posisi line/col
  gate1_lab_test.go — integration (build tag), adapter v6/v7
```

- Interface dipakai: `transport.StructuredTransport` saja (Command dengan
  `Script`) — transport-agnostic, siap dipakai ssh (Fase 7) via
  `ConsoleTransport` + classifier yang sama.
- `transport/nativeapi`: tambah `ProbeParse`/`SupportsParse` (flag sesi,
  atomik, pola `ProbeInspect` Fase 4).

### 2.5 Kriteria selesai Fase 3

- [ ] Unit test classifier tanpa jaringan (fixture 7.21.5 + centrs) → benar
      untuk tiap fixture; preflight kutip teruji.
- [ ] Integration v7 (MT-2): command valid lolos; command tak dikenal →
      `validation/syntax` dengan posisi; **atribut salah nama → hasil akhir
      mengikuti R10**: bila pesan 7.21.5 bisa dibedakan →
      `validation/unknown-attribute`, bila tidak → coarse `validation/syntax`
      (identifikasi presisi diserahkan Gate 2/M19).
- [ ] Integration v6 (MT-1): `SupportsParse=false` → Gate 1 di-skip
      (diverifikasi tanpa error per-command).
- [ ] `go test -race` hijau untuk `gate/...` + seluruh suite (no regresi).

---

## 3. Fase 4 — Gate 2: validasi semantik + discovery `CommandSchema`

### 3.1 Prasyarat sesi (dibangun di fase ini, bagian dari sisa Fase 2)

`transport/nativeapi`: `ProbeInspect(ctx)` + `SupportsInspect() bool`
(dipanggil sekali setelah login; probe `request=child` ke path `system`).
- sukses → `true`; trap `no such command` → `false` (degradasi senyap);
  error lain (timeout/io) → dikembalikan (masalah nyata).
- v6 terverifikasi `false`; v7 terverifikasi `true` (nodes=34) — unit test
  dengan mock + lab keduanya sudah siap dari Fase 2 riset.

### 3.2 Primitif inspect — port 1:1 dari centrs `inspect.ts`

Di `schema/inspect.go` (fungsi murni, tanpa I/O):

| Primitif Go | Acuan centrs | Catatan |
|---|---|---|
| `PathTokens(path string) []string` | `pathTokens` | split `/`, buang kosong |
| `InspectPath(tokens []string) string` | `inspectPath` | **gabung koma** (`ip,address`) — slash tidak cocok (grounded CHR) |
| `IsArgumentNode(n transport.InspectNode) bool` | `isArgumentNode` | `type==arg` atau `node-type==arg` |
| `IsCommandNode(n, name) bool` | `isCommandNode` | `name==...` dan `type==cmd` |
| `ExtractCompletionNames(rows []transport.InspectNode) []string` | `extractCompletionNames` | baca `completion/name/value/text`, potong dari `=` pertama, buang kosong; **tanpa** dedup (caller yang dedup) |
| `InspectChildrenOrEmpty(...)` | `inspectChildrenOrEmpty` | telan trap `unknown-path`/`command-failed` → `[]` (untuk probe eksistensi, BUKAN discovery atribut) |

### 3.3 Discovery `CommandSchema` — strategi union `child`+`completion`

Di `schema/discover.go`. Acuan: `centrs/src/execute.ts`
`inspectExecuteAttributes` + `centrs/src/retrieve.ts` (L966–1052).

**Alur `Discover(ctx, path, verb) (*CommandSchema, error)`:**

1. `tokens = PathTokens(path) + [verb]`; `children = Inspect(
   request=child, InspectPath(tokens))`.
2. `childAttrs` = filter `IsArgumentNode` → `name` (non-kosong).
3. `completionRows = Inspect(request=completion, InspectPath(tokens))`;
   `completionNames = ExtractCompletionNames(completionRows)`.
4. **Union + dedup + sort**: `attrs = sort(unique(childAttrs ∪
   completionNames))`.
5. **Trik field discovery** (retrieve.ts): jika command `print`-style →
   `request=completion` pada argument `.proplist`; jika singleton `get`-style
   → pada argument `value-name`. Yaitu: `completion on tokens+[argument]`
   dengan `argument = "proplist" | "value-name"` tergantung dukungan
   `isCommandNode(children, "print")` / `"get"`. Hasilnya digabung ke `attrs`.
   ⏳ **Perlu probe lab di v7 (R9)** — bentuk `request=completion` di 7.21.5
   belum diverifikasi (baru `child`).
6. **Kategori** (eksplisit, tiga nilai):
   - `table` — ada command `print` atau `get` → field bisa didapat statis.
   - `action` — ada node command lain (punya argumen input; output hanya "ada"
     saat dijalankan). **Jangan** kembalikan field kosong tanpa menjelaskan
     lewat Category.
   - `unknown` — inspect tidak didukung / tidak menemukan apa pun.
   - **Override manual**: `schema.RegisterCategory(path, Category)` — peta
     pemanggil path→kategori menang atas hasil discovery.
7. Cache (lihat 3.4).

**Tipe Go (di `schema/schema.go`):**

```go
type Category string // "table" | "action" | "unknown"

type Attribute struct {
    Name string
    // ekstensi masa depan: tipe nilai, default, etc. — jangan tambah sekarang
}

type CommandSchema struct {
    Path, Verb string
    Category   Category
    Attributes []Attribute // terurut
    Source     string      // "inspect child+completion" | "override" | ...
}
```

### 3.4 Cache `CommandSchema`

`schema/cache.go`:
- Kunci: `path+"\x00"+verb` (token path + verb).
- Nilai: `*CommandSchema` (immutable setelah disimpan).
- TTL default pendek — konstanta `DefaultSchemaTTL` (mis. 30s), bisa
  dikonfigurasi via `Client` (Fase 6) dan `schema.Cache{TTL}`.
- Invalidasi manual: `cache.Delete(path, verb)` / `cache.Clear()`.
- Verifikasi efektivitas: **mock transport menghitung panggilan** — dua
  `Discover` beruntun untuk path yang sama → hanya 1× `Inspect` (cache hit
  tidak round-trip kedua). Ini kriteria selesai wajib.

### 3.5 Gate 2 — validasi atribut

`gate/gate2.go`:
- `Gate2{transport transport.StructuredTransport; schema *schema.Store}`.
- `Validate(ctx, cmd *transport.Command) error`:
  1. `!SupportsInspect` (sesi v6) → **skip senyap** (return nil + catatan
     kapabilitas di context, bukan error) — PLAN §5.
  2. `schema = Discover(ctx, cmd.Path, cmd.Verb)`.
  3. `requested` = kunci `cmd.Attributes`; `missing` = requested ∉
     `schema.Attributes` → error:
     `roserr.Error{Code: CodeValidationUnknownAttribute, Context.Extra:
     {path, verb, missing, available, validationSource}}`.
  4. Category `table` → validasi penuh; `action` → validasi atribut input
     (dari arg nodes; output field tak divalidasi); `unknown` → skip dengan
     catatan.
- Dipanggil orkestrator (Fase 6); di fase ini cukup method + diuji.

### 3.6 Struktur kode & aturan dependensi

```
schema/
  schema.go     — CommandSchema, Category, Attribute
  inspect.go    — primitif murni (3.2)
  discover.go   — Discover (3.3)
  cache.go      — cache TTL + invalidation (3.4)
  *_test.go     — unit fixture reply child+completion (mock transport)
gate/
  gate1.go      — Fase 3 (2.4)
  gate2.go      — Gate2 (3.5)
  gate2_test.go — unit dengan mock + schema store
  gate2_lab_test.go — integration v6/v7
```

Dependensi (PLAN §2): `schema` → `transport`, `roserr` saja; `gate` →
`transport`, `schema`, `roserr`; `transport/nativeapi` menambah
`ProbeParse`/`SupportsParse`/`ProbeInspect`/`SupportsInspect` (flag sesi).
`roserr` tidak bergantung ke siapa pun. Tidak ada import cycle.

### 3.7 Kriteria selesai Fase 4

- [ ] Unit fixture `child` + `completion` (bentuk 7.21.5 & centrs): union+dedup
      benar; trik `.proplist`/`value-name` benar; kategori table vs action
      sesuai fixture; cache hit = 1× panggilan inspect (penghitung mock);
      override manual dipakai.
- [ ] Integration v7 (MT-2): `Discover` path nyata (`/ip/address/print` →
      table; `/tool/ping` → action) → `CommandSchema` benar; Gate 2 menolak
      atribut salah nama dengan Missing/Available; valid lolos.
- [ ] Integration v6 (MT-1): `SupportsInspect=false` → Gate 2 di-skip (bukan
      error).
- [ ] `go test -race` hijau (`gate/...`, `schema/...`, `transport/nativeapi`)
      + seluruh suite no regresi.

---

## 4. Infrastruktur test integration

Per PLAN.md §15 (sudah berjalan dari Fase 2):
- Build tag `integration`; file `*_lab_test.go`; env `ROUTEROS_TEST_*`.
- `Makefile`: `make test`, `make test-race`, `make test-integration`,
  `make lab-probe` ✅ (sudah ada).
- Pola test lab yang dipakai: `labConfig` (skip saat env kosong, sebelum
  dial), `labClient` (`t.Cleanup` close), timeout per-call 10s, `-count=1`.

## 5. Test matrix real-device (lanjutan dari PLAN-FASE2-FASE3.md §5)

| # | Skenario (semua read-only) | v6 (MT-1) | v7 (MT-2) |
|---|---|---|---|
| M1 | Dial + login (legacy & adapter) | ✅ verified | ✅ verified (7.21.5) |
| M2 | Adapter `Command` print → `!done` | ✅ verified | ✅ verified |
| M3 | Adapter `List` → baris + `!done` | ✅ verified | ✅ verified |
| M4 | Command tidak dikenal → `CodeCommandFailed` + `*DeviceError` | ✅ verified | ✅ verified |
| M5 | Inspect probe → flag sesi | ✅ `false` | ✅ `true` (nodes=34) |
| M6 | `/execute` + `as-string` | ❌ `unknown parameter` | ✅ `ret="hello"` |
| M7 | `/execute` tanpa `as-string` | ❌ `rows=0` | ✅ `ret="*45"` (ref) |
| M8 | Gate 1 valid command → lolos | ✅ skip (degradasi) | ✅ `(evl …)` → nil |
| **M9** | Gate 1 atribut tak dikenal → per R10: `unknown-attribute` atau coarse `syntax` (presisi = Gate 2/M19) | ✅ skip (assert `SupportsParse=false`) | ✅ coarse `validation/syntax` (R10: tak bisa dibedakan) |
| M10 | Gate 1 command tak dikenal → `validation/syntax` | ✅ skip (assert `SupportsParse=false`) | ✅ `syntax error (L1 C10)` → `validation/syntax` + posisi |
| M11 | Autentikasi gagal → `auth/failed` | ⏳ | ⏳ |
| M12 | Koneksi putus tengah jalan → error jelas | ⏳ | ⏳ |
| M13 | TLS (8729) dial | ⏳ | ⏳ |
| **M14** | Inspect `request=child` path nyata (`/ip/address`) → node rows | ❌ (v6 no inspect) | ✅ 13 node |
| **M15** | Inspect `request=completion` (path + arg `.proplist`) → nama field | ❌ | ✅ 19 nama (sampah struktural difilter di Discover) |
| **M16** | `Discover` → `CommandSchema` kategori `table` (`/ip/address/print`) | n/a | ✅ table, 29 attrs (union child+proplist) |
| **M17** | `Discover` → kategori `action` (`/tool/ping`) | n/a | ✅ action, argumen input |
| **M18** | Gate 2: atribut valid → lolos | ✅ skip | ✅ nil |
| **M19** | Gate 2: atribut salah nama → Missing/Available | ✅ skip | ✅ missing=[interface1], available=29 |
| **M20** | Cache: 2× Discover → 1× panggilan inspect — **unit (mock transport**, bukan device) | ✅ unit | ✅ unit |
| **M21** | v6: Gate 2 di-skip (`SupportsInspect=false`), bukan error | ✅ | — |

Legend: ✅ terverifikasi · ⏳ belum · ❌ tidak didukung versi tersebut.

## 6. Eksekusi — langkah & hasil

1. ✅ (lalu) Probe v6/v7 identitas, adapter, inspect, `/execute`, `:parse` —
   semua data di RESEARCH.md.
2. ✅ Implementasi prasyarat sesi: `ProbeInspect`/`SupportsInspect` +
   `ProbeParse`/`SupportsParse` di `transport/nativeapi` (unit + lab M5/M8-v6).
3. ✅ Implementasi Gate 1: classifier multi-pattern + `StringLiteral` +
   `HasUnbalancedQuotes` + `Gate1` → unit fixture → integration v7 M8–M10
   (v6 = skip).
4. ✅ Implementasi schema: primitif → `Discover` → cache → kategori + override
   → unit fixture → integration v7 M14–M17. Probe lab R9/R10 dikerjakan dulu
   dan dicatat ke RESEARCH.md §11 (bentuk completion + sampah struktural;
   pesan typo atribut identik dengan syntax rusak).
5. ✅ Implementasi Gate 2 → unit → integration v7 M18–M19, v6 M21.
6. ✅ `go test -race ./...` penuh + `go vet` tiap langkah.
7. ✅ Perbarui `docs/RESEARCH.md` (R9, R10) & `docs/DECISIONS.md` (D-008:
   classifier multi-pattern; D-009: degradasi v6 Gate 1/2; D-010: kategori +
   override; D-011: cache TTL).

## 7. Risiko & mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Format pesan `:parse` beda per versi 7.x (7.21.5 ≠ centrs 7.23.x) | Classifier salah klasifikasi | Multi-pattern + fixture per versi; verifikasi di 7.x lain saat tersedia (target §15: dua 7.x) |
| `request=completion` bentuk di 7.21.5 belum diverifikasi (R9) | Discovery atribut tidak lengkap | Probe completion dulu (M15) sebelum implementasi `Discover`; fallback: child-only |
| Pesan atribut tak dikenal di 7.21.5 tak bisa dibedakan dari syntax rusak (R10) | Gate 1 tidak bisa memberi `unknown-attribute` presisi di 7.21.5 | Coarse `validation/syntax` di Gate 1 + identifikasi presisi via Gate 2 (`/console/inspect`, Missing/Available) — pembagian peran eksplisit di §2.2 |
| v6 tanpa Gate 1/2 penuh | Validasi minimal di v6 | Degradasi eksplisit terdokumentasi (bukan gagal); spike mekanisme v6 ditunda dengan persetujuan pemilik |
| Cache schema basi | Field baru tidak terdeteksi | TTL pendek default + invalidasi manual + metrik round-trip inspect (PLAN §14) |
| `(evl bad parameter …)` bentuk terbungkus | Salah lolos kalau pola #4 (evl) dicek dulu | Urutan matching #1→#3 sebelum #4 (grounded centrs) |
| Device v6 produksi aktif | Gangguan | Hanya read-only, frekuensi rendah, `-count=1` |

## 8. DoD dokumen ini (definisi selesai)

- [ ] Spesifikasi Fase 3 & Fase 4 lengkap (alur, tipe, struktur kode,
      dependensi) — grounded fakta lab v6/v7 + acuan centrs per baris.
- [ ] Test matrix M8–M21 terdefinisi lengkap (unit + integration, v6 & v7)
      dengan hasil yang sudah terverifikasi ditandai.
- [ ] Keputusan desain yang mengubah PLAN.md (classifier multi-pattern,
      degradasi v6, kategori+override, cache TTL) tercatat eksplisit untuk
      ditulis ke `docs/DECISIONS.md` saat implementasi (D-008..D-011).
- [ ] Tidak ada kredensial di repo; semua command lab read-only.
- [ ] Cross-ref konsisten: PLAN.md §7–8, RESEARCH.md §6–9,
      PLAN-FASE2-FASE3.md §3–5.
