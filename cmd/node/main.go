package main

import (
	"fmt"

	"moa-chain/agent/httpclient"
	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/proposing"
	"moa-chain/blockprocessing/validation"
	"moa-chain/cmd"
	"moa-chain/consensus/miniround2"
)

func main() {
	cfg := cmd.DefaultNodeConfig()

	// agentClient implements both agent.BatchAgent (LabelBatch / AnswerBatch)
	// and agent.AnswersJudge (JudgeTransactionAnswers).
	agentClient := httpclient.New(httpclient.Config{
		BaseURL:             cfg.Agent.BaseURL,
		TimeoutSeconds:      cfg.Agent.TimeoutSeconds,
		LabelPromptVersion:  cfg.Agent.LabelPromptVersion,
		LabelPromptHash:     cfg.Agent.LabelPromptHash,
		AnswerPromptVersion: cfg.Agent.AnswerPromptVersion,
		AnswerPromptHash:    cfg.Agent.AnswerPromptHash,
	})

	base := blockprocessing.Base{
		BatchAgent: agentClient,
	}

	// blockCreator and blockProcessor pick up BatchAgent from Base automatically.
	_ = proposing.NewBlockCreator(base)
	_ = validation.NewBlockProcessor(base)

	// MR2 handler uses the same client as AnswerJudge for the classification step.
	_ = miniround2.MiniRoundTwoHandlerArgs{AnswerJudge: agentClient}

	fmt.Println("MoA Chain Node running!")
}
