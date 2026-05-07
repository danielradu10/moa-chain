package testscommon

import (
	"moa-chain/data"
)

type LabelerStub struct {
	Err            error
	LabelsByTxHash map[string][]string
	LabelCalled    func(tx data.Transaction, amILeader bool) ([]string, error)
}

func (tl *LabelerStub) Label(tx data.Transaction, amILeader bool) ([]string, error) {
	if tl.LabelCalled != nil {
		return tl.LabelCalled(tx, amILeader)
	}

	if tl.Err != nil {
		return nil, tl.Err
	}

	labels, ok := tl.LabelsByTxHash[string(tx.GetTxHash())]
	if !ok {
		return []string{}, nil
	}

	return labels, nil
}
