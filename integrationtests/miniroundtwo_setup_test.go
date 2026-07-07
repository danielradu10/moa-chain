package integrationtests

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

type scenarioValidators []*validators.Validator

func generateScenarioKeys(t *testing.T, count int) ([][]byte, [][]byte) {
	t.Helper()
	publicKeys := make([][]byte, 0, count)
	privateKeys := make([][]byte, 0, count)
	for index := 0; index < count; index++ {
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		publicKeys = append(publicKeys, publicKey)
		privateKeys = append(privateKeys, privateKey)
	}
	return publicKeys, privateKeys
}

func createScenarioValidators(publicKeys [][]byte) scenarioValidators {
	result := make(scenarioValidators, 0, len(publicKeys))
	for index, publicKey := range publicKeys {
		validator := validators.NewValidator(fmt.Sprintf("validator-%d", index+1), publicKey, 1)
		validator.SetSubdomainScores(defaultIntegrationTestSubdomainScores())
		result = append(result, validator)
	}
	return result
}

func scenarioTransactions(scenario miniRoundTwoScenario) []data.Transaction {
	transactions := make([]data.Transaction, 0, len(scenario.Transactions))
	for _, fixture := range scenario.Transactions {
		transactions = append(transactions, createTransactionFromFixture(fixture.miniRoundOneTransactionFixture))
	}
	return transactions
}

func scenarioSubdomainFrequencies(scenario miniRoundTwoScenario, votes uint64) data.SubdomainsFrequency {
	frequencies := make(data.SubdomainsFrequency)
	for _, transaction := range scenario.Transactions {
		for _, label := range transaction.Labels {
			frequencies[label] += votes
		}
	}
	return frequencies
}

func selectScenarioCommittees(
	t *testing.T,
	registeredValidators scenarioValidators,
	frequencies data.SubdomainsFrequency,
) scenarioCommittees {
	t.Helper()

	orderedValidators := orderedScenarioValidators(registeredValidators)
	blockchainState := &testscommon.BlockchainStateStub{CurrentBlockHeaderValue: currentIntegrationTestHeader()}
	miniRoundOneKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
	miniRoundTwoKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}

	miniRoundOne := selectMiniRoundOneCommittee(t, blockchainState, orderedValidators, miniRoundOneKey)
	miniRoundTwo := selectMiniRoundTwoCommittee(t, blockchainState, orderedValidators, miniRoundTwoKey, frequencies)

	return scenarioCommittees{miniRoundOne: miniRoundOne, miniRoundTwo: miniRoundTwo}
}

func orderedScenarioValidators(registeredValidators scenarioValidators) scenarioValidators {
	// ValidatorRegistry canonicalizes validators by public ID before invoking the
	// selector. Preselection must use that same ordering or fixture roles would
	// target a different committee than the protocol actually selects.
	orderedValidators := append(scenarioValidators(nil), registeredValidators...)
	sort.Slice(orderedValidators, func(left, right int) bool {
		return orderedValidators[left].PublicID() < orderedValidators[right].PublicID()
	})
	return orderedValidators
}

func selectMiniRoundOneCommittee(
	t *testing.T,
	blockchainState *testscommon.BlockchainStateStub,
	orderedValidators scenarioValidators,
	roundKey data.RoundKey,
) []string {
	t.Helper()

	selector := validators.NewConsensusSelector()
	committee, err := selector.SelectConsensusGroupMiniRoundOne(blockchainState, orderedValidators, roundKey)
	require.NoError(t, err)
	return committee
}

func selectMiniRoundTwoCommittee(
	t *testing.T,
	blockchainState *testscommon.BlockchainStateStub,
	orderedValidators scenarioValidators,
	roundKey data.RoundKey,
	frequencies data.SubdomainsFrequency,
) []string {
	t.Helper()

	selector := validators.NewConsensusSelector()
	committee, err := selector.SelectConsensusGroupMiniRoundTwo(
		blockchainState,
		orderedValidators,
		roundKey,
		frequencies,
	)
	require.NoError(t, err)
	return committee
}

func scenarioCommitteeRole(index int) string {
	if index == 0 {
		return "leader"
	}
	return fmt.Sprintf("member-%d", index)
}

func scenarioRolesByValidator(committee []string) map[string]string {
	roles := make(map[string]string, len(committee))
	for index, validatorID := range committee {
		roles[validatorID] = scenarioCommitteeRole(index)
	}
	return roles
}

func scenarioIDsForRoles(t *testing.T, committee []string, roles []string) []string {
	t.Helper()
	ids := make([]string, 0, len(roles))
	for _, role := range roles {
		if role == "leader" {
			ids = append(ids, committee[0])
			continue
		}

		var memberIndex int
		_, err := fmt.Sscanf(role, "member-%d", &memberIndex)
		require.NoError(t, err)
		require.GreaterOrEqual(t, memberIndex, 1)
		require.Less(t, memberIndex, len(committee))
		ids = append(ids, committee[memberIndex])
	}
	return ids
}

func makeScenarioInboxes(count int) []chan data.RoundEvent {
	inboxes := make([]chan data.RoundEvent, 0, count)
	for index := 0; index < count; index++ {
		inboxes = append(inboxes, make(chan data.RoundEvent, 256))
	}
	return inboxes
}
