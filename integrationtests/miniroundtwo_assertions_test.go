package integrationtests

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func requireScenarioFinalizedState(
	t *testing.T,
	scenario miniRoundTwoScenario,
	nodes []*integrationTestNode,
	committees scenarioCommittees,
	expectedFrequencies data.SubdomainsFrequency,
	firstMiniRoundOneBlock *data.BlockOnChain,
	firstMiniRoundTwoBlock *data.BlockOnChain,
	classificationVotes []*data.AnswerClassificationVote,
	miniRoundOneKey data.RoundKey,
	miniRoundTwoKey data.RoundKey,
) {
	t.Helper()

	require.Equal(t, expectedFrequencies, firstMiniRoundOneBlock.SubdomainsFrequencies)
	require.NotNil(t, firstMiniRoundTwoBlock.AnswerEvidence)
	require.Len(t, firstMiniRoundTwoBlock.AggregatedExecutionResults, len(scenario.Transactions))
	require.Len(t, firstMiniRoundTwoBlock.AnswerClassifications, len(scenario.Transactions))

	expectedProducers := scenarioIDsForRoles(t, committees.miniRoundTwo, scenario.Expected.AnswerEvidenceProducers)
	sort.Strings(expectedProducers)
	require.Equal(t, expectedProducers, firstMiniRoundTwoBlock.AnswerEvidence.Signers)
	require.Len(t, classificationVotes, scenario.Network.Quorum)

	expectedVoters := scenarioIDsForRoles(t, committees.miniRoundTwo, scenario.Expected.ClassificationVoters)
	sort.Strings(expectedVoters)
	actualVoters := make([]string, 0, len(classificationVotes))
	rolesByValidator := scenarioRolesByValidator(committees.miniRoundTwo)
	for _, vote := range classificationVotes {
		actualVoters = append(actualVoters, vote.JudgeID)
		require.Len(t, vote.AnswerClassifications, scenario.Network.Quorum*len(scenario.Transactions))
		requireScenarioVoteMatchesJudgeProfile(t, scenario, vote, rolesByValidator, firstMiniRoundTwoBlock.AnswerEvidence)
	}
	require.Equal(t, expectedVoters, actualVoters)

	requireScenarioTransactions(
		t,
		scenario,
		firstMiniRoundTwoBlock,
		expectedProducers,
		rolesByValidator,
	)

	requireAllScenarioNodesFinalizedSameBlocks(t, nodes, firstMiniRoundOneBlock, firstMiniRoundTwoBlock, miniRoundOneKey, miniRoundTwoKey)
}

func requireScenarioVoteMatchesJudgeProfile(
	t *testing.T,
	scenario miniRoundTwoScenario,
	vote *data.AnswerClassificationVote,
	rolesByValidator map[string]string,
	evidence *data.AggregatedExecutionResultsMessage,
) {
	t.Helper()

	for _, answerClassification := range vote.AnswerClassifications {
		require.Equal(
			t,
			expectedScenarioCategory(
				t,
				scenario,
				rolesByValidator[vote.JudgeID],
				answerClassification.CandidateID,
				evidence,
			),
			answerClassification.Category,
		)
	}
}

func requireAllScenarioNodesFinalizedSameBlocks(
	t *testing.T,
	nodes []*integrationTestNode,
	firstMiniRoundOneBlock *data.BlockOnChain,
	firstMiniRoundTwoBlock *data.BlockOnChain,
	miniRoundOneKey data.RoundKey,
	miniRoundTwoKey data.RoundKey,
) {
	t.Helper()

	for _, node := range nodes {
		miniRoundOneBlock, err := node.blockFinalizer.GetFinalizedBlockInMROne(miniRoundOneKey)
		require.NoError(t, err)
		require.Equal(t, firstMiniRoundOneBlock.Block.Header.HeaderHash, miniRoundOneBlock.Block.Header.HeaderHash)
		require.Equal(t, firstMiniRoundOneBlock.SubdomainsFrequencies, miniRoundOneBlock.SubdomainsFrequencies)

		miniRoundTwoBlock, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey)
		require.NoError(t, err)
		require.Equal(t, firstMiniRoundTwoBlock.AggregatedExecutionResults, miniRoundTwoBlock.AggregatedExecutionResults)
		require.Equal(t, firstMiniRoundTwoBlock.AnswerEvidence, miniRoundTwoBlock.AnswerEvidence)
		require.Equal(t, firstMiniRoundTwoBlock.AnswerClassifications, miniRoundTwoBlock.AnswerClassifications)
	}
}

func requireScenarioTransactions(
	t *testing.T,
	scenario miniRoundTwoScenario,
	block *data.BlockOnChain,
	expectedProducers []string,
	rolesByValidator map[string]string,
) {
	t.Helper()
	expectedByTxHash := make(map[string]scenarioExpectedTransaction, len(scenario.Expected.Transactions))
	for _, expected := range scenario.Expected.Transactions {
		expectedByTxHash[expected.TxHash] = expected
	}

	for _, transaction := range block.AnswerClassifications {
		txHash := string(transaction.TxHash)
		expected, exists := expectedByTxHash[txHash]
		require.Truef(t, exists, "unexpected finalized transaction %s", txHash)
		requireScenarioTransaction(t, transaction, expected, expectedProducers, rolesByValidator)
	}

	requireScenarioAnswerEvidence(t, scenario, block, rolesByValidator)
}

func requireScenarioTransaction(
	t *testing.T,
	transaction data.TransactionAnswerClassification,
	expected scenarioExpectedTransaction,
	expectedProducers []string,
	rolesByValidator map[string]string,
) {
	t.Helper()

	txHash := string(transaction.TxHash)
	require.Len(t, transaction.Counts, expected.Answers)
	require.Len(t, transaction.Groups.Correct, expected.Groups.Correct)
	require.Len(t, transaction.Groups.Hallucination, expected.Groups.Hallucination)
	require.Len(t, transaction.Groups.Malicious, expected.Groups.Malicious)
	require.Len(t, transaction.Groups.Wrong, expected.Groups.Wrong)
	require.Equal(t, expected.Status, transaction.Status)

	for _, counts := range transaction.Counts {
		require.Equal(t, txHash, string(counts.CandidateID.TxHash))
		require.Contains(t, expectedProducers, counts.CandidateID.ProducerID)
		expectedCounts := expectedCountsForProducerRole(
			expected,
			rolesByValidator[counts.CandidateID.ProducerID],
		)
		require.Equal(t, expectedCounts.Correct, counts.Correct)
		require.Equal(t, expectedCounts.Hallucination, counts.Hallucination)
		require.Equal(t, expectedCounts.Malicious, counts.Malicious)
		require.Equal(t, expectedCounts.Wrong, counts.Wrong)
	}
}

func requireScenarioAnswerEvidence(
	t *testing.T,
	scenario miniRoundTwoScenario,
	block *data.BlockOnChain,
	rolesByValidator map[string]string,
) {
	t.Helper()

	for signerIndex, signerID := range block.AnswerEvidence.Signers {
		role := rolesByValidator[signerID]
		executor := scenarioExecutorForRole(t, scenario.Executors, role)
		for txHash, answer := range block.AnswerEvidence.Answers[signerIndex] {
			require.Equal(t, executor.Answers[txHash], answer.Answer)
		}
	}
}

func expectedCountsForProducerRole(
	expected scenarioExpectedTransaction,
	producerRole string,
) scenarioExpectedCategoryCounts {
	for _, override := range expected.CountOverrides {
		if override.ProducerRole == producerRole {
			return override.Counts
		}
	}
	return expected.CountsPerAnswer
}

func expectedScenarioCategory(
	t *testing.T,
	scenario miniRoundTwoScenario,
	judgeRole string,
	candidate data.AnswerCandidateID,
	evidence *data.AggregatedExecutionResultsMessage,
) data.AnswerCategory {
	t.Helper()
	judge := scenarioJudgeForRole(t, scenario.Judges, judgeRole)
	answer := scenarioAnswerForCandidate(t, evidence, candidate)
	return scenarioCategoryForCandidate(string(candidate.TxHash), answer, judge)
}

func scenarioAnswerForCandidate(
	t *testing.T,
	evidence *data.AggregatedExecutionResultsMessage,
	candidate data.AnswerCandidateID,
) string {
	t.Helper()
	for signerIndex, signerID := range evidence.Signers {
		if signerID != candidate.ProducerID {
			continue
		}
		answer, exists := evidence.Answers[signerIndex][string(candidate.TxHash)]
		require.True(t, exists)
		return answer.Answer
	}
	require.FailNow(t, "candidate producer missing from answer evidence", candidate.ProducerID)
	return ""
}
