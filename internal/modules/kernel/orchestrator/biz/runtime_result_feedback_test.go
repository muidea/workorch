package orchestrator

import (
	"reflect"
	"strings"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeFeedbackDoesNotPresentHistoricalSatisfiedAssessmentAsCurrentCredit(t *testing.T) {
	aggregate := runtimeTerminalAggregate(t)
	coverage := &aggregate.Facts.Coverage[0]
	coverage.State = runtimecontract.RuntimeCoverageItemStateOpen
	aggregate.Facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{ID: "historical-pass", CoverageWorkItemIDs: []common.CoverageWorkItemID{coverage.ID}, Disposition: runtimecontract.AssessmentDispositionSatisfied}}
	before := aggregate.Facts.Assessments[0]
	outcomes := runtimeRequirementOutcomes(aggregate, runtimecontract.RuntimeOwnerRevisionSet{})
	for _, outcome := range outcomes {
		if outcome.CoverageWorkItemID != coverage.ID {
			continue
		}
		if outcome.AssessmentState != runtimecontract.RuntimeRequirementIndeterminate || outcome.AssessmentReasonCode != "current_coverage_not_satisfied" {
			t.Fatalf("historical credit presented as current: %+v", outcome)
		}
	}
	if !reflect.DeepEqual(before, aggregate.Facts.Assessments[0]) {
		t.Fatal("projection rewrote historical assessment")
	}
	coverage.State = runtimecontract.RuntimeCoverageItemStateSatisfied
	for _, outcome := range runtimeRequirementOutcomes(aggregate, runtimecontract.RuntimeOwnerRevisionSet{}) {
		if outcome.CoverageWorkItemID == coverage.ID && outcome.AssessmentState != runtimecontract.RuntimeRequirementSatisfied {
			t.Fatal("current satisfied coverage lost its assessment")
		}
	}
}

func TestRuntimeTerminalFeedbackPreservesOwnerCauseAndUnassessedRequirements(t *testing.T) {
	aggregate := runtimeTerminalAggregate(t)
	aggregate.ResultFeedbackVersion = runtimecontract.RuntimeResultFeedbackVersionCurrent
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OriginalClass: "candidate_contract_failure", OriginalCode: "patch_basehash_mismatch", OriginalMessage: "conflict: patch baseHash mismatch",
		Signature: "file-preflight/mismatch", OperationID: "operation-1", TargetRefs: []string{"session.txt"},
	}
	diagnostic = diagnostic.CloneOriginal()
	aggregate.Facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassNoProgressExhausted, Code: "equivalent_authority_no_progress", Signature: "equivalent-authority/1", Message: "unchanged authority inputs", OriginalDiagnostic: diagnostic}
	var err error
	aggregate.Decision, err = runtime.ReduceRuntimeState(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	item := mustRuntimeResultMaterial(t, aggregate)
	if item.RuntimeCloseout.FailureCode != "equivalent_authority_no_progress" || !reflect.DeepEqual(item.RuntimeCloseout.OriginalDiagnostic, diagnostic) {
		t.Fatalf("closeout lost terminal/root-cause distinction: got=%+v want=%+v", item.RuntimeCloseout.OriginalDiagnostic, diagnostic)
	}
	if !strings.Contains(item.SummaryDetail.VerificationSummary, "No validation evidence") || item.DoDAssessment.Satisfied || len(item.DoDAssessment.Items) == 0 {
		t.Fatalf("result omitted unexecuted/unassessed facts: %+v", item)
	}
	for _, assessment := range item.DoDAssessment.Items {
		if assessment.State != "unassessed" || len(assessment.EvidenceRefs) != 0 {
			t.Fatalf("missing assessment became failed/passed validation: %+v", assessment)
		}
	}
	if len(item.SummaryDetail.KnownLimitations) == 0 || len(item.SummaryDetail.ResidualRisks) == 0 || len(item.SummaryDetail.NextActions) == 0 || !reflect.DeepEqual(item.SummaryDetail.DoDItems, item.DoDAssessment.Items) {
		t.Fatal("formal result feedback is incomplete")
	}
	copy := item.RuntimeCloseout.Normalized()
	copy.OriginalDiagnostic.TargetRefs[0] = "changed"
	if diagnostic.TargetRefs[0] != "session.txt" || item.RuntimeCloseout.OriginalDiagnostic.TargetRefs[0] != "session.txt" {
		t.Fatal("closeout clone aliases owner diagnostic")
	}
	if copy.WithCloseoutHash().CloseoutHash == item.RuntimeCloseout.CloseoutHash {
		t.Fatal("formal result hash ignored original cause")
	}
	if repeated := mustRuntimeResultMaterial(t, aggregate); !reflect.DeepEqual(item, repeated) {
		t.Fatal("terminal projection changed without new owner facts")
	}
	reloaded := runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(aggregate))
	if repeated := mustRuntimeResultMaterial(t, reloaded); !reflect.DeepEqual(item, repeated) {
		t.Fatal("terminal feedback version/content changed through opaque persistence")
	}
}

func TestRuntimeTerminalFeedbackRejectsNoncurrentVersionWithoutMutation(t *testing.T) {
	for _, version := range []uint32{0, 1, 2, runtimecontract.RuntimeResultFeedbackVersionCurrent + 1} {
		aggregate := runtimeTerminalAggregate(t)
		aggregate.ResultFeedbackVersion = version
		record := runtimeStateRecordFromAggregate(aggregate)
		before := record.Clone()
		if err := aggregate.Validate(); err == nil {
			t.Fatalf("unsupported renderer %d reached Runtime drive", version)
		}
		if err := record.Validate(); err == nil {
			t.Fatalf("unsupported renderer %d admitted by Run contract", version)
		}
		reloaded := runtimeAggregateFromRecord(record)
		outcome, _, err := runtime.DeriveTerminalOutcome(reloaded.Decision, reloaded.Termination)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtimeResultMaterialFromAggregate(reloaded, outcome); err == nil {
			t.Fatalf("unsupported renderer %d fell back to different material", version)
		}
		if !reflect.DeepEqual(before, runtimeStateRecordFromAggregate(reloaded)) {
			t.Fatal("unsupported archive was mutated")
		}
	}
}

func TestRuntimeReadinessTerminalCauseSurvivesResultMaterialAndOpaqueRoundTrip(t *testing.T) {
	aggregate := runtimeTerminalAggregate(t)
	aggregate.ResultFeedbackVersion = runtimecontract.RuntimeResultFeedbackVersionCurrent
	aggregate.Facts.Failure = nil
	aggregate.Decision.State = runtimecontract.ConvergenceStateFailed
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OriginalClass: "validation_readiness", OriginalCode: "native_check_assertion_unresolved", OriginalMessage: "exact native check has no supported assertion", Signature: "readiness/cwi", TargetRefs: []string{"checks/cwi.test"}}
	aggregate.Decision.Terminal = &runtimecontract.RuntimeTerminalDecision{Code: "owner_fact_convergence_exhausted", Summary: "bounded acquisition exhausted", OriginalDiagnostic: diagnostic}
	// The materializer consumes already-sealed facts; reducer/invariant tests
	// separately prove their owner event and exact-catalog authority.
	item := mustRuntimeResultMaterial(t, aggregate)
	if !reflect.DeepEqual(item.RuntimeCloseout.OriginalDiagnostic, diagnostic) || item.RuntimeCloseout.FailureCode != "owner_fact_convergence_exhausted" {
		t.Fatal("formal Result lost exact readiness cause")
	}
	restored := runtimeAggregateFromRecord(runtimeStateRecordFromAggregate(aggregate))
	if repeated := mustRuntimeResultMaterial(t, restored); !reflect.DeepEqual(item, repeated) {
		t.Fatal("delivery retry/restart changed Result material")
	}
	item.RuntimeCloseout.OriginalDiagnostic.TargetRefs[0] = "modified"
	if diagnostic.TargetRefs[0] != "checks/cwi.test" {
		t.Fatal("Result aliases terminal authority")
	}
}
