package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"strings"
)

// encoding/json silently discards excess elements of a fixed Go array. Reject
// the raw list before conversion so persisted/profile authority cannot lose an
// alternative. Preserve the enclosing Runtime decoder's unknown-field policy.
func (r *RuntimeLineageAssertionRule) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("decode lineage rule: nil target")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key, value := range fields {
		if !strings.EqualFold(key, "ReceiptOperationChoices") {
			continue
		}
		var choices []json.RawMessage
		if err := json.Unmarshal(value, &choices); err != nil || len(choices) < 2 || len(choices) > 4 {
			return fmt.Errorf("receipt operation choices require two to four exact alternatives")
		}
	}
	type plainRule RuntimeLineageAssertionRule
	var decoded plainRule
	if err := decodeCurrentRuntimeJSON(data, &decoded, "lineage rule"); err != nil {
		return err
	}
	*r = RuntimeLineageAssertionRule(decoded)
	return nil
}
