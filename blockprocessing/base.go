package blockprocessing

import (
	"log/slog"

	"moa-chain/agent"
	"moa-chain/mempool"
	"moa-chain/state"
)

type Base struct {
	AccountsSnapshotFactory state.AccountsSnapshotFactory
	BlockchainState         state.BlockchainState
	Labeler                 agent.Labeler
	AccountState            state.AccountsState
	Mempool                 mempool.Mempool
	Logger                  *slog.Logger
}
