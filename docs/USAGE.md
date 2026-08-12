# Panduan Penggunaan Lengkap — goros v4

Dokumen ini adalah panduan penggunaan **dan** laporan hasil uji terhadap device
RouterOS asli. Semua contoh di bawah sudah **diverifikasi bekerja** di lab
nyata (read-only, tanpa memakai RoMON/Winbox):

| Device | Identitas | Versi | Alamat | API `:8728` | API-SSL `:8729` | SSH `:22` |
|---|---|---|---|---|---|---|
| MT-1 (v6) | `quixiq` | 6.49.x | `192.168.233.1` | ✅ | ⚠️ no cert | ✅ |
| MT-2 (v7) | `mikrotik-sim` | 7.21.x | `192.168.230.3` | ✅ | ⚠️ no cert | ✅ |

> ⚠️ API-SSL: port 8729 terbuka, tapi di kedua device
> `/ip service set api-ssl certificate=...` bernilai **`none`** sehingga RouterOS
> menolak handshake TLS. Ini konfigurasi device, bukan bug library. Lihat §9.

---

## 1. Instalasi

```bash
go get github.com/quiqxiq/goros/v4@latest
```

Requires **Go 1.22 or newer**.

## 2. Koneksi dasar (native API)

```go
import routeros "github.com/quiqxiq/goros/v4"

// Dial — block sampai login berhasil atau gagal.
c, err := routeros.Dial("192.168.88.1:8728", "admin", "password")
if err != nil {
    log.Fatal(err)
}
defer c.Close()
```

Varian dengan kontrol waktu dan konteks:

| Fungsi | Kegunaan |
|---|---|
| `Dial(addr, user, pass)` | Tanpa batas waktu |
| `DialTimeout(addr, user, pass, d)` | Batas waktu total (konek + login) |
| `DialContext(ctx, addr, user, pass)` | Batalkan lewat `context` |
| `DialTLS*` | API-SSL (port 8729) — lihat §9 |
| `NewClient(rwc)` + `Login()` | Koneksi manual di atas `io.ReadWriteCloser` apa pun |

`Client.Queue` mengatur buffer saluran listener (default `0`; contoh memakai
`100`). Setel **sebelum** `Listen*()`.

## 3. Menjalankan perintah satu kali — `Run`

```go
r, err := c.Run("/system/identity/print")
// atau bentuk slice (setiap elemen = satu kata sentence):
r, err = c.RunArgs([]string{"/system/identity/print"})
// atau dengan context:
r, err = c.RunContext(ctx, "/ip/address/print", "?disabled=false")
```

- `r.Re` — slice `*proto.Sentence` untuk setiap baris `!re` (satu per baris
  tabel). `r.Re[i].Map["name"]` mengambil satu kolom.
- `r.Done` — sentence `!done`.
- Error dikembalikan bila device menjawab `!trap`/`!fatal` (lihat §10).

```go
// Contoh nyata (output terverifikasi):
r, _ := c.Run("/system/identity/print")
fmt.Println("identity =", r.Re[0].Map["name"]) // v6: quixiq, v7: mikrotik-sim
```

Contoh lengkap: [`examples/run`](../examples/run/main.go) —
jalankan:

```bash
go run ./examples/run -address 192.168.233.1:8728 -username admin -password <password> \
  -command '/system/identity/print'
```

## 4. Streaming / listener — `Listen`

Dipakai untuk perintah yang menghasilkan aliran data terus-menerus, misalnya
`monitor-traffic`, `/log print follow`, atau `:listen`.

```go
l, err := c.ListenContext(ctx, "/interface/monitor-traffic", "=interface=ether1")
if err != nil {
    log.Fatal(err)
}

for sen := range l.Chan() {
    log.Info("Update", slog.String("sentence", sen.String()))
}
if err := l.Err(); err != nil {
    log.Fatal(err)
}
```

Varian:

| Fungsi | Kegunaan |
|---|---|
| `Listen(sentence ...string)` | Auto-async, tanpa context |
| `ListenContext(ctx, ...)` | Batalkan listener lewat context |
| `ListenArgs(sentence []string)` | Bentuk slice kata |
| `ListenArgsContext(ctx, sentence []string)` | Slice + context |
| `ListenArgsQueue(ctx, sentence, n)` | Kontrol ukuran buffer saluran |

Menghentikan listener (dua cara, keduanya setara):

```go
// 1) Batalkan context si listener → client otomatis kirim /cancel =tag=...
cancel() // ctx yang dipakai ListenContext

// 2) Manual
l.Cancel()        // kirim /cancel =tag=... ke device
l.CancelContext(ctx)
```

Saluran `l.Chan()` **tertutup** begitu `!done`/`!trap`/`!fatal` diterima atau
client ditutup. `l.Err()` berisi error pertama (bila ada).

Contoh lengkap: [`examples/listen`](../examples/listen/main.go) — mode
**implicit async** (panggil `Listen*`, async dimulai otomatis) dan **explicit
async** (`c.Async()` dulu, hasil stream saluran `<-chan error` tersedia).
Keduanya terverifikasi menerima update per detik dari `monitor-traffic` di v6
dan v7:

```bash
# implicit async (v6)
go run ./examples/listen -address 192.168.233.1:8728 -username admin -password <password> \
  -timeout 3s -command '/interface/monitor-traffic =interface=ether1'
# explicit async (v7)
go run ./examples/listen -address 192.168.230.3:8728 -username admin -password <password> \
  -timeout 3s -async -command '/interface/monitor-traffic =interface=ether1'
```

## 5. Polling tabel — pola `tab`

Cocok untuk dashboard: loop `Run` dengan `?query` dan `=.proplist` lalu cetak
kolom per detik.

```bash
go run ./examples/tab -address 192.168.233.1:8728 -username admin -password <password> \
  -properties 'name,rx-byte,tx-byte,rx-packet,tx-packet' -interval 1s
```

Contoh lengkap: [`examples/tab`](../examples/tab/main.go).

## 6. Banyak listener dalam SATU koneksi — perilaku yang diverifikasi

Pertanyaan kunci: *"apakah ketika satu listen di-close, yang lain ikut mati?"*

**Jawaban (terverifikasi di v6 & v7): TIDAK.** Setiap listener memakai tag
unik (`l1`, `l2`, …) dan di-cancel sendiri-sendiri; cancel satu listener hanya
mengirim `/cancel =tag=<tagnya>` ke device, stream lain tidak tersentuh.

Output nyata `examples/multi_listen` (identik di v6 dan v7):

```
=== 2 streams started. Stream1 will be cancelled in 5s ===
[stream1] received sentence #1   [stream2] received sentence #1
[stream1] received sentence #5   [stream2] received sentence #5
>>> stream1 context CANCELLED (5s timeout) <<<
[stream1] ENDED after 5 sentences (err=<nil>)
[stream2] received sentence #6   [stream2] received sentence #13
=== SUCCESS: stream2 survived 8s after stream1 was cancelled! ===
[stream2] ENDED after 13 sentences (err=<nil>)
```

Perilaku yang dijamin:

| Skenario | Hasil terverifikasi |
|---|---|
| `Cancel()`/context satu listener | Listener lain **tetap jalan** |
| Semua listener pada satu koneksi | Berbagi satu koneksi & async loop |
| `Client.Close()` | **Semua** listener di koneksi itu tertutup (satu koneksi = satu sesi) |
| Koneksi berbeda | Sepenuhnya independen (§7) |

Test otomasinya: `TestLabMultiListenCtxCancelIsolation` dan
`TestLabMultiListenManualCancelIsolation` di
[`multi_lab_test.go`](../multi_lab_test.go) (tag `integration`).

Contoh lengkap: [`examples/multi_listen`](../examples/multi_listen/main.go).

```bash
go run ./examples/multi_listen -address 192.168.230.3:8728 -username admin -password <password>
```

## 7. Banyak koneksi (multi-connection)

Setiap `Dial*` adalah koneksi TCP + sesi login sendiri. Koneksi tidak saling
berbagi state apa pun. Terverifikasi:

| Skenario | Hasil |
|---|---|
| 5 koneksi paralel ke device sama, `Run` bersamaan | Semua sukses |
| Tutup 1 dari 3 koneksi | 2 lainnya **tetap hidup** |
| Koneksi ke v6 & v7 secara paralel dari satu proses | Keduanya sukses |

Test: `TestLabMultiConnectionConcurrentRun`, `TestLabCloseOneConnectionOthersAlive`,
`TestLabCrossDeviceConcurrent` di `multi_lab_test.go`.

```go
// Boleh buka banyak client sekaligus — tidak ada global state.
c1, _ := routeros.Dial("192.168.233.1:8728", "admin", "pw")
c2, _ := routeros.Dial("192.168.230.3:8728", "admin", "pw")
defer c1.Close()
defer c2.Close()

var wg sync.WaitGroup
for _, c := range []*routeros.Client{c1, c2} {
    wg.Add(1)
    go func(c *routeros.Client) {
        defer wg.Done()
        r, err := c.Run("/system/resource/print")
        if err == nil && len(r.Re) == 1 {
            fmt.Println("ok:", r.Re[0].Map["uptime"])
        }
    }(c)
}
wg.Wait()
```

## 8. Validasi & eksekusi terstruktur (Fase 6–7)

Tiga method modern di atas `Client` (dokumen desain: `docs/DESIGN.md` §4.7):

```go
import (
    routeros "github.com/quixiq/goros/v4"
    "github.com/quixiq/goros/v4/transport"
)

c, _ := routeros.DialContext(ctx, "192.168.88.1:8728", "admin", "pw")
defer c.Close()

// 1) Validate — dry-run: gate yang applicable dijalankan, command TIDAK dieksekusi.
if err := c.Validate(ctx, &transport.Command{
    Path: "/ip/address", Verb: "add",
    Attributes: map[string]string{"address": "10.0.0.1/24", "interface": "ether1"},
}); err != nil {
    return err
}

// 2) Inspect — discovery murni skema command (butuh v7 / RouterOS 7.18+).
schema, err := c.Inspect(ctx, "/ip/address", "print")
if err != nil {
    return err
}

// 3) RunStructured — validasi lalu eksekusi sebagai sentence-nya sendiri.
rep, err := c.RunStructured(ctx, &transport.Command{Path: "/system/resource", Verb: "print"})
if err != nil {
    return err
}
```

Catatan perangkat keras yang terverifikasi:

- Gate 2 (inspect, `validation/unknown-attribute`) berjalan di **v7**; di **v6**
  tidak didukung dan otomatis di-skip (desain D-015).
- Gate 1 (syntax) di native-api hanya jalan di v7; di v6 via SSH tetap viable
  (temuan R12 — bentuk spasi).
- Error selalu `roserr` terstruktur: `roserr.IsCode(err, roserr.CodeValidationSyntax)`.

## 9. API-SSL (TLS) — DialTLS

```go
c, err := routeros.DialTLS("192.168.230.3:8729", "admin", "password",
    &tls.Config{ServerName: "mikrotik-sim"}) // atau InsecureSkipVerify untuk lab
```

Temuan lab: RouterOS butuh sertifikat terpasang sebelum API-SSL menerima
koneksi. Kalau `/ip service print` menunjukkan `api-ssl ... certificate=none`
(mis. `none`), handshake gagal dengan `remote error: tls: handshake failure`
**bahkan** dengan `InsecureSkipVerify: true` — bukan bug library. Aktifkan
dengan:

```bash
# di RouterOS (Winbox/SSH), bukan dari Go:
/certificate add name=api-ssl-crt common-name=mikrotik-sim days-valid=3650 key-size=2048
/certificate sign api-ssl-crt
/ip service set api-ssl certificate=api-ssl-crt
```

## 10. Penanganan error

- Error dari device (`!trap`/`!fatal`) dibungkus `*routeros.DeviceError`
  (gunakan `errors.As`). Contoh: login salah →
  `cannot log in`, duplikat entry → `entry already exists`.
- `TestInvalidLogin` & `TestTrapHandling` (client_test.go) memverifikasi pola
  ini di device nyata; `TestTrapHandling` menambahkan entry DNS sementara lalu
  **menghapusnya sendiri** (auto-cleanup).
- Semua error transport/validasi terstruktur via `roserr` (§8).

## 11. Transport kedua — SSH console (`transport/ssh`)

Menjalankan command console RouterOS via SSH, tanpa PTY. Untuk **v6 wajib
bentuk spasi** (`/ip address print`); v7 menerima bentuk slash juga. Goros
menangani ini otomatis via `ConsoleCLI()` (D-016/R12).

```go
import sshtransport "github.com/quixiq/goros/v4/transport/ssh"

c, err := sshtransport.Dial(ctx, "192.168.233.1:22", "admin",
    sshtransport.WithPassword("password"),
    sshtransport.WithTimeout(10*time.Second),
)
defer c.Close()

out, err := c.Run(ctx, "/system identity print")
if err := c.Validate(ctx, "/ip address print bogus=1"); err != nil {
    // roserr.CodeValidationSyntax / CodeValidationUnknownAttribute
}
```

Fitur: TOFU host-key (`WithHostKey*`), insecure mode lab, error mapping 6 kode
→ `roserr`, output dibersihkan dari prompt/ANSI. Terverifikasi di v6 & v7
(`transport/ssh/ssh_lab_test.go`, M22–M29), termasuk kesetaraan kode error
dengan native-api di v7.

## 12. Logging

`Client` memakai `log/slog`. Ganti handler:

```go
c.SetLogHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
```

Contoh `examples/run` dan `examples/listen` punya flag `-debug`.

---

## Lampiran A — Hasil uji lengkap (lab nyata, Agustus 2026)

Semua perintah di bawah read-only kecuali `TestTrapHandling` (auto-cleanup) dan
`TestLabRunStructuredExecutesAndCleansUp` (tambah + hapus alamat sementara).

### A.1 Unit test (tanpa device)

```bash
go test -race -count=1 -timeout 240s ./...
# 12 paket PASS: root, gate, proto, roserr, schema, transport, mock, nativeapi, ssh
```

### A.2 Integration (tag `integration`) — v6 `192.168.233.1`

| Grup | Hasil |
|---|---|
| client_test (RunSync/Async/Error, Dial*, InvalidLogin, TrapHandling) | 9 PASS |
| multi_lab (isolasi listener ×2, multi-koneksi ×3) | 5 PASS |
| orchestrate_lab (Fase 6 DoD) | 4 PASS, 1 SKIP (Gate2 — v6 tanpa inspect) |
| gate + nativeapi | PASS |
| transport/ssh (M22–M29) | PASS (22 s) |

### A.3 Integration — v7 `192.168.230.3`

Semua grup di atas **PASS**, termasuk Gate2 (v7 mendukung inspect). SSH PASS.

### A.4 Contoh (kedua device)

| Contoh | Hasil |
|---|---|
| `examples/run` | identity terverifikasi (`quixiq`, `mikrotik-sim`) |
| `examples/listen` (implicit & explicit async) | update per detik diterima |
| `examples/tab` | tabel dicetak per interval |
| `examples/multi_listen` | **stream2 selamat 8 s setelah stream1 di-cancel** |
| TLS 8729 | handshake ditolak device (`certificate=none`) — bukan bug library |

### A.5 Cara menjalankan ulang

```bash
# v6 sebagai device utama
ROUTEROS_TEST_ADDRESS=192.168.233.1:8728 ROUTEROS_TEST_USERNAME=admin \
ROUTEROS_TEST_PASSWORD=... ROUTEROS_TEST_ADDRESS_2=192.168.230.3:8728 \
ROUTEROS_TEST_USERNAME_2=admin ROUTEROS_TEST_PASSWORD_2=... \
go test -tags integration -count=1 -timeout 300s ./...

# SSH
ROUTEROS_TEST_SSH_ADDRESS=...:22 ROUTEROS_TEST_SSH_USERNAME=admin \
ROUTEROS_TEST_SSH_PASSWORD=... go test -tags integration ./transport/ssh/
```

Kredensial hanya lewat env var; tidak ada kredensial di repo.
