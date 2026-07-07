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

type miniRoundTwoScenario struct {
	Name         string                       `json:"name"`
	Description  string                       `json:"description"`
	Network      scenarioNetworkFixture       `json:"network"`
	Transactions []scenarioTransactionFixture `json:"transactions"`
	Executors    []scenarioExecutorFixture    `json:"executors"`
	Judges       []scenarioJudgeFixture       `json:"judges"`
	Delivery     scenarioDeliveryFixture      `json:"delivery"`
	Expected     scenarioExpectedFixture      `json:"expected"`
	directory    string
}

type scenarioNetworkFixture struct {
	RegisteredNodes int `json:"registeredNodes"`
	CommitteeSize   int `json:"committeeSize"`
	Quorum          int `json:"quorum"`
}

type scenarioTransactionFixture struct {
	miniRoundOneTransactionFixture
	Role   string   `json:"role"`
	Labels []string `json:"labels"`
}

type scenarioExecutorFixture struct {
	Role    string            `json:"role"`
	Answers map[string]string `json:"answers"`
}

type scenarioJudgeFixture struct {
	Role            string                               `json:"role"`
	Mode            string                               `json:"mode"`
	ErrorTxHash     string                               `json:"errorTxHash,omitempty"`
	DefaultCategory data.AnswerCategory                  `json:"defaultCategory"`
	Classifications []scenarioJudgeClassificationFixture `json:"classifications,omitempty"`
}

type scenarioJudgeClassificationFixture struct {
	TxHash   string              `json:"txHash"`
	Answer   string              `json:"answer"`
	Category data.AnswerCategory `json:"category"`
}

type scenarioDeliveryFixture struct {
	ProducerOrder                   []string                 `json:"producerOrder"`
	JudgeOrder                      []string                 `json:"judgeOrder"`
	AnswerEvidenceFaults            []scenarioTransportFault `json:"answerEvidenceFaults,omitempty"`
	ClassificationVoteFaults        []scenarioTransportFault `json:"classificationVoteFaults,omitempty"`
	ClassificationCertificateFaults []scenarioTransportFault `json:"classificationCertificateFaults,omitempty"`
	TimeoutStep                     data.Step                `json:"timeoutStep,omitempty"`
}

type scenarioTransportFault struct {
	SenderRole string `json:"senderRole"`
	Type       string `json:"type"`
}

type scenarioExpectedFixture struct {
	RoundFinalized          bool                          `json:"roundFinalized"`
	FinalizedNodes          int                           `json:"finalizedNodes"`
	ErrorContains           string                        `json:"errorContains,omitempty"`
	AnswerEvidenceProducers []string                      `json:"answerEvidenceProducers"`
	ClassificationVoters    []string                      `json:"classificationVoters"`
	Transactions            []scenarioExpectedTransaction `json:"transactions"`
}

type scenarioExpectedTransaction struct {
	TxHash          string                          `json:"txHash"`
	Answers         int                             `json:"answers"`
	CountsPerAnswer scenarioExpectedCategoryCounts  `json:"countsPerAnswer"`
	CountOverrides  []scenarioExpectedCountOverride `json:"countOverrides,omitempty"`
	Groups          scenarioExpectedGroupSizes      `json:"groups"`
	Status          data.TransactionAnswerStatus    `json:"status"`
}

type scenarioExpectedCountOverride struct {
	ProducerRole string                         `json:"producerRole"`
	Counts       scenarioExpectedCategoryCounts `json:"counts"`
}

type scenarioExpectedGroupSizes struct {
	Correct       int `json:"correct"`
	Hallucination int `json:"hallucination"`
	Malicious     int `json:"malicious"`
	Wrong         int `json:"wrong"`
}

type scenarioExpectedCategoryCounts struct {
	Correct       uint64 `json:"correct"`
	Hallucination uint64 `json:"hallucination"`
	Malicious     uint64 `json:"malicious"`
	Wrong         uint64 `json:"wrong"`
}

type scenarioCommittees struct {
	miniRoundOne []string
	miniRoundTwo []string
}

func loadMiniRoundTwoScenario(t *testing.T, name string) miniRoundTwoScenario {
	t.Helper()

	directory := filepath.Join("testData", "miniround2", "scenarios", name)
	rawFixture, err := os.ReadFile(filepath.Join(directory, "scenario.json"))
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(rawFixture))
	decoder.DisallowUnknownFields()

	var scenario miniRoundTwoScenario
	require.NoError(t, decoder.Decode(&scenario))
	require.Equal(t, name, scenario.Name)
	scenario.directory = directory

	validateMiniRoundTwoScenario(t, scenario)
	return scenario
}

func validateMiniRoundTwoScenario(t *testing.T, scenario miniRoundTwoScenario) {
	t.Helper()

	require.NotEmpty(t, scenario.Description)
	require.Equal(t, 20, scenario.Network.RegisteredNodes)
	require.Equal(t, scenario.Network.RegisteredNodes/2, scenario.Network.CommitteeSize)
	require.Equal(t, consensusQuorum(scenario.Network.CommitteeSize), scenario.Network.Quorum)
	require.Len(t, scenario.Transactions, 3)
	if scenario.Expected.RoundFinalized {
		require.Equal(t, scenario.Network.RegisteredNodes, scenario.Expected.FinalizedNodes)
		require.Empty(t, scenario.Expected.ErrorContains)
		require.Len(t, scenario.Expected.AnswerEvidenceProducers, scenario.Network.Quorum)
		require.Len(t, scenario.Expected.ClassificationVoters, scenario.Network.Quorum)
		require.Len(t, scenario.Expected.Transactions, len(scenario.Transactions))
	} else {
		require.Less(t, scenario.Expected.FinalizedNodes, scenario.Network.RegisteredNodes)
		require.NotEmpty(t, scenario.Expected.ErrorContains)
	}

	txHashes := validateScenarioTransactions(t, scenario.Transactions)
	validateScenarioProfiles(t, scenario, txHashes)
	validateScenarioRoleOrder(t, scenario.Delivery.ProducerOrder, scenario.Network.CommitteeSize)
	validateScenarioRoleOrder(t, scenario.Delivery.JudgeOrder, scenario.Network.CommitteeSize)
	validateScenarioTransportFaults(t, scenario.Delivery.AnswerEvidenceFaults)
	validateScenarioTransportFaults(t, scenario.Delivery.ClassificationVoteFaults)
	validateScenarioTransportFaults(t, scenario.Delivery.ClassificationCertificateFaults)
	validateScenarioExpectations(t, scenario, txHashes)
}

func validateScenarioTransactions(
	t *testing.T,
	transactions []scenarioTransactionFixture,
) map[string]struct{} {
	t.Helper()

	txHashes := make(map[string]struct{}, len(transactions))
	for _, transaction := range transactions {
		require.NotEmpty(t, transaction.Role)
		require.NotEmpty(t, transaction.TxHash)
		require.NotEmpty(t, transaction.Prompt)
		_, duplicated := txHashes[transaction.TxHash]
		require.Falsef(t, duplicated, "duplicated transaction %s", transaction.TxHash)
		txHashes[transaction.TxHash] = struct{}{}

		validateScenarioTransactionLabels(t, transaction)
	}
	return txHashes
}

func validateScenarioTransactionLabels(t *testing.T, transaction scenarioTransactionFixture) {
	t.Helper()

	require.Len(t, transaction.Labels, 6)
	seenLabels := make(map[string]struct{}, len(transaction.Labels))
	for _, label := range transaction.Labels {
		_, valid := possibleSubDomains[label]
		require.Truef(t, valid, "invalid label %q for transaction %s", label, transaction.TxHash)
		_, duplicated := seenLabels[label]
		require.Falsef(t, duplicated, "duplicated label %q for transaction %s", label, transaction.TxHash)
		seenLabels[label] = struct{}{}
	}
}

func validateScenarioExpectations(
	t *testing.T,
	scenario miniRoundTwoScenario,
	txHashes map[string]struct{},
) {
	t.Helper()
	if !scenario.Expected.RoundFinalized {
		require.Empty(t, scenario.Expected.Transactions)
		return
	}

	expectedTxHashes := make(map[string]struct{}, len(scenario.Expected.Transactions))
	evidenceRoles := setFromStrings(scenario.Expected.AnswerEvidenceProducers)
	for _, expected := range scenario.Expected.Transactions {
		_, exists := txHashes[expected.TxHash]
		require.Truef(t, exists, "expected result references unknown transaction %s", expected.TxHash)
		_, duplicated := expectedTxHashes[expected.TxHash]
		require.Falsef(t, duplicated, "duplicated expected transaction %s", expected.TxHash)
		expectedTxHashes[expected.TxHash] = struct{}{}
		require.Equal(t, scenario.Network.Quorum, expected.Answers)
		require.Equal(
			t,
			expected.Answers,
			expected.Groups.Correct+expected.Groups.Hallucination+expected.Groups.Malicious+expected.Groups.Wrong,
		)
		require.True(t, expected.Status.IsValid())
		validateExpectedCategoryCounts(t, expected.CountsPerAnswer, scenario.Network.Quorum)
		validateExpectedCountOverrides(t, expected.CountOverrides, evidenceRoles, scenario.Network.Quorum)
	}
}

func validateExpectedCountOverrides(
	t *testing.T,
	overrides []scenarioExpectedCountOverride,
	evidenceRoles map[string]struct{},
	quorum int,
) {
	t.Helper()

	overriddenRoles := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		_, isEvidenceProducer := evidenceRoles[override.ProducerRole]
		require.Truef(t, isEvidenceProducer, "count override references non-producer role %s", override.ProducerRole)
		_, duplicated := overriddenRoles[override.ProducerRole]
		require.Falsef(t, duplicated, "duplicated count override for role %s", override.ProducerRole)
		overriddenRoles[override.ProducerRole] = struct{}{}
		validateExpectedCategoryCounts(t, override.Counts, quorum)
	}
}

func validateExpectedCategoryCounts(
	t *testing.T,
	counts scenarioExpectedCategoryCounts,
	quorum int,
) {
	t.Helper()
	require.Equal(
		t,
		uint64(quorum),
		counts.Correct+counts.Hallucination+counts.Malicious+counts.Wrong,
	)
}

func validateScenarioProfiles(t *testing.T, scenario miniRoundTwoScenario, txHashes map[string]struct{}) {
	t.Helper()

	validateExecutorProfiles(t, scenario.Executors, txHashes)
	validateJudgeProfiles(t, scenario.Judges, txHashes)
}

func validateExecutorProfiles(
	t *testing.T,
	executors []scenarioExecutorFixture,
	txHashes map[string]struct{},
) {
	t.Helper()

	executorRoles := make(map[string]struct{}, len(executors))
	for _, executor := range executors {
		require.NotEmpty(t, executor.Role)
		_, duplicated := executorRoles[executor.Role]
		require.Falsef(t, duplicated, "duplicated executor role %s", executor.Role)
		executorRoles[executor.Role] = struct{}{}

		for txHash := range txHashes {
			answer, exists := executor.Answers[txHash]
			require.Truef(t, exists, "executor %s has no answer for %s", executor.Role, txHash)
			require.NotEmpty(t, answer)
		}
	}
	_, hasDefaultExecutor := executorRoles["default"]
	require.True(t, hasDefaultExecutor)
}

func validateJudgeProfiles(
	t *testing.T,
	judges []scenarioJudgeFixture,
	txHashes map[string]struct{},
) {
	t.Helper()

	judgeRoles := make(map[string]struct{}, len(judges))
	for _, judge := range judges {
		require.NotEmpty(t, judge.Role)
		_, duplicated := judgeRoles[judge.Role]
		require.Falsef(t, duplicated, "duplicated judge role %s", judge.Role)
		judgeRoles[judge.Role] = struct{}{}
		validateJudgeMode(t, judge, txHashes)
		validateJudgeClassifications(t, judge, txHashes)
	}
	_, hasDefaultJudge := judgeRoles["default"]
	require.True(t, hasDefaultJudge)
}

func validateJudgeMode(
	t *testing.T,
	judge scenarioJudgeFixture,
	txHashes map[string]struct{},
) {
	t.Helper()

	switch judge.Mode {
	case "valid":
		require.Empty(t, judge.ErrorTxHash)
	case "execution_error",
		"malformed_json",
		"missing_classification",
		"unknown_answer",
		"invalid_category":
		_, exists := txHashes[judge.ErrorTxHash]
		require.Truef(t, exists, "judge %s has unknown errorTxHash %s", judge.Role, judge.ErrorTxHash)
	default:
		require.Failf(t, "unsupported judge mode", "judge %s uses unsupported mode %s", judge.Role, judge.Mode)
	}
	require.True(t, judge.DefaultCategory.IsValid())
}

func validateJudgeClassifications(
	t *testing.T,
	judge scenarioJudgeFixture,
	txHashes map[string]struct{},
) {
	t.Helper()

	for _, classificationFixture := range judge.Classifications {
		_, exists := txHashes[classificationFixture.TxHash]
		require.Truef(t, exists, "judge %s references unknown transaction %s", judge.Role, classificationFixture.TxHash)
		require.NotEmpty(t, classificationFixture.Answer)
		require.True(t, classificationFixture.Category.IsValid())
	}
}

func validateScenarioRoleOrder(t *testing.T, roles []string, committeeSize int) {
	t.Helper()
	require.Len(t, roles, committeeSize)
	require.Equal(t, "leader", roles[0])

	// Delivery order is a full committee-role permutation. Fault scenarios can
	// move failing validators after the first valid quorum without removing them
	// from the explicit transport plan.
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		_, duplicated := seen[role]
		require.False(t, duplicated)
		seen[role] = struct{}{}
	}
	for index := 0; index < committeeSize; index++ {
		_, exists := seen[scenarioCommitteeRole(index)]
		require.True(t, exists)
	}
}

func validateScenarioTransportFaults(t *testing.T, faults []scenarioTransportFault) {
	t.Helper()

	seen := make(map[string]struct{}, len(faults))
	for _, fault := range faults {
		require.NotEmpty(t, fault.SenderRole)
		_, duplicated := seen[fault.SenderRole]
		require.False(t, duplicated)
		seen[fault.SenderRole] = struct{}{}

		switch fault.Type {
		case "drop",
			"invalid_signature",
			"mutate_evidence_hash",
			"omit_certificate_vote",
			"reorder_certificate_votes":
		default:
			require.Failf(t, "unsupported transport fault", "sender %s uses unsupported fault %s", fault.SenderRole, fault.Type)
		}
	}
}

func setFromStrings(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
