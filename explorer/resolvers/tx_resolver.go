package resolvers

import (
	"bytes"
	"encoding/hex"

	"moa-chain/data"
	"moa-chain/explorer"
	"moa-chain/tracker"
)

// TxResolver looks up a transaction's lifecycle state across all sources.
type TxResolver interface {
	// Resolve returns the full state of a transaction by its hex-encoded hash.
	// Sources are checked in priority order: TxTracker (status), mempool
	// (pending details), PrecomputedStore (labels/answer), and chain (finalized).
	Resolve(hexHash string) (explorer.TransactionResponse, bool)
	// ResolveAll returns all known transactions: finalized ones from the chain
	// followed by in-flight ones from the mempool.
	ResolveAll() []explorer.TransactionResponse
}

type txResolver struct {
	node explorer.NodeFacade
}

// NewTxResolver returns a TxResolver backed by the given node.
func NewTxResolver(node explorer.NodeFacade) TxResolver {
	return &txResolver{node: node}
}

func (r *txResolver) Resolve(hexHash string) (explorer.TransactionResponse, bool) {
	hashBytes, err := hex.DecodeString(hexHash)
	if err != nil {
		return explorer.TransactionResponse{}, false
	}

	status, ok := r.node.GetTransactionStatus(hashBytes)
	if !ok {
		return explorer.TransactionResponse{}, false
	}

	resp := explorer.TransactionResponse{
		TxHash: hexHash,
		Status: string(status),
	}

	switch status {
	case tracker.TxStatusFinalized:
		r.enrichFromChain(hashBytes, &resp)
	default:
		r.enrichFromMempool(hashBytes, &resp)
		if status == tracker.TxStatusPending {
			r.enrichFromStore(hashBytes, &resp)
		}
	}

	return resp, true
}

func (r *txResolver) ResolveAll() []explorer.TransactionResponse {
	seen := make(map[string]struct{})
	var result []explorer.TransactionResponse

	for _, block := range r.node.AllBlocks() {
		blockHash := hex.EncodeToString(block.Header.HeaderHash)
		for _, tx := range block.Body.Transactions {
			hexHash := hex.EncodeToString(tx.GetTxHash())
			if _, ok := seen[hexHash]; ok {
				continue
			}
			seen[hexHash] = struct{}{}

			resp := explorer.TransactionResponse{
				TxHash:    hexHash,
				Status:    string(tracker.TxStatusFinalized),
				Sender:    string(tx.GetSender()),
				Prompt:    string(tx.GetPrompt()),
				Labels:    tx.GetDomainLabels(),
				BlockHash: blockHash,
			}
			for _, fa := range block.FinalAnswers {
				if bytes.Equal(fa.TxHash, tx.GetTxHash()) {
					resp.FinalAnswer = fa.Answer
					resp.FinalStatus = string(fa.Status)
				}
			}
			result = append(result, resp)
		}
	}

	for _, tx := range r.node.GetPendingTransactions() {
		hexHash := hex.EncodeToString(tx.GetTxHash())
		if _, ok := seen[hexHash]; ok {
			continue
		}
		seen[hexHash] = struct{}{}

		status, _ := r.node.GetTransactionStatus(tx.GetTxHash())
		resp := explorer.TransactionResponse{
			TxHash: hexHash,
			Status: string(status),
			Sender: string(tx.GetSender()),
			Prompt: string(tx.GetPrompt()),
			Labels: tx.GetDomainLabels(),
		}
		if status == tracker.TxStatusPending {
			r.enrichFromStore(tx.GetTxHash(), &resp)
		}
		result = append(result, resp)
	}

	return result
}

func (r *txResolver) enrichFromMempool(txHash []byte, resp *explorer.TransactionResponse) {
	tx, ok := r.node.GetPendingTransaction(txHash)
	if !ok {
		return
	}
	resp.Sender = string(tx.GetSender())
	resp.Prompt = string(tx.GetPrompt())
	resp.Labels = tx.GetDomainLabels()
}

func (r *txResolver) enrichFromStore(txHash []byte, resp *explorer.TransactionResponse) {
	if labels, ok := r.node.GetPrecomputedLabels(txHash); ok {
		resp.Labels = labels
	}
	if answer, ok := r.node.GetPrecomputedAnswer(txHash); ok {
		resp.LocalAnswer = answer
	}
}

func (r *txResolver) enrichFromChain(txHash []byte, resp *explorer.TransactionResponse) {
	for _, block := range r.node.AllBlocks() {
		for _, tx := range block.Body.Transactions {
			if !bytes.Equal(tx.GetTxHash(), txHash) {
				continue
			}
			resp.Sender = string(tx.GetSender())
			resp.Prompt = string(tx.GetPrompt())
			resp.Labels = tx.GetDomainLabels()
			resp.BlockHash = hex.EncodeToString(block.Header.HeaderHash)

			for _, fa := range block.FinalAnswers {
				if bytes.Equal(fa.TxHash, txHash) {
					resp.FinalAnswer = fa.Answer
					resp.FinalStatus = string(fa.Status)
				}
			}

			resp.ValidatorAnswers = buildValidatorAnswers(block, txHash)
			resp.LabelVotes = buildLabelVotes(block, txHash)
			resp.SynthesisProposer = block.SynthesisProposerID
			resp.SynthesisApprovers = block.SynthesisApprovers
			return
		}
	}
}

// buildLabelVotes extracts each MR1 validator's label assignments for a specific transaction.
func buildLabelVotes(block *data.BlockOnChain, txHash []byte) []explorer.ValidatorLabelVote {
	if len(block.LabelVotes) == 0 {
		return nil
	}
	txKey := string(txHash)
	var result []explorer.ValidatorLabelVote
	for _, lv := range block.LabelVotes {
		labels, ok := lv.Labels[txKey]
		if !ok || len(labels) == 0 {
			continue
		}
		result = append(result, explorer.ValidatorLabelVote{
			ValidatorID: lv.ValidatorID,
			Labels:      labels,
		})
	}
	return result
}

// buildValidatorAnswers extracts per-validator MR2 answers with the consensus category,
// per-category judge vote counts, and the full per-judge verdict list.
func buildValidatorAnswers(block *data.BlockOnChain, txHash []byte) []explorer.ValidatorAnswer {
	if block.AnswerEvidence == nil {
		return nil
	}
	evidence := block.AnswerEvidence
	// AnswersTxMessage keys are raw tx-hash bytes cast to string (matches miniround2 handler).
	txKey := string(txHash)

	// Build producerID → consensus category from canonical answer groups.
	categoryByProducer := make(map[string]string)
	// Build producerID → vote counts from per-candidate counts.
	type voteCounts struct{ correct, hallucination, malicious, wrong uint64 }
	countsByProducer := make(map[string]voteCounts)
	for _, cls := range block.AnswerClassifications {
		if !bytes.Equal(cls.TxHash, txHash) {
			continue
		}
		for _, id := range cls.Groups.Correct {
			categoryByProducer[id.ProducerID] = "CORRECT"
		}
		for _, id := range cls.Groups.Hallucination {
			categoryByProducer[id.ProducerID] = "HALLUCINATION"
		}
		for _, id := range cls.Groups.Malicious {
			categoryByProducer[id.ProducerID] = "MALICIOUS"
		}
		for _, id := range cls.Groups.Wrong {
			categoryByProducer[id.ProducerID] = "WRONG"
		}
		for _, c := range cls.Counts {
			countsByProducer[c.CandidateID.ProducerID] = voteCounts{
				correct:       c.Correct,
				hallucination: c.Hallucination,
				malicious:     c.Malicious,
				wrong:         c.Wrong,
			}
		}
		break
	}

	// Build producerID → per-judge verdicts from stored classification votes.
	verdictsByProducer := make(map[string][]explorer.JudgeVerdict)
	for _, vote := range block.ClassificationVotes {
		for _, clf := range vote.AnswerClassifications {
			if !bytes.Equal(clf.CandidateID.TxHash, txHash) {
				continue
			}
			pid := clf.CandidateID.ProducerID
			verdictsByProducer[pid] = append(verdictsByProducer[pid], explorer.JudgeVerdict{
				JudgeID:  vote.JudgeID,
				Category: string(clf.Category),
			})
		}
	}

	var answers []explorer.ValidatorAnswer
	for i, signerID := range evidence.Signers {
		if i >= len(evidence.Answers) {
			break
		}
		result, ok := evidence.Answers[i][txKey]
		if !ok {
			continue
		}
		vc := countsByProducer[signerID]
		answers = append(answers, explorer.ValidatorAnswer{
			ValidatorID:        signerID,
			Answer:             result.Answer,
			Category:           categoryByProducer[signerID],
			Consumption:        result.ActualConsumption,
			CorrectVotes:       vc.correct,
			HallucinationVotes: vc.hallucination,
			MaliciousVotes:     vc.malicious,
			WrongVotes:         vc.wrong,
			JudgeVerdicts:      verdictsByProducer[signerID],
		})
	}
	return answers
}
