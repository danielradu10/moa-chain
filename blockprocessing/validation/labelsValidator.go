package validation

import (
	"log/slog"

	"moa-chain/blockprocessing"
	"moa-chain/data"
	"moa-chain/logging"
)

const (
	numAcceptedLabels = 6
)

type labelsValidator struct {
	logger *slog.Logger
}

func NewLabelsValidator(loggers ...*slog.Logger) *labelsValidator {
	return &labelsValidator{
		logger: logging.FromOptional(loggers...),
	}
}

func (lv *labelsValidator) ValidateLabels(txsSubdomains data.Subdomains) error {
	lv.logger.Debug("validation.ValidateLabels started", "numTransactions", len(txsSubdomains))

	for _, subdomains := range txsSubdomains {
		if len(subdomains) != numAcceptedLabels {
			lv.logger.Error("validation.ValidateLabels invalid number of subdomains", "numSubdomains", len(subdomains), "expected", numAcceptedLabels)
			return blockprocessing.ErrInvalidNumSubdomains
		}

		uniqueLabels := make(map[string]struct{})
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
	}

	lv.logger.Debug("validation.ValidateLabels finished", "numTransactions", len(txsSubdomains))
	return nil
}

// AggregateLabels aggregates the labels of the transactions.
func (lv *labelsValidator) AggregateLabels(aggregatedSubdomains []data.Subdomains, consensusGroupSize uint64) (data.SubdomainsFrequency, error) {
	quorumThreshold := (2*consensusGroupSize)/3 + 1
	lv.logger.Info(
		"validation.AggregateLabels started",
		"numValidatorSubdomainMaps", len(aggregatedSubdomains),
		"consensusGroupSize", consensusGroupSize,
		"quorumThreshold", quorumThreshold,
	)

	aggregatedLabelsPerTxHash := make(map[string]map[string]uint64)
	for _, validatorSubdomains := range aggregatedSubdomains {
		for txHash, subdomains := range validatorSubdomains {
			_, ok := aggregatedLabelsPerTxHash[txHash]
			if !ok {
				aggregatedLabelsPerTxHash[txHash] = make(map[string]uint64)
			}

			for _, subdomain := range subdomains {
				aggregatedLabelsPerTxHash[txHash][subdomain]++
			}
		}
	}

	subdomainsFrequency := make(data.SubdomainsFrequency)
	for txHash, subdomains := range aggregatedLabelsPerTxHash {
		for subdomain, freq := range subdomains {
			if freq < quorumThreshold {
				lv.logger.Debug(
					"label did not reach threshold for transaction",
					"txHash", txHash,
					"subdomain", subdomain,
					"frequency", freq,
					"threshold", quorumThreshold,
				)
				continue
			}

			lv.logger.Debug(
				"label reached threshold for transaction",
				"txHash", txHash,
				"subdomain", subdomain,
				"frequency", freq,
				"threshold", quorumThreshold,
			)
			subdomainsFrequency[subdomain] += freq
		}
	}

	lv.logger.Info("validation.AggregateLabels finished", "frequencies", subdomainsFrequency)
	return subdomainsFrequency, nil
}
