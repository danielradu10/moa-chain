// experiment runs one heterogeneous real-agent full-round test.
//
// Flow:
//
//	submit tx → async preprocessing (LabelBatch + AnswerBatch) → mempool
//	→ MR1 → MR2 → MR3 → finalization
//
// The transaction may enter the mempool after several empty rounds have
// passed; that is expected and not treated as a failure.
//
// Usage:
//
//	go run ./cmd/experiment \
//	  --config  configs/experiment-heterogeneous.json \
//	  --out     agent-python/testresults/real-agents/<run_id>
//	  --timeout 20m
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"moa-chain/agent"
	"moa-chain/agent/httpclient"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/data"
	"moa-chain/experiment"
	"moa-chain/explorer"
	"moa-chain/localchain"
	"moa-chain/tracker"
	"moa-chain/validators"
)

func main() {
	configPath := flag.String("config", "configs/experiment-heterogeneous.json", "path to experiment config JSON")
	outDir := flag.String("out", "", "output directory; default: agent-python/testresults/real-agents/<run_id>")
	timeout := flag.Duration("timeout", 20*time.Minute, "maximum time to wait for MR3 finalization")
	flag.Parse()

	runID := experiment.GenerateRunID()
	startTime := time.Now()

	cfg, err := experiment.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	dir := *outDir
	if dir == "" {
		dir = filepath.Join("agent-python", "testresults", "real-agents", runID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	manifest := experiment.BuildManifest(runID, cfg, startTime)
	if err := experiment.WriteManifest(dir, manifest); err != nil {
		log.Fatalf("write manifest: %v", err)
	}

	tl, err := experiment.NewTimeline(dir, runID)
	if err != nil {
		log.Fatalf("open timeline: %v", err)
	}
	defer tl.Close()

	rr, err := experiment.NewRoundRecorder(dir, runID)
	if err != nil {
		log.Fatalf("create round recorder: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	tl.Emit(experiment.Event{EventType: "health_check_start"})
	if err := checkAllAgents(cfg.Validators, 10*time.Second); err != nil {
		log.Fatalf("agent health check failed: %v", err)
	}
	tl.Emit(experiment.Event{EventType: "health_check_done"})
	slog.Info("all agents healthy")

	agents := make([]agent.BatchAgent, len(cfg.Validators))
	for i, v := range cfg.Validators {
		agents[i] = httpclient.New(httpclient.Config{
			BaseURL:                        v.AgentEndpoint,
			TimeoutSeconds:                 120,
			RunID:                          runID,
			ValidatorName:                  v.ValidatorName,
			LabelPromptVersion:             "labeler_v3",
			AnswerPromptVersion:            "answerer_v1",
			SynthesizePromptVersion:        "synthesizer_v1",
			EvaluateSynthesisPromptVersion: "synthesis_evaluator_v1",
		})
	}

	committeeStrategy := validators.CommitteeStrategyHalf
	if cfg.CommitteeStrategy == "full" {
		committeeStrategy = validators.CommitteeStrategyFull
	}

	// Shared state updated by hook goroutines; always accessed under mu.
	var (
		mu sync.Mutex

		submittedTxHash string // hex; set after SubmitTransaction
		submittedAt     time.Time

		mr1FinTimes = make(map[uint64]time.Time) // round → MR1 finalization time
		mr2FinTimes = make(map[uint64]time.Time) // round → MR2 finalization time

		emptyMR1Rounds int // MR1 rounds without our tx before selection
		selectedRound  uint64
		selectedEpoch  uint64
		txSelected     bool
	)

	type doneResult struct {
		key   data.RoundKey
		block *data.BlockOnChain
	}
	txDone := make(chan doneResult, 1)

	lchain, err := localchain.New(localchain.Config{
		NumNodes:                  len(cfg.Validators),
		StartRound:                cfg.StartRound,
		MiniRoundDuration:         cfg.MiniRoundDuration,
		VoteCollectionDeadline:    cfg.VoteCollectionDeadline,
		ClassificationGracePeriod: cfg.ClassificationGracePeriod,
		CommitteeStrategy:         committeeStrategy,
		Agents:                    agents,
		Logger:                    logger,
		ExtraHooks: blockFinalizer.BlockFinalizerHooks{
			OnMR1Finalized: func(key data.RoundKey, block *data.BlockOnChain) {
				now := time.Now()
				mu.Lock()
				mr1FinTimes[key.Round] = now
				localHash := submittedTxHash
				if !txSelected && localHash != "" {
					if containsTx(block, localHash) {
						txSelected = true
						selectedRound = key.Round
						selectedEpoch = key.Epoch
					} else {
						emptyMR1Rounds++
					}
				}
				mu.Unlock()

				txHashes := hexTxHashes(block)
				if err2 := rr.WriteMR1(key.Epoch, key.Round, block, now, now, txHashes); err2 != nil {
					slog.Error("WriteMR1", "err", err2)
				}
				tl.Emit(experiment.Event{
					EventType: "mr1_finalized",
					Epoch:     key.Epoch,
					Round:     key.Round,
					Details:   map[string]any{"tx_count": len(block.Body.Transactions)},
				})
			},
			OnMR2Finalized: func(key data.RoundKey, block *data.BlockOnChain) {
				now := time.Now()
				mu.Lock()
				mr1Fin := mr1FinTimes[key.Round]
				mr2FinTimes[key.Round] = now
				mu.Unlock()

				txHashes := hexTxHashes(block)
				if err2 := rr.WriteMR2(key.Epoch, key.Round, block, mr1Fin, mr1Fin, mr1Fin, now, txHashes); err2 != nil {
					slog.Error("WriteMR2", "err", err2)
				}
				tl.Emit(experiment.Event{
					EventType: "mr2_finalized",
					Epoch:     key.Epoch,
					Round:     key.Round,
					Details:   map[string]any{"tx_count": len(block.Body.Transactions)},
				})
			},
			OnMR3Finalized: func(key data.RoundKey, block *data.BlockOnChain) {
				now := time.Now()
				mu.Lock()
				mr2Fin := mr2FinTimes[key.Round]
				localHash := submittedTxHash
				mu.Unlock()

				if err2 := rr.WriteMR3(key.Epoch, key.Round, block, mr2Fin, now); err2 != nil {
					slog.Error("WriteMR3", "err", err2)
				}
				tl.Emit(experiment.Event{
					EventType: "mr3_finalized",
					Epoch:     key.Epoch,
					Round:     key.Round,
					Details: map[string]any{
						"tx_count":      len(block.Body.Transactions),
						"final_answers": len(block.FinalAnswers),
					},
				})

				if localHash != "" && containsTx(block, localHash) {
					select {
					case txDone <- doneResult{key: key, block: block}:
					default:
					}
				}
			},
		},
	})
	if err != nil {
		log.Fatalf("create localchain: %v", err)
	}

	lchain.Start()
	defer func() {
		lchain.Stop()
		lchain.Close()
	}()

	tl.Emit(experiment.Event{EventType: "chain_started"})

	submitResp, err := lchain.NodeView.SubmitTransaction(explorer.SubmitTransactionRequest{
		Sender: "alice",
		Prompt: cfg.Question,
		Nonce:  0,
		Tip:    0,
	})
	if err != nil {
		log.Fatalf("submit transaction: %v", err)
	}
	mu.Lock()
	submittedTxHash = submitResp.TxHash
	submittedAt = time.Now()
	mu.Unlock()

	tl.Emit(experiment.Event{EventType: "tx_submitted", TxHash: submitResp.TxHash})
	slog.Info("transaction submitted", "tx_hash", submitResp.TxHash)

	// Poll for mempool admission (preprocessing complete) in the background.
	txHashBytes, _ := hex.DecodeString(submitResp.TxHash)
	rawTxKey := string(txHashBytes)
	pendingCh := make(chan time.Time, 1)
	go func() {
		for {
			time.Sleep(200 * time.Millisecond)
			status, ok := lchain.NodeView.TxTracker.GetStatus(rawTxKey)
			if ok && (status == tracker.TxStatusPending || status == tracker.TxStatusFinalized) {
				pendingCh <- time.Now()
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	outcome := "timeout"
	var finalBlock *data.BlockOnChain
	var finalKey data.RoundKey
	var pendingAt time.Time

loop:
	for {
		select {
		case t := <-pendingCh:
			pendingAt = t
			pendingCh = nil
			elapsed := t.Sub(submittedAt)
			tl.Emit(experiment.Event{
				EventType: "tx_pending",
				TxHash:    submitResp.TxHash,
				Details:   map[string]any{"preprocessing_ms": elapsed.Milliseconds()},
			})
			slog.Info("transaction in mempool", "tx_hash", submitResp.TxHash, "preprocessing_ms", elapsed.Milliseconds())

		case result := <-txDone:
			outcome = "pass"
			finalBlock = result.block
			finalKey = result.key
			break loop

		case <-ctx.Done():
			slog.Error("timed out waiting for finalization", "timeout", *timeout)
			break loop
		}
	}

	mu.Lock()
	selRound := selectedRound
	selEpoch := selectedEpoch
	empty := emptyMR1Rounds
	mu.Unlock()

	finalTxStatus := ""
	if finalBlock != nil {
		for _, fa := range finalBlock.FinalAnswers {
			if hex.EncodeToString(fa.TxHash) == submitResp.TxHash {
				finalTxStatus = string(fa.Status)
				break
			}
		}
	}

	tl.Emit(experiment.Event{
		EventType: "experiment_done",
		Details:   map[string]any{"outcome": outcome, "final_tx_status": finalTxStatus},
	})

	writeSummary(dir, runID, outcome, startTime, submittedAt, pendingAt,
		selRound, selEpoch, empty, finalTxStatus, submitResp.TxHash, finalKey)
	slog.Info("experiment complete", "outcome", outcome, "dir", dir)
}

// containsTx reports whether the block contains a transaction with the given hex hash.
func containsTx(block *data.BlockOnChain, txHashHex string) bool {
	for _, tx := range block.Body.Transactions {
		if hex.EncodeToString(tx.GetTxHash()) == txHashHex {
			return true
		}
	}
	return false
}

// hexTxHashes returns the hex-encoded hashes of all transactions in the block.
func hexTxHashes(block *data.BlockOnChain) []string {
	hashes := make([]string, len(block.Body.Transactions))
	for i, tx := range block.Body.Transactions {
		hashes[i] = hex.EncodeToString(tx.GetTxHash())
	}
	return hashes
}

// healthResponse is the subset of the Python /health response we need.
type healthResponse struct {
	Status    string `json:"status"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Reachable bool   `json:"reachable"`
}

func checkAllAgents(specs []experiment.ValidatorSpec, perAgentTimeout time.Duration) error {
	hc := &http.Client{Timeout: perAgentTimeout}
	for _, v := range specs {
		resp, err := hc.Get(v.AgentEndpoint + "/health")
		if err != nil {
			return fmt.Errorf("agent %s (%s): unreachable: %w", v.ValidatorName, v.AgentEndpoint, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("agent %s: /health returned HTTP %d", v.ValidatorName, resp.StatusCode)
		}
		var hr healthResponse
		if err := json.Unmarshal(body, &hr); err != nil {
			return fmt.Errorf("agent %s: parse health response: %w", v.ValidatorName, err)
		}
		if !hr.Reachable {
			return fmt.Errorf("agent %s: provider %q reports not reachable", v.ValidatorName, hr.Provider)
		}
		if hr.Provider != v.Provider {
			return fmt.Errorf("agent %s: provider mismatch: want %q, got %q", v.ValidatorName, v.Provider, hr.Provider)
		}
		if hr.Model != v.Model {
			return fmt.Errorf("agent %s: model mismatch: want %q, got %q", v.ValidatorName, v.Model, hr.Model)
		}
		slog.Info("agent ok", "name", v.ValidatorName, "provider", hr.Provider, "model", hr.Model)
	}
	return nil
}

type experimentSummary struct {
	RunID                   string  `json:"run_id"`
	CompletedAt             string  `json:"completed_at"`
	Outcome                 string  `json:"outcome"`
	TotalDurationS          float64 `json:"total_duration_seconds"`
	TxHash                  string  `json:"tx_hash"`
	SubmittedAt             string  `json:"submitted_at,omitempty"`
	PendingAt               string  `json:"pending_at,omitempty"`
	PreprocessingMS         int64   `json:"preprocessing_ms,omitempty"`
	SelectedEpoch           uint64  `json:"selected_epoch,omitempty"`
	SelectedRound           uint64  `json:"selected_round,omitempty"`
	EmptyRoundsBeforeSelect int     `json:"empty_rounds_before_selection"`
	FinalizedEpoch          uint64  `json:"finalized_epoch,omitempty"`
	FinalizedRound          uint64  `json:"finalized_round,omitempty"`
	FinalTxStatus           string  `json:"final_tx_status,omitempty"`
}

func writeSummary(dir, runID, outcome string,
	startTime, submittedAt, pendingAt time.Time,
	selectedRound, selectedEpoch uint64,
	emptyRounds int,
	finalTxStatus, txHash string,
	finalKey data.RoundKey,
) {
	s := experimentSummary{
		RunID:                   runID,
		CompletedAt:             time.Now().UTC().Format(time.RFC3339),
		Outcome:                 outcome,
		TotalDurationS:          time.Since(startTime).Seconds(),
		TxHash:                  txHash,
		EmptyRoundsBeforeSelect: emptyRounds,
		FinalTxStatus:           finalTxStatus,
	}
	if !submittedAt.IsZero() {
		s.SubmittedAt = submittedAt.UTC().Format(time.RFC3339Nano)
	}
	if !pendingAt.IsZero() {
		s.PendingAt = pendingAt.UTC().Format(time.RFC3339Nano)
		s.PreprocessingMS = pendingAt.Sub(submittedAt).Milliseconds()
	}
	if selectedRound > 0 {
		s.SelectedRound = selectedRound
		s.SelectedEpoch = selectedEpoch
	}
	if finalKey.Round > 0 {
		s.FinalizedRound = finalKey.Round
		s.FinalizedEpoch = finalKey.Epoch
	}
	raw, _ := json.MarshalIndent(s, "", "  ")
	raw = append(raw, '\n')
	_ = os.WriteFile(filepath.Join(dir, "summary.json"), raw, 0o644)
}
