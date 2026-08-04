package processor

import "github.com/tiktoken-go/tokenizer"

// CountTokensFromAnswer returns the number of tokens in the given answer string
// using the cl100k_base tokenizer (the same encoding used by GPT-4 / Codex).
// Exported so blockprocessing can compute consumption in the batch answer path.
func CountTokensFromAnswer(answer string) (uint64, error) {
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		return 0, err
	}

	numTokens, err := enc.Count(answer)
	if err != nil {
		return 0, err
	}

	return uint64(numTokens), nil
}
