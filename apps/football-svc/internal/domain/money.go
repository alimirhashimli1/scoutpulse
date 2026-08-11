package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// DefaultCurrency is used when a caller does not specify one. Transfer fees
// and valuations in this dataset are quoted in euros by convention.
const DefaultCurrency = "EUR"

// Minor represents an amount in a currency's smallest unit (cents for EUR).
//
// Money is never a float here. Binary floating point cannot represent most
// decimal amounts exactly, and the error compounds through every sum and
// comparison -- which matters when the data is transfer fees and market values.
type Minor int64

// FromUnits converts a major-unit amount (euros) to minor units (cents).
// Callers holding a float should prefer parsing the decimal string.
func FromUnits(units float64) Minor {
	return Minor(math.Round(units * 100))
}

// Units returns the amount in major units. It is for display only; do not
// compute with the result.
func (m Minor) Units() float64 { return float64(m) / 100 }

// String renders the amount with two decimal places, without a currency
// symbol.
func (m Minor) String() string {
	neg := m < 0
	v := m
	if neg {
		v = -v
	}

	s := fmt.Sprintf("%d.%02d", int64(v)/100, int64(v)%100)
	if neg {
		return "-" + s
	}
	return s
}

// MarshalJSON emits the amount as an integer count of minor units, so no
// precision is lost in transit and clients cannot mistake it for a decimal.
func (m Minor) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(m), 10)), nil
}

// UnmarshalJSON accepts an integer number of minor units, either bare (2500)
// or quoted ("2500"). The quoted form is allowed because JavaScript clients
// lose precision on integers above 2^53 and often send large amounts as
// strings.
//
// A decimal value is rejected in both forms rather than silently truncated:
// accepting 1.5 would leave the caller guessing whether it meant one cent or
// two.
func (m *Minor) UnmarshalJSON(data []byte) error {
	// null means "not supplied", as it does for every other type in
	// encoding/json: the field keeps whatever value it already had. Rejecting
	// it here made `{"market_value_minor": null}` an error with a message
	// about integers, which describes neither the input nor the fix.
	if string(data) == "null" {
		return nil
	}

	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("money must be an integer number of minor units: %w", err)
	}

	v, err := strconv.ParseInt(n.String(), 10, 64)
	if err != nil {
		return fmt.Errorf("money must be an integer number of minor units, got %s", n.String())
	}

	*m = Minor(v)
	return nil
}

// Money pairs an amount with its currency.
type Money struct {
	Amount   Minor  `json:"amount_minor"`
	Currency string `json:"currency"`
}

// NewMoney builds a Money, defaulting a blank currency to DefaultCurrency.
func NewMoney(amount Minor, currency string) Money {
	if currency == "" {
		currency = DefaultCurrency
	}
	return Money{Amount: amount, Currency: currency}
}

func (m Money) String() string { return m.Amount.String() + " " + m.Currency }
