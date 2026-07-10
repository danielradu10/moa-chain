package validation

import (
	"log/slog"
	"sort"

	"moa-chain/blockprocessing"
	"moa-chain/data"
	"moa-chain/logging"
)

const (
	maxAcceptedLabels = 3
)

type labelsValidator struct {
	logger *slog.Logger
}

func NewLabelsValidator(loggers ...*slog.Logger) *labelsValidator {
	return &labelsValidator{
		logger: logging.FromOptional(loggers...),
	}
}

// ValidateLabels enforces the labeling rules for a single validator's subdomain map:
//   - A transaction labeled non_related must have exactly that one label.
//   - A transaction with real labels must have between 1 and maxAcceptedLabels labels,
//     all from PossibleSubDomains, with no duplicates and no mixing with non_related.
func (lv *labelsValidator) ValidateLabels(txsSubdomains data.Subdomains) error {
	lv.logger.Debug("validation.ValidateLabels started", "numTransactions", len(txsSubdomains))

	for _, subdomains := range txsSubdomains {
		if err := lv.validateTransactionLabels(subdomains); err != nil {
			return err
		}
	}

	lv.logger.Debug("validation.ValidateLabels finished", "numTransactions", len(txsSubdomains))
	return nil
}

func (lv *labelsValidator) validateTransactionLabels(subdomains []string) error {
	if len(subdomains) == 0 {
		lv.logger.Error("validation.ValidateLabels empty label list")
		return blockprocessing.ErrInvalidNumSubdomains
	}

	hasNonRelated := false
	for _, subdomain := range subdomains {
		if subdomain == data.NonRelatedSubdomain {
			hasNonRelated = true
			break
		}
	}

	// non_related must be the only label when present.
	if hasNonRelated {
		if len(subdomains) != 1 {
			lv.logger.Error("validation.ValidateLabels non_related mixed with real subdomain", "numSubdomains", len(subdomains))
			return blockprocessing.ErrNonRelatedMixedWithLabel
		}
		return nil
	}

	if len(subdomains) > maxAcceptedLabels {
		lv.logger.Error("validation.ValidateLabels too many subdomains", "numSubdomains", len(subdomains), "max", maxAcceptedLabels)
		return blockprocessing.ErrInvalidNumSubdomains
	}

	uniqueLabels := make(map[string]struct{}, len(subdomains))
	for _, subdomain := range subdomains {
		_, ok := data.PossibleSubDomains[subdomain]
		if !ok {
			lv.logger.Error("validation.ValidateLabels invalid subdomain", "subdomain", subdomain)
			return blockprocessing.ErrInvalidSubdomain
		}

		_, ok = uniqueLabels[subdomain]
		if ok {
			lv.logger.Error("validation.ValidateLabels duplicated label", "subdomain", subdomain)
			return blockprocessing.ErrDuplicatedLabel
		}

		uniqueLabels[subdomain] = struct{}{}
	}

	return nil
}

// AggregateLabels derives the block-level subdomain frequency map from the accepted
// quorum certificate and identifies which transaction hashes have non_related as their
// dominant label. A label reaches the frequency map only when its count across the
// provided validator subdomain maps meets the quorum threshold.
//
// A transaction is non-related when non_related is the only label that reaches the
// quorum threshold for that tx. The returned slice of non-related tx hashes is sorted
// for deterministic finalization.
func (lv *labelsValidator) AggregateLabels(aggregatedSubdomains []data.Subdomains, consensusGroupSize uint64) (data.SubdomainsFrequency, []string, error) {
	quorumThreshold := (2*consensusGroupSize)/3 + 1
	lv.logger.Info(
		"validation.AggregateLabels started",
		"numValidatorSubdomainMaps", len(aggregatedSubdomains),
		"consensusGroupSize", consensusGroupSize,
		"quorumThreshold", quorumThreshold,
	)

	// First pass: count how many validators assigned each label to each tx.
	countPerTx := make(map[string]map[string]uint64)
	for _, validatorSubdomains := range aggregatedSubdomains {
		for txHash, subdomains := range validatorSubdomains {
			if _, ok := countPerTx[txHash]; !ok {
				countPerTx[txHash] = make(map[string]uint64)
			}
			for _, subdomain := range subdomains {
				countPerTx[txHash][subdomain]++
			}
		}
	}

	subdomainsFrequency := make(data.SubdomainsFrequency)
	var nonRelatedTxHashes []string

	for txHash, labelCounts := range countPerTx {
		nonRelatedCount := labelCounts[data.NonRelatedSubdomain]

		if nonRelatedCount >= quorumThreshold {
			// non_related reached quorum for this transaction: the protocol mixing
			// rule means no real subdomain can also reach quorum for the same tx.
			nonRelatedTxHashes = append(nonRelatedTxHashes, txHash)
			lv.logger.Debug("transaction labeled non_related by quorum", "txHash", txHash, "count", nonRelatedCount, "threshold", quorumThreshold)
			continue
		}

		for subdomain, freq := range labelCounts {
			if subdomain == data.NonRelatedSubdomain {
				continue
			}
			if freq < quorumThreshold {
				lv.logger.Debug("label did not reach threshold for transaction", "txHash", txHash, "subdomain", subdomain, "frequency", freq, "threshold", quorumThreshold)
				continue
			}
			lv.logger.Debug("label reached threshold for transaction", "txHash", txHash, "subdomain", subdomain, "frequency", freq, "threshold", quorumThreshold)
			subdomainsFrequency[subdomain] += freq
		}
	}

	sort.Strings(nonRelatedTxHashes)

	lv.logger.Info("validation.AggregateLabels finished", "frequencies", subdomainsFrequency, "numNonRelatedTxs", len(nonRelatedTxHashes))
	return subdomainsFrequency, nonRelatedTxHashes, nil
}
