package runtime

import (
	"fmt"
	"reflect"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Unaccepted snapshots use the complete source audit. Production commits check
// only a newly admitted batch; a persisted generation is never re-adjudicated.
func validateReconciliationExecutionSources(a RuntimeAggregate) error {
	if err := validateReconciliationBatchExecutionSources(a, a.Reconciliation.Request, a.Reconciliation.Batch); err != nil {
		return err
	}
	for _, h := range a.Reconciliation.History {
		if err := validateReconciliationBatchExecutionSources(a, &h.Request, h.Batch); err != nil {
			return err
		}
	}
	return nil
}

func validateReconciliationBatchExecutionSources(a RuntimeAggregate, request *runtimecontract.RuntimeCWIReconciliationRequest, batch *runtimecontract.RuntimeCWIReconciliationBatch) error {
	if request == nil || batch == nil {
		return nil
	}
	for _, o := range batch.Observations {
		if o.ValidationExecution == nil {
			continue
		}
		matches := 0
		for _, binding := range request.Bindings {
			if binding.BindingID != o.BindingID {
				continue
			}
			matches++
			scope, recipes, hash := reconciliationValidationContext(a, binding, true, o.ValidationExecution.SourceReceiptHash)
			if hash == "" || hash != o.ValidationExecution.SourceReceiptHash || runtimecontract.RuntimeValidationExecutionScopeHash(scope, recipes) != o.ValidationExecution.SourceScopeHash {
				return fmt.Errorf("reconciliation execution use differs from original accepted receipt or sealed scope")
			}
		}
		if matches != 1 {
			return fmt.Errorf("reconciliation execution use has no unique request binding")
		}
	}
	return nil
}

// The exact formal preimage supplies already committed generations. They may
// only move, unchanged, to history. A new batch still proves its original
// accepted execution and sealed scope in the one owning CAS transaction.
func validateReconciliationExecutionTransition(previous, next RuntimeAggregate) error {
	before, after := previous.Reconciliation, next.Reconciliation
	if len(after.History) < len(before.History) || len(after.History) > len(before.History)+1 || !reflect.DeepEqual(before.History, after.History[:len(before.History)]) {
		// nil and empty history carry the same empty inventory, not different proof.
		if len(before.History) != 0 || len(after.History) > 1 {
			return fmt.Errorf("committed reconciliation history was rewritten")
		}
	}
	check := func(request *runtimecontract.RuntimeCWIReconciliationRequest, batch *runtimecontract.RuntimeCWIReconciliationBatch, results []runtimecontract.RuntimeCWIReconciliationResult, reopened []runtimecontract.RuntimeCWIReconciliationReopenedLineage) error {
		if before.Request != nil && request != nil && before.Request.RequestID == request.RequestID {
			if !reflect.DeepEqual(*before.Request, *request) {
				return fmt.Errorf("committed reconciliation request was rewritten")
			}
			if before.Batch != nil {
				if !reflect.DeepEqual(before.Batch, batch) || !reflect.DeepEqual(before.Results, results) || !reflect.DeepEqual(before.Reopened, reopened) {
					return fmt.Errorf("committed reconciliation result was rewritten")
				}
				return nil
			}
		}
		return validateReconciliationBatchExecutionSources(next, request, batch)
	}
	if len(after.History) > len(before.History) {
		h := after.History[len(before.History)]
		if before.Request == nil || !h.Superseded || h.RequestHash != before.RequestHash || !reflect.DeepEqual(h.Request, *before.Request) {
			return fmt.Errorf("reconciliation history is not the previous current generation")
		}
		if err := check(&h.Request, h.Batch, h.Results, h.Reopened); err != nil {
			return err
		}
	} else if before.Request != nil && (after.Request == nil || before.Request.RequestID != after.Request.RequestID) {
		return fmt.Errorf("reconciliation generation disappeared without its original history")
	}
	return check(after.Request, after.Batch, after.Results, after.Reopened)
}
