package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMinor_NoFloatDrift is the reason money is an integer type. Summing a
// decimal amount repeatedly in float64 accumulates error; in minor units it
// cannot.
func TestMinor_NoFloatDrift(t *testing.T) {
	var asFloat float64
	var asMinor Minor

	// A fee of 0.10 added a hundred times should be exactly 10.
	for i := 0; i < 100; i++ {
		asFloat += 0.10
		asMinor += 10
	}

	assert.NotEqual(t, 10.0, asFloat, "float64 is expected to drift, which is why it is not used")
	assert.Equal(t, Minor(1000), asMinor)
	assert.Equal(t, "10.00", asMinor.String())
}

func TestMinor_String(t *testing.T) {
	tests := []struct {
		amount Minor
		want   string
	}{
		{0, "0.00"},
		{5, "0.05"},
		{50, "0.50"},
		{100, "1.00"},
		{123_456_789, "1234567.89"},
		{-2550, "-25.50"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.amount.String())
	}
}

func TestMinor_JSONIsAnInteger(t *testing.T) {
	// Serialising as an integer count of minor units means no precision is
	// lost in transit, and a client cannot mistake the value for a decimal.
	raw, err := json.Marshal(Minor(150_000_000))
	require.NoError(t, err)
	assert.Equal(t, "150000000", string(raw))
}

func TestMinor_Unmarshal(t *testing.T) {
	t.Run("rejects decimals", func(t *testing.T) {
		// Accepting 1.5 would leave the caller guessing whether it meant one
		// cent or two, so it is refused rather than truncated.
		var m Minor
		assert.Error(t, json.Unmarshal([]byte("1.5"), &m))
		assert.Error(t, json.Unmarshal([]byte(`"1.5"`), &m))
		assert.Error(t, json.Unmarshal([]byte(`"not a number"`), &m))
	})

	t.Run("accepts integers, bare or quoted", func(t *testing.T) {
		// The quoted form exists for JavaScript clients, which lose precision
		// on integers above 2^53.
		var m Minor
		require.NoError(t, json.Unmarshal([]byte("2500"), &m))
		assert.Equal(t, Minor(2500), m)

		require.NoError(t, json.Unmarshal([]byte(`"2500"`), &m))
		assert.Equal(t, Minor(2500), m)
	})

	t.Run("round trips", func(t *testing.T) {
		original := Minor(150_000_000)
		raw, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded Minor
		require.NoError(t, json.Unmarshal(raw, &decoded))
		assert.Equal(t, original, decoded)
	})
}

func TestFromUnitsRounds(t *testing.T) {
	assert.Equal(t, Minor(1050), FromUnits(10.50))
	assert.Equal(t, Minor(1), FromUnits(0.005), "should round rather than truncate")
	assert.Equal(t, 10.50, FromUnits(10.50).Units())
}

func TestNewMoneyDefaultsCurrency(t *testing.T) {
	assert.Equal(t, DefaultCurrency, NewMoney(100, "").Currency)
	assert.Equal(t, "GBP", NewMoney(100, "GBP").Currency)
	assert.Equal(t, "1.00 EUR", NewMoney(100, "").String())
}
