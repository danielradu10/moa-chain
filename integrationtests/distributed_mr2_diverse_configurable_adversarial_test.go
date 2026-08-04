//go:build integration

package integrationtests

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/agent/httpclient"
	"moa-chain/consensus/miniround2/classification"
	"moa-chain/data"
)

// configurableAdversarialCandidate records canonical identity, not just a
// category count, so offline analysis can distinguish false acceptance of an
// adversarial producer from false rejection of a legitimate producer.
type configurableAdversarialCandidate struct {
	ProducerID string `json:"producer_id"`
	AnswerHash string `json:"answer_hash"`
}

type configurableAdversarialTxResult struct {
	TxHash        string `json:"tx_hash"`
	Status        string `json:"status"`
	Correct       int    `json:"correct"`
	Wrong         int    `json:"wrong"`
	Hallucination int    `json:"hallucination"`
	Malicious     int    `json:"malicious"`

	CorrectCandidates       []configurableAdversarialCandidate `json:"correct_candidates"`
	WrongCandidates         []configurableAdversarialCandidate `json:"wrong_candidates"`
	HallucinationCandidates []configurableAdversarialCandidate `json:"hallucination_candidates"`
	MaliciousCandidates     []configurableAdversarialCandidate `json:"malicious_candidates"`
}

type configurableAdversarialRunResult struct {
	Timestamp                  string                            `json:"timestamp"`
	Group                      string                            `json:"group"`
	RoundNumber                uint64                            `json:"round_number"`
	NumValidators              int                               `json:"num_validators"`
	BadProducerCount           int                               `json:"bad_producer_count"`
	BadProducerIDs             []string                          `json:"bad_producer_ids"`
	DurationSeconds            float64                           `json:"duration_seconds"`
	Finalized                  bool                              `json:"finalized"`
	AllNodesEqual              bool                              `json:"all_nodes_equal"`
	ProtocolErrors             []string                          `json:"protocol_errors,omitempty"`
	ClassificationGracePeriod  string                            `json:"classification_grace_period"`
	PromptVersion              string                            `json:"prompt_version"`
	PromptHash                 string                            `json:"prompt_hash"`
	CertificateVoteCount       int                               `json:"certificate_vote_count,omitempty"`
	CertificateLeaderID        string                            `json:"certificate_leader_id,omitempty"`
	CertificateJudgeIDs        []string                          `json:"certificate_judge_ids,omitempty"`
	ByzantineCertificateJudges []string                          `json:"byzantine_certificate_judges,omitempty"`
	MockedJudgeCalls           []configurableMockedJudgeCall     `json:"mocked_judge_calls,omitempty"`
	TxResults                  []configurableAdversarialTxResult `json:"tx_results,omitempty"`
}

type configurableMockedClassification struct {
	CandidateID string `json:"candidateId"`
	Category    string `json:"category"`
}

type configurableMockedJudgeCall struct {
	JudgeID         string                             `json:"judge_id"`
	TransactionHash string                             `json:"transaction_hash"`
	Classifications []configurableMockedClassification `json:"classifications"`
}

type configurableMockRecorder struct {
	mu    sync.Mutex
	calls []configurableMockedJudgeCall
}

func (recorder *configurableMockRecorder) append(call configurableMockedJudgeCall) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.calls = append(recorder.calls, call)
}

func (recorder *configurableMockRecorder) snapshot() []configurableMockedJudgeCall {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]configurableMockedJudgeCall(nil), recorder.calls...)
}

type configurableByzantineJudge struct {
	judgeID  string
	badTexts map[string]struct{}
	recorder *configurableMockRecorder
}

// JudgeTransactionAnswers deliberately protects all colluding adversarial
// producers and rejects every legitimate producer. It never calls an LLM.
func (judge *configurableByzantineJudge) JudgeTransactionAnswers(request agent.AnswerJudgeRequest) (string, error) {
	var input struct {
		TransactionHash string `json:"transactionHash"`
		Candidates      []struct {
			CandidateID string `json:"candidateId"`
			Answer      string `json:"answer"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(request.UserPrompt), &input); err != nil {
		return "", err
	}

	call := configurableMockedJudgeCall{
		JudgeID:         judge.judgeID,
		TransactionHash: input.TransactionHash,
		Classifications: make([]configurableMockedClassification, 0, len(input.Candidates)),
	}
	for _, candidate := range input.Candidates {
		category := string(data.AnswerCategoryWrong)
		if _, isByzantineAnswer := judge.badTexts[candidate.Answer]; isByzantineAnswer {
			category = string(data.AnswerCategoryCorrect)
		}
		call.Classifications = append(call.Classifications, configurableMockedClassification{
			CandidateID: candidate.CandidateID,
			Category:    category,
		})
	}
	judge.recorder.append(call)

	output := struct {
		Classifications []configurableMockedClassification `json:"classifications"`
	}{Classifications: call.Classifications}
	raw, err := json.Marshal(output)
	return string(raw), err
}

func configuredAdversarialBadProducerCount(t *testing.T) int {
	t.Helper()
	raw := os.Getenv("MOA_BAD_PRODUCERS")
	require.NotEmpty(t, raw, "MOA_BAD_PRODUCERS must be set to 1, 2, or 3")
	count, err := strconv.Atoi(raw)
	require.NoError(t, err, "MOA_BAD_PRODUCERS must be an integer")
	require.GreaterOrEqual(t, count, 1, "MOA_BAD_PRODUCERS must be at least 1")
	require.LessOrEqual(t, count, 3, "MOA_BAD_PRODUCERS must be at most 3")
	return count
}

func configuredClassificationGracePeriod(t *testing.T) time.Duration {
	t.Helper()
	raw := os.Getenv("MOA_CLASSIFICATION_GRACE_PERIOD")
	require.NotEmpty(t, raw, "MOA_CLASSIFICATION_GRACE_PERIOD must be set, for example 180s")
	duration, err := time.ParseDuration(raw)
	require.NoError(t, err, "invalid MOA_CLASSIFICATION_GRACE_PERIOD")
	require.GreaterOrEqual(t, duration, time.Duration(0))
	return duration
}

func configurableAdversarialBadProducerIDs(count int) []string {
	ids := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		ids = append(ids, fmt.Sprintf("validator-%d", i))
	}
	return ids
}

func configurableAdversarialCandidates(ids []data.AnswerCandidateID) []configurableAdversarialCandidate {
	out := make([]configurableAdversarialCandidate, 0, len(ids))
	for _, id := range ids {
		out = append(out, configurableAdversarialCandidate{
			ProducerID: id.ProducerID,
			AnswerHash: hex.EncodeToString(id.AnswerHash),
		})
	}
	return out
}

func configurableAdversarialTxResults(block *data.BlockOnChain) []configurableAdversarialTxResult {
	out := make([]configurableAdversarialTxResult, 0, len(block.AnswerClassifications))
	for _, tx := range block.AnswerClassifications {
		out = append(out, configurableAdversarialTxResult{
			TxHash:                  string(tx.TxHash),
			Status:                  string(tx.Status),
			Correct:                 len(tx.Groups.Correct),
			Wrong:                   len(tx.Groups.Wrong),
			Hallucination:           len(tx.Groups.Hallucination),
			Malicious:               len(tx.Groups.Malicious),
			CorrectCandidates:       configurableAdversarialCandidates(tx.Groups.Correct),
			WrongCandidates:         configurableAdversarialCandidates(tx.Groups.Wrong),
			HallucinationCandidates: configurableAdversarialCandidates(tx.Groups.Hallucination),
			MaliciousCandidates:     configurableAdversarialCandidates(tx.Groups.Malicious),
		})
	}
	return out
}

func saveConfigurableAdversarialResult(t *testing.T, result configurableAdversarialRunResult) {
	t.Helper()
	dir := os.Getenv("MOA_TEST_RESULTS_DIR")
	if dir == "" {
		return
	}
	filename := fmt.Sprintf("distributed-mr2-diverse-adversarial-%s-q%d-%s-%s.json",
		result.Group, result.BadProducerCount, result.PromptVersion,
		time.Now().UTC().Format("20060102T150405"))
	path := filepath.Join(dir, filename)
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Logf("[results] failed to marshal JSON: %v", err)
		return
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Logf("[results] failed to write %s: %v", path, err)
		return
	}
	t.Logf("[results] saved to %s", path)
}

// runConfigurableAdversarialMR2Round is intentionally observational about
// semantics. It fails only when all nodes do not finalize the same canonical
// MR2 result. False accepts, false rejects, category choices, and MR3 readiness
// are preserved in JSON for analysis rather than asserted in the test.
func runConfigurableAdversarialMR2Round(
	t *testing.T,
	cfg clusterConfig,
	group string,
	badAnswers map[string]string,
	badProducerCount int,
	classificationGracePeriod time.Duration,
	roundNumber uint64,
) {
	t.Helper()

	n := len(cfg.Agents)
	publicKeys, privateKeys := generateScenarioKeys(t, n)
	registeredValidators := createScenarioValidators(publicKeys)
	transactions := realAgentMR2Transactions()

	inboxes := make([]chan data.RoundEvent, n)
	for i := range inboxes {
		inboxes[i] = make(chan data.RoundEvent, 256)
	}

	nodes := make([]*integrationTestNode, n)
	mockRecorder := &configurableMockRecorder{}
	badTexts := make(map[string]struct{}, len(badAnswers))
	for _, answer := range badAnswers {
		badTexts[answer] = struct{}{}
	}
	for i, entry := range cfg.Agents {
		answers := mr2RADiverseAnswers[i%len(mr2RADiverseAnswers)]
		var answerJudge agent.AnswersJudge
		if i < badProducerCount {
			answers = badAnswers
			answerJudge = &configurableByzantineJudge{
				judgeID:  fmt.Sprintf("validator-%d", i+1),
				badTexts: badTexts,
				recorder: mockRecorder,
			}
		} else {
			answerJudge = httpclient.New(httpclient.Config{
				BaseURL:             entry.URL,
				TimeoutSeconds:      720,
				JudgeTimeoutSeconds: 150,
				LabelPromptVersion:  "labeler_v3",
				AnswerPromptVersion: "answerer_v1",
			})
		}

		nodes[i] = createNodeWithCommitteeStrategyAndClassificationGrace(
			t,
			fmt.Sprintf("validator-%d", i+1),
			privateKeys[i],
			registeredValidators,
			inboxes,
			inboxes[i],
			cloneTransactions(transactions),
			&realAgentMR2Agent{labels: mr2RATxLabels, answers: answers, judge: answerJudge},
			false,
			0,
			committeeStrategyFromConfig(cfg),
			classificationGracePeriod,
		)
	}

	errCh := make(chan error, 256)
	for _, node := range nodes {
		cn := node
		go func() {
			for err := range cn.loop.Errors() {
				errCh <- err
			}
		}()
		go func(nd *integrationTestNode) { nd.loop.Run() }(node)
	}

	mr1Key := data.RoundKey{Epoch: 0, Round: roundNumber, MiniRound: uint64(data.MiniRoundOne)}
	mr2Key := data.RoundKey{Epoch: 0, Round: roundNumber, MiniRound: uint64(data.MiniRoundTwo)}
	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StartRoundEvent, RoundKey: mr1Key}
	}

	t.Logf("[configurable-adversarial-%s-q%d] round %d — %d validators, strategy=%s",
		group, badProducerCount, roundNumber, n, cfg.CommitteeStrategy)

	protocolErrors := make([]string, 0)
	roundStart := time.Now()
	finalized := assert.Eventually(t, func() bool {
		for {
			select {
			case err := <-errCh:
				protocolErrors = append(protocolErrors, err.Error())
				t.Logf("[configurable-adversarial-%s-q%d] observed error: %v", group, badProducerCount, err)
			default:
				goto errorsDrained
			}
		}
	errorsDrained:
		for _, node := range nodes {
			if _, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(mr2Key); err != nil {
				return false
			}
		}
		return true
	}, 8*time.Minute, 5*time.Second)
	duration := time.Since(roundStart)

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StopEvent}
	}

	result := configurableAdversarialRunResult{
		Timestamp:                 time.Now().UTC().Format(time.RFC3339),
		Group:                     group,
		RoundNumber:               roundNumber,
		NumValidators:             n,
		BadProducerCount:          badProducerCount,
		BadProducerIDs:            configurableAdversarialBadProducerIDs(badProducerCount),
		DurationSeconds:           duration.Seconds(),
		Finalized:                 finalized,
		AllNodesEqual:             false,
		ProtocolErrors:            protocolErrors,
		ClassificationGracePeriod: classificationGracePeriod.String(),
		PromptVersion:             classification.AnswerJudgePromptVersion,
		PromptHash:                hex.EncodeToString(classification.AnswerJudgePromptHash()),
		MockedJudgeCalls:          mockRecorder.snapshot(),
	}

	if !finalized {
		saveConfigurableAdversarialResult(t, result)
		return
	}

	block, err := nodes[0].blockFinalizer.GetFinalizedBlockInMRTwo(mr2Key)
	require.NoError(t, err)
	require.NotNil(t, block)
	for _, node := range nodes[1:] {
		other, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(mr2Key)
		require.NoError(t, err)
		require.Equal(t, block.AnswerClassifications, other.AnswerClassifications,
			"all validators must store the same canonical MR2 result")
	}

	result.AllNodesEqual = true
	result.TxResults = configurableAdversarialTxResults(block)
	var certificateVotes []*data.AnswerClassificationVote
	for _, node := range nodes {
		votes, err := node.roundState.GetAnswerClassificationVotes(mr2Key)
		if err == nil && len(votes) > len(certificateVotes) {
			certificateVotes = votes
			result.CertificateLeaderID = node.id
		}
	}
	badIDSet := make(map[string]struct{}, badProducerCount)
	for _, id := range result.BadProducerIDs {
		badIDSet[id] = struct{}{}
	}
	for _, vote := range certificateVotes {
		result.CertificateJudgeIDs = append(result.CertificateJudgeIDs, vote.JudgeID)
		if _, isByzantine := badIDSet[vote.JudgeID]; isByzantine {
			result.ByzantineCertificateJudges = append(result.ByzantineCertificateJudges, vote.JudgeID)
		}
	}
	result.CertificateVoteCount = len(result.CertificateJudgeIDs)
	logMR2BlockSummary(t, block)
	saveConfigurableAdversarialResult(t, result)
}

func runConfiguredAdversarialGroup(t *testing.T, group string) {
	t.Helper()
	cfg := loadClusterConfig(t)
	pingClusterOrSkip(t, cfg)
	badProducerCount := configuredAdversarialBadProducerCount(t)
	classificationGracePeriod := configuredClassificationGracePeriod(t)
	runConfigurableAdversarialMR2Round(
		t,
		cfg,
		group,
		mr2RABadAnswers[group],
		badProducerCount,
		classificationGracePeriod,
		uint64(time.Now().Unix()),
	)
}

func TestDistributedMR2_Diverse_ConfigurableAdversarial_GroupD(t *testing.T) {
	runConfiguredAdversarialGroup(t, "group_d")
}

func TestDistributedMR2_Diverse_ConfigurableAdversarial_GroupE(t *testing.T) {
	runConfiguredAdversarialGroup(t, "group_e")
}

func TestDistributedMR2_Diverse_ConfigurableAdversarial_GroupF(t *testing.T) {
	runConfiguredAdversarialGroup(t, "group_f")
}

func TestConfigurableByzantineJudgeProtectsBadAndRejectsHonestAnswers(t *testing.T) {
	recorder := &configurableMockRecorder{}
	badAnswer := mr2RABadAnswers["group_d"][mr2RATxHash1]
	judge := &configurableByzantineJudge{
		judgeID:  "validator-1",
		badTexts: map[string]struct{}{badAnswer: {}},
		recorder: recorder,
	}
	requestBody := map[string]any{
		"transactionHash": "tx-1",
		"candidates": []map[string]string{
			{"candidateId": "candidate-1", "answer": badAnswer},
			{"candidateId": "candidate-2", "answer": "legitimate answer"},
		},
	}
	rawRequest, err := json.Marshal(requestBody)
	require.NoError(t, err)

	response, err := judge.JudgeTransactionAnswers(agent.AnswerJudgeRequest{UserPrompt: string(rawRequest)})
	require.NoError(t, err)
	require.JSONEq(t, `{"classifications":[{"candidateId":"candidate-1","category":"CORRECT"},{"candidateId":"candidate-2","category":"WRONG"}]}`, response)
	require.Len(t, recorder.snapshot(), 1)
}
