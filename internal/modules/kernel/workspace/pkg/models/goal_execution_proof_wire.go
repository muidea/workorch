package workspace

import (
	"encoding/json"
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// DoD source mappings must not carry even an empty Goal-only field.
// The strict enclosing decoder separately rejects unknown/duplicate/null fields.
func ValidateExecutionProofMappingWireFields(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	var contract string
	if err := json.Unmarshal(fields["ContractRef"], &contract); err != nil {
		return err
	}
	_, goal := fields["ConsumerGoalText"]
	if (contract == runtimecontract.RuntimeGoalSourceExecutionProof && goal) || (contract == runtimecontract.RuntimeDoDSourceExecutionProof && !goal) {
		return nil
	}
	return fmt.Errorf("execution proof mapping contract/Goal field presence mismatch")
}
