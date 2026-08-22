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
	"path/filepath"
	"syscall"
	"time"

	"moa-chain/explorer/controllers"
	"moa-chain/explorer/service"
	"moa-chain/localchain"
)

func main() {
	numNodes          := flag.Int("nodes", 10, "number of validator nodes")
	startRound        := flag.Uint64("start-round", 2, "round number to start from (genesis is round 1)")
	addr              := flag.String("addr", ":8080", "explorer HTTP server address")
	agentDelay        := flag.Duration("agent-delay", 5*time.Second, "simulated LLM latency per agent call")
	miniRoundDuration := flag.Duration("mini-round-duration", 6*time.Second, "fixed slot per mini-round; 0 = advance immediately")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	logDir := filepath.Join("logs", "localchain", time.Now().Format("20060102T150405"))
	fmt.Fprintf(os.Stderr, "localchain: starting %d nodes, round %d, server %s, agent-delay %s, mini-round-duration %s\n",
		*numNodes, *startRound, *addr, *agentDelay, *miniRoundDuration)
	fmt.Fprintf(os.Stderr, "localchain: node logs → %s/\n", logDir)

	lc, err := localchain.New(localchain.Config{
		NumNodes:          *numNodes,
		StartRound:        *startRound,
		AgentDelay:        *agentDelay,
		MiniRoundDuration: *miniRoundDuration,
		LogDir:            logDir,
		Logger:            logger,
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
	lc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil && err != http.ErrServerClosed {
		logger.Error("localchain: server shutdown error", "error", err)
	}

	fmt.Fprintln(os.Stderr, "localchain: stopped")
}
