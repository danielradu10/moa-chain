package processor

import (
	"bytes"

	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/transactionprocessing"
)

const (
	numGeneratedLabelsByLeader = 3
	numGeneratedLabelsByMe     = 6
)

var possibleSubDomains = map[string]struct{}{
	"systems_programming":                {},
	"web_front_end":                      {},
	"back_end_with_apis":                 {},
	"ml_ai_engineering":                  {},
	"data_engineering":                   {},
	"dev_ops":                            {},
	"security":                           {},
	"mobile_dev":                         {},
	"test_engineering_and_qa_automation": {},
	"blockchain_engineering":             {},
	"cloud_engineering":                  {},
	"databases":                          {},
}

type txProcessor struct {
	accountsProvider state.AccountsProvider
	accountState     state.AccountsState
	mempool          mempool.Mempool
	labeler          agent.Labeler
}

func NewTxProcessor(
	accountsProvider state.AccountsProvider,
	accountState state.AccountsState,
	labeler agent.Labeler,
	mempool mempool.Mempool,
) (*txProcessor, error) {
	return &txProcessor{
		accountsProvider: accountsProvider,
		accountState:     accountState,
		labeler:          labeler,
		mempool:          mempool,
	}, nil
}

func (tp *txProcessor) ProcessTransactionEconomically(tx data.Transaction, miniRound data.MiniRound) (uint64, error) {
	sender := tx.GetSender()

	senderAccount, err := tp.accountsProvider.LoadAccount(string(sender))
	if err != nil {
		return 0, err
	}

	escrowAccount, err := tp.accountsProvider.LoadEscrowAccount()
	if err != nil {
		return 0, err
	}

	err = tp.validateTransaction(tx, senderAccount)
	if err != nil {
		return 0, err
	}

	switch miniRound {
	case data.MiniRoundOne:
		return tx.GetEstimatedConsumption(), tp.reserveTransaction(tx, senderAccount, escrowAccount)
	case data.MiniRoundTwo:
		return 0, transactionprocessing.ErrNotImplemented
	case data.MiniRoundThree:
		return 0, transactionprocessing.ErrNotImplemented
	default:
		return 0, transactionprocessing.ErrUnsupportedMiniRound
	}
}

func (tp *txProcessor) validateTransaction(
	tx data.Transaction,
	senderAccount state.AccountHandler,
) error {
	err := tp.validateTransactionNonce(tx, senderAccount)
	if err != nil {
		return err
	}

	err = tp.validateTransactionBalance(tx, senderAccount)
	if err != nil {
		return err
	}

	return nil
}

func (tp *txProcessor) validateTransactionNonce(
	tx data.Transaction,
	senderAccount state.AccountHandler,
) error {
	txNonce := tx.GetNonce()

	senderNonce, err := senderAccount.Nonce()
	if err != nil {
		return err
	}

	if txNonce != senderNonce {
		return transactionprocessing.ErrWrongTransactionNonce
	}

	return nil
}

func (tp *txProcessor) validateTransactionBalance(
	tx data.Transaction,
	senderAccount state.AccountHandler,
) error {
	senderBalance, err := senderAccount.Balance()
	if err != nil {
		return err
	}

	reservedBudget := tp.computeReservedBudget(tx)
	if reservedBudget > senderBalance {
		return transactionprocessing.ErrWrongTransactionBalance
	}

	return nil
}

func (tp *txProcessor) LabelTransaction(tx data.Transaction, amILeader bool) ([]string, error) {
	return tp.labeler.Label(tx, amILeader)
}

func (tp *txProcessor) ValidateLabels(
	labelsGeneratedByLeader []string,
	labelsGeneratedByMe []string,
) error {
	err := tp.validateMaxLabels(labelsGeneratedByLeader, numGeneratedLabelsByLeader, transactionprocessing.ErrLeaderGeneratedTooManyLabels)
	if err != nil {
		return err
	}

	err = tp.validateMaxLabels(labelsGeneratedByMe, numGeneratedLabelsByMe, transactionprocessing.ErrValidatorGeneratedTooManyLabels)
	if err != nil {
		return err
	}

	leaderLabelSet, err := tp.buildValidatedLabelSet(
		labelsGeneratedByLeader,
		transactionprocessing.ErrLeaderProposedDuplicatedLabels,
	)
	if err != nil {
		return err
	}

	myLabelSet, err := tp.buildValidatedLabelSet(
		labelsGeneratedByMe,
		transactionprocessing.ErrValidatorGeneratedDuplicatedLabels,
	)
	if err != nil {
		return err
	}

	err = tp.validateLeaderLabelsAreContainedInMyLabels(leaderLabelSet, myLabelSet)
	if err != nil {
		return err
	}

	return nil
}

func (tp *txProcessor) validateMaxLabels(
	labels []string,
	maxAllowed int,
	errToReturn error,
) error {
	if len(labels) > maxAllowed {
		return errToReturn
	}

	return nil
}

func (tp *txProcessor) buildValidatedLabelSet(
	labels []string,
	duplicatedLabelErr error,
) (map[string]struct{}, error) {
	labelSet := make(map[string]struct{}, len(labels))

	for _, label := range labels {
		err := tp.validateKnownLabel(label)
		if err != nil {
			return nil, err
		}

		err = tp.validateLabelNotDuplicated(label, labelSet, duplicatedLabelErr)
		if err != nil {
			return nil, err
		}

		labelSet[label] = struct{}{}
	}

	return labelSet, nil
}

func (tp *txProcessor) validateKnownLabel(label string) error {
	_, ok := possibleSubDomains[label]
	if !ok {
		return transactionprocessing.ErrUnknownLabel
	}

	return nil
}

func (tp *txProcessor) validateLabelNotDuplicated(
	label string,
	existingLabels map[string]struct{},
	duplicatedLabelErr error,
) error {
	_, alreadyExists := existingLabels[label]
	if alreadyExists {
		return duplicatedLabelErr
	}

	return nil
}

func (tp *txProcessor) validateLeaderLabelsAreContainedInMyLabels(
	leaderLabelSet map[string]struct{},
	myLabelSet map[string]struct{},
) error {
	for label := range leaderLabelSet {
		_, ok := myLabelSet[label]
		if !ok {
			return transactionprocessing.ErrLabelIsNotValid
		}
	}

	return nil
}

func (tp *txProcessor) computeReservedBudget(tx data.Transaction) uint64 {
	return tx.GetEstimatedFee() + tx.GetTip()
}

func (tp *txProcessor) reserveTransaction(
	tx data.Transaction,
	senderAccount state.AccountHandler,
	escrowAccount state.AccountHandler,
) error {
	err := senderAccount.IncreaseNonce(1)
	if err != nil {
		return err
	}

	reservedBudget := tp.computeReservedBudget(tx)

	err = senderAccount.DecreaseBalance(reservedBudget)
	if err != nil {
		return err
	}

	err = escrowAccount.IncreaseBalance(reservedBudget)
	if err != nil {
		return err
	}

	return nil
}

func (tp *txProcessor) ValidateTransactionsOrdering(
	previousTransaction data.Transaction,
	currentTransaction data.Transaction,
) error {
	prevScore := previousTransaction.GetEstimatedScore()
	currScore := currentTransaction.GetEstimatedScore()

	if prevScore < currScore {
		return transactionprocessing.ErrTxsDoNotRespectProtocolOrder
	}

	if prevScore > currScore {
		return nil
	}

	prevConsumption := previousTransaction.GetEstimatedConsumption()
	currConsumption := currentTransaction.GetEstimatedConsumption()

	if prevConsumption > currConsumption {
		return transactionprocessing.ErrTxsDoNotRespectProtocolOrder
	}

	if prevConsumption < currConsumption {
		return nil
	}

	if bytes.Compare(previousTransaction.GetTxHash(), currentTransaction.GetTxHash()) > 0 {
		return transactionprocessing.ErrTxsDoNotRespectProtocolOrder
	}

	return nil
}

func (tp *txProcessor) SelectTransactions() []data.Transaction {
	selectedTxs := tp.mempool.SelectTransactions(tp.accountState)
	return selectedTxs
}
