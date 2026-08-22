package testscommon

import (
	"moa-chain/data"
	"moa-chain/explorer"
	"moa-chain/tracker"
)

var _ explorer.NodeFacade = (*NodeFacadeStub)(nil)

// NodeFacadeStub is a test double for explorer.NodeFacade.
type NodeFacadeStub struct {
	Blocks       []*data.BlockOnChain
	Epoch        uint64
	RoundEntries map[uint64]tracker.RoundEntry
}

func (s *NodeFacadeStub) ChainLength() uint64               { return uint64(len(s.Blocks)) }
func (s *NodeFacadeStub) CurrentRound() (uint64, error)     { return 0, nil }
func (s *NodeFacadeStub) CurrentMiniRound() (uint64, error) { return 0, nil }
func (s *NodeFacadeStub) CurrentEpoch() (uint64, error)     { return s.Epoch, nil }
func (s *NodeFacadeStub) GetTransactionStatus(hash string) (tracker.TxStatus, bool) {
	return "", false
}
func (s *NodeFacadeStub) AllBlocks() []*data.BlockOnChain { return s.Blocks }
func (s *NodeFacadeStub) GetRound(epoch, round uint64) (tracker.RoundEntry, bool) {
	e, ok := s.RoundEntries[round]
	return e, ok
}
