package domain

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/lib/pq"
)

// StringList is a Postgres TEXT[] that behaves itself in JSON.
//
// Two problems it solves, both of which otherwise land on every consumer:
//
// A bare []string does not implement sql.Scanner, so scanning a TEXT[] into
// one fails at run time rather than at compile time — the error arrives from
// the driver on the first row that has the column.
//
// And an empty or NULL array marshals as `null` unless something intervenes,
// which forces `?? []` at every use site in the frontend and makes "no
// nationalities recorded" and "the field is missing" indistinguishable. This
// always emits an array.
type StringList []string

// Scan reads a TEXT[] from the driver. NULL becomes an empty list, not nil,
// so a caller never has to distinguish the two.
func (l *StringList) Scan(src any) error {
	var arr pq.StringArray
	if err := arr.Scan(src); err != nil {
		return err
	}
	if arr == nil {
		*l = StringList{}
		return nil
	}
	*l = StringList(arr)
	return nil
}

// Value writes a TEXT[]. A nil list is stored as an empty array rather than
// NULL, matching the NOT NULL DEFAULT '{}' the columns carry.
func (l StringList) Value() (driver.Value, error) {
	if l == nil {
		return pq.StringArray{}.Value()
	}
	return pq.StringArray(l).Value()
}

// MarshalJSON always produces an array, never null.
func (l StringList) MarshalJSON() ([]byte, error) {
	if l == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(l))
}

// UnmarshalJSON accepts an array, and treats null as empty for symmetry with
// the marshalling side.
func (l *StringList) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*l = StringList{}
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	*l = StringList(values)
	return nil
}
