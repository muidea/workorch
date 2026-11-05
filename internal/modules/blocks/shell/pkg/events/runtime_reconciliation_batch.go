package events

import (
	"fmt"
	"reflect"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// One current observation transaction. Members are exact projections of the
// persisted request, not a request to apply another CWI's previous PASS.
type QueryRuntimeCWIReconciliationBatchCommand struct {
	Request runtimecontract.RuntimeCWIReconciliationRequest
	Members []runtimecontract.RuntimeCWIOwnerReconciliationRequest
}

type QueryRuntimeCWIReconciliationBatchResult struct {
	Observations []runtimecontract.RuntimeCWIReconciliationObservation
}

func (c QueryRuntimeCWIReconciliationBatchCommand) ValidateForOwner(owner string) error {
	if c.Request.Validate() != nil || len(c.Members) < 2 || len(c.Members) > runtimecontract.RuntimeValidationExecutionMaxConsumers {
		return fmt.Errorf("reconciliation batch requires bounded members and sealed request")
	}
	seen := make(map[string]bool)
	for _, m := range c.Members {
		if runtimecontract.ValidateRuntimeCWIOwnerReconciliationRequest(m, owner) != nil || seen[m.Binding.BindingID] ||
			m.WorkspaceID != c.Request.WorkspaceID || m.RunID != c.Request.RunID || m.RequestID != c.Request.RequestID ||
			m.Generation != c.Request.Generation || m.DefinitionRevision != c.Request.DefinitionRevision || m.BindingSetHash != c.Request.BindingSetHash ||
			m.WorkspaceBaselineRevision != c.Request.WorkspaceBaselineRevision || m.WorkspaceBaselineRef != c.Request.WorkspaceBaselineRef {
			return fmt.Errorf("reconciliation batch member identity mismatch")
		}
		seen[m.Binding.BindingID] = true
		found := false
		for _, b := range c.Request.Bindings {
			if reflect.DeepEqual(b, m.Binding) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("reconciliation batch member is outside sealed request")
		}
	}
	return nil
}

func (c QueryRuntimeCWIReconciliationBatchCommand) ValidateResult(r QueryRuntimeCWIReconciliationBatchResult) error {
	if len(r.Observations) != len(c.Members) {
		return fmt.Errorf("reconciliation batch owner returned incomplete members")
	}
	seen := make(map[string]bool)
	for _, o := range r.Observations {
		if o.ValidateForRequest(c.Request) != nil || seen[o.BindingID] {
			return fmt.Errorf("reconciliation batch returned invalid observation")
		}
		found := false
		for _, m := range c.Members {
			if o.BindingID == m.Binding.BindingID && o.OwnerModuleID == m.OwnerModuleID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("reconciliation batch returned foreign observation")
		}
		if o.ValidationExecution != nil {
			for _, m := range c.Members {
				if m.Binding.BindingID == o.BindingID && (o.ValidationExecution.SourceReceiptHash != m.ValidationSourceReceiptHash || o.ValidationExecution.SourceScopeHash != runtimecontract.RuntimeValidationExecutionScopeHash(m.ValidationScopeTargetRefs, m.ValidationRecipes)) {
					return fmt.Errorf("reconciliation execution changed source receipt or scope")
				}
			}
		}
		seen[o.BindingID] = true
	}
	return runtimecontract.ValidateRuntimeValidationExecutionUses(c.Request, r.Observations)
}
