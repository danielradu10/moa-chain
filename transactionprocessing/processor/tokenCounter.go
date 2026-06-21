package processor

import "github.com/tiktoken-go/tokenizer"

func calculateNumTokensFromPrompt(answer string) (uint64, error) {
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
