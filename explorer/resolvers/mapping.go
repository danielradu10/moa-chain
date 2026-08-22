package resolvers

import (
	"encoding/hex"

	"moa-chain/data"
	"moa-chain/explorer"
)

func toBlockResponse(block *data.BlockOnChain) explorer.BlockResponse {
	return explorer.BlockResponse{
		HeaderHash:            hex.EncodeToString(block.Header.HeaderHash),
		PreviousHash:          hex.EncodeToString(block.Header.PreviousHash),
		Round:                 block.Header.Round,
		Epoch:                 block.Header.Epoch,
		Transactions:          toTxSummaries(block.Body.Transactions),
		SubdomainsFrequency:   block.SubdomainsFrequencies,
		NonRelatedTxHashes:    block.NonRelatedTransactionHashes,
		AnswerClassifications: toClassificationSummaries(block.AnswerClassifications),
		FinalAnswers:          toFinalAnswerSummaries(block.FinalAnswers),
	}
}

func toMR1Response(block *data.BlockOnChain) *explorer.MR1Response {
	if block == nil {
		return nil
	}

	return &explorer.MR1Response{
		BlockHash:           hex.EncodeToString(block.Header.HeaderHash),
		Transactions:        toTxSummaries(block.Body.Transactions),
		SubdomainsFrequency: block.SubdomainsFrequencies,
		NonRelatedTxHashes:  block.NonRelatedTransactionHashes,
	}
}

func toMR2Response(block *data.BlockOnChain) *explorer.MR2Response {
	if block == nil {
		return nil
	}

	return &explorer.MR2Response{
		BlockHash:             hex.EncodeToString(block.Header.HeaderHash),
		AnswerClassifications: toClassificationSummaries(block.AnswerClassifications),
	}
}

func toMR3Response(block *data.BlockOnChain) *explorer.MR3Response {
	if block == nil {
		return nil
	}

	return &explorer.MR3Response{
		BlockHash:    hex.EncodeToString(block.Header.HeaderHash),
		FinalAnswers: toFinalAnswerSummaries(block.FinalAnswers),
	}
}

func toTxSummaries(txs []data.Transaction) []explorer.TxSummary {
	out := make([]explorer.TxSummary, 0, len(txs))
	for _, tx := range txs {
		out = append(out, explorer.TxSummary{
			TxHash: hex.EncodeToString(tx.GetTxHash()),
			Sender: string(tx.GetSender()),
			Prompt: string(tx.GetPrompt()),
			Labels: tx.GetDomainLabels(),
		})
	}

	return out
}

func toClassificationSummaries(cs []data.TransactionAnswerClassification) []explorer.TxClassificationSummary {
	out := make([]explorer.TxClassificationSummary, 0, len(cs))
	for _, c := range cs {
		var correct uint64
		for _, count := range c.Counts {
			correct += count.Correct
		}

		out = append(out, explorer.TxClassificationSummary{
			TxHash:       hex.EncodeToString(c.TxHash),
			Status:       string(c.Status),
			CorrectCount: correct,
		})
	}

	return out
}

func toFinalAnswerSummaries(answers []data.FinalAnswer) []explorer.FinalAnswerSummary {
	out := make([]explorer.FinalAnswerSummary, 0, len(answers))
	for _, a := range answers {
		out = append(out, explorer.FinalAnswerSummary{
			TxHash: hex.EncodeToString(a.TxHash),
			Status: string(a.Status),
			Answer: a.Answer,
		})
	}

	return out
}
