package integrationtests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

type scenarioResult struct {
	Scenario   string                     `json:"scenario"`
	Passed     bool                       `json:"passed"`
	Network    scenarioNetworkFixture     `json:"network"`
	Committees scenarioResultCommittees   `json:"committees"`
	MiniRound1 scenarioMiniRoundOneResult `json:"miniRoundOne"`
	MiniRound2 scenarioMiniRoundTwoResult `json:"miniRoundTwo"`
}

type scenarioResultCommittees struct {
	MiniRoundOne []scenarioResultMember `json:"miniRoundOne"`
	MiniRoundTwo []scenarioResultMember `json:"miniRoundTwo"`
}

type scenarioResultMember struct {
	Role        string `json:"role"`
	ValidatorID string `json:"validatorId"`
}

type scenarioMiniRoundOneResult struct {
	FinalizedNodes       int                      `json:"finalizedNodes"`
	SubdomainFrequencies data.SubdomainsFrequency `json:"subdomainFrequencies"`
}

type scenarioMiniRoundTwoResult struct {
	FinalizedNodes int                          `json:"finalizedNodes"`
	AnswerEvidence scenarioAnswerEvidenceResult `json:"answerEvidence"`
	Votes          []scenarioVoteResult         `json:"classificationVotes"`
	Transactions   []scenarioTransactionResult  `json:"transactions"`
}

type scenarioAnswerEvidenceResult struct {
	LeaderID           string                 `json:"leaderId"`
	Producers          []scenarioResultMember `json:"producers"`
	SignaturesVerified bool                   `json:"signaturesVerified"`
}

type scenarioVoteResult struct {
	Judge               scenarioResultMember           `json:"judge"`
	ClassificationCount int                            `json:"classificationCount"`
	CategoryTotals      scenarioExpectedCategoryCounts `json:"categoryTotals"`
}

type scenarioTransactionResult struct {
	TxHash     string                       `json:"txHash"`
	Status     data.TransactionAnswerStatus `json:"status"`
	Candidates []scenarioCandidateResult    `json:"candidates"`
	Groups     scenarioGroupsResult         `json:"groups"`
}

type scenarioCandidateResult struct {
	ProducerRole string                         `json:"producerRole"`
	ProducerID   string                         `json:"producerId"`
	Answer       string                         `json:"answer"`
	Counts       scenarioExpectedCategoryCounts `json:"counts"`
}

type scenarioGroupsResult struct {
	Correct       []string `json:"correct"`
	Hallucination []string `json:"hallucination"`
	Malicious     []string `json:"malicious"`
	Wrong         []string `json:"wrong"`
}

func writeScenarioResult(
	t *testing.T,
	scenario miniRoundTwoScenario,
	committees scenarioCommittees,
	miniRoundOneBlock *data.BlockOnChain,
	miniRoundTwoBlock *data.BlockOnChain,
	votes []*data.AnswerClassificationVote,
) {
	t.Helper()

	result := buildScenarioResult(scenario, committees, miniRoundOneBlock, miniRoundTwoBlock, votes)
	encoded, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)
	encoded = append(encoded, '\n')
	require.NoError(t, os.WriteFile(filepath.Join(scenario.directory, "result.json"), encoded, 0o644))
}

func buildScenarioResult(
	scenario miniRoundTwoScenario,
	committees scenarioCommittees,
	miniRoundOneBlock *data.BlockOnChain,
	miniRoundTwoBlock *data.BlockOnChain,
	votes []*data.AnswerClassificationVote,
) scenarioResult {
	rolesByValidator := scenarioRolesByValidator(committees.miniRoundTwo)
	return scenarioResult{
		Scenario: scenario.Name,
		Passed:   true,
		Network:  scenario.Network,
		Committees: scenarioResultCommittees{
			MiniRoundOne: scenarioResultCommittee(committees.miniRoundOne),
			MiniRoundTwo: scenarioResultCommittee(committees.miniRoundTwo),
		},
		MiniRound1: scenarioMiniRoundOneResult{
			FinalizedNodes:       scenario.Network.RegisteredNodes,
			SubdomainFrequencies: miniRoundOneBlock.SubdomainsFrequencies,
		},
		MiniRound2: scenarioMiniRoundTwoResult{
			FinalizedNodes: scenario.Network.RegisteredNodes,
			AnswerEvidence: scenarioAnswerEvidenceResult{
				LeaderID:           miniRoundTwoBlock.AnswerEvidence.SenderID,
				Producers:          scenarioResultProducers(miniRoundTwoBlock.AnswerEvidence.Signers, rolesByValidator),
				SignaturesVerified: true,
			},
			Votes:        scenarioResultVotes(votes, rolesByValidator),
			Transactions: scenarioResultTransactions(miniRoundTwoBlock, rolesByValidator),
		},
	}
}

func scenarioResultCommittee(committee []string) []scenarioResultMember {
	members := make([]scenarioResultMember, 0, len(committee))
	for index, validatorID := range committee {
		members = append(members, scenarioResultMember{
			Role: scenarioCommitteeRole(index), ValidatorID: validatorID,
		})
	}
	return members
}

func scenarioResultProducers(signers []string, roles map[string]string) []scenarioResultMember {
	producers := make([]scenarioResultMember, 0, len(signers))
	for _, signerID := range signers {
		producers = append(producers, scenarioResultMember{Role: roles[signerID], ValidatorID: signerID})
	}
	return producers
}

func scenarioResultVotes(
	votes []*data.AnswerClassificationVote,
	roles map[string]string,
) []scenarioVoteResult {
	results := make([]scenarioVoteResult, 0, len(votes))
	for _, vote := range votes {
		results = append(results, scenarioVoteResult{
			Judge:               scenarioResultMember{Role: roles[vote.JudgeID], ValidatorID: vote.JudgeID},
			ClassificationCount: len(vote.AnswerClassifications),
			CategoryTotals:      categoryTotalsForVote(vote),
		})
	}
	return results
}

func categoryTotalsForVote(vote *data.AnswerClassificationVote) scenarioExpectedCategoryCounts {
	categoryTotals := scenarioExpectedCategoryCounts{}
	for _, answerClassification := range vote.AnswerClassifications {
		switch answerClassification.Category {
		case data.AnswerCategoryCorrect:
			categoryTotals.Correct++
		case data.AnswerCategoryHallucination:
			categoryTotals.Hallucination++
		case data.AnswerCategoryMalicious:
			categoryTotals.Malicious++
		case data.AnswerCategoryWrong:
			categoryTotals.Wrong++
		}
	}
	return categoryTotals
}

func scenarioResultTransactions(
	block *data.BlockOnChain,
	roles map[string]string,
) []scenarioTransactionResult {
	answers := scenarioResultAnswersByTxHash(block.AnswerEvidence)
	transactions := make([]scenarioTransactionResult, 0, len(block.AnswerClassifications))
	for _, transaction := range block.AnswerClassifications {
		txHash := string(transaction.TxHash)
		transactions = append(transactions, scenarioTransactionResult{
			TxHash:     txHash,
			Status:     transaction.Status,
			Candidates: scenarioResultCandidates(transaction.Counts, answers[txHash], roles),
			Groups: scenarioGroupsResult{
				Correct:       scenarioGroupRoles(transaction.Groups.Correct, roles),
				Hallucination: scenarioGroupRoles(transaction.Groups.Hallucination, roles),
				Malicious:     scenarioGroupRoles(transaction.Groups.Malicious, roles),
				Wrong:         scenarioGroupRoles(transaction.Groups.Wrong, roles),
			},
		})
	}
	return transactions
}

func scenarioResultAnswersByTxHash(
	evidence *data.AggregatedExecutionResultsMessage,
) map[string]map[string]string {
	answers := make(map[string]map[string]string)
	for signerIndex, signerID := range evidence.Signers {
		for txHash, answer := range evidence.Answers[signerIndex] {
			if answers[txHash] == nil {
				answers[txHash] = make(map[string]string)
			}
			answers[txHash][signerID] = answer.Answer
		}
	}
	return answers
}

func scenarioResultCandidates(
	counts []data.AnswerCategoryCounts,
	answersByProducer map[string]string,
	roles map[string]string,
) []scenarioCandidateResult {
	candidates := make([]scenarioCandidateResult, 0, len(counts))
	for _, candidateCounts := range counts {
		producerID := candidateCounts.CandidateID.ProducerID
		candidates = append(candidates, scenarioCandidateResult{
			ProducerRole: roles[producerID],
			ProducerID:   producerID,
			Answer:       answersByProducer[producerID],
			Counts: scenarioExpectedCategoryCounts{
				Correct:       candidateCounts.Correct,
				Hallucination: candidateCounts.Hallucination,
				Malicious:     candidateCounts.Malicious,
				Wrong:         candidateCounts.Wrong,
			},
		})
	}
	return candidates
}

func scenarioGroupRoles(candidates []data.AnswerCandidateID, roles map[string]string) []string {
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, roles[candidate.ProducerID])
	}
	return result
}
