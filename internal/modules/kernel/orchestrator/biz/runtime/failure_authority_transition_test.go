package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type failureAuthorityCASMode uint8

const (
	failureAuthorityCASNormal failureAuthorityCASMode = iota
	failureAuthorityCASFailBeforeWrite
	failureAuthorityCASLoseResponse
)

var errFailureAuthorityCASCut = errors.New("failure authority CAS cut")

type failureAuthorityCASRepository struct {
	aggregate    RuntimeAggregate
	mode         failureAuthorityCASMode
	saveAttempts int
}

func (r *failureAuthorityCASRepository) Load(_ context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if r.aggregate.Facts.WorkspaceID != workspaceID || r.aggregate.Facts.RunID != runID {
		return RuntimeAggregate{}, common.ErrNotFound
	}
	return cloneFailureAuthorityAggregate(r.aggregate)
}

func (r *failureAuthorityCASRepository) Save(_ context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	if r.aggregate.StateRevision != expectedRevision {
		return common.ErrConflict
	}
	r.saveAttempts++
	if r.mode == failureAuthorityCASFailBeforeWrite {
		return errFailureAuthorityCASCut
	}
	cloned, err := cloneFailureAuthorityAggregate(aggregate)
	if err != nil {
		return err
	}
	r.aggregate = cloned
	if r.mode == failureAuthorityCASLoseResponse {
		return errFailureAuthorityCASCut
	}
	return nil
}

func cloneFailureAuthorityAggregate(aggregate RuntimeAggregate) (RuntimeAggregate, error) {
	payload, err := json.Marshal(aggregate)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	var cloned RuntimeAggregate
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return RuntimeAggregate{}, err
	}
	return cloned, nil
}

func TestFailureAuthorityRetainInterludePreservesExactRepairContinuation(t *testing.T) {
	oldFailure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassBuildFailure, Code: "validation_failed",
		Signature: "confirmed-file-failure", Message: "validation failed", TargetRefs: []string{"main.go"},
	}
	target := runtimecontract.FailureTarget{Signature: oldFailure.Signature, TargetRefs: []string{"main.go"}}
	directive := runtimecontract.RuntimeRepairDirective{
		ID: "confirmed-repair/focus/closure/state-change", FocusID: "focus", Trigger: runtimecontract.RuntimeRepairTriggerRejectedValidation,
		RequiredState: runtimecontract.ConvergenceStateNeedStateChange, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"main.go"},
	}
	facts := runtimecontract.RuntimeFacts{Failure: &oldFailure, Resources: runtimecontract.RuntimeResourceFacts{FailureTarget: &target}, RepairDirective: &directive}
	interlude := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceRequired, Code: "approval_required",
		Signature: "governance/repair", Message: "repair requires approval",
	}
	if err := replaceRuntimeFailureAuthority(&facts, interlude, runtimeFailureAuthorityRetainInterlude, nil); err != nil {
		t.Fatal(err)
	}
	if facts.Failure == nil || facts.Failure.Signature != interlude.Signature || facts.Resources.FailureTarget == nil ||
		facts.Resources.FailureTarget.Signature != target.Signature || facts.RepairDirective == nil || facts.RepairDirective.ID != directive.ID {
		t.Fatalf("governance interlude discarded exact repair continuation: %+v", facts)
	}
}

func TestFailureAuthorityReplacementHasSingleProductionWriter(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "failure_authority_transition.go" {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Clean(name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for index, left := range assignment.Lhs {
				if !runtimeFactsFailureAssignment(left) || index >= len(assignment.Rhs) || nilIdentifier(assignment.Rhs[index]) {
					continue
				}
				t.Errorf("%s retains a direct non-nil RuntimeFacts.Failure writer; use replaceRuntimeFailureAuthority", name)
			}
			return true
		})
	}
}

func runtimeFactsFailureAssignment(expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Failure" {
		return false
	}
	switch value := selector.X.(type) {
	case *ast.Ident:
		return value.Name == "facts"
	case *ast.SelectorExpr:
		return value.Sel.Name == "Facts"
	default:
		return false
	}
}

func nilIdentifier(expression ast.Expr) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == "nil"
}

func TestOwnerInfeasibilityAtomicallyRetiresConfirmedRepairAuthority(t *testing.T) {
	prepared, event := prepareConfirmedRepairOwnerInfeasibility(t)
	assertConfirmedRepairFailureAuthorityForReplacementTest(t, prepared)

	for _, tc := range []struct {
		name string
		mode failureAuthorityCASMode
	}{
		{name: "normal", mode: failureAuthorityCASNormal},
		{name: "CAS before write", mode: failureAuthorityCASFailBeforeWrite},
		{name: "CAS response loss", mode: failureAuthorityCASLoseResponse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := cloneRuntimeAggregateForMCPProvenanceTest(t, prepared)
			repository := &failureAuthorityCASRepository{aggregate: cloneRuntimeAggregateForMCPProvenanceTest(t, prepared), mode: tc.mode}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			result, applyErr := controller.ApplyInternalEvent(context.Background(), event)
			switch tc.mode {
			case failureAuthorityCASFailBeforeWrite:
				if !errors.Is(applyErr, errFailureAuthorityCASCut) || repository.saveAttempts != 1 {
					t.Fatalf("CAS-before owner infeasibility error=%v attempts=%d", applyErr, repository.saveAttempts)
				}
				assertConfirmedRepairFailureAuthorityForReplacementTest(t, repository.aggregate)
				if repository.aggregate.StateRevision != before.StateRevision || repository.aggregate.CandidateAttempts[len(repository.aggregate.CandidateAttempts)-1].State != runtimecontract.RuntimeCandidateAttemptStatePending {
					t.Fatalf("CAS-before leaked owner infeasibility: %+v", repository.aggregate)
				}
				return
			case failureAuthorityCASLoseResponse:
				if !errors.Is(applyErr, errFailureAuthorityCASCut) || repository.saveAttempts != 1 {
					t.Fatalf("CAS-after owner infeasibility error=%v attempts=%d", applyErr, repository.saveAttempts)
				}
			case failureAuthorityCASNormal:
				if applyErr != nil || repository.saveAttempts != 1 || result.Aggregate.StateRevision != before.StateRevision+1 {
					t.Fatalf("normal owner infeasibility result=%+v error=%v attempts=%d", result, applyErr, repository.saveAttempts)
				}
			}

			assertRetiredFailureAuthorityAfterOwnerInfeasibility(t, repository.aggregate, before)
			restarted, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := restarted.Load(context.Background(), before.Facts.WorkspaceID, before.Facts.RunID)
			if err != nil {
				t.Fatalf("fresh load rejected atomic owner infeasibility: %v", err)
			}
			assertRetiredFailureAuthorityAfterOwnerInfeasibility(t, loaded, before)
			attempts := repository.saveAttempts
			revision := loaded.StateRevision
			duplicate, err := restarted.ApplyInternalEvent(context.Background(), event)
			if err != nil || duplicate.Aggregate.StateRevision != revision || repository.aggregate.StateRevision != revision || repository.saveAttempts != attempts {
				t.Fatalf("duplicate owner infeasibility changed authority: result=%+v err=%v attempts=%d/%d", duplicate, err, repository.saveAttempts, attempts)
			}
		})
	}
}

func prepareConfirmedRepairOwnerInfeasibility(t *testing.T) (RuntimeAggregate, RuntimeInternalEvent) {
	t.Helper()
	ctx := context.Background()
	fixture := newConfirmedRepairAuthorityFixture(t, []string{"main.go"})
	aggregate := fixture.aggregate
	bindingID := aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "live-owner-infeasible-repair", Kind: runtimecontract.StepKindFileWrite,
		BindingRefs: []string{bindingID}, TargetRefs: []string{"main.go"}, Parameters: runtimeFileWriteParameters("updated main.go"),
	}}}}}
	if _, err := fixture.controller.BeginCandidateAttemptForTest(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "live-owner-infeasible-repair"); err != nil {
		t.Fatal(err)
	}
	proposed, err := fixture.controller.recordCandidateProposal(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	operationID := runtimeTestCandidateOperationID(t, *proposed.CandidateAttempts[len(proposed.CandidateAttempts)-1].Proposal, 0)
	inventory, found := runtimeOperationAdmissibilityForKind(proposed.Facts.Resources.OperationAdmissibilities, runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileWrite)
	if !found {
		t.Fatal("confirmed owner repair has no current File write admissibility")
	}
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_replay_proof_unavailable",
		Signature: "file/replay-proof-unavailable", Message: "oldText not found in the current File snapshot", TargetRefs: []string{"main.go"},
	}
	feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
		WorkspaceID: proposed.Facts.WorkspaceID, RunID: proposed.Facts.RunID, AuthoritySnapshotID: proposed.Decision.AuthoritySnapshot.ID,
	}, runtimecontract.RuntimeFeedbackDomainFeasibility, runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOwnerFeasibilityCategoryObservationUnknown), runtimecontract.RuntimeModuleFile, failure)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = operationID
	observation := runtimecontract.RuntimeOwnerFeasibilityObservation{
		Outcome:  runtimecontract.RuntimeOwnerFeasibilityInfeasible,
		Feedback: &feedback,
		OwnerPolicy: runtimecontract.RuntimeOwnerPolicyIdentity{
			OwnerModuleID: runtimecontract.RuntimeModuleFile, PolicyRevision: "file-owner-feasibility-v1", CatalogSnapshotID: inventory.SnapshotID,
		},
	}
	if _, err := fixture.controller.recordCandidateFeasibility(ctx, proposed.Facts.WorkspaceID, proposed.Facts.RunID, 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: operationID, AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
		CandidateManifestHash: runtimeTestPendingCandidateManifestHash(t, fixture.controller, proposed.Facts.WorkspaceID, proposed.Facts.RunID, 0),
		Observation:           &observation,
	}); err != nil {
		t.Fatal(err)
	}
	prepared, err := fixture.controller.Load(ctx, proposed.Facts.WorkspaceID, proposed.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, RuntimeInternalEvent{
		Kind:      RuntimeInternalEventOwnerFeasibilityObserved,
		Scope:     RuntimeInternalEventScope{WorkspaceID: prepared.Facts.WorkspaceID, RunID: prepared.Facts.RunID},
		Authority: &RuntimeInternalAuthorityPayload{Attempt: 0, OwnerFeasibility: &observation},
	}
}

func assertConfirmedRepairFailureAuthorityForReplacementTest(t *testing.T, aggregate RuntimeAggregate) {
	t.Helper()
	if aggregate.Facts.Resources.FailureTarget == nil || aggregate.Facts.Resources.FailureTarget.TargetObservation == nil ||
		aggregate.Facts.Resources.FailureTarget.TargetObservation.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		aggregate.Facts.RepairDirective == nil || aggregate.Facts.RepairDirective.Closure == nil {
		t.Fatalf("fixture has no exact confirmed owner repair authority: %+v", aggregate.Facts)
	}
}

func assertRetiredFailureAuthorityAfterOwnerInfeasibility(t *testing.T, got, before RuntimeAggregate) {
	t.Helper()
	if got.Facts.Failure == nil || got.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible ||
		got.Facts.Resources.FailureTarget != nil || got.Facts.RepairDirective != nil ||
		got.StateRevision != before.StateRevision+1 || got.CandidateAttempts[len(got.CandidateAttempts)-1].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated {
		t.Fatalf("owner infeasibility left torn failure authority: %+v", got)
	}
	if len(got.OperationReceipts) != len(before.OperationReceipts) || len(got.Facts.Evidence.Accepted) != len(before.Facts.Evidence.Accepted) ||
		len(got.Facts.Evidence.Rejected) != len(before.Facts.Evidence.Rejected) || got.ActiveDispatch != nil ||
		!reflect.DeepEqual(got.ExecutionBudget, before.ExecutionBudget) {
		t.Fatalf("owner infeasibility produced provider/capability/evidence side effects: before=%+v after=%+v", before, got)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("owner infeasibility aggregate is not fresh-load valid: %v", err)
	}
}

func TestFirstTargetlessRejectedReceiptRetiresConfirmedRepairAuthority(t *testing.T) {
	fixture := newConfirmedRepairSingleOperationFixture(t)
	prepared := cloneRuntimeAggregateForMCPProvenanceTest(t, fixture.repository.aggregate)
	assertConfirmedRepairFailureAuthorityForReplacementTest(t, prepared)
	dispatch := *prepared.ActiveDispatch
	operation := candidateRuntimeOperations(dispatch.Candidate)[0].Normalized()
	evidence := ExecutionEvidence{
		EventID:     orchestratorevents.RuntimeOperationEventID(dispatch.ID, operation.ID),
		WorkspaceID: prepared.Facts.WorkspaceID, RunID: prepared.Facts.RunID,
		DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
		OperationID: operation.ID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusRejected,
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(dispatch.ID, operation.ID),
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "repair_member_rejected",
			Signature: "targetless-repair-member-rejected", Message: "File owner rejected the repair without a concrete target observation",
		},
	}
	attachRuntimeTestOwnerReceipt(t, &prepared, &evidence)
	before := cloneRuntimeAggregateForMCPProvenanceTest(t, prepared)
	repository := &failureAuthorityCASRepository{aggregate: prepared}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil || repository.saveAttempts != 1 || committed.Aggregate.StateRevision != before.StateRevision+1 {
		t.Fatalf("targetless rejection result=%+v err=%v attempts=%d", committed, err, repository.saveAttempts)
	}
	got := repository.aggregate
	if got.Facts.Failure == nil || got.Facts.Failure.Signature != evidence.Failure.Signature || got.Facts.Resources.FailureTarget != nil {
		t.Fatalf("targetless rejection retained preceding File target authority: %+v", got.Facts)
	}
	if got.Facts.RepairDirective != nil {
		t.Fatalf("targetless rejection recreated repair authority without an exact owner target: %+v", got.Facts.RepairDirective)
	}
	if len(got.OperationReceipts) != len(before.OperationReceipts) || len(got.Facts.Evidence.Accepted) != len(before.Facts.Evidence.Accepted) ||
		len(got.Facts.Evidence.Rejected) != len(before.Facts.Evidence.Rejected)+1 || !reflect.DeepEqual(got.ExecutionBudget, before.ExecutionBudget) {
		t.Fatalf("targetless rejection produced an unrelated effect: before=%+v after=%+v", before, got)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("targetless rejection failed fresh-load validation: %v", err)
	}
	restarted, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	attempts := repository.saveAttempts
	duplicate, err := restarted.commitEvidence(context.Background(), evidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || repository.saveAttempts != attempts || duplicate.Aggregate.StateRevision != got.StateRevision {
		t.Fatalf("duplicate targetless rejection changed authority: result=%+v err=%v attempts=%d/%d", duplicate, err, repository.saveAttempts, attempts)
	}
}
