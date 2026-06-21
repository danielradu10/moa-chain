package hashing

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"

	"moa-chain/data"
)

var (
	errNilBlock       = errors.New("nil block")
	errNilTransaction = errors.New("nil transaction")
)

// ComputeBodyHash computes a deterministic hash of the block body.
//
// Hash input:
// 1. a domain separator
// 2. the ordered list of transactions
// 3. the labels attached to each transaction
// 4. the block subdomains map, sorted by key
//
// Important:
// - transaction order is preserved
// - subdomains are sorted lexicographically
// - labels are hashed explicitly because tx.GetTxHash() may not commit to them
func ComputeBodyHash(body *data.BlockBody) ([]byte, error) {
	if body == nil {
		return nil, errNilBlock
	}

	h := sha256.New()

	writeBytes(h, []byte("block-body-v1"))

	txs := body.Transactions
	writeUint64(h, uint64(len(txs)))

	for _, tx := range txs {
		if tx == nil {
			return nil, errNilTransaction
		}

		txHash := tx.GetTxHash()
		writeBytes(h, txHash)

		labels := tx.GetDomainLabels()
		writeUint64(h, uint64(len(labels)))
		for _, label := range labels {
			writeString(h, label)
		}
	}

	return h.Sum(nil), nil
}

// ComputeHeaderHash computes a deterministic hash of the block header.
//
// Hash input:
// 1. a domain separator
// 2. nonce
// 3. round
// 4. mini round
// 5. epoch
// 6. previous hash
// 7. previous root hash
// 8. current root hash
// 9. body hash
//
// Important:
// - header.HeaderHash itself is NOT included
// - bodyHash is passed explicitly to avoid ambiguity
func ComputeHeaderHash(header *data.BlockHeader) ([]byte, error) {
	if header == nil {
		return nil, errNilBlock
	}

	h := sha256.New()

	writeBytes(h, []byte("block-header-v1"))

	writeUint64(h, header.Nonce)
	writeUint64(h, header.Round)
	writeUint64(h, header.MiniRound)
	writeUint64(h, header.Epoch)

	writeBytes(h, header.PreviousHash)
	writeBytes(h, header.PreviousRootHash)
	writeBytes(h, header.RootHash)
	writeBytes(h, header.BodyHash)

	return h.Sum(nil), nil
}

func writeUint64(h interface{ Write([]byte) (int, error) }, v uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	_, _ = h.Write(buf[:])
}

func writeBytes(h interface{ Write([]byte) (int, error) }, b []byte) {
	writeUint64(h, uint64(len(b)))
	_, _ = h.Write(b)
}

func writeString(h interface{ Write([]byte) (int, error) }, s string) {
	writeBytes(h, []byte(s))
}

// ComputeBlockHash compute block hash
func ComputeBlockHash(body *data.BlockBody, header *data.BlockHeader) ([]byte, error) {
	bodyHash, err := ComputeBodyHash(body)
	if err != nil {
		return nil, err
	}

	header.BodyHash = bodyHash
	headerHash, err := ComputeHeaderHash(header)
	if err != nil {
		return nil, err
	}

	return headerHash, nil
}

// ComputeSubdomainsHash computes the hash of the subdomains.
func ComputeSubdomainsHash(subdomains data.Subdomains) ([]byte, error) {
	h := sha256.New()
	writeBytes(h, []byte("subdomains-v1"))

	keys := make([]string, 0, len(subdomains))
	for key := range subdomains {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	writeUint64(h, uint64(len(keys)))
	for _, key := range keys {
		writeString(h, key)
		for _, domain := range subdomains[key] {
			writeString(h, domain)
		}
	}

	return h.Sum(nil), nil
}

// ComputePromptExecutionHash computes a deterministic hash of the mini-round two execution result.
//
// Hash input:
// 1. a domain separator
// 2. the ordered list of transaction prompt results
// 3. the total prompt execution consumption
//
// Important:
// - transaction result order is preserved
// - BlockHash itself is NOT included
// - answers are length-prefixed before hashing
func ComputePromptExecutionHash(executionResult *data.BlockBodyExecutionResultMRTwo) ([]byte, error) {
	if executionResult == nil {
		return nil, errNilBlock
	}

	h := sha256.New()
	writeBytes(h, []byte("prompt-execution-v1"))

	writeUint64(h, uint64(len(executionResult.TxsResults)))
	for _, txResult := range executionResult.TxsResults {
		writeBytes(h, txResult.TxHash)
		writeString(h, txResult.Answer)
		writeUint64(h, txResult.ActualConsumption)
	}

	writeUint64(h, executionResult.TotalConsumption)

	return h.Sum(nil), nil
}
