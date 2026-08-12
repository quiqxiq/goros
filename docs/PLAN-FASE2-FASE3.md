# PLAN EKSEKUSI — Sisa Fase 2 (validasi lab) + Fase 3 (Gate 1) — Lab Mikrotik v6/v7

> Dokumen eksekusi yang **grounded di fakta lab nyata** (probe 2026-08-12
> terhadap v6) — bukan rencana dari asumsi. Fakta lengkap: `docs/RESEARCH.md`.
> Induk rencana fase: `PLAN.md`. Log keputusan: `docs/DECISIONS.md`.
>
> **Keputusan pemilik project (2026-08-12):**
> 1. Satu dokumen gabungan: validasi real-device sisa Fase 2 + rencana Gate 1 (Fase 3).
> 2. Device v7 **`192.168.230.3`** (admin, kredensial via env var) ✅
>    **reachable** (2026-08-12) —
>    matriks v7 di bawah sudah terisi hasil nyata M1–M7 + probe `:parse`
>    (§8 RESEARCH.md); sisa v7 tinggal implementasi Gate 1 (M8–M10) dan M11–M13.
> 3. Skenario read-only terhadap v6 **dieksekusi sekarang**.

---

## 0. Ringkasan status

| Fase | Status implementasi | Status validasi lab |
|---|---|---|
| Fase 1 — fondasi (kontrak, roserr, mock) | ✅ selesai | n/a (unit test) |
| Fase 2 — adaptasi native-api (`transport/nativeapi`) | ✅ selesai (D-007) | ✅ **M1–M5 PASS di v6 & v7** (termasuk inspect v6=false, v7=true). **Probe inspect + cache sesi belum diimplementasi** (fase berikutnya) |
| Fase 3 — Gate 1 (`:parse`) | ⬜ belum dimulai | ✅ riset selesai: v7 jalur `as-string` terverifikasi + format pesan `:parse` 7.21.5 sudah direkam; v6 → degradasi eksplisit (temuan `as-string`) |

Yang tersisa dari DoD Fase 2 (`PLAN.md` §6):
- **Probe `/console/inspect` sekali di awal sesi + cache flag per sesi** — belum
  diimplementasi (dulu dianggap Fase 5, tapi PLAN Fase 2 eksplisit memintanya).
- **"Diuji ke ≥2 versi RouterOS (satu mendukung inspect, satu tidak)"** — v6
  (tidak mendukung) ✅ terverifikasi; v7 (mendukung) ⛔ menunggu akses.

---

## 1. Inventaris lab & aturan main

Lihat `docs/RESEARCH.md` §Inventaris. Ringkas:

| Alias | IP | Versi | Akses | Peran dalam test |
|---|---|---|---|---|
| MT-1 | `192.168.233.1` | 6.49.11 | ✅ | v6: **tanpa** inspect, `/execute as-string` gagal, `/execute` tanpa output |
| MT-2 | `192.168.230.3` | 7.21.5 (long-term) | ✅ | v7: **dengan** inspect (`nodes=34`), `/execute as-string` OK (`ret` string), `:parse` tidak throw |

**Aturan main (wajib):**
1. **Hanya command read-only** terhadap device (print/identity/resource/parse).
   Tidak ada `add/set/remove` dalam testing fase ini.
2. Kredensial hanya lewat env var; jangan commit.
3. Frekuensi rendah (v6 CPU ~91%) — pakai `-count=1`, tanpa loop berlebihan.
4. Setiap fakta baru → catat di `docs/RESEARCH.md`.

---

## 2. Sisa Fase 2 — probe inspect + cache sesi (implementasi)

**Tujuan:** sesi tahu sejak awal apakah `/console/inspect` tersedia, supaya
Gate 2 (Fase 4) bisa berdegradasi rapi; cache per sesi (bukan per command).

**Desain yang diusulkan (di `transport/nativeapi`):**
- Tambah state sesi di `Adapter`: `inspectProbed atomic.Bool` +
  `inspectSupported atomic.Bool`.
- `ProbeInspect(ctx) error` — panggil `Inspect(ctx, child, "system")` sekali;
  sukses → `inspectSupported=true`; error ber-kode `CodeCommandFailed` (trap
  `no such command`) → `inspectSupported=false` (degradasi senyap, tanpa error
  ke pemanggil). Error selain trap (mis. timeout/io) → dikembalikan (masalah
  nyata, bukan degradasi).
- `SupportsInspect() bool` — baca flag; tanpa probe → false.
- Dipanggil otomatis oleh facade/orkestrator setelah login (Fase 6); di Fase 2
  cukup tersedia sebagai method + diuji.
- Nama path probe: `system` (pasti ada di semua versi; verifikasi saat v7
  reachable — R5).

**Kriteria selesai (unit + lab):**
- Unit (mock `transport/mock`): probe sukses → flag true; probe trap → flag
  false; probe io-error → error dikembalikan; `SupportsInspect` sebelum probe →
  false. (Ini menutup skenario Fase 5 juga.)
- Lab v6: `SupportsInspect()==false` (verified: `no such command`).
- Lab v7 ✅: `SupportsInspect()==true` (`nodes=34`, 7.21.5) — terverifikasi.

---

## 3. Fase 3 — Gate 1: validasi syntax via `:parse`

### 3.1 Spesifikasi (dari `PLAN.md` §7 + acuan centrs)

Acuan: `.refrences/centrs/src/execute.ts` — `runSyntaxGate` (L803),
`classifyParseResult` (L30/L792/L831), `routerOsStringLiteral` (L1431),
`parseScriptFor` (L31/L789), `validateConsoleParseCommand` (L767).

1. Script: `:put [:parse "<command asli>"]`, command di-escape sebagai string
   literal RouterOS (`routerOsStringLiteral`: escape `\` dan `"`).
2. **Preflight kutip lokal** sebelum mengirim (centrs `validateConsoleParseCommand`):
   kalau kutip tidak seimbang → gagalkan lokal tanpa round-trip.
3. Kirim via `/execute` + `=script=` + `=as-string=` — **jalur v7** (lihat 3.3
   untuk v6).
4. Dua jalur hasil:
   - `/execute` sendiri gagal (`!trap`) → kegagalan level script/transport,
     bukan hasil parse command yang divalidasi (bedakan!).
   - `/execute` sukses → isi `ret` = hasil `:parse`; cocokkan pola:
     - `bad parameter <name>` → ekstrak nama → `validation/unknown-attribute`.
     - variasi syntax rusak (command salah / command tidak dikenal / bagian
       wajib kosong) → `validation/syntax`.
     - Tidak cocok → lolos.
5. Ekstrak posisi baris/kolom bila ada di pesan.
6. **Classifier = fungsi murni** dipakai native-api DAN console (Fase 7) —
   jangan disalin.

### 3.2 Struktur kode yang diusulkan

```
gate/
  gate1.go        — Gate1{transport transport.StructuredTransport}; Run(ctx, cmd) error
                     + PureSyntaxClassifier (fungsi murni, tanpa I/O)
                     + routerOsStringLiteral + preflight kutip lokal
  gate1_test.go   — unit, fixture teks ret (no network)
  gate1_lab_test.go — integration (build tag), memakai Adapter v6/v7
```

- `PureSyntaxClassifier.Classify(ret string) *ClassifyResult` —
  `{Valid bool, Code roserr.Code, Attribute string, Line, Col int, Message string}`.
- Fixture unit (dari centrs + R2/R3): command valid, `bad parameter address`,
  command tidak dikenal, syntax rusak dengan/tanpa posisi.
- Transport interface dipakai: `transport.StructuredTransport` (Command dengan
  `Script` → sudah didukung adapter Fase 2: `/execute =as-string=`).

### 3.3 Jalur v7 vs v6 — temuan lab yang mengubah desain

| | v7 (MT-2, terverifikasi) | v6 (MT-1, terverifikasi) |
|---|---|---|
| `/execute` + `=as-string=` | ✅ `ret="hello"` (as-string= kosong & `=yes` sama-sama OK) | ❌ `unknown parameter` |
| `/execute` tanpa `as-string` | ✅ OK tapi `ret="*45"` (ref objek, bukan string) | ✅ OK tapi `rows=0` (output tidak kembali) |
| `:parse` | ✅ tidak throw; valid=`(evl …)`, error=`syntax error (line X column Y)` / `expected end of command` | n/a (output tak sampai) |
| Kesimpulan | jalur Gate 1 centrs berlaku — **dengan catatan format pesan classifier 7.21.5 ≠ fixture centrs** | **jalur Gate 1 berbeda/tidak tersedia apa adanya** → degradasi eksplisit |

**Keputusan desain yang diusulkan (perlu dicatat di DECISIONS.md saat
diimplementasi):** Gate 1 punya dua backend:
- **v7/native-api-v7**: `/execute` + `as-string` (jalur centrs).
- **v6**: sebelum Fase 8/9 spike, tetapkan **degradasi eksplisit** —
  `SupportsParse` per sesi, mirip pola `SupportsInspect`; jika false, Gate 1
  di-skip dengan catatan kapabilitas (pemanggil sadar), bukan gagal per command.
  (Konsisten dengan prinsip "escape hatch eksplisit", DESIGN.md §2.1.)

**Spike mekanisme v6 (read-only, sebelum memutuskan):**
1. `/execute` + variabel global di script, baca lewat command kedua — uji.
2. `/system/script/run` pada script bernama — butuh membuat script = **tulis**,
   TIDAK boleh di lab ini; hanya layak jika pemilik setuju.
3. Jalur lain yang ditemukan saat spike — dokumentasikan.
Kriteria spike: bisa membaca hasil `:parse` di v6 secara read-only. Kalau tidak
ada → degradasi eksplisit (di atas).

### 3.4 Kriteria selesai Fase 3

- Unit test classifier tanpa jaringan (fixture `ret`) → benar untuk tiap fixture.
- Preflight kutip lokal teruji (kutip tidak seimbang gagal lokal).
- Integration v7 ✅ (probe selesai; implementasi ⏳): command valid lolos
  (`(evl …)`); atribut salah nama → `expected end of command (line X column Y)`
  — **bukan** `bad parameter <name>` (update classifier multi-pattern); command
  tak dikenal → `syntax error (line X column Y)`. Classifier dipetakan ke
  `validation/unknown-attribute` / `validation/syntax` saat Gate 1 diimplementasi.
- Integration v6: sesuai hasil spike (degradasi atau jalur v6).

---

## 4. Infrastruktur test integration

Per `PLAN.md` §15: integration test dipisah lewat **build tag `integration`**,
dijalankan sebagai job CI terpisah, tidak memblokir pipeline utama.

- File lab: `*_lab_test.go` dengan header `//go:build integration` (pola
  `package nativeapi_test`; yang sudah dibuat: `transport/nativeapi/nativeapi_lab_test.go`).
- Env vars (pola existing `client_test.go`): `ROUTEROS_TEST_ADDRESS` /
  `ROUTEROS_TEST_USERNAME` / `ROUTEROS_TEST_PASSWORD`. Satu suite dijalankan
  dua kali (v6 dulu, v7 saat reachable) — hasil dibandingkan lintas versi.
- `Makefile` (baru): `make test`, `make test-integration`, `make lab-probe`.
- CI: job `integration` terpisah (workflow_dispatch/schedule), env dari
  GitHub Secrets — bukan nilai hardcode.

## 5. Test matrix real-device (sumber: PLAN §15 + DoD Fase 2/3)

| # | Skenario (semua read-only) | v6 (MT-1) | v7 (MT-2) |
|---|---|---|---|
| M1 | Dial + login (legacy & adapter) | ✅ verified | ⛔ |
| M2 | Adapter `Command` print → `!done` | ✅ verified | ⛔ |
| M3 | Adapter `List` → baris + `!done` | ✅ verified | ⛔ |
| M4 | Command tidak dikenal → `CodeCommandFailed` + `*DeviceError` reachable | ✅ verified (`no such command`) | ⛔ |
| M5 | Inspect probe → flag sesi | ✅ `false` (`no such command`) | ✅ **`true` (`nodes=34`)** |
| M6 | `/execute` + `as-string` | ❌ `unknown parameter` | ✅ `ret="hello"` |
| M7 | `/execute` tanpa `as-string` → output? | ❌ `rows=0` | ✅ `ret="*45"` (ref objek) |
| M8 | Gate 1 valid command → lolos | ⏳ spike | ✅ probe: `(evl …)`; impl ⏳ |
| M9 | Gate 1 atribut tak dikenal → `validation/unknown-attribute` | ⏳ spike | ✅ probe: `expected end of command (L1 C24)`; impl ⏳ |
| M10 | Gate 1 command tak dikenal / syntax rusak → `validation/syntax` | ⏳ spike | ✅ probe: `syntax error (L1 C10)`; impl ⏳ |
| M11 | Autentikasi gagal → `auth/failed` | ⏳ | ⏳ |
| M12 | Koneksi putus tengah jalan → error jelas | ⏳ | ⏳ |
| M13 | TLS (8729) dial | ⏳ | ⏳ |

Legend: ✅ terverifikasi · ⏳ belum · ⛔ blocked.

## 6. Eksekusi lab (read-only, v6 & v7) — langkah & hasil

1. Probe identitas & versi (`_lab/probe`) — ✅ selesai (6.49.11, lihat RESEARCH.md).
2. Integration test adapter vs v6: `go test -tags integration` → M1–M5 ✅
   (dicatat di RESEARCH.md).
3. Integration test adapter vs v7 (`192.168.230.3:8728`) → M1–M5 ✅
   (inspect `nodes=34`; dicatat di RESEARCH.md §6).
4. Probe v7 `/execute` + `:parse` (M6–M10) → semua data terkumpul
   (RESEARCH.md §7–8) — **format pesan classifier 7.21.5 ≠ centrs**.
5. Implementasi sisa Fase 2 (probe+cache) → unit test → jalankan M5 lagi
   (v6 & v7).
6. Implementasi Gate 1 (classifier multi-pattern) → unit fixture →
   integration M8–M10 di v7 + degradasi v6.
7. Perbarui DECISIONS.md untuk setiap keputusan desain baru (jalur v6 Gate 1,
   classifier multi-pattern, nama path probe, dll).

## 7. Risiko & mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| v7 semula unreachable | M5–M10 v7 terblokir | ✅ Teratasi: reachable via `192.168.230.3`; M1–M7 + probe :parse selesai. Risiko tersisa: format pesan `:parse` berbeda per versi (6.49 tidak ada jalur, 7.21.5 ≠ centrs 7.23.x) → classifier multi-pattern + verifikasi lintas v7 |
| `as-string` tidak didukung v6 (verified) | Gate 1 v6 gagal | Spike mekanisme v6 + degradasi eksplisit (bukan error per command) |
| Device v6 produksi aktif (write-sect tinggi, CPU 91%) | Gangguan / rate-limit | Hanya read-only, frekuensi rendah, `-count=1` |
| Perubahan wire tidak sengaja | Regresi pengguna | Seluruh test existing wajib hijau tiap langkah (`go test ./...`) |
| Path probe inspect salah nama | Flag sesi salah di v7 | Verifikasi di v7 (R5); fallback `system` → `ip` jika perlu |

## 8. DoD dokumen ini (definisi selesai)

- [x] RESEARCH.md memuat semua fakta v6 **dan v7** yang bisa diverifikasi +
      pertanyaan terbuka bertanda jelas (✅/⏳/⛔).
- [ ] Sisa Fase 2 (probe + cache sesi) terimplementasi & lulus unit + lab
      v6 **dan v7**.
- [ ] Gate 1 (classifier multi-pattern + fixture + preflight) terimplementasi
      & lulus unit.
- [x] Integration test build-tag `integration` jalan terhadap **v6 (M1–M5)
      dan v7 (M1–M5)**; probe v7 M6–M10 selesai, implementasi menyusul.
- [x] Tidak ada kredensial di repo; semua command lab read-only.
- [ ] Setiap keputusan desain baru tercatat di DECISIONS.md (termasuk
      classifier multi-pattern 7.21.5).
