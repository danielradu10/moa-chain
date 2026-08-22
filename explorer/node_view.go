package explorer

import (
	"encoding/hex"
	"errors"
	"time"

	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/chain"
	"moa-chain/data"
	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/tracker"
	"moa-chain/txpipeline"
	"moa-chain/validators"
)

// TxSubmitter accepts and validates a new transaction for processing.
// *txpipeline.TxInterceptor satisfies this interface.
type TxSubmitter interface {
	Submit(tx data.Transaction) error
}

// LiveStateReader provides a thread-safe snapshot of the current consensus
// round key and step. *consensus.RoundLoop satisfies this interface.
type LiveStateReader interface {
	LiveState() (data.RoundKey, data.Step)
}

// NodeView aggregates the read-only state references the explorer needs.
// All fields are set once at wiring time and never mutated after that.
// Every referenced component is already thread-safe (RWMutex or atomic),
// so handlers can read concurrently without additional locking.
type NodeView struct {
	Chain             chain.Chain
	BlockchainState   state.BlockchainState
	BlockFinalizer    blockFinalizer.BlockFinalizer
	ValidatorRegistry validators.ValidatorRegistry
	Store             txpipeline.PrecomputedStore
	Mempool           mempool.Mempool
	TxTracker         *tracker.TxTracker
	RoundTracker      *tracker.RoundTracker
	RoundLoop         LiveStateReader
	RoundHub          *RoundHub
	TxHub             *TxHub
	TxSubmitter       TxSubmitter
}

// ChainLength returns the number of finalized blocks on the chain.
func (nv *NodeView) ChainLength() uint64 { return nv.Chain.Len() }

// CurrentRound returns the round number the node is currently processing.
func (nv *NodeView) CurrentRound() (uint64, error) { return nv.BlockchainState.CurrentRound() }

// CurrentMiniRound returns the mini-round number within the current round.
func (nv *NodeView) CurrentMiniRound() (uint64, error) {
	return nv.BlockchainState.CurrentMiniRound()
}

// CurrentEpoch returns the current epoch number.
func (nv *NodeView) CurrentEpoch() (uint64, error) { return nv.BlockchainState.CurrentEpoch() }

// GetTransactionStatus returns the lifecycle status of a transaction by its raw hash bytes.
func (nv *NodeView) GetTransactionStatus(txHash []byte) (tracker.TxStatus, bool) {
	return nv.TxTracker.GetStatus(string(txHash))
}

// GetPendingTransactions returns a snapshot of all transactions currently in the mempool.
func (nv *NodeView) GetPendingTransactions() []data.Transaction {
	return nv.Mempool.GetPendingTransactions()
}

// GetPendingTransaction returns a transaction currently in the mempool by its raw hash bytes.
func (nv *NodeView) GetPendingTransaction(txHash []byte) (data.Transaction, bool) {
	for _, tx := range nv.Mempool.GetPendingTransactions() {
		if string(tx.GetTxHash()) == string(txHash) {
			return tx, true
		}
	}
	return nil, false
}

// GetPrecomputedLabels returns the domain labels assigned to a transaction during preprocessing.
func (nv *NodeView) GetPrecomputedLabels(txHash []byte) ([]string, bool) {
	return nv.Store.GetLabels(txHash)
}

// GetPrecomputedAnswer returns the local model answer for a transaction from the precomputed store.
func (nv *NodeView) GetPrecomputedAnswer(txHash []byte) (string, bool) {
	return nv.Store.GetAnswer(txHash)
}

// AllBlocks returns all finalized blocks on the chain, oldest first.
func (nv *NodeView) AllBlocks() []*data.BlockOnChain { return nv.Chain.Blocks() }

// GetRound returns the tracked state for a specific epoch/round pair.
func (nv *NodeView) GetRound(epoch, round uint64) (tracker.RoundEntry, bool) {
	return nv.RoundTracker.GetRound(epoch, round)
}

// GetLiveRoundState returns the current round key and consensus step.
// Returns false if no round loop is wired (e.g. in tests).
func (nv *NodeView) GetLiveRoundState() (data.RoundKey, data.Step, bool) {
	if nv.RoundLoop == nil {
		return data.RoundKey{}, 0, false
	}
	key, step := nv.RoundLoop.LiveState()
	return key, step, true
}

// SubscribeLiveRound returns a channel of StepEvents for SSE streaming and an
// unsubscribe function. Returns false when no round hub is wired.
func (nv *NodeView) SubscribeLiveRound() (<-chan StepEvent, func(), bool) {
	if nv.RoundHub == nil {
		return nil, nil, false
	}
	ch, unsub := nv.RoundHub.Subscribe()
	return ch, unsub, true
}

// SubscribeTxEvents returns a channel of TxEvents for the given transaction
// and an unsubscribe function. Returns false when no tx hub is wired.
func (nv *NodeView) SubscribeTxEvents(txHash []byte) (<-chan TxEvent, func(), bool) {
	if nv.TxHub == nil {
		return nil, nil, false
	}
	ch, unsub := nv.TxHub.Subscribe(string(txHash))
	return ch, unsub, true
}

// SubmitTransaction builds a transaction from the request, computes its
// canonical hash, and forwards it to the wired TxSubmitter.
// Returns an error when no submitter is wired or submission fails.
func (nv *NodeView) SubmitTransaction(req SubmitTransactionRequest) (SubmitTransactionResponse, error) {
	if nv.TxSubmitter == nil {
		return SubmitTransactionResponse{}, errors.New("transaction submission not enabled")
	}
	timestamp := uint64(time.Now().UnixNano())
	txHash := txpipeline.ComputeTxHash(req.Sender, req.Nonce, req.Prompt, req.Tip, timestamp)

	tx := mempool.NewTransaction()
	tx.SetSender([]byte(req.Sender))
	tx.SetReceiver([]byte("moa-chain"))
	tx.SetPrompt([]byte(req.Prompt))
	tx.SetNonce(req.Nonce)
	tx.SetTip(req.Tip)
	tx.SetTimestamp(timestamp)
	tx.SetEstimatedFee(1)
	tx.SetThinkingMode("fast")
	tx.SetUserOutputDimension("short")
	tx.SetTxHash(txHash)

	if err := nv.TxSubmitter.Submit(tx); err != nil {
		return SubmitTransactionResponse{}, err
	}
	return SubmitTransactionResponse{
		TxHash:    hex.EncodeToString(txHash),
		Timestamp: timestamp,
	}, nil
}
