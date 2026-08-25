package store

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchFuzzyDocument_DeduplicatesRepeatedQueryTokens(t *testing.T) {
	t.Parallel()

	t.Run("single fuzzy token does not scale with repeats", func(t *testing.T) {
		const primary = "work"
		baseRank, baseFuzzy, baseMatch := matchFuzzyDocument("worj", primary, "", "")
		require.True(t, baseMatch)
		require.True(t, baseFuzzy)
		require.Equal(t, 1, baseRank.edits)

		for _, repeats := range []int{2, 3, 8, 200} {
			query := strings.TrimSpace(strings.Repeat("worj ", repeats))
			rank, fuzzy, match := matchFuzzyDocument(query, primary, "", "")
			assert.Equalf(t, baseMatch, match, "%d repeats must not change the match decision", repeats)
			assert.Equalf(t, baseFuzzy, fuzzy, "%d repeats must not change the fuzzy-only flag", repeats)
			assert.Equalf(t, baseRank, rank, "%d repeats must not scale the rank", repeats)
		}
	})

	t.Run("distinct tokens are preserved while their repeats collapse", func(t *testing.T) {
		const primary, description = "work", "server"
		baseRank, baseFuzzy, baseMatch := matchFuzzyDocument("worj sever", primary, description, "")
		require.True(t, baseMatch, "both distinct tokens must match")
		require.True(t, baseFuzzy)

		rank, fuzzy, match := matchFuzzyDocument("worj sever worj sever sever worj", primary, description, "")
		assert.Equal(t, baseMatch, match)
		assert.Equal(t, baseFuzzy, fuzzy)
		assert.Equal(t, baseRank, rank)
	})
}
