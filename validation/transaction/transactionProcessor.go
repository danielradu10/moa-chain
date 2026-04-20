package transaction

import (
	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/validation"
)

const (
	numGeneratedLabelsByLeader = 3
	numGeneratedLabelsByMe     = 6
)

var possibleSubDomains = map[string]struct{}{
	"algorithms":      {},
	"architecture":    {},
	"software_design": {},
	"security":        {},
	"smart_contracts": {},
	"apis":            {},
	"testing":         {},
	"performance":     {},
	"parallelism":     {},
	"ml":              {},
	"cloud":           {},
	"databases":       {},
}

type txProcessor struct {
	accountsProvider state.AccountsProvider
	labeler          agent.Labeler
}

func NewTxProcessor(
	accountsProvider state.AccountsProvider,
	labeler agent.Labeler,
) (*txProcessor, error) {
	return &txProcessor{
		accountsProvider: accountsProvider,
		labeler:          labeler,
	}, nil
}

func (tp *txProcessor) ProcessTransaction(tx data.Transaction, miniRound validation.MiniRound) (uint64, error) {
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
	case validation.MiniRoundOne:
		return tx.GetEstimatedConsumption(), tp.reserveTransaction(tx, senderAccount, escrowAccount)
	case validation.MiniRoundTwo:
		return 0, validation.ErrNotImplemented
	case validation.MiniRoundThree:
		return 0, validation.ErrNotImplemented
	default:
		return 0, validation.ErrUnsupportedMiniRound
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

	labelsGeneratedByMe, err := tp.labelTransaction(tx)
	if err != nil {
		return err
	}

	err = tp.validateLabels(tx.GetDomainLabels(), labelsGeneratedByMe)
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
		return validation.ErrWrongTransactionNonce
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
		return validation.ErrWrongTransactionBalance
	}

	return nil
}

func (tp *txProcessor) labelTransaction(tx data.Transaction) ([]string, error) {
	return tp.labeler.Label(tx)
}

func (tp *txProcessor) validateLabels(
	labelsGeneratedByLeader []string,
	labelsGeneratedByMe []string,
) error {
	err := tp.validateMaxLabels(labelsGeneratedByLeader, numGeneratedLabelsByLeader, validation.ErrLeaderGeneratedTooManyLabels)
	if err != nil {
		return err
	}

	err = tp.validateMaxLabels(labelsGeneratedByMe, numGeneratedLabelsByMe, validation.ErrValidatorGeneratedTooManyLabels)
	if err != nil {
		return err
	}

	leaderLabelSet, err := tp.buildValidatedLabelSet(
		labelsGeneratedByLeader,
		validation.ErrLeaderProposedDuplicatedLabels,
	)
	if err != nil {
		return err
	}

	myLabelSet, err := tp.buildValidatedLabelSet(
		labelsGeneratedByMe,
		validation.ErrValidatorGeneratedDuplicatedLabels,
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
		return validation.ErrUnknownLabel
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
			return validation.ErrLabelIsNotValid
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
