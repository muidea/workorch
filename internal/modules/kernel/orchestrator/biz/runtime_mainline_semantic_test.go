package orchestrator

import (
	"reflect"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOperationAdmissibilityTargetsExposeFileCatalogOnlyToPreSealShellCatalog(t *testing.T) {
	aggregate := runtime.RuntimeAggregate{
		Facts: runtimecontract.RuntimeFacts{
			Resources: runtimecontract.RuntimeResourceFacts{OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{{
				PrerequisiteID: "file-catalog", SnapshotID: "file-catalog-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
				OperationKinds: []string{runtimecontract.StepKindFileRead}, TargetRefs: []string{"z.go", "a.go", "a.go"}, ObservedRevision: "tree-1",
			}}},
		},
		SemanticPlanning: runtimecontract.RuntimeSemanticPlanningLedger{Phase: runtimecontract.RuntimeSemanticPlanningRequired},
	}
	if got, want := runtimeOperationAdmissibilityTargetRefs(aggregate, runtimecontract.RuntimeModuleShell), []string{"a.go", "z.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pre-seal Shell targets = %v, want %v", got, want)
	}
	if got := runtimeOperationAdmissibilityTargetRefs(aggregate, runtimecontract.RuntimeModuleFile); len(got) != 0 {
		t.Fatalf("File catalog received cross-owner target injection: %v", got)
	}
	aggregate.SemanticPlanning.Phase = runtimecontract.RuntimeSemanticPlanningSealed
	if got := runtimeOperationAdmissibilityTargetRefs(aggregate, runtimecontract.RuntimeModuleShell); len(got) != 0 {
		t.Fatalf("sealed Shell catalog widened back to manifest: %v", got)
	}
}
