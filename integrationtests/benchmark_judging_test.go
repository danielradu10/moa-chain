//go:build integration

package integrationtests

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/consensus/miniround2/classification"
)

// ── Shared judge record types ─────────────────────────────────────────────────
//
// judgeBenchmarkResult and judgedCandidate are also used by the real-agent
// classification scenario tests (Groups A–F). Define them here once.

// judgeBenchmarkResult is one data point written to judge_benchmark_results.jsonl.
// One record is written per /judge HTTP call (one call covers all candidates for
// one transaction). To compute block-level duration for Experiment F, sum
// call_duration_ms across all records sharing the same (experiment, run,
// num_txs_in_block) key.
type judgeBenchmarkResult struct {
	Experiment     string           `json:"experiment"`
	Run            int              `json:"run"`
	TxIndex        int              `json:"tx_index"`        // 0-based position of this tx in the block
	NumTxsInBlock  int              `json:"num_txs_in_block"`
	CallDurationMs int64            `json:"call_duration_ms"`
	Timestamp      string           `json:"timestamp"`
	TxPrompt       string           `json:"tx_prompt"`
	NumCandidates  int              `json:"num_candidates"`
	Candidates     []judgedCandidate `json:"candidates"`
	RawResponse    string           `json:"raw_response"`
}

// judgedCandidate is one candidate entry inside a judgment record.
// GroundTruth is set by the test author at fixture time and is never sent to
// the judge. Use "unknown" for pure latency benchmarks where no intent is defined.
type judgedCandidate struct {
	Alias       string `json:"alias"`
	AnswerText  string `json:"answer_text"`
	GroundTruth string `json:"ground_truth"`
	Category    string `json:"category"`
}

// ── Internal types for building and parsing judge requests ────────────────────

type benchJudgeInput struct {
	TransactionHash string             `json:"transactionHash"`
	Prompt          string             `json:"prompt"`
	Candidates      []benchJudgeEntry  `json:"candidates"`
}

type benchJudgeEntry struct {
	CandidateID string `json:"candidateId"`
	Answer      string `json:"answer"`
}

type benchJudgeOutput struct {
	Classifications []benchJudgeClassification `json:"classifications"`
}

type benchJudgeClassification struct {
	CandidateID string `json:"candidateId"`
	Category    string `json:"category"`
}

// ── Candidate pool ────────────────────────────────────────────────────────────

type candidateFixture struct {
	answer      string
	groundTruth string
}

// backpropCandidates provides 7 distinct candidate answers of varying style for
// the backpropagation question used in Experiment E (vary candidate count).
var backpropCandidates = []candidateFixture{
	{
		answer:      "Backpropagation computes gradients of the loss function with respect to each weight by applying the chain rule of calculus. Starting from the output layer, it propagates error signals backward through the network. The chain rule is essential because the output of each layer is a function of the previous layer's output, so the derivative of the loss with respect to an early-layer weight is the product of all intermediate partial derivatives along the path to the loss.",
		groundTruth: "unknown",
	},
	{
		answer:      "In backpropagation, a forward pass computes activations and caches them. The backward pass then computes ∂L/∂w for every weight using the chain rule: ∂L/∂w = (∂L/∂a) × (∂a/∂z) × (∂z/∂w), where a is the post-activation value and z is the pre-activation. Each layer's local gradient is multiplied by the upstream gradient arriving from the next layer, reusing intermediate results efficiently via dynamic programming.",
		groundTruth: "unknown",
	},
	{
		answer:      "Backpropagation is the algorithm used to train neural networks by computing how much each weight contributed to the prediction error. It uses the chain rule to differentiate through multiple composed functions layer by layer. Without the chain rule it would be impossible to propagate the gradient signal from the output loss back through many nested transformations. The computed gradients are then used by an optimizer such as stochastic gradient descent to update each weight.",
		groundTruth: "unknown",
	},
	{
		answer:      "The backpropagation algorithm has two phases. During the forward pass, activations are computed and stored at every layer. During the backward pass, the gradient of the loss flows in reverse: δ_l = (W_{l+1}^T δ_{l+1}) ⊙ σ'(z_l), where σ' is the derivative of the activation function and δ is the error signal. This recurrence is the chain rule applied repeatedly across layers, making the computation O(n) in the number of parameters.",
		groundTruth: "unknown",
	},
	{
		answer:      "Backpropagation relies on the chain rule to compute gradients efficiently across all layers of a deep network. The key insight is that the gradient of the loss with respect to layer l's parameters depends only on the gradient already computed at layer l+1 and the local Jacobian of layer l. This allows a single backward pass to compute all gradients simultaneously, rather than differentiating each parameter independently.",
		groundTruth: "unknown",
	},
	{
		answer:      "Neural networks are trained with backpropagation, which propagates the loss gradient backward through the computation graph. The chain rule decomposes the derivative of a composite function f(g(x)) as f'(g(x)) × g'(x). In a deep network this decomposition extends across every layer. Vanishing gradients arise when these products shrink to near zero in early layers, preventing learning — a problem addressed by residual connections and careful weight initialisation.",
		groundTruth: "unknown",
	},
	{
		answer:      "Backpropagation is a special case of reverse-mode automatic differentiation applied to neural networks. Given a computation graph where each node is a differentiable operation, the algorithm computes gradients by traversing the graph in reverse topological order. The chain rule ensures the gradient at any node equals the sum of contributions from all downstream paths, each weighted by the local derivative along that path. This makes backpropagation exact and computationally equivalent to one forward pass in cost.",
		groundTruth: "unknown",
	},
}

// judgeExpFFixtures provides prompts and 5-candidate pools for Experiment F
// (vary tx count with 5 candidates per tx).
var judgeExpFFixtures = []struct {
	prompt     string
	candidates []candidateFixture
}{
	{
		prompt:     "How does backpropagation work in a neural network and why is the chain rule needed?",
		candidates: backpropCandidates[:5],
	},
	{
		prompt: "What is the difference between a proof-of-work and a proof-of-stake consensus mechanism in blockchain networks?",
		candidates: []candidateFixture{
			{answer: "Proof-of-work requires miners to find a nonce such that the block hash falls below a target value, making block production computationally expensive. Proof-of-stake selects validators proportionally to the amount of cryptocurrency they have staked as collateral, replacing energy expenditure with economic commitment. PoW is secured by real-world energy cost; PoS is secured by the threat of losing staked capital through slashing.", groundTruth: "unknown"},
			{answer: "In PoW the probability of mining the next block is proportional to the fraction of total hash rate a participant controls. In PoS the probability of being selected as the next validator is proportional to their fraction of total staked tokens. PoW attacks require controlling more than 50% of hash rate; PoS attacks require accumulating more than one-third of staked supply, which is expensive to acquire without moving the market price.", groundTruth: "unknown"},
			{answer: "PoW consensus requires repeated hashing to create an unforgeable proof of resource expenditure, making block production costly and therefore Sybil-resistant. PoS replaces thermodynamic irreversibility with financial irreversibility: producing conflicting blocks risks slashing of the validator's deposit. Both mechanisms achieve Sybil resistance but with very different energy profiles — PoS consumes orders of magnitude less electricity than PoW.", groundTruth: "unknown"},
			{answer: "The fundamental resource differs: PoW uses computational work, PoS uses locked capital. PoW rewards miners with block rewards proportional to hash rate; PoS rewards validators proportional to stake. PoW has a long security track record in Bitcoin. PoS introduces the nothing-at-stake problem and long-range attack vectors, which modern PoS designs address with slashing conditions, finality gadgets, and weak subjectivity checkpoints.", groundTruth: "unknown"},
			{answer: "PoW creates artificial scarcity of block production by requiring costly computation. PoS achieves the same result through economic lockup. PoW mining favours those with access to cheap electricity and efficient hardware, tending to centralise around low-cost energy regions. PoS favours those with large existing token holdings, which critics argue entrenches early adopters. Both models have governance and centralisation trade-offs beyond the pure security model.", groundTruth: "unknown"},
		},
	},
	{
		prompt: "What is the difference between horizontal and vertical scaling in cloud infrastructure, and when should each be used?",
		candidates: []candidateFixture{
			{answer: "Vertical scaling increases the resources of a single machine — more CPU, RAM, or storage. Horizontal scaling adds more machines and distributes load across them. Vertical scaling has a physical ceiling and often requires downtime. Horizontal scaling is theoretically unbounded but requires the application to handle distributed state or be stateless, adding architectural complexity.", groundTruth: "unknown"},
			{answer: "Scale vertically when your workload is not easily parallelisable, such as a large relational database with complex transactions that are hard to shard. Scale horizontally when handling variable traffic via auto-scaling, when services are stateless, or when you need fault tolerance and geographic distribution. Most modern cloud architectures favour horizontal scaling for application tiers and accept vertical scaling only for stateful storage layers.", groundTruth: "unknown"},
			{answer: "Horizontal scaling provides fault tolerance: losing one instance is absorbed by others. It enables auto-scaling based on load metrics. Vertical scaling is simpler operationally — no load balancer changes needed — and works well for single-threaded workloads or applications that require large shared-memory spaces. The choice depends on whether the bottleneck is concurrency, which favours horizontal, or per-request resource intensity, which favours vertical.", groundTruth: "unknown"},
			{answer: "Cost models differ. A single large machine is often more expensive per compute unit than many small machines; horizontal scaling allows use of spot or preemptible instances. Vertical scaling offers better network locality since all computation is co-located, which matters for latency-sensitive in-process communication. For operations that would incur network round-trips in a distributed system, a single large machine can outperform a distributed cluster.", groundTruth: "unknown"},
			{answer: "Cloud architectures typically use both simultaneously. Horizontal scaling handles user-facing stateless services: API gateways, microservices, web servers. Vertical scaling applies to databases that are difficult to distribute. Read replicas provide a form of horizontal scaling for read-heavy database workloads. The practical question is not which is better but which dimension of scale is needed for the specific bottleneck being addressed.", groundTruth: "unknown"},
		},
	},
	{
		prompt: "What is the virtual DOM in React and how does it improve rendering performance compared to direct DOM manipulation?",
		candidates: []candidateFixture{
			{answer: "The virtual DOM is an in-memory JavaScript representation of the real DOM tree. When state changes, React renders the component tree into a new virtual DOM, diffs it against the previous snapshot using the reconciliation algorithm, and applies only the minimal set of real DOM mutations needed. This batching and diffing avoids redundant real DOM operations, which are slow because they can trigger browser layout reflow and repaint.", groundTruth: "unknown"},
			{answer: "Direct DOM manipulation is expensive because accessing or writing DOM properties can force the browser to recompute layout. React's virtual DOM batches all updates and computes a diff in JavaScript before touching the real DOM. The reconciliation algorithm uses heuristics — same component type implies same subtree, keys identify list items — to make diffing O(n) rather than the naive O(n³) tree edit distance.", groundTruth: "unknown"},
			{answer: "The virtual DOM allows React to use a declarative programming model: the developer describes what the UI should look like as a function of state, and React determines how to update the DOM to match. This separation makes code easier to reason about. Internally React batches multiple state updates into a single render cycle, reducing the number of times the browser layout engine runs and therefore reducing jank.", groundTruth: "unknown"},
			{answer: "Performance gains from the virtual DOM come from diffing and batching. Diffing identifies the minimum change set, avoiding full DOM reconstruction on every state update. Batching groups multiple state changes into one render pass. React 18's concurrent rendering extends this by allowing updates to be interrupted and prioritised, so high-priority interactions like typing are not blocked by slower background render work.", groundTruth: "unknown"},
			{answer: "It is worth noting the virtual DOM is not always faster than direct DOM manipulation. For infrequent, simple updates, direct access skips the diffing overhead and can be faster. The virtual DOM pays off when updates are frequent and affect large component trees. Svelte compiles components to direct DOM operations at build time, demonstrating that the virtual DOM is a design trade-off rather than an absolute performance win.", groundTruth: "unknown"},
		},
	},
	{
		prompt: "Explain the CAP theorem and its implications for distributed database design.",
		candidates: []candidateFixture{
			{answer: "The CAP theorem states that a distributed system can guarantee at most two of three properties: Consistency (every read returns the most recent write or an error), Availability (every request receives a non-error response), and Partition tolerance (the system operates despite network message loss). Since partitions are unavoidable in practice, real systems must choose between consistency and availability during a partition event.", groundTruth: "unknown"},
			{answer: "CAP is often misread as a static three-way choice. In practice all distributed systems must tolerate partitions, so the real decision is what to sacrifice when a partition occurs. CP systems such as HBase and Zookeeper reject or time out requests during a partition to preserve consistency. AP systems such as Cassandra and DynamoDB continue serving requests with potentially stale data. The choice depends on whether the application can tolerate stale reads versus brief unavailability.", groundTruth: "unknown"},
			{answer: "The CAP theorem was formalised by Brewer and proved by Gilbert and Lynch. It applies to a single data item across a distributed system, not to the system globally. PACELC extends CAP by also characterising the latency-consistency trade-off in the absence of partitions. Most real-world databases offer tunable consistency levels — for example, Cassandra quorum reads — allowing operators to choose a point on the spectrum per operation rather than committing globally.", groundTruth: "unknown"},
			{answer: "CAP guides database architects toward explicit consistency guarantees. Financial systems where double-spending is catastrophic require strong consistency (CP). Social media feeds can tolerate eventual consistency (AP). The trade-off affects not just correctness but also operational complexity: eventual consistency requires application-level conflict resolution strategies such as last-write-wins, vector clocks, or CRDTs to merge concurrent writes deterministically.", groundTruth: "unknown"},
			{answer: "Modern distributed databases blur CAP boundaries through configurable consistency. Google Spanner achieves external consistency across geographically distributed nodes using atomic clocks and TrueTime to bound clock uncertainty. DynamoDB offers both eventually and strongly consistent reads per request. The practical lesson of CAP is not that you must choose a side permanently, but that you must explicitly design partition-event behaviour and document the consistency guarantee your application relies on.", groundTruth: "unknown"},
		},
	},
}

// ── Result persistence ────────────────────────────────────────────────────────

func openJudgeBenchmarkFile(t *testing.T) (*os.File, func()) {
	t.Helper()
	path := filepath.Join("testData", "miniround2", "results", "judge_benchmark_results.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	return f, func() { _ = f.Close() }
}

func writeJudgeBenchmarkResult(t *testing.T, f *os.File, r judgeBenchmarkResult) {
	t.Helper()
	b, err := json.Marshal(r)
	require.NoError(t, err)
	_, err = fmt.Fprintf(f, "%s\n", b)
	require.NoError(t, err)
}

// ── Judge call helper ─────────────────────────────────────────────────────────

// invokeJudge builds the canonical judge request for one transaction, calls
// the agent, parses the response, and returns a populated judgeBenchmarkResult.
// Returns an error if the HTTP call fails so the caller can log and skip.
// Panics via require on internal errors (JSON encoding of local structs) that
// indicate a programming mistake rather than an expected runtime failure.
func invokeJudge(
	t *testing.T,
	judge agent.AnswersJudge,
	txPrompt string,
	candidates []candidateFixture,
) (judgeBenchmarkResult, error) {
	t.Helper()

	txHash := promptHash(txPrompt)

	input := benchJudgeInput{
		TransactionHash: txHash,
		Prompt:          txPrompt,
		Candidates:      make([]benchJudgeEntry, len(candidates)),
	}
	for i, c := range candidates {
		input.Candidates[i] = benchJudgeEntry{
			CandidateID: fmt.Sprintf("candidate-%d", i+1),
			Answer:      c.answer,
		}
	}

	userPrompt, err := json.Marshal(input)
	require.NoError(t, err)

	req := agent.AnswerJudgeRequest{
		SystemPrompt: classification.AnswerJudgeProtocolPrompt,
		UserPrompt:   string(userPrompt),
	}

	t0 := time.Now()
	rawResponse, callErr := judge.JudgeTransactionAnswers(req)
	dur := time.Since(t0)
	if callErr != nil {
		return judgeBenchmarkResult{}, callErr
	}

	var parsed benchJudgeOutput
	require.NoError(t, json.Unmarshal([]byte(rawResponse), &parsed))

	categoryByAlias := make(map[string]string, len(parsed.Classifications))
	for _, cl := range parsed.Classifications {
		categoryByAlias[cl.CandidateID] = cl.Category
	}

	judged := make([]judgedCandidate, len(candidates))
	for i, c := range candidates {
		alias := fmt.Sprintf("candidate-%d", i+1)
		judged[i] = judgedCandidate{
			Alias:       alias,
			AnswerText:  c.answer,
			GroundTruth: c.groundTruth,
			Category:    categoryByAlias[alias],
		}
	}

	return judgeBenchmarkResult{
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		CallDurationMs: dur.Milliseconds(),
		TxPrompt:       txPrompt,
		NumCandidates:  len(candidates),
		Candidates:     judged,
		RawResponse:    rawResponse,
	}, nil
}

// promptHash returns the SHA-256 hex digest of the prompt, used as a stable
// transaction hash in benchmark judge requests.
func promptHash(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(h[:])
}

// ── Benchmark test ────────────────────────────────────────────────────────────

// TestMiniRoundTwo_JudgingLatencyBenchmark measures how judging latency scales
// with the number of candidate answers and the number of transactions per block.
//
// Experiment E — fixed 1 transaction, vary candidate count: 1, 3, 5, 7
//
//	Isolates how a single /judge call scales with more candidates to classify.
//	Directly calibrates the per-tx judging timeout.
//
// Experiment F — fixed 5 candidates per tx, vary tx count: 1, 3, 5
//
//	Isolates how total block judging time scales when the judge is called once
//	per transaction. Sum call_duration_ms across records with the same
//	(experiment, run, num_txs_in_block) to get the total block judging duration.
//
// Each data point is repeated 3 times to smooth warm-model variance.
// Results are appended to testData/miniround2/results/judge_benchmark_results.jsonl.
//
// Run with: make test-realagent-mr2-benchmark
func TestMiniRoundTwo_JudgingLatencyBenchmark(t *testing.T) {
	pingAgentOrSkip(t)

	client := realAgentClient()
	f, closeFile := openJudgeBenchmarkFile(t)
	defer closeFile()

	cooldown := func() { time.Sleep(3 * time.Second) }
	const runsPerPoint = 3

	// ── Experiment E: vary candidate count ────────────────────────────────────
	t.Log("=== Experiment E: vary candidate count (1 tx, candidates = 1, 3, 5, 7) ===")

	candidateCounts := []int{1, 3, 5, 7}

	for _, n := range candidateCounts {
		pool := backpropCandidates[:n]

		for run := 1; run <= runsPerPoint; run++ {
			cooldown()
			t.Logf("[E] candidates=%d run=%d — calling judge...", n, run)

			result, err := invokeJudge(t, client, judgeExpFFixtures[0].prompt, pool)
			if err != nil {
				t.Logf("[E] candidates=%d run=%d → SKIPPED (%v)", n, run, err)
				continue
			}

			result.Experiment = "E_vary_candidate_count"
			result.Run = run
			result.TxIndex = 0
			result.NumTxsInBlock = 1

			t.Logf("[E] candidates=%d run=%d → %dms  categories=%v",
				n, run, result.CallDurationMs, categoriesFrom(result.Candidates))
			writeJudgeBenchmarkResult(t, f, result)
		}
	}

	// ── Experiment F: vary tx count ───────────────────────────────────────────
	t.Log("=== Experiment F: vary tx count (5 candidates/tx, txs = 1, 3, 5) ===")

	txCounts := []int{1, 3, 5}
	const candidatesPerTx = 5

	for _, numTxs := range txCounts {
		fixtures := judgeExpFFixtures[:numTxs]

		for run := 1; run <= runsPerPoint; run++ {
			cooldown()
			t.Logf("[F] txs=%d run=%d — judging each tx...", numTxs, run)

			for txIdx, fixture := range fixtures {
				result, err := invokeJudge(t, client, fixture.prompt, fixture.candidates[:candidatesPerTx])
				if err != nil {
					t.Logf("[F] txs=%d tx=%d run=%d → SKIPPED (%v)", numTxs, txIdx, run, err)
					continue
				}

				result.Experiment = "F_vary_tx_count"
				result.Run = run
				result.TxIndex = txIdx
				result.NumTxsInBlock = numTxs

				t.Logf("[F] txs=%d tx=%d run=%d → %dms  categories=%v",
					numTxs, txIdx+1, run, result.CallDurationMs, categoriesFrom(result.Candidates))
				writeJudgeBenchmarkResult(t, f, result)
			}
		}
	}
}

// categoriesFrom extracts just the category labels for compact log output.
func categoriesFrom(candidates []judgedCandidate) []string {
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.Category
	}
	return out
}
