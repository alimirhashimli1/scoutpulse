package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPageClamps(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		offset     int
		wantLimit  int
		wantOffset int
	}{
		{"defaults", 0, 0, DefaultPageSize, 0},
		{"negative limit falls back", -5, 0, DefaultPageSize, 0},
		{"oversized limit is capped", 100_000, 0, MaxPageSize, 0},
		{"negative offset is zeroed", 10, -3, 10, 0},
		{"valid values pass through", 50, 100, 50, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := NewPage(tt.limit, tt.offset)
			assert.Equal(t, tt.wantLimit, page.Limit)
			assert.Equal(t, tt.wantOffset, page.Offset)
		})
	}
}

// TestFetchLimit covers the sentinel row: repositories ask for one more than
// the page size so has_more is known without a second COUNT query.
func TestFetchLimit(t *testing.T) {
	assert.Equal(t, 26, NewPage(25, 0).FetchLimit())
}

func TestNewListResult(t *testing.T) {
	page := NewPage(2, 0)

	t.Run("trims the sentinel row and reports more", func(t *testing.T) {
		result := NewListResult([]int{1, 2, 3}, page)
		assert.Equal(t, []int{1, 2}, result.Items)
		assert.True(t, result.HasMore)
	})

	t.Run("a full page with no sentinel means no more", func(t *testing.T) {
		result := NewListResult([]int{1, 2}, page)
		assert.Equal(t, []int{1, 2}, result.Items)
		assert.False(t, result.HasMore)
	})

	t.Run("empty serialises as [] not null", func(t *testing.T) {
		// A null would force every client to nil-check before iterating.
		raw, err := json.Marshal(NewListResult([]int(nil), page))
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"items":[]`)
	})
}
