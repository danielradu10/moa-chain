package miniround2

import "moa-chain/data"

// filterNonRelatedTransactions returns a BlockBody containing only the
// transactions whose hash is not in the non-related set. Transactions are
// kept in their original canonical order so the resulting slice is
// deterministic across all validators.
func filterNonRelatedTransactions(body data.BlockBody, nonRelatedTxHashes []string) data.BlockBody {
	if len(nonRelatedTxHashes) == 0 {
		return body
	}

	excluded := make(map[string]struct{}, len(nonRelatedTxHashes))
	for _, h := range nonRelatedTxHashes {
		excluded[h] = struct{}{}
	}

	filtered := make([]data.Transaction, 0, len(body.Transactions))
	for _, tx := range body.Transactions {
		if _, isExcluded := excluded[string(tx.GetTxHash())]; !isExcluded {
			filtered = append(filtered, tx)
		}
	}

	return data.BlockBody{Transactions: filtered}
}
