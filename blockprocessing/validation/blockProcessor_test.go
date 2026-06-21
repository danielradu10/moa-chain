package validation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/blockprocessing"
	"moa-chain/data"
	"moa-chain/testscommon"
	"moa-chain/transactionprocessing"
)

func TestBlockProcessor_ValidateBlock(t *testing.T) {
	t.Parallel()

	t.Run("should return ErrNilBlock when block is nil", func(t *testing.T) {
		t.Parallel()

		blockProcessor := &blockProcessor{}

		_, _, _, err := blockProcessor.ValidateBlock(nil)

		require.Equal(t, blockprocessing.ErrNilBlock, err)
	})

	t.Run("should return current block header error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("current block header error")

		blockProcessor := &blockProcessor{
			blockprocessing.Base{
				BlockchainState: &testscommon.BlockchainStateStub{
					CurrentBlockHeaderErr: expectedErr,
				},
			},
		}

		_, _, _, err := blockProcessor.ValidateBlock(&data.Block{})

		require.Equal(t, expectedErr, err)
	})

	t.Run("should return ErrBlockNonceNotContinuous when block nonce is not continuous", func(t *testing.T) {
		t.Parallel()

		currentHeader := createCurrentBlockHeader(data.MiniRoundOne)
		proposedBlock := createValidBlockForCurrentHeader(currentHeader, nil, map[string]uint64{})
		proposedBlock.Header.Nonce = currentHeader.Nonce + 2

		blockProcessor := &blockProcessor{
			blockprocessing.Base{
				BlockchainState: &testscommon.BlockchainStateStub{
					CurrentBlockHeaderValue: currentHeader,
				},
			},
		}

		_, _, _, err := blockProcessor.ValidateBlock(proposedBlock)

		require.Equal(t, blockprocessing.ErrBlockNonceNotContinuous, err)
	})

	t.Run("should return snapshot factory error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("snapshot factory error")
		currentHeader := createCurrentBlockHeader(data.MiniRoundOne)
		proposedBlock := createValidBlockForCurrentHeader(currentHeader, nil, map[string]uint64{})

		blockProcessor := &blockProcessor{
			blockprocessing.Base{
				BlockchainState: &testscommon.BlockchainStateStub{
					CurrentBlockHeaderValue: currentHeader,
				},
				AccountsSnapshotFactory: &testscommon.AccountsSnapshotFactoryStub{
					CreateSnapshotErr: expectedErr,
				},
			},
		}

		_, _, _, err := blockProcessor.ValidateBlock(proposedBlock)

		require.Equal(t, expectedErr, err)
	})

	t.Run("should return transaction validation error when block contains invalid transaction", func(t *testing.T) {
		t.Parallel()

		currentHeader := createCurrentBlockHeader(data.MiniRoundOne)

		tx := createTestTransaction(testTransactionArgs{
			nonce:                0,
			sender:               "alice",
			txHash:               "txHash1",
			estimatedScore:       100,
			estimatedConsumption: 100,
			estimatedFee:         10,
			tip:                  5,
			domainLabels:         []string{"security"},
		})

		proposedBlock := createValidBlockForCurrentHeader(
			currentHeader,
			[]data.Transaction{tx},
			map[string]uint64{
				"security": 1,
			},
		)

		snapshot := &testscommon.AccountsSnapshotStub{
			Accounts: map[string]*testscommon.AccountHandlerStub{
				"alice": testscommon.NewAccountHandlerStub(1, 100),
			},
			EscrowAccount: testscommon.NewAccountHandlerStub(0, 0),
		}

		blockProcessor := &blockProcessor{
			blockprocessing.Base{
				BlockchainState: &testscommon.BlockchainStateStub{
					CurrentBlockHeaderValue: currentHeader,
				},
				AccountsSnapshotFactory: &testscommon.AccountsSnapshotFactoryStub{
					Snapshot: snapshot,
				},
				Labeler: &testscommon.LabelerStub{
					LabelsByTxHash: map[string][]string{
						"txHash1": {"security", "cloud_engineering"},
					},
				},
			},
		}

		_, _, _, err := blockProcessor.ValidateBlock(proposedBlock)

		require.Equal(t, transactionprocessing.ErrWrongTransactionNonce, err)
		require.True(t, snapshot.DiscardCalled)
	})

	t.Run("should validate block successfully and discard snapshot", func(t *testing.T) {
		t.Parallel()

		currentHeader := createCurrentBlockHeader(data.MiniRoundOne)

		tx := createTestTransaction(testTransactionArgs{
			nonce:                0,
			sender:               "alice",
			txHash:               "txHash1",
			estimatedScore:       100,
			estimatedConsumption: 100,
			estimatedFee:         10,
			tip:                  5,
			domainLabels:         []string{"security", "cloud_engineering", "databases"},
		})

		proposedBlock := createValidBlockForCurrentHeader(
			currentHeader,
			[]data.Transaction{tx},
			map[string]uint64{
				"security":          1,
				"cloud_engineering": 1,
				"databases":         1,
			},
		)

		snapshot := &testscommon.AccountsSnapshotStub{
			Accounts: map[string]*testscommon.AccountHandlerStub{
				"alice": testscommon.NewAccountHandlerStub(0, 100),
			},
			EscrowAccount: testscommon.NewAccountHandlerStub(0, 0),
		}

		blockProcessor := &blockProcessor{
			blockprocessing.Base{
				BlockchainState: &testscommon.BlockchainStateStub{
					CurrentBlockHeaderValue: currentHeader,
				},
				AccountsSnapshotFactory: &testscommon.AccountsSnapshotFactoryStub{
					Snapshot: snapshot,
				},
				Labeler: &testscommon.LabelerStub{
					LabelsByTxHash: map[string][]string{
						"txHash1": {
							"security",
							"cloud_engineering",
							"databases",
							"dev_ops",
						},
					},
				},
			},
		}

		_, _, _, err := blockProcessor.ValidateBlock(proposedBlock)

		require.NoError(t, err)
		require.True(t, snapshot.DiscardCalled)
	})
}

func TestBlockProcessor_ExecuteBlockPrompts(t *testing.T) {
	t.Parallel()

	t.Run("should execute block prompts and discard snapshot", func(t *testing.T) {
		t.Parallel()

		tx1 := createTestTransaction(testTransactionArgs{
			txHash: "txHash1",
		})
		tx2 := createTestTransaction(testTransactionArgs{
			txHash: "txHash2",
		})
		snapshot := &testscommon.AccountsSnapshotStub{}
		blockProcessor := &blockProcessor{
			Base: blockprocessing.Base{
				AccountsSnapshotFactory: &testscommon.AccountsSnapshotFactoryStub{
					Snapshot: snapshot,
				},
				Labeler: &testscommon.LabelerStub{
					AnswersByTxHash: map[string]string{
						"txHash1": "answer one",
						"txHash2": "answer two",
					},
				},
			},
		}

		result, err := blockProcessor.ExecuteBlockPrompts(&data.BlockBody{
			Transactions: []data.Transaction{tx1, tx2},
		})

		require.NoError(t, err)
		require.Len(t, result.TxsResults, 2)
		require.Equal(t, []byte("txHash1"), result.TxsResults[0].TxHash)
		require.Equal(t, "answer one", result.TxsResults[0].Answer)
		require.Greater(t, result.TxsResults[0].ActualConsumption, uint64(0))
		require.Equal(t, []byte("txHash2"), result.TxsResults[1].TxHash)
		require.Equal(t, "answer two", result.TxsResults[1].Answer)
		require.Greater(t, result.TxsResults[1].ActualConsumption, uint64(0))
		require.Equal(t, result.TxsResults[0].ActualConsumption+result.TxsResults[1].ActualConsumption, result.TotalConsumption)
		require.True(t, snapshot.DiscardCalled)
	})

	t.Run("should return snapshot factory error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("snapshot factory error")
		blockProcessor := &blockProcessor{
			Base: blockprocessing.Base{
				AccountsSnapshotFactory: &testscommon.AccountsSnapshotFactoryStub{
					CreateSnapshotErr: expectedErr,
				},
			},
		}

		result, err := blockProcessor.ExecuteBlockPrompts(&data.BlockBody{})

		require.Nil(t, result)
		require.Equal(t, expectedErr, err)
	})
}

func TestBlockProcessor_validateRoundAndMiniRoundContinuity(t *testing.T) {
	t.Parallel()

	t.Run("should validate transition from mini round one to mini round two in same round", func(t *testing.T) {
		t.Parallel()

		blockProcessor := &blockProcessor{}
		currentHeader := createCurrentBlockHeader(data.MiniRoundOne)
		nextHeader := createValidNextHeaderForCurrentHeader(currentHeader)

		err := blockProcessor.validateRoundAndMiniRoundContinuity(nextHeader, currentHeader)

		require.NoError(t, err)
	})

	t.Run("should validate transition from mini round two to mini round three in same round", func(t *testing.T) {
		t.Parallel()

		blockProcessor := &blockProcessor{}
		currentHeader := createCurrentBlockHeader(data.MiniRoundTwo)
		nextHeader := createValidNextHeaderForCurrentHeader(currentHeader)

		err := blockProcessor.validateRoundAndMiniRoundContinuity(nextHeader, currentHeader)

		require.NoError(t, err)
	})

	t.Run("should validate transition from mini round three to mini round one in next round", func(t *testing.T) {
		t.Parallel()

		blockProcessor := &blockProcessor{}
		currentHeader := createCurrentBlockHeader(data.MiniRoundThree)
		nextHeader := createValidNextHeaderForCurrentHeader(currentHeader)

		err := blockProcessor.validateRoundAndMiniRoundContinuity(nextHeader, currentHeader)

		require.NoError(t, err)
	})

	t.Run("should return ErrWrongMiniBlockRound for invalid mini round transition", func(t *testing.T) {
		t.Parallel()

		blockProcessor := &blockProcessor{}
		currentHeader := createCurrentBlockHeader(data.MiniRoundOne)
		nextHeader := createValidNextHeaderForCurrentHeader(currentHeader)
		nextHeader.MiniRound = uint64(data.MiniRoundThree)

		err := blockProcessor.validateRoundAndMiniRoundContinuity(nextHeader, currentHeader)

		require.Equal(t, blockprocessing.ErrWrongMiniBlockRound, err)
	})

	t.Run("should return ErrWrongMiniBlockRound for invalid round transition", func(t *testing.T) {
		t.Parallel()

		blockProcessor := &blockProcessor{}
		currentHeader := createCurrentBlockHeader(data.MiniRoundThree)
		nextHeader := createValidNextHeaderForCurrentHeader(currentHeader)
		nextHeader.Round = currentHeader.Round

		err := blockProcessor.validateRoundAndMiniRoundContinuity(nextHeader, currentHeader)

		require.Equal(t, blockprocessing.ErrWrongMiniBlockRound, err)
	})
}

func TestBlockProcessor_validateBlockBody(t *testing.T) {
	t.Parallel()

	t.Run("should return ErrDuplicatedTransaction when block contains duplicate transaction hashes", func(t *testing.T) {
		t.Parallel()

		blockProcessor := &blockProcessor{}
		txProcessor := &testscommon.TxProcessorStub{
			ProcessTransactionCalled: func(tx data.Transaction, miniRound data.MiniRound) (uint64, error) {
				return 100, nil
			},
		}

		tx1 := createTestTransaction(testTransactionArgs{
			txHash:               "sameHash",
			estimatedScore:       100,
			estimatedConsumption: 100,
			domainLabels:         []string{"security"},
		})
		tx2 := createTestTransaction(testTransactionArgs{
			txHash:               "sameHash",
			estimatedScore:       90,
			estimatedConsumption: 100,
			domainLabels:         []string{"cloud_engineering"},
		})

		body := &data.BlockBody{
			Transactions: []data.Transaction{tx1, tx2},
		}

		_, err := blockProcessor.validateAndExecuteBlockBody(body, txProcessor)

		require.Equal(t, blockprocessing.ErrDuplicatedTransaction, err)
	})

	t.Run("should return ErrTxsDoNotRespectProtocolOrder when transactions are not ordered", func(t *testing.T) {
		t.Parallel()

		blockProcessor := &blockProcessor{}
		txProcessor := &testscommon.TxProcessorStub{
			ProcessTransactionCalled: func(tx data.Transaction, miniRound data.MiniRound) (uint64, error) {
				return 100, nil
			},
			ValidateTransactionsOrderingCalled: func(previousTransaction data.Transaction, currentTransaction data.Transaction) error {
				return transactionprocessing.ErrTxsDoNotRespectProtocolOrder
			},
		}

		tx1 := createTestTransaction(testTransactionArgs{
			txHash:               "txHash1",
			estimatedScore:       90,
			estimatedConsumption: 100,
			domainLabels:         []string{"security"},
		})
		tx2 := createTestTransaction(testTransactionArgs{
			txHash:               "txHash2",
			estimatedScore:       100,
			estimatedConsumption: 100,
			domainLabels:         []string{"cloud_engineering"},
		})

		body := &data.BlockBody{
			Transactions: []data.Transaction{tx1, tx2},
		}

		_, err := blockProcessor.validateAndExecuteBlockBody(body, txProcessor)

		require.Equal(t, transactionprocessing.ErrTxsDoNotRespectProtocolOrder, err)
	})

	t.Run("should return ErrBlockConsumptionReached when block consumption exceeds limit", func(t *testing.T) {
		t.Parallel()

		blockProcessor := &blockProcessor{}
		txProcessor := &testscommon.TxProcessorStub{
			ProcessTransactionCalled: func(tx data.Transaction, miniRound data.MiniRound) (uint64, error) {
				if string(tx.GetTxHash()) == "txHash1" {
					return 9000, nil
				}

				return 2000, nil
			},
		}

		tx1 := createTestTransaction(testTransactionArgs{
			txHash:               "txHash1",
			estimatedScore:       100,
			estimatedConsumption: 9000,
			domainLabels:         []string{"security"},
		})
		tx2 := createTestTransaction(testTransactionArgs{
			txHash:               "txHash2",
			estimatedScore:       90,
			estimatedConsumption: 2000,
			domainLabels:         []string{"cloud_engineering"},
		})

		body := &data.BlockBody{
			Transactions: []data.Transaction{tx1, tx2},
		}

		_, err := blockProcessor.validateAndExecuteBlockBody(body, txProcessor)

		require.Equal(t, blockprocessing.ErrBlockConsumptionReached, err)
	})

	t.Run("should validate block body successfully", func(t *testing.T) {
		t.Parallel()

		blockProcessor := &blockProcessor{}
		txProcessor := &testscommon.TxProcessorStub{
			ProcessTransactionCalled: func(tx data.Transaction, miniRound data.MiniRound) (uint64, error) {
				return tx.GetEstimatedConsumption(), nil
			},
		}

		tx1 := createTestTransaction(testTransactionArgs{
			txHash:               "txHash1",
			estimatedScore:       100,
			estimatedConsumption: 100,
			domainLabels:         []string{"security", "cloud_engineering"},
		})
		tx2 := createTestTransaction(testTransactionArgs{
			txHash:               "txHash2",
			estimatedScore:       90,
			estimatedConsumption: 200,
			domainLabels:         []string{"security", "databases"},
		})

		body := &data.BlockBody{
			Transactions: []data.Transaction{tx1, tx2},
		}

		_, err := blockProcessor.validateAndExecuteBlockBody(body, txProcessor)

		require.NoError(t, err)
	})
}

type testTransactionArgs struct {
	nonce                uint64
	sender               string
	txHash               string
	estimatedScore       uint64
	estimatedConsumption uint64
	estimatedFee         uint64
	tip                  uint64
	domainLabels         []string
}

func createTestTransaction(args testTransactionArgs) *testscommon.TransactionStub {
	ts := &testscommon.TransactionStub{}
	ts.SetNonce(args.nonce)
	ts.SetSender([]byte(args.sender))
	ts.SetTxHash([]byte(args.txHash))
	ts.SetEstimatedConsumption(args.estimatedConsumption)
	ts.SetEstimatedFee(args.estimatedFee)
	ts.SetTip(args.tip)
	ts.SetDomainLabels(args.domainLabels)
	ts.SetEstimatedScore(args.estimatedScore)

	return ts
}

func createCurrentBlockHeader(currentMiniRound data.MiniRound) *data.BlockHeader {
	return &data.BlockHeader{
		HeaderHash: []byte("currentHeaderHash"),
		RootHash:   []byte("currentRootHash"),
		Nonce:      7,
		Round:      10,
		MiniRound:  uint64(currentMiniRound),
		Epoch:      1,
	}
}

func createValidNextHeaderForCurrentHeader(currentHeader *data.BlockHeader) *data.BlockHeader {
	nextMiniRound := data.MiniRoundTwo
	nextRound := currentHeader.Round

	switch data.MiniRound(currentHeader.MiniRound) {
	case data.MiniRoundOne:
		nextMiniRound = data.MiniRoundTwo
		nextRound = currentHeader.Round
	case data.MiniRoundTwo:
		nextMiniRound = data.MiniRoundThree
		nextRound = currentHeader.Round
	case data.MiniRoundThree:
		nextMiniRound = data.MiniRoundOne
		nextRound = currentHeader.Round + 1
	}

	return &data.BlockHeader{
		PreviousHash:     currentHeader.HeaderHash,
		PreviousRootHash: currentHeader.RootHash,
		Nonce:            currentHeader.Nonce + 1,
		Round:            nextRound,
		MiniRound:        uint64(nextMiniRound),
		Epoch:            currentHeader.Epoch,
	}
}

func createValidBlockForCurrentHeader(
	currentHeader *data.BlockHeader,
	transactions []data.Transaction,
	_ map[string]uint64,
) *data.Block {
	return &data.Block{
		Header: *createValidNextHeaderForCurrentHeader(currentHeader),
		Body: data.BlockBody{
			Transactions: transactions,
		},
	}
}
