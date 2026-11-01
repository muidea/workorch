package runtime

import (
	"fmt"
	"reflect"

	contract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CandidateWorkContent replays the private current reservation. It does not
// re-read mutable files or replace the original owner observation on recovery.
func CandidateWorkContent(a RuntimeAggregate, m contract.RuntimeCandidateContractManifest) (*contract.RuntimeImplementationWorkContent, error) {
	if m.ImplementationWorkingBinding == nil {
		return nil, nil
	}
	r, err := CurrentRuntimeImplementationWorkReservation(a)
	if err != nil || r == nil || r.WorkingBinding == nil || r.WorkingBinding.Hash != m.ImplementationWorkingBinding.Hash || r.ScopeObservation == nil {
		return nil, fmt.Errorf("candidate content lacks exact current implementation reservation")
	}
	intent, err := contract.ResolveRuntimeImplementationWorkIntent(*r, a.SemanticPlanning)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(intent, m.ImplementationWorkingBinding.IntentBinding) {
		return nil, fmt.Errorf("candidate intent differs from original purpose")
	}
	c := r.ScopeObservation.Content.Clone()
	if _, err := contract.ProjectRuntimeCandidateWorkContent(contract.RuntimeCandidateContext{ImplementationWorkContent: c}, m); err != nil {
		return nil, err
	}
	return c, nil
}
