package orchestrator

import (
	"reflect"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCandidateSchemaFailureClassifiesManifestValueRule(t *testing.T) {
	violation := &runtimecontract.RuntimeOperationValueRuleViolation{
		RuleRef: "operation_value/file/edit/distinct_parameters/oldText/newText", OperationKind: runtimecontract.StepKindFileEdit,
		ParameterName: "newText", ActualShape: runtimecontract.RuntimeOperationValueActualShapeEqualParameters,
		RequiredShape: runtimecontract.RuntimeOperationValueRequiredShapeDistinct,
	}
	failure := runtimeCandidateSchemaFailure(&runtimecontract.RuntimeCandidatePayloadDiagnostic{
		Path: "$.operations[0].parameters[name=newText]", Actual: string(runtimecontract.RuntimeOperationValueActualShapeEqualParameters),
		ConstraintRef: violation.RuleRef,
		RequiredShape: string(runtimecontract.RuntimeOperationValueRequiredShapeDistinct),
		Cause:         violation,
	})
	if failure == nil || failure.Class != runtimecontract.RuntimeCandidateFailureClassContract ||
		failure.Code != "candidate_operation_value_rule_nonconformant" || failure.Diagnostic == nil ||
		failure.Diagnostic.ConstraintRef == "" || failure.Diagnostic.RequiredShape != string(runtimecontract.RuntimeOperationValueRequiredShapeDistinct) {
		t.Fatalf("value-rule failure classification = %+v", failure)
	}

	spoofed := runtimeCandidateSchemaFailure(&runtimecontract.RuntimeCandidatePayloadDiagnostic{
		Path: "$.operations[0].parameters[name=newText]", Actual: string(runtimecontract.RuntimeOperationValueActualShapeEqualParameters),
		ConstraintRef: violation.RuleRef, RequiredShape: string(runtimecontract.RuntimeOperationValueRequiredShapeDistinct),
	})
	if spoofed.Class != runtimecontract.RuntimeCandidateFailureClassSchema || spoofed.Code != "candidate_schema_invalid" {
		t.Fatalf("untyped value-rule-shaped diagnostic gained correction authority: %+v", spoofed)
	}
}

func TestRuntimeOperationAdmissibilityTargetsExposeFileCatalogOnlyToShellCatalog(t *testing.T) {
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
	if got, want := runtimeOperationAdmissibilityTargetRefs(aggregate, runtimecontract.RuntimeModuleShell), []string{"a.go", "z.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("post-seal Shell targets = %v, want retained File-owner intersection %v", got, want)
	}
}
