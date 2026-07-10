package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/blockprocessing"
	"moa-chain/data"
)

func TestLabelsValidator_ValidateLabels(t *testing.T) {
	t.Parallel()

	lv := NewLabelsValidator()

	t.Run("valid — single non_related label", func(t *testing.T) {
		t.Parallel()

		err := lv.ValidateLabels(data.Subdomains{
			"txHash1": {"non_related"},
		})
		require.NoError(t, err)
	})

	t.Run("valid — one real label", func(t *testing.T) {
		t.Parallel()

		err := lv.ValidateLabels(data.Subdomains{
			"txHash1": {"databases"},
		})
		require.NoError(t, err)
	})

	t.Run("valid — three real labels", func(t *testing.T) {
		t.Parallel()

		err := lv.ValidateLabels(data.Subdomains{
			"txHash1": {"databases", "security", "back_end_with_apis"},
		})
		require.NoError(t, err)
	})

	t.Run("valid — multiple transactions with different label counts", func(t *testing.T) {
		t.Parallel()

		err := lv.ValidateLabels(data.Subdomains{
			"txHash1": {"databases"},
			"txHash2": {"databases", "security"},
			"txHash3": {"databases", "security", "back_end_with_apis"},
		})
		require.NoError(t, err)
	})

	t.Run("invalid — empty label list", func(t *testing.T) {
		t.Parallel()

		err := lv.ValidateLabels(data.Subdomains{
			"txHash1": {},
		})
		require.ErrorIs(t, err, blockprocessing.ErrInvalidNumSubdomains)
	})

	t.Run("invalid — more than three real labels", func(t *testing.T) {
		t.Parallel()

		err := lv.ValidateLabels(data.Subdomains{
			"txHash1": {"databases", "security", "back_end_with_apis", "dev_ops"},
		})
		require.ErrorIs(t, err, blockprocessing.ErrInvalidNumSubdomains)
	})

	t.Run("invalid — non_related mixed with real label", func(t *testing.T) {
		t.Parallel()

		err := lv.ValidateLabels(data.Subdomains{
			"txHash1": {"non_related", "databases"},
		})
		require.ErrorIs(t, err, blockprocessing.ErrNonRelatedMixedWithLabel)
	})

	t.Run("invalid — non_related with two real labels", func(t *testing.T) {
		t.Parallel()

		err := lv.ValidateLabels(data.Subdomains{
			"txHash1": {"databases", "non_related", "security"},
		})
		require.ErrorIs(t, err, blockprocessing.ErrNonRelatedMixedWithLabel)
	})

	t.Run("invalid — unknown subdomain", func(t *testing.T) {
		t.Parallel()

		err := lv.ValidateLabels(data.Subdomains{
			"txHash1": {"not_a_domain"},
		})
		require.ErrorIs(t, err, blockprocessing.ErrInvalidSubdomain)
	})

	t.Run("invalid — duplicate label", func(t *testing.T) {
		t.Parallel()

		err := lv.ValidateLabels(data.Subdomains{
			"txHash1": {"databases", "databases"},
		})
		require.ErrorIs(t, err, blockprocessing.ErrDuplicatedLabel)
	})
}

func TestLabelsValidator_AggregateLabels(t *testing.T) {
	t.Parallel()

	lv := NewLabelsValidator()

	// helpers
	makeSubdomains := func(txHash string, labels ...string) data.Subdomains {
		return data.Subdomains{txHash: labels}
	}

	t.Run("non_related reaching quorum — tx excluded from frequency map, hash in returned slice", func(t *testing.T) {
		t.Parallel()

		// 3 validators, quorum = floor(6/3)+1 = 3; all three label txHash1 as non_related.
		subdomains := []data.Subdomains{
			makeSubdomains("txHash1", "non_related"),
			makeSubdomains("txHash1", "non_related"),
			makeSubdomains("txHash1", "non_related"),
		}

		freq, nonRelated, err := lv.AggregateLabels(subdomains, 3)

		require.NoError(t, err)
		require.Empty(t, freq)
		require.Equal(t, []string{"txHash1"}, nonRelated)
	})

	t.Run("non_related below quorum — tx treated as normally labeled", func(t *testing.T) {
		t.Parallel()

		// 3 validators, quorum = 3; only 2 label as non_related, 1 labels as databases.
		// non_related does not reach quorum; databases does not either — no label makes it.
		subdomains := []data.Subdomains{
			makeSubdomains("txHash1", "non_related"),
			makeSubdomains("txHash1", "non_related"),
			makeSubdomains("txHash1", "databases"),
		}

		freq, nonRelated, err := lv.AggregateLabels(subdomains, 3)

		require.NoError(t, err)
		require.Empty(t, freq)
		require.Empty(t, nonRelated)
	})

	t.Run("real label reaching quorum — included in frequency map", func(t *testing.T) {
		t.Parallel()

		// 3 validators, quorum = 3; all agree on databases for txHash1.
		subdomains := []data.Subdomains{
			makeSubdomains("txHash1", "databases"),
			makeSubdomains("txHash1", "databases"),
			makeSubdomains("txHash1", "databases"),
		}

		freq, nonRelated, err := lv.AggregateLabels(subdomains, 3)

		require.NoError(t, err)
		require.Equal(t, data.SubdomainsFrequency{"databases": 3}, freq)
		require.Empty(t, nonRelated)
	})

	t.Run("label below quorum — not included in frequency map", func(t *testing.T) {
		t.Parallel()

		// 4 validators, quorum = floor(8/3)+1 = 3; only 2 agree on databases.
		subdomains := []data.Subdomains{
			makeSubdomains("txHash1", "databases"),
			makeSubdomains("txHash1", "databases"),
			makeSubdomains("txHash1", "security"),
			makeSubdomains("txHash1", "security"),
		}

		freq, nonRelated, err := lv.AggregateLabels(subdomains, 4)

		require.NoError(t, err)
		require.Empty(t, freq)
		require.Empty(t, nonRelated)
	})

	t.Run("mixed transactions — some non_related, some real", func(t *testing.T) {
		t.Parallel()

		// 3 validators, quorum = 3.
		// txHash1 → non_related by all three.
		// txHash2 → databases by all three.
		subdomains := []data.Subdomains{
			{"txHash1": {"non_related"}, "txHash2": {"databases"}},
			{"txHash1": {"non_related"}, "txHash2": {"databases"}},
			{"txHash1": {"non_related"}, "txHash2": {"databases"}},
		}

		freq, nonRelated, err := lv.AggregateLabels(subdomains, 3)

		require.NoError(t, err)
		require.Equal(t, data.SubdomainsFrequency{"databases": 3}, freq)
		require.Equal(t, []string{"txHash1"}, nonRelated)
	})

	t.Run("non_related tx hashes are returned sorted", func(t *testing.T) {
		t.Parallel()

		// 3 validators, quorum = 3; two transactions both non_related.
		subdomains := []data.Subdomains{
			{"txHashB": {"non_related"}, "txHashA": {"non_related"}},
			{"txHashB": {"non_related"}, "txHashA": {"non_related"}},
			{"txHashB": {"non_related"}, "txHashA": {"non_related"}},
		}

		_, nonRelated, err := lv.AggregateLabels(subdomains, 3)

		require.NoError(t, err)
		require.Equal(t, []string{"txHashA", "txHashB"}, nonRelated)
	})

	t.Run("empty input — returns empty results", func(t *testing.T) {
		t.Parallel()

		freq, nonRelated, err := lv.AggregateLabels(nil, 3)

		require.NoError(t, err)
		require.Empty(t, freq)
		require.Empty(t, nonRelated)
	})
}
