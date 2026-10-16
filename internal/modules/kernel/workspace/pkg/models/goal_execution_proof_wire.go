package workspace

import (
	"encoding/json"
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Presence matters: legacy entries must not carry even an empty Goal extension.
// The strict enclosing decoder separately rejects unknown/duplicate/null fields.
func ValidateExecutionProofMappingWireFields(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	var version string
	if err := json.Unmarshal(fields["ContractRef"], &version); err != nil {
		return err
	}
	_, goal := fields["ConsumerGoalText"]
	if (version == runtimecontract.RuntimeGoalSourceExecutionProofV2 && goal) || (version == runtimecontract.RuntimeCrossSourceExecutionProofV1 && !goal) {
		return nil
	}
	return fmt.Errorf("execution proof mapping version/Goal field presence mismatch")
}
