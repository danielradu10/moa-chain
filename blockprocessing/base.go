package blockprocessing

import (
	"moa-chain/agent"
	"moa-chain/state"
)

type Base struct {
	AccountsSnapshotFactory state.AccountsSnapshotFactory
	BlockchainState         state.BlockchainState
	Labeler                 agent.Labeler
	AccountState            state.AccountsState
}
