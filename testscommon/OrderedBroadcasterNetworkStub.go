package testscommon

import (
	"fmt"
	"sync"

	"moa-chain/broadcast"
	"moa-chain/data"
)

// OrderedDeliveryRule configures deterministic delivery for messages sent to a leader.
type OrderedDeliveryRule struct {
	MessageType data.ConsensusMessageType
	SenderOrder []string
	StartIndex  int
	Mutate      OrderedMessageMutator
}

// OrderedBroadcastRule configures mutations for messages broadcast by a leader.
type OrderedBroadcastRule struct {
	MessageType data.ConsensusMessageType
	Mutate      OrderedBroadcastMutator
}

// OrderedMessageMutator can alter or drop a leader-bound message before delivery.
// Returning false drops the message.
type OrderedMessageMutator func(senderID string, message data.ConsensusMessage) (data.ConsensusMessage, bool)

// OrderedBroadcastMutator mutates a leader broadcast in place before any receiver
// gets it. Production code may also keep using the same message pointer after
// Broadcast returns, so malicious-leader tests use this to make local and remote
// verification see the same tampered artifact.
type OrderedBroadcastMutator func(senderID string, message *data.ConsensusMessage)

// OrderedBroadcasterNetworkStub delivers consensus messages through validator inboxes.
// Optional ordering rules pin the first quorum in fixture-based scenario tests,
// so assertions verify the intended edge case instead of goroutine scheduling.
type OrderedBroadcasterNetworkStub struct {
	mutex             sync.Mutex
	inboxes           map[string]chan data.RoundEvent
	delivery          map[data.ConsensusMessageType]*orderedDeliveryState
	broadcastMutators map[data.ConsensusMessageType]OrderedBroadcastMutator
}

type orderedDeliveryState struct {
	order   []string
	next    int
	pending map[string]data.RoundEvent
	mutate  OrderedMessageMutator
}

// NewOrderedBroadcasterNetworkStub creates a network stub with optional ordered delivery rules.
func NewOrderedBroadcasterNetworkStub(
	inboxes map[string]chan data.RoundEvent,
	rules []OrderedDeliveryRule,
	broadcastRules ...OrderedBroadcastRule,
) *OrderedBroadcasterNetworkStub {
	delivery := make(map[data.ConsensusMessageType]*orderedDeliveryState, len(rules))
	for _, rule := range rules {
		delivery[rule.MessageType] = &orderedDeliveryState{
			order:   append([]string(nil), rule.SenderOrder...),
			next:    rule.StartIndex,
			pending: make(map[string]data.RoundEvent),
			mutate:  rule.Mutate,
		}
	}
	broadcastMutators := make(map[data.ConsensusMessageType]OrderedBroadcastMutator, len(broadcastRules))
	for _, rule := range broadcastRules {
		broadcastMutators[rule.MessageType] = rule.Mutate
	}

	return &OrderedBroadcasterNetworkStub{
		inboxes:           cloneRoundEventInboxes(inboxes),
		delivery:          delivery,
		broadcastMutators: broadcastMutators,
	}
}

// BroadcasterForNode returns a broadcaster stub that sends messages as nodeID.
func (network *OrderedBroadcasterNetworkStub) BroadcasterForNode(nodeID string) broadcast.Broadcaster {
	return &OrderedBroadcasterStub{nodeID: nodeID, network: network}
}

func (network *OrderedBroadcasterNetworkStub) sendToLeader(
	senderID string,
	leaderID string,
	message *data.ConsensusMessage,
) error {
	if message == nil {
		return broadcast.ErrNilConsensusMessage
	}

	network.mutex.Lock()
	defer network.mutex.Unlock()

	delivery, hasOrderedDelivery := network.delivery[message.ConsensusMessageType]
	if hasOrderedDelivery {
		event := data.RoundEvent{Type: data.ConsensusMessageEvent, Message: *message}
		if delivery.mutate != nil {
			mutated, keep := delivery.mutate(senderID, event.Message)
			if !keep {
				return nil
			}
			event.Message = mutated
		}
		return network.enqueueOrdered(senderID, leaderID, event, delivery)
	}

	event := data.RoundEvent{Type: data.ConsensusMessageEvent, Message: *message}
	return network.deliver(leaderID, event)
}

func (network *OrderedBroadcasterNetworkStub) enqueueOrdered(
	senderID string,
	receiverID string,
	event data.RoundEvent,
	delivery *orderedDeliveryState,
) error {
	if _, exists := delivery.pending[senderID]; exists {
		return fmt.Errorf("ordered broadcaster received duplicate ordered message from %s", senderID)
	}
	delivery.pending[senderID] = event

	// Release buffered messages only when the next configured sender is present.
	// First-quorum protocols can finalize different valid results depending on
	// arrival timing; fixture tests use this to make that quorum explicit.
	for delivery.next < len(delivery.order) {
		nextSender := delivery.order[delivery.next]
		nextEvent, exists := delivery.pending[nextSender]
		if !exists {
			break
		}
		if err := network.deliver(receiverID, nextEvent); err != nil {
			return err
		}
		delete(delivery.pending, nextSender)
		delivery.next++
	}
	return nil
}

func (network *OrderedBroadcasterNetworkStub) broadcast(
	message *data.ConsensusMessage,
	senderID string,
	receivers []string,
) error {
	if message == nil {
		return broadcast.ErrNilConsensusMessage
	}

	network.mutex.Lock()
	defer network.mutex.Unlock()

	if mutate := network.broadcastMutators[message.ConsensusMessageType]; mutate != nil {
		mutate(senderID, message)
	}

	for _, receiverID := range receivers {
		if receiverID == senderID {
			continue
		}
		if err := network.deliver(receiverID, data.RoundEvent{
			Type:    data.ConsensusMessageEvent,
			Message: cloneConsensusMessage(message),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (network *OrderedBroadcasterNetworkStub) deliver(receiverID string, event data.RoundEvent) error {
	inbox, exists := network.inboxes[receiverID]
	if !exists {
		return broadcast.ErrInvalidValidator
	}
	inbox <- event
	return nil
}

// OrderedBroadcasterStub implements broadcast.Broadcaster through an ordered network stub.
type OrderedBroadcasterStub struct {
	nodeID  string
	network *OrderedBroadcasterNetworkStub
}

func (broadcaster *OrderedBroadcasterStub) SendVoteToLeader(
	message *data.ConsensusMessage,
	leaderID string,
) error {
	return broadcaster.network.sendToLeader(broadcaster.nodeID, leaderID, message)
}

func (broadcaster *OrderedBroadcasterStub) SendAnswerClassificationVoteToLeader(
	message *data.ConsensusMessage,
	leaderID string,
) error {
	return broadcaster.network.sendToLeader(broadcaster.nodeID, leaderID, message)
}

func (broadcaster *OrderedBroadcasterStub) BroadcastProposedBlock(
	message *data.ConsensusMessage,
	myID string,
	receivers []string,
) error {
	return broadcaster.network.broadcast(message, myID, receivers)
}

func (broadcaster *OrderedBroadcasterStub) BroadcastAggregatedVotes(
	message *data.ConsensusMessage,
	myID string,
	receivers []string,
) error {
	return broadcaster.network.broadcast(message, myID, receivers)
}

func (broadcaster *OrderedBroadcasterStub) BroadcastAnswerEvidence(
	message *data.ConsensusMessage,
	myID string,
	receivers []string,
) error {
	return broadcaster.network.broadcast(message, myID, receivers)
}

func (broadcaster *OrderedBroadcasterStub) BroadcastAnswerClassificationCertificate(
	message *data.ConsensusMessage,
	myID string,
	receivers []string,
) error {
	return broadcaster.network.broadcast(message, myID, receivers)
}

func (broadcaster *OrderedBroadcasterStub) BroadcastProposedSynthesis(
	message *data.ConsensusMessage,
	myID string,
	receivers []string,
) error {
	return broadcaster.network.broadcast(message, myID, receivers)
}

func (broadcaster *OrderedBroadcasterStub) SendSynthesisVoteToLeader(
	message *data.ConsensusMessage,
	leaderID string,
) error {
	return broadcaster.network.sendToLeader(broadcaster.nodeID, leaderID, message)
}

func (broadcaster *OrderedBroadcasterStub) BroadcastAggregatedSynthesisVotes(
	message *data.ConsensusMessage,
	myID string,
	receivers []string,
) error {
	return broadcaster.network.broadcast(message, myID, receivers)
}

func cloneRoundEventInboxes(inboxes map[string]chan data.RoundEvent) map[string]chan data.RoundEvent {
	cloned := make(map[string]chan data.RoundEvent, len(inboxes))
	for validatorID, inbox := range inboxes {
		cloned[validatorID] = inbox
	}
	return cloned
}

func cloneConsensusMessage(message *data.ConsensusMessage) data.ConsensusMessage {
	cloned := *message
	if message.ProposedBlockMessage == nil {
		return cloned
	}

	proposedBlock := *message.ProposedBlockMessage
	if message.ProposedBlockMessage.Block != nil {
		block := *message.ProposedBlockMessage.Block
		block.Header.BodyHash = copyByteSlice(block.Header.BodyHash)
		block.Header.HeaderHash = copyByteSlice(block.Header.HeaderHash)
		block.Header.PreviousHash = copyByteSlice(block.Header.PreviousHash)
		block.Header.RootHash = copyByteSlice(block.Header.RootHash)
		block.Header.PreviousRootHash = copyByteSlice(block.Header.PreviousRootHash)
		block.Body.Transactions = cloneDataTransactions(block.Body.Transactions)
		proposedBlock.Block = &block
	}
	cloned.ProposedBlockMessage = &proposedBlock
	return cloned
}

func cloneDataTransactions(transactions []data.Transaction) []data.Transaction {
	clonedTransactions := make([]data.Transaction, 0, len(transactions))
	for _, tx := range transactions {
		clonedTx := &TransactionStub{}
		clonedTx.SetNonce(tx.GetNonce())
		clonedTx.SetSender(copyByteSlice(tx.GetSender()))
		clonedTx.SetReceiver(copyByteSlice(tx.GetReceiver()))
		clonedTx.SetTransferredValue(tx.GetTransferredValue())
		clonedTx.SetPrompt(copyByteSlice(tx.GetPrompt()))
		clonedTx.SetTip(tx.GetTip())
		clonedTx.SetTimestamp(tx.GetTimestamp())
		clonedTx.SetTxHash(copyByteSlice(tx.GetTxHash()))
		clonedTx.SetDomainLabels(copyStringSlice(tx.GetDomainLabels()))
		clonedTx.SetNumInputTokens(tx.GetNumInputTokens())
		clonedTx.SetUserOutputDimension(tx.GetUserOutputDimension())
		clonedTx.SetThinkingMode(tx.GetThinkingMode())
		clonedTx.SetEstimatedConsumption(tx.GetEstimatedConsumption())
		clonedTx.SetEstimatedFee(tx.GetEstimatedFee())
		clonedTx.SetEstimatedScore(tx.GetEstimatedScore())
		clonedTransactions = append(clonedTransactions, clonedTx)
	}
	return clonedTransactions
}

func copyByteSlice(input []byte) []byte {
	if input == nil {
		return nil
	}

	output := make([]byte, len(input))
	copy(output, input)
	return output
}

func copyStringSlice(input []string) []string {
	if input == nil {
		return nil
	}

	output := make([]string, len(input))
	copy(output, input)
	return output
}

var _ broadcast.Broadcaster = (*OrderedBroadcasterStub)(nil)
