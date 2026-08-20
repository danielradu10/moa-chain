package txpipeline

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrecomputedStore_LabelsRoundtrip(t *testing.T) {
	store := NewPrecomputedStore()
	hash := []byte("tx-abc")
	labels := []string{"math", "science"}

	store.StoreLabels(hash, labels)

	got, ok := store.GetLabels(hash)
	require.True(t, ok)
	require.Equal(t, labels, got)
}

func TestPrecomputedStore_AnswerRoundtrip(t *testing.T) {
	store := NewPrecomputedStore()
	hash := []byte("tx-abc")
	answer := "42 is the answer"

	store.StoreAnswer(hash, answer)

	got, ok := store.GetAnswer(hash)
	require.True(t, ok)
	require.Equal(t, answer, got)
}

func TestPrecomputedStore_MissingReturnsNotFound(t *testing.T) {
	store := NewPrecomputedStore()
	hash := []byte("tx-unknown")

	_, ok := store.GetLabels(hash)
	require.False(t, ok)

	_, ok = store.GetAnswer(hash)
	require.False(t, ok)
}

func TestPrecomputedStore_RemoveClearsBothEntries(t *testing.T) {
	store := NewPrecomputedStore()
	hash := []byte("tx-abc")

	store.StoreLabels(hash, []string{"math"})
	store.StoreAnswer(hash, "some answer")
	store.Remove(hash)

	_, ok := store.GetLabels(hash)
	require.False(t, ok)

	_, ok = store.GetAnswer(hash)
	require.False(t, ok)
}

func TestPrecomputedStore_RemoveIsIdempotent(t *testing.T) {
	store := NewPrecomputedStore()
	hash := []byte("tx-abc")

	require.NotPanics(t, func() {
		store.Remove(hash)
		store.Remove(hash)
	})
}

func TestPrecomputedStore_IndependentHashes(t *testing.T) {
	store := NewPrecomputedStore()

	store.StoreLabels([]byte("tx-1"), []string{"math"})
	store.StoreAnswer([]byte("tx-2"), "answer for tx-2")

	_, ok := store.GetAnswer([]byte("tx-1"))
	require.False(t, ok)

	_, ok = store.GetLabels([]byte("tx-2"))
	require.False(t, ok)
}

func TestPrecomputedStore_ConcurrentAccess(t *testing.T) {
	store := NewPrecomputedStore()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	for i := range goroutines {
		hash := []byte{byte(i)}

		go func() {
			defer wg.Done()
			store.StoreLabels(hash, []string{"label"})
		}()

		go func() {
			defer wg.Done()
			store.StoreAnswer(hash, "answer")
		}()

		go func() {
			defer wg.Done()
			store.GetLabels(hash)
			store.GetAnswer(hash)
		}()
	}

	wg.Wait()
}
