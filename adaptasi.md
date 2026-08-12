# Roadmap Pengembangan: Ekstensi `go-routeros` — Multi-Transport & Validasi Command Terstruktur

> Dokumen planning murni. Tidak ada contoh kode — setiap fase berisi arahan,
> keputusan desain yang harus diambil, dan kriteria selesai (Definition of
> Done) yang cukup jelas untuk dieksekusi langsung, oleh siapa pun/apa pun
> yang mengimplementasikan.

---

## 0. Ruang Lingkup Project

**Tujuan inti:** mengembangkan `go-routeros` secara langsung (bukan wrapper
eksternal di atasnya) supaya:

1. Mendukung banyak jenis transport ke perangkat RouterOS: native-api
   (sudah ada), SSH, MAC-Telnet, RoMON.
2. Punya lapisan validasi command berbasis `/console/inspect` yang
   mengembalikan informasi atribut/parameter yang valid untuk sebuah
   command, dipakai untuk memvalidasi sebelum eksekusi.
3. Return value dari command tidak lagi berupa data mentah tak-berstruktur
   (`map[string]string` generik tanpa makna semantik), tapi tipe Go yang
   terstruktur dan bermakna — **khusus untuk hasil validasi/inspeksi**
   (misalnya daftar atribut suatu command dikembalikan sebagai slice of
   struct bernama, bukan `[]map[string]string` mentah).

**Yang secara eksplisit DI LUAR lingkup project ini** (dicatat supaya tidak
scope-creep saat implementasi):

- Auto-generate struct Go per-path RouterOS dari hasil discovery (mis.
  `type IPAddressAddParams struct {...}` otomatis). Itu fitur terpisah untuk
  fase pengembangan berikutnya, di luar project ini. Project ini cukup
  berhenti di titik "mengembalikan data atribut/parameter secara
  terstruktur", bukan sampai menghasilkan file `.go` struct baru.
- Business logic apa pun di atas RouterOS (subscriber management, billing,
  dsb). Ini murni pengembangan library transport+validasi.

---

## 1. Prinsip Desain yang Harus Dipegang di Semua Fase

Tulis prinsip-prinsip ini di dokumen kontribusi/README project sejak awal,
supaya semua fase berikutnya konsisten:

- **Transport-agnostic di level atas.** Kode validasi, discovery, dan tipe
  data hasil tidak boleh tahu (dan tidak boleh peduli) apakah di baliknya
  jalan native-api, SSH, MAC-Telnet, atau RoMON. Semua transport harus
  mengekspos kontrak yang sama ke lapisan atasnya.
- **Jangan pecah kompatibilitas kode native-api yang sudah ada** kecuali
  benar-benar perlu. Kode native-api `go-routeros` saat ini sudah matang
  dan dipakai orang lain — perubahan di situ harus lewat proses refactor
  hati-hati (perkenalkan interface baru, migrasikan implementasi lama ke
  interface itu, jangan tulis ulang dari nol), bukan rewrite total.
- **`context.Context` wajib di setiap operasi jaringan**, di semua
  transport, sejak desain interface pertama kali — bukan ditambahkan
  belakangan. Ini krusial karena beberapa transport baru (MAC-Telnet,
  RoMON) sifatnya broadcast/discovery-based dan gampang nge-hang tanpa
  timeout yang jelas.
- **Error harus punya taksonomi konsisten**, bukan `errors.New(...)` bebas
  di banyak tempat. Rancang kategori error (transport, protokol,
  autentikasi, validasi-syntax, validasi-semantic, tidak-didukung-oleh-
  transport-ini) sebagai bagian dari desain, bukan detail implementasi yang
  menyusul belakangan.
- **Setiap fase harus berakhir dengan sesuatu yang testable dan
  ter-dokumentasi**, bukan cuma "kode jalan di laptop saya". Definisikan
  kriteria selesai eksplisit per fase (ada di tiap bagian di bawah).
- **Transport baru yang risikonya tinggi/spesifikasinya tidak resmi (MAC-
  Telnet, RoMON) harus punya jalur riset terpisah sebelum coding**, supaya
  waktu implementasi tidak dihabiskan untuk trial-and-error protokol.

---

## 2. Fase 0 — Riset & Validasi Asumsi (WAJIB sebelum menulis kode apa pun)

Tujuan fase ini murni mengumpulkan fakta, supaya fase-fase implementasi
berikutnya tidak menebak-nebak di tengah jalan. Semua poin di bawah harus
punya jawaban tertulis (dicatat di dokumen riset internal project) sebelum
Fase 1 dimulai.

**Poin yang wajib diriset dan dikonfirmasi di lab (router/CHR sungguhan):**

- Apakah sesi SSH ke RouterOS mendukung eksekusi command non-interaktif
  satu-shot (exec channel biasa di SSH), atau cuma mode interaktif
  berbasis PTY yang mengharuskan parsing teks prompt/echo? Ini menentukan
  seberapa rumit implementasi transport SSH nantinya.
- Bagaimana perilaku paging console RouterOS lewat SSH untuk output
  panjang (apakah flag semacam "without-paging" bisa dipaksa lewat
  argumen command, atau harus di-disable lewat pengaturan console
  terpisah dulu)?
- Untuk MAC-Telnet: konfirmasi ulang skema handshake dan apakah enkripsi
  diwajibkan pada versi RouterOS yang jadi target dukungan project ini
  (perilaku ini diketahui berubah antar versi RouterOS — jangan asumsikan
  dari dokumentasi lama, verifikasi di versi yang jadi target).
- Untuk MAC-Telnet: konfirmasi kebutuhan raw socket level Ethernet
  (bukan soket TCP/UDP biasa) dan implikasinya di tiap OS target (Linux,
  Windows, macOS) — termasuk kebutuhan privilese (capability/administrator)
  saat runtime.
- Untuk RoMON: kumpulkan semua sumber referensi yang tersedia (dokumentasi
  resmi MikroTik yang ada, tulisan komunitas, implementasi open-source
  yang sudah reverse-engineer protokol ini) dan nilai secara jujur:
  apakah cukup lengkap untuk implementasi produksi, atau harus ditandai
  "eksperimental/best-effort" di rilis awal.
- Apakah `/console/inspect` bisa dipanggil lewat sesi console berbasis
  teks (SSH/MAC-Telnet/RoMON) dan menghasilkan output yang bisa
  di-parse balik jadi data terstruktur — atau cuma benar-benar berfungsi
  penuh lewat native-api/REST. Ini menentukan apakah Gate semantic (lihat
  Fase 6) bisa dipakai seragam di semua transport atau harus punya
  jalur fallback berbeda untuk transport berbasis console murni.
- Rentang versi RouterOS yang jadi target dukungan resmi project ini
  (mis. "v7.x ke atas penuh, v6.x best-effort tanpa gate semantic") —
  ini harus jadi keputusan eksplisit tertulis, bukan diasumsikan diam-diam.

**Kriteria selesai Fase 0:** ada dokumen riset internal (boleh cukup satu
file catatan) yang menjawab semua poin di atas dengan hasil pengujian nyata
(bukan tebakan), plus keputusan eksplisit soal versi RouterOS mana yang
didukung penuh vs sebagian.

---

## 3. Fase 1 — Refactor Fondasi: Abstraksi Transport

Tujuan fase ini: menyiapkan "slot" di dalam `go-routeros` supaya transport
baru bisa ditambahkan tanpa mengganggu kode native-api yang sudah ada, dan
supaya lapisan validasi/discovery di fase-fase berikutnya bisa ditulis
sekali dan otomatis bekerja di semua transport.

**Yang perlu dikerjakan:**

- Definisikan satu **kontrak transport** (interface) yang jadi titik temu
  semua jenis koneksi: kemampuan mengirim satu command dan menerima balikan
  terstruktur, kemampuan menjalankan script/`:parse` mentah (dibutuhkan
  Fase 6), kemampuan melaporkan apakah transport ini mendukung
  `/console/inspect` atau tidak, dan kemampuan menutup koneksi dengan bersih.
- Definisikan **satu bentuk data balikan kanonik** yang tidak terikat ke
  detail wire protocol tertentu (native-api punya bentuk sentence sendiri,
  console-based transport punya bentuk teks tabel sendiri) — semua
  transport wajib menerjemahkan hasil mentahnya ke bentuk kanonik ini
  sebelum dikembalikan ke pemanggil.
- Definisikan taksonomi error di level ini juga (lihat prinsip desain di
  atas) — supaya transport baru tinggal memetakan error internalnya ke
  kategori yang sudah ada, bukan menciptakan kategori baru sembarangan.
- Tentukan strategi kompatibilitas untuk kode native-api yang sudah ada:
  apakah tipe/fungsi publik lama tetap dipertahankan sebagai alias/wrapper
  di atas arsitektur baru (supaya pengguna lama tidak perlu ubah kode), atau
  project ini memang berencana rilis versi major baru dengan API yang boleh
  berbeda. Ini keputusan yang harus diambil sadar di awal, bukan menyusul.
- Rancang bagaimana proses "membangun koneksi" (dial/connect) dibedakan per
  transport tapi tetap menghasilkan objek yang sama-sama memenuhi kontrak
  transport di atas — termasuk bagaimana parameter koneksi yang sifatnya
  spesifik-transport (misalnya alamat MAC untuk MAC-Telnet, vs
  host:port untuk native-api/SSH) direpresentasikan tanpa mengotori
  kontrak umum dengan field yang cuma relevan untuk sebagian transport.

**Kriteria selesai Fase 1:** ada kontrak transport yang terdefinisi jelas
dan terdokumentasi, kode native-api existing sudah dipetakan/diadaptasi ke
kontrak ini (lihat Fase 2) sebagai bukti kontraknya masuk akal, dan ada
setidaknya satu implementasi transport "dummy"/mock yang dipakai untuk
menguji lapisan-lapisan di atasnya tanpa perlu router sungguhan.

---

## 4. Fase 2 — Transport Native-API (Adaptasi, Bukan Rewrite)

Kode native-api `go-routeros` yang sudah ada (codec wire protocol, tag
multiplexing, login handshake, mode sync/async) **sudah matang** — tugas
fase ini murni mengadaptasi kode itu ke kontrak transport dari Fase 1,
bukan menulis ulang logikanya.

**Yang perlu dikerjakan:**

- Petakan setiap kemampuan yang dibutuhkan kontrak transport (kirim
  command, jalankan script mentah, deteksi dukungan inspect, tutup
  koneksi) ke fungsi-fungsi yang sudah ada di kode native-api saat ini.
- Pastikan proses adaptasi ini **tidak mengubah perilaku wire-level yang
  sudah teruji** — perubahan di fase ini seharusnya murni di titik masuk/
  keluar (bagaimana command dikirim, bagaimana hasil diterjemahkan ke
  bentuk kanonik), bukan di codec/parsing byte-level.
- Tambahkan mekanisme deteksi dukungan `/console/inspect` di level koneksi
  native-api (probe sekali di awal sesi ke path yang pasti selalu ada di
  semua versi, cache hasilnya) — ini dibutuhkan Fase 6 dan Fase 7, jadi
  paling masuk akal disiapkan begitu transport native-api sudah teradaptasi.
- Tulis test regresi yang membandingkan perilaku transport native-api
  sebelum dan sesudah adaptasi (memastikan tidak ada perilaku yang
  diam-diam berubah untuk pengguna yang sudah pakai fungsi lama).

**Kriteria selesai Fase 2:** transport native-api berjalan penuh lewat
kontrak baru, seluruh test yang sudah ada di `go-routeros` (kalau ada)
tetap lulus, dan deteksi dukungan inspect berfungsi dan sudah diuji ke
minimal dua versi RouterOS berbeda (satu yang mendukung `/console/inspect`,
satu yang tidak, sesuai temuan Fase 0).

---

## 5. Fase 3 — Transport SSH

**Yang perlu dikerjakan:**

- Tentukan strategi eksekusi command berdasarkan temuan Fase 0: kalau
  RouterOS mendukung exec channel non-interaktif, prioritaskan jalur itu
  (jauh lebih sederhana dan robust). Kalau tidak, rancang jalur interaktif
  berbasis PTY dengan penanganan prompt/echo yang eksplisit.
- Rancang parser output console RouterOS: command semacam `print`
  menghasilkan tabel teks berkolom, sementara command semacam `get`/status
  command lain sering menghasilkan format "nama: nilai" per baris. Parser
  ini harus cukup general untuk menangani kedua pola, dan harus punya jalur
  eksplisit untuk kasus yang gagal di-parse (jangan diam-diam mengembalikan
  data kosong/salah tanpa sinyal error).
- Tangani encoding/ANSI escape sequence yang mungkin muncul di output
  console (RouterOS console punya elemen styling/warna di beberapa
  konteks) — pastikan parser membersihkan ini sebelum parsing data,
  bukan ikut ke-parse sebagai bagian dari data.
- Rancang penanganan autentikasi SSH (password, dan idealnya juga
  dukungan public-key sejak awal karena ini pola umum di lingkungan
  produksi) dan pastikan error autentikasi dipetakan ke kategori error yang
  jelas (beda dari error transport/jaringan biasa).
- Pastikan transport ini melaporkan dukungan `/console/inspect` sesuai
  hasil riset Fase 0 (kemungkinan besar "ya, tapi lewat jalur teks", atau
  "tidak" — tergantung temuan riset).

**Kriteria selesai Fase 3:** transport SSH bisa menjalankan command apa pun
dan mengembalikan data dalam bentuk kanonik yang sama seperti transport
native-api untuk command yang sama (diuji dengan membandingkan hasil
`print` yang sama lewat native-api vs SSH, hasilnya harus setara secara
semantik), termasuk penanganan error yang jelas untuk kasus autentikasi
gagal, koneksi putus di tengah, dan output yang gagal di-parse.

---

## 6. Fase 4 — Transport MAC-Telnet

Fase ini secara teknis jauh lebih berat dari SSH karena beroperasi di
level Ethernet frame, bukan di atas TCP/IP biasa. Rencanakan alokasi waktu
lebih besar untuk fase ini dibanding fase transport lainnya.

**Yang perlu dikerjakan:**

- Tentukan strategi akses raw socket per platform (Linux, Windows, macOS)
  berdasarkan hasil riset Fase 0 — termasuk keputusan apakah project ini
  akan bergantung pada library eksternal untuk packet capture/raw socket,
  atau mengimplementasikan sendiri per-platform. Ini keputusan arsitektur
  besar yang berdampak ke seluruh fase ini, jadi harus diputuskan di awal
  fase, bukan di tengah implementasi.
- Rancang mekanisme discovery perangkat (untuk kasus perangkat tanpa IP
  yang cuma bisa ditemukan lewat broadcast di layer 2) sebagai kemampuan
  terpisah dari koneksi itu sendiri — supaya API-nya jelas membedakan
  "temukan perangkat yang ada di segment ini" vs "hubungkan ke MAC address
  spesifik yang sudah diketahui".
- Implementasikan handshake sesi MAC-Telnet sesuai temuan riset (nomor
  urut paket, acknowledgment, session ID, dan enkripsi kalau memang
  diwajibkan versi target).
- Setelah sesi MAC-Telnet terbentuk, ujungnya adalah stream console teks
  yang sama sifatnya dengan SSH — **manfaatkan ulang parser output
  console dari Fase 3**, jangan menulis parser kedua yang terpisah. Titik
  perbedaan MAC-Telnet dari SSH murni ada di lapisan pembentukan
  koneksi/framing paket, bukan di parsing hasil command.
- Rancang penanganan kebutuhan privilese runtime (banyak OS mewajibkan hak
  akses admin/root untuk raw socket) — pastikan library memberi pesan
  error yang jelas dan actionable kalau proses berjalan tanpa privilese
  yang cukup, bukan gagal dengan pesan low-level yang membingungkan.

**Kriteria selesai Fase 4:** transport MAC-Telnet bisa menemukan dan
terhubung ke perangkat di segment jaringan lokal tanpa IP, menjalankan
command, dan mengembalikan hasil dalam bentuk kanonik yang sama seperti
transport lain — diuji di jaringan lab dengan minimal dua versi RouterOS
berbeda sesuai kebutuhan enkripsi yang ditemukan di Fase 0.

---

## 7. Fase 5 — Transport RoMON

Fase ini punya risiko tertinggi karena spesifikasi protokolnya paling
sedikit terdokumentasi resmi. Perlakukan fase ini sebagai "best-effort
berbasis bukti", dan **jangan mulai coding sebelum keputusan kelayakan
dari Fase 0 jelas.**

**Yang perlu dikerjakan:**

- Berdasarkan hasil riset Fase 0, putuskan secara eksplisit apakah fase
  ini dikerjakan penuh di rilis pertama, atau ditandai sebagai fitur
  eksperimental yang dirilis terpisah setelah transport lain stabil.
  Keputusan ini sebaiknya diambil sebelum alokasi waktu ke fase ini
  dikunci, supaya tidak mengorbankan fase-fase yang lebih pasti.
- Rancang mekanisme discovery agent RoMON (perangkat RoMON yang bisa
  dijangkau di jaringan) sebagai kemampuan terpisah, mengikuti pola yang
  sama seperti discovery MAC-Telnet di Fase 4.
- Rancang bagaimana RoMON dipakai sebagai **jalur tunneling** menuju sesi
  transport lain (umumnya menuju sesi ala MAC-Telnet di perangkat tujuan
  yang tidak terjangkau langsung) — ini berarti transport RoMON kemungkinan
  besar bukan endpoint tersendiri, tapi lapisan pembawa (carrier) yang
  membungkus salah satu transport lain. Desain ini harus eksplisit sejak
  awal supaya tidak duplikasi logika console-parsing.
- Karena tingkat ketidakpastian spesifikasi tinggi, alokasikan waktu
  khusus untuk pengujian empiris berulang di lab (coba-implementasi-uji
  berulang) sebagai bagian resmi dari fase ini, bukan dianggap "bonus"
  di luar jadwal.

**Kriteria selesai Fase 5:** minimal satu skenario nyata berhasil — RoMON
dipakai untuk menjangkau dan menjalankan command di perangkat yang tidak
punya jalur akses langsung — didemonstrasikan dan didokumentasikan
batasannya secara jujur (versi RouterOS mana yang teruji, kondisi jaringan
apa yang dibutuhkan, apa yang belum didukung).

---

## 8. Fase 6 — Lapisan Validasi Dua-Gate (Universal Lintas Transport)

Fase ini dibangun di atas kontrak transport dari Fase 1, dan harus bekerja
sama untuk semua transport yang sudah selesai sampai titik ini (native-api
minimal, transport lain sesuai yang sudah rampung).

**Yang perlu dikerjakan:**

- Rancang **Gate sintaks**: memanfaatkan kemampuan "jalankan script mentah"
  dari kontrak transport untuk memvalidasi bentuk command sebelum
  dieksekusi. Pastikan hasil validasi ini membedakan dengan jelas antara
  tiga kondisi: command valid, command tidak valid secara sintaks (dengan
  detail lokasi/pesan errornya kalau tersedia), dan kegagalan yang murni
  disebabkan masalah transport/koneksi (bukan masalah command-nya).
- Rancang **Gate semantik**: memanfaatkan `/console/inspect` (kalau
  transport yang dipakai mendukungnya, sesuai deteksi dari Fase 2 dan
  temuan Fase 0 untuk transport lain) untuk memvalidasi bahwa
  path/command yang diminta memang ada, dan setiap parameter yang dikirim
  memang dikenal oleh command tersebut.
- Rancang **perilaku degradasi yang jelas** ketika Gate semantik tidak bisa
  dipakai (transport/versi RouterOS tidak mendukung inspect): pastikan
  Gate sintaks tetap berjalan penuh, dan hasil validasi secara eksplisit
  memberi tahu pemanggil bahwa pengecekan semantik dilewati (jangan diam-
  diam terlihat seperti "sudah tervalidasi penuh" padahal cuma sebagian).
- Rancang aturan tegas: tidak boleh ada jalur di API publik yang
  memungkinkan pemanggil mematikan validasi hanya supaya sebuah command
  "lolos" tanpa pemanggil sadar itu yang mereka lakukan. Kalau memang perlu
  ada opsi lewati-validasi (untuk kasus command yang memang belum
  didukung skema-nya), opsi itu harus eksplisit disebut sebagai
  "melewati validasi" di namanya, bukan tersembunyi di balik parameter
  lain.
- Definisikan dengan jelas urutan eksekusi: command asli baru benar-benar
  dijalankan setelah kedua gate (atau gate yang tersedia) lolos — dan
  pastikan proses validasi ini sendiri tidak pernah mengubah state
  perangkat (murni baca/cek, tanpa efek samping).

**Kriteria selesai Fase 6:** ada satu titik masuk validasi yang bekerja
konsisten di semua transport yang sudah selesai, dengan hasil validasi yang
membedakan jenis kegagalan secara jelas, teruji dengan command valid,
command dengan parameter salah, dan command dengan sintaks rusak — di
setidaknya dua transport berbeda untuk membuktikan konsistensi lintas
transport.

---

## 9. Fase 7 — API Discovery Atribut/Parameter (Return Terstruktur)

Ini titik di mana tujuan "return dari MikroTik tidak lagi data mentah"
direalisasikan — **khusus untuk hasil discovery/validasi**, sesuai batasan
lingkup di §0.

**Yang perlu dikerjakan:**

- Rancang tipe data publik yang merepresentasikan "deskripsi sebuah
  command RouterOS": mencakup apakah path/command itu valid, daftar nama
  parameter input yang dikenal, dan (kalau berhasil ditemukan) daftar nama
  field yang dikembalikan command tersebut. Tipe ini harus jadi struct Go
  bernama dan terdokumentasi field-per-field, bukan map generik.
- Rancang secara eksplisit **perbedaan cara discovery untuk command
  bertipe tabel-data (print/get/add/set) vs command bertipe aksi/
  komputasi** (command yang hasilnya berupa nilai terhitung, bukan field
  tabel yang tersimpan) — dua kategori ini butuh strategi query yang
  berbeda ke `/console/inspect`, dan tipe hasil discovery harus punya cara
  menandai command yang termasuk kategori mana, termasuk kasus di mana
  nama field output genuinely tidak bisa ditemukan tanpa menjalankan
  command-nya (lihat catatan di bawah).
- Rancang secara sadar: untuk command aksi/komputasi yang field output-nya
  tidak bisa ditemukan secara statis, API discovery **tidak boleh diam-diam
  menjalankan command tersebut** untuk menebak field-nya. Kalau kemampuan
  "coba jalankan sekali untuk lihat field aslinya" memang mau disediakan,
  itu harus jadi fungsi terpisah yang secara eksplisit dipanggil sendiri
  oleh pengguna library (karena itu efeknya beda — benar-benar menjalankan
  sesuatu di perangkat, bukan sekadar bertanya).
- Pastikan tipe hasil discovery ini dipakai ulang oleh Gate semantik di
  Fase 6 (bukan dua implementasi terpisah yang kebetulan mirip) — Gate
  semantik pada dasarnya adalah "panggil discovery, lalu bandingkan
  parameter yang diminta dengan hasilnya".
- Sediakan mekanisme cache hasil discovery per command (karena skema
  command RouterOS tidak berubah dalam satu sesi kerja) sebagai bagian
  dari desain API ini, supaya pemanggil berulang tidak perlu round-trip
  jaringan berulang kali untuk command yang sama.

**Kriteria selesai Fase 7:** ada satu fungsi publik yang menerima sebuah
path/command RouterOS dan mengembalikan tipe struct terstruktur berisi
status validitas, daftar parameter input, dan daftar field output (atau
penanda eksplisit "tidak diketahui" untuk command aksi/komputasi) —
diuji minimal untuk satu command bertipe tabel-data dan satu command
bertipe aksi/komputasi, dan hasilnya didokumentasikan sebagai bagian dari
API publik library (bukan detail internal).

---

## 10. Fase 8 — Cross-Cutting: Konsolidasi Error, Context, Logging

Fase ini sebenarnya berjalan paralel sepanjang Fase 2–7, tapi perlu satu
putaran konsolidasi eksplisit sebelum project dianggap siap dipakai orang
lain.

**Yang perlu dikerjakan:**

- Audit ulang semua transport dan lapisan validasi: pastikan taksonomi
  error yang dirancang di Fase 1 benar-benar dipakai konsisten di semua
  tempat, tidak ada transport yang "curang" melempar error generik di
  luar taksonomi yang sudah disepakati.
- Audit ulang penggunaan `context.Context`: pastikan setiap operasi
  jaringan di semua transport benar-benar menghormati cancellation/
  timeout dari context yang diberikan, termasuk operasi yang sifatnya
  broadcast/discovery (MAC-Telnet, RoMON) yang secara alami rawan
  menunggu tanpa batas kalau tidak dijaga.
- Pastikan mekanisme logging (kalau project ini melanjutkan pola logging
  terstruktur yang sudah ada di `go-routeros`) diperluas konsisten ke
  semua transport baru, bukan cuma ada di native-api.
- Review keamanan dasar: pastikan kredensial (password, kunci privat SSH)
  tidak pernah ikut ter-log secara tidak sengaja di jalur logging manapun,
  di semua transport.

**Kriteria selesai Fase 8:** ada checklist audit yang dijalankan terhadap
setiap transport dan lapisan validasi, dengan hasil "sesuai" di semua
poin di atas, dan celah yang ditemukan sudah diperbaiki (bukan sekadar
dicatat sebagai utang teknis).

---

## 11. Fase 9 — Testing & Lab CHR

**Yang perlu dikerjakan:**

- Susun matriks pengujian: kombinasi (transport) × (versi RouterOS target
  dari Fase 0) × (skenario: command valid, parameter salah, path tidak
  ada, koneksi terputus di tengah, autentikasi gagal).
- Untuk transport yang bisa diuji lewat jaringan biasa (native-api, SSH),
  rancang agar pengujian ini bisa berjalan otomatis (mis. terhadap
  instance RouterOS virtual yang bisa dijalankan berulang), supaya
  regresi di masa depan cepat ketahuan.
- Untuk transport berbasis layer-2 (MAC-Telnet, RoMON), akui secara jujur
  bahwa pengujian otomatis penuh kemungkinan besar lebih sulit (butuh
  akses jaringan level tertentu) — rancang minimal prosedur pengujian
  manual yang terdokumentasi dan bisa diulang orang lain, sebagai baseline
  sebelum (kalau memungkinkan) diotomatisasi belakangan.
- Pastikan hasil pengujian Gate validasi (Fase 6) dan Discovery (Fase 7)
  ikut masuk matriks ini, bukan cuma pengujian transport murni.

**Kriteria selesai Fase 9:** matriks pengujian di atas punya hasil
terdokumentasi (lulus/gagal per sel), dan bagian yang otomatis benar-benar
berjalan dan bisa diulang, bukan cuma laporan pengujian manual satu kali.

---

## 12. Fase 10 — Dokumentasi & Rilis

**Yang perlu dikerjakan:**

- Tulis dokumentasi publik yang menjelaskan: cara memilih transport, cara
  memakai lapisan validasi, cara memakai API discovery, dan batasan yang
  jujur per transport (terutama MAC-Telnet/RoMON yang levelnya
  eksperimental kalau memang begitu hasil Fase 4–5).
- Tulis catatan migrasi untuk pengguna `go-routeros` versi sebelumnya
  kalau ada perubahan API yang tidak sepenuhnya backward-compatible
  (sesuai keputusan di Fase 1).
- Tentukan skema versi rilis (mengikuti semantic versioning) dan apakah
  perubahan ini perlu bump versi major karena ada perubahan API besar.
- Siapkan changelog yang memetakan setiap fase di atas ke perubahan yang
  terlihat pengguna.

**Kriteria selesai Fase 10:** dokumentasi publik lengkap tersedia,
keputusan versi rilis final sudah diambil dan diterapkan, dan ada
setidaknya satu rilis resmi yang mencakup seluruh fase 1–9 di atas.

---

## 13. Struktur Modul yang Disarankan (Level Konsep, Bukan Kode)

Pisahkan project menjadi area tanggung jawab yang jelas, supaya tiap fase
di atas punya "rumah" yang jelas dan tidak saling tumpang tindih:

- **Area inti/kontrak** — tempat kontrak transport, tipe data kanonik, dan
  taksonomi error didefinisikan (hasil Fase 1). Area ini tidak boleh
  bergantung ke kode transport spesifik mana pun.
- **Area per-transport** — satu area terpisah untuk masing-masing dari
  native-api, SSH, MAC-Telnet, RoMON (hasil Fase 2–5). Tiap area ini
  bergantung ke area inti, tapi tidak saling bergantung satu sama lain
  — kecuali MAC-Telnet dan RoMON yang secara sadar berbagi parser
  console teks (sesuai arahan Fase 4–5, bukan duplikasi).
- **Area validasi** — lapisan Gate sintaks + semantik (hasil Fase 6),
  bergantung ke area inti dan ke area discovery, tidak bergantung ke
  transport manapun secara langsung.
- **Area discovery** — API deskripsi command terstruktur (hasil Fase 7),
  bergantung ke area inti saja.
- **Area pengujian** — terpisah dari kode produksi, berisi matriks
  pengujian dari Fase 9, termasuk dokumentasi prosedur manual untuk
  transport yang belum bisa diotomatisasi penuh.

---

## 14. Definition of Done — Keseluruhan Project

Project ini dianggap selesai (siap rilis versi pertama) kalau semua berikut
terpenuhi sekaligus:

- Fase 0–2 selesai penuh (riset, fondasi, native-api teradaptasi) —
  ini adalah baseline wajib, tidak bisa dilewati.
- Minimal satu transport baru (disarankan SSH, karena risikonya paling
  rendah) selesai penuh sampai Fase 3, sehingga project ini benar-benar
  membuktikan klaim "multi-transport", bukan cuma native-api ditata ulang.
- Fase 6 dan 7 (validasi dua-gate + discovery terstruktur) selesai dan
  bekerja di minimal dua transport yang sudah rampung — ini inti nilai
  tambah project di luar sekadar "transport tambahan".
- Fase 8–10 (audit cross-cutting, testing, dokumentasi/rilis) selesai
  untuk seluruh scope yang sudah dikerjakan sampai titik rilis tersebut.
- MAC-Telnet dan RoMON **boleh** dirilis belakangan sebagai minor release
  susulan kalau Fase 4–5 ternyata butuh waktu jauh lebih lama dari
  perkiraan (mengingat tingkat risiko keduanya sudah ditandai tinggi sejak
  awal dokumen ini) — ini bukan kegagalan project, tapi keputusan
  prioritas yang wajar berdasarkan Fase 0.

---

## 15. Risiko Utama & Mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| MAC-Telnet/RoMON ternyata jauh lebih rumit dari perkiraan (enkripsi, spesifikasi tidak resmi) | Molor jadwal signifikan kalau dianggap wajib di rilis pertama | Fase 0 wajib memutuskan kelayakan di awal; Fase 15 §14 secara eksplisit mengizinkan keduanya dirilis belakangan |
| Refactor Fase 1 tidak sengaja merusak perilaku native-api yang sudah dipakai orang | Regresi yang merugikan pengguna existing | Test regresi wajib di Fase 2 sebelum lanjut fase manapun setelahnya |
| `/console/inspect` ternyata tidak konsisten perilakunya lintas versi RouterOS 7.x | Gate semantik memberi hasil salah/tidak bisa diandalkan | Fase 0 dan Fase 9 mewajibkan pengujian di lebih dari satu versi RouterOS, bukan cuma satu versi acuan |
| Scope creep ke arah auto-generate struct (yang eksplisit di luar lingkup §0) | Project tidak pernah selesai karena terus melebar | Tegaskan ulang batasan lingkup ini di setiap tinjauan progres fase |