//go:build integration

package integrationtests

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"moa-chain/data"
)

// qualifiedClassificationObservation is an analysis-friendly rendering of one
// signed classification. Signature material remains available in validator
// logs; this structure binds the semantic decision to model and ground truth.
type qualifiedClassificationObservation struct {
	TxHash        string `json:"tx_hash"`
	ProducerID    string `json:"producer_id"`
	AnswerHash    string `json:"answer_hash"`
	Actual        string `json:"actual"`
	Expected      string `json:"expected"`
	IsAdversarial bool   `json:"is_adversarial"`
	SemanticError bool   `json:"semantic_error"`
	ProtocolError bool   `json:"protocol_error"`
}

type qualifiedClassificationVote struct {
	JudgeID         string                               `json:"judge_id"`
	Machine         string                               `json:"machine"`
	Model           string                               `json:"model"`
	MockedByzantine bool                                 `json:"mocked_byzantine"`
	Classifications []qualifiedClassificationObservation `json:"classifications"`
}

func qualifiedRoundTimeout(t *testing.T, fallback time.Duration) time.Duration {
	t.Helper()
	raw := os.Getenv("MOA_MR2_ROUND_TIMEOUT")
	if raw == "" {
		return fallback
	}
	timeout, err := time.ParseDuration(raw)
	require.NoError(t, err, "MOA_MR2_ROUND_TIMEOUT must be a Go duration")
	require.Positive(t, timeout, "MOA_MR2_ROUND_TIMEOUT must be positive")
	return timeout
}

func qualifiedJudgeTimeoutSeconds(t *testing.T, fallback int) int {
	t.Helper()
	raw := os.Getenv("MOA_JUDGE_TIMEOUT_SECONDS")
	if raw == "" {
		return fallback
	}
	timeout, err := strconv.Atoi(raw)
	require.NoError(t, err, "MOA_JUDGE_TIMEOUT_SECONDS must be an integer")
	require.Positive(t, timeout, "MOA_JUDGE_TIMEOUT_SECONDS must be positive")
	return timeout
}

func qualifiedExpectedCategory(group string) data.AnswerCategory {
	switch group {
	case "group_c":
		return data.AnswerCategoryMalicious
	case "group_d":
		return data.AnswerCategoryHallucination
	default:
		return data.AnswerCategoryWrong
	}
}

func qualifiedValidatorIndex(validatorID string) int {
	var index int
	if _, err := fmt.Sscanf(validatorID, "validator-%d", &index); err != nil {
		return -1
	}
	return index - 1
}

func collectQualifiedClassificationVotes(
	nodes []*integrationTestNode,
	roundKey data.RoundKey,
	cfg clusterConfig,
	group string,
	badProducerIDs []string,
	mockedJudgeIDs []string,
) (string, []qualifiedClassificationVote) {
	var sourceID string
	var sourceVotes []*data.AnswerClassificationVote
	for _, node := range nodes {
		votes, err := node.roundState.GetAnswerClassificationVotes(roundKey)
		if err == nil && len(votes) > len(sourceVotes) {
			sourceID = node.id
			sourceVotes = votes
		}
	}

	badProducers := make(map[string]struct{}, len(badProducerIDs))
	for _, id := range badProducerIDs {
		badProducers[id] = struct{}{}
	}
	mockedJudges := make(map[string]struct{}, len(mockedJudgeIDs))
	for _, id := range mockedJudgeIDs {
		mockedJudges[id] = struct{}{}
	}

	out := make([]qualifiedClassificationVote, 0, len(sourceVotes))
	for _, vote := range sourceVotes {
		index := qualifiedValidatorIndex(vote.JudgeID)
		machine, model := "unknown", "unknown"
		if index >= 0 && index < len(cfg.Agents) {
			machine = cfg.Agents[index].Machine
			model = cfg.Agents[index].Model
		}
		_, mocked := mockedJudges[vote.JudgeID]
		observed := qualifiedClassificationVote{
			JudgeID: vote.JudgeID, Machine: machine, Model: model, MockedByzantine: mocked,
			Classifications: make([]qualifiedClassificationObservation, 0, len(vote.AnswerClassifications)),
		}
		for _, classification := range vote.AnswerClassifications {
			_, adversarial := badProducers[classification.CandidateID.ProducerID]
			expected := data.AnswerCategoryCorrect
			if adversarial {
				expected = qualifiedExpectedCategory(group)
			}
			observed.Classifications = append(observed.Classifications, qualifiedClassificationObservation{
				TxHash:        string(classification.CandidateID.TxHash),
				ProducerID:    classification.CandidateID.ProducerID,
				AnswerHash:    hex.EncodeToString(classification.CandidateID.AnswerHash),
				Actual:        string(classification.Category),
				Expected:      string(expected),
				IsAdversarial: adversarial,
				SemanticError: !mocked && classification.Category != expected,
				ProtocolError: !mocked && ((adversarial && classification.Category == data.AnswerCategoryCorrect) ||
					(!adversarial && classification.Category != data.AnswerCategoryCorrect)),
			})
		}
		out = append(out, observed)
	}
	return sourceID, out
}

func qualifiedBadProducerIDs(count int) []string {
	ids := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		ids = append(ids, fmt.Sprintf("validator-%d", i))
	}
	return ids
}

func qualifiedMissingJudgeIDs(votes []qualifiedClassificationVote, validatorCount int) []string {
	observed := make(map[string]struct{}, len(votes))
	for _, vote := range votes {
		observed[vote.JudgeID] = struct{}{}
	}
	missing := make([]string, 0, validatorCount-len(observed))
	for index := 1; index <= validatorCount; index++ {
		id := fmt.Sprintf("validator-%d", index)
		if _, ok := observed[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing
}
