# Rencana Pengembangan go-routeros

Dokumen ini adalah spesifikasi perencanaan untuk pengembangan `go-routeros` sebagai
library Go berdiri sendiri: client multi-transport untuk RouterOS (native-api, ssh,
mac-telnet, romon, opsional REST) dengan dua gate validasi dan output command yang
terstruktur (bukan raw data). Struct/type generator BUKAN bagian dari proyek ini —
lihat bagian "Di luar scope".

Dokumen ini ditulis supaya bisa dieksekusi tahap demi tahap tanpa harus menebak-nebak
keputusan desain. Setiap fase punya tujuan, cakupan kerja, spesifikasi teknis yang
wajib diikuti, dan kriteria selesai yang bisa diverifikasi lewat test — bukan lewat
opini "kelihatannya sudah benar".

---

## 0. Keputusan yang harus ditetapkan sebelum Fase 0 dimulai

Ini bukan hal yang bisa diasumsikan begitu saja karena salah pilih di sini berarti
mengubah ulang seluruh import path di semua fase berikutnya.

1. **Module path.** Apakah ini fork keras dari package `go-routeros/routeros` yang
   sudah ada (dengan risiko konflik ekspektasi API dengan pengguna lama), atau module
   baru sepenuhnya dengan nama sendiri? Rekomendasi: module baru, karena bentuk API-nya
   memang didesain ulang total (multi-transport, dua gate, return terstruktur) — bukan
   sekadar tambal package lama.
2. **Versi Go minimum.** Tentukan satu versi (mis. Go 1.22 ke atas) supaya bisa pakai
   fitur `context` modern dan generics kalau nanti diperlukan di `schema`.
3. **Kebijakan versi.** Mulai dari `v0.x`, breaking change diperbolehkan bebas selama
   masih `v0`. Baru kunci ke `v1.0.0` setelah Fase 6 (orkestrasi validasi native-api)
   selesai dan API publik dianggap stabil.
4. **Lisensi.** Tentukan sebelum repo publik dibuat (MIT sejalan dengan referensi
   `centrs` yang jadi rujukan awal, atau lisensi lain sesuai preferensi).

Sampai keempat hal ini ditetapkan, jangan mulai Fase 0.

---

## 1. Prinsip desain yang tidak bisa dinegosiasikan

Prinsip ini berlaku di semua fase, tulis di `docs/DESIGN.md` di root repo supaya semua
orang (manusia atau AI) yang mengerjakan fase mana pun merujuk ke dokumen yang sama:

- **Validasi bukan fitur tambahan, itu produk utamanya.** Tidak ada jalur untuk
  "matikan validasi supaya command lolos" di level API publik. Kalau memang perlu
  escape hatch, itu ditaruh sebagai field konfigurasi saat membuat `Client` (satu kali,
  eksplisit, terlihat di kode inisialisasi), bukan sebagai parameter yang dikirim
  ulang di tiap pemanggilan method.
- **Return selalu terstruktur, tidak pernah raw string mentah ke pemanggil publik.**
  Layer paling luar (`transport`, `wire`) boleh bicara byte dan string mentah karena
  itu memang levelnya. Tapi begitu naik ke `gate` dan `schema`, semuanya sudah jadi
  tipe Go bernama dengan field jelas.
- **Setiap gate harus bisa dijelaskan alasannya, bukan cuma pass/fail.** Error dari
  gate manapun wajib membawa kode, apa yang hilang/salah, dan apa yang tersedia
  sebagai alternatif — supaya pemanggil (manusia, tool lain, atau agent AI) bisa
  memperbaiki sendiri tanpa harus menebak.
- **Jangan gabungkan "cek" dengan "eksekusi".** Command kategori aksi/komputasi (mis.
  `monitor-traffic`, `ping`, `torch`) punya efek samping nyata kalau dijalankan.
  Method yang sifatnya cek/dry-run tidak boleh pernah mengeksekusi command aslinya.
- **Satu paket = satu tanggung jawab.** `wire` cuma urus encoding/decoding byte.
  `session` cuma urus koneksi dan multiplexing. `gate` cuma urus keputusan valid/tidak.
  `schema` cuma urus bentuk data command. Kalau satu fungsi butuh tahu detail dari
  lebih dari satu tanggung jawab ini, itu tanda salah taruh, bukan tanda perlu
  digabung.

---

## 2. Struktur package dan tata letak repo

Package tingkat atas (semuanya di bawah satu module):

- `wire` — codec murni: encode/decode length prefix, word, sentence. Tidak tahu
  apa-apa soal jaringan, RouterOS, atau semantik command. Fungsi-fungsinya harus bisa
  diuji tanpa koneksi apa pun.
- `session` — mengelola satu koneksi native-api: login (dua skema), goroutine reader,
  tag multiplexing, `Talk`/`TalkCtx`. Bergantung pada `wire`, tidak bergantung pada
  `gate` atau `schema`.
- `transport` — interface `StructuredTransport` dan `ConsoleTransport` plus
  implementasi konkret per jenis koneksi: `nativeapi` (pakai `session`), `ssh`,
  `mactelnet`, `romon`, dan `rest` (opsional, belakangan). Setiap implementasi ada di
  sub-package sendiri (`transport/nativeapi`, `transport/ssh`, dst).
- `gate` — `Gate1` (syntax, pakai `:parse`) dan `Gate2` (semantic, pakai
  `/console/inspect`), plus orkestrator yang menjalankan gate sesuai urutan dan jenis
  transport.
- `schema` — tipe `CommandSchema`, `Attribute`, logika discovery dan cache. Ini yang
  dipakai `gate` untuk tahu atribut apa saja yang valid, dan yang nanti dikonsumsi
  proyek lain (codegen struct) sebagai data mentahnya.
- `roserr` — semua tipe error terstruktur lintas package: `ValidationError`,
  `TransportError`, `ProtocolError`. Ditaruh di package terpisah supaya `wire`,
  `session`, `gate` semua bisa mengembalikan tipe error yang sama tanpa import
  siklik.
- root package (nama sama dengan module, misal `routeros`) — facade publik: tipe
  `Client`, method `Validate`, `Inspect`, `Run`. Ini satu-satunya package yang
  dokumentasinya ditujukan untuk pengguna akhir; package lain di atas boleh dianggap
  "internal-facing" meskipun tetap exported (supaya bisa dites dan dipakai terpisah
  kalau perlu).

Tata letak test: setiap package punya `_test.go` di sebelah kode aslinya (unit test,
tidak butuh jaringan). Integration test yang butuh router sungguhan (CHR atau
perangkat fisik) ditaruh di file terpisah dengan build tag `integration`, supaya
`go test ./...` biasa tidak pernah gagal gara-gara tidak ada router yang bisa diakses.

Dokumentasi hidup yang wajib ada sejak awal:
- `docs/DESIGN.md` — prinsip di Bagian 1, tidak berubah kecuali ada alasan kuat.
- `docs/DECISIONS.md` — log singkat tiap keputusan desain non-trivial beserta
  alasannya (format: tanggal, keputusan, alasan, alternatif yang ditolak). Ini penting
  justru karena pengerjaannya lintas sesi dan mungkin lintas pelaksana (termasuk AI) —
  tanpa log ini, keputusan yang sama akan didebat ulang tiap fase.

---

## 3. Roadmap per fase

Urutan ini wajib linear. Fase berikutnya tidak boleh dimulai sebelum kriteria selesai
fase sebelumnya terpenuhi dan lulus test.

### Fase 0 — Kontrak publik (tanpa logika)

**Tujuan:** semua interface dan tipe data publik didefinisikan dan terdokumentasi
lengkap sebelum satu baris logika pun ditulis. Ini yang membuat fase-fase berikutnya
bisa dikerjakan terpisah (bahkan oleh pelaksana berbeda) tanpa saling menebak bentuk
data.

**Cakupan kerja:**
- Definisikan tipe `Command` (representasi satu command yang akan dikirim: path/verb
  atau field `Cmd` gabungan, plus map atribut string ke string).
- Definisikan tipe `Reply` dengan field: tipe reply (enum `!re`/`!done`/`!trap`/
  `!fatal`/`!empty`), map atribut, tag, dan slice word mentah untuk keperluan debug.
- Definisikan interface `StructuredTransport` (method untuk kirim command terstruktur,
  method untuk memanggil `/console/inspect`) dan `ConsoleTransport` (satu method untuk
  menjalankan teks console dan menerima teks balasan).
- Definisikan interface `Gate` (satu method: menerima context dan command terstruktur,
  mengembalikan error atau nil).
- Definisikan `CommandSchema` (path, verb, kategori, slice atribut, slice nama field)
  dan `Attribute` (nama, wajib-atau-tidak).
- Definisikan `ValidationError` dengan field kode, daftar atribut hilang, daftar
  atribut tersedia, pesan detail asli dari RouterOS, dan posisi baris/kolom kalau ada.
  Tentukan set nilai kode yang valid sejak awal: `syntax`, `unknown-attribute`,
  `unknown-path`, `transport`, `unsupported`.
- Tulis godoc untuk tiap tipe dan method di atas yang menjelaskan precondition,
  postcondition, dan kasus error — bukan cuma satu baris deskripsi generik.

**Kriteria selesai:** seluruh kode di atas terkompilasi sebagai skeleton (method boleh
mengembalikan "belum diimplementasikan"), godoc lengkap untuk 100% simbol yang
diekspor, dan sudah di-review terhadap Bagian 1 (prinsip desain) — pastikan tidak ada
satu pun method publik yang membocorkan opsi "matikan validasi" sebagai parameter.

---

### Fase 1 — Wire protocol (`wire`)

**Tujuan:** codec length-prefix, word, dan sentence yang benar dan teruji penuh,
lepas dari jaringan.

**Spesifikasi teknis wajib:**

| Rentang panjang payload | Jumlah byte prefix | Pola bit byte pertama |
|---|---|---|
| kurang dari 0x80 | 1 | `0xxxxxxx` |
| kurang dari 0x4000 | 2 | `10xxxxxx` |
| kurang dari 0x200000 | 3 | `110xxxxx` |
| kurang dari 0x10000000 | 4 | `1110xxxx` |
| 0x10000000 ke atas | 5 | byte `0xF0` diikuti 4 byte big-endian mentah |

- Fungsi decode wajib membedakan tiga kondisi secara eksplisit sebagai nilai balik
  yang berbeda, jangan digabung jadi satu boolean: (a) buffer belum cukup untuk
  membaca prefix atau body — ini BUKAN error, artinya tunggu byte berikutnya; (b) byte
  pertama tidak cocok pola manapun di tabel — ini error protokol sungguhan; (c)
  sukses, kembalikan nilai panjang dan berapa byte yang dipakai prefix.
- Word = prefix panjang (sesuai tabel) diikuti payload UTF-8 mentah tanpa escaping
  apa pun, termasuk kalau payload-nya sendiri mengandung karakter `=`.
- Sentence = rangkaian word, ditutup satu word dengan panjang nol.
- `SentenceReader` (atau nama setara) wajib inkremental: menerima potongan byte
  berapa pun ukurannya, menyimpan sisa yang belum lengkap di buffer internal, dan
  mengembalikan nol atau lebih sentence lengkap setiap kali dipanggil. Ini wajib
  karena satu `Read()` dari TCP bisa berisi setengah word atau beberapa sentence
  sekaligus — tidak boleh diasumsikan satu `Read()` = satu sentence.

**Kriteria selesai:** test table-driven yang mencakup: setiap rentang di tabel
(termasuk nilai tepat di batas bawah dan atas tiap rentang), byte pertama invalid,
input yang diumpankan dalam potongan-potongan kecil acak (mensimulasikan TCP
terfragmentasi) dan hasil akhirnya tetap sama dengan input yang diumpankan sekaligus,
serta payload yang mengandung byte `0x00` dan karakter `=` di dalamnya untuk
memastikan tidak ada parsing yang salah asumsi soal delimiter.

---

### Fase 2 — Session dan login (`session`)

**Tujuan:** satu koneksi native-api yang bisa menjalankan banyak command konkuren
dengan aman, plus login yang mendukung dua skema RouterOS.

**Spesifikasi teknis wajib:**

- Parsing reply: word pertama menentukan tipe reply. Word berikutnya masuk kategori
  atribut (diawali `=`, pisahkan pada tanda `=` **pertama** setelah itu — supaya value
  yang mengandung `=` tetap utuh) atau tag (diawali `.tag=`).
- Login modern: kirim `name` dan `password` sekaligus, sukses ditandai `!done` tanpa
  atribut `ret`.
- Login legacy: percobaan pertama balas `!done` dengan atribut `ret` berisi challenge
  heksadesimal. Hitung response dengan urutan input MD5 yang presisi: byte `0x00`,
  lalu password dalam bentuk byte, lalu challenge yang sudah didecode dari hex.
  Encode hasil MD5 ke hex dan tempelkan prefix `"00"` di depannya, kirim ulang
  `/login` dengan `name` dan `response`.
- Tag multiplexing: pemberi tag berikutnya wajib atomic (aman dipanggil dari banyak
  goroutine sekaligus). Ada satu map pending command per tag, dilindungi mutex, dan
  hanya goroutine reader (satu-satunya yang membaca dari koneksi) yang boleh menulis
  hasil ke pending command yang sesuai — pemanggil (`Talk`) hanya menunggu lewat
  channel selesai.
- `Talk` dasarnya blocking sampai reply `!done` atau `!trap` untuk tag tersebut
  diterima. `TalkCtx` membungkusnya dengan context untuk timeout/cancel — dan kalau
  context habis, definisikan perilakunya menutup seluruh koneksi (bukan cuma command
  yang sedang jalan), supaya state di reader goroutine tidak jadi tidak konsisten.
- `!fatal` berarti seluruh sesi harus dianggap mati: semua pending command yang masih
  menunggu wajib digagalkan serentak, bukan dibiarkan menunggu selamanya.

**Kriteria selesai:** unit test dengan koneksi tiruan (`net.Pipe` atau setara) yang
mencakup: login modern, login legacy dengan challenge-response yang nilainya
diverifikasi manual, beberapa `Talk` konkuren dengan tag berbeda yang masing-masing
mendapat jawaban benar, satu `Talk` yang menerima `!trap` dan errornya benar, dan satu
`!fatal` yang menggagalkan semua pending command sekaligus. Seluruh test package ini
wajib lulus `go test -race` tanpa ada race yang terdeteksi.

---

### Fase 3 — Gate 1: syntax (`gate`)

**Tujuan:** validasi syntax command lewat `:parse`, termasuk membaca isi `ret` secara
eksplisit — ini poin yang bahkan referensi awal (`centrs`) sendiri akui belum mereka
kerjakan di native-api, jadi ini nilai tambah nyata sejak fase awal.

**Spesifikasi teknis wajib:**

- Bangun script dengan pola: `:put [:parse "<command asli>"]`, di mana command asli
  di-escape sebagai string literal RouterOS (backslash dan tanda kutip ganda di dalam
  command harus di-escape supaya tidak merusak literal string-nya).
- Kirim lewat command `/execute` dengan atribut `script` berisi script di atas dan
  atribut `as-string` diset kosong (menandakan hasil `:put` dikembalikan sebagai
  atribut, bukan dicetak ke output interaktif).
- Ada dua jalur hasil yang harus dibedakan: kalau `/execute` sendiri gagal total
  (misalnya script-nya sendiri tidak valid karena kutip tidak seimbang), RouterOS
  balas `!trap` — ini murni kegagalan level script/transport, bukan hasil parse dari
  command yang sedang divalidasi.
- Kalau `/execute` sukses (`!done`), isi `ret` dari reply itulah yang berisi hasil
  `:parse` sesungguhnya. Cocokkan isi `ret` terhadap dua pola: frasa yang menandakan
  atribut tidak dikenal (diikuti nama atributnya, untuk diekstrak sebagai bagian dari
  `ValidationError`), dan frasa yang menandakan syntax rusak (beberapa variasi kalimat
  berbeda menandakan hal yang sama — command salah, nama command tidak dikenal, atau
  ada bagian yang seharusnya diisi tapi tidak ada). Kalau salah satu pola cocok,
  kembalikan `ValidationError` dengan kode yang sesuai; kalau tidak ada yang cocok,
  command dianggap lolos syntax.
- Kalau pesan errornya menyertakan informasi posisi (baris dan kolom), ekstrak dan
  simpan di field posisi pada `ValidationError` — ini bukan wajib untuk lolos gate,
  tapi meningkatkan kualitas pesan error ke pemanggil.

**Kriteria selesai:** unit test yang tidak menyentuh jaringan — pakai fixture teks
yang mewakili isi `ret` untuk berbagai kasus nyata (command valid, atribut tidak
dikenal, command tidak dikenal, syntax rusak dengan dan tanpa informasi posisi) dan
verifikasi classifier menghasilkan `ValidationError` yang benar untuk tiap fixture.

---

### Fase 4 — Gate 2: semantic dan schema discovery (`gate` + `schema`)

**Tujuan:** validasi keberadaan atribut/path lewat `/console/inspect`, sekaligus
membangun `CommandSchema` yang jadi nilai tambah utama library ini, plus caching
supaya tidak boros round-trip.

**Spesifikasi teknis wajib:**

- `/console/inspect` dikirim sebagai command native-api biasa (bukan endpoint
  khusus), dengan atribut `request` (`child` atau `completion`) dan atribut `path`.
- Atribut `path` **wajib** digabung dengan koma antar token menu (misalnya menu
  `ip address add` menjadi satu string dipisah koma), **tidak boleh** memakai garis
  miring — bentuk garis miring tidak akan cocok apa pun dan hasilnya selalu kosong.
- `request=child`: hasilnya daftar node di bawah path tersebut. Filter node yang
  tipenya menandakan "argumen" (bukan "sub-command") — itulah daftar atribut valid
  untuk command tersebut.
- `request=completion`: dipakai untuk dua kebutuhan berbeda yang harus ditangani
  terpisah tapi digabung hasilnya sebagai satu daftar atribut tambahan (union dengan
  hasil `child`, dengan dedup): (a) melengkapi atribut yang mungkin tidak muncul di
  `child` tapi tetap valid, dan (b) trik discovery nama field data — untuk command
  `print`, tanyakan completion pada argumen semacam daftar-field (contoh: `.proplist`)
  karena jawabannya justru berupa daftar nama field data itu sendiri; untuk command
  singleton semacam `get`, pola yang sama berlaku lewat argumen nilai (`value-name`).
- Kategorisasi command wajib eksplisit di `CommandSchema.Category`, dengan tiga nilai:
  "table" (command yang mendukung trik discovery field lewat completion di atas —
  attribut dan field bisa didapat statis), "action" (command yang punya argumen input
  valid lewat `child` tapi TIDAK punya mekanisme statis untuk tahu field output-nya —
  field output-nya cuma "ada" saat command benar-benar dijalankan), dan "unknown"
  (belum bisa ditentukan, misalnya karena inspect tidak didukung router). Jangan
  pernah mengembalikan field kosong untuk command kategori "action" tanpa
  memberitahu lewat `Category` kenapa itu terjadi.
- Sediakan mekanisme override manual untuk kategori — sebuah daftar pemetaan path ke
  kategori yang bisa diisi pemanggil, untuk kasus di mana heuristik otomatis salah
  menebak.
- Cache: kunci cache adalah gabungan token path dan verb. Nilai yang disimpan adalah
  `CommandSchema` lengkap. TTL default pendek (definisikan sebagai konstanta yang bisa
  dikonfigurasi lewat `Client`, jangan hardcode tanpa jalan keluar), plus method untuk
  invalidasi manual satu entri atau seluruh cache.

**Kriteria selesai:** unit test dengan fixture reply tiruan untuk `/console/inspect`
(`child` dan `completion`) yang memverifikasi: union dan dedup atribut benar, trik
discovery field lewat `.proplist`/`value-name` menghasilkan daftar field yang benar,
kategori "table" vs "action" terisi sesuai fixture, cache hit tidak melakukan
round-trip kedua (diverifikasi lewat penghitung pemanggilan transport tiruan), dan
override manual kategori benar-benar dipakai saat ada.

---

### Fase 5 — Deteksi versi dan fallback v6/v7

**Tujuan:** `/console/inspect` adalah fitur RouterOS v7. Sesi harus tahu sejak awal
apakah fitur ini tersedia, supaya Gate 2 bisa berdegradasi dengan baik tanpa membuat
tiap command gagal satu-satu.

**Cakupan kerja:**
- Sekali di awal sesi (setelah login sukses), lakukan satu panggilan `/console/inspect`
  dengan `request=child` ke path yang pasti ada di semua versi RouterOS (path identitas
  sistem). Simpan hasilnya sebagai flag boolean di level sesi, jangan probe ulang tiap
  kali dibutuhkan.
- Kalau tidak didukung: Gate 2 di-skip otomatis untuk seluruh sesi itu, Gate 1 tetap
  jalan penuh karena `:parse` bukan fitur v7-only. Jangan lempar error ke pemanggil
  hanya karena Gate 2 tidak tersedia — ini kondisi yang valid dan harus ditangani
  senyap di level orkestrasi (Fase 6), bukan jadi kejutan di tiap pemanggilan command.
- Sediakan strategi fallback discovery opsional untuk v6 (dijalankan hanya kalau
  pemanggil memilihnya secara eksplisit, karena ini benar-benar mengeksekusi command
  print tanpa filter dan mengambil union nama field dari hasilnya — bukan dry-check):
  jalankan command print pada path yang diminta, ambil semua nama atribut yang muncul
  di record hasilnya sebagai pengganti hasil trik `.proplist`.

**Kriteria selesai:** dua skenario test dengan transport tiruan — satu yang membalas
sukses untuk probe inspect, satu yang membalas gagal (misalnya command tidak dikenal)
— dan verifikasi flag sesi terisi benar di kedua kasus, serta Gate 2 benar-benar
di-skip (bukan gagal) pada skenario kedua.

---

### Fase 6 — Orkestrasi validasi dan API publik (`Client.Validate` / `Inspect` / `Run`)

**Tujuan:** menyatukan Gate 1, Gate 2, dan eksekusi command asli jadi satu alur yang
konsisten, dan di sinilah API publik utama library ini terbentuk.

**Cakupan kerja:**
- Definisikan urutan pasti: kalau command punya bentuk string CLI bebas, jalankan
  Gate 1 dulu; kalau command sudah terstruktur (path/verb/atribut tanpa string bebas),
  Gate 1 dilewati. Setelah itu jalankan Gate 2 kalau sesi mendukungnya (lihat Fase 5).
  Hanya kalau kedua gate yang applicable lolos, command asli dikirim — dan dikirim
  langsung sebagai sentence-nya sendiri (path+verb sebagai `Cmd`, atribut sebagai
  attribute word), **bukan** dibungkus lagi lewat `/execute`. `/execute` hanya dipakai
  untuk keperluan Gate 1 dan untuk command yang memang tidak bisa distrukturkan
  (mengandung subshell atau tidak diawali `/`).
- Sediakan tiga method publik di `Client` yang jelas bedanya: satu yang hanya
  menjalankan gate tanpa pernah mengeksekusi command asli (dry-run, aman dipanggil
  berkali-kali termasuk untuk command kategori "action"), satu yang mengembalikan
  `CommandSchema` untuk suatu path tanpa perlu ada command konkret yang mau dijalankan
  (murni discovery), dan satu yang menjalankan penuh (validasi lalu eksekusi).
  Method dry-run dan method run harus benar-benar dua method terpisah — jangan satu
  method dengan flag boolean "eksekusi atau tidak", karena itu membuka celah flag
  itu disalahgunakan sebagai "skip validation" yang dilarang di Bagian 1.
- Error yang dikembalikan ke pemanggil publik selalu `ValidationError` (atau error
  transport/protocol yang juga terstruktur) — tidak pernah string mentah dari
  RouterOS tanpa dibungkus.

**Kriteria selesai:** integration test terhadap CHR (Cloud Hosted Router) sungguhan —
bukan lagi mock — mencakup minimal: satu command tabel data valid (berhasil dan
hasilnya benar), satu command dengan atribut salah nama (gagal di Gate 2 dengan daftar
`Missing`/`Available` yang benar), satu command dengan syntax rusak (gagal di Gate 1),
satu command kategori aksi dipanggil lewat method dry-run (lolos gate, TIDAK
benar-benar tereksekusi — diverifikasi lewat efek samping yang seharusnya tidak
terjadi), dan command yang sama dipanggil lewat method run (benar-benar tereksekusi).

---

### Fase 7 — ConsoleTransport: SSH

**Tujuan:** transport kedua, sekaligus jadi pola untuk transport console-based
lainnya (mac-telnet, romon) di fase berikutnya.

**Cakupan kerja:**
- Tentukan mekanisme menjalankan command di RouterOS lewat SSH: RouterOS
  memperlakukan sesi SSH sebagai console RouterOS sendiri (bukan shell Unix biasa),
  jadi eksekusi command dilakukan dengan mengirim teks command ke sesi tersebut dan
  membaca teks balasannya, bukan lewat mekanisme "exec command" generik SSH.
- Karena console text sudah memuat info syntax dan semantic sekaligus, gate untuk
  transport ini cuma satu (bukan dua terpisah seperti native-api): kirim
  `:put [:parse "<command>"]` yang sama seperti Fase 3, tapi hasilnya dibaca dari teks
  output console, dan classifier pola-nya sama persis dengan yang dipakai Gate 1 —
  ini alasan classifier itu sebaiknya jadi fungsi murni yang bisa dipakai ulang dari
  kedua tempat, bukan disalin.
- Definisikan model concurrency untuk transport ini: satu koneksi SSH pada dasarnya
  synchronous (satu command harus selesai sebelum command berikutnya dikirim di sesi
  yang sama, karena outputnya berupa teks berurutan tanpa tag multiplexing seperti
  native-api). Kalau pemanggil butuh beberapa command konkuren lewat SSH, itu berarti
  membuka beberapa koneksi SSH terpisah, bukan memultipleks satu koneksi.

**Kriteria selesai:** integration test terhadap CHR sungguhan lewat SSH: command valid
dan command dengan syntax/atribut salah, dan verifikasi pesan `ValidationError` yang
dihasilkan punya kode dan makna yang setara dengan hasil dari native-api untuk
kesalahan yang sama (supaya pemanggil di level `Client` tidak perlu tahu transport
mana yang sedang dipakai di baliknya).

---

### Fase 8 — RoMON dan MAC-Telnet

**Tujuan:** transport untuk akses perangkat yang belum punya IP/route — relevan untuk
skenario provisioning awal perangkat baru.

**Sebelum mulai fase ini — wajib spike/riset kelayakan dulu:** akses MAC-Telnet dan
RoMON di level Go kemungkinan besar butuh operasi socket mentah di layer data-link
(bukan sekadar TCP/UDP biasa), yang di Linux biasanya butuh privilege khusus
(`CAP_NET_RAW`) dan pustaka pendukung yang belum tentu matang di ekosistem Go. Jangan
alokasikan waktu penuh untuk fase ini sebelum ada bukti singkat (spike beberapa hari)
bahwa pendekatan yang dipilih bisa jalan di lingkungan target. Kalau spike menunjukkan
ini terlalu mahal untuk sekarang, fase ini boleh ditunda dan ditandai sebagai backlog
tanpa menghentikan fase-fase lain.

**Cakupan kerja (setelah spike berhasil):**
- RoMON butuh router perantara yang mendukung RoMON aktif sebagai jembatan ke
  perangkat tujuan yang belum punya IP — definisikan bagaimana `Client` menerima
  parameter router perantara ini saat membuat transport jenis ini.
- Kedua transport ini sama-sama console-based, jadi reuse gate gabungan yang sama
  seperti Fase 7 (satu `:parse` gabungan, classifier yang sama).
- Sediakan mekanisme discovery perangkat di level L2 (broadcast) sebagai langkah
  sebelum bisa connect — ini kebutuhan yang tidak ada di native-api/SSH karena
  perangkat tujuan belum punya alamat routable.

**Kriteria selesai:** koneksi dan eksekusi command berhasil terhadap minimal satu
perangkat uji nyata di lab (bukan simulasi), untuk kedua transport.

---

### Fase 9 (opsional) — REST transport

**Tujuan:** hanya dikerjakan kalau memang ada kebutuhan konkret setelah native-api
solid — jangan dikerjakan lebih dulu "karena kelihatan gampang".

**Cakupan kerja:**
- Transport ini tidak butuh wire protocol custom (Fase 1) — cukup HTTP dan JSON.
- Karena bentuk input REST sudah terstruktur (bukan string CLI bebas), Gate 1 dilewati
  sama seperti command terstruktur di native-api. Gate 2 tetap wajib jalan dengan cara
  yang sama seperti Fase 4 (REST tetap bisa memanggil `/console/inspect` sebagai
  endpoint biasa).

**Kriteria selesai:** setara Fase 6, tapi lewat REST — satu command valid dan satu
command gagal validasi, hasilnya konsisten dengan transport lain.

---

### Fase 10 — Observability, dokumentasi, dan pengerasan (hardening)

**Cakupan kerja:**
- Logging terstruktur (bukan `fmt.Println`) untuk tiap panggilan gate dan transport,
  dengan level yang bisa diatur dari luar, dan tanpa membocorkan kredensial (password,
  MD5 challenge mentah) ke log dalam kondisi apa pun.
- Metrik dasar: jumlah round-trip ke `/console/inspect` per sesi (untuk membuktikan
  cache di Fase 4 benar-benar efektif) dan latensi tiap gate.
- Godoc lengkap untuk 100% simbol publik yang tersisa (bukan cuma yang di Fase 0).
- README dengan alur pemakaian tingkat tinggi (ditulis belakangan, bukan bagian dari
  dokumen perencanaan ini).

**Kriteria selesai:** `go vet` dan linter bersih, semua simbol publik terdokumentasi,
metrik dan log bisa diverifikasi manual lewat satu sesi percobaan nyata.

---

## 4. Strategi testing dan kriteria akseptasi global

- Tidak ada fase yang dianggap selesai tanpa lulus unit test-nya sendiri. Fase yang
  bicara ke jaringan sungguhan (5 ke atas untuk sebagian, 6, 7, 8, 9) juga wajib lulus
  integration test terhadap router nyata sebelum dianggap tuntas — mock saja tidak
  cukup untuk fase-fase itu.
- `go test -race` wajib khusus untuk package `session` dan `gate` di tiap CI run,
  karena keduanya yang paling rawan race (multiplexing, cache).
- Integration test dipisah dari unit test lewat build tag, dan dijalankan sebagai job
  CI terpisah (manual atau terjadwal) karena butuh akses ke perangkat/CHR — jangan
  membuat pipeline utama gagal hanya karena lab sedang tidak bisa diakses.
- Target versi RouterOS untuk pengujian: satu versi 7.x terbaru sebagai target utama,
  minimal satu versi 7.x lain untuk memeriksa kestabilan format pesan `:parse` dan
  `/console/inspect` antar-versi (formatnya berpotensi berubah), dan satu instance v6
  khusus untuk memverifikasi jalur fallback di Fase 5.
- Simpan `docs/DECISIONS.md` (lihat Bagian 2) terus diperbarui setiap ada keputusan
  yang menyimpang dari dokumen ini — dokumen ini adalah rencana awal, bukan sumber
  kebenaran yang tidak boleh direvisi, tapi revisinya harus tercatat.

---

## 5. Di luar scope (sengaja tidak dikerjakan di sini)

- **Struct/type generator.** Proyek terpisah di masa depan yang mengonsumsi
  `CommandSchema` dari library ini sebagai input. Library ini berhenti tanggung
  jawabnya di titik "sini data dan skemanya, sudah tervalidasi" — bukan sampai
  "sini kode struct Go-nya".
- **Request mode `highlight` dan `syntax` di `/console/inspect`.** Bahkan referensi
  awal (`centrs`) belum meng-wire dua mode ini. Tandai sebagai extension point untuk
  nanti kalau memang dibutuhkan, jangan dikerjakan sekarang.
- **Integrasi ke sistem/driver/agent lain apa pun.** Library ini berdiri sendiri;
  siapa pun yang memakainya (termasuk proyek lain) adalah konsumen eksternal, bukan
  bagian dari rencana ini.

---

## 6. Urutan eksekusi ringkas

Fase 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 (dengan spike kelayakan lebih dulu) →
9 (opsional, kapan saja setelah Fase 6 kalau dibutuhkan) → 10.

Gerbang wajib di tiap transisi: kriteria selesai fase sebelumnya terpenuhi dan lulus
test yang relevan, sebelum fase berikutnya dimulai.