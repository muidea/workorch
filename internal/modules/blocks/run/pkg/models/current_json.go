package run

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func decodeCurrentRunJSON(data []byte, target any, name string) error {
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

func (r *Run) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("decode current Run: nil target")
	}
	type currentRun Run
	var decoded currentRun
	if err := decodeCurrentRunJSON(data, &decoded, "Run"); err != nil {
		return err
	}
	*r = Run(decoded)
	return nil
}

func (i *ReplacementIntent) UnmarshalJSON(data []byte) error {
	if i == nil {
		return fmt.Errorf("decode current replacement intent: nil target")
	}
	type currentReplacementIntent ReplacementIntent
	var decoded currentReplacementIntent
	if err := decodeCurrentRunJSON(data, &decoded, "replacement intent"); err != nil {
		return err
	}
	*i = ReplacementIntent(decoded)
	return nil
}
