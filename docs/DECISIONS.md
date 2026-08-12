# DECISIONS — Log Keputusan Desain `goros`

> **Format tiap entri:** tanggal · keputusan · alasan · alternatif yang ditolak.
>
> Dokumen ini adalah sumber kebenaran untuk keputusan non-trivial. Diperbarui
> setiap kali ada keputusan baru atau keputusan yang menyimpang dari `PLAN.md`.
> Rencana awal bukan dokumen yang membatu — tapi revisinya harus tercatat di sini.

---

## D-001 — Module path: `github.com/quiqxiq/goros/v3`

- **Tanggal:** 2026-08-12
- **Keputusan:** Module path pindah dari `github.com/go-routeros/routeros/v3` ke
  **`github.com/quiqxiq/goros/v3`**.
  Catatan teknis: karena kebijakan versi melanjutkan garis v3 (D-003), Go
  mewajibkan suffix `/vN` untuk major ≥ 2 — kombinasi "module baru + lanjut v3"
  menghasilkan path `github.com/quiqxiq/goros/v3`. Implikasi rilis: tag versi
  harus `v3.x.y` (bukan `v0.x`/`v1.x`) agar `go get` bisa resolve module ini.
  **Penerapan** (ubah go.mod + seluruh import + contoh + README) dilakukan di
  **Fase 1**, bukan sekarang, supaya repo tetap buildable selama fondasi
  disiapkan. Nama package root tetap `routeros` (kompatibilitas), lihat PLAN.md §2.
- **Alasan:** Repo ini adalah fork yang di-publish di `github.com/quiqxiq/goros`
  dengan API yang didesain ulang signifikan (multi-transport, dua gate, return
  terstruktur). Identitas module sendiri lebih jujur dan bebas konflik ekspektasi
  API dengan upstream.
- **Alternatif yang ditolak:**
  - (a) Pertahankan `github.com/go-routeros/routeros/v3` — cocok jika ingin
    drop-in untuk pengguna go-routeros atau kontribusi balik ke upstream; ditolak
    karena identitas fork sendiri.
  - (b) `github.com/quiqxiq/goros` tanpa suffix — bertentangan dengan kebijakan
    lanjut v3 (D-003); Go tidak mengizinkan major ≥ 2 tanpa suffix.

---

## D-002 — Versi Go minimum: 1.22

- **Tanggal:** 2026-08-12
- **Keputusan:** Naikkan `go 1.21` → **`go 1.22`** di `go.mod` (diterapkan di Fase 1).
- **Alasan:** Membuka fitur loop-var semantics (perbaikan bug umum) tanpa
  menuntut toolchain yang terlalu baru; tidak membatasi pengguna secara berarti.
- **Alternatif yang ditolak:**
  - Tetap `go 1.21` — cukup untuk kebutuhan saat ini, tapi menutup loop-var
    semantics untuk kode baru.
  - `go 1.23+` — belum ada kebutuhan yang membenarkan pembatasan toolchain lebih
    ketat.

---

## D-003 — Kebijakan versi: lanjut garis v3, backward-compat

- **Tanggal:** 2026-08-12
- **Keputusan:** API publik existing (`Client`, `Dial*`, `Run`, `RunContext`,
  `Listen`, `RunAsync`, `Close`) **tetap tersedia**. Penambahan API baru masuk
  rilis `v3.x`. Bump `v4` hanya jika perubahan API publik tidak bisa dihindari —
  keputusan final ditunda ke **Fase 6**, saat API baru (Validate / Inspect /
  run terstruktur) sudah final.
- **Alasan:** Strategi adaptasi berarti kode dan API lama tetap bernilai;
  menghindari migrasi paksa yang belum tentu perlu.
- **Alternatif yang ditolak:** Segera bump `v4`/`v0.x` — lebih bersih untuk
  desain ulang total, tapi memaksa migrasi pengguna tanpa kebutuhan yang jelas.

---

## D-004 — Lisensi: MIT (copyright André Luiz dos Santos + catatan fork)

- **Tanggal:** 2026-08-12
- **Keputusan:** Pertahankan `LICENSE` MIT dengan copyright asli
  "Copyright (c) 2016 André Luiz dos Santos". Tambahkan catatan bahwa repo ini
  adalah **fork** dari `github.com/go-routeros/routeros` (baris catatan di
  `LICENSE` dan/atau `README.md`).
- **Alasan:** Kode dasar berasal dari upstream berlisensi MIT; mempertahankan
  copyright asli adalah hal yang benar dan aman secara hukum untuk fork.
- **Alternatif yang ditolak:** Mengganti copyright atas nama sendiri — berisiko
  secara hukum tanpa penulisan ulang penuh atas kode upstream.

---

## D-005 — Strategi: adaptasi, bukan rewrite dari nol

- **Tanggal:** 2026-08-12
- **Keputusan:** Kembangkan repo ini **in-place** dengan mempertahankan kode
  native-api yang matang (`client.go`, `proto/`, `async.go`, `listen.go`).
  Perkenalkan interface baru, migrasikan implementasi lama ke interface tersebut,
  pertahankan API publik lama sebagai wrapper.
- **Alasan:** Kode existing sudah matang dan teruji; menulis ulang = buang waktu,
  risiko regresi, dan kehilangan kompatibilitas.
- **Alternatif yang ditolak:** Rewrite total dari nol (risiko tinggi, nilai
  rendah); modul terpisah yang menyalin ulang logika (duplikasi tanpa manfaat).

---

## D-006 — Scope transport: tanpa RoMON & Winbox; MAC-Telnet opsional

- **Tanggal:** 2026-08-12
- **Keputusan:** Transport yang direncanakan:
  - **native-api** (adaptasi existing) — wajib, fondasi.
  - **SSH** (console, gate `:parse`) — wajib, transport kedua.
  - **MAC-Telnet** — opsional, hanya dikerjakan jika spike kelayakan (Fase 8)
    lolos; boleh ditunda ke backlog.
  - **REST** — opsional, hanya jika ada kebutuhan konkret.
  - **RoMON dan Winbox: DIKELUARKAN dari scope** — tidak direncanakan sama sekali.
- **Alasan:** Fokus pada transport yang memberi nilai dengan risiko terkendali.
  RoMON/Winbox memiliki spesifikasi paling sedikit terdokumentasi resmi dan tidak
  dibutuhkan saat ini.
- **Alternatif yang ditolak:**
  - Memasukkan RoMON/Winbox (ditolak langsung oleh pemilik project).
  - Mengeluarkan MAC-Telnet juga — tetap dipertahankan sebagai opsional karena
    ada acuan implementasi lengkap di `.refrences/centrs/src/protocols/mac-telnet.ts`.

---

## D-007 — Fase 2: adapter `transport/nativeapi` membungkus `Client` (+ `Command.Words()`)

- **Tanggal:** 2026-08-12
- **Keputusan:** Adaptasi native-api ke kontrak transport dilakukan lewat
  package baru **`transport/nativeapi`** yang membungkus `*routeros.Client` dan
  mengimplementasikan `transport.StructuredTransport` (Command, List, Inspect,
  Capabilities, Close). `client.go` dan `proto/` **tidak diubah** — perilaku
  wire-level dipertahankan; perubahan hanya di titik masuk/keluar (per PLAN.md
  Fase 2). Translasi dua arah:
  - `transport.Command` → sentence terstruktur via helper **`Command.Words()`**
    (tambahan aditif di `transport/contract.go`, bentuk structured setara `CLI()`);
    command `Script` dikirim sebagai `/execute =script= =as-string=` (pola
    `executeScript` centrs) — jalur yang akan dipakai Gate 1 (Fase 3).
  - `routeros.Reply` / `proto.Sentence` → `transport.Reply` kanonik;
    `!trap` → `roserr.CodeCommandFailed`, `!fatal` → `roserr.CodeSessionClosed`,
    dengan `*routeros.DeviceError` asli tetap terjangkau via `errors.As` (cause).
  - Helper baru di `roserr`: `ContextOf(err) (Context, bool)` untuk membaca
    konteks terstruktur tanpa parse teks.
  - `Adapter.List()` mengembalikan semua sentence (baris `!re` + terminal),
    `Adapter.Command()` mengembalikan sentence terminal saja.
- **Alasan:** Menjaga `Client` legacy bersih dan backward-compat (D-003);
  "satu paket = satu tanggung jawab" (DESIGN.md §2.9) — kontrak hidup di
  `transport`, bukan dibebankan ke facade lama; sesuai opsi pertama yang
  ditulis PLAN.md Fase 2; memenuhi kriteria "perbandingan perilaku sebelum/
  sesudah" dengan test fake-server yang mengasertikan words terkirim & reply
  setara.
- **Alternatif yang ditolak:** Menambah method `Command`/`Inspect` langsung di
  `Client` — mencampur tanggung jawab, menambah import `transport` ke root
  package, dan memperluas API publik tanpa kebutuhan nyata.

---

## D-008 — Fase 3: classifier Gate 1 = fungsi murni multi-pattern (urutan matching wajib)

- **Tanggal:** 2026-08-12
- **Keputusan:** Validasi syntax via `:parse` memakai **`PureSyntaxClassifier`**
  — fungsi murni tanpa I/O dengan **urutan matching wajib** (grounded grammar
  `classifyParseResult` centrs + fakta lab 7.21.5 di `docs/RESEARCH.md` §8/§10):
  1. `(evl bad parameter <name> …)` (bentuk terbungkus console) →
     `validation/unknown-attribute`;
  2. `^bad parameter <name>` (anchored) → `validation/unknown-attribute`;
  3. `^(?:syntax error|bad command name|expected …)` + `(line X column Y)` →
     `validation/syntax` + posisi;
  4. diawali `(evl …)` → valid;
  5. tidak cocok pola error → **lolos** (defensive, jangan false-positive).
  Classifier dipakai bersama oleh native-api (Fase 3) dan transport console
  (Fase 7) — tidak disalin (PLAN.md §7).
- **Alasan:** Fakta lab R10 — RouterOS **7.21.5 tidak mengeluarkan** pola
  `bad parameter <name>`; atribut tak dikenal memunculkan `expected end of
  command (line 1 column 24)` yang identik dengan syntax rusak → di 7.21.5
  Gate 1 hanya memberi sinyal **coarse** `validation/syntax`; identifikasi
  presisi (Missing/Available) adalah tugas Gate 2 (`/console/inspect`), bukan
  Gate 1. Format pesan juga terbukti beda antarversi 7.x → classifier harus
  multi-pattern, bukan satu pola per versi. Posisi `(line X column Y)`
  konsisten → diekstrak sebagai lokasi error.
- **Alternatif yang ditolak:** (a) Classifier versi-spesifik tunggal — rentan
  salah klasifikasi saat format pesan berubah antar rilis 7.x; (b) regex
  unanchored / cek `(evl` dulu — akan membiarkan `(evl bad parameter …)`
  terbungkus lolos sebagai valid (false-negative).

---

## D-009 — Fase 3/4: degradasi v6 via flag sesi (skip, bukan error)

- **Tanggal:** 2026-08-12
- **Keputusan:** `transport/nativeapi` mendapat flag sesi yang di-probe **sekali
  setelah login**: `ProbeInspect`/`SupportsInspect` (probe `request=child` ke
  `system`) dan `ProbeParse`/`SupportsParse` (probe `:put "probe"` via
  `/execute =as-string=`). Kebijakan seragam: **trap device (fitur tidak ada di
  build ini) → resolve `false` diam-diam** (bukan error); error non-trap
  (timeout/io) → dikembalikan sebagai masalah nyata. Gate 1 (`SupportsParse`)
  dan Gate 2 (`SupportsInspect`) **skip senyap** saat flag `false` — tidak ada
  error per-command. Terverifikasi di lab: v6 6.49.11 → `false`/`false`; v7
  7.21.5 → `true`/`true`.
- **Alasan:** v6 tidak punya `/console/inspect` maupun `as-string` pada
  `/execute` (fakta lab, RESEARCH.md §6–7). Validasi di v6 harus **degradasi
  eksplisit terdokumentasi**, bukan gagal — konsisten dengan prinsip DESIGN.md
  §2.1 (escape hatch eksplisit, pemanggil sadar).
- **Alternatif yang ditolak:** (a) Menolak error di v6 — memecah pemanggil yang
  hanya memakai v7; (b) menebak versi dari string versi — rapuh; probe perilaku
  lebih jujur.

---

## D-010 — Fase 4: kategori eksplisit `table`/`action`/`unknown` + override manual

- **Tanggal:** 2026-08-12
- **Keputusan:** `CommandSchema.Category` memiliki **tepat tiga nilai eksplisit**:
  `table` (verb `print`/`get` — field valid sebagai atribut), `action` (verb
  command lain — `Attributes` = argumen input, output hanya ada saat dijalankan),
  `unknown` (inspect tidak bisa resolve — caller skip dengan catatan). Kategori
  diturunkan **per verb** (bukan per path): verb `print`/`get` di path yang
  punya node-nya → `table`; verb command lain → `action`; verb tak dikenal →
  `unknown`. Override manual `Store.RegisterCategory(path, verb, category)`
  **menang atas hasil discovery** (peta kunci `path+verb`, sesuai kunci
  `Discover`).
- **Alasan:** `Attributes` kosong tidak boleh diam-diam diartikan "tidak ada
  field" — kategori menjelaskan makna daftar atribut (field vs argumen input vs
  tidak diketahui). Kunci per `(path, verb)` karena `Discover` sendiri per
  command (penyempurnaan dari formulasi awal plan yang per-path: `/ip/address`
  punya print (table) dan add (action) sekaligus).
- **Alternatif yang ditolak:** (a) Dua nilai `table`/`action` saja — tidak ada
  status "tidak bisa resolve"; (b) override per path saja — salah kategori untuk
  command berbeda di path yang sama.

---

## D-011 — Fase 4: cache `CommandSchema` TTL pendek + verifikasi call-count

- **Tanggal:** 2026-08-12
- **Keputusan:** `schema.Cache` — TTL default **30 s** (`DefaultSchemaTTL`, bisa
  dikonfigurasi `NewCacheWithTTL`), kunci `path+\x00+verb`, nilai immutable,
  invalidasi manual `Delete(path, verb)`/`Clear()`. **Efektivitas cache adalah
  kriteria selesai**: dua `Discover` beruntun untuk command yang sama → hanya
  1× round-trip inspect, diverifikasi dengan penghitung panggilan mock transport
  (M20) dan lab.
- **Alasan:** Discovery inspect mahal (4 probe per command) dan field bisa
  berubah; TTL pendek menyeimbangkan freshness dengan biaya round-trip.
- **Alternatif yang ditolak:** (a) Tanpa cache — tiap validasi = 4 probe;
  (b) cache permanen — field baru tidak pernah terdeteksi.

---

## D-012 — Fase 6: seam & probe kanonik pindah ke `Client` (root); `nativeapi` mendelegasikan

- **Tanggal:** 2026-08-12
- **Keputusan:** Implementasi kanonik kontrak transport native-api — `Command`,
  `List`, `InspectNodes`, translasi error/reply (`TranslateError`,
  `TranslateReply`), dan probe sesi (`ProbeInspect`/`SupportsInspect`,
  `ProbeParse`/`SupportsParse`) — dipindahkan ke `*routeros.Client` (package
  root, `client_transport.go`). `transport/nativeapi.Adapter` menjadi wrapper
  tipis yang **mendelegasikan 1:1** ke seam kanonik tersebut; translasi tidak
  disalin dua kali.
- **Alasan:** PLAN.md §10 mewajibkan tiga method publik di `Client`
  (Validate/Inspect/RunStructured) yang butuh seam `transport.StructuredTransport`
  di level root; namun root **tidak bisa mengimpor** `transport/nativeapi`
  karena nativeapi mengimpor root (import cycle, lihat D-007). Memindahkan
  implementasi kanonik ke root memutus cycle sekaligus menjadikan satu-satunya
  sumber translasi (prinsip yang sama dengan classifier Gate 1: tidak pernah
  disalin). Perilaku wire tidak berubah — seluruh test fake-server nativeapi
  tetap hijau tanpa modifikasi.
- **Alternatif yang ditolak:**
  - (a) `clientTransport` unexported di root + translasi tetap di nativeapi —
    menduplikasi translasi (dua sumber kebenaran).
  - (b) Orkestrator di package terpisah dengan method di luar `Client` —
    melanggar PLAN.md §10 ("Tiga method publik di Client").
  - Catatan: D-007 (Fase 2) sebelumnya menolak method `Command`/`Inspect` di
    `Client`. Keputusan ini **merevisi sebagian** D-007 untuk Fase 6: kebutuhan
    facade publik mengubah trade-off (root wajib mengimpor `transport` dan
    `schema` sekarang), dan nama seam dipakai `InspectNodes` untuk menghindari
    tabrakan dengan method publik `Inspect(path, verb)` (D-013).

---

## D-013 — Fase 6: API publik `Validate` / `Inspect` / `RunStructured` + routing gate + pipeline lazy

- **Tanggal:** 2026-08-12
- **Keputusan:** Tiga method publik di `Client` (`orchestrate.go`), sesuai
  PLAN.md §10:
  1. `Validate(ctx, *transport.Command) error` — **dry-run**: gate yang
     applicable dijalankan, command **tidak pernah dieksekusi** (aman dipanggil
     berulang, termasuk command action).
  2. `Inspect(ctx, path, verb) (*schema.CommandSchema, error)` — discovery
     murni, tanpa command konkret.
  3. `RunStructured(ctx, *transport.Command) (*transport.Reply, error)` —
     validasi lalu eksekusi command **sebagai sentence-nya sendiri** (path+verb
     = command word), bukan dibungkus `/execute` (`/execute` khusus untuk
     command `Script` dan Gate 1).
  Routing gate (PLAN.md §10): command `Script` (CLI bebas) → **Gate 1 saja**;
  command terstruktur (path/verb/atribut) → Gate 1 dilewati, **Gate 2 saja**.
  Sesi tanpa dukungan (v6) → gate skip senyap (D-009). Pipeline (probes →
  `schema.Store` → gate1 → gate2) dibangun **lazy sekali** di first-use
  (`ensureRun`, mutex-protected) — pengguna API legacy `Run*` tidak
  terpengaruh (tidak ada probe saat Dial).
- **Alasan:** Pemisahan tegas cek vs eksekusi (DESIGN.md §2.8) — dry-run dan
  run adalah dua method, bukan satu method dengan flag. Lazy probe menjaga
  backward-compat (D-003). Nama seam `InspectNodes` membebaskan nama
  `Inspect(path, verb)` untuk discovery publik sesuai PLAN.md §10.
- **Alternatif yang ditolak:**
  - (a) Eager probe saat `Dial` — menambah round-trip di tiap sesi termasuk
    yang hanya memakai legacy `Run`, mengubah perilaku sesi existing.
  - (b) Satu method dengan flag boolean dry-run — melanggar DESIGN.md §2.8
    (flag akan disalahgunakan sebagai skip validation).
  - (c) Seam bernama `Inspect` di `Client` — tabrakan nama dengan discovery
    publik.

---

## D-014 — Keputusan final versi: bump v4 (revisi D-003) — dikonfirmasi pemilik

- **Tanggal:** 2026-08-12
- **Keputusan:** Keputusan final kebijakan versi (D-003, ditunda ke Fase 6):
  **bump major ke v4**. Module path berubah menjadi
  `github.com/quiqxiq/goros/v4`; seluruh import internal, contoh, dan README
  ikut bermigrasi. **Tag rilis menjadi `v4.x.y`.**
- **Alasan:** API baru Fase 6 (`Validate`/`Inspect`/`RunStructured`) sudah
  final dan mengubah permukaan API publik secara substansial (multi-transport,
  dua gate, return terstruktur, `roserr`). Pemilik memilih bump v4 daripada
  memaksakan semua itu ke dalam garis v3 yang backward-compat — ini adalah
  desain ulang yang jujur, dan D-003 memang membuka bump jika "tak
  terhindarkan". Migrasi dilakukan mekanis (go.mod + seluruh import).
- **Alternatif yang ditolak:** Tetap v3 backward-compat (opsi yang ditawarkan)
  — menahan API baru dalam garis versi lama; pemilik memilih v4.

---

## D-015 — Persetujuan & hasil spike Gate 1 v6 (R4): mekanisme alternatif TIDAK viable

- **Tanggal:** 2026-08-12
- **Keputusan:** Pemilik **menyetujui eksperimen tulis kecil** di MT-1 (v6,
  produksi): spike untuk mencari mekanisme Gate 1 alternatif (global var +
  `environment print`, script temp + `run`) dengan auto-cleanup. Hasil spike
  (R4, RESEARCH.md §14): **tidak ada mekanisme viable** — variabel global dari
  `/execute` tidak tampil di `environment print`, `/system/script/run` gagal,
  `ret` tetap referensi internal. → Gate 1 di v6 **tetap degradasi skip**
  (D-009), kini berbasis fakta. Fallback discovery v6 (union field via
  `print` sungguhan, PLAN.md §9) tetap opsional dan **membutuhkan persetujuan
  baru** bila ingin dieksekusi (mengeksekusi `print` tanpa filter di device
  produksi).
- **Alasan:** Menghindari asumsi "Gate 1 bisa dibuat jalan di v6"; spike
  membuktikan sebaliknya dengan biaya minimal (tulis kecil + auto-cleanup
  terverifikasi).
- **Alternatif yang ditolak:** (a) Tidak melakukan spike dan tetap berasumsi
  — degradasi bisa jadi keputusan prematur; (b) fallback discovery v6 tanpa
  persetujuan — mengeksekusi command pada device produksi.

---

## D-016 — Fase 7: persempit `Gate1.Transport` ke antarmuka minimal (seam terkecil)

- **Tanggal:** 2026-08-12
- **Keputusan:** Field `Gate1.Transport` dipersempit dari
  `transport.StructuredTransport` (mewajibkan `Command` **dan** `Inspect`)
  ke antarmuka minimal baru `gate.CommandTransport` — hanya
  `Command(ctx, *transport.Command) (*transport.Reply, error)`. Gate 1 tidak
  pernah membutuhkan `Inspect` (itu tugas Gate 2), jadi mempersempit ke seam
  terkecil (DESIGN.md §2.9). Adapter console `gate/console.go` memetakan
  `Command{Script: s}` → `Run(ctx, s)` → `Reply{Type: ReplyRe, Attributes:
  {"ret": out}}`, sehingga Gate 1 berjalan tanpa perubahan logika dan
  **memakai `PureSyntaxClassifier` yang sama persis** dengan native-api
  (tidak ada classifier kedua). `RenderCLI func(*Command) string` (nil =
  `CLI()` slash-form untuk native-api; console me-wire ke `ConsoleCLI()`
  bentuk spasi, R12).
- **Alasan:** Console transport hanya punya `Run(ctx, line)`; memaksa adapter
  mengimplementasikan `Inspect` (mis. dengan capability-unsupported) membebani
  adapter dengan method yang tak relevan. Persempitan tidak memutus native-api
  — adapter native-api, `clientTransport`, dan mock semuanya sudah punya
  `Command`.
- **Alternatif yang ditolak:** (a) Adapter console memalsukan `Inspect` dengan
  error capability-unsupported — method tak relevan di tiap adapter console;
  (b) `Gate1.Transport` tetap `StructuredTransport` — menghalangi console
  tanpa duplikasi adapter.

---

## D-017 — Fase 7: helper `ValidateConsole` / `ssh.Client.Validate` (satu titik validasi console)

- **Tanggal:** 2026-08-12
- **Keputusan:** Helper validasi console sebagai satu titik masuk: `gate.ValidateConsole(ctx,
  ct CommandTransport, line string) error` — membangun script `:put [:parse
  "<cli>"]` (dengan preflight `HasUnbalancedQuotes`), menjalankan via adapter,
  dan mengklasifikasi hasil dengan `PureSyntaxClassifier`. Package `transport/ssh`
  menyediakan `Client.Validate(ctx, line)` yang mendelegasikan ke helper ini
  (console pre-wired: adapter + `RenderCLI: ConsoleCLI`).
- **Alasan:** Konsistensi alur Gate 1 console di satu tempat (bukan tiap
  pemakai menyalin alur script/parse); classifier tetap satu-satunya sumber
  klasifikasi (D-008/D-016).
- **Alternatif yang ditolak:** Tiap pemakai menyusun script `:parse` sendiri —
  duplikasi alur & risiko drift format script antar call-site.

---

## D-018 — Fase 7: `Dial` SSH memakai functional options (`DialOption`), bukan struct config

- **Tanggal:** 2026-08-12
- **Keputusan:** `ssh.Dial(ctx, addr, user string, opts ...DialOption)` memakai
  pola **functional options** — `WithPassword`, `WithPrivateKey`,
  `WithPrivateKeyFile`, `WithHostKeyPolicy` (TOFU default / Insecure opt-out
  eksplisit), `WithKnownHosts` (pin ketat format OpenSSH), `WithTimeout`.
  Zero-value call (`ssh.Dial(ctx, addr, user)`) = aman & deterministik
  (password dibaca dari env `ROUTEROS_TEST_SSH_PASSWORD` saat test, atau
  gagal jelas saat runtime tanpa auth).
- **Alasan:** API ekstensi-friendly dan tetap backward-compatible saat opsi
  baru ditambahkan (prinsip API publik, DESIGN.md §2.5); konsisten dengan pola
  opsi yang sudah ada di project. Bentuk konkret dari sketsa `DialOptions` di
  PLAN-FASE7 §5.2.
- **Alternatif yang ditolak:** Struct config `DialOptions` yang di-pass by
  value — tiap penambahan field adalah breaking change bagi pemakai yang
  memakai keyed literal.

---

## D-019 — Fase 10: metrik sesi via `Client.Metrics()` (counter inspect + latensi gate)

- **Tanggal:** 2026-08-12
- **Keputusan:** `runState` mendapat counter atomik: `InspectRoundTrips`
  (di-inkremen di `InspectNodes` — membuktikan efektivitas cache schema) dan
  latensi gate 1/2 (diukur di `validate()`). API publik
  `Client.Metrics() Metrics` mengembalikan snapshot read-only. Unit test
  mengasertikan: dua `Discover` beruntun untuk command sama →
  `InspectRoundTrips` naik hanya 1× (cache hit); `Validate`/`RunStructured`
  tercatat.
- **Alasan:** Fase 10 PLAN.md mewajibkan metrik; counter inspect memberi
  bukti terukur bahwa cache TTL (D-011) bekerja, dan latensi gate memberi
  sinyal biaya validasi per command tanpa menambah dependency observability
  eksternal.
- **Alternatif yang ditolak:** (a) Ekspor counter sebagai global package-level
  — mengganggu sesi paralel; (b) menunggu library metrik eksternal — dependency
  baru tanpa kebutuhan konkret.

---

## D-020 — Repo hygiene: `.refrences/` & `.understand-anything/` masuk `.gitignore`

- **Tanggal:** 2026-08-12
- **Keputusan:** Tambahkan `.refrences/` dan `.understand-anything/` ke
  `.gitignore`. Direktori ini berisi acuan riset lokal (centrs TypeScript,
  materi lain) dan artefak tooling (`knowledge-graph.json`) — **tidak boleh
  ikut ter-commit**.
- **Alasan:** Mencegah repo membengkak dan — yang lebih penting — mencegah
  kredensial/konfigurasi acuan lokal masuk ke git. Rilis v4.0.0 (Workstream
  D/E) memakai `git add .`; tanpa ini, direktori lokal ikut ter-push.
- **Alternatif yang ditolak:** Mencabut file acuan sebelum rilis — acuan tetap
  dibutuhkan untuk pengembangan lanjutan (mis. Fase 8 MAC-Telnet); gitignore
  lebih aman daripada mengandalkan disiplin manual.

---

## Entri berikutnya

Setiap keputusan non-trivial berikutnya (mis. hasil spike MAC-Telnet di Fase 8)
ditambahkan di sini dengan format yang sama:
**tanggal · keputusan · alasan · alternatif yang ditolak.**
