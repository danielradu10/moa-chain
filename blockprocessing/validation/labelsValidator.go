package validation

import (
	"log/slog"

	"moa-chain/blockprocessing"
	"moa-chain/data"
)

const (
	numAcceptedLabels = 6
)

type labelsValidator struct {
}

func NewLabelsValidator() *labelsValidator {
	return &labelsValidator{}
}

func (lv *labelsValidator) ValidateLabels(txsSubdomains data.Subdomains) error {
	for _, subdomains := range txsSubdomains {
		if len(subdomains) != numAcceptedLabels {
			slog.Error("Invalid number of subdomains", "numSubdomains", len(subdomains))
			return blockprocessing.ErrInvalidNumSubdomains
		}

		uniqueLabels := make(map[string]struct{})
		for _, subdomain := range subdomains {
			_, ok := data.PossibleSubDomains[subdomain]
			if !ok {
				return blockprocessing.ErrInvalidSubdomain
			}

			_, ok = uniqueLabels[subdomain]
			if ok {
				return blockprocessing.ErrDuplicatedLabel
			}

			uniqueLabels[subdomain] = struct{}{}
		}
	}

	return nil
}

// AggregateLabels aggregates the labels of the transactions.
func (lv *labelsValidator) AggregateLabels(aggregatedSubdomains []data.Subdomains) (data.SubdomainsFrequency, error) {
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

	// TODO we can check now per transaction if a label has 2f+1 at least occurrences. if not, skip. do not use it for finalized domains.

	subdomainsFrequency := make(data.SubdomainsFrequency)
	for _, subdomains := range aggregatedLabelsPerTxHash {
		for subdomain, freq := range subdomains {
			subdomainsFrequency[subdomain] += int(freq)
		}
	}

	return subdomainsFrequency, nil
}
