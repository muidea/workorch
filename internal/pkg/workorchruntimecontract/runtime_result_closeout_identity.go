package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
)

// SamePersistedValue compares the exact concrete persistence representation.
// Optional empty collections tagged omitempty have the same representation as
// nil after a file-owner read. Go allocation shape is not a delivery identity.
// Do not normalize values here: order, duplicates, whitespace, source facts and
// every field present in the persisted contract must still compare exactly.
// This performs no business judgment and never rewrites either original.
func (c *RuntimeResultCloseout) SamePersistedValue(other *RuntimeResultCloseout) bool {
	left, err := json.Marshal(c)
	if err != nil {
		return false
	}
	right, err := json.Marshal(other)
	return err == nil && bytes.Equal(left, right)
}
