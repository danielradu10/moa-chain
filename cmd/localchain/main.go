// localchain runs a self-contained MoA Chain simulator.
// It starts N nodes with local mocks or configured real HTTP agents, exposes
// an explorer HTTP API on node 0, and advances rounds until interrupted.
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

	"moa-chain/agent"
	"moa-chain/agent/httpclient"
	"moa-chain/experiment"
	"moa-chain/explorer/controllers"
	"moa-chain/explorer/service"
	"moa-chain/localchain"
	"moa-chain/validators"
)

func main() {
	numNodes := flag.Int("nodes", 10, "number of validator nodes")
	startRound := flag.Uint64("start-round", 2, "round number to start from (genesis is round 1)")
	addr := flag.String("addr", ":8080", "explorer HTTP server address")
	agentDelay := flag.Duration("agent-delay", 5*time.Second, "simulated LLM latency per agent call")
	miniRoundDuration := flag.Duration("mini-round-duration", 15*time.Second, "fixed slot per mini-round; 0 = advance immediately")
	mr1VoteDeadline := flag.Duration("mr1-vote-collection-deadline", 10*time.Second, "MR1 fallback deadline for collecting label votes; 0 = wait for all committee members")
	mr2ExecutionDeadline := flag.Duration("mr2-execution-results-deadline", 10*time.Second, "MR2 fallback deadline for collecting execution results; 0 = wait for all committee members")
	mr2GracePeriod := flag.Duration("mr2-classification-grace-period", 10*time.Second, "MR2 post-quorum grace period for collecting classification votes; 0 = certify immediately at quorum")
	mr3GracePeriod := flag.Duration("mr3-approval-grace-period", 10*time.Second, "MR3 post-quorum grace period for collecting approval votes; 0 = certify immediately at quorum")
	agentConfigPath := flag.String("agent-config", "", "experiment config providing real agent endpoints and validator names; empty uses local mocks")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	configuredNodes := *numNodes
	var configuredAgents []agent.BatchAgent
	var configuredValidatorIDs []string
	committeeStrategy := validators.CommitteeStrategyHalf
	if *agentConfigPath != "" {
		agentCfg, err := experiment.LoadConfig(*agentConfigPath)
		if err != nil {
			log.Fatalf("localchain: load agent config: %v", err)
		}
		configuredNodes = len(agentCfg.Validators)
		configuredAgents = make([]agent.BatchAgent, configuredNodes)
		configuredValidatorIDs = make([]string, configuredNodes)
		for i, validator := range agentCfg.Validators {
			configuredValidatorIDs[i] = validator.ValidatorName
			configuredAgents[i] = httpclient.New(httpclient.Config{
				BaseURL:                        validator.AgentEndpoint,
				TimeoutSeconds:                 120,
				ValidatorName:                  validator.ValidatorName,
				LabelPromptVersion:             "labeler_v3",
				AnswerPromptVersion:            "answerer_v1",
				SynthesizePromptVersion:        "synthesizer_v1",
				EvaluateSynthesisPromptVersion: "synthesis_evaluator_v1",
			})
		}
		if agentCfg.CommitteeStrategy == "full" {
			committeeStrategy = validators.CommitteeStrategyFull
		}
	}

	logDir := filepath.Join("logs", "localchain", time.Now().Format("20060102T150405"))
	fmt.Fprintf(os.Stderr, "localchain: starting %d nodes, round %d, server %s, agent-delay %s, mini-round-duration %s, mr1-vote-deadline %s, mr2-execution-deadline %s, mr2-grace-period %s, mr3-grace-period %s, agent-config %q\n",
		configuredNodes, *startRound, *addr, *agentDelay, *miniRoundDuration, *mr1VoteDeadline, *mr2ExecutionDeadline, *mr2GracePeriod, *mr3GracePeriod, *agentConfigPath)
	fmt.Fprintf(os.Stderr, "localchain: node logs → %s/\n", logDir)

	lc, err := localchain.New(localchain.Config{
		NumNodes:                           configuredNodes,
		StartRound:                         *startRound,
		AgentDelay:                         *agentDelay,
		MiniRoundDuration:                  *miniRoundDuration,
		VoteCollectionDeadline:             *mr1VoteDeadline,
		ExecutionResultsCollectionDeadline: *mr2ExecutionDeadline,
		ClassificationGracePeriod:          *mr2GracePeriod,
		SynthesisApprovalGracePeriod:       *mr3GracePeriod,
		LogDir:                             logDir,
		Logger:                             logger,
		CommitteeStrategy:                  committeeStrategy,
		Agents:                             configuredAgents,
		ValidatorIDs:                       configuredValidatorIDs,
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
