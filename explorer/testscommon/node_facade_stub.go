package testscommon

import (
	"moa-chain/data"
	"moa-chain/explorer"
	"moa-chain/tracker"
)



var _ explorer.NodeFacade = (*NodeFacadeStub)(nil)

// NodeFacadeStub is a test double for explorer.NodeFacade.
type NodeFacadeStub struct {
	Blocks              []*data.BlockOnChain
	Epoch               uint64
	RoundEntries        map[uint64]tracker.RoundEntry
	TxStatuses          map[string]tracker.TxStatus
	PendingTransactions []data.Transaction
	PrecomputedLabels   map[string][]string
	PrecomputedAnswers  map[string]string
}

func (s *NodeFacadeStub) ChainLength() uint64               { return uint64(len(s.Blocks)) }
func (s *NodeFacadeStub) CurrentRound() (uint64, error)     { return 0, nil }
func (s *NodeFacadeStub) CurrentMiniRound() (uint64, error) { return 0, nil }
func (s *NodeFacadeStub) CurrentEpoch() (uint64, error)     { return s.Epoch, nil }

func (s *NodeFacadeStub) GetTransactionStatus(txHash []byte) (tracker.TxStatus, bool) {
	status, ok := s.TxStatuses[string(txHash)]
	return status, ok
}

func (s *NodeFacadeStub) GetPendingTransactions() []data.Transaction {
	return s.PendingTransactions
}

func (s *NodeFacadeStub) GetPendingTransaction(txHash []byte) (data.Transaction, bool) {
	for _, tx := range s.PendingTransactions {
		if string(tx.GetTxHash()) == string(txHash) {
			return tx, true
		}
	}
	return nil, false
}

func (s *NodeFacadeStub) GetPrecomputedLabels(txHash []byte) ([]string, bool) {
	labels, ok := s.PrecomputedLabels[string(txHash)]
	return labels, ok
}

func (s *NodeFacadeStub) GetPrecomputedAnswer(txHash []byte) (string, bool) {
	answer, ok := s.PrecomputedAnswers[string(txHash)]
	return answer, ok
}

func (s *NodeFacadeStub) AllBlocks() []*data.BlockOnChain { return s.Blocks }

func (s *NodeFacadeStub) GetRound(epoch, round uint64) (tracker.RoundEntry, bool) {
	e, ok := s.RoundEntries[round]
	return e, ok
}

func (s *NodeFacadeStub) GetLiveRoundState() (data.RoundKey, data.Step, bool) {
	return data.RoundKey{}, 0, false
}

func (s *NodeFacadeStub) SubscribeLiveRound() (<-chan explorer.StepEvent, func(), bool) {
	return nil, nil, false
}
