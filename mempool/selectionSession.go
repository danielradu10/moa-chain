package mempool

import (
	"moa-chain/data"
	"moa-chain/state"
)

type selectionSession struct {
	virtualRecords map[string]*virtualRecord
	accountsState  state.AccountsState

	skippedSenders map[string]struct{}
}

func newSelectionSession(accountsState state.AccountsState) *selectionSession {
	return &selectionSession{
		virtualRecords: make(map[string]*virtualRecord),
		accountsState:  accountsState,

		skippedSenders: make(map[string]struct{}),
	}
}

// OnSelectedTransaction updates the virtual state of a sender
func (session *selectionSession) OnSelectedTransaction(transaction data.Transaction) error {
	sender := string(transaction.GetSender())
	vr, ok := session.virtualRecords[sender]
	if !ok {
		initialNonce, err := session.accountsState.GetNonceByAddress(sender)
		if err != nil {
			return err
		}

		initialBalance, err := session.accountsState.GetBalanceByAddress(sender)
		if err != nil {
			return err
		}

		vr = newVirtualRecord(
			initialNonce,
			initialBalance,
			transaction.GetNonce(),
		)
		session.virtualRecords[sender] = vr
	}

	vr.updateNonce(transaction.GetNonce())
	vr.accumulateBalance(transaction.GetTransferredValue())

	return nil
}

func (session *selectionSession) senderShouldBeSkipped(transaction data.Transaction) bool {
	sender := string(transaction.GetSender())
	_, ok := session.skippedSenders[sender]
	if ok {
		return true
	}

	txNonce := transaction.GetNonce()

	vr, ok := session.virtualRecords[sender]
	if !ok {
		// search for an initial nonce gap
		// if there is any, that sender can be completely be skipped in this session, because its txs are sorted by nonce
		// so if the first tx has a nonce gap, the other ones will have too
		if session.higherNonceThanInitialNonce(sender, txNonce) {
			session.skippedSenders[sender] = struct{}{}
			return true
		}
	}

	if session.higherNonceThanCurrentNonce(vr, txNonce) {
		return true
	}

	return false
}

func (session *selectionSession) higherNonceThanInitialNonce(sender string, txNonce uint64) bool {
	initialNonce, err := session.accountsState.GetNonceByAddress(sender)
	if err != nil || txNonce > initialNonce {
		return true
	}

	return false
}

func (session *selectionSession) higherNonceThanCurrentNonce(vr *virtualRecord, txNonce uint64) bool {
	if vr == nil {
		return false
	}

	lastNonce := vr.currentNonce.Value
	return txNonce > lastNonce+1
}

func (session *selectionSession) transactionShouldBeSkipped(transaction data.Transaction) bool {
	sender := string(transaction.GetSender())
	_, ok := session.skippedSenders[sender]
	if ok {
		return true
	}

	txNonce := transaction.GetNonce()
	vr, ok := session.virtualRecords[sender]
	if !ok {
		if session.lowerOrDuplicatedNonceThanInitialNonce(sender, txNonce) {
			return true
		}
	}

	if session.lowerOrDuplicatedNonceThanCurrentNonce(vr, txNonce) {
		return true
	}

	if session.initialBalanceWillBeExceeded(sender, transaction) {
		return true
	}

	return false
}

func (session *selectionSession) lowerOrDuplicatedNonceThanInitialNonce(sender string, txNonce uint64) bool {
	initialNonce, err := session.accountsState.GetNonceByAddress(sender)
	if err != nil || txNonce < initialNonce {
		return true
	}

	return false
}

func (session *selectionSession) lowerOrDuplicatedNonceThanCurrentNonce(vr *virtualRecord, txNonce uint64) bool {
	if vr == nil {
		return false
	}

	lastNonce := vr.currentNonce.Value
	return txNonce < lastNonce+1
}

func (session *selectionSession) initialBalanceWillBeExceeded(sender string, transaction data.Transaction) bool {
	vr, ok := session.virtualRecords[sender]
	if !ok {
		initialBalance, err := session.accountsState.GetBalanceByAddress(sender)
		if err != nil {
			return true
		}

		// TODO here, we should add also the tip and the estimated fee.
		// TODO Assure this corresponds with the one from transaction processor
		if transaction.GetTransferredValue() > initialBalance {
			return true
		}

		return false
	}

	accumulatedBalance := vr.accumulatedBalance
	return accumulatedBalance+transaction.GetTransferredValue() > vr.initialBalance
}
