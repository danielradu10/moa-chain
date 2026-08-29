package resolvers

import (
	"encoding/hex"

	"moa-chain/data"
	"moa-chain/explorer"
)

// BlockResolver looks up finalized chain blocks.
type BlockResolver interface {
	// ResolveHash finds a block by its hex-encoded header hash.
	ResolveHash(hash string) (explorer.BlockResponse, bool)
}

type blockResolver struct {
	node explorer.NodeFacade
}

// NewBlockResolver returns a BlockResolver backed by the given node.
func NewBlockResolver(node explorer.NodeFacade) BlockResolver {
	return &blockResolver{node: node}
}

func (r *blockResolver) ResolveHash(hash string) (explorer.BlockResponse, bool) {
	block, ok := r.byHash(hash)
	if !ok {
		return explorer.BlockResponse{}, false
	}

	return toBlockResponse(block), true
}

func (r *blockResolver) byHash(hash string) (*data.BlockOnChain, bool) {
	for _, block := range r.node.AllBlocks() {
		if hex.EncodeToString(block.Header.HeaderHash) == hash {
			return block, true
		}
	}

	return nil, false
}

func (r *blockResolver) byRound(round uint64) (*data.BlockOnChain, bool) {
	for _, block := range r.node.AllBlocks() {
		if block.Header.Round == round {
			return block, true
		}
	}

	return nil, false
}
