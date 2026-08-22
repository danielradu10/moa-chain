// localchain runs a self-contained MoA Chain simulator.
// It starts N nodes with a mocked LLM agent, exposes an explorer HTTP API on
// node 0, and advances rounds continuously until interrupted.
//
// Usage:
//
//	go run ./cmd/localchain --nodes 10 --start-round 2 --addr :8080
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"moa-chain/explorer/controllers"
	"moa-chain/explorer/service"
	"moa-chain/localchain"
)

func main() {
	numNodes   := flag.Int("nodes", 10, "number of validator nodes")
	startRound := flag.Uint64("start-round", 2, "round number to start from (genesis is round 1)")
	addr       := flag.String("addr", ":8080", "explorer HTTP server address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	fmt.Fprintf(os.Stderr, "localchain: starting %d nodes, round %d, server %s\n",
		*numNodes, *startRound, *addr)

	lc, err := localchain.New(localchain.Config{
		NumNodes:   *numNodes,
		StartRound: *startRound,
		Logger:     logger,
	})
	if err != nil {
		log.Fatalf("localchain: setup failed: %v", err)
	}

	svc := service.NewExplorerService(lc.NodeView)
	srv := controllers.NewServer(svc, *addr)
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("localchain: server error: %v", err)
		}
	}()

	lc.Start()

	logger.Info("localchain: running — press Ctrl+C to stop")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintln(os.Stderr, "\nlocalchain: shutting down…")

	lc.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil && err != http.ErrServerClosed {
		logger.Error("localchain: server shutdown error", "error", err)
	}

	fmt.Fprintln(os.Stderr, "localchain: stopped")
}
