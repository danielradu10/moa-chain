package blockprocessing

import (
	"log/slog"

	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/txpipeline"
)

type Base struct {
	AccountsSnapshotFactory state.AccountsSnapshotFactory
	BlockchainState         state.BlockchainState
	Store                   txpipeline.PrecomputedStore
	AccountState            state.AccountsState
	Mempool                 mempool.Mempool
	Logger                  *slog.Logger
}
