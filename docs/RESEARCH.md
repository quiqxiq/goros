# RESEARCH — Fakta Lab & Pertanyaan Riset

> Log riset Fase 0 (PLAN.md §4). Setiap poin wajib punya jawaban **teruji di
> lab nyata**, bukan asumsi. Diperbarui setiap kali ada pengujian baru.
> Status label: ✅ terverifikasi · ⏳ perlu diuji · ⛔ terblokir.

## Inventaris lab

| Alias | IP | Port API | Versi | Status akses |
|---|---|---|---|---|
| MT-1 (v6) | `192.168.233.1` | 8728 (API), 8729 (API-TLS), 22 (SSH), 8291 (Winbox) | **RouterOS 6.49.11 (stable)** — RB750G, identitas `quixiq`, mipsbe, 32MB RAM | ✅ Reachable dari mesin dev (ping OK, semua port OPEN) |
| MT-2 (v7) | `192.168.230.3` | 8728 (API), 8729 (API-TLS), 22 (SSH), 8291 (Winbox) | **RouterOS 7.21.5 (long-term)** — hEX (RB750Gr3), identitas `mikrotik-sim`, mmips, 256MB RAM | ✅ Reachable dari mesin dev via `192.168.230.3` (alamat lama `.230.2` tidak reachable; pemilik memberi IP baru) |

**Catatan keamanan:** kredensial lab (password) tidak pernah ditulis ke repo.
Semua tooling lab membaca dari env var (`M_ADDR`/`M_USER`/`M_PASS` untuk probe,
`ROUTEROS_TEST_*` untuk test). Alamat IP lab hanya sebagai inventaris
(reachable-hanya dari LAN pribadi, RFC1918). Hanya command **read-only** yang dijalankan
terhadap device. **Pengecualian (2026-08-12, persetujuan pemilik):** DoD Fase 6
menulis `/ip/address/add` + `remove` auto-cleanup di MT-2 (v7) — lihat §12.

## Fakta terverifikasi — probe nyata 2026-08-12 (v6 6.49.11)

Dijalankan lewat `_lab/probe` (harness di repo, `_`-prefixed sehingga diabaikan
`go build ./...`), memakai library ini sendiri (legacy `Client` + adapter
`nativeapi`).

### 1. Stack native-api existing & adapter — BERPINDAH BENAR di device nyata ✅

| Probe | Hasil di v6 |
|---|---|
| `Client.Run` `/system/resource/print` | ✅ `version=6.49.11 (stable)` |
| `nativeapi.Adapter.Command` `/system/identity/print` | ✅ `!done` |
| `nativeapi.Adapter.List` `/system/identity/print` | ✅ 1 baris `!re` + `!done` |
| Trap mapping: command tidak dikenal → `roserr.CodeCommandFailed` + `*DeviceError` terjangkau | ✅ pesan trap: `no such command prefix` (via integration test M4) |

Implikasi: **DoD Fase 2 "native-api berjalan penuh lewat kontrak baru" tercapai
untuk jalur Command/List/error** terhadap perangkat sungguhan.

### 2. `/console/inspect` — fitur v7-only (data fallback Fase 5) ✅

- `Inspect(child, "system")` di **v6** → `!trap` `no such command`, dipetakan ke
  `roserr.CodeCommandFailed` (`[via=native-api]`). Dikonfirmasi dua kali:
  probe `_lab/probe` dan integration test `TestLabInspectProbe` (M5).
- Artinya: probe inspect di v6 **gagal bersih**, tidak menggantung, dan bisa
  dideteksi → flag sesi `SupportsInspect=false`.
- ✅ v7: **`SupportsInspect=true`** — `nodes=34` (lihat §6). Konfirmasi silang
  v6=false vs v7=true → kriteria DoD Fase 2 "≥2 versi (satu mendukung inspect,
  satu tidak)" **terpenuhi**.

### 2b. Matrix M1–M5 — hasil integration test real-device (2026-08-12) ✅

Dijalankan: `go test -tags integration -count=1 -v ./transport/nativeapi/`
terhadap `192.168.233.1:8728` (v6). Semua PASS:

| Test (skenario) | Hasil |
|---|---|
| `TestLabVersion` (M1) | `version=6.49.11 (stable)` |
| `TestLabAdapterCommand` (M2) | `!done` |
| `TestLabAdapterList` (M3) | `rows=1` + `!done` |
| `TestLabTrapMapping` (M4) | `CodeCommandFailed`, message `no such command prefix`, `*DeviceError` reachable |
| `TestLabInspectProbe` (M5) | unsupported: `no such command` → `SupportsInspect=false` (v6) |

### 3. `/execute` + `as-string` — TIDAK didukung v6 ❌ (perubahan desain!)

| Script | `as-string` | Hasil v6 |
|---|---|---|
| `:put "hello"` | tanpa | ✅ OK, `rows=0` |
| `:put "hello"` | dengan | ❌ `!trap`: **`unknown parameter`** |
| `:put [:parse "..."]` | tanpa | ✅ OK, `rows=0` |
| `/system/resource/print` (script polos) | tanpa | ✅ OK, `rows=0` |

Kesimpulan yang mengubah desain Gate 1:

1. **`as-string` adalah parameter v7** — di v6, mengirim `=as-string=` membuat
   `/execute` gagal `unknown parameter`.
2. **Di v6, `/execute` tidak mengembalikan output script** (selalu `rows=0`),
   bahkan untuk `print` polos. Output `:put`/`print` tidak sampai ke API.

Implikasi: jalur Gate 1 centrs (`/execute` + `as-string`, grounded di CHR
7.23.1/7.23.3) **hanya berlaku untuk v7**. Untuk v6 perlu mekanisme lain (lihat
§4.3 di `docs/PLAN-FASE2-FASE3.md`) atau degradasi eksplisit.

### 4. Karakteristik device v6

- CPU load ~91% saat di-probe (RB750G kecil, 32MB RAM) → **frekuensi command
  uji harus rendah**; jangan hammer device.
- Uptime 1w3d; write-sect aktif (perangkat produksi aktif?) → hati-hati,
  prioritas read-only.

## Fakta terverifikasi — probe nyata 2026-08-12 (v7 7.21.5)

Dijalankan lewat `_lab/probe` (versi diperkuat: cetak atribut `!done`
termasuk `ret`) + integration test terhadap `192.168.230.3:8728` (v7).

### 5. Identitas & versi MT-2 (v7) ✅

- **RouterOS 7.21.5 (long-term)**; board `hEX` (RB750Gr3); arch `mmips`;
  CPU `MIPS 1004Kc` 4-core @880MHz; 256MB RAM; identitas `mikrotik-sim`;
  `factory-software=6.46.3` (device pernah v6, di-upgrade ke v7); uptime 46m.
- Semua port terbuka (8728/8729/22/8291); `cpu-load=0` (idle — kontras dengan
  v6 yang 91%).

### 6. Matrix M1–M5 — hasil integration test real-device (v7 7.21.5) ✅

`go test -tags integration -count=1 -v ./transport/nativeapi/` vs
`192.168.230.3:8728`. **Semua PASS:**

| Test (skenario) | Hasil |
|---|---|
| `TestLabVersion` (M1) | `version=7.21.5 (long-term)` |
| `TestLabAdapterCommand` (M2) | `!done` |
| `TestLabAdapterList` (M3) | `rows=1` + `!done` |
| `TestLabTrapMapping` (M4) | `CodeCommandFailed`, message `no such command prefix`, `*DeviceError` reachable |
| `TestLabInspectProbe` (M5) | **SUPPORTED: `nodes=34`** → `SupportsInspect=true` |

### 7. `/execute` + `as-string` di v7 — DIDUKUNG ✅ (kebalikan v6)

| Script | `as-string` | Hasil v7 |
|---|---|---|
| `:put "hello"` | tanpa | ✅ OK, `ret="*45"` — **referensi internal, bukan string** |
| `:put "hello"` | `=as-string=` | ✅ OK, `ret="hello"` |
| `:put "hello"` | `=as-string=yes` | ✅ OK, `ret="hello"` |

Kesimpulan: di v7, `as-string` (nilai kosong **atau** `yes`) **wajib** untuk
mendapatkan output script sebagai string; tanpa itu `ret` berisi referensi
objek (`*45`). Jalur Gate 1 centrs (`/execute` + `as-string`) **valid untuk v7**.

### 8. `:parse` via `/execute as-string` di v7 (R2 & R3) ✅ — format pesan BERBEDA dari centrs

| Command yang di-`:parse` | `ret` (v7 7.21.5) | Klasifikasi Gate 1 |
|---|---|---|
| `/system/resource/print` (valid) | `(evl /system/resource/print)` | **lolos** |
| `/system/resource/print bogus=1` (atribut tak dikenal) | `expected end of command (line 1 column 24)` | syntax error + posisi (L1 C24) |
| `/nonsense/command` (command tak dikenal) | `syntax error (line 1 column 10)` | syntax error + posisi (L1 C10) |
| `/system/resource print` (path terpisah spasi) | `(evl /system/resource/print)` | **lolos** — `:parse` menerima & menormalkan |

Temuan yang mengubah desain classifier:

1. **Pola `bad parameter <name>` (centrs) TIDAK muncul di 7.21.5** — atribut
   tak dikenal menghasilkan `expected end of command (line 1 column N)`.
   Classifier wajib memetakan multi-pattern (per versi; v6.49 tidak punya jalur
   ini sama sekali).
2. `:parse` **tidak throw** di v7 (konfirmasi asumsi GH#230) — hasilnya string
   diagnostik di `ret`, persis yang diasumsikan R2.
3. Posisi `(line X column Y)` konsisten → bisa dipakai untuk lokasi error.
4. `/system/resource print` (spasi, tanpa slash kedua) di-parse dan
   dinormalisasi → **jangan** menganggap spasi sebagai syntax error saat
   preflight lokal.

### 9. Karakteristik device v7

- Idle (`cpu-load=0`), uptime 46m, `write-sect=91` — jauh lebih ringan dari v6.
- `factory-software=6.46.3` → perangkat di-upgrade v6→v7; verifikasi lintas
  v7 lain (mis. CHR) tetap berguna untuk format pesan.

### 10. R10 — `:parse` TIDAK bisa membedakan atribut tak dikenal vs syntax rusak di 7.21.5 ✅

| Command yang di-`:parse` | `ret` (7.21.5) | Klasifikasi |
|---|---|---|
| `/ip/address/print` (valid) | `(evl /ip/address/print)` | lolos |
| `/ip/address/print interface1=ether1` (**typo atribut nyata**) | `expected end of command (line 1 column 19)` | **identik** dengan syntax rusak |
| `/ip/address/print bogus=1` | `expected end of command (line 1 column 19)` | identik |
| `/nonsense/command` | `syntax error (line 1 column 10)` | syntax |
| `/ip/address/print ?` (rusak) | `expected end of command (line 1 column 19)` | identik |

**Kesimpulan desain:** di 7.21.5, Gate 1 (`:parse`) hanya bisa memberi sinyal
**coarse `validation/syntax`** untuk kasus atribut tak dikenal — tidak ada pola
pembeda `bad parameter <name>`. Identifikasi presisi (Missing/Available) adalah
tugas **Gate 2** (`/console/inspect`). Ini menetapkan fallback di
`docs/PLAN-FASE3-FASE4.md` §2.2/§2.5.

### 11. R9 — bentuk `/console/inspect` di 7.21.5: `child` ✅, `completion` ✅, trik `.proplist` ✅

**Bentuk node (`request=child`):**
- `type` ∈ `self` | `child`; `node-type` ∈ `dir` | `cmd` | `arg` — **predikat
  harus cek `node-type`** (bukan `type`): `IsArgumentNode` = `node-type==arg`,
  `IsCommandNode` = `node-type==cmd && name==…`.
- `child ip,address` → `self/address(dir)` + 12 `child/*(cmd)` (add, print, get,
  set, remove, …) → tabel punya `print` & `get`.
- `child ip,address,print` → `self/print(cmd)` + **16 arg** (append, brief,
  detail, from, interval, proplist, where, without-paging, …).
- `child tool,ping` → `self/ping(cmd)` + 14 arg (address, count, interface,
  size, ttl, src-address, vrf, …) → **action** (tanpa print/get).

**Bentuk node (`request=completion`):**
- `completion ip,address,print` → 1 node sampah (`completion=" "`, text=
  `whitespace`) → **tidak berguna**.
- **`completion ip,address,print,proplist` → 19 node = nama field print!**
  `address` (text "Local IP address"), `interface`, `comment`, `disabled`,
  `dynamic`, `network`, `netmask`, `broadcast`, `vrf`, `slave`, `invalid`,
  `actual-interface`, … **plus sampah struktural** yang wajib difilter:
  `[`, `(`, `$`, `"`, `*` (id prefix), `<value>` (literal), `about`.
  → konfirmasi **trik `.proplist`** centrs (`retrieve.ts`) berlaku di 7.21.5.
- `completion ip,address,print,value-name` → **0 node** (value-name khusus
  command `get`-style, bukan print) → pilih argumen per dukungan print/get.
- `child tool,ping,ping` → 0 node (path + verb ganda tidak ada).

**Kesimpulan desain:** `Discover` union `child`(args) + `completion`(proplist)
+ filter sampah struktural; kategori table (ada print/get) vs action (ada
command, tanpa print/get) vs unknown. Fixture unit memakai bentuk ini.

### 12. Fase 6 — orkestrasi & API publik (2026-08-12) ✅ (v7 7.21.5)

DoD Fase 6 (PLAN.md §10) diuji ke MT-2 (v7) lewat `go test -tags integration
./` (`TestLabRunStructured*`, `TestLabValidateDryRunDoesNotExecute`). Dengan
persetujuan pemilik, test terakhir **menulis** ke device + auto-cleanup.

| Skenario | Hasil v7 |
|---|---|
| `RunStructured` `/system/resource/print` (tabel valid) | ✅ `!done` |
| `RunStructured` `/ip/address/print bogus=1` (atribut salah nama) | ✅ Gate 2 tolak: `validation/unknown-attribute`, `missing=[bogus]`, `available` = daftar field nyata, `validationSource=inspect child+completion` |
| `RunStructured` script `/nonsense command` (syntax rusak) | ✅ Gate 1 tolak: `validation/syntax` (setelah R11, §13) |
| `Validate` (dry-run) `/ip/address/add` | ✅ lolos gate, **tidak membuat entry** (print `?comment=` → kosong) |
| `RunStructured` `/ip/address/add` (tulis) | ✅ entry dibuat & diverifikasi, lalu auto-cleanup `remove =.id=` → device bersih |

Implikasi: DoD Fase 6 (command tabel valid; atribut salah nama gagal Gate 2
dengan Missing/Available; syntax rusak gagal Gate 1; action via dry-run lolos
tapi tidak tereksekusi; command sama via run benar-benar tereksekusi)
**terpenuhi di device nyata v7**. Di v6 seluruh test lulus dengan degradasi
sesuai desain (gate skip senyap, eksekusi langsung).

### 13. R11 — `:parse` bentuk error terbungkus `(<% …` (command tak dikenal bentuk spasi) ✅

| Command yang di-`:parse` | `ret` (7.21.5) | Sebelum fix | Setelah fix |
|---|---|---|---|
| `/nonsense command` (path + verb dipisah spasi) | `(<% bad command name nonsense (line 1 column 2) nonsense;command)` | ⚠️ **lolos salah** (bentuk tak dikenal) | ✅ `validation/syntax` + posisi (L1 C2) |
| `/nonsense/command` (slash) | `syntax error (line 1 column 10)` | ✅ syntax + posisi | ✅ tetap |

- **Temuan desain:** `:parse` di 7.21.5 mengembalikan error **terbungkus**
  `(<% … %>)` ketika command yang gagal berada dalam konteks ekspresi script
  (di sini bentuk spasi membuat parser memperlakukannya sebagai ekspresi).
  Classifier Gate 1 diperluas: pola wrapped
  `\(<%[^()]*(?:syntax error|bad command name|expected[^()]*)` diperiksa
  **sebelum** fallthrough `(evl …)` valid. `(<%` adalah pembeda aman — hasil
  `:parse` valid selalu `(evl …)`. Urutan matching tetap wajib (D-008).
- Diperbarui: `gate/gate1.go` (`wrappedSyntaxRe`), fixture unit
  `orchestrate_test.go`.

### 14. R4 — spike Gate 1 di v6: TIDAK ada mekanisme viable untuk membaca output script ✅

Dilakukan atas persetujuan pemilik (D-015, 2026-08-12): eksperimen tulis kecil
(global var + script temp) di MT-1 (v6, produksi) dengan auto-cleanup.
Harness: `_lab/spike_v6_gate1{,,b,c}` + `_lab/spike_v6_cleanup`.

| Kandidat | Hasil v6 | Kesimpulan |
|---|---|---|
| `:global x "hello42"` via `/execute` → `environment print` | variabel **tidak muncul** (tetap `rows=0`), `/execute` sukses (`ret=*A040` referensi internal) | ❌ var dari `/execute` tidak persist/tampil |
| `:global x [:parse "…"]` → env print | tidak muncul | ❌ sama |
| `:global x [:tostr [:parse "…"]]` → env print | tidak muncul | ❌ sama |
| `/system/script/add` + `/system/script/run` → env print | `run` gagal `syntax error`; env tetap 0 | ❌ tidak viable |
| `ret` dari `/execute` (tanpa `as-string`) | `*A040`/`*A043`/… (referensi objek, bukan string) | konsisten §3 |

**Kesimpulan desain:** v6 **tidak punya mekanisme** untuk membaca output
script lewat native API → Gate 1 di v6 tetap **degradasi skip** (D-009),
sekarang berbasis fakta (bukan asumsi). Jalur validasi v6 yang tersedia:
Gate 1 skip + Gate 2 skip (`SupportsInspect=false`) + eksekusi langsung.
Fallback discovery v6 (union field via `print` sungguhan, PLAN.md §9) tetap
opsional dan **memerlukan persetujuan** karena mengeksekusi `print` tanpa
filter di device produksi.

**Cleanup terverifikasi:** script temp `__goros_spike_test` dihapus
(`remove .id=*2`), `environment print` kosong, script lain milik device
(`cache-update-trigger`) tidak disentuh.

### 15. R6/R7/R6b/R6c — probe SSH (Fase 7) via `_lab/probe_ssh` ✅

Harness: `_lab/probe_ssh` (x/crypto/ssh, tanpa PTY, read-only).

#### v7 7.21.5 — output bersih, `:parse` identik native-api, tanpa paging ✅

| Probe | Hasil v7 |
|---|---|
| `/system/identity/print` (slash) | ✅ `  name: mikrotik-sim\r\n\r\n` — bersih, indent 2 spasi + blank line tepi |
| `/system/resource/print` | ✅ 818 B, kolom dengan trailing padding (trim per baris diperlukan) |
| `/ip/address/print count-only` | ✅ `4\r\n` |
| `:put [:parse "/ip/address/print"]` | ✅ `(evl /ip/address/print)` — **identik native-api** |
| `:put [:parse "/ip/address/print bogus=1"]` | ✅ `expected end of command (line 1 column 19)` — identik (R10) |
| `:put [:parse "/nonsense/command"]` | ✅ `syntax error (line 1 column 10)` — identik |
| `:put [:parse "/nonsense command"]` | ✅ `(<% bad command name nonsense (line 1 column 2) …)` — identik (R11) |
| `/ip/route/print detail` (panjang) | ✅ 1983 B **lengkap** — tidak terpotong, tanpa paging |
| `/ip/address/print` | ✅ 383 B lengkap |

**Kesimpulan:** classifier Gate 1 (D-008) yang sudah ada **berlaku apa adanya**
untuk SSH v7 — tidak ada pola baru. Output perlu `CleanConsoleOutput`
(CRLF→LF, trim trailing per baris, buang baris kosong tepi — port centrs
`cleanConsoleOutput`, ssh.ts L182).

#### v6 6.49.11 — SSH exec hanya menerima bentuk SPASI (R12, mengubah desain!) ✅

| Command (bentuk) | Hasil v6 |
|---|---|
| `/system/identity/print` (slash-joined) | ❌ ERROR exit 1: `expected command name (line 1 column 8)` |
| `/system identity print` (spasi + slash) | ✅ `  name: quixiq\r\n\r\n` |
| `identity print` (tanpa slash) | ❌ `bad command name identity (line 1 column 1)` |
| `/ip address print` (spasi + slash) | ✅ 536 B (5 alamat) |
| `/ip address print count-only` (spasi + atribut) | ✅ `5\r\n` |
| `:put [:parse "/ip address print"]` (spasi) | ✅ **`(eval /ip address print)`** — VALID (classifier fallthrough lolos) |
| `:put [:parse "/ip/address/print"]` (slash) | ❌ `expected command name (line 1 column 4)` |

**Kesimpulan desain (mengubah matriks Fase 7):**
1. **SSH exec v6 membutuhkan bentuk console spasi** (`/ip address print`),
   bukan slash-joined (`/ip/address/print`). Bentuk spasi + slash juga
   diterima v7 (RESEARCH §8 poin 4: `/system/resource print` dinormalisasi
   jadi `(evl /system/resource/print)`) → **transport/ssh merender CLI bentuk
   spasi secara universal** (`/path verb key=value` dari `PathTokens()`),
   kompatibel kedua versi.
2. **Gate 1 console di v6 VIABLE** (output langsung dari session SSH, tanpa
   `/execute`/`as-string` yang jadi blocker di native-api R4). Hasil `:parse`
   valid di v6 = `(eval …)` — classifier fallthrough (tidak cocok pola error)
   → lolos. Ini **membalik kesimpulan R4 untuk jalur SSH**: degradasi Gate 1
   v6 hanya berlaku native-api, bukan console.
3. Error `bad command name` / `expected command name` (v6) → syntaxRe
   classifier cocok (`^bad command name` / `^expected …` + posisi) →
   `validation/syntax` ✅ — perlu fixture v6 di unit test classifier.
4. v7 menerima kedua bentuk → tidak ada regresi untuk v7.

#### R6c — failure modes (kedua versi) ✅

| Kasus | Error yang teramati | Pemetaan roserr |
|---|---|---|
| Password salah | `ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain` | `auth/failed` |
| Port tertutup | `dial tcp <host>:6553: connect: connection refused` | `transport/connection-refused` |
| Host-key | v6: `ssh-rsa` sha256=`crG5+Z7KyJ0Bcz83arjozrYulyQngONo7kZKAMMyIY=`; v7: `ssh-rsa` sha256=`Xfu3rLDoxAV37FFSd1B7zEU5QPP7yduet/aGbJdDHs4=` — TOFU policy diimplementasikan di transport (D-016) | `transport/host-key-mismatch` (via callback) |

### 16. M22–M29 — integration test SSH (Fase 7) di device nyata ✅

Dijalankan: `go test -tags integration -count=1 -v ./transport/ssh/` dengan
`ROUTEROS_TEST_SSH_*` (SSH) + `ROUTEROS_TEST_*` (native-api, untuk M29) ke
MT-1 (v6 6.49.11) & MT-2 (v7 7.21.5). **Semua test PASS** di kedua device;
M29 di v6 SKIP sesuai desain (native-api Gate 1 tidak didukung v6 — D-009).

| # | Skenario (via SSH) | v6 6.49.11 | v7 7.21.5 |
|---|---|---|---|
| M22 | Dial + `/system identity print` (bentuk spasi, R12) → output bersih | ✅ `  name: quixiq` (14 B) | ✅ `  name: mikrotik-sim` (20 B) |
| M23 | `/nonsense command` → Gate 1 tolak | ✅ `validation/syntax` (`(<% bad command name nonsense (line 1 column 2) …)` — pola wrapped R11 berlaku di v6 juga) | ✅ `validation/syntax` (identik) |
| M24 | `print bogus=1` → Gate 1 tolak | ✅ `validation/syntax` (`expected end of command (line 1 column 19)` — coarse R10) | ✅ `validation/syntax` (identik) |
| M25 | `/ip route print detail` output panjang tidak terpotong (R7) | ✅ 1371 B lengkap | ✅ 1913 B lengkap |
| M26 | Auth gagal (password salah) → `auth/failed` | ✅ | ✅ |
| M27 | Host-key TOFU accept-new + `Insecure` opt-out | ✅ keduanya dial sukses | ✅ keduanya dial sukses |
| M28 | ctx timeout → `transport/timeout`, koneksi tetap usable | ✅ | ✅ |
| M29 | ValidationError SSH **setara native-api** (DoD §11) | ⛔ SKIP (native-api Gate 1 v6 skip, D-009) | ✅ kode sama (`validation/syntax`) |

**Implikasi:** DoD Fase 7 (kriteria §11 PLAN.md + matriks §15) **terpenuhi**:
- Gate 1 console di **v6 VIABLE** (bentuk spasi, R12) — `:parse` v6 mengeluarkan
  pola error yang sama persis dengan v7 (`(<% bad command name …)`, `expected
  end of command …`) → classifier D-008 berlaku apa adanya, tanpa fixture baru.
- **M29** membuktikan kode & makna ValidationError konsisten lintas transport
  (SSH == native-api) untuk kesalahan yang sama — prinsip "classifier tunggal"
  (D-008/D-016) terkonfirmasi di device nyata.

## Pertanyaan riset terbuka

| # | Pertanyaan | Status | Target |
|---|---|---|---|
| R1 | Versi persis & karakteristik MT-2 (v7)? `/console/inspect` tersedia? | ✅ **7.21.5 (long-term)**, hEX; inspect tersedia (`nodes=34`) | MT-2 |
| R2 | Di v7, `:parse` benar-benar tidak throw (GH#230) dan hasil diagnostik muncul sebagai string? | ✅ tidak throw; string di `ret` | MT-2 |
| R3 | Di v7, hasil `:parse` via `/execute as-string`? | ✅ valid=`(evl …)`; atribut tak dikenal=`expected end of command (line X column Y)`; command tak dikenal=`syntax error (line X column Y)`. **Pola `bad parameter <name>` tidak muncul di 7.21.5** — classifier multi-pattern | MT-2 |
| R4 | Mekanisme baca output script di v6 selain `/execute` (mis. `/system/script/run` + baca output, atau variabel global + command kedua) — mana yang read-only & berfungsi? | ✅ **tidak ada yang viable** — variabel global (string/`:parse`/`:tostr`) tidak tampil di `environment print`; `/system/script/run` gagal + tetap `rows=0` (§14) | MT-1 |
| R5 | Bentuk path inspect: koma (`ip,address`) cocok, slash tidak | ✅ **koma cocok** (`child ip,address` = 13 node, 7.21.5); bentuk field `completion` juga koma | MT-2 |
| R9 | Bentuk `request=completion` + trik `.proplist`/`value-name` di 7.21.5? | ✅ `proplist` → 19 nama field (sampah struktural harus difilter); `value-name` → 0 node (khusus get); `completion` polos → sampah | MT-2 |
| R10 | Bisa bedakan typo atribut nyata dari syntax rusak via `:parse` di 7.21.5? | ✅ **tidak bisa** — keduanya `expected end of command (line X column Y)` → Gate 1 coarse `validation/syntax`, presisi via Gate 2 | MT-2 |
| R11 | Bentuk pesan `:parse` untuk command tak dikenal **bentuk spasi** (`/nonsense command`)? | ✅ error terbungkus `(<% bad command name … (line 1 column 2) …)` — classifier diperluas (wrapped syntax), lihat §13 | MT-2 |
| R4 | (ulang) Spike Gate 1 v6: mekanisme alternatif untuk membaca output script? | ✅ **tidak viable** — lihat §14 | MT-1 |
| R6 | SSH di v6/v7: `ssh user@host "cmd"` tanpa PTY mengembalikan output bersih? (untuk Fase 7) | ✅ v7 bersih (tanpa prompt/ANSI, hanya indent + blank line tepi → `CleanConsoleOutput`); v6 **hanya menerima bentuk spasi** (`/ip address print`), bentuk slash-joined gagal `expected command name` (§15, R12) | MT-1, MT-2 |
| R7 | Perilaku paging console untuk output panjang via SSH | ✅ v7: output panjang (1983 B routes detail) **lengkap, tidak terpotong, tanpa paging** (§15) | MT-2 |
| R6b | `:put [:parse "<cmd>"]` via SSH: di v7 sama dengan native-api (`(evl …)` / error)? Di v6 bentuk apa (klasifikabel)? | ✅ v7 **identik native-api**: `(evl …)`, `expected end of command`, `syntax error`, `(<% …` (R11) — classifier sama berlaku apa adanya. v6: bentuk spasi → `(eval …)` valid (classifier fallthrough lolos); bentuk slash → `expected command name` (§15) | MT-1, MT-2 |
| R6c | Bentuk error connect/auth SSH yang bisa dipetakan (host-key mismatch, user salah, port salah) | ✅ `unable to authenticate` → `auth/failed`; `connection refused` → `transport/connection-refused`; host-key sha256 tercapture (§15) | MT-1, MT-2 |
