package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func changeReviewFixture() (RuntimeAggregate, []rc.RuntimeValidationInputReviewFile) {
	a := RuntimeAggregate{Definition: rc.RuntimeDefinitionContext{Revision: "definition"}, Facts: rc.RuntimeFacts{WorkspaceID: "ws", RunID: "run"}}
	var files []rc.RuntimeValidationInputReviewFile
	// More than the general recent-16 window. The first four are auxiliary
	// dispatches and deliberately do not appear in ImplementationWork.Rounds.
	for i := range 21 {
		target := fmt.Sprintf("resource-%02d", i)
		body := fmt.Sprintf("original %d", i)
		op := rc.RuntimeOperation{ID: fmt.Sprintf("op-%d", i), Kind: rc.StepKindFileWrite, TargetRefs: []string{target}, Parameters: []rc.OperationParameter{{Name: "content", String: &body}}}
		key := fmt.Sprintf("exec-%d", i)
		r := rc.RuntimeOperationReceipt{ExecutionKey: key, Fence: 1, OperationID: op.ID, Outcome: rc.RuntimeOperationOutcomeAccepted, ObservedEffectScope: rc.RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: op.TargetRefs, AfterHash: rc.RuntimeValidationInputContentHash(body)}
		a.OperationReceipts = append(a.OperationReceipts, r)
		a.CompletedDispatches = append(a.CompletedDispatches, rc.RuntimeDispatchRecord{ID: fmt.Sprintf("dispatch-%d", i), WorkspaceID: "ws", RunID: "run", Candidate: rc.RuntimeCandidate{StateChange: &rc.StateChangeCandidate{Operations: []rc.StateChangingOperation{{Operation: op}}}}, OperationExecutions: []rc.RuntimeOperationExecutionRecord{{OperationID: op.ID, OperationHash: rc.StableRuntimeOperationHash(op), ExecutionKey: key, Fence: 1, State: rc.OperationExecutionEvidenceCommitted, OwnerReceiptRef: "runtime-receipt/" + key, OwnerReceiptHash: rc.StableRuntimeOperationReceiptHash(r)}}})
		files = append(files, rc.RuntimeValidationInputReviewFile{TargetRef: target, Content: body, ContentHash: r.AfterHash})
	}
	return a, files
}
func TestChangeReviewIncludesAuxiliaryEffectsOutsideRecentHistoryAndConsumesOnce(t *testing.T) {
	a, files := changeReviewFixture()
	before, _ := json.Marshal(a)
	c, err := ChangeReviewContext(a, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Changes) != 21 || c.Changes[0].ExecutionKey != "exec-0" {
		t.Fatal("lost initial auxiliary creation")
	}
	after, _ := json.Marshal(a)
	if string(before) != string(after) {
		t.Fatal("projection changed owner result")
	}
	raw, _ := json.Marshal(a)
	var restored RuntimeAggregate
	if json.Unmarshal(raw, &restored) != nil {
		t.Fatal("reload")
	}
	again, err := ChangeReviewContext(restored, files)
	if err != nil || !reflect.DeepEqual(c, again) {
		t.Fatal("duplicate/reload changed projection", err)
	}
	*c.Changes[0].Operation.Parameters[0].String = "changed clone only"
	if *a.CompletedDispatches[0].Candidate.StateChange.Operations[0].Operation.Parameters[0].String == "changed clone only" {
		t.Fatal("operation parameters alias committed owner source")
	}
	c.Current[0].Content = "mutated"
	if files[0].Content == "mutated" {
		t.Fatal("mutable source alias")
	}
	for _, mutation := range []string{"receipt", "source", "fence", "duplicate", "workspace", "drift", "missing-preimage", "operation"} {
		t.Run(mutation, func(t *testing.T) {
			a, files := changeReviewFixture()
			switch mutation {
			case "receipt":
				a.OperationReceipts[0].AfterHash = "bad"
			case "source":
				a.CompletedDispatches = a.CompletedDispatches[1:]
			case "fence":
				a.CompletedDispatches[0].OperationExecutions[0].Fence++
			case "duplicate":
				a.CompletedDispatches = append(a.CompletedDispatches, a.CompletedDispatches[0])
			case "workspace":
				a.CompletedDispatches[0].WorkspaceID = "other"
			case "operation":
				a.CompletedDispatches[0].Candidate.StateChange.Operations[0].Operation.Kind = rc.StepKindFileRead
			case "drift":
				files[0].Content += "new"
				// Keep the original hash: corrupt bytes, not a legitimate new owner fact.
			case "missing-preimage":
				a.OperationReceipts[0].BeforeHash = "unavailable"
				a.CompletedDispatches[0].OperationExecutions[0].OwnerReceiptHash = rc.StableRuntimeOperationReceiptHash(a.OperationReceipts[0])
			}
			if _, err := ChangeReviewContext(a, files); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}
func TestChangeReviewFixedLiveOwnerArchive(t *testing.T) {
	path := os.Getenv("WORKORCH_WORK_COMPILER_ARCHIVE")
	if path == "" {
		t.Skip("explicit protected offline archive only")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var runs map[string]struct{ RuntimeState rc.RuntimeStateRecord }
	if err = json.Unmarshal(raw, &runs); err != nil {
		t.Fatal(err)
	}
	a, err := ReadCommittedRuntimeOwnerStateRecord(runs["run-001"].RuntimeState)
	if err != nil {
		t.Fatal("formal owner read", err)
	}
	last := a.SemanticPlanning.History[len(a.SemanticPlanning.History)-1].Snapshot
	scope := last.Manifest.ProofObligations[0].ImplementationWork.Scope.Content
	var files []rc.RuntimeValidationInputReviewFile
	for _, s := range scope.Snapshots {
		files = append(files, rc.RuntimeValidationInputReviewFile{TargetRef: s.TargetRef, Content: s.Content, ContentHash: s.ContentHash})
	}
	c, err := ChangeReviewContext(a, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Changes) != 29 || len(c.Current) != 9 {
		t.Fatalf("fixed corpus incomplete: %d mutations/%d files", len(c.Changes), len(c.Current))
	}
	t.Log("all 29 original effects, including four auxiliary creates, are preserved without provider calls, effects or historical readjudgment")
}

func TestChangeReviewNewCurrentFactDoesNotRewriteHistoricalReceipt(t *testing.T) {
	a, files := changeReviewFixture()
	original := a.OperationReceipts[0]
	files[0].Content = "new independent owner observation"
	files[0].ContentHash = rc.RuntimeValidationInputContentHash(files[0].Content)
	c, err := ChangeReviewContext(a, files)
	if err != nil || c.Changes[0].AfterHash == c.Current[0].ContentHash || !reflect.DeepEqual(original, a.OperationReceipts[0]) {
		t.Fatal("current applicability was confused with historical result", err)
	}
	// Unrelated dispatches are not re-admitted while selecting this scope.
	a.CompletedDispatches = append(a.CompletedDispatches, rc.RuntimeDispatchRecord{OperationExecutions: []rc.RuntimeOperationExecutionRecord{{ExecutionKey: "unrelated"}, {ExecutionKey: "unrelated"}}})
	if _, err = ChangeReviewContext(a, files); err != nil {
		t.Fatal("unrelated history was made a new admission gate", err)
	}
	for _, state := range []rc.ConvergenceState{rc.ConvergenceStateNeedPrerequisite, rc.ConvergenceStateComplete, rc.ConvergenceStateFailed} {
		if c, err := CandidateChangeReviewContext(a, rc.RuntimeCandidateContext{RequiredState: state}); err != nil || c != nil {
			t.Fatal("unrelated candidate stage received corpus")
		}
	}
	c, err = SemanticChangeReviewContext(a, rc.RuntimeSemanticPlanRequestContext{PlanStep: rc.RuntimeSemanticPlanStepCoverageOrder}, rc.RuntimeSemanticPlanManifest{})
	if err != nil || c != nil {
		t.Fatal("ordinary coverage ordering received source corpus")
	}
}

func TestChangeReviewExistingTargetUsesSealedPreimageAndRejectsMissingSource(t *testing.T) {
	a, files := changeReviewFixture()
	body := "original existing resource\r\n"
	before := rc.RuntimeValidationInputContentHash(body)
	a.OperationReceipts[0].BeforeHash = before
	a.CompletedDispatches[0].OperationExecutions[0].OwnerReceiptHash = rc.StableRuntimeOperationReceiptHash(a.OperationReceipts[0])
	a.CompletedDispatches[0].ImplementationWorkCommitment = &rc.RuntimeImplementationWorkRound{ScopeObservation: &rc.RuntimeImplementationWorkScopeObservation{Content: &rc.RuntimeImplementationWorkContent{Snapshots: []rc.TargetSnapshot{{TargetRef: files[0].TargetRef, Content: body, ContentHash: before}}}}}
	c, err := ChangeReviewContext(a, files)
	if err != nil || c.Changes[0].Baseline == nil || c.Changes[0].Baseline.Content != body {
		t.Fatal("original sealed preimage missing", err)
	}
	clone := c.Clone()
	clone.Changes[0].Baseline.Content = "tampered"
	if c.Changes[0].Baseline.Content != body || clone.Validate("ws", "run", "definition") == nil {
		t.Fatal("baseline alias or corruption accepted")
	}
	a.CompletedDispatches[0].ImplementationWorkCommitment = nil
	if _, err = ChangeReviewContext(a, files); err == nil {
		t.Fatal("test reconstructed an unavailable baseline")
	}
}

func TestChangeReviewStageSelectionUsesExactCWIAndOwnerDependencyCorpus(t *testing.T) {
	a, files := changeReviewFixture()
	surface := rc.RuntimeValidationSurface{GenerationContext: &rc.RuntimeValidationGenerationContext{Files: files[:1]}, ProofRequirementPreparations: []rc.RuntimeValidationProofPreparation{{CoverageWorkItemID: "selected"}}}
	catalog := rc.RuntimeOperationAdmissibility{ValidationSurfaces: []rc.RuntimeValidationSurface{surface}}
	c := rc.RuntimeCandidateContext{RequiredState: rc.ConvergenceStateNeedAssessment, Focus: rc.CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{"selected"}}, Resources: rc.RuntimeResourceFacts{OperationAdmissibilities: []rc.RuntimeOperationAdmissibility{catalog}}}
	projected, err := CandidateChangeReviewContext(a, c)
	if err != nil || projected == nil || len(projected.Current) != 1 || len(projected.Changes) != 1 {
		t.Fatal("assessment lost exact current scope", err)
	}
	c.Focus.CoverageWorkItemIDs = []common.CoverageWorkItemID{"other"}
	if projected, err = CandidateChangeReviewContext(a, c); err != nil || projected != nil {
		t.Fatal("foreign CWI corpus included", err)
	}
	manifest := rc.RuntimeSemanticPlanManifest{ProofObligations: []rc.RuntimeSemanticProofObligation{{CoverageWorkItemID: "selected"}}, OwnerFactDependencyCatalogs: []rc.RuntimeOperationAdmissibility{catalog}}
	context := rc.RuntimeSemanticPlanRequestContext{PlanStep: rc.RuntimeSemanticPlanStepProofBatch, BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"selected"}}
	if projected, err = SemanticChangeReviewContext(a, context, manifest); err != nil || projected == nil || len(projected.Changes) != 1 {
		t.Fatal("input preparation lost dependency-owner sources", err)
	}
	context.BatchCoverageWorkItemIDs = []common.CoverageWorkItemID{"other"}
	if projected, err = SemanticChangeReviewContext(a, context, manifest); err != nil || projected != nil {
		t.Fatal("unselected batch source escaped scope", err)
	}
}
