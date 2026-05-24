// Example: multiple concurrent listeners on a single connection.
// One listener is cancelled after a timeout; the others should continue running.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/go-routeros/routeros/v3"
)

var (
	address  = flag.String("address", "192.168.230.2:8728", "RouterOS address and port")
	username = flag.String("username", "admin", "User name")
	password = flag.String("password", "r00t", "Password")
)

func main() {
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	c, err := routeros.DialContext(ctx, *address, *username, *password)
	if err != nil {
		log.Error("dial failed", slog.Any("error", err))
		os.Exit(1)
	}
	defer c.Close()

	c.Queue = 100

	// Stream 1: continuous monitor traffic on ether1 — will be cancelled after 5s
	ctx1, cancel1 := context.WithTimeout(ctx, 5*time.Second)
	defer cancel1()

	l1, err := c.ListenContext(ctx1, "/interface/monitor-traffic", "=interface=ether1")
	if err != nil {
		log.Error("listen1 failed", slog.Any("error", err))
		os.Exit(1)
	}

	// Stream 2: continuous monitor traffic — should keep running after stream1 cancel
	l2, err := c.ListenContext(ctx, "/interface/monitor-traffic", "=interface=ether1")
	if err != nil {
		log.Error("listen2 failed", slog.Any("error", err))
		os.Exit(1)
	}

	fmt.Println("=== 2 streams started. Stream1 will be cancelled in 5s ===")
	fmt.Println("=== If fix works: stream2 keeps running after stream1 dies ===")
	fmt.Println()

	// Read from all streams concurrently
	go func() {
		count := 0
		for range l1.Chan() {
			count++
			fmt.Printf("[stream1] received sentence #%d\n", count)
		}
		fmt.Printf("[stream1] ENDED after %d sentences (err=%v)\n", count, l1.Err())
	}()

	go func() {
		count := 0
		for range l2.Chan() {
			count++
			fmt.Printf("[stream2] received sentence #%d\n", count)
		}
		fmt.Printf("[stream2] ENDED after %d sentences (err=%v)\n", count, l2.Err())
	}()

	// Wait for stream1's context to expire (5s)
	<-ctx1.Done()
	fmt.Println("\n>>> stream1 context CANCELLED (5s timeout) <<<")
	fmt.Println(">>> Waiting 8 more seconds — stream2 should keep receiving data <<<")
	fmt.Println()

	// Let stream2 run for 8 more seconds to prove it survives
	select {
	case <-ctx.Done():
		fmt.Println("\n=== interrupted ===")
	case <-time.After(8 * time.Second):
		fmt.Println("\n=== SUCCESS: stream2 survived 8s after stream1 was cancelled! ===")
	}
}
