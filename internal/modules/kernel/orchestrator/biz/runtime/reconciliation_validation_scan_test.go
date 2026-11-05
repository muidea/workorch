package runtime

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func assertReconciliationSourceLookup(t *testing.T, a RuntimeAggregate, binding runtimecontract.RuntimeCWIReconciliationBindingRef) {
	t.Helper()
	before := a.CloneRaw()
	_, _, originalHash := referenceReconciliationValidationContext(a, binding, true, "")
	for _, accepted := range []bool{false, true} {
		for _, hash := range []string{"", originalHash, "unrelated-receipt"} {
			wantScope, wantRecipes, wantHash := referenceReconciliationValidationContext(a, binding, accepted, hash)
			gotScope, gotRecipes, gotHash := reconciliationValidationContext(a, binding, accepted, hash)
			if !reflect.DeepEqual(gotScope, wantScope) || !reflect.DeepEqual(gotRecipes, wantRecipes) || gotHash != wantHash {
				t.Fatalf("source lookup differs: accepted=%v receipt=%q", accepted, hash)
			}
		}
	}
	if !reflect.DeepEqual(a, before) {
		t.Fatal("source lookup mutated its input")
	}
}

// Invoked by the real sealed managed-validation/restart fixture as well as the
// archive replay. This is result equivalence, not another production validator.
func assertReconciliationSourceScanMutations(t *testing.T, source RuntimeAggregate, binding runtimecontract.RuntimeCWIReconciliationBindingRef) {
	t.Helper()
	for _, name := range []string{"original", "json-reload", "workspace", "run", "definition", "no-dispatch", "no-receipt", "rejected", "receipt-fence", "receipt-body", "authority", "empty-authority", "uncommitted", "binding-body", "binding-missing", "binding-hash", "execution-hash", "execution-missing", "recipe", "operation", "duplicate", "reverse", "unrelated-later", "tampered-later"} {
		t.Run("scan/"+name, func(t *testing.T) {
			a, requested := source.CloneRaw(), binding.Clone()
			switch name {
			case "json-reload":
				body, err := json.Marshal(ownerStateRecord(a))
				if err != nil {
					t.Fatal(err)
				}
				var record runtimecontract.RuntimeStateRecord
				if err := json.Unmarshal(body, &record); err != nil {
					t.Fatal(err)
				}
				a = RuntimeAggregateFromRecord(record)
			case "workspace":
				a.Facts.WorkspaceID = "another-workspace"
			case "run":
				a.Facts.RunID = "another-run"
			case "definition":
				a.Facts.DefinitionRevision = "another-definition"
			case "no-dispatch":
				a.CompletedDispatches = nil
			case "no-receipt":
				a.OperationReceipts = nil
			case "binding-hash":
				requested.BindingHash = "another-binding"
			case "duplicate":
				a.CompletedDispatches = append(a.CompletedDispatches, a.CompletedDispatches...)
			case "reverse":
				slices.Reverse(a.CompletedDispatches)
			case "unrelated-later", "tampered-later":
				later := source.CloneRaw().CompletedDispatches[len(source.CompletedDispatches)-1]
				if name == "unrelated-later" {
					later.CoverageBindings = nil
				}
				later.DispatchAuthorityHash = "tampered"
				a.CompletedDispatches = append(a.CompletedDispatches, later)
			default:
				for i := range a.OperationReceipts {
					r := &a.OperationReceipts[i]
					switch name {
					case "rejected":
						r.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
					case "receipt-fence":
						r.Fence++
					case "receipt-body":
						r.BeforeHash = "same-key-different-body"
					}
				}
				for i := range a.CompletedDispatches {
					d := &a.CompletedDispatches[i]
					switch name {
					case "authority":
						d.DispatchAuthorityHash = "tampered"
					case "empty-authority":
						d.DispatchAuthorityHash = ""
					case "uncommitted":
						d.State = runtimecontract.RuntimeDispatchState("")
					case "binding-body":
						for j := range d.CoverageBindings {
							d.CoverageBindings[j].DefinitionRevision = "changed"
						}
					case "binding-missing":
						d.CoverageBindings = nil
					case "execution-hash":
						for j := range d.OperationExecutions {
							d.OperationExecutions[j].OwnerReceiptHash = "changed"
						}
					case "execution-missing":
						d.OperationExecutions = nil
					case "recipe":
						for j := range d.ValidationRecipes {
							d.ValidationRecipes[j].TimeoutMillis++
						}
					case "operation":
						d.CompletedOperationIDs = nil
					}
				}
			}
			assertReconciliationSourceLookup(t, a, requested)
		})
	}
}

func reconciliationScanArchive(tb testing.TB) (RuntimeAggregate, []runtimecontract.RuntimeCWIReconciliationBindingRef) {
	tb.Helper()
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		tb.Skip("explicit complete read-only archive required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	var runs map[string]struct {
		RuntimeState runtimecontract.RuntimeStateRecord
	}
	if err := json.Unmarshal(body, &runs); err != nil || len(runs) != 1 {
		tb.Fatalf("invalid single-run archive: %v", err)
	}
	for _, run := range runs {
		a := RuntimeAggregateFromRecord(run.RuntimeState)
		var bindings []runtimecontract.RuntimeCWIReconciliationBindingRef
		for _, h := range a.Reconciliation.History {
			bindings = append(bindings, h.Request.Bindings...)
		}
		if a.Reconciliation.Request != nil {
			bindings = append(bindings, a.Reconciliation.Request.Bindings...)
		}
		if len(bindings) == 0 {
			tb.Fatal("archive lacks reconciliation requests")
		}
		return a, bindings
	}
	tb.Fatal("empty archive")
	return RuntimeAggregate{}, nil
}

func TestReconciliationSourceScanCompleteArchive(t *testing.T) {
	a, bindings := reconciliationScanArchive(t)
	for _, binding := range bindings {
		assertReconciliationSourceLookup(t, a, binding)
	}
	assertReconciliationSourceLookup(t, a, runtimecontract.RuntimeCWIReconciliationBindingRef{BindingID: "absent"})
}

func BenchmarkReconciliationSourceScanCompleteArchive(b *testing.B) {
	a, bindings := reconciliationScanArchive(b)
	for _, name := range []string{"reference", "current"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				for _, binding := range bindings {
					if name == "reference" {
						referenceReconciliationValidationContext(a, binding, true, "")
					} else {
						reconciliationValidationContext(a, binding, true, "")
					}
				}
			}
		})
	}
}
