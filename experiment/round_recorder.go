package experiment

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"moa-chain/data"
)

// MR1RoundResult captures MR1 consensus data for one round.
type MR1RoundResult struct {
	StartTimestamp        string            `json:"start_ts"`
	FinalizationTimestamp string            `json:"finalization_ts"`
	FinalizationMS        float64           `json:"finalization_ms"`
	SubdomainFrequencies  map[string]uint64 `json:"subdomain_frequencies"`
	LabelVotes            []LabelVoteRecord `json:"label_votes"`
}

// LabelVoteRecord is one validator's label assignments in MR1.
type LabelVoteRecord struct {
	ValidatorID string              `json:"validator_id"`
	Labels      map[string][]string `json:"labels"`
}

// CandidateResult is one producer's answer with classification counts in MR2.
type CandidateResult struct {
	ProducerID    string `json:"producer_id"`
	Answer        string `json:"answer"`
	Correct       uint64 `json:"correct"`
	Wrong         uint64 `json:"wrong"`
	Hallucination uint64 `json:"hallucination"`
	Malicious     uint64 `json:"malicious"`
}

// ClassificationVoteResult is one judge's vote summary for one round.
type ClassificationVoteResult struct {
	JudgeID             string `json:"judge_id"`
	ClassificationCount int    `json:"classification_count"`
	Correct             uint64 `json:"correct"`
	Wrong               uint64 `json:"wrong"`
	Hallucination       uint64 `json:"hallucination"`
	Malicious           uint64 `json:"malicious"`
}

// TxResult captures the classification outcome for one transaction.
type TxResult struct {
	TxHash     string            `json:"tx_hash"`
	Status     string            `json:"status"`
	Candidates []CandidateResult `json:"candidates"`
	LeaderID   string            `json:"leader_id"`
}

// FinalAnswerRecord is one transaction's on-chain output after MR3.
type FinalAnswerRecord struct {
	TxHash string `json:"tx_hash"`
	Status string `json:"status"`
	Answer string `json:"answer,omitempty"`
}

// MR3RoundResult captures MR3 synthesis data for one round.
type MR3RoundResult struct {
	StartTimestamp        string              `json:"start_ts"`
	FinalizationTimestamp string              `json:"finalization_ts"`
	FinalizationMS        float64             `json:"finalization_ms"`
	SynthesisProposerID   string              `json:"synthesis_proposer_id"`
	SynthesisApprovers    []string            `json:"synthesis_approvers"`
	FinalAnswers          []FinalAnswerRecord `json:"final_answers"`
}

// MR2RoundResult captures MR2 consensus data for one round.
type MR2RoundResult struct {
	StartTimestamp        string                     `json:"start_ts"`
	FinalizationTimestamp string                     `json:"finalization_ts"`
	FinalizationMS        float64                    `json:"finalization_ms"`
	LeaderID              string                     `json:"leader_id"`
	Producers             []string                   `json:"producers"`
	ClassificationVotes   []ClassificationVoteResult `json:"classification_votes"`
	Transactions          []TxResult                 `json:"transactions"`
}

// RoundResult is persisted to rounds/round-EEEE-RRRR.json.
type RoundResult struct {
	RunID        string          `json:"run_id"`
	Epoch        uint64          `json:"epoch"`
	Round        uint64          `json:"round"`
	RecordedAt   string          `json:"recorded_at"`
	SelectedTxs  []string        `json:"selected_tx_hashes"`
	MR1          *MR1RoundResult `json:"mr1,omitempty"`
	MR2          *MR2RoundResult `json:"mr2,omitempty"`
	MR3          *MR3RoundResult `json:"mr3,omitempty"`
	RoundOutcome string          `json:"round_outcome"` // "mr3_finalized", "mr2_finalized", "mr1_only"
}

// RoundRecorder writes per-round JSON files under {dir}/rounds/.
type RoundRecorder struct {
	dir   string
	runID string
}

// NewRoundRecorder creates the rounds/ subdirectory under dir.
func NewRoundRecorder(dir, runID string) (*RoundRecorder, error) {
	roundsDir := filepath.Join(dir, "rounds")
	if err := os.MkdirAll(roundsDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir rounds: %w", err)
	}
	return &RoundRecorder{dir: roundsDir, runID: runID}, nil
}

// WriteMR1 creates the round file with MR1 data.
func (r *RoundRecorder) WriteMR1(
	epoch, round uint64,
	block *data.BlockOnChain,
	startTime, finalTime time.Time,
	selectedTxHashes []string,
) error {
	freqs := make(map[string]uint64, len(block.SubdomainsFrequencies))
	for k, v := range block.SubdomainsFrequencies {
		freqs[k] = v
	}

	votes := make([]LabelVoteRecord, 0, len(block.LabelVotes))
	for _, lv := range block.LabelVotes {
		labels := make(map[string][]string, len(lv.Labels))
		for txHash, subs := range lv.Labels {
			labels[txHash] = subs
		}
		votes = append(votes, LabelVoteRecord{
			ValidatorID: lv.ValidatorID,
			Labels:      labels,
		})
	}

	result := RoundResult{
		RunID:       r.runID,
		Epoch:       epoch,
		Round:       round,
		RecordedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SelectedTxs: selectedTxHashes,
		MR1: &MR1RoundResult{
			StartTimestamp:        startTime.UTC().Format(time.RFC3339Nano),
			FinalizationTimestamp: finalTime.UTC().Format(time.RFC3339Nano),
			FinalizationMS:        float64(finalTime.Sub(startTime).Milliseconds()),
			SubdomainFrequencies:  freqs,
			LabelVotes:            votes,
		},
		RoundOutcome: "mr1_only",
	}
	return r.write(epoch, round, result)
}

// WriteMR2 updates (or creates) the round file with MR2 data.
func (r *RoundRecorder) WriteMR2(
	epoch, round uint64,
	block *data.BlockOnChain,
	mr1StartTime, mr1FinalTime, mr2StartTime, mr2FinalTime time.Time,
	selectedTxHashes []string,
) error {
	existing, err := r.read(epoch, round)
	if err != nil {
		existing = &RoundResult{
			RunID:       r.runID,
			Epoch:       epoch,
			Round:       round,
			SelectedTxs: selectedTxHashes,
		}
	}

	if existing.MR1 == nil {
		freqs := make(map[string]uint64, len(block.SubdomainsFrequencies))
		for k, v := range block.SubdomainsFrequencies {
			freqs[k] = v
		}
		existing.MR1 = &MR1RoundResult{
			StartTimestamp:        mr1StartTime.UTC().Format(time.RFC3339Nano),
			FinalizationTimestamp: mr1FinalTime.UTC().Format(time.RFC3339Nano),
			FinalizationMS:        float64(mr1FinalTime.Sub(mr1StartTime).Milliseconds()),
			SubdomainFrequencies:  freqs,
			LabelVotes:            nil,
		}
	}

	existing.MR2 = buildMR2Result(block, mr2StartTime, mr2FinalTime)
	existing.RoundOutcome = "mr2_finalized"
	existing.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return r.write(epoch, round, *existing)
}

func buildMR2Result(
	block *data.BlockOnChain,
	startTime, finalTime time.Time,
) *MR2RoundResult {
	classVotes := make([]ClassificationVoteResult, 0, len(block.ClassificationVotes))
	for _, v := range block.ClassificationVotes {
		cv := ClassificationVoteResult{
			JudgeID:             v.JudgeID,
			ClassificationCount: len(v.AnswerClassifications),
		}
		for _, ac := range v.AnswerClassifications {
			switch ac.Category {
			case data.AnswerCategoryCorrect:
				cv.Correct++
			case data.AnswerCategoryWrong:
				cv.Wrong++
			case data.AnswerCategoryHallucination:
				cv.Hallucination++
			case data.AnswerCategoryMalicious:
				cv.Malicious++
			}
		}
		classVotes = append(classVotes, cv)
	}

	leaderID := ""
	var producers []string

	// Build answer lookup: txHash → producerID → answer string.
	answersByTxByProducer := make(map[string]map[string]string)
	if block.AnswerEvidence != nil {
		leaderID = block.AnswerEvidence.SenderID
		producers = block.AnswerEvidence.Signers
		for signerIdx, signerID := range block.AnswerEvidence.Signers {
			if signerIdx >= len(block.AnswerEvidence.Answers) {
				break
			}
			for txHashStr, result := range block.AnswerEvidence.Answers[signerIdx] {
				if answersByTxByProducer[txHashStr] == nil {
					answersByTxByProducer[txHashStr] = make(map[string]string)
				}
				answersByTxByProducer[txHashStr][signerID] = result.Answer
			}
		}
	}

	txResults := make([]TxResult, 0, len(block.AnswerClassifications))
	for _, ac := range block.AnswerClassifications {
		txHashStr := string(ac.TxHash)
		candidates := make([]CandidateResult, 0, len(ac.Counts))
		for _, cnt := range ac.Counts {
			pid := cnt.CandidateID.ProducerID
			answer := ""
			if m, ok := answersByTxByProducer[txHashStr]; ok {
				answer = m[pid]
			}
			candidates = append(candidates, CandidateResult{
				ProducerID:    pid,
				Answer:        answer,
				Correct:       cnt.Correct,
				Wrong:         cnt.Wrong,
				Hallucination: cnt.Hallucination,
				Malicious:     cnt.Malicious,
			})
		}
		txResults = append(txResults, TxResult{
			TxHash:     txHashStr,
			Status:     string(ac.Status),
			Candidates: candidates,
			LeaderID:   leaderID,
		})
	}

	return &MR2RoundResult{
		StartTimestamp:        startTime.UTC().Format(time.RFC3339Nano),
		FinalizationTimestamp: finalTime.UTC().Format(time.RFC3339Nano),
		FinalizationMS:        float64(finalTime.Sub(startTime).Milliseconds()),
		LeaderID:              leaderID,
		Producers:             producers,
		ClassificationVotes:   classVotes,
		Transactions:          txResults,
	}
}

// WriteMR3 updates the round file with MR3 synthesis data.
func (r *RoundRecorder) WriteMR3(
	epoch, round uint64,
	block *data.BlockOnChain,
	mr3StartTime, mr3FinalTime time.Time,
) error {
	existing, err := r.read(epoch, round)
	if err != nil {
		existing = &RoundResult{RunID: r.runID, Epoch: epoch, Round: round}
	}

	answers := make([]FinalAnswerRecord, len(block.FinalAnswers))
	for i, fa := range block.FinalAnswers {
		answers[i] = FinalAnswerRecord{
			TxHash: hex.EncodeToString(fa.TxHash),
			Status: string(fa.Status),
			Answer: fa.Answer,
		}
	}

	existing.MR3 = &MR3RoundResult{
		StartTimestamp:        mr3StartTime.UTC().Format(time.RFC3339Nano),
		FinalizationTimestamp: mr3FinalTime.UTC().Format(time.RFC3339Nano),
		FinalizationMS:        float64(mr3FinalTime.Sub(mr3StartTime).Milliseconds()),
		SynthesisProposerID:   block.SynthesisProposerID,
		SynthesisApprovers:    block.SynthesisApprovers,
		FinalAnswers:          answers,
	}
	existing.RoundOutcome = "mr3_finalized"
	existing.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return r.write(epoch, round, *existing)
}

func (r *RoundRecorder) write(epoch, round uint64, result RoundResult) error {
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal round result: %w", err)
	}
	raw = append(raw, '\n')
	path := filepath.Join(r.dir, fmt.Sprintf("round-%04d-%04d.json", epoch, round))
	return os.WriteFile(path, raw, 0o644)
}

func (r *RoundRecorder) read(epoch, round uint64) (*RoundResult, error) {
	path := filepath.Join(r.dir, fmt.Sprintf("round-%04d-%04d.json", epoch, round))
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result RoundResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
