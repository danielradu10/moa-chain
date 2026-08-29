package txpipeline

import (
	"crypto/sha256"
	"encoding/binary"
)

// ComputeTxHash computes the canonical transaction hash from the fields that
// uniquely identify a transaction. The domain separator ensures these hashes
// are distinct from any other SHA-256 usage in the codebase.
func ComputeTxHash(sender string, nonce uint64, prompt string, tip uint64, timestamp uint64) []byte {
	h := sha256.New()
	h.Write([]byte("moa-chain-transaction-v1"))
	hashWriteString(h, sender)
	hashWriteUint64(h, nonce)
	hashWriteString(h, prompt)
	hashWriteUint64(h, tip)
	hashWriteUint64(h, timestamp)
	return h.Sum(nil)
}

func hashWriteUint64(h interface{ Write([]byte) (int, error) }, v uint64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], v)
	_, _ = h.Write(buf[:])
}

func hashWriteString(h interface{ Write([]byte) (int, error) }, s string) {
	hashWriteUint64(h, uint64(len(s)))
	_, _ = h.Write([]byte(s))
}
