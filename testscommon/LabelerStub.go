package testscommon

import (
	"moa-chain/data"
)

type LabelerStub struct {
	Err            error
	LabelsByTxHash map[string][]string
}

func (tl *LabelerStub) Label(tx data.Transaction) ([]string, error) {
	if tl.Err != nil {
		return nil, tl.Err
	}

	labels, ok := tl.LabelsByTxHash[string(tx.GetTxHash())]
	if !ok {
		return []string{}, nil
	}

	return labels, nil
}
