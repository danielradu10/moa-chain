package main

import (
	"fmt"

	"moa-chain/agent/httpclient"
	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/proposing"
	"moa-chain/blockprocessing/validation"
	"moa-chain/cmd"
	"moa-chain/consensus/miniround2"
	"moa-chain/txpipeline"
)

func main() {
	cfg := cmd.DefaultNodeConfig()

	// agentClient is used by the TxPreprocessor (LabelBatch / AnswerBatch) and
	// as AnswerJudge for the MR2 classification step.
	agentClient := httpclient.New(httpclient.Config{
		BaseURL:             cfg.Agent.BaseURL,
		TimeoutSeconds:      cfg.Agent.TimeoutSeconds,
		LabelPromptVersion:  cfg.Agent.LabelPromptVersion,
		LabelPromptHash:     cfg.Agent.LabelPromptHash,
		AnswerPromptVersion: cfg.Agent.AnswerPromptVersion,
		AnswerPromptHash:    cfg.Agent.AnswerPromptHash,
	})

	// store is populated by TxPreprocessor before consensus; body executors read from it.
	store := txpipeline.NewPrecomputedStore()

	base := blockprocessing.Base{
		Store: store,
	}

	_ = proposing.NewBlockCreator(base)
	_ = validation.NewBlockProcessor(base)

	// MR2 handler uses the agent client as AnswerJudge for the classification step.
	_ = miniround2.MiniRoundTwoHandlerArgs{AnswerJudge: agentClient}

	fmt.Println("MoA Chain Node running!")
}
