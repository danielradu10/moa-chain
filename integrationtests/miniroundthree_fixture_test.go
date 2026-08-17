package integrationtests

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

type miniRoundThreeScenario struct {
	Name         string                       `json:"name"`
	Description  string                       `json:"description"`
	Network      scenarioNetworkFixture       `json:"network"`
	Transactions []scenarioTransactionFixture `json:"transactions"`
	Executors    []scenarioExecutorFixture    `json:"executors"`
	Judges       []scenarioJudgeFixture       `json:"judges"`
	Synthesizer  scenarioSynthesizerFixture   `json:"synthesizer"`
	Evaluators   []scenarioEvaluatorFixture   `json:"evaluators"`
	Delivery     mr3ScenarioDeliveryFixture   `json:"delivery"`
	Expected     mr3ScenarioExpectedFixture   `json:"expected"`
	directory    string
}

type scenarioSynthesizerFixture struct {
	Answers map[string]string `json:"answers"`
}

type scenarioEvaluatorFixture struct {
	Role        string `json:"role"`
	Mode        string `json:"mode"`
	ErrorTxHash string `json:"errorTxHash,omitempty"`
}

type mr3ScenarioDeliveryFixture struct {
	ProducerOrder                   []string                     `json:"producerOrder"`
	JudgeOrder                      []string                     `json:"judgeOrder"`
	AnswerEvidenceFaults            []scenarioTransportFault     `json:"answerEvidenceFaults,omitempty"`
	ClassificationVoteFaults        []scenarioTransportFault     `json:"classificationVoteFaults,omitempty"`
	ClassificationCertificateFaults []scenarioTransportFault     `json:"classificationCertificateFaults,omitempty"`
	SynthesizerVoteOrder            []string                     `json:"synthesizerVoteOrder"`
	SynthesisVoteFaults             []scenarioSynthesisVoteFault `json:"synthesisVoteFaults,omitempty"`
}

type scenarioSynthesisVoteFault struct {
	SenderRole string `json:"senderRole"`
	Type       string `json:"type"`
}

type mr3ScenarioExpectedFixture struct {
	RoundFinalized          bool                          `json:"roundFinalized"`
	FinalizedNodes          int                           `json:"finalizedNodes"`
	ErrorContains           string                        `json:"errorContains,omitempty"`
	AnswerEvidenceProducers []string                      `json:"answerEvidenceProducers"`
	ClassificationVoters    []string                      `json:"classificationVoters"`
	Transactions            []scenarioExpectedTransaction `json:"transactions"`
	MR3RoundFinalized       bool                          `json:"mr3RoundFinalized"`
	MR3FinalizedNodes       int                           `json:"mr3FinalizedNodes"`
	MR3ErrorContains        string                        `json:"mr3ErrorContains,omitempty"`
	SynthesisVoters         []string                      `json:"synthesisVoters"`
	FinalAnswers            []scenarioExpectedFinalAnswer `json:"finalAnswers"`
}

type scenarioExpectedFinalAnswer struct {
	TxHash string                `json:"txHash"`
	Status data.FinalAnswerStatus `json:"status"`
	Answer string                `json:"answer,omitempty"`
}

func loadMiniRoundThreeScenario(t *testing.T, name string) miniRoundThreeScenario {
	t.Helper()

	directory := filepath.Join("testData", "miniround3", "scenarios", name)
	rawFixture, err := os.ReadFile(filepath.Join(directory, "scenario.json"))
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(rawFixture))
	decoder.DisallowUnknownFields()

	var scenario miniRoundThreeScenario
	require.NoError(t, decoder.Decode(&scenario))
	require.Equal(t, name, scenario.Name)
	scenario.directory = directory

	validateMiniRoundThreeScenario(t, scenario)
	return scenario
}

func validateMiniRoundThreeScenario(t *testing.T, scenario miniRoundThreeScenario) {
	t.Helper()

	require.NotEmpty(t, scenario.Description)
	require.Equal(t, 20, scenario.Network.RegisteredNodes)
	require.Equal(t, scenario.Network.RegisteredNodes/2, scenario.Network.CommitteeSize)
	require.Equal(t, consensusQuorum(scenario.Network.CommitteeSize), scenario.Network.Quorum)
	require.Len(t, scenario.Transactions, 3)

	// MR2 always finalizes in MR3 scenarios.
	require.True(t, scenario.Expected.RoundFinalized)
	require.Equal(t, scenario.Network.RegisteredNodes, scenario.Expected.FinalizedNodes)
	require.Empty(t, scenario.Expected.ErrorContains)
	require.Len(t, scenario.Expected.AnswerEvidenceProducers, scenario.Network.CommitteeSize)
	require.Len(t, scenario.Expected.Transactions, len(scenario.Transactions))

	if scenario.Expected.MR3RoundFinalized {
		require.Equal(t, scenario.Network.RegisteredNodes, scenario.Expected.MR3FinalizedNodes)
		require.Empty(t, scenario.Expected.MR3ErrorContains)
		require.Len(t, scenario.Expected.FinalAnswers, len(scenario.Transactions))
	} else {
		require.Less(t, scenario.Expected.MR3FinalizedNodes, scenario.Network.RegisteredNodes)
		require.NotEmpty(t, scenario.Expected.MR3ErrorContains)
		require.Empty(t, scenario.Expected.FinalAnswers)
	}

	txHashes := validateScenarioTransactions(t, scenario.Transactions)
	validateExecutorProfiles(t, scenario.Executors, txHashes)
	validateJudgeProfiles(t, scenario.Judges, txHashes)
	validateMR3EvaluatorProfiles(t, scenario.Evaluators)
	validateScenarioRoleOrder(t, scenario.Delivery.ProducerOrder, scenario.Network.CommitteeSize)
	validateScenarioRoleOrder(t, scenario.Delivery.JudgeOrder, scenario.Network.CommitteeSize)
	validateScenarioRoleOrder(t, scenario.Delivery.SynthesizerVoteOrder, scenario.Network.CommitteeSize)
	validateScenarioTransportFaults(t, scenario.Delivery.AnswerEvidenceFaults)
	validateScenarioTransportFaults(t, scenario.Delivery.ClassificationVoteFaults)
	validateScenarioTransportFaults(t, scenario.Delivery.ClassificationCertificateFaults)
	validateMR3SynthesisVoteFaults(t, scenario.Delivery.SynthesisVoteFaults)
}

func validateMR3EvaluatorProfiles(t *testing.T, evaluators []scenarioEvaluatorFixture) {
	t.Helper()

	roles := make(map[string]struct{}, len(evaluators))
	for _, ev := range evaluators {
		require.NotEmpty(t, ev.Role)
		_, dup := roles[ev.Role]
		require.Falsef(t, dup, "duplicated evaluator role %s", ev.Role)
		roles[ev.Role] = struct{}{}

		switch ev.Mode {
		case "approve_all", "reject_all", "execution_error":
		default:
			require.Failf(t, "unsupported evaluator mode", "evaluator %s uses mode %s", ev.Role, ev.Mode)
		}
	}
	_, hasDefault := roles["default"]
	require.True(t, hasDefault, "evaluators must include a default role")
}

func validateMR3SynthesisVoteFaults(t *testing.T, faults []scenarioSynthesisVoteFault) {
	t.Helper()

	seen := make(map[string]struct{}, len(faults))
	for _, fault := range faults {
		require.NotEmpty(t, fault.SenderRole)
		_, dup := seen[fault.SenderRole]
		require.False(t, dup)
		seen[fault.SenderRole] = struct{}{}

		switch fault.Type {
		case "drop", "invalid_signature", "wrong_synthesis_hash":
		default:
			require.Failf(t, "unsupported synthesis vote fault", "sender %s uses %s", fault.SenderRole, fault.Type)
		}
	}
}
