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
	BatchAgent              agent.BatchAgent
	AccountState            state.AccountsState
	Mempool                 mempool.Mempool
	Logger                  *slog.Logger
}
