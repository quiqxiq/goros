# RouterOS Client for the Go language

Go library for accessing Mikrotik devices using the RouterOS API.

Look in the examples directory to learn how to use this library:
[run](examples/run/main.go),
[listen](examples/listen/main.go),
[tab](examples/tab/main.go),
[multi_listen](examples/multi_listen/main.go).

Panduan penggunaan lengkap (cara pakai + contoh terverifikasi di device
RouterOS v6 & v7 asli, termasuk multi-connection dan perilaku banyak
listener): [docs/USAGE.md](docs/USAGE.md).

API documentation is available at [pkg.go.dev](https://pkg.go.dev/github.com/quiqxiq/goros/v4).  
Page on the [Mikrotik Wiki](http://wiki.mikrotik.com/wiki/API_in_Go).

Usage of `gopkg.in` was removed in favor of Go modules. Please, update your import paths to
`github.com/quiqxiq/goros/v4`.

To install it, run:
`go get github.com/quiqxiq/goros/v4`

Requires **Go 1.22 or newer**.

## Fork

This repository is a fork of
[go-routeros/routeros](https://github.com/go-routeros/routeros), maintained at
`github.com/quiqxiq/goros`. It follows the `v4` module line — the redesigned
API (multi-transport, two validation gates, structured returns) lives there
(see `docs/DECISIONS.md` D-001, D-003 → D-014). Old released versions of the
upstream project:
[**v2**](https://github.com/go-routeros/routeros/tree/v2),
[**v1**](https://github.com/go-routeros/routeros/tree/v1)

## Validasi & eksekusi terstruktur

Selain API legacy (`Run`, `RunContext`, `Listen`, …), `Client` menyediakan tiga
method untuk validasi & eksekusi terstruktur (Fase 6–7, `docs/DESIGN.md` §4.7):

```go
import (
    "context"
    routeros "github.com/quiqxiq/goros/v4"
    "github.com/quiqxiq/goros/v4/transport"
)

func example(ctx context.Context) error {
    c, err := routeros.DialContext(ctx, "192.168.88.1:8728", "admin", "password")
    if err != nil {
        return err
    }
    defer c.Close()

    // 1. Validate — dry-run: gate yang applicable dijalankan, command TIDAK
    //    pernah dieksekusi (aman dipanggil berulang, termasuk command action).
    if err := c.Validate(ctx, &transport.Command{
        Path: "/ip/address", Verb: "add",
        Attributes: map[string]string{"address": "10.0.0.1/24", "interface": "ether1"},
    }); err != nil {
        return err
    }

    // 2. Inspect — discovery murni CommandSchema (path + verb).
    schema, err := c.Inspect(ctx, "/ip/address", "print")
    if err != nil {
        return err
    }
    _ = schema // CommandSchema{Category: table, Attributes: [...]}

    // 3. RunStructured — validasi lalu eksekusi command sebagai sentence-nya
    //    sendiri (bukan dibungkus /execute).
    rep, err := c.RunStructured(ctx, &transport.Command{
        Path: "/system/resource", Verb: "print",
    })
    if err != nil {
        return err
    }
    _ = rep
    return nil
}
```

### Transport kedua: SSH console

`transport/ssh` menjalankan command console RouterOS lewat SSH (tanpa PTY,
output bersih — RouterOS tidak memberi pseudo-tty). Gate validasi dipakai
ulang dari jalur native-api (classifier yang sama, tidak disalin):

```go
import (
    "context"
    "time"

    sshtransport "github.com/quiqxiq/goros/v4/transport/ssh"
)

func sshExample(ctx context.Context) error {
    c, err := sshtransport.Dial(ctx, "192.168.88.1:22", "admin",
        sshtransport.WithPassword("password"),
        sshtransport.WithTimeout(10*time.Second),
    )
    if err != nil {
        return err
    }
    defer c.Close()

    out, err := c.Run(ctx, "/system identity print")
    if err != nil {
        return err
    }
    _ = out

    // Gate 1 console: validasi syntax command sebelum dieksekusi.
    if err := c.Validate(ctx, "/ip address print bogus=1"); err != nil {
        return err
    }
    return nil
}
```

Kredensial device lab hanya lewat env var (`ROUTEROS_TEST_*`); tidak ada
kredensial di repo. Error selalu `roserr` terstruktur (`roserr.IsCode`),
termasuk taksonomi lintas transport (`auth/failed`, `transport/timeout`,
`transport/host-key-mismatch`, `validation/syntax`, `validation/unknown-attribute`,
…).
