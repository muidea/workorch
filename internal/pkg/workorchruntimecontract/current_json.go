package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// decodeCurrentRuntimeJSON accepts exactly one value in the current Runtime
// data shape. Runtime persistence has no version negotiation or migration
// path: retired and unknown fields fail before owner validation or recovery.
func decodeCurrentRuntimeJSON(data []byte, target any, name string) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode current %s: %w", name, err)
	}
	if len(bytes.TrimSpace(data[decoder.InputOffset():])) != 0 {
		return fmt.Errorf("decode current %s: multiple JSON values", name)
	}
	return nil
}

func (r *RuntimeStateRecord) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("decode current runtime state: nil target")
	}
	type currentRuntimeState RuntimeStateRecord
	var decoded currentRuntimeState
	if err := decodeCurrentRuntimeJSON(data, &decoded, "runtime state"); err != nil {
		return err
	}
	*r = RuntimeStateRecord(decoded)
	return nil
}

func (b *RuntimeBootstrap) UnmarshalJSON(data []byte) error {
	if b == nil {
		return fmt.Errorf("decode current runtime bootstrap: nil target")
	}
	type currentRuntimeBootstrap RuntimeBootstrap
	var decoded currentRuntimeBootstrap
	if err := decodeCurrentRuntimeJSON(data, &decoded, "runtime bootstrap"); err != nil {
		return err
	}
	*b = RuntimeBootstrap(decoded)
	return nil
}
