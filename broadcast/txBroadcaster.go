package broadcast

import (
	"log/slog"

	"moa-chain/data"
	"moa-chain/logging"
)

type txBroadcaster struct {
	registry TxPeerRegistry
	logger   *slog.Logger
}

// NewTxBroadcaster creates a TxBroadcaster backed by the given registry.
func NewTxBroadcaster(registry TxPeerRegistry, loggers ...*slog.Logger) TxBroadcaster {
	return &txBroadcaster{
		registry: registry,
		logger:   logging.FromOptional(loggers...),
	}
}

// BroadcastTransaction sends tx to every registered peer's inbox except senderID.
func (b *txBroadcaster) BroadcastTransaction(tx data.Transaction, senderID string) {
	peers := b.registry.GetAll()
	for nodeID, inbox := range peers {
		if nodeID == senderID {
			continue
		}
		b.logger.Debug("txBroadcaster: delivering transaction to peer",
			"senderID", senderID, "peerID", nodeID, "txHash", string(tx.GetTxHash()))
		inbox <- tx
	}
}
