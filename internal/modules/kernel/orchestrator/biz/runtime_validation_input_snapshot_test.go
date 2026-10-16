package orchestrator

import (
	"encoding/json"
	"reflect"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeReviewedInputSnapshotSourcesOwnerMainlineAndRestart(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterConjunctiveEntrypoint, ValidationCount: 1,
		ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2, SourceObligations: true, SnapshotInputSources: true}
	baseline := runValidationInputReviewMainline(t, "consistent", "", options)
	for _, cut := range []string{"after_review_binding", "after_result_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "consistent", cut, options); calls != baseline {
				t.Fatal("input-source recovery consumed another provider call")
			}
		})
	}
}

// Real File/Shell/Run review, execution and file-backed delivery. This checks
// source derivation only; it does not pretend a Git query or terminal snapshot
// consumer ran in this fixture.
func assertRuntimeReviewedInputSnapshotSources(t *testing.T, aggregate runtime.RuntimeAggregate) {
	t.Helper()
	before, _ := json.Marshal(aggregate)
	matches := 0
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.Candidate.Validation == nil {
			continue
		}
		for _, execution := range dispatch.OperationExecutions {
			input, err := runtime.CompileRuntimeValidationInputSnapshotSource(aggregate, dispatch.ID, execution.OperationID, execution.OwnerReceiptHash)
			if err != nil || input.InputReview == nil || input.InputReview.ValidateAgainstBinding(input) != nil {
				t.Fatalf("actual validation has no original accepted reviewed inputs: %+v %v", input, err)
			}
			matches++
			var operation runtimecontract.RuntimeOperation
			found := false
			for _, member := range dispatch.Candidate.Validation.Operations {
				if member.Operation.ID == execution.OperationID {
					operation, found = member.Operation, true
				}
			}
			if !found {
				// Resolve the exact operation from the candidate's typed family.
				t.Fatal("input source lost original validation operation")
			}
			binding, found := runtimecontract.CoverageEvidenceBindingForRef(dispatch.CoverageBindings, operation.BindingRefs[0])
			if !found {
				t.Fatal("input source lost exact binding")
			}
			original, err := runtimecontract.CompileRuntimeBindingValidationInputReview(binding, aggregate.SemanticPlanning)
			if err != nil || !reflect.DeepEqual(input, original) {
				t.Fatalf("input source differs from unique accepted semantic batch: %v", err)
			}
			input.InputReview.Files[0].Content = "caller"
			repeat, err := runtime.CompileRuntimeValidationInputSnapshotSource(aggregate, dispatch.ID, execution.OperationID, execution.OwnerReceiptHash)
			if err != nil || !reflect.DeepEqual(repeat, original) {
				t.Fatal("input-source returned alias to persisted subject")
			}
			encoded, _ := json.Marshal(runtimeStateRecordFromAggregate(aggregate))
			var reloaded runtimecontract.RuntimeStateRecord
			if json.Unmarshal(encoded, &reloaded) != nil {
				t.Fatal("input source reload failed")
			}
			fromReload, err := runtime.CompileRuntimeValidationInputSnapshotSource(runtimeAggregateFromRecord(reloaded), dispatch.ID, execution.OperationID, execution.OwnerReceiptHash)
			if err != nil || !reflect.DeepEqual(fromReload, original) {
				t.Fatalf("independent reload substituted reviewed input source: %v", err)
			}
			for _, scenario := range []string{"receipt", "operation", "dispatch", "review", "binding", "current"} {
				t.Run("source-"+scenario, func(t *testing.T) {
					bad := aggregate.CloneRaw()
					id, op, hash := dispatch.ID, execution.OperationID, execution.OwnerReceiptHash
					switch scenario {
					case "receipt":
						hash = "forged"
					case "operation":
						op = "another-operation"
					case "dispatch":
						id = "another-dispatch"
					case "review":
						bad.SemanticPlanning = runtimecontract.RuntimeSemanticPlanningLedger{}
					case "binding":
						binding.Hash = "self-consistent-substitute"
						if _, err := runtimecontract.CompileRuntimeBindingValidationInputReview(binding, aggregate.SemanticPlanning); err == nil {
							t.Fatal("unrelated accepted report certified substituted binding")
						}
						return
					case "current":
						for i := range bad.Facts.Coverage {
							if bad.Facts.Coverage[i].ID == original.CoverageWorkItemID {
								bad.Facts.Coverage[i].CurrentBindingRef = nil
							}
						}
					}
					if _, err := runtime.CompileRuntimeValidationInputSnapshotSource(bad, id, op, hash); err == nil {
						t.Fatal("source compiler accepted unrelated/stale/unreviewed source")
					}
				})
			}
		}
	}
	if matches != 1 {
		t.Fatalf("expected one original current validation input source, got %d", matches)
	}
	after, _ := json.Marshal(aggregate)
	if string(before) != string(after) {
		t.Fatal("input source compiler modified original aggregate/credit/budget")
	}
}
