package orchestrator

// Owner Biz persistence tests.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	runmodule "github.com/muidea/workorch/internal/modules/blocks/run"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// failOnceRuntimeStateRepository injects a process loss immediately before the
// Run-owner CAS write. It deliberately delegates Load to the real repository
// so restart assertions never observe a test-local aggregate cache.
type failOnceRuntimeStateRepository struct {
	runtime.RuntimeStateRepository
	err    error
	failed bool
}

func (r *failOnceRuntimeStateRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if !r.failed {
		r.failed = true
		return r.err
	}
	return r.RuntimeStateRepository.Save(ctx, aggregate, expectedRevision)
}

// failWhenRuntimeStateRepository targets a single persisted aggregate shape.
// It models a process loss at a named CAS boundary without treating earlier
// transitions (for example the execution claim) as failed writes.
type failWhenRuntimeStateRepository struct {
	runtime.RuntimeStateRepository
	err    error
	match  func(runtime.RuntimeAggregate) bool
	failed bool
}

// panicAfterRuntimeStateSaveRepository models a caller that loses the return
// path only after the Run-owner CAS write has become durable. It must never be
// used to simulate a pre-commit error: restart recovery has to observe the
// committed aggregate and must not replay its transition.
type panicAfterRuntimeStateSaveRepository struct {
	runtime.RuntimeStateRepository
	match  func(runtime.RuntimeAggregate) bool
	marker string
	fired  bool
}

func (r *panicAfterRuntimeStateSaveRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	matched := !r.fired && r.match != nil && r.match(aggregate)
	if err := r.RuntimeStateRepository.Save(ctx, aggregate, expectedRevision); err != nil {
		return err
	}
	if matched {
		r.fired = true
		panic(r.marker)
	}
	return nil
}

func (r *failWhenRuntimeStateRepository) Save(ctx context.Context, aggregate runtime.RuntimeAggregate, expectedRevision uint64) error {
	if !r.failed && r.match != nil && r.match(aggregate) {
		r.failed = true
		return r.err
	}
	return r.RuntimeStateRepository.Save(ctx, aggregate, expectedRevision)
}

type runtimeOperationExecutorFunc func(context.Context, runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error)

func (f runtimeOperationExecutorFunc) ExecuteRuntimeOperation(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
	return f(ctx, request)
}

const (
	runtimeFileBackedValidationRecipeID = "test-file-backed-managed-validation"
	runtimeFileBackedValidationCheck    = "repair-validation"
)

// runtimeFactsWithFileBackedManagedValidationContract models the complete
// C11/C12 owner-confirmed validation path used by the file-backed restart
// tests.  The recipe declaration is activation input; the admissibility
// snapshot is the independent Shell-owner fact from which SM-2 may build a
// sealed validation authorization.  A file/read capability is not a
// substitute for either of these facts.
func runtimeFactsWithFileBackedManagedValidationContract(facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	policy := runtimeFileBackedManagedValidationEvidencePolicy()
	const targetRef = "target.txt"
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID:                   "operation-admissibility-shell-file-backed-validation",
		Kind:                 runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID:        runtimecontract.RuntimeModuleShell,
		RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		Reason:               "test Shell owner preflight",
	}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: prerequisite.ID,
		OwnerModuleID:  runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
			},
		}},
		TargetRefs:           []string{targetRef},
		ValidationRecipeRefs: []string{runtimeFileBackedValidationRecipeID},
		ValidationRecipes: runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(
			[]runtimecontract.RuntimeEvidencePolicy{policy},
		),
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: runtimeFileBackedValidationRecipeID, OwnerModuleID: runtimecontract.RuntimeModuleShell,
			OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness:     runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "file-backed-validation-input-v1",
			InputBindingRef: "file-backed-validation-input", CheckRefs: []string{runtimeFileBackedValidationCheck},
			ProofApplicability: runtimecontract.RuntimeValidationProofDirect, EffectScope: runtimecontract.RuntimeEffectEphemeralSession,
			IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		}},
		ObservedRevision: "shell-policy-file-backed-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Activation.EvidencePolicies = append(facts.Activation.EvidencePolicies, policy)
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, prerequisite)
	facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, inventory)
	facts.Resources.PrerequisiteObservations = append(facts.Resources.PrerequisiteObservations, runtimeFileAdmissibilityObservation(inventory))
	return facts
}

func runtimeFileBackedManagedValidationEvidencePolicy() runtimecontract.RuntimeEvidencePolicy {
	return runtimecontract.RuntimeEvidencePolicy{
		ID: runtimeFileBackedValidationRecipeID, AppliesTo: runtimecontract.StepKindShellCommand,
		CommandName: "true", RecipeDeclared: true, RecipeArgs: []string{"workorch-file-backed-validation"},
		RecipeTimeoutMillis: 5_000,
	}
}

// commitRuntimeAssessmentForTest exercises the same owner-local dispatcher
// used by production; tests must not retain a second direct mutation route.
func commitRuntimeAssessmentForTest(ctx context.Context, controller *runtime.RuntimeController, workspaceID common.WorkspaceID, runID common.RunID) (runtime.RuntimeAggregate, error) {
	result, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventAssessmentObserved, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID}, Authority: &runtime.RuntimeInternalAuthorityPayload{},
	})
	return result.Aggregate, err
}

// commitRuntimeAssessmentTargetSnapshotForTest closes the File-owned target
// grounding prerequisite produced by the canonical semantic compiler. The
// assessment persistence fixtures used to inherit an impossible post-compile
// focus overlay that disabled grounding; keeping this acquisition explicit
// makes their pre-assessment operation lineage match production.
func commitRuntimeAssessmentTargetSnapshotForTest(t *testing.T, ctx context.Context, controller *runtime.RuntimeController, aggregate runtime.RuntimeAggregate, targetRef string) runtime.RuntimeAggregate {
	t.Helper()
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, pending := range aggregate.Facts.Prerequisites.Pending {
		if pending.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && pending.OwnerModuleID == runtimecontract.RuntimeModuleFile && pending.SubjectRef == targetRef {
			prerequisite = pending
			break
		}
	}
	if prerequisite.ID == "" {
		t.Fatalf("assessment fixture has no File target-snapshot prerequisite for %q: %+v", targetRef, aggregate.Facts.Prerequisites.Pending)
	}
	snapshot := runtimecontract.TargetSnapshot{
		TargetRef: targetRef, ContentHash: "assessment-target-current-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: snapshot.ContentHash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: targetRef, Revision: snapshot.ContentHash,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	grounded, err := runtimeTestControllerCommitPrerequisite(controller, ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtime.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot, Observation: &observation,
	})
	if err != nil {
		t.Fatalf("commit assessment fixture target snapshot %q: %v", targetRef, err)
	}
	return grounded
}

// executeRuntimeFreshValidationForAssessment closes the mandatory C9 repair
// handoff through the ordinary sealed-dispatch/owner-receipt path. It is kept
// in the file-backed persistence tests so their assessment crash assertions do
// not rely on an invalid shortcut from durable repair evidence to assessment.
func executeRuntimeFreshValidationForAssessment(t *testing.T, ctx context.Context, controller *runtime.RuntimeController, coverageIDs []common.CoverageWorkItemID, prefix, eventID, operationID string) runtime.RuntimeAggregate {
	t.Helper()
	if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, prefix); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{
		Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: operationID, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), coverageIDs...), TargetRefs: []string{"target.txt"},
			Parameters: []runtimecontract.OperationParameter{{
				Name: "recipeId", String: runtimeStringPointer(runtimeFileBackedValidationRecipeID),
			}},
		}}}},
	}
	if proposed, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate); err != nil || !proposed.Admission.Accepted() {
		t.Fatalf("fresh validation proposal=%+v err=%v", proposed, err)
	}
	recordRuntimeFileBackedManagedValidationFeasibility(t, ctx, controller)
	sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
	if err != nil || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("fresh validation seal=%+v err=%v", sealed, err)
	}
	executor := runtimeOperationExecutorFunc(func(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
		receipt := runtimecontract.RuntimeOperationReceipt{
			ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
			ObservedEffectScope: runtimecontract.RuntimeEffectEphemeralSession, TargetRefs: append([]string(nil), request.Operation.TargetRefs...),
			Evidence: runtimecontract.RuntimeOperationEvidence{
				Summary: "fresh repair validation", Quality: common.EvidenceQualityValid,
				Validation: &runtimecontract.RuntimeValidationObservation{
					RecipeID:       runtimeFileBackedValidationRecipeID,
					ExecutedChecks: []string{runtimeFileBackedValidationCheck},
					PassedChecks:   []string{runtimeFileBackedValidationCheck},
				},
			},
		}
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(request.Operation.Kind)
		if !found || descriptor.OwnerModuleID == "" {
			return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("unknown operation kind %q", request.Operation.Kind)
		}
		_, committed, err := runtimeTestControllerCommitOperationReceipt(controller, ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, descriptor.OwnerModuleID, receipt)
		if err != nil {
			return runtime.RuntimeOperationExecutionResult{}, err
		}
		return runtime.RuntimeOperationExecutionResult{
			EventID: eventID, OperationID: request.Operation.ID, EvidenceKind: runtimecontract.EvidenceKindValidation,
			Status: runtime.ExecutionEvidenceStatusAccepted, Evidence: committed.Evidence, Receipt: committed,
		}, nil
	})
	afterValidation, err := runtimeTestControllerExecuteDispatch(controller, ctx, "ws-v2", "run-v2", executor)
	if err != nil {
		t.Fatal(err)
	}
	return afterValidation
}

// recordRuntimeFileBackedManagedValidationFeasibility supplies the
// side-effect-free Shell-owner probe fact required before the sealed dispatch
// may use a managed validation recipe.  It deliberately happens after the
// persisted candidate proposal and before seal, mirroring SM-3's production
// order instead of letting a test executor imply feasibility retroactively.
func recordRuntimeFileBackedManagedValidationFeasibility(t *testing.T, ctx context.Context, controller *runtime.RuntimeController) {
	t.Helper()
	aggregate, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	var inventory *runtimecontract.RuntimeOperationAdmissibility
	for index := range aggregate.Facts.Resources.OperationAdmissibilities {
		candidate := aggregate.Facts.Resources.OperationAdmissibilities[index]
		if candidate.OwnerModuleID != runtimecontract.RuntimeModuleShell {
			continue
		}
		for _, kind := range candidate.OperationKinds {
			if runtimecontract.CanonicalRuntimeBlockKind(kind) == runtimecontract.RuntimeBlockKindManagedValidationSession {
				inventory = &candidate
				break
			}
		}
		if inventory != nil {
			break
		}
	}
	if inventory == nil {
		t.Fatalf("managed validation fixture has no Shell admissibility inventory: %+v", aggregate.Facts.Resources.OperationAdmissibilities)
	}
	operationID := ""
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.State != runtimecontract.RuntimeCandidateAttemptStatePending || attempt.Proposal == nil ||
			attempt.Proposal.Validation == nil || len(attempt.Proposal.Validation.Operations) != 1 {
			continue
		}
		operationID = attempt.Proposal.Validation.Operations[0].Operation.ID
		break
	}
	if operationID == "" {
		t.Fatalf("managed validation fixture has no owner-derived pending operation: %+v", aggregate.CandidateAttempts)
	}
	if _, err := runtimeTestControllerRecordCandidateFeasibility(controller, ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: operationID, AdmissibilitySnapshotID: inventory.SnapshotID,
		AcquisitionBinding:    inventory.AcquisitionBinding,
		CandidateManifestHash: runtimePendingCandidateManifestHashForBizTest(t, ctx, controller, "ws-v2", "run-v2", 0),
		Observation:           &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
	}); err != nil {
		t.Fatal(err)
	}
}

// recordRuntimeFileEditFeasibility supplies the persisted File-owner probe
// required when a candidate manifest is bound to a fresh admissibility
// observation. The record uses the manifest-derived operation ID retained by
// the pending proposal; no test-local operation identity is inferred.
func recordRuntimeFileEditFeasibility(t *testing.T, ctx context.Context, controller *runtime.RuntimeController) {
	t.Helper()
	aggregate, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	var inventory *runtimecontract.RuntimeOperationAdmissibility
	for index := range aggregate.Facts.Resources.OperationAdmissibilities {
		candidate := aggregate.Facts.Resources.OperationAdmissibilities[index]
		if candidate.OwnerModuleID != runtimecontract.RuntimeModuleFile {
			continue
		}
		for _, kind := range candidate.OperationKinds {
			if runtimecontract.CanonicalRuntimeBlockKind(kind) == runtimecontract.StepKindFileEdit {
				inventory = &candidate
				break
			}
		}
		if inventory != nil {
			break
		}
	}
	if inventory == nil {
		t.Fatalf("File edit fixture has no owner admissibility inventory: %+v", aggregate.Facts.Resources.OperationAdmissibilities)
	}
	operationID := ""
	for _, attempt := range aggregate.CandidateAttempts {
		if attempt.State != runtimecontract.RuntimeCandidateAttemptStatePending || attempt.Proposal == nil ||
			attempt.Proposal.StateChange == nil || len(attempt.Proposal.StateChange.Operations) != 1 {
			continue
		}
		operationID = attempt.Proposal.StateChange.Operations[0].Operation.ID
		break
	}
	if operationID == "" {
		t.Fatalf("File edit fixture has no owner-derived pending operation: %+v", aggregate.CandidateAttempts)
	}
	if _, err := runtimeTestControllerRecordCandidateFeasibility(controller, ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{
		OperationID: operationID, AdmissibilitySnapshotID: inventory.SnapshotID,
		AcquisitionBinding:    inventory.AcquisitionBinding,
		CandidateManifestHash: runtimePendingCandidateManifestHashForBizTest(t, ctx, controller, "ws-v2", "run-v2", 0),
		Observation:           &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
	}); err != nil {
		t.Fatal(err)
	}
}

// executeRejectedRuntimeFileEditForRepositoryTest persists the complete SM-3
// transaction and File-owner receipt lineage used by restart/replay fixtures.
// A content-bound edit rejection makes its target snapshot stale; refresh that
// owner prerequisite before returning so the next candidate request starts
// from a real current authority rather than a hand-written EvidenceFact.
func executeRejectedRuntimeFileEditForRepositoryTest(
	t *testing.T,
	ctx context.Context,
	controller *runtime.RuntimeController,
	stablePrefixHash string,
	candidate runtimecontract.RuntimeCandidate,
	eventID string,
	failure runtimecontract.FailureFact,
) runtime.RuntimeAggregate {
	t.Helper()
	if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, stablePrefixHash); err != nil {
		t.Fatalf("begin rejected File edit attempt: %v", err)
	}
	proposed, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !proposed.Admission.Accepted() {
		t.Fatalf("admit rejected File edit proposal: result=%+v err=%v", proposed, err)
	}
	recordRuntimeFileEditFeasibility(t, ctx, controller)
	sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("seal rejected File edit: result=%+v err=%v", sealed, err)
	}
	observed, err := runtimeTestControllerExecuteDispatch(controller, ctx, "ws-v2", "run-v2", runtimeOperationExecutorFunc(func(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
		receipt := runtimecontract.RuntimeOperationReceipt{
			ExecutionKey:        request.Execution.ExecutionKey,
			OperationID:         request.Operation.ID,
			Outcome:             runtimecontract.RuntimeOperationOutcomeRejected,
			ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
			Evidence: runtimecontract.RuntimeOperationEvidence{
				Summary: failure.Message,
			},
		}
		feedback, feedbackErr := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
			WorkspaceID: request.WorkspaceID, RunID: request.RunID,
			AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID, DispatchID: request.DispatchID,
		}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(receipt.Outcome), runtimecontract.RuntimeModuleFile, failure)
		if feedbackErr != nil {
			return runtime.RuntimeOperationExecutionResult{}, feedbackErr
		}
		feedback.OriginalDiagnostic.OperationID = request.Operation.ID
		receipt.Feedback = &feedback
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(request.Operation.Kind)
		if !found || descriptor.OwnerModuleID == "" {
			return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("unknown operation kind %q", request.Operation.Kind)
		}
		_, committed, commitErr := runtimeTestControllerCommitOperationReceipt(controller, ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, descriptor.OwnerModuleID, receipt)
		if commitErr != nil {
			return runtime.RuntimeOperationExecutionResult{}, commitErr
		}
		return runtime.RuntimeOperationExecutionResult{
			EventID: eventID, OperationID: request.Operation.ID,
			EvidenceKind: runtimecontract.EvidenceKindStateChange,
			Status:       runtime.ExecutionEvidenceStatusRejected,
			SourceRef:    "workorch://file/rejected-edit/" + eventID,
			Failure:      &failure, Evidence: committed.Evidence, Receipt: committed,
		}, nil
	}))
	if err != nil {
		t.Fatalf("execute rejected File edit: %v", err)
	}
	if observed.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
		var prerequisite runtimecontract.RuntimePrerequisite
		for _, pending := range observed.Facts.Prerequisites.Pending {
			if pending.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && pending.OwnerModuleID == runtimecontract.RuntimeModuleFile {
				prerequisite = pending
				break
			}
		}
		if prerequisite.ID == "" {
			t.Fatalf("rejected File edit requested no exact target snapshot: %+v", observed.Facts.Prerequisites.Pending)
		}
		revision := "repository-rejected-refresh-" + eventID
		snapshot := runtimecontract.TargetSnapshot{
			TargetRef: prerequisite.SubjectRef, Content: "old", ContentHash: revision,
			ContentState: runtimecontract.SnapshotContentStateComplete,
		}
		observation := runtimecontract.RuntimePrerequisiteObservation{
			PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
			ContentHash: snapshot.ContentHash,
			Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
				Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				SubjectRef: snapshot.TargetRef, Revision: snapshot.ContentHash,
			}},
		}
		observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
		observed, err = runtimeTestControllerCommitPrerequisite(controller, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
			PrerequisiteID: prerequisite.ID, TargetSnapshot: &snapshot, Observation: &observation,
		})
		if err != nil {
			t.Fatalf("refresh rejected File edit target snapshot: %v", err)
		}
	}
	if len(observed.CompletedDispatches) != 1 || len(observed.OperationReceipts) != 1 ||
		len(observed.Facts.Evidence.Rejected) != 1 || observed.EvidenceSequence != 1 ||
		len(runtime.ActiveRejectedOperationShapes(observed)) != 1 {
		t.Fatalf("rejected File edit did not retain complete receipt/evidence lineage: %+v", observed)
	}
	return observed
}

func assertRuntimeFileEditDispatchTargetPreimage(t *testing.T, aggregate runtime.RuntimeAggregate) {
	t.Helper()
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.Candidate.StateChange == nil ||
		len(aggregate.ActiveDispatch.Candidate.StateChange.Operations) != 1 || len(aggregate.ActiveDispatch.OperationTargetAuthorizations) != 1 {
		t.Fatalf("sealed File edit dispatch has no singleton target preimage: %+v", aggregate.ActiveDispatch)
	}
	operation := aggregate.ActiveDispatch.Candidate.StateChange.Operations[0].Operation.Normalized()
	authorization := aggregate.ActiveDispatch.OperationTargetAuthorizations[0].Normalized()
	if aggregate.ActiveDispatch.CandidateManifestHash == "" || authorization.OperationID != operation.ID ||
		authorization.OperationHash != runtimecontract.StableRuntimeOperationHash(operation) {
		t.Fatalf("sealed File edit dispatch target preimage drifted: operation=%+v authorization=%+v manifest=%q", operation, authorization, aggregate.ActiveDispatch.CandidateManifestHash)
	}
}

func runtimeStringPointer(value string) *string {
	return &value
}

// recordAcceptedRuntimeDispatchAdmissionForTest keeps direct persistence
// fixtures honest about the SM-3 entry that produced an executable dispatch.
// The complete provider session is history only; execution authority remains
// the exact accepted attempt bound to the immutable dispatch seal.
func recordAcceptedRuntimeDispatchAdmissionForTest(
	t *testing.T,
	aggregate *runtime.RuntimeAggregate,
	dispatch runtime.SealedDispatch,
	manifest runtimecontract.RuntimeCandidateContractManifest,
) {
	t.Helper()
	admissionTargetAuthorizations, err := runtimecontract.RuntimeDispatchOperationTargetAuthorizationsFromManifest(manifest, dispatch.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	admissionValidationRecipes, err := runtimecontract.RuntimeCandidateManagedValidationRecipeSnapshot(manifest, dispatch.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	stablePrefixHash := "fixture-accepted-dispatch"
	correctionHash := "fixture-accepted-" + dispatch.ID
	logicalRequestID := runtimecontract.StableRuntimeCandidateRequestSessionID(
		runtimecontract.RuntimeProviderDocumentOperations,
		aggregate.Facts.WorkspaceID,
		aggregate.Facts.RunID,
		dispatch.Transaction,
		stablePrefixHash,
		manifest.Hash,
		correctionHash,
	)
	manifestBody := manifest.Clone()
	aggregate.CandidateRequestSessions = append(aggregate.CandidateRequestSessions, runtimecontract.RuntimeCandidateRequestSession{
		LogicalRequestID:  logicalRequestID,
		DocumentKind:      runtimecontract.RuntimeProviderDocumentOperations,
		Transaction:       dispatch.Transaction,
		StablePrefixHash:  stablePrefixHash,
		ManifestHash:      manifest.Hash,
		CandidateManifest: &manifestBody,
		CorrectionHash:    correctionHash,
		State:             runtimecontract.RuntimeCandidateRequestSessionDocumentComplete,
		NextSequence:      1,
		LastPhysicalCallID: runtimecontract.StableRuntimeCandidatePhysicalCallID(
			logicalRequestID,
			1,
		),
		ProviderSubmitted: true,
		LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{
			Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentComplete,
		},
	})
	attempt := runtimecontract.RuntimeCandidateAttemptRecord{
		AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, dispatch.Transaction),
		Transaction:         dispatch.Transaction,
		Attempt:             0,
		StablePrefixHash:    stablePrefixHash,
		ManifestHash:        manifest.Hash,
		LogicalRequestID:    logicalRequestID,
		State:               runtimecontract.RuntimeCandidateAttemptStateAccepted,
		CandidateHash:       dispatch.CandidateHash,
		DispatchID:          dispatch.ID,
		AdmissionTargetAuthorizations: runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(
			admissionTargetAuthorizations,
		),
		TargetAuthorizationHash: runtimecontract.StableRuntimeDispatchOperationTargetAuthorizationsHash(
			admissionTargetAuthorizations,
		),
		AdmissionValidationRecipes: runtimecontract.CloneRuntimeValidationRecipes(admissionValidationRecipes),
		AdmissionCoverageBindings:  runtimecontract.CloneCoverageEvidenceBindings(dispatch.CoverageBindings),
		AdmissionCoverageBindingHash: runtimecontract.StableRuntimeCoverageEvidenceBindingSnapshotHash(
			dispatch.CoverageBindings,
		),
		DispatchAuthorityHash: dispatch.DispatchAuthorityHash,
	}
	if len(admissionValidationRecipes) > 0 {
		attempt.ValidationRecipeHash = runtimecontract.StableRuntimeValidationRecipeSnapshotHash(admissionValidationRecipes)
	}
	aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, attempt)
}

// startRuntimeFileBackedRunOwner creates an isolated process boundary around
// the durable Run repository. Callers deliberately invoke the returned stop
// function before constructing the next process: a new controller alone is
// not evidence that EventHub and the Run owner can recover persisted state.
func startRuntimeFileBackedRunOwner(t *testing.T, ctx context.Context, runs *filerepository.RunRepository) (magicevent.Hub, func()) {
	t.Helper()
	hub := magicevent.NewHub(16)
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		hub.Terminate(ctx)
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		hub.Terminate(ctx)
		t.Fatal(err)
	}
	stopped := false
	return hub, func() {
		if stopped {
			return
		}
		stopped = true
		_ = runOwner.Stop(ctx)
		hub.Terminate(ctx)
	}
}

func TestRuntimeStateRepositoryPersistsAndLoadsThroughRunOwner(t *testing.T) {
	ctx := context.Background()
	if err := runtimeBootstrap().Validate(); err != nil {
		t.Fatalf("test runtime bootstrap must validate: %v", err)
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	// This boundary must survive a real Run-owner persistence reload. A memory
	// repository can prove reducer atomicity, but it cannot prove that the
	// committed assessment/coverage/repair delta is what a restarted process
	// reads from its durable aggregate.
	runsPath := filepath.Join(t.TempDir(), "runs.json")
	runs := filerepository.NewRunRepository(runsPath)
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	module := runmodule.New()
	if err := module.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	repository := runtimeStateRepository{hub: hub}
	aggregate := runtimeAggregate(t, 1)
	transaction := aggregate.Decision.AuthorityTransaction()
	requestSession := runtimecontract.RuntimeCandidateRequestSession{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, Transaction: transaction, StablePrefixHash: "stable-prefix-v2", ManifestHash: "candidate-manifest-v2",
		State: runtimecontract.RuntimeCandidateRequestSessionRecoveryPending, NextSequence: 2, IncompleteOutcomes: 1,
		LastPhysicalCallID: "physical-call-1", ProviderSubmitted: false,
		LastOutcome: runtimecontract.RuntimeCandidateRequestOutcome{
			Class: runtimecontract.RuntimeCandidateRequestOutcomeDocumentIncomplete,
			Code:  runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON, Signature: runtimecontract.RuntimeProviderDocumentSignatureIncompleteJSON,
		},
	}
	requestSession.LogicalRequestID = runtimecontract.StableRuntimeCandidateRequestSessionID(
		requestSession.DocumentKind, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction, requestSession.StablePrefixHash, requestSession.ManifestHash, requestSession.CorrectionHash,
	)
	expected := runtimecontract.RuntimeExpectedDocumentContract{
		SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent, DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		ManifestHash: requestSession.ManifestHash, LogicalRequestID: requestSession.LogicalRequestID, StablePrefixHash: requestSession.StablePrefixHash,
		MaxOutputTokens: 8192, MaxRecoveryAttempts: 2,
	}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	requestSession.ExpectedDocument = &expected
	documentOutcome := runtimecontract.RuntimeProviderDocumentFraming{
		Disposition: runtimecontract.RuntimeProviderDocumentIncomplete, ExpectedDocumentHash: expected.Hash,
		ResponseHash: runtimecontract.StableRuntimeProviderResponseHash([]byte("partial")), ResponseBytes: 7,
		Code: runtimecontract.RuntimeProviderDocumentCodeIncompleteJSON, Signature: runtimecontract.RuntimeProviderDocumentSignatureIncompleteJSON,
	}
	requestSession.LastDocumentFraming = &documentOutcome
	aggregate.CandidateRequestSessions = []runtimecontract.RuntimeCandidateRequestSession{requestSession}
	if current, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2"); err != nil || current.RuntimeBootstrap == nil || !runtimecontract.SameRuntimeBootstrap(*current.RuntimeBootstrap, runtimeBootstrap()) {
		t.Fatalf("persisted bootstrap mismatch current=%+v err=%v", current, err)
	}
	if err := repository.Save(ctx, aggregate, 0); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := repository.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.StateRevision != 1 || loaded.Decision.AuthoritySnapshot.ID != aggregate.Decision.AuthoritySnapshot.ID ||
		!runtimecontract.SameRuntimeDefinitionContext(loaded.Definition, aggregate.Definition) {
		t.Fatalf("loaded aggregate = %+v", loaded)
	}
	if len(loaded.CandidateRequestSessions) != 1 || !reflect.DeepEqual(loaded.CandidateRequestSessions[0], requestSession) {
		t.Fatalf("loaded request session = %+v, want %+v", loaded.CandidateRequestSessions, requestSession)
	}
	// Record conversion must not retain aliases to the caller's nested session
	// values. A later controller transition must be a new CAS record.
	record := runtimeStateRecordFromAggregate(aggregate)
	record.CandidateRequestSessions[0].LastDocumentFraming.ResponseHash = "mutated-record-outcome"
	if aggregate.CandidateRequestSessions[0].LastDocumentFraming.ResponseHash == "mutated-record-outcome" {
		t.Fatalf("record conversion aliased request-session document outcome: %+v", aggregate.CandidateRequestSessions)
	}
	reloadedFromRecord := runtimeAggregateFromRecord(record)
	reloadedFromRecord.CandidateRequestSessions[0].LastDocumentFraming.ResponseHash = "mutated-aggregate-outcome"
	if record.CandidateRequestSessions[0].LastDocumentFraming.ResponseHash != "mutated-record-outcome" {
		t.Fatalf("aggregate conversion aliased request-session document outcome: %+v", record.CandidateRequestSessions)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if persisted.RuntimeState == nil || persisted.RuntimeBootstrap == nil {
		t.Fatalf("persisted runtime state = %+v", persisted)
	}
	if !runtimecontract.SameRuntimeDefinitionContext(persisted.RuntimeState.Definition, runtimeDefinition()) {
		t.Fatalf("persisted runtime definition = %+v", persisted.RuntimeState.Definition)
	}
}

func TestRuntimeStateRepositoryFileReloadRetainsSemanticPlanningGenerationHistory(t *testing.T) {
	ctx := context.Background()
	runsPath := filepath.Join(t.TempDir(), "runs.json")
	runs := filerepository.NewRunRepository(runsPath)
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	firstHub, stopFirst := startRuntimeFileBackedRunOwner(t, ctx, runs)
	firstRepository := runtimeStateRepository{hub: firstHub}
	firstController, err := runtime.NewRuntimeController(firstRepository)
	if err != nil {
		stopFirst()
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(firstController, ctx, runtimeAggregate(t, 1).Facts, runtimeDefinition(), nil)
	if err != nil {
		stopFirst()
		t.Fatal(err)
	}
	sealed := runtimeTestAggregateWithSemanticPlanning(t, initialized)
	if sealed.SemanticPlanning.Manifest == nil || sealed.SemanticPlanning.Document == nil || len(sealed.SemanticPlanning.Batches) == 0 || len(sealed.SemanticPlanning.Feasibilities) == 0 {
		stopFirst()
		t.Fatalf("semantic fixture did not produce a complete generation: %+v", sealed.SemanticPlanning)
	}
	// Keep the catalog itself current while removing only its observation. This
	// makes the private invalidation event exercise the same missing-observation
	// guard used by the deployed semantic probe path.
	sealed.Facts.Resources.OperationAdmissibilities = append([]runtimecontract.RuntimeOperationAdmissibility(nil), sealed.SemanticPlanning.Manifest.OwnerCatalogs...)
	sealed.Facts.Resources.PrerequisiteObservations = nil
	if err := sealed.Validate(); err != nil {
		stopFirst()
		t.Fatalf("file-backed semantic fixture invalid before retirement: %v", err)
	}
	expectedSealedRevision := sealed.StateRevision
	sealed.StateRevision++
	if err := firstRepository.Save(ctx, sealed, expectedSealedRevision); err != nil {
		stopFirst()
		t.Fatalf("persist sealed semantic fixture: %v", err)
	}
	old := sealed.SemanticPlanning.Clone()
	retiredResult, err := firstController.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind: runtime.RuntimeInternalEventSemanticCatalogInvalidated, Scope: runtime.RuntimeInternalEventScope{WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID}, Authority: &runtime.RuntimeInternalAuthorityPayload{ManifestHash: old.Manifest.Hash, LogicalRequestID: old.LogicalRequestID},
	})
	if err != nil {
		stopFirst()
		t.Fatal(err)
	}
	if retiredResult.Aggregate.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || len(retiredResult.Aggregate.SemanticPlanning.History) != 1 {
		stopFirst()
		t.Fatalf("semantic generation was not retired before reload: %+v", retiredResult.Aggregate.SemanticPlanning)
	}
	stopFirst()

	secondHub, stopSecond := startRuntimeFileBackedRunOwner(t, ctx, filerepository.NewRunRepository(runsPath))
	t.Cleanup(stopSecond)
	secondController, err := runtime.NewRuntimeController(runtimeStateRepository{hub: secondHub})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := secondController.Load(ctx, sealed.Facts.WorkspaceID, sealed.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("reloaded semantic generation history failed validation: %v", err)
	}
	if reloaded.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired || reloaded.SemanticPlanning.Manifest != nil ||
		reloaded.SemanticPlanning.Document != nil || len(reloaded.SemanticPlanning.Batches) != 0 || len(reloaded.SemanticPlanning.Feasibilities) != 0 {
		t.Fatalf("fresh generation retained retired authority as current: %+v", reloaded.SemanticPlanning)
	}
	history := reloaded.SemanticPlanning.History
	if len(history) != 1 || history[0].Snapshot.Manifest == nil || history[0].Snapshot.Manifest.Hash != old.Manifest.Hash ||
		history[0].Snapshot.DocumentHash != old.DocumentHash || len(history[0].Snapshot.Batches) != len(old.Batches) ||
		len(history[0].Snapshot.BatchDocuments) != len(old.BatchDocuments) || len(history[0].Snapshot.Feasibilities) != len(old.Feasibilities) ||
		len(history[0].Snapshot.SealedLineageRefs) != len(old.SealedLineageRefs) {
		t.Fatalf("file-backed reload lost complete retired semantic generation: history=%+v old=%+v", history, old)
	}
}

func TestRuntimeStateRepositoryRoundTripsSealedValidationRecipeSnapshot(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	hub, stop := startRuntimeFileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stop)

	aggregate := runtimeAggregate(t, 1)
	aggregate.Facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	aggregate.Facts.DefinitionFocuses[0].TargetRole = "validation_input"
	aggregate.Facts.Focus.TargetRole = "validation_input"
	aggregate.Facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	aggregate.Facts.Focus.ValidationRecipeID = "sealed"
	aggregate.Facts.Focus.TargetRefs = []string{"target.txt"}
	aggregate.Facts.Capabilities = runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}}
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "sealed", Command: "sh", Args: []string{"-c", "true"}}
	const checkRef = "check/sealed-validation"
	aggregate.Facts.Focus.RequiredCheckRefs = []string{checkRef}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "admissibility-shell", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"target.txt"},
		ValidationRecipeRefs: []string{recipe.ID}, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		}},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: recipe.ID, OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "sealed-validation-input-v1", InputBindingRef: "sealed-validation-input",
			CheckRefs: []string{checkRef}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
			EffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		}},
		ObservedRevision: "shell-policy-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	aggregate.Facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}}
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	aggregate.Facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "target.txt", ContentHash: "sealed-validation-target-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	aggregate = runtimeTestAggregateWithSemanticPlanning(t, aggregate)
	facts, decision, err := runtime.ReduceRuntimeStateWithDirective(aggregate.Facts, nil, aggregate.CoverageProofLineage)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	decision = aggregate.Decision
	recipeID := recipe.ID
	bindingRef := aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		BindingRefs: []string{bindingRef}, TargetRefs: []string{"target.txt"},
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}}}}}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(aggregate.Facts.Resources))
	if err != nil {
		t.Fatal(err)
	}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	dispatch, err := runtime.SealDispatchWithManifestAndFeasibilities(decision, candidate, aggregate.Facts.Activation, manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	recordAcceptedRuntimeDispatchAdmissionForTest(t, &aggregate, dispatch, manifest)
	aggregate.ActiveDispatch = &dispatch
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("fixture aggregate: %v", err)
	}

	repository := runtimeStateRepository{hub: hub}
	if err := repository.Save(ctx, aggregate, 0); err != nil {
		t.Fatal(err)
	}
	reloaded, err := repository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ActiveDispatch == nil || reloaded.ActiveDispatch.FocusTargetRole != "validation_input" || !reflect.DeepEqual(reloaded.ActiveDispatch.ValidationRecipes, []runtimecontract.RuntimeValidationRecipe{recipe}) {
		t.Fatalf("reloaded sealed validation recipes = %+v", reloaded.ActiveDispatch)
	}
	if reloaded.ActiveDispatch.CandidateManifestHash != manifest.Hash || len(reloaded.ActiveDispatch.OperationTargetAuthorizations) != 1 ||
		reloaded.ActiveDispatch.Candidate.Validation == nil || len(reloaded.ActiveDispatch.Candidate.Validation.Operations) != 1 {
		t.Fatalf("reloaded validation dispatch lost manifest-bound target authority: %+v", reloaded.ActiveDispatch)
	}
	if len(reloaded.CandidateAttempts) != 1 ||
		!runtimecontract.RuntimeDispatchOperationTargetAuthorizationProjectionsEqual(
			reloaded.CandidateAttempts[0].AdmissionTargetAuthorizations,
			reloaded.ActiveDispatch.OperationTargetAuthorizations,
		) ||
		reloaded.CandidateAttempts[0].TargetAuthorizationHash != runtimecontract.StableRuntimeDispatchOperationTargetAuthorizationsHash(
			reloaded.CandidateAttempts[0].AdmissionTargetAuthorizations,
		) {
		t.Fatalf("reloaded accepted admission lost independent target projection: %+v", reloaded.CandidateAttempts)
	}
	if !runtimecontract.RuntimeValidationRecipeProjectionsEqual(
		reloaded.CandidateAttempts[0].AdmissionValidationRecipes,
		reloaded.ActiveDispatch.ValidationRecipes,
	) || reloaded.CandidateAttempts[0].ValidationRecipeHash != runtimecontract.StableRuntimeValidationRecipeSnapshotHash(
		reloaded.CandidateAttempts[0].AdmissionValidationRecipes,
	) {
		t.Fatalf("reloaded accepted admission lost independent validation-recipe projection: %+v", reloaded.CandidateAttempts)
	}
	operation := reloaded.ActiveDispatch.Candidate.Validation.Operations[0].Operation.Normalized()
	authorization := reloaded.ActiveDispatch.OperationTargetAuthorizations[0].Normalized()
	if authorization.OperationID != operation.ID || authorization.OperationHash != runtimecontract.StableRuntimeOperationHash(operation) ||
		authorization.Target.Policy != runtimecontract.RuntimeOperationTargetPolicyExactTarget || !reflect.DeepEqual(authorization.Target.ExactTargetRefs, []string{"target.txt"}) {
		t.Fatalf("reloaded validation target authority drifted: operation=%+v authorization=%+v", operation, authorization)
	}
	// Conversion must deep-copy the sealed snapshot just like other persisted
	// contract fields; a later resource refresh cannot mutate an old dispatch.
	reloaded.ActiveDispatch.ValidationRecipes[0].Args[1] = "false"
	if aggregate.ActiveDispatch.ValidationRecipes[0].Args[1] != "true" {
		t.Fatalf("persistence conversion aliased sealed recipe arguments: %+v", aggregate.ActiveDispatch.ValidationRecipes)
	}
	reloaded.CandidateAttempts[0].AdmissionValidationRecipes[0].Args[1] = "attempt-false"
	if aggregate.CandidateAttempts[0].AdmissionValidationRecipes[0].Args[1] != "true" {
		t.Fatalf("persistence conversion aliased accepted admission recipe arguments: %+v", aggregate.CandidateAttempts[0].AdmissionValidationRecipes)
	}
	reloaded.CandidateAttempts[0].AdmissionTargetAuthorizations[0].Target.ExactTargetRefs[0] = "mutated.txt"
	if aggregate.CandidateAttempts[0].AdmissionTargetAuthorizations[0].Target.ExactTargetRefs[0] != "target.txt" {
		t.Fatalf("persistence conversion aliased accepted admission target projection: %+v", aggregate.CandidateAttempts[0].AdmissionTargetAuthorizations)
	}
}

func TestRuntimeStateRepositoryRejectsInvalidDuplicateCWILedgerWithoutRewriting(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	run := runtimeRunForTest(common.RunStateActive, runtimeDefinition())
	record := runtimeStateRecordFromAggregate(runtimeAggregate(t, 9))
	// The same canonical CWI is projected in both active and completed focus
	// queues. Loading rejects the current data shape without repairing or
	// classifying it into another lifecycle.
	record.Facts.CompletedFocuses = []runtimecontract.CoverageFocus{record.Facts.Focus.Normalized()}
	run.RuntimeState = &record
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}

	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := runmodule.New()
	if err := module.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	_, err := (runtimeStateRepository{hub: hub}).Load(ctx, run.WorkspaceID, run.ID)
	if err == nil {
		t.Fatal("duplicate CWI record was accepted")
	}
	stored, err := runs.GetByWorkspaceID(ctx, run.WorkspaceID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RuntimeState == nil || stored.RuntimeState.StateRevision != record.StateRevision || len(stored.RuntimeState.Facts.CompletedFocuses) != 1 {
		t.Fatalf("load rewrote opaque invalid record: %+v", stored.RuntimeState)
	}
}

func TestRuntimeInitializesFromPersistedRunBootstrap(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	bootstrap := runtimeBootstrap()
	if err := runs.Create(ctx, &runmodel.Run{
		ID: "run-v2", WorkspaceID: "ws-v2", GoalID: "goal-v2", DoDID: "dod-v2",
		DefinitionRevision: "definition-v2", RuntimeBootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, State: common.RunStatePending,
	}); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	biz := newRuntimeTestOwner()
	if err := biz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	run, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := biz.initializeRuntimeFromRun(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.Facts.Focus.ID != "focus-v2" || aggregate.Definition.GoalID != "goal-v2" || aggregate.StateRevision != 1 {
		t.Fatalf("initialized aggregate = %+v", aggregate)
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.RuntimeBootstrap == nil {
		t.Fatalf("persisted run = %+v err=%v", persisted, err)
	}
}

func TestRuntimeStateRepositoryRejectsStaleRevision(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	aggregate := runtimeAggregate(t, 2)
	record := runtimeStateRecordFromAggregate(aggregate)
	run := runtimeRunForTest(common.RunStateActive, runtimeDefinition())
	run.RuntimeState = &record
	if err := runs.Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	module := runmodule.New()
	if err := module.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	aggregate.StateRevision = 3
	err := (runtimeStateRepository{hub: hub}).Save(ctx, aggregate, 1)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Save stale revision err = %v, want conflict", err)
	}
	var cas *runtime.RuntimeCASConflictError
	if !errors.As(err, &cas) || cas.ExpectedRevision != 1 || cas.ObservedRevision != 2 {
		t.Fatalf("Save stale revision outcome = %+v", cas)
	}
}

func TestRuntimeStateRepositoryTreatsRuntimeDefinitionAsOpaque(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	current := runtimeRunForTest(common.RunStateActive, runtimeDefinition())
	record := runtimeStateRecordFromAggregate(runtimeAggregate(t, 0))
	current.RuntimeState = &record
	// Definition is SM-2 content. Run persists the current-schema payload
	// opaquely and must not decode it to select an SM-1 outcome.
	current.DefinitionRevision = "definition-drift"
	if err := runs.Create(ctx, current); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	if err := (runtimeStateRepository{hub: hub}).Save(ctx, runtimeAggregate(t, 1), 0); err != nil {
		t.Fatalf("opaque Runtime update rejected: %v", err)
	}
}

// The owner-catalog dependency is the authority for an MCP failure target.
// Exercise the production Run-owner CAS repository here, rather than only the
// controller fixture repository: after a process restart the stale target must
// not be reconstructed from the persisted failure or a prior candidate
// attempt.
func TestRuntimeStateRepositoryRestartPersistsMCPOwnerCatalogStaleCandidateInvalidation(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	// Keep the File catalog that owns the semantic state-change proof, then add
	// the independent MCP catalog whose stale/reacquire lifecycle is under test.
	// Both facts exist before the one semantic seal; neither is overlaid later.
	facts := runtimeCandidateSealFileEditFacts(t)
	facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	facts.Focus.TargetRole = "implementation"
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:blocks/mcp", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{"server-a"}, ObservedRevision: "mcp-catalog-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, RequiredCapabilities: []string{runtimecontract.StepKindMCPToolCall},
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			SubjectRef: "mcp_server_catalog", Revision: "mcp-catalog-v1",
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, inventory)
	facts.Resources.PrerequisiteObservations = append(facts.Resources.PrerequisiteObservations, observation)
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, prerequisite)

	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
	if initialized, err = beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "mcp-owner-catalog-prefix"); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		SubjectRef: "mcp_server_catalog", ExpectedRevision: "mcp-catalog-v1", ObservedRevision: "mcp-catalog-v2",
		Reason: "MCP owner catalog changed",
	}
	// A crash before the CAS commit must leave the prior sealed observation and
	// pending candidate intact. A fresh controller can then safely retry the
	// same owner stale signal; no in-memory mutation is authority.
	beforeCommitRepository := &failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository, err: errors.New("injected stale invalidation save failure"),
	}
	beforeCommitController, err := runtime.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeTestControllerInvalidateCandidateStale(beforeCommitController, ctx, "ws-v2", "run-v2", 0, stale); err == nil {
		t.Fatal("stale invalidation unexpectedly committed despite injected Run-owner CAS failure")
	}
	beforeCommit, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeCommit.Facts.Prerequisites.Pending) != 0 ||
		len(beforeCommit.CandidateAttempts) != 1 || beforeCommit.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStatePending ||
		beforeCommit.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("pre-commit failure left a partially invalidated aggregate: %+v", beforeCommit)
	}
	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "mcp-owner-catalog-stale-response-loss",
		match: func(aggregate runtime.RuntimeAggregate) bool {
			mcpRetired, fileRetained := true, false
			for _, catalog := range aggregate.Facts.Resources.OperationAdmissibilities {
				mcpRetired = mcpRetired && catalog.OwnerModuleID != runtimecontract.RuntimeModuleMCP
				fileRetained = fileRetained || catalog.OwnerModuleID == runtimecontract.RuntimeModuleFile
			}
			return mcpRetired && fileRetained && len(aggregate.Facts.Prerequisites.Pending) == 1 &&
				len(aggregate.CandidateAttempts) == 1 && aggregate.CandidateAttempts[0].State == runtimecontract.RuntimeCandidateAttemptStateInvalidated
		},
	}
	responseLost, err := runtime.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "mcp-owner-catalog-stale-response-loss" {
				t.Fatalf("unexpected stale response-loss marker: %v", recovered)
			}
		}()
		_, _ = runtimeTestControllerInvalidateCandidateStale(responseLost, ctx, "ws-v2", "run-v2", 0, stale)
		t.Fatal("expected stale invalidation response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("stale invalidation response-loss CAS point was not reached")
	}

	// New controller instance models a process restart; it must load only the
	// Run-owner persisted aggregate, with no process-local authority cache.
	restarted, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateRevision <= beforeCommit.StateRevision || len(loaded.Facts.Resources.OperationAdmissibilities) != 1 ||
		loaded.Facts.Resources.OperationAdmissibilities[0].OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		loaded.Facts.RepairDirective != nil || len(loaded.Facts.Prerequisites.Pending) != 1 ||
		loaded.Facts.Prerequisites.Pending[0].ID != prerequisite.ID || len(loaded.CandidateAttempts) != 1 ||
		loaded.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated {
		t.Fatalf("restart retained stale MCP repair authority: loaded=%+v", loaded)
	}
	duplicate, err := runtimeTestControllerInvalidateCandidateStale(restarted, ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.StateRevision != loaded.StateRevision || duplicate.Facts.Recovery.PrerequisiteRefreshes != loaded.Facts.Recovery.PrerequisiteRefreshes {
		t.Fatalf("duplicate stale invalidation changed persisted recovery state: before=%+v after=%+v", loaded.Facts.Recovery, duplicate.Facts.Recovery)
	}

	// Reacquisition is a new owner fact, not a resurrection of the stale
	// catalog. Persist the catalog-v2 inventory and prove a subsequent restart
	// retains only that revision. Direct failure-target retirement is exercised
	// separately through the real MCP receipt/evidence matrix.
	refreshedInventory := inventory
	refreshedInventory.ObservedRevision = "mcp-catalog-v2"
	refreshedInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(refreshedInventory)
	refreshedObservation := observation
	refreshedObservation.ContentHash = refreshedInventory.SnapshotID
	refreshedObservation.Dependencies[0].Revision = "mcp-catalog-v2"
	refreshedObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(refreshedObservation)
	reacquired, err := runtimeTestControllerCommitPrerequisite(restarted, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &refreshedInventory, Observation: &refreshedObservation,
	})
	if err != nil || len(reacquired.Facts.Prerequisites.Pending) != 0 || len(reacquired.Facts.Resources.OperationAdmissibilities) != 2 ||
		!runtimeRepositoryHasOwnerCatalogRevision(reacquired.Facts.Resources.OperationAdmissibilities, runtimecontract.RuntimeModuleMCP, "mcp-catalog-v2") {
		t.Fatalf("stale reacquisition did not replace only the owner catalog fact: aggregate=%+v err=%v", reacquired, err)
	}
	restartedAfterReacquire, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := restartedAfterReacquire.Load(ctx, "ws-v2", "run-v2")
	if err != nil || reloaded.StateRevision != reacquired.StateRevision || len(reloaded.Facts.Resources.OperationAdmissibilities) != 2 ||
		!runtimeRepositoryHasOwnerCatalogRevision(reloaded.Facts.Resources.OperationAdmissibilities, runtimecontract.RuntimeModuleMCP, "mcp-catalog-v2") ||
		!runtimeRepositoryHasOwnerCatalogObservationRevision(reloaded.Facts.Resources.PrerequisiteObservations, runtimecontract.RuntimeModuleMCP, "mcp-catalog-v2") ||
		reloaded.Facts.Resources.FailureTarget != nil {
		t.Fatalf("restart did not retain only reacquired owner revision: aggregate=%+v err=%v", reloaded, err)
	}
	if _, err := runtimeTestControllerCommitPrerequisite(restartedAfterReacquire, ctx, "ws-v2", "run-v2", runtime.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID, OperationAdmissibility: &refreshedInventory, Observation: &refreshedObservation,
	}); err == nil {
		t.Fatal("duplicate stale reacquisition unexpectedly mutated aggregate")
	}
}

// Prerequisite acquisition is the first authority transition before candidate
// generation. Use the production Run-owner CAS repository to prove that a
// process loss before that CAS cannot make a scratch resource appear
// committed, and that a restarted controller resumes from the sole committed
// aggregate rather than a controller-local mutation.
func TestRuntimeStateRepositoryPrerequisiteCommitCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	repository := runtimeStateRepository{hub: hub}
	facts := runtimeAggregate(t, 1).Facts
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), []runtimecontract.RuntimeRequirement{{
		Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
		Reason: "test prerequisite commit atomicity",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(initialized.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("initialized aggregate = %+v", initialized)
	}
	prerequisiteID := initialized.Facts.Prerequisites.Pending[0].ID
	result := runtime.PrerequisiteResult{
		PrerequisiteID:    prerequisiteID,
		ValidationScratch: &runtimecontract.ValidationScratch{Reference: ".workorch-validation/run-v2", Revision: "scratch-v1"},
	}

	beforeCommitRepository := &failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected prerequisite CAS failure"),
	}
	beforeCommitController, err := runtime.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeTestControllerCommitPrerequisite(beforeCommitController, ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("prerequisite commit unexpectedly persisted despite injected Run-owner CAS failure")
	}

	// A new controller instance is the restart boundary: it must see the old
	// pending prerequisite and no resource written by the failed CAS attempt.
	restartedBeforeCommit, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedBeforeCommit, err := restartedBeforeCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedBeforeCommit.StateRevision != initialized.StateRevision || loadedBeforeCommit.Facts.Resources.ValidationScratch != nil ||
		len(loadedBeforeCommit.Facts.Prerequisites.Pending) != 1 || loadedBeforeCommit.Facts.Prerequisites.Pending[0].ID != prerequisiteID {
		t.Fatalf("failed prerequisite CAS partially persisted aggregate: %+v", loadedBeforeCommit)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "prerequisite-response-loss",
		match: func(aggregate runtime.RuntimeAggregate) bool {
			return aggregate.Facts.Resources.ValidationScratch != nil && aggregate.Facts.Resources.ValidationScratch.Revision == "scratch-v1" && len(aggregate.Facts.Prerequisites.Pending) == 0
		},
	}
	responseLost, err := runtime.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "prerequisite-response-loss" {
				t.Fatalf("unexpected prerequisite response-loss marker: %v", recovered)
			}
		}()
		_, _ = runtimeTestControllerCommitPrerequisite(responseLost, ctx, "ws-v2", "run-v2", result)
		t.Fatal("expected prerequisite response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("prerequisite response-loss CAS point was not reached")
	}

	restartedAfterCommit, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedAfterCommit, err := restartedAfterCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedAfterCommit.StateRevision <= initialized.StateRevision || loadedAfterCommit.Facts.Resources.ValidationScratch == nil ||
		loadedAfterCommit.Facts.Resources.ValidationScratch.Revision != "scratch-v1" || len(loadedAfterCommit.Facts.Prerequisites.Pending) != 0 {
		t.Fatalf("restart did not reuse committed prerequisite aggregate: %+v", loadedAfterCommit)
	}
	if _, err := runtimeTestControllerCommitPrerequisite(restartedAfterCommit, ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("duplicate prerequisite result unexpectedly mutated committed aggregate")
	}
	final, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if final.StateRevision != loadedAfterCommit.StateRevision || final.Facts.Resources.ValidationScratch == nil || final.Facts.Resources.ValidationScratch.Revision != "scratch-v1" {
		t.Fatalf("duplicate prerequisite result changed persisted aggregate: %+v", final)
	}
}

// FailureTarget is an Orchestrator-local mapping, but it still crosses the
// same Run-owner CAS boundary as an external owner receipt. A failed save must
// not expose the mapping; a lost response after the save must not derive it a
// second time or start any candidate/side-effect work after reload.
func TestRuntimeStateRepositoryValidationScratchStaleInvalidationIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	facts := runtimeAggregate(t, 1).Facts
	prerequisite := runtimecontract.RuntimePrerequisite{ID: "validation-scratch:orchestrator", Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator}
	scratch := &runtimecontract.ValidationScratch{Reference: ".workorch-internal/validation-scratch/ws-v2/run-v2/cwi-v2", Revision: "scratch-v1"}
	observation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: "scratch-v1", Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceLocalResource, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, SubjectRef: "validation-scratch/ws-v2/run-v2/cwi-v2", Revision: scratch.Revision}}}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.ValidationScratch = scratch
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID, Domain: runtimecontract.RuntimeResourceLocalResource, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator, SubjectRef: "validation-scratch/ws-v2/run-v2/cwi-v2", ExpectedRevision: "scratch-v1", ObservedRevision: "scratch-missing-v2", Reason: "validation scratch missing"}
	unrelated := stale
	unrelated.PrerequisiteID = "validation-scratch:other-focus"
	if _, err := runtimeTestControllerInvalidateReadOnlyStale(controller, ctx, "ws-v2", "run-v2", unrelated); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unrelated scratch stale error = %v, want current-observation conflict", err)
	}
	unchanged, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil || unchanged.StateRevision != initialized.StateRevision || unchanged.Facts.Resources.ValidationScratch == nil || unchanged.Facts.Recovery.PrerequisiteRefreshes != 0 || len(unchanged.PrerequisiteInvalidations) != 0 {
		t.Fatalf("unrelated scratch stale changed aggregate=%+v err=%v", unchanged, err)
	}

	before := &failOnceRuntimeStateRepository{RuntimeStateRepository: repository, err: errors.New("injected scratch stale CAS failure")}
	beforeController, err := runtime.NewRuntimeController(before)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeTestControllerInvalidateReadOnlyStale(beforeController, ctx, "ws-v2", "run-v2", stale); err == nil {
		t.Fatal("scratch stale invalidation unexpectedly persisted before CAS")
	}
	loaded, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.StateRevision != initialized.StateRevision || loaded.Facts.Resources.ValidationScratch == nil || len(loaded.Facts.Prerequisites.Pending) != 0 || loaded.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("before-CAS scratch stale leaked: aggregate=%+v err=%v", loaded, err)
	}

	after := &panicAfterRuntimeStateSaveRepository{RuntimeStateRepository: repository, marker: "scratch-stale-response-loss", match: func(aggregate runtime.RuntimeAggregate) bool {
		return aggregate.Facts.Resources.ValidationScratch == nil && aggregate.Facts.Recovery.PrerequisiteRefreshes == 1
	}}
	afterController, err := runtime.NewRuntimeController(after)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "scratch-stale-response-loss" {
				t.Fatalf("unexpected scratch stale marker: %v", recovered)
			}
		}()
		_, _ = runtimeTestControllerInvalidateReadOnlyStale(afterController, ctx, "ws-v2", "run-v2", stale)
		t.Fatal("expected scratch stale response loss")
	}()
	restarted, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.Facts.Resources.ValidationScratch != nil || len(loaded.Facts.Prerequisites.Pending) != 1 || loaded.Facts.Prerequisites.Pending[0].ID != prerequisite.ID || loaded.Facts.Recovery.PrerequisiteRefreshes != 1 || len(loaded.CandidateRequestSessions) != 0 || len(loaded.CandidateAttempts) != 0 || loaded.ActiveDispatch != nil || len(loaded.GovernanceRequests) != 0 || len(loaded.OperationReceipts) != 0 {
		t.Fatalf("after-CAS scratch stale did not preserve clean re-acquisition: aggregate=%+v err=%v", loaded, err)
	}
	duplicate, err := runtimeTestControllerInvalidateReadOnlyStale(restarted, ctx, "ws-v2", "run-v2", stale)
	if err != nil || duplicate.StateRevision != loaded.StateRevision || duplicate.Facts.Recovery.PrerequisiteRefreshes != 1 {
		t.Fatalf("duplicate scratch stale was not a no-op: aggregate=%+v err=%v", duplicate, err)
	}
}

// Target selection carries the owner-bound repair surface. It is therefore not
// sufficient to cover a generic prerequisite: a failed selection CAS must not
// leave a restarted controller with an apparently grounded target that File
// never committed into RuntimeFacts.
func TestRuntimeStateRepositoryTargetSelectionCommitCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	facts := runtimeAggregate(t, 1).Facts
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/unique_workspace_entry"
	facts.Focus.RequireTargetGrounding = true
	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, value := range initialized.Facts.Prerequisites.Pending {
		if value.Kind == runtimecontract.PrerequisiteKindTargetSelection {
			prerequisite = value
			break
		}
	}
	if prerequisite.ID == "" {
		t.Fatalf("target selection prerequisite missing: %+v", initialized.Facts.Prerequisites)
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/unique_workspace_entry",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected,
		TargetRefs:  []string{"implementation.go"}, CandidateTargetRefs: []string{"implementation.go"}, CandidateCount: 1,
		ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: "workspace-tree-v1",
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	result := runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection, Observation: &observation}

	beforeCommitRepository := &failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected target selection CAS failure"),
	}
	beforeCommitController, err := runtime.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeTestControllerCommitPrerequisite(beforeCommitController, ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("target selection unexpectedly persisted despite injected Run-owner CAS failure")
	}

	restartedBeforeCommit, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedBeforeCommit, err := restartedBeforeCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedBeforeCommit.StateRevision != initialized.StateRevision || loadedBeforeCommit.Facts.Resources.TargetSelection != nil {
		t.Fatalf("failed target-selection CAS partially persisted authority: %+v", loadedBeforeCommit)
	}
	foundPending := false
	for _, pending := range loadedBeforeCommit.Facts.Prerequisites.Pending {
		foundPending = foundPending || pending.ID == prerequisite.ID
	}
	if !foundPending {
		t.Fatalf("failed target-selection CAS removed prerequisite: %+v", loadedBeforeCommit.Facts.Prerequisites)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "target-selection-response-loss",
		match: func(aggregate runtime.RuntimeAggregate) bool {
			return aggregate.Facts.Resources.TargetSelection != nil && aggregate.Facts.Resources.TargetSelection.SnapshotID == selection.SnapshotID
		},
	}
	responseLost, err := runtime.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "target-selection-response-loss" {
				t.Fatalf("unexpected target-selection response-loss marker: %v", recovered)
			}
		}()
		_, _ = runtimeTestControllerCommitPrerequisite(responseLost, ctx, "ws-v2", "run-v2", result)
		t.Fatal("expected target-selection response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("target-selection response-loss CAS point was not reached")
	}
	restartedAfterCommit, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedAfterCommit, err := restartedAfterCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedAfterCommit.StateRevision <= initialized.StateRevision || loadedAfterCommit.Facts.Resources.TargetSelection == nil ||
		loadedAfterCommit.Facts.Resources.TargetSelection.SnapshotID != selection.SnapshotID {
		t.Fatalf("restart did not reuse committed target selection: %+v", loadedAfterCommit)
	}
	if _, err := runtimeTestControllerCommitPrerequisite(restartedAfterCommit, ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("duplicate target selection unexpectedly mutated committed aggregate")
	}
	final, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if final.StateRevision != loadedAfterCommit.StateRevision || final.Facts.Resources.TargetSelection == nil ||
		final.Facts.Resources.TargetSelection.SnapshotID != selection.SnapshotID {
		t.Fatalf("duplicate target selection changed persisted aggregate: %+v", final)
	}
}

// A selected target is not usable until File has committed its current
// snapshot. Verify the second half of that grounding gate has the same CAS
// atomicity: a restart may see either the pending prerequisite or the complete
// snapshot, never a hybrid that lets a provider request bypass freshness.
func TestRuntimeStateRepositoryTargetSnapshotCommitCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := runtimeAggregate(t, 1).Facts
	facts.Focus.TargetRefs = []string{"target.txt"}
	facts.Focus.TargetRole = "test_target"
	facts.Focus.RequireTargetGrounding = true
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, value := range initialized.Facts.Prerequisites.Pending {
		if value.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && value.SubjectRef == "target.txt" {
			prerequisite = value
			break
		}
	}
	if prerequisite.ID == "" {
		t.Fatalf("target snapshot prerequisite missing: %+v", initialized.Facts.Prerequisites)
	}
	result := runtime.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{
			TargetRef: "target.txt", Content: "before\n", ContentHash: "target-hash-v1",
			ContentState: runtimecontract.SnapshotContentStateComplete,
		},
	}
	beforeCommitRepository := &failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected target snapshot CAS failure"),
	}
	beforeCommitController, err := runtime.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeTestControllerCommitPrerequisite(beforeCommitController, ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("target snapshot unexpectedly persisted despite injected Run-owner CAS failure")
	}

	restartedBeforeCommit, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedBeforeCommit, err := restartedBeforeCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedBeforeCommit.StateRevision != initialized.StateRevision || len(loadedBeforeCommit.Facts.Resources.TargetSnapshots) != 0 {
		t.Fatalf("failed target snapshot CAS partially persisted authority: %+v", loadedBeforeCommit)
	}
	foundPending := false
	for _, pending := range loadedBeforeCommit.Facts.Prerequisites.Pending {
		foundPending = foundPending || pending.ID == prerequisite.ID
	}
	if !foundPending {
		t.Fatalf("failed target snapshot CAS removed prerequisite: %+v", loadedBeforeCommit.Facts.Prerequisites)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: repository, marker: "target-snapshot-response-loss",
		match: func(aggregate runtime.RuntimeAggregate) bool {
			return len(aggregate.Facts.Resources.TargetSnapshots) == 1 && aggregate.Facts.Resources.TargetSnapshots[0].ContentHash == "target-hash-v1"
		},
	}
	responseLost, err := runtime.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "target-snapshot-response-loss" {
				t.Fatalf("unexpected target-snapshot response-loss marker: %v", recovered)
			}
		}()
		_, _ = runtimeTestControllerCommitPrerequisite(responseLost, ctx, "ws-v2", "run-v2", result)
		t.Fatal("expected target-snapshot response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("target-snapshot response-loss CAS point was not reached")
	}
	restartedAfterCommit, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loadedAfterCommit, err := restartedAfterCommit.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loadedAfterCommit.StateRevision <= initialized.StateRevision || len(loadedAfterCommit.Facts.Resources.TargetSnapshots) != 1 ||
		loadedAfterCommit.Facts.Resources.TargetSnapshots[0].ContentHash != "target-hash-v1" {
		t.Fatalf("restart did not reuse committed target snapshot: %+v", loadedAfterCommit)
	}
	if _, err := runtimeTestControllerCommitPrerequisite(restartedAfterCommit, ctx, "ws-v2", "run-v2", result); err == nil {
		t.Fatal("duplicate target snapshot unexpectedly mutated committed aggregate")
	}
	final, err := controller.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if final.StateRevision != loadedAfterCommit.StateRevision || len(final.Facts.Resources.TargetSnapshots) != 1 ||
		final.Facts.Resources.TargetSnapshots[0].ContentHash != "target-hash-v1" {
		t.Fatalf("duplicate target snapshot changed persisted aggregate: %+v", final)
	}
}

// Manifest and operation-admissibility are owner observations that gate all
// later provider work. A response lost after their Run-owner CAS must retain
// exactly the committed owner fact; a restart must never reacquire it from a
// controller-local value or create a second observation.
func TestRuntimeStateRepositoryOwnerPrerequisiteResponseLossIsCommittedOnly(t *testing.T) {
	testCases := []struct {
		name         string
		requirement  runtimecontract.RuntimeRequirement
		resultFor    func(runtimecontract.RuntimePrerequisite) runtime.PrerequisiteResult
		committed    func(runtime.RuntimeAggregate) bool
		assertLoaded func(*testing.T, runtime.RuntimeAggregate)
	}{
		{
			name:        "manifest",
			requirement: runtimecontract.RuntimeRequirement{Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "file owner manifest"},
			resultFor: func(prerequisite runtimecontract.RuntimePrerequisite) runtime.PrerequisiteResult {
				return runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, Manifest: &runtimecontract.WorkspaceManifest{Revision: "workspace-tree-v1", TargetRefs: []string{"implementation.go"}}}
			},
			committed: func(aggregate runtime.RuntimeAggregate) bool {
				return aggregate.Facts.Resources.Manifest != nil && aggregate.Facts.Resources.Manifest.Revision == "workspace-tree-v1"
			},
			assertLoaded: func(t *testing.T, aggregate runtime.RuntimeAggregate) {
				t.Helper()
				if aggregate.Facts.Resources.Manifest == nil || aggregate.Facts.Resources.Manifest.Revision != "workspace-tree-v1" || len(aggregate.Facts.Resources.Manifest.TargetRefs) != 1 {
					t.Fatalf("committed manifest missing after response loss: %+v", aggregate.Facts.Resources.Manifest)
				}
			},
		},
		{
			name: "operation_admissibility",
			requirement: runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				RequiredCapabilities: []string{runtimecontract.StepKindFileWrite}, Reason: "file owner inventory",
			},
			resultFor: func(prerequisite runtimecontract.RuntimePrerequisite) runtime.PrerequisiteResult {
				inventory := runtimecontract.RuntimeOperationAdmissibility{
					PrerequisiteID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"implementation.go"}, ObservedRevision: "workspace-tree-v1",
				}
				inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
				return runtime.PrerequisiteResult{PrerequisiteID: prerequisite.ID, OperationAdmissibility: &inventory}
			},
			committed: func(aggregate runtime.RuntimeAggregate) bool {
				return len(aggregate.Facts.Resources.OperationAdmissibilities) == 1 && aggregate.Facts.Resources.OperationAdmissibilities[0].ObservedRevision == "workspace-tree-v1"
			},
			assertLoaded: func(t *testing.T, aggregate runtime.RuntimeAggregate) {
				t.Helper()
				if len(aggregate.Facts.Resources.OperationAdmissibilities) != 1 || aggregate.Facts.Resources.OperationAdmissibilities[0].SnapshotID == "" ||
					aggregate.Facts.Resources.OperationAdmissibilities[0].OperationKinds[0] != runtimecontract.StepKindFileWrite {
					t.Fatalf("committed admissibility missing after response loss: %+v", aggregate.Facts.Resources.OperationAdmissibilities)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(ctx) })
			runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
			if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
				t.Fatal(err)
			}
			runOwner := runmodule.New()
			if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
				t.Fatal(err)
			}
			if err := runOwner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runOwner.Stop(ctx) })

			repository := runtimeStateRepository{hub: hub}
			controller, err := runtime.NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			initialized, err := runtimeTestControllerInitialize(controller, ctx, runtimeAggregate(t, 1).Facts, runtimeDefinition(), []runtimecontract.RuntimeRequirement{testCase.requirement})
			if err != nil || len(initialized.Facts.Prerequisites.Pending) != 1 {
				t.Fatalf("prerequisite initialization aggregate=%+v err=%v", initialized, err)
			}
			result := testCase.resultFor(initialized.Facts.Prerequisites.Pending[0])
			responseLossRepository := &panicAfterRuntimeStateSaveRepository{
				RuntimeStateRepository: repository, marker: "owner-prerequisite-response-loss/" + testCase.name,
				match: testCase.committed,
			}
			responseLost, err := runtime.NewRuntimeController(responseLossRepository)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != "owner-prerequisite-response-loss/"+testCase.name {
						t.Fatalf("unexpected %s response-loss marker: %v", testCase.name, recovered)
					}
				}()
				_, _ = runtimeTestControllerCommitPrerequisite(responseLost, ctx, "ws-v2", "run-v2", result)
				t.Fatal("expected owner prerequisite response loss after durable CAS")
			}()
			if !responseLossRepository.fired {
				t.Fatal("owner prerequisite response-loss CAS point was not reached")
			}

			restarted, err := runtime.NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
			if err != nil || loaded.StateRevision <= initialized.StateRevision || len(loaded.Facts.Prerequisites.Pending) != 0 {
				t.Fatalf("restart did not consume committed owner prerequisite: aggregate=%+v err=%v", loaded, err)
			}
			testCase.assertLoaded(t, loaded)
			if _, err := runtimeTestControllerCommitPrerequisite(restarted, ctx, "ws-v2", "run-v2", result); err == nil {
				t.Fatal("duplicate owner prerequisite unexpectedly mutated committed aggregate")
			}
			reloaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
			if err != nil || reloaded.StateRevision != loaded.StateRevision {
				t.Fatalf("duplicate owner prerequisite changed aggregate: reloaded=%+v err=%v", reloaded, err)
			}
		})
	}
}

// Candidate proposal and dispatch seal are two distinct SM-3 CAS boundaries.
// A process loss before either Run-owner write must leave a restart with the
// last committed pending attempt only: a provider response is not a proposal
// until it is persisted, and a proposal is not executable until the sealed
// dispatch is persisted with it.
func TestRuntimeStateRepositoryCandidateProposalAndDispatchSealCrashRestartAreAtomic(t *testing.T) {
	newFixture := func(t *testing.T) (context.Context, runtime.RuntimeStateRepository, *runtime.RuntimeController) {
		t.Helper()
		ctx := context.Background()
		hub := magicevent.NewHub(16)
		t.Cleanup(func() { hub.Terminate(ctx) })
		runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
		if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
			t.Fatal(err)
		}
		runOwner := runmodule.New()
		if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
			t.Fatal(err)
		}
		if err := runOwner.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = runOwner.Stop(ctx) })

		repository := runtimeStateRepository{hub: hub}
		controller, err := runtime.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		initialized, err := runtimeTestControllerInitialize(controller, ctx, runtimeCandidateSealFileEditFacts(t), runtimeDefinition(), nil)
		if err != nil {
			t.Fatal(err)
		}
		runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
		if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "candidate-cas-prefix"); err != nil {
			t.Fatal(err)
		}
		return ctx, repository, controller
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "candidate-cas-edit", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
		Parameters: runtimeFileEditParameters("target.txt"),
	}}}}}

	t.Run("proposal", func(t *testing.T) {
		ctx, repository, _ := newFixture(t)
		beforeCommit, err := runtime.NewRuntimeController(&failOnceRuntimeStateRepository{
			RuntimeStateRepository: repository,
			err:                    errors.New("injected candidate proposal CAS failure"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admitRuntimeCandidateProposalForTest(ctx, beforeCommit, "ws-v2", "run-v2", 0, candidate); err == nil {
			t.Fatal("candidate proposal unexpectedly persisted despite injected Run-owner CAS failure")
		}

		restarted, err := runtime.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded.CandidateAttempts) != 1 || loaded.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStatePending ||
			loaded.CandidateAttempts[0].Proposal != nil || loaded.ActiveDispatch != nil {
			t.Fatalf("failed proposal CAS partially persisted candidate authority: %+v", loaded)
		}

		proposal, err := admitRuntimeCandidateProposalForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
		if err != nil || !proposal.Admission.Accepted() || proposal.Aggregate.CandidateAttempts[0].Proposal == nil {
			t.Fatalf("proposal retry=%+v err=%v", proposal, err)
		}
		persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || persisted.CandidateAttempts[0].Proposal == nil || persisted.ActiveDispatch != nil {
			t.Fatalf("restart did not reuse committed proposal: aggregate=%+v err=%v", persisted, err)
		}
	})

	t.Run("dispatch_seal", func(t *testing.T) {
		ctx, repository, controller := newFixture(t)
		proposal, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
		if err != nil || !proposal.Admission.Accepted() || proposal.Aggregate.CandidateAttempts[0].Proposal == nil {
			t.Fatalf("proposal setup=%+v err=%v", proposal, err)
		}
		recordRuntimeFileEditFeasibility(t, ctx, controller)
		beforeCommit, err := runtime.NewRuntimeController(&failOnceRuntimeStateRepository{
			RuntimeStateRepository: repository,
			err:                    errors.New("injected dispatch seal CAS failure"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, beforeCommit, "ws-v2", "run-v2", 0, candidate); err == nil {
			t.Fatal("dispatch seal unexpectedly persisted despite injected Run-owner CAS failure")
		}

		restarted, err := runtime.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded.CandidateAttempts) != 1 || loaded.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStatePending ||
			loaded.CandidateAttempts[0].Proposal == nil || loaded.ActiveDispatch != nil {
			t.Fatalf("failed dispatch-seal CAS partially persisted executable authority: %+v", loaded)
		}

		sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
		if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil ||
			sealed.Aggregate.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted ||
			sealed.Aggregate.CandidateAttempts[0].Proposal != nil {
			t.Fatalf("dispatch-seal retry=%+v err=%v", sealed, err)
		}
		assertRuntimeFileEditDispatchTargetPreimage(t, sealed.Aggregate)
		persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || persisted.ActiveDispatch == nil ||
			persisted.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			t.Fatalf("restart did not reuse committed sealed dispatch: aggregate=%+v err=%v", persisted, err)
		}
	})

	t.Run("proposal_response_loss", func(t *testing.T) {
		ctx, repository, _ := newFixture(t)
		responseLossRepository := &panicAfterRuntimeStateSaveRepository{
			RuntimeStateRepository: repository, marker: "candidate-proposal-response-loss",
			match: func(aggregate runtime.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch == nil && len(aggregate.CandidateAttempts) == 1 &&
					aggregate.CandidateAttempts[0].State == runtimecontract.RuntimeCandidateAttemptStatePending && aggregate.CandidateAttempts[0].Proposal != nil
			},
		}
		responseLost, err := runtime.NewRuntimeController(responseLossRepository)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != "candidate-proposal-response-loss" {
					t.Fatalf("unexpected proposal response-loss marker: %v", recovered)
				}
			}()
			_, _ = admitRuntimeCandidateProposalForTest(ctx, responseLost, "ws-v2", "run-v2", 0, candidate)
			t.Fatal("expected proposal response loss after durable CAS")
		}()
		if !responseLossRepository.fired {
			t.Fatal("proposal response-loss CAS point was not reached")
		}

		restarted, err := runtime.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || loaded.ActiveDispatch != nil || len(loaded.CandidateAttempts) != 1 || loaded.CandidateAttempts[0].Proposal == nil {
			t.Fatalf("restart did not retain committed proposal after response loss: aggregate=%+v err=%v", loaded, err)
		}
		persistedCandidateHash := runtimecontract.StableRuntimeCandidateHash(*loaded.CandidateAttempts[0].Proposal)
		recordRuntimeFileEditFeasibility(t, ctx, restarted)
		sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
		if err != nil || sealed.Aggregate.ActiveDispatch == nil || sealed.Aggregate.ActiveDispatch.CandidateHash != persistedCandidateHash {
			t.Fatalf("response-loss proposal did not seal the original candidate: sealed=%+v err=%v", sealed, err)
		}
	})

	t.Run("dispatch_seal_response_loss", func(t *testing.T) {
		ctx, repository, controller := newFixture(t)
		if _, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate); err != nil {
			t.Fatal(err)
		}
		recordRuntimeFileEditFeasibility(t, ctx, controller)
		responseLossRepository := &panicAfterRuntimeStateSaveRepository{
			RuntimeStateRepository: repository, marker: "dispatch-seal-response-loss",
			match: func(aggregate runtime.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch != nil && len(aggregate.CandidateAttempts) == 1 &&
					aggregate.CandidateAttempts[0].State == runtimecontract.RuntimeCandidateAttemptStateAccepted
			},
		}
		responseLost, err := runtime.NewRuntimeController(responseLossRepository)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != "dispatch-seal-response-loss" {
					t.Fatalf("unexpected dispatch-seal response-loss marker: %v", recovered)
				}
			}()
			_, _ = admitAndSealRuntimeCandidateAttemptForTest(ctx, responseLost, "ws-v2", "run-v2", 0, candidate)
			t.Fatal("expected dispatch-seal response loss after durable CAS")
		}()
		if !responseLossRepository.fired {
			t.Fatal("dispatch-seal response-loss CAS point was not reached")
		}

		restarted, err := runtime.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || loaded.ActiveDispatch == nil || len(loaded.CandidateAttempts) != 1 ||
			loaded.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || loaded.CandidateAttempts[0].Proposal != nil {
			t.Fatalf("restart did not retain sealed dispatch after response loss: aggregate=%+v err=%v", loaded, err)
		}
		assertRuntimeFileEditDispatchTargetPreimage(t, loaded)
		replayed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
		if err != nil || replayed.Aggregate.StateRevision != loaded.StateRevision || replayed.Aggregate.ActiveDispatch == nil ||
			replayed.Aggregate.ActiveDispatch.ID != loaded.ActiveDispatch.ID || len(replayed.Aggregate.CompletedDispatches) != len(loaded.CompletedDispatches) {
			t.Fatalf("committed dispatch response-loss replay was not idempotent: replayed=%+v loaded=%+v err=%v", replayed, loaded, err)
		}
	})
}

// P5-A: proposal and seal have no owner side effect, but both are durable SM-3
// boundaries. A fresh process must see either the prior proposal or the exact
// sealed dispatch; it must never synthesize a second dispatch after a lost
// response.
func TestRuntimeStateRepositoryDispatchSealFreshProcessCrashRestart(t *testing.T) {
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "fresh-process-seal-edit", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
		Parameters: runtimeFileEditParameters("target.txt"),
	}}}}}
	newFixture := func(t *testing.T) (context.Context, *filerepository.RunRepository, runtimeStateRepository, *runtime.RuntimeController, func()) {
		t.Helper()
		ctx := context.Background()
		runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
		if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
			t.Fatal(err)
		}
		hub, stop := startRuntimeFileBackedRunOwner(t, ctx, runs)
		repository := runtimeStateRepository{hub: hub}
		controller, err := runtime.NewRuntimeController(repository)
		if err != nil {
			stop()
			t.Fatal(err)
		}
		initialized, err := runtimeTestControllerInitialize(controller, ctx, runtimeCandidateSealFileEditFacts(t), runtimeDefinition(), nil)
		if err != nil {
			stop()
			t.Fatal(err)
		}
		runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
		if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "fresh-process-seal-prefix"); err != nil {
			stop()
			t.Fatal(err)
		}
		if proposal, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate); err != nil || !proposal.Admission.Accepted() || proposal.Aggregate.ActiveDispatch != nil {
			stop()
			t.Fatalf("proposal setup=%+v err=%v", proposal, err)
		}
		recordRuntimeFileEditFeasibility(t, ctx, controller)
		return ctx, runs, repository, controller, stop
	}

	t.Run("before_seal_cas", func(t *testing.T) {
		ctx, runs, repository, _, stopFirstProcess := newFixture(t)
		t.Cleanup(stopFirstProcess)
		beforeCommit, err := runtime.NewRuntimeController(&failOnceRuntimeStateRepository{
			RuntimeStateRepository: repository, err: errors.New("injected dispatch seal CAS failure"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, beforeCommit, "ws-v2", "run-v2", 0, candidate); err == nil {
			t.Fatal("dispatch seal unexpectedly persisted before Run-owner CAS")
		}
		stopFirstProcess()
		hub, stopSecondProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
		t.Cleanup(stopSecondProcess)
		restarted, err := runtime.NewRuntimeController(runtimeStateRepository{hub: hub})
		if err != nil {
			t.Fatal(err)
		}
		pending, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || pending.ActiveDispatch != nil || len(pending.CandidateAttempts) != 1 ||
			pending.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStatePending || pending.CandidateAttempts[0].Proposal == nil {
			t.Fatalf("pre-CAS fresh process leaked sealed dispatch: aggregate=%+v err=%v", pending, err)
		}
		sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
		if err != nil || sealed.Aggregate.ActiveDispatch == nil || sealed.Aggregate.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			t.Fatalf("fresh process did not seal persisted proposal: aggregate=%+v err=%v", sealed, err)
		}
		assertRuntimeFileEditDispatchTargetPreimage(t, sealed.Aggregate)
	})

	t.Run("after_seal_cas_response_loss", func(t *testing.T) {
		ctx, runs, repository, _, stopFirstProcess := newFixture(t)
		t.Cleanup(stopFirstProcess)
		responseLossRepository := &panicAfterRuntimeStateSaveRepository{
			RuntimeStateRepository: repository, marker: "fresh-process-dispatch-seal-response-loss",
			match: func(aggregate runtime.RuntimeAggregate) bool {
				return aggregate.ActiveDispatch != nil && len(aggregate.CandidateAttempts) == 1 &&
					aggregate.CandidateAttempts[0].State == runtimecontract.RuntimeCandidateAttemptStateAccepted
			},
		}
		responseLost, err := runtime.NewRuntimeController(responseLossRepository)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != "fresh-process-dispatch-seal-response-loss" {
					t.Fatalf("unexpected dispatch seal response-loss marker: %v", recovered)
				}
			}()
			_, _ = admitAndSealRuntimeCandidateAttemptForTest(ctx, responseLost, "ws-v2", "run-v2", 0, candidate)
			t.Fatal("expected dispatch seal response loss after durable CAS")
		}()
		if !responseLossRepository.fired {
			t.Fatal("dispatch seal response-loss CAS point was not reached")
		}
		stopFirstProcess()
		hub, stopSecondProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
		t.Cleanup(stopSecondProcess)
		restarted, err := runtime.NewRuntimeController(runtimeStateRepository{hub: hub})
		if err != nil {
			t.Fatal(err)
		}
		persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || persisted.ActiveDispatch == nil || len(persisted.CandidateAttempts) != 1 ||
			persisted.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || persisted.CandidateAttempts[0].Proposal != nil {
			t.Fatalf("post-CAS fresh process did not retain one sealed dispatch: aggregate=%+v err=%v", persisted, err)
		}
		assertRuntimeFileEditDispatchTargetPreimage(t, persisted)
		revision, dispatchID := persisted.StateRevision, persisted.ActiveDispatch.ID
		duplicate, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, restarted, "ws-v2", "run-v2", 0, candidate)
		if err != nil || duplicate.Aggregate.StateRevision != revision || duplicate.Aggregate.ActiveDispatch == nil || duplicate.Aggregate.ActiveDispatch.ID != dispatchID ||
			len(duplicate.Aggregate.CompletedDispatches) != len(persisted.CompletedDispatches) {
			t.Fatalf("sealed dispatch duplicate changed fresh-process authority: aggregate=%+v err=%v", duplicate, err)
		}
	})
}

// A scope-bound validation failure observation first becomes an owner receipt;
// it cannot become repair authority until the later File-confirmation SM-2
// transition. Exercise the first boundary through the production Run-owner
// repository: a crash before the receipt CAS must not leak any failure fact or
// observation. Managed
// validation is at-most-once: after its claim is durable, restart must preserve
// atomicity by fencing the ambiguous owner call as outcome_unknown rather than
// replaying it.
func TestRuntimeStateRepositoryValidationFailureObservationReceiptCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	// The response-loss half of this boundary must survive a real Run-owner
	// persistence reload. An in-memory repository can prove reducer atomicity,
	// but cannot prove that the committed owner observation is all a restarted
	// process can observe.
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	facts := runtimeAggregate(t, 1).Facts
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Focus.ValidationRecipeID = "failure-observation-validation"
	facts.Focus.TargetRefs = []string{"validation-entry.txt"}
	facts.Focus.TargetRole = "implementation"
	facts.Focus.RequireTargetGrounding = true
	facts.Capabilities = runtimecontract.RuntimeCapabilities{
		StateChangeKinds: []string{runtimecontract.StepKindFileEdit},
		ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	validationRecipe := runtimecontract.RuntimeValidationRecipe{ID: facts.Focus.ValidationRecipeID, Command: "true"}
	const validationCheck = "check/failure-observation"
	facts.Focus.RequiredCheckRefs = []string{validationCheck}
	validationInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation-admissibility-shell-failure-observation",
		OwnerModuleID:  runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
			},
		}},
		TargetRefs:           []string{"validation-entry.txt"},
		ValidationRecipeRefs: []string{validationRecipe.ID},
		ValidationRecipes:    []runtimecontract.RuntimeValidationRecipe{validationRecipe},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: validationRecipe.ID, OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "failure-observation-input-v1", InputBindingRef: "failure-observation-input",
			CheckRefs: []string{validationCheck}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
			EffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		}},
		ObservedRevision: "shell-policy-failure-observation-v1",
	}
	validationInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(validationInventory)
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "validation-entry.txt", ContentHash: "validation-entry-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{
		ID: validationInventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID:        runtimecontract.RuntimeModuleShell,
		RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Reason: "Shell validation inventory",
	}}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{validationInventory}
	facts.Resources.PrerequisiteObservations = nil

	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil || initialized.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("validation aggregate initialization=%+v err=%v", initialized, err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
	if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "failure-observation-prefix"); err != nil {
		t.Fatal(err)
	}
	path := "validation-entry.txt"
	recipeID := validationRecipe.ID
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "failure-observation-read", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{path},
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}}}}}
	if proposed, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate); err != nil || !proposed.Admission.Accepted() {
		t.Fatalf("failure-observation validation proposal=%+v err=%v", proposed, err)
	}
	recordRuntimeFileBackedManagedValidationFeasibility(t, ctx, controller)
	sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("validation candidate seal=%+v err=%v", sealed, err)
	}
	if sealed.Aggregate.ActiveDispatch.Candidate.Validation == nil || len(sealed.Aggregate.ActiveDispatch.Candidate.Validation.Operations) != 1 {
		t.Fatalf("validation candidate sealed unexpected authority=%+v", sealed.Aggregate.ActiveDispatch.Candidate)
	}
	sealedOperationID := sealed.Aggregate.ActiveDispatch.Candidate.Validation.Operations[0].Operation.ID

	newExecutor := func(owner *runtime.RuntimeController, crashOnReceiptError bool) runtime.RuntimeOperationExecutor {
		return runtimeOperationExecutorFunc(func(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
			if len(request.ValidationScopeTargetRefs) == 0 {
				return runtime.RuntimeOperationExecutionResult{}, errors.New("sealed validation scope is required")
			}
			observation := runtimecontract.RuntimeValidationFailureObservation{
				WorkspaceID: request.WorkspaceID, RunID: request.RunID,
				AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID, DispatchID: request.DispatchID, OperationID: request.Operation.ID,
				FailureSignature: "implementation-check-failed", TargetRole: request.TargetRole,
				DiagnosticClass:     runtimecontract.ValidationDiagnosticBuildFailed,
				CandidateTargetRefs: []string{path}, ValidationScopeTargetRefs: append([]string(nil), request.ValidationScopeTargetRefs...),
				DiagnosticHash: strings.Repeat("f", 64),
			}
			observedFailure := runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassBuildFailure, Code: "implementation_check_failed",
				Signature: observation.FailureSignature, Message: "capability observed implementation failure",
				ValidationDiagnostic:         &runtimecontract.ValidationFailureDiagnostic{Class: runtimecontract.ValidationDiagnosticBuildFailed, TargetRefs: append([]string(nil), observation.CandidateTargetRefs...)},
				ValidationFailureObservation: &observation,
			}
			receipt := runtimecontract.RuntimeOperationReceipt{
				ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
				Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectEphemeralSession,
				Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "implementation check failed"},
			}
			descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(request.Operation.Kind)
			if !found {
				return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("unknown operation kind %q", request.Operation.Kind)
			}
			feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
				WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID, DispatchID: request.DispatchID,
			}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(receipt.Outcome), descriptor.OwnerModuleID, observedFailure)
			if err != nil {
				return runtime.RuntimeOperationExecutionResult{}, err
			}
			feedback.OriginalDiagnostic.OperationID = request.Operation.ID
			receipt.Feedback = &feedback
			_, committed, err := runtimeTestControllerCommitOperationReceipt(owner, ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, descriptor.OwnerModuleID, receipt)
			if err != nil {
				if crashOnReceiptError {
					panic("failure-observation-receipt-cas")
				}
				return runtime.RuntimeOperationExecutionResult{}, err
			}
			return runtime.RuntimeOperationExecutionResult{
				EventID: "failure-observation-event", OperationID: request.Operation.ID, EvidenceKind: runtimecontract.EvidenceKindValidation,
				Status: runtime.ExecutionEvidenceStatusRejected, SourceRef: "workorch://validation/failure-observation",
				Failure: &observedFailure, Evidence: committed.Evidence, Receipt: committed,
			}, nil
		})
	}

	beforeCommitRepository := &failWhenRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected owner failure receipt CAS failure"),
		match: func(aggregate runtime.RuntimeAggregate) bool {
			return len(aggregate.OperationReceipts) == 1
		},
	}
	beforeCommitController, err := runtime.NewRuntimeController(beforeCommitRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "failure-observation-receipt-cas" {
				t.Fatalf("unexpected receipt crash marker: %v", recovered)
			}
		}()
		_, _ = runtimeTestControllerExecuteDispatch(beforeCommitController, ctx, "ws-v2", "run-v2", newExecutor(beforeCommitController, true))
		t.Fatal("expected injected failure-observation receipt crash")
	}()
	if !beforeCommitRepository.failed {
		t.Fatal("failure-observation receipt CAS crash point was not reached")
	}

	restarted, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	beforeRetry, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeRetry.OperationReceipts) != 0 || beforeRetry.Facts.Failure != nil || beforeRetry.Facts.Resources.FailureTarget != nil ||
		len(beforeRetry.Facts.Evidence.Rejected) != 0 || beforeRetry.ActiveDispatch == nil {
		t.Fatalf("receipt CAS failure partially persisted failure authority: %+v", beforeRetry)
	}

	ownerCalled := false
	_, err = runtimeTestControllerExecuteDispatch(restarted, ctx, "ws-v2", "run-v2", runtimeOperationExecutorFunc(func(context.Context, runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
		ownerCalled = true
		return runtime.RuntimeOperationExecutionResult{}, errors.New("managed validation must not be replayed")
	}))
	var outcomeUnknown *runtime.OperationOutcomeUnknownError
	if !errors.As(err, &outcomeUnknown) {
		t.Fatalf("receipt-loss restart err=%v, want outcome_unknown", err)
	}
	if ownerCalled {
		t.Fatal("receipt-loss restart invoked the managed-validation owner")
	}
	persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.ActiveDispatch == nil || len(persisted.OperationReceipts) != 0 ||
		len(persisted.Facts.Evidence.Rejected) != 0 || persisted.Facts.Failure != nil || persisted.Facts.Resources.FailureTarget != nil {
		t.Fatalf("receipt-loss restart partially persisted failure authority: aggregate=%+v err=%v", persisted, err)
	}
	execution, found := runtimeDriveOperationExecutionRecord(persisted.ActiveDispatch.OperationExecutions, sealedOperationID)
	if !found || execution.State != runtimecontract.OperationExecutionOutcomeUnknown {
		t.Fatalf("receipt-loss restart execution=%+v found=%v", execution, found)
	}
}

// A receipt may carry only observations authored by the sealed operation
// owner. Prove that a File operation cannot smuggle an MCP target observation
// through the production Run-owner persistence boundary, and that restart sees
// neither a receipt nor any derived SM-2 repair authority.
func TestRuntimeFileBackedRestartRejectsCrossOwnerTargetObservationAtomically(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}

	firstHub := magicevent.NewHub(16)
	t.Cleanup(func() { firstHub.Terminate(ctx) })
	firstRunOwner := runmodule.New()
	if err := firstRunOwner.Configure(runOwnerTestDependencies(runs, nil, nil, firstHub)); err != nil {
		t.Fatal(err)
	}
	if err := firstRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = firstRunOwner.Stop(ctx) })

	facts := runtimeCandidateSealExactFileEditFacts(t, "main.go")
	facts.Focus.TargetRole = "implementation"
	facts.Focus.RequireTargetGrounding = true
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "main.go", ContentHash: "main-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}

	firstRepository := runtimeStateRepository{hub: firstHub}
	firstController, err := runtime.NewRuntimeController(firstRepository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(firstController, ctx, facts, runtimeDefinition(), nil)
	if err != nil || initialized.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("initial cross-owner aggregate=%+v err=%v", initialized, err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, firstRepository, initialized)
	if _, err := beginRuntimeCandidateAttemptForTest(ctx, firstController, "ws-v2", "run-v2", 0, "cross-owner-receipt-prefix"); err != nil {
		t.Fatal(err)
	}
	mainTarget := "main.go"
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "cross-owner-file-edit", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{mainTarget},
		Parameters: runtimeFileEditParameters(mainTarget),
	}}}}}
	if proposed, proposalErr := admitRuntimeCandidateProposalForTest(ctx, firstController, "ws-v2", "run-v2", 0, candidate); proposalErr != nil || !proposed.Admission.Accepted() {
		t.Fatalf("cross-owner candidate proposal=%+v err=%v", proposed, proposalErr)
	}
	recordRuntimeFileEditFeasibility(t, ctx, firstController)
	sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, firstController, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("cross-owner candidate seal=%+v err=%v", sealed, err)
	}

	ownerFailure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "cross_owner_target_observation",
		Signature: "cross-owner-target-observation", Message: "File receipt carried MCP target authority",
		TargetRefs: []string{"implementation.go"},
		TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
			OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation",
			ObservedRevision: "mcp-catalog-v1", TargetRefs: []string{"implementation.go"},
		},
	}
	executor := runtimeOperationExecutorFunc(func(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
		receipt := runtimecontract.RuntimeOperationReceipt{
			ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
			Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
			Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "invalid cross-owner observation"},
		}
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(request.Operation.Kind)
		if !found {
			return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("unknown operation kind %q", request.Operation.Kind)
		}
		feedback, err := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
			WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID, DispatchID: request.DispatchID,
		}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(receipt.Outcome), descriptor.OwnerModuleID, ownerFailure)
		if err != nil {
			return runtime.RuntimeOperationExecutionResult{}, err
		}
		feedback.OriginalDiagnostic.OperationID = request.Operation.ID
		receipt.Feedback = &feedback
		_, _, err = runtimeTestControllerCommitOperationReceipt(firstController, ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, descriptor.OwnerModuleID, receipt)
		return runtime.RuntimeOperationExecutionResult{}, err
	})
	if _, err := runtimeTestControllerExecuteDispatch(firstController, ctx, "ws-v2", "run-v2", executor); err == nil ||
		!strings.Contains(err.Error(), "operation descriptor does not admit direct target observations") {
		t.Fatalf("cross-owner File receipt error = %v", err)
	}
	beforeRestart, err := firstController.Load(ctx, "ws-v2", "run-v2")
	if err != nil || beforeRestart.ActiveDispatch == nil || len(beforeRestart.OperationReceipts) != 0 ||
		beforeRestart.Facts.Failure != nil || beforeRestart.Facts.Resources.FailureTarget != nil ||
		len(beforeRestart.Facts.Evidence.Rejected) != 0 || len(runtime.ActiveRejectedOperationShapes(beforeRestart)) != 0 {
		t.Fatalf("cross-owner receipt partially persisted before restart: aggregate=%+v err=%v", beforeRestart, err)
	}

	if err := firstRunOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	firstHub.Terminate(ctx)
	secondHub := magicevent.NewHub(16)
	t.Cleanup(func() { secondHub.Terminate(ctx) })
	secondRunOwner := runmodule.New()
	if err := secondRunOwner.Configure(runOwnerTestDependencies(runs, nil, nil, secondHub)); err != nil {
		t.Fatal(err)
	}
	if err := secondRunOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondRunOwner.Stop(ctx) })

	restarted, err := runtime.NewRuntimeController(runtimeStateRepository{hub: secondHub})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.ActiveDispatch == nil || len(loaded.OperationReceipts) != 0 ||
		loaded.Facts.Failure != nil || loaded.Facts.Resources.FailureTarget != nil ||
		len(loaded.Facts.Evidence.Rejected) != 0 || len(runtime.ActiveRejectedOperationShapes(loaded)) != 0 {
		t.Fatalf("file-backed restart recovered cross-owner authority: aggregate=%+v err=%v", loaded, err)
	}
}

// Assessment is the SM-2 boundary that turns accepted evidence into coverage
// progress (and possibly a terminal authority). Its CAS must therefore keep
// the assessment dispatch, coverage, repair ledger and terminal decision
// indivisible across a Run-owner restart.
func TestRuntimeStateRepositoryAssessmentCommitCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	hub, stopFirstProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFirstProcess)

	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := runtimeCandidateSealExactFileEditFacts(t, "target.txt")
	facts.Focus.RequireAssessment = true
	facts.DefinitionFocuses[0].RequireAssessment = true
	target := "target.txt"
	facts.Focus.TargetRefs = []string{target}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
	initialized = commitRuntimeAssessmentTargetSnapshotForTest(t, ctx, controller, initialized, target)
	if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "assessment-state-change-prefix"); err != nil {
		t.Fatalf("begin assessment state-change attempt: %v aggregate=%+v", err, initialized)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "assessment-write", Kind: runtimecontract.StepKindFileEdit,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target},
		Parameters: runtimeFileEditParameters(target),
	}}}}}
	if proposed, proposalErr := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate); proposalErr != nil || !proposed.Admission.Accepted() {
		t.Fatalf("assessment state-change proposal=%+v err=%v", proposed, proposalErr)
	}
	recordRuntimeFileEditFeasibility(t, ctx, controller)
	sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, candidate)
	if err != nil || !sealed.Admission.Accepted() || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("state-change seal=%+v err=%v", sealed, err)
	}
	stateChangeExecutor := runtimeOperationExecutorFunc(func(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
		receipt := runtimecontract.RuntimeOperationReceipt{
			ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
			Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
			Changed: true, TargetRefs: append([]string(nil), request.Operation.TargetRefs...),
			Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "state change applied"},
		}
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(request.Operation.Kind)
		if !found || descriptor.OwnerModuleID == "" {
			return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("unknown operation kind %q", request.Operation.Kind)
		}
		_, committed, err := runtimeTestControllerCommitOperationReceipt(controller, ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, descriptor.OwnerModuleID, receipt)
		if err != nil {
			return runtime.RuntimeOperationExecutionResult{}, err
		}
		return runtime.RuntimeOperationExecutionResult{
			EventID: "assessment-state-change-evidence", OperationID: request.Operation.ID, EvidenceKind: runtimecontract.EvidenceKindStateChange,
			Status: runtime.ExecutionEvidenceStatusAccepted, SourceRef: "workorch://state-change/assessment-write",
			Evidence: committed.Evidence, Receipt: committed,
		}, nil
	})
	afterEvidence, err := runtimeTestControllerExecuteDispatch(controller, ctx, "ws-v2", "run-v2", stateChangeExecutor)
	if err != nil || afterEvidence.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("state-change evidence did not reach assessment: aggregate=%+v err=%v", afterEvidence, err)
	}
	if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "assessment-commit-prefix"); err != nil {
		t.Fatal(err)
	}
	assessmentCandidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: afterEvidence.Decision.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "current focus satisfied",
		EvidenceRefs: []string{"assessment-state-change-evidence"},
		Items: []runtimecontract.RuntimeAssessmentItemFact{{
			CoverageWorkItemID: "cwi-v2", Satisfied: true, EvidenceRefs: []string{"assessment-state-change-evidence"},
		}},
	}}
	assessmentDispatch, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, assessmentCandidate)
	if err != nil || !assessmentDispatch.Admission.Accepted() || assessmentDispatch.Aggregate.ActiveDispatch == nil {
		t.Fatalf("assessment seal disposition=%s failure=%+v correction=%+v err=%v", assessmentDispatch.Admission.Disposition, assessmentDispatch.Admission.Failure, assessmentDispatch.Admission.Correction, err)
	}

	beforeCommit, err := runtime.NewRuntimeController(&failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository,
		err:                    errors.New("injected assessment CAS failure"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commitRuntimeAssessmentForTest(ctx, beforeCommit, "ws-v2", "run-v2"); err == nil {
		t.Fatal("assessment unexpectedly persisted despite injected Run-owner CAS failure")
	}

	// A failed pre-CAS transition must disappear with its process. Rebuild the
	// Hub and Run owner before loading, rather than proving only a new
	// controller can read through the previous process' owner.
	stopFirstProcess()
	restartedHub, stopSecondProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopSecondProcess)
	restartedRepository := runtimeStateRepository{hub: restartedHub}
	restarted, err := runtime.NewRuntimeController(restartedRepository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StateRevision != assessmentDispatch.Aggregate.StateRevision || loaded.ActiveDispatch == nil || loaded.ActiveDispatch.Candidate.Assessment == nil ||
		len(loaded.Facts.Assessments) != 0 || loaded.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateOpen ||
		loaded.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("assessment CAS failure partially persisted progress or terminal authority: %+v", loaded)
	}

	// The same assessment transition now commits, but its caller loses the
	// response after the Run-owner durable write. The next controller is only
	// allowed to consume that exact aggregate; it must not re-run assessment or
	// advance focus a second time.
	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: restartedRepository, marker: "assessment-cas-response-loss",
		match: func(aggregate runtime.RuntimeAggregate) bool {
			return aggregate.Decision.State == runtimecontract.ConvergenceStateComplete && aggregate.ActiveDispatch == nil &&
				len(aggregate.Facts.Assessments) == 1 && aggregate.Facts.Coverage[0].State == runtimecontract.RuntimeCoverageItemStateSatisfied
		},
	}
	responseLost, err := runtime.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "assessment-cas-response-loss" {
				t.Fatalf("unexpected assessment response-loss marker: %v", recovered)
			}
		}()
		observed, observedErr := commitRuntimeAssessmentForTest(ctx, responseLost, "ws-v2", "run-v2")
		t.Logf("same-gap response-loss attempt aggregate=%+v err=%v", observed, observedErr)
		t.Fatal("expected assessment response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("assessment response-loss CAS point was not reached")
	}

	// Repeat the full process reconstruction after the durable CAS / lost
	// response boundary. This is the real recovery path; it must only consume
	// the committed assessment delta.
	stopSecondProcess()
	finalHub, stopFinalProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFinalProcess)
	finalRepository := runtimeStateRepository{hub: finalHub}
	restartedAfterResponseLoss, err := runtime.NewRuntimeController(finalRepository)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := restartedAfterResponseLoss.Load(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.StateRevision <= assessmentDispatch.Aggregate.StateRevision || persisted.Decision.State != runtimecontract.ConvergenceStateComplete ||
		persisted.ActiveDispatch != nil || len(persisted.Facts.Assessments) != 1 || persisted.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateSatisfied || persisted.Facts.Repair.LastProgressRevision == "" {
		t.Fatalf("restart did not reuse response-loss assessment transition: aggregate=%+v err=%v", persisted, err)
	}
	if _, err := commitRuntimeAssessmentForTest(ctx, restartedAfterResponseLoss, "ws-v2", "run-v2"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("committed assessment was replayed instead of rejected: %v", err)
	}
	reloaded, err := restartedAfterResponseLoss.Load(ctx, "ws-v2", "run-v2")
	if err != nil || reloaded.StateRevision != persisted.StateRevision || len(reloaded.Facts.Assessments) != 1 ||
		reloaded.Facts.Repair.LastProgressRevision != persisted.Facts.Repair.LastProgressRevision {
		t.Fatalf("assessment response-loss replay changed committed aggregate: reloaded=%+v err=%v", reloaded, err)
	}
}

// Failure facts are SM-2 inputs, not delivery diagnostics. The failure,
// terminal authority and closed candidate surface must therefore survive the
// same full-process CAS boundaries as assessment repair facts.
func TestRuntimeStateRepositoryFailureFactCrashRestartIsAtomic(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassRuntimeContractMisalignment, Code: "runtime_drive_failed",
		Signature: "runtime-drive-failed", Message: "sealed transaction cannot be reconciled",
	}
	hub, stopFirstProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFirstProcess)
	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := runtimeTestControllerInitialize(controller, ctx, runtimeAggregate(t, 1).Facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	beforeCommit, err := runtime.NewRuntimeController(&failOnceRuntimeStateRepository{
		RuntimeStateRepository: repository, err: errors.New("injected failure fact CAS failure"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeTestControllerRecordFailure(beforeCommit, ctx, "ws-v2", "run-v2", failure); err == nil {
		t.Fatal("failure fact unexpectedly persisted before Run-owner CAS")
	}

	stopFirstProcess()
	restartedHub, stopSecondProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopSecondProcess)
	restartedRepository := runtimeStateRepository{hub: restartedHub}
	restarted, err := runtime.NewRuntimeController(restartedRepository)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || pending.StateRevision != initialized.StateRevision || pending.Facts.Failure != nil ||
		pending.Decision.State == runtimecontract.ConvergenceStateFailed || pending.ActiveDispatch != nil {
		t.Fatalf("pre-CAS failure leaked through fresh process: aggregate=%+v err=%v", pending, err)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: restartedRepository, marker: "failure-fact-cas-response-loss",
		match: func(aggregate runtime.RuntimeAggregate) bool {
			return aggregate.Facts.Failure != nil && aggregate.Facts.Failure.Signature == failure.Signature &&
				aggregate.Decision.State == runtimecontract.ConvergenceStateFailed && aggregate.Decision.Terminal != nil
		},
	}
	responseLost, err := runtime.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "failure-fact-cas-response-loss" {
				t.Fatalf("unexpected failure fact response-loss marker: %v", recovered)
			}
		}()
		_, _ = runtimeTestControllerRecordFailure(responseLost, ctx, "ws-v2", "run-v2", failure)
		t.Fatal("expected failure fact response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("failure fact response-loss CAS point was not reached")
	}

	stopSecondProcess()
	finalHub, stopFinalProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFinalProcess)
	final, err := runtime.NewRuntimeController(runtimeStateRepository{hub: finalHub})
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := final.Load(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.StateRevision <= initialized.StateRevision || persisted.Facts.Failure == nil ||
		persisted.Facts.Failure.Signature != failure.Signature || persisted.Decision.State != runtimecontract.ConvergenceStateFailed ||
		persisted.Decision.Terminal == nil || persisted.ActiveDispatch != nil {
		t.Fatalf("post-CAS failure did not survive fresh process: aggregate=%+v err=%v", persisted, err)
	}
	revision := persisted.StateRevision
	duplicate, err := runtimeTestControllerRecordFailure(final, ctx, "ws-v2", "run-v2", failure)
	if err != nil || duplicate.StateRevision != revision || duplicate.Facts.Failure == nil || duplicate.Facts.Failure.Signature != failure.Signature {
		t.Fatalf("terminal failure duplicate changed authority: aggregate=%+v err=%v", duplicate, err)
	}
}

// A producer may claim a repairable same gap even when persisted, owner-backed
// evidence already closes the coverage. The claim remains audit data; the
// canonical assessment transition completes exactly once across CAS failure,
// response loss, and a fresh-process reload.
func TestRuntimeStateRepositoryClaimedSameGapAssessmentCrashRestartUsesCanonicalCoverage(t *testing.T) {
	ctx := context.Background()
	runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
	bootstrap := runtimeFileBackedAssessmentBootstrap([]common.CoverageWorkItemID{"cwi-v2"})
	if err := runs.Create(ctx, runtimeRunForTestBootstrap(common.RunStateActive, bootstrap)); err != nil {
		t.Fatal(err)
	}
	hub, stopFirstProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFirstProcess)

	repository := runtimeStateRepository{hub: hub}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := bootstrap.InitialFacts("ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	facts.Focus.RequireAssessment = true
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Focus.ValidationRecipeID = runtimeFileBackedValidationRecipeID
	facts.Focus.RequiredCheckRefs = []string{runtimeFileBackedValidationCheck}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileEdit}
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: facts.Focus.ID, SliceID: facts.Focus.SliceID, RepairAttempts: 2, NoProgressCount: 1,
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...), Reason: "same gap"},
	}
	fileOwnerFacts := runtimeCandidateSealExactFileEditFacts(t, "target.txt")
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, fileOwnerFacts.Prerequisites.Satisfied...)
	facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, fileOwnerFacts.Resources.OperationAdmissibilities...)
	facts.Resources.PrerequisiteObservations = append(facts.Resources.PrerequisiteObservations, fileOwnerFacts.Resources.PrerequisiteObservations...)
	facts = runtimeFactsWithFileBackedManagedValidationContract(facts)
	initialized, err := runtimeTestControllerInitialize(controller, ctx, facts, runtimeDefinition(), nil)
	if err != nil {
		t.Fatal(err)
	}
	initialized = runtimeTestPersistSealedSemanticAuthority(t, ctx, repository, initialized)
	initialized = commitRuntimeAssessmentTargetSnapshotForTest(t, ctx, controller, initialized, "target.txt")
	begun, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "same-gap-state-prefix")
	if err != nil {
		t.Fatalf("begin same-gap state-change attempt: %v aggregate=%+v", err, initialized)
	}
	if begun.Facts.RepairDirective != nil || begun.Decision.RequiredOutput.RepairClosureID != "" {
		t.Fatalf("superseded same-gap repair closure leaked into canonical candidate authority: aggregate=%+v", begun)
	}
	stateCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "same-gap-write", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileEditParameters("target.txt"),
	}}}}}
	if proposed, proposalErr := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, stateCandidate); proposalErr != nil || !proposed.Admission.Accepted() {
		t.Fatalf("same-gap state candidate proposal=%+v err=%v", proposed, proposalErr)
	}
	recordRuntimeFileEditFeasibility(t, ctx, controller)
	sealed, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, stateCandidate)
	if err != nil || sealed.Aggregate.ActiveDispatch == nil {
		t.Fatalf("same-gap state candidate seal=%+v err=%v", sealed, err)
	}
	if sealed.Aggregate.Facts.RepairDirective != nil || sealed.Aggregate.Decision.RequiredOutput.RepairClosureID != "" ||
		sealed.Aggregate.ActiveDispatch.Transaction.RequiredOutputHash != sealed.Aggregate.Decision.AuthorityTransaction().RequiredOutputHash {
		t.Fatalf("same-gap sealed dispatch did not retain canonical authority: aggregate=%+v", sealed.Aggregate)
	}
	executor := runtimeOperationExecutorFunc(func(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
		receipt := runtimecontract.RuntimeOperationReceipt{
			ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
			ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: append([]string(nil), request.Operation.TargetRefs...), Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "same-gap write"},
		}
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(request.Operation.Kind)
		if !found || descriptor.OwnerModuleID == "" {
			return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("unknown operation kind %q", request.Operation.Kind)
		}
		_, committed, err := runtimeTestControllerCommitOperationReceipt(controller, ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, descriptor.OwnerModuleID, receipt)
		if err != nil {
			return runtime.RuntimeOperationExecutionResult{}, err
		}
		return runtime.RuntimeOperationExecutionResult{EventID: "same-gap-evidence", OperationID: request.Operation.ID, EvidenceKind: runtimecontract.EvidenceKindStateChange, Status: runtime.ExecutionEvidenceStatusAccepted, Evidence: committed.Evidence, Receipt: committed}, nil
	})
	afterEvidence, err := runtimeTestControllerExecuteDispatch(controller, ctx, "ws-v2", "run-v2", executor)
	if err != nil || afterEvidence.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("same-gap repair state evidence did not require fresh validation: decision=%+v terminal=%+v repairDirective=%+v aggregate=%+v err=%v", afterEvidence.Decision, afterEvidence.Decision.Terminal, afterEvidence.Facts.RepairDirective, afterEvidence, err)
	}
	afterValidation := executeRuntimeFreshValidationForAssessment(t, ctx, controller, facts.Focus.CoverageWorkItemIDs, "same-gap-validation-prefix", "same-gap-validation-evidence", "same-gap-validation")
	if afterValidation.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("same-gap fresh validation did not reach assessment: aggregate=%+v", afterValidation)
	}
	if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "same-gap-assessment-prefix"); err != nil {
		t.Fatal(err)
	}
	assessmentCandidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{
		FocusID: afterValidation.Facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary: "same gap persists", Gap: &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Reason: "same gap"},
	}}
	assessmentDispatch, err := admitAndSealRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, assessmentCandidate)
	if err != nil || assessmentDispatch.Aggregate.ActiveDispatch == nil {
		t.Fatalf("same-gap assessment seal=%+v err=%v", assessmentDispatch, err)
	}

	beforeCommit, err := runtime.NewRuntimeController(&failOnceRuntimeStateRepository{RuntimeStateRepository: repository, err: errors.New("injected same-gap assessment CAS failure")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commitRuntimeAssessmentForTest(ctx, beforeCommit, "ws-v2", "run-v2"); err == nil {
		t.Fatal("same-gap assessment unexpectedly persisted before CAS")
	}
	stopFirstProcess()
	restartedHub, stopSecondProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopSecondProcess)
	restartedRepository := runtimeStateRepository{hub: restartedHub}
	restartedBefore, err := runtime.NewRuntimeController(restartedRepository)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := restartedBefore.Load(ctx, "ws-v2", "run-v2")
	if err != nil || pending.ActiveDispatch == nil || pending.ActiveDispatch.Candidate.Assessment == nil ||
		pending.ActiveDispatch.ID != assessmentDispatch.Aggregate.ActiveDispatch.ID || len(pending.Facts.Assessments) != 0 ||
		!reflect.DeepEqual(pending.Facts.Repair, assessmentDispatch.Aggregate.Facts.Repair) {
		t.Fatalf("same-gap pre-commit restart leaked progress: aggregate=%+v err=%v", pending, err)
	}

	responseLossRepository := &panicAfterRuntimeStateSaveRepository{
		RuntimeStateRepository: restartedRepository, marker: "same-gap-assessment-response-loss",
		match: func(aggregate runtime.RuntimeAggregate) bool {
			return aggregate.ActiveDispatch == nil && len(aggregate.Facts.Assessments) == 1 &&
				aggregate.Facts.Assessments[0].ClaimedDisposition == runtimecontract.AssessmentDispositionRepairableGap &&
				aggregate.Facts.Assessments[0].Disposition == runtimecontract.AssessmentDispositionSatisfied
		},
	}
	responseLost, err := runtime.NewRuntimeController(responseLossRepository)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != "same-gap-assessment-response-loss" {
				t.Fatalf("unexpected same-gap response-loss marker: %v", recovered)
			}
		}()
		_, _ = commitRuntimeAssessmentForTest(ctx, responseLost, "ws-v2", "run-v2")
		t.Fatal("expected same-gap assessment response loss after durable CAS")
	}()
	if !responseLossRepository.fired {
		t.Fatal("same-gap assessment response-loss CAS point was not reached")
	}

	stopSecondProcess()
	finalHub, stopFinalProcess := startRuntimeFileBackedRunOwner(t, ctx, runs)
	t.Cleanup(stopFinalProcess)
	restarted, err := runtime.NewRuntimeController(runtimeStateRepository{hub: finalHub})
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.StateRevision <= assessmentDispatch.Aggregate.StateRevision || persisted.ActiveDispatch != nil || len(persisted.Facts.Assessments) != 1 ||
		persisted.Facts.Assessments[0].ClaimedDisposition != runtimecontract.AssessmentDispositionRepairableGap ||
		persisted.Facts.Assessments[0].Disposition != runtimecontract.AssessmentDispositionSatisfied ||
		persisted.Facts.Coverage[0].State != runtimecontract.RuntimeCoverageItemStateSatisfied || persisted.Facts.Repair.ActiveGap != nil ||
		persisted.Decision.State != runtimecontract.ConvergenceStateComplete || persisted.Decision.Terminal == nil {
		t.Fatalf("same-gap response-loss restart did not retain canonical coverage: aggregate=%+v err=%v", persisted, err)
	}
	if _, err := commitRuntimeAssessmentForTest(ctx, restarted, "ws-v2", "run-v2"); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same-gap assessment replay was not rejected: %v", err)
	}
}

// Partial assessment is no longer a valid way to close a shared semantic
// focus. Each CWI receives a distinct focus, receipt and assessment. Verify
// that the Run-owner bootstrap rejects the old persisted shape before it can
// be saved or later recovered as an opaque aggregate.
func TestRuntimeStateRepositoryRejectsSharedSemanticFocusBeforePersistence(t *testing.T) {
	bootstrap := runtimeFileBackedAssessmentBootstrap([]common.CoverageWorkItemID{"cwi-a", "cwi-b"})
	bootstrap.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{{
		ID: "shared-focus", SliceID: bootstrap.Slices[0].ID,
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-a", "cwi-b"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		RequireAssessment:     true,
	}}
	if err := bootstrap.Validate(); err == nil {
		t.Fatalf("shared semantic focus bootstrap was accepted: %+v", bootstrap)
	}
}

func TestRuntimeMainlinePreparesLocalPrerequisiteThroughPersistedController(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceInternalDirectory := t.TempDir()
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: workspaceInternalDirectory,
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	biz := newRuntimeTestOwner()
	if err := biz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	facts := runtimeAggregate(t, 1).Facts
	initialized, err := biz.InitializeRuntime(ctx, facts, runtimeDefinition(), []runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindValidationScratch}})
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	prepared, err := module.prepareRuntimePrerequisites(ctx, initialized)
	if err != nil {
		t.Fatalf("prepareRuntimePrerequisites: %v", err)
	}
	if prepared.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || prepared.Facts.Resources.ValidationScratch == nil {
		t.Fatalf("prepared aggregate = %+v", prepared)
	}
	loaded, err := biz.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.StateRevision != prepared.StateRevision {
		t.Fatalf("persisted prepared aggregate = %+v err=%v", loaded, err)
	}
}

func TestRuntimeOperationAdmissibilityRefreshUsesGroundedFailureTarget(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	observer := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	defer observer.Unsubscribe(fileevents.TopicAcquireRuntimeOperationAdmissibility)
	var captured fileevents.AcquireRuntimeOperationAdmissibilityCommand
	observer.Subscribe(fileevents.TopicAcquireRuntimeOperationAdmissibility, func(event magicevent.Event, result magicevent.Result) {
		captured = event.Data().(fileevents.AcquireRuntimeOperationAdmissibilityCommand)
		inventory := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: captured.PrerequisiteID, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: captured.RequiredKinds, TargetRefs: captured.RequiredTargetRefs, ObservedRevision: "file-inventory-v1",
		}
		inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
		observation := runtimeFileAdmissibilityObservation(inventory)
		result.Set(fileevents.AcquireRuntimeOperationAdmissibilityResult{Acquisition: runtimecontract.RuntimeOperationAdmissibilityAcquisition{
			WorkspaceID: captured.WorkspaceID, RunID: captured.RunID, AuthoritySnapshotID: captured.AuthoritySnapshotID,
			Admissibility: inventory, Observation: observation,
		}}, nil)
	})

	aggregate := runtimeAggregate(t, 1)
	aggregate.Facts.Focus.TargetRefs = nil
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Focus.RequireTargetGrounding = true
	aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{TargetRefs: []string{"implementation.go"}}
	decision, err := runtimecontract.SealRuntimeDecision(aggregate.Facts, runtimecontract.RuntimeDecision{
		State: aggregate.Decision.State, Focus: aggregate.Facts.Focus, RequiredOutput: aggregate.Decision.RequiredOutput,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "operation_admissibility:file-refresh", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit},
	}
	module := newTestBiz(newRuntimeTestOwner(), hub, nil)
	resolved, err := module.resolveRuntimePrerequisite(ctx, aggregate, t.TempDir(), t.TempDir(), prerequisite)
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.RequiredTargetRefs) != 1 || captured.RequiredTargetRefs[0] != "implementation.go" ||
		resolved.OperationAdmissibility == nil || len(resolved.OperationAdmissibility.TargetRefs) != 1 || resolved.OperationAdmissibility.TargetRefs[0] != "implementation.go" {
		t.Fatalf("operation admissibility refresh lost persisted grounded failure target: command=%+v result=%+v", captured, resolved)
	}
}

func TestRuntimeMainlineRequestsAndSealsTypedCandidate(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	startRuntimeReceiptTestOwner(t, hub)
	startRuntimeCandidateFileEditAdmissionOwners(t, hub)

	var requestedDefinition runtimecontract.RuntimeDefinitionContext
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		requestedDefinition = command.DefinitionContext
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeFileEditParameters("target.txt"),
		}}}}}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(fileevents.TopicExecuteFileEditOperation, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(fileevents.ExecuteFileEditOperationCommand)
		receipt := orchestratorevents.NewRuntimeOperationReceipt(command.Execution, runtimecontract.RuntimeModuleFile, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusAccepted, command.TargetRefs, runtimecontract.RuntimeOperationEvidence{}, nil, nil, "2026-07-20T00:00:00Z")
		committed, err := commitRuntimeOperationReceiptForTest(event.Context(), hub, runtimecontract.RuntimeModuleFile, command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
		if err != nil {
			t.Errorf("commit owner receipt: %v", err)
			return
		}
		result.Set(fileevents.ExecuteOperationResult{
			Receipt: committed,
		}, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(fileevents.TopicExecuteFileEditOperation) })

	biz := newRuntimeTestOwner()
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, biz, runtimeRepository, runtimeCandidateSealFileEditFacts(t), runtimeDefinition(), nil)
	module := newTestBiz(biz, hub, nil)
	sealed, err := module.requestAndSealRuntimeCandidate(ctx, initialized, "stable-prefix-v2", 0)
	if err != nil {
		t.Fatalf("requestAndSealRuntimeCandidate: %v", err)
	}
	if sealed.ActiveDispatch == nil || sealed.ActiveDispatch.Transaction != initialized.Decision.AuthorityTransaction() {
		t.Fatalf("sealed aggregate = %+v", sealed)
	}
	if !runtimecontract.SameRuntimeDefinitionContext(requestedDefinition, runtimeDefinition()) {
		t.Fatalf("candidate request definition = %+v", requestedDefinition)
	}
	completed, err := module.executeRuntimeActiveDispatch(ctx, sealed)
	if err != nil {
		t.Fatalf("executeRuntimeActiveDispatch: %v", err)
	}
	if completed.ActiveDispatch != nil || completed.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("completed aggregate = %+v", completed)
	}
	loaded, err := biz.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.ActiveDispatch != nil || loaded.Decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("persisted completed aggregate = %+v err=%v", loaded, err)
	}
}

// The C6 correction boundary must survive a real Run-owner reload: a provider
// can rename operations or reorder JSON fields, but that cannot turn a prior
// complete rejected candidate into a new semantic correction. The two subtests
// cover the CAS-before and response-loss-after terminal boundaries without
// involving a provider replay or a UI recovery path.
func TestRuntimeFileBackedSemanticCandidateReplayExhaustionSurvivesCASBoundaries(t *testing.T) {
	newBaseline := func(t *testing.T) (context.Context, runtime.RuntimeStateRepository, runtimecontract.RuntimeCandidate, runtimecontract.RuntimeCandidate, runtimecontract.RuntimeCandidateContractManifest, string) {
		t.Helper()
		ctx := context.Background()
		hub := magicevent.NewHub(16)
		t.Cleanup(func() { hub.Terminate(ctx) })
		runs := filerepository.NewRunRepository(filepath.Join(t.TempDir(), "runs.json"))
		if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
			t.Fatal(err)
		}
		runOwner := runmodule.New()
		if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
			t.Fatal(err)
		}
		if err := runOwner.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = runOwner.Stop(ctx) })

		repository := runtimeStateRepository{hub: hub}
		aggregate := runtimeAggregate(t, 1)
		aggregate.Facts = runtimeCandidateSealExactFileEditFacts(t, "implementation.go")
		aggregate.Facts.Focus.RequireTargetGrounding = true
		aggregate.Facts.Focus.TargetRole = "implementation"
		aggregate.Facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
			TargetRef: "implementation.go", ContentHash: "implementation-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
		}}
		aggregate = runtimeTestAggregateWithSemanticPlanning(t, aggregate)
		if err := repository.Save(ctx, aggregate, 0); err != nil {
			t.Fatal(err)
		}
		controller, err := runtime.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		wrong := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "provider-first-id", Kind: runtimecontract.StepKindFileEdit,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
			Parameters: runtimeFileEditParameters("target.txt"),
		}}}}}
		equivalent := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{
			Operations: append([]runtimecontract.StateChangingOperation(nil), wrong.StateChange.Operations...),
		}}
		equivalent.StateChange.Operations[0].Operation.ID = "provider-second-id"
		parameters := append([]runtimecontract.OperationParameter(nil), equivalent.StateChange.Operations[0].Operation.Parameters...)
		for left, right := 0, len(parameters)-1; left < right; left, right = left+1, right-1 {
			parameters[left], parameters[right] = parameters[right], parameters[left]
		}
		equivalent.StateChange.Operations[0].Operation.Parameters = parameters
		if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 0, "semantic-replay-prefix"); err != nil {
			t.Fatal(err)
		}
		first, err := admitRuntimeCandidateProposalForTest(ctx, controller, "ws-v2", "run-v2", 0, wrong)
		if err != nil || first.Admission.Failure == nil || first.Admission.Failure.Code != "candidate_contract_invalid" ||
			first.Admission.Failure.Diagnostic == nil || first.Admission.Failure.Diagnostic.Path != "$.operations[0].targetRefs" ||
			first.Aggregate.Decision.State == runtimecontract.ConvergenceStateFailed {
			t.Fatalf("first complete candidate rejection = %+v err=%v", first, err)
		}
		if _, err := beginRuntimeCandidateAttemptForTest(ctx, controller, "ws-v2", "run-v2", 1, "semantic-replay-prefix"); err != nil {
			t.Fatal(err)
		}
		manifest, logicalRequestID, err := directRuntimeCandidateAttemptTestBinding(ctx, controller, "ws-v2", "run-v2", 1)
		if err != nil {
			t.Fatal(err)
		}
		return ctx, repository, wrong, equivalent, manifest, logicalRequestID
	}

	t.Run("before terminal CAS", func(t *testing.T) {
		ctx, repository, _, equivalent, _, _ := newBaseline(t)
		crashing, err := runtime.NewRuntimeController(&failOnceRuntimeStateRepository{
			RuntimeStateRepository: repository, err: errors.New("injected semantic exhaustion CAS failure"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admitRuntimeCandidateProposalForTest(ctx, crashing, "ws-v2", "run-v2", 1, equivalent); err == nil {
			t.Fatal("semantic exhaustion unexpectedly persisted before injected CAS failure")
		}
		restarted, err := runtime.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || len(loaded.CandidateAttempts) != 2 || loaded.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStatePending || loaded.Decision.State == runtimecontract.ConvergenceStateFailed {
			t.Fatalf("pre-CAS restart did not retain only pending correction: aggregate=%+v err=%v", loaded, err)
		}
		committed, err := admitRuntimeCandidateProposalForTest(ctx, restarted, "ws-v2", "run-v2", 1, equivalent)
		if err != nil || committed.Admission.Failure == nil || committed.Admission.Failure.Code != "candidate_replays_rejected_candidate_shape" ||
			committed.Aggregate.Decision.Terminal == nil || committed.Aggregate.Decision.Terminal.Code != "candidate_retry_exhausted" || committed.Aggregate.ActiveDispatch != nil {
			t.Fatalf("restarted semantic exhaustion = %+v err=%v", committed, err)
		}
	})

	t.Run("after terminal CAS response loss", func(t *testing.T) {
		ctx, repository, _, equivalent, manifest, logicalRequestID := newBaseline(t)
		const marker = "semantic-exhaustion-response-loss"
		responseLost, err := runtime.NewRuntimeController(&panicAfterRuntimeStateSaveRepository{
			RuntimeStateRepository: repository, marker: marker,
			match: func(aggregate runtime.RuntimeAggregate) bool {
				return len(aggregate.CandidateAttempts) == 2 && aggregate.CandidateAttempts[1].State == runtimecontract.RuntimeCandidateAttemptStateFailed &&
					aggregate.Decision.State == runtimecontract.ConvergenceStateFailed && aggregate.Decision.Terminal != nil && aggregate.Decision.Terminal.Code == "candidate_retry_exhausted"
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != marker {
					t.Fatalf("unexpected post-CAS response-loss marker: %v", recovered)
				}
			}()
			_, _ = admitRuntimeCandidateProposalForTest(ctx, responseLost, "ws-v2", "run-v2", 1, equivalent)
			t.Fatal("expected response loss after terminal semantic exhaustion CAS")
		}()
		restarted, err := runtime.NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || loaded.Decision.Terminal == nil || loaded.Decision.Terminal.Code != "candidate_retry_exhausted" ||
			len(loaded.CandidateAttempts) != 2 || loaded.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateFailed || loaded.ActiveDispatch != nil || len(loaded.GovernanceRequests) != 0 {
			t.Fatalf("post-CAS restart did not preserve terminal semantic exhaustion: aggregate=%+v err=%v", loaded, err)
		}
		revision := loaded.StateRevision
		persisted := loaded.CandidateAttempts[1].Proposal
		if persisted == nil {
			t.Fatal("terminal response-loss fixture lost its durable rejected proposal")
		}
		duplicate, err := runtimeTestControllerAdmitCandidateProposal(restarted, ctx, "ws-v2", "run-v2", 1, manifest, logicalRequestID, *persisted)
		if err != nil || duplicate.Admission.Failure == nil || duplicate.Admission.Failure.Code != loaded.CandidateAttempts[1].Failure.Code || duplicate.Aggregate.StateRevision != revision {
			t.Fatalf("post-CAS duplicate changed terminal authority: result=%+v err=%v", duplicate, err)
		}
		operation := persisted.StateChange.Operations[0].Operation
		operation.Parameters = append([]runtimecontract.OperationParameter(nil), operation.Parameters...)
		for index := range operation.Parameters {
			if operation.Parameters[index].Name == "newText" {
				changed := "different terminal replay"
				operation.Parameters[index].String = &changed
			}
		}
		operation.ID = ""
		operation.DependsOn = nil
		different := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: operation}}}}
		different, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, different)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtimeTestControllerAdmitCandidateProposal(restarted, ctx, "ws-v2", "run-v2", 1, manifest, logicalRequestID, different); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("post-terminal non-duplicate err=%v, want conflict", err)
		}
		afterDifferent, err := restarted.Load(ctx, "ws-v2", "run-v2")
		if err != nil || afterDifferent.StateRevision != revision {
			t.Fatalf("post-terminal non-duplicate changed authority: aggregate=%+v err=%v", afterDifferent, err)
		}
	})
}

func TestRuntimeOperationExecutorRoutesTypedCommandToCapabilityOwner(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: "/tmp/workspace", InternalDirectory: "/tmp/workspace-internal",
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	var captured fileevents.ExecuteFileEditOperationCommand
	fileObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	fileObserver.Subscribe(fileevents.TopicExecuteFileEditOperation, func(event magicevent.Event, result magicevent.Result) {
		captured = event.Data().(fileevents.ExecuteFileEditOperationCommand)
		result.Set(fileevents.ExecuteOperationResult{}, nil)
	})
	t.Cleanup(func() { fileObserver.Unsubscribe(fileevents.TopicExecuteFileEditOperation) })

	operation := runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileEditParameters("target.txt"),
	}
	execution := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: runtimecontract.StableRuntimeOperationHash(operation),
		ExecutionKey: "exec/ws-v2/run-v2/dispatch-1/write-1", Fence: 1,
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}
	execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(execution)
	response, err := (eventRuntimeOperationExecutor{hub: hub}).ExecuteRuntimeOperation(ctx, runtime.RuntimeOperationExecutionRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: "dispatch-1",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-1"},
		Execution:   execution, Operation: operation,
		EvidenceKind:     runtimecontract.EvidenceKindStateChange,
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{ID: "policy-1", AppliesTo: runtimecontract.StepKindFileEdit}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sealedOperation, sealedErr := captured.SealedOperation.ValidateForExecution(captured.RuntimeOperationExecutionContext, runtimecontract.StepKindFileEdit)
	if sealedErr != nil || response.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || captured.WorkspaceDeliverableDirectory != "/tmp/workspace" || captured.OperationID != "write-1" || len(sealedOperation.TargetRefs) != 1 || sealedOperation.TargetRefs[0] != "target.txt" ||
		len(captured.EvidencePolicies) != 1 || captured.EvidencePolicies[0].ID != "policy-1" {
		t.Fatalf("response=%+v command=%+v sealedErr=%v", response, captured, sealedErr)
	}
}

func TestRuntimeOperationExecutorPassesFocusTargetRoleToMCPOwner(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: "/tmp/workspace", InternalDirectory: "/tmp/workspace-internal",
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	var captured mcpevents.ExecuteOperationCommand
	mcpObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleMCP, hub)
	mcpObserver.Subscribe(mcpevents.TopicExecuteOperation, func(event magicevent.Event, result magicevent.Result) {
		captured = event.Data().(mcpevents.ExecuteOperationCommand)
		result.Set(mcpevents.ExecuteOperationResult{}, nil)
	})
	t.Cleanup(func() { mcpObserver.Unsubscribe(mcpevents.TopicExecuteOperation) })

	serverID, tool := "server-a", "lookup.references"
	capabilityID := common.MCPCapabilityID("server-a:tool:lookup.references")
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "mcp-catalog", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, ObservedRevision: "mcp-registry-v1",
		MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{{
			ID: capabilityID, ServerID: common.MCPServerID(serverID), Kind: "tool", OperationKind: runtimecontract.StepKindMCPToolCall, Name: tool, Enabled: true, Status: "enabled",
			EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
		}},
		OperationPolicies: []runtimecontract.RuntimeOperationPolicyFact{{
			ID: "mcp-policy", OwnerModuleID: runtimecontract.RuntimeModuleMCP, OperationKind: runtimecontract.StepKindMCPToolCall, SubjectRef: string(capabilityID),
			ParameterMatches: []runtimecontract.RuntimeOperationParameterMatch{{Name: "serverId", String: &serverID}, {Name: "tool", String: &tool}},
			EffectScope:      runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		}},
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	operation := runtimecontract.RuntimeOperation{
		ID: "mcp-1", Kind: runtimecontract.StepKindMCPToolCall, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		Parameters: []runtimecontract.OperationParameter{{Name: "serverId", String: &serverID}, {Name: "tool", String: &tool}},
	}
	execution := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: runtimecontract.StableRuntimeOperationHash(operation),
		ExecutionKey: "exec/ws-v2/run-v2/dispatch-1/mcp-1", Fence: 1,
		ExpectedEffectScope: runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
	}
	execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(execution)
	response, err := (eventRuntimeOperationExecutor{hub: hub}).ExecuteRuntimeOperation(ctx, runtime.RuntimeOperationExecutionRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", DispatchID: "dispatch-1",
		Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-1"}, TargetRole: "implementation",
		Execution: execution, Operation: operation, EvidenceKind: runtimecontract.EvidenceKindValidation, OwnerCatalog: &catalog,
	})
	sealedOperation, sealedAuthorization, sealedErr := captured.SealedOperation.ValidateForExecution(captured.RuntimeOperationExecutionContext)
	sealedData, dataErr := mcpevents.OperationDataFromRuntimeOperation(sealedOperation)
	if err != nil || response.Status != runtimecontract.RuntimeOperationExecutionStatusAccepted || captured.TargetRole != "implementation" ||
		sealedErr != nil || dataErr != nil || sealedData.ServerID != "server-a" || sealedData.Tool != "lookup.references" ||
		sealedAuthorization.Capability.ID != capabilityID {
		t.Fatalf("response=%+v command=%+v err=%v sealedErr=%v dataErr=%v", response, captured, err, sealedErr, dataErr)
	}
}

func TestRuntimeMainlineRetriesSameCandidateContractOnce(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	startRuntimeCandidateFileEditAdmissionOwners(t, hub)
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	var attempts []llmevents.RequestRuntimeCandidateCommand
	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		attempts = append(attempts, command)
		if command.CandidateAttempt == 0 {
			result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{
				Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
				ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
				DocumentOutcome:      runtimeCompleteDocumentOutcome(command.ExpectedDocument, []byte(`{"unexpected":true}`)),
			}), nil)
			return
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "write-1", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			TargetRefs: []string{"target.txt"}, Parameters: runtimeFileEditParameters("target.txt"),
		}}}}}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	biz := newRuntimeTestOwner()
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, biz, runtimeRepository, runtimeCandidateSealFileEditFacts(t), runtimeDefinition(), nil)
	module := newTestBiz(biz, hub, nil)
	sealed, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, initialized, "stable-prefix-v2")
	if err != nil {
		t.Fatal(err)
	}
	if sealed.ActiveDispatch == nil || len(attempts) != 2 || attempts[0].CandidateAttempt != 0 || attempts[1].CandidateAttempt != 1 || attempts[0].CandidateCorrectionObservation != nil || attempts[0].CandidateCorrection != nil || attempts[1].CandidateCorrectionObservation == nil || attempts[1].CandidateCorrection == nil || attempts[1].CandidateCorrection.Witness.Path != "$.unexpected" || attempts[1].CandidateCorrection.Witness.AlternativeValue != "field_absent" || attempts[1].CandidateCorrectionObservation.Diagnostic == nil || attempts[1].CandidateCorrectionObservation.Diagnostic.Path != "$.unexpected" || attempts[1].CandidateCorrectionObservation.Diagnostic.ManifestHash != attempts[1].CandidateManifest.Hash ||
		attempts[0].Transaction != attempts[1].Transaction || attempts[0].StablePrefixHash != attempts[1].StablePrefixHash || attempts[0].RequestIdentity.LogicalRequestID == attempts[1].RequestIdentity.LogicalRequestID {
		t.Fatalf("attempts=%+v sealed=%+v", attempts, sealed)
	}
	if len(sealed.CandidateAttempts) != 2 || sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateFailed || sealed.CandidateAttempts[0].Failure == nil || sealed.CandidateAttempts[0].Failure.Diagnostic == nil || sealed.CandidateAttempts[0].Correction == nil || !reflect.DeepEqual(runtimeCandidateCorrectionInputFromDirective(sealed.CandidateAttempts[0].Correction), attempts[1].CandidateCorrection) || sealed.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
		t.Fatalf("persisted attempts = %+v", sealed.CandidateAttempts)
	}
	wantCorrectionHash := attempts[1].CandidateCorrection.StableRequestCorrectionHash(attempts[1].CandidateCorrectionObservation)
	correctionSessionFound := false
	for _, session := range sealed.CandidateRequestSessions {
		if session.LogicalRequestID == attempts[1].RequestIdentity.LogicalRequestID {
			correctionSessionFound = session.CorrectionHash == wantCorrectionHash
		}
	}
	if !correctionSessionFound {
		t.Fatalf("attempt 1 request session did not bind detached correction input hash %q: %+v", wantCorrectionHash, sealed.CandidateRequestSessions)
	}
	duplicate, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, sealed, "stable-prefix-v2")
	if err != nil || duplicate.ActiveDispatch == nil || len(attempts) != 2 || len(duplicate.CandidateAttempts) != 2 {
		t.Fatalf("duplicate correction notification replayed provider: aggregate=%+v attempts=%+v err=%v", duplicate, attempts, err)
	}
}

func TestRuntimeMainlineCandidateAuthorityExhaustionHasNoCorrectionProviderOrBudgetEffect(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	biz := newRuntimeTestOwner()
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	facts := runtimeAggregate(t, 1).Facts
	selectorRef := facts.Focus.TargetSelectorRef
	if selectorRef == "" {
		selectorRef = "test/fixture-owner-selector"
	}
	ownerCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "test-candidate-authority-file-edit", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, SelectorRefs: []string{selectorRef},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.StepKindFileEdit, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
			SelectorRefs: []string{selectorRef},
		}},
		ObservedRevision: "test-candidate-authority-file-edit-v1",
	}
	ownerCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(ownerCatalog)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{ownerCatalog}
	runtimeTestInitializeOperationFixture(t, ctx, biz, runtimeRepository, facts, runtimeDefinition(), nil)
	rawCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
		TargetRefs: []string{"target.txt"}, Parameters: runtimeFileEditParameters("target.txt"),
	}}}}}
	seeded := executeRejectedRuntimeFileEditForRepositoryTest(t, ctx, biz.runtimeController, "candidate-authority-seed-prefix", rawCandidate, "candidate-authority-rejected-shape", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassCandidateContractFailure, Code: "single-shape-rejected",
		Signature: "single-shape-rejected", Message: "the only candidate operation shape was rejected by File",
	})

	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		result.Set(runtimeCandidateResultForTest(t, command, rawCandidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	beforeCorrectionBudget, err := runtimecontract.RuntimeBudgetRemaining(seeded.ExecutionBudget, runtimecontract.RuntimeBudgetDimensionCandidateCorrection)
	if err != nil {
		t.Fatal(err)
	}
	module := newTestBiz(biz, hub, nil)
	terminal, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, seeded, "candidate-authority-prefix")
	if err != nil {
		t.Fatal(err)
	}
	afterCorrectionBudget, err := runtimecontract.RuntimeBudgetRemaining(terminal.ExecutionBudget, runtimecontract.RuntimeBudgetDimensionCandidateCorrection)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || beforeCorrectionBudget != afterCorrectionBudget || terminal.ActiveDispatch != nil || len(terminal.OperationReceipts) != 1 || len(terminal.CandidateRequestSessions) != 2 ||
		terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil || terminal.Decision.Terminal.Code != "candidate_authority_exhausted" ||
		len(terminal.CandidateAttempts) != 2 || terminal.CandidateAttempts[1].Correction == nil || terminal.CandidateAttempts[1].Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionRejectedFinal ||
		terminal.CandidateAttempts[1].Correction.TerminalCode != "candidate_authority_exhausted" {
		t.Fatalf("candidate authority exhaustion performed a correction effect: calls=%d budget=%d->%d aggregate=%+v", providerCalls, beforeCorrectionBudget, afterCorrectionBudget, terminal)
	}

	reloaded, err := biz.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	revision := reloaded.StateRevision
	replayed, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, reloaded, "candidate-authority-prefix")
	if err != nil || providerCalls != 1 || replayed.StateRevision != revision || len(replayed.CandidateAttempts) != 2 {
		t.Fatalf("restart/duplicate replayed exhausted candidate authority: calls=%d aggregate=%+v err=%v", providerCalls, replayed, err)
	}
}

func TestRuntimeMainlineRenderedPreflightStopsBeforeCandidateAttempt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	preflightCalls, candidateCalls := 0, 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		preflightCalls++
		command := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
		result.Set(llmevents.PreflightRuntimeCandidateResult{
			Transaction:  command.Transaction,
			DocumentKind: command.DocumentKind,
			Feedback:     runtimePreflightFeedbackForTest(t, command, "P4.current_snapshots is redacted"),
		}, nil)
	})
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		candidateCalls++
	})
	t.Cleanup(func() {
		llmObserver.Unsubscribe(llmevents.TopicPreflightRuntimeCandidate)
		llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate)
	})

	facts := runtimeCandidateSealFileEditFacts(t)
	facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:                runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 96 * 1024,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
	}
	biz := newRuntimeTestOwner()
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, biz, runtimeRepository, facts, runtimeDefinition(), nil)
	terminal, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeCandidate(ctx, initialized, "context-preflight-prefix", 0)
	if err != nil {
		t.Fatal(err)
	}
	if preflightCalls != 1 || candidateCalls != 0 || len(terminal.CandidateAttempts) != 0 || terminal.Decision.Terminal == nil || terminal.Facts.Failure == nil || terminal.Facts.Failure.Code != "runtime_context_integrity" {
		t.Fatalf("rendered preflight consumed a candidate attempt or called provider: preflight=%d candidate=%d aggregate=%+v", preflightCalls, candidateCalls, terminal)
	}
	if terminal.Facts.Repair.RepairAttempts != 0 || terminal.Facts.Recovery.ProviderTransportAttempts != 0 {
		t.Fatalf("rendered preflight consumed a repair/provider budget: %+v", terminal.Facts)
	}
}

func TestDriveRuntimeDecisionReacquiresMissingSealedProviderCeilingBeforeLLM(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	preflightCalls, candidateCalls := 0, 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicPreflightRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		preflightCalls++
	})
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		candidateCalls++
	})
	t.Cleanup(func() {
		llmObserver.Unsubscribe(llmevents.TopicPreflightRuntimeCandidate)
		llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate)
	})

	facts := runtimeAggregate(t, 1).Facts
	facts.Resources.ProviderCeiling = nil
	requirement := runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		MinimumBytes: 96 * 1024, Reason: "provider ceiling required",
	}

	biz := newRuntimeTestOwner()
	if err := biz.ConfigureRuntimeRepository(runtimeStateRepository{hub: hub}); err != nil {
		t.Fatal(err)
	}
	initialized, err := biz.InitializeRuntime(ctx, facts, runtimeDefinition(), []runtimecontract.RuntimeRequirement{requirement})
	if err != nil {
		t.Fatal(err)
	}
	if initialized.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(initialized.Facts.Prerequisites.Pending) != 1 {
		t.Fatalf("fixture did not start at provider-ceiling prerequisite acquisition: %+v", initialized)
	}
	providerPrerequisite := initialized.Facts.Prerequisites.Pending[0]
	ceiling := runtimecontract.ProviderCeiling{
		Selection:                runtimecontract.ProviderInferenceSelection{Target: facts.Activation.ProviderSelection.Target},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 96 * 1024,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
	}
	committed, err := biz.CommitRuntimePrerequisite(ctx, facts.WorkspaceID, facts.RunID, runtime.PrerequisiteResult{
		PrerequisiteID: providerPrerequisite.ID, ProviderCeiling: &ceiling,
	})
	if err != nil || committed.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("commit provider ceiling prerequisite: aggregate=%+v err=%v", committed, err)
	}
	// Persist the owner-resource-loss boundary through the same Run CAS adapter
	// used by production. The stale decision remains candidate-shaped only until
	// driveRuntimeDecision reconciles it back to the sealed prerequisite.
	broken := committed
	broken.Facts.Resources.ProviderCeiling = nil
	broken.StateRevision++
	if err := (runtimeStateRepository{hub: hub}).Save(ctx, broken, committed.StateRevision); err != nil {
		t.Fatalf("persist missing provider ceiling aggregate: %v", err)
	}

	reconciled, err := newTestBiz(biz, hub, nil).driveRuntimeDecision(ctx, broken, "provider-ceiling-reacquire-prefix")
	if err != nil {
		t.Fatalf("driveRuntimeDecision: %v", err)
	}
	if reconciled.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || preflightCalls != 0 || candidateCalls != 0 ||
		len(reconciled.CandidateAttempts) != 0 || reconciled.Facts.Repair.RepairAttempts != 0 ||
		reconciled.Facts.Recovery.ProviderTransportAttempts != 0 || reconciled.Facts.Recovery.PrerequisiteRefreshes != 0 {
		t.Fatalf("missing provider ceiling reached LLM or spent a budget: preflight=%d candidate=%d aggregate=%+v", preflightCalls, candidateCalls, reconciled)
	}
	if len(reconciled.Facts.Prerequisites.Pending) != 1 || reconciled.Facts.Prerequisites.Pending[0].ID != providerPrerequisite.ID ||
		len(reconciled.Facts.Prerequisites.Satisfied) != 0 || reconciled.Facts.Resources.ProviderCeiling != nil {
		t.Fatalf("missing provider ceiling was not returned to active prerequisite acquisition: %+v", reconciled.Facts)
	}
}

func TestRuntimeMainlineRecordsInfeasibleCandidateBeforeDispatchSeal(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(fileevents.ProbeRuntimeOperationFeasibilityCommand)
		failure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "operation-admissibility/file/edit/operation_target_not_regular", Message: "target is a directory", TargetRefs: []string{"."}}
		response, err := fileevents.NewProbeRuntimeOperationFeasibilityResult(command, runtimecontract.RuntimeOwnerFeasibilityCategoryTargetNotRegular, failure, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() {
		probeObserver.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility)
	})

	candidateCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		candidateCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		candidate := runtimecontract.RuntimeCandidate{
			StateChange: &runtimecontract.StateChangeCandidate{
				Operations: []runtimecontract.StateChangingOperation{{
					Operation: runtimecontract.RuntimeOperation{
						ID: "directory-edit", Kind: runtimecontract.StepKindFileEdit,
						CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"."},
						Parameters: runtimeFileEditParameters("."),
					},
				}},
			},
		}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	facts := runtimeAggregate(t, 1).Facts
	// Infeasibility is an owner feasibility observation, not wire exact_target
	// membership. Clear focus TargetRefs so Decision does not seal an exclusive
	// path that rejects the deliberately non-regular inventory target ".".
	facts.Focus.TargetRefs = nil
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"."},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.StepKindFileEdit,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
				runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			},
		}},
		ObservedRevision: "test-revision",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{
		runtimeOperationAdmissibilityPrerequisite(inventory),
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{
		runtimeFileAdmissibilityObservation(inventory),
	}
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{TargetRef: ".", ContentHash: "directory-target-v1", ContentState: runtimecontract.SnapshotContentStateComplete}}
	biz := newRuntimeTestOwner()
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, biz, runtimeRepository, facts, runtimeDefinition(), nil)
	module := newTestBiz(biz, hub, nil)
	failed, err := module.requestAndSealRuntimeCandidate(ctx, initialized, "stable-admissibility-prefix", 0)
	if err != nil {
		t.Fatal(err)
	}
	if failed.ActiveDispatch != nil || len(failed.CandidateAttempts) != 1 || failed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated || failed.CandidateAttempts[0].Failure != nil || failed.CandidateAttempts[0].OwnerFeasibility == nil || failed.CandidateAttempts[0].OwnerFeasibility.Outcome != runtimecontract.RuntimeOwnerFeasibilityInfeasible || failed.CandidateAttempts[0].OwnerFeasibility.Feedback == nil || failed.CandidateAttempts[0].OwnerFeasibility.Feedback.OriginalDiagnostic == nil || failed.CandidateAttempts[0].OwnerFeasibility.Feedback.OriginalDiagnostic.OriginalCode != "operation_target_not_regular" || failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible {
		t.Fatalf("owner infeasibility was not persisted as a non-correction SM-2 fact: %+v", failed)
	}
	if len(failed.OperationReceipts) != 0 || len(failed.Facts.Evidence.Accepted) != 0 || len(failed.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("infeasible candidate created execution state: %+v", failed)
	}
	if candidateCalls != 1 {
		t.Fatalf("owner infeasibility requested a correction candidate: calls=%d aggregate=%+v", candidateCalls, failed)
	}
}

func TestRuntimeCandidateAdmissionSkipsOwnerProbeForAssessment(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(1)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := newTestBiz(newRuntimeTestOwner(), hub, nil)
	candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: "focus-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "assessment-only"}}
	failure, stale, prerequisiteRequest, err := module.probeRuntimeCandidateFeasibility(ctx, runtime.RuntimeAggregate{}, t.TempDir(), candidate, runtimecontract.RuntimeCandidateContractManifest{}, 0)
	if err != nil || failure != nil || stale != nil || prerequisiteRequest != nil {
		t.Fatalf("assessment candidate should not enter owner operation probe: failure=%+v stale=%+v prerequisite=%+v err=%v", failure, stale, prerequisiteRequest, err)
	}
}

func TestRuntimeFileBackedOwnerInfeasibilityDoesNotRequestCorrection(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runsPath := filepath.Join(t.TempDir(), "runs.json")
	runs := filerepository.NewRunRepository(runsPath)
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	probes := 0
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		probes++
		command := event.Data().(fileevents.ProbeRuntimeOperationFeasibilityCommand)
		failure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "operation-admissibility/file/edit/operation_target_not_regular", Message: "target is a directory", TargetRefs: []string{"."}}
		response, err := fileevents.NewProbeRuntimeOperationFeasibilityResult(command, runtimecontract.RuntimeOwnerFeasibilityCategoryTargetNotRegular, failure, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() {
		probeObserver.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility)
	})

	var requests []llmevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		requests = append(requests, command)
		if command.CandidateAttempt != 0 {
			t.Fatalf("owner infeasibility must not request correction: %+v", command)
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "edit-.", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"."}, Parameters: runtimeFileEditParameters("."),
		}}}}}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	facts := runtimeAggregate(t, 1).Facts
	facts.Focus.TargetRefs = nil
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{".", "target.txt"},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.StepKindFileEdit,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
				runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			},
		}},
		ObservedRevision: "test-revision",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{
		runtimeOperationAdmissibilityPrerequisite(inventory),
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{
		runtimeFileAdmissibilityObservation(inventory),
	}
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{
		{TargetRef: ".", ContentHash: "directory-target-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
		{TargetRef: "target.txt", ContentHash: "target-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
	}
	biz := newRuntimeTestOwner()
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, biz, runtimeRepository, facts, runtimeDefinition(), nil)
	first, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeCandidate(ctx, initialized, "stable-admissibility-prefix", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || len(first.CandidateAttempts) != 1 || first.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated || first.CandidateAttempts[0].OwnerFeasibility == nil || first.CandidateAttempts[0].Failure != nil || first.Facts.Failure == nil || first.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible {
		t.Fatalf("owner infeasibility was not durably handed to SM-2: requests=%+v aggregate=%+v", requests, first)
	}
}

func TestRuntimeFileBackedOwnerInfeasibilityPersistsWithoutCorrection(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	root := t.TempDir()
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })

	probes := 0
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		probes++
		command := event.Data().(fileevents.ProbeRuntimeOperationFeasibilityCommand)
		var failure *runtimecontract.FailureFact
		category := runtimecontract.RuntimeOwnerFeasibilityCategory("")
		if probes == 1 {
			category = runtimecontract.RuntimeOwnerFeasibilityCategoryTargetNotRegular
			failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "operation-admissibility/file/edit/operation_target_not_regular", Message: "target is a directory", TargetRefs: []string{"."}}
		}
		response, err := fileevents.NewProbeRuntimeOperationFeasibilityResult(command, category, failure, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() {
		probeObserver.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility)
	})

	facts := runtimeAggregate(t, 1).Facts
	facts.Focus.TargetRefs = nil
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{".", "target.txt"},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.StepKindFileEdit,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
				runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			},
		}},
		ObservedRevision: "test-revision",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{runtimeOperationAdmissibilityPrerequisite(inventory)}
	// Keep the prerequisite observation in the file-backed aggregate: correction
	// admission must use this same owner inventory after restart.
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{runtimeFileAdmissibilityObservation(inventory)}
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{
		{TargetRef: ".", ContentHash: "directory-target-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
		{TargetRef: "target.txt", ContentHash: "target-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
	}

	contentsRoot := filepath.Join(root, "candidate-contents")
	firstOwner := newRuntimeTestOwnerWithEvidence(RuntimeEvidenceRepositories{
		ArtifactContinuations: filerepository.NewArtifactContinuationRepository(contentsRoot),
	})
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := firstOwner.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, firstOwner, runtimeRepository, facts, runtimeDefinition(), nil)
	const prefix = "stable-admissibility-prefix"
	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		providerCalls++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		if command.CandidateAttempt != 0 {
			t.Errorf("restart replayed provider for correction attempt: %+v", command)
			return
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "edit-.", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"."}, Parameters: runtimeFileEditParameters("."),
		}}}}}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	failed, err := newTestBiz(firstOwner, hub, nil).requestAndSealRuntimeCandidate(ctx, initialized, prefix, 0)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 1 || len(failed.CandidateAttempts) != 1 || failed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated || failed.CandidateAttempts[0].OwnerFeasibility == nil || failed.CandidateAttempts[0].Failure != nil || failed.Facts.Failure == nil || failed.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible {
		t.Fatalf("owner infeasibility was not persisted: providerCalls=%d aggregate=%+v", providerCalls, failed)
	}
}

func TestRuntimeMainlineExhaustsTwoInfeasibleCandidatesWithoutDispatch(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(fileevents.ProbeRuntimeOperationFeasibilityCommand)
		failure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_target_not_regular", Signature: "operation-admissibility/file/edit/operation_target_not_regular", Message: "target is a directory", TargetRefs: []string{"."}}
		response, err := fileevents.NewProbeRuntimeOperationFeasibilityResult(command, runtimecontract.RuntimeOwnerFeasibilityCategoryTargetNotRegular, failure, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() {
		probeObserver.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility)
	})

	requests := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requests++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		target := "target.txt"
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
			ID: "directory-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{target}, Parameters: runtimeFileEditParameters(target),
		}}}}}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	facts := runtimeAggregate(t, 1).Facts
	// This fixture deliberately leaves the manifest target set unrestrictive
	// while keeping two owner-admissible targets. Attempt 0 rejects target.txt,
	// so correction has one real structural alternative (replacement.txt).
	// Attempt 1 deliberately reuses the rejected shape to prove bounded
	// exhaustion rather than the B2b no-objective terminal fence.
	facts.Focus.TargetRefs = nil
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt", "replacement.txt"},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.StepKindFileEdit,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
				runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			},
		}},
		ObservedRevision: "test-revision",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{
		runtimeOperationAdmissibilityPrerequisite(inventory),
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{
		runtimeFileAdmissibilityObservation(inventory),
	}
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{
		{TargetRef: "target.txt", ContentHash: "target-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
		{TargetRef: "replacement.txt", ContentHash: "replacement-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
	}
	biz := newRuntimeTestOwner()
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, biz, runtimeRepository, facts, runtimeDefinition(), nil)
	terminal, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeCandidate(ctx, initialized, "stable-admissibility-prefix", 0)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || terminal.ActiveDispatch != nil || len(terminal.CandidateAttempts) != 1 || terminal.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated || terminal.CandidateAttempts[0].OwnerFeasibility == nil || terminal.Facts.Failure == nil || terminal.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible {
		t.Fatalf("owner infeasibility incorrectly entered candidate exhaustion/correction: requests=%d aggregate=%+v", requests, terminal)
	}
	if len(terminal.OperationReceipts) != 0 || len(terminal.Facts.Evidence.Accepted) != 0 || len(terminal.Facts.Evidence.Rejected) != 0 {
		t.Fatalf("infeasible candidates created execution state: %+v", terminal)
	}
}

func TestRuntimeMainlineRestartsFromPersistedCandidateProposalWithoutProviderReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	providerCalls := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) {
		providerCalls++
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	biz := newRuntimeTestOwner()
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	operationFacts := runtimeCandidateSealExactFileEditFacts(t, "target.txt")
	operationFacts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "target.txt", ContentHash: "persisted-proposal-target-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	runtimeTestInitializeOperationFixture(t, ctx, biz, runtimeRepository, operationFacts, runtimeDefinition(), nil)
	const prefix = "persisted-proposal-prefix"
	pending, err := biz.BeginRuntimeCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix)
	if err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "persisted-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileEditParameters("target.txt"),
	}}}}}
	persisted, err := biz.RecordRuntimeCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.StateRevision <= pending.StateRevision || len(persisted.CandidateAttempts) != 1 || persisted.CandidateAttempts[0].Proposal == nil {
		t.Fatalf("candidate proposal was not persisted: pending=%+v persisted=%+v", pending, persisted)
	}
	restarted, err := biz.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	startRuntimeCandidateFileEditAdmissionOwners(t, hub)
	module := newTestBiz(biz, hub, nil)
	sealed, err := module.requestAndSealRuntimeCandidate(ctx, restarted, prefix, 0)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || sealed.ActiveDispatch == nil || sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || sealed.CandidateAttempts[0].Proposal != nil {
		t.Fatalf("restart replayed provider or did not seal persisted proposal: calls=%d aggregate=%+v", providerCalls, sealed)
	}
}

func TestRuntimeMainlinePartialReplayClosesAttemptThenSealsCorrectedRetry(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	bootstrap := runtimeBootstrap()
	bootstrap.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{
		ID: "test-partial-replay-concrete-alternatives",
		EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
			ID: "cwi-v2", Scope: "test", TargetRole: "deliverable",
			TargetRefs:            []string{"replacement.txt", "target.txt"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			AcceptedStepKinds:     []string{runtimecontract.StepKindFileEdit},
		}},
	}}
	bootstrap = runtimeTestBootstrapWithReboundBindings(bootstrap)
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTestBootstrap(common.RunStateActive, bootstrap)); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir(),
		}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(fileevents.ProbeRuntimeOperationFeasibilityCommand)
		response, err := fileevents.NewProbeRuntimeOperationFeasibilityResult(command, "", nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		result.Set(response, nil)
	})
	t.Cleanup(func() { probeObserver.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility) })

	biz := newRuntimeTestOwner()
	repository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(repository); err != nil {
		t.Fatal(err)
	}
	facts, err := bootstrap.InitialFacts("ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	fileCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "test-semantic-" + runtimecontract.RuntimeModuleFile,
		OwnerModuleID:  runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind:                  runtimecontract.StepKindFileEdit,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		}},
		TargetRefs:       []string{"replacement.txt", "target.txt"},
		ObservedRevision: "test-semantic-revision-" + runtimecontract.RuntimeModuleFile,
	}
	fileCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileEdit}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{fileCatalog}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{runtimeOperationAdmissibilityPrerequisite(fileCatalog)}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{runtimeFileAdmissibilityObservation(fileCatalog)}
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{
		{TargetRef: "target.txt", ContentHash: "partial-replay-target-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
		{TargetRef: "replacement.txt", ContentHash: "partial-replay-replacement-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, biz, repository, facts, bootstrap.Definition, bootstrap.Requirements)
	if len(initialized.Facts.Focus.CoverageEvidenceBindings) != 1 {
		t.Fatalf("partial replay fixture requires one sealed coverage binding: %+v", initialized.Facts.Focus)
	}
	replayedOperation := runtimecontract.RuntimeOperation{
		ID: "replayed-edit", Kind: runtimecontract.StepKindFileEdit,
		BindingRefs: []string{initialized.Facts.Focus.CoverageEvidenceBindings[0].ID}, TargetRefs: []string{"target.txt"},
		Parameters: runtimeFileEditParameters("target.txt"),
	}
	rejectedCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: replayedOperation}}}}
	seeded := executeRejectedRuntimeFileEditForRepositoryTest(t, ctx, biz.runtimeController, "partial-replay-seed-prefix", rejectedCandidate, "rejected-edit", runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassCandidateContractFailure, Code: "base_mismatch",
		Signature: "rejected-edit", Message: "the File base content changed before edit",
	})
	if len(seeded.CompletedDispatches) != 1 || seeded.CompletedDispatches[0].Candidate.StateChange == nil ||
		len(seeded.CompletedDispatches[0].Candidate.StateChange.Operations) != 1 {
		t.Fatalf("partial replay seed lost committed File operation: %+v", seeded.CompletedDispatches)
	}
	replayedOperation = seeded.CompletedDispatches[0].Candidate.StateChange.Operations[0].Operation

	var requests []llmevents.RequestRuntimeCandidateCommand
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		requests = append(requests, command)
		candidate := runtimecontract.RuntimeCandidate{}
		newOperation := replayedOperation
		newOperation.ID = "new-edit"
		newText := "corrected"
		newOperation.Parameters = runtimeFileEditParameters("target.txt")
		newOperation.Parameters[1].String = &newText
		if command.CandidateAttempt == 0 {
			candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: replayedOperation}, {Operation: newOperation}}}
		} else {
			if command.CandidateCorrectionObservation == nil || command.CandidateCorrectionObservation.Code != "candidate_partially_replays_rejected_shape" {
				t.Errorf("retry missing typed partial replay observation: %+v", command.CandidateCorrectionObservation)
			}
			corrected := replayedOperation
			corrected.ID = "corrected-edit"
			corrected.TargetRefs = []string{"replacement.txt"}
			corrected.Parameters = runtimeFileEditParameters("replacement.txt")
			candidate.StateChange = &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: corrected}, {Operation: newOperation}}}
		}
		result.Set(runtimeCandidateResultForTest(t, command, candidate), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })

	module := newTestBiz(biz, hub, nil)
	first, err := module.requestAndSealRuntimeCandidate(ctx, seeded, "partial-replay-prefix", 0)
	if err != nil || len(requests) != 1 || len(first.CandidateAttempts) != 2 || first.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || first.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateFailed || first.CandidateAttempts[1].Failure == nil || first.CandidateAttempts[1].Failure.Code != "candidate_partially_replays_rejected_shape" || first.Decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("partial replay did not atomically close attempt 0 before restart: aggregate=%+v err=%v requests=%+v", first, err, requests)
	}
	correction := first.CandidateAttempts[1].Correction
	if correction == nil || correction.Witness == nil ||
		correction.Witness.Dimension != runtimecontract.RuntimeCandidateCorrectionWitnessTarget ||
		correction.Witness.RejectedValue != "target.txt" || correction.Witness.AlternativeValue != "replacement.txt" ||
		correction.Witness.RejectedShapeHash == correction.Witness.AlternativeShapeHash {
		t.Fatalf("partial replay did not seal the manifest's concrete target alternative: %+v", correction)
	}
	restarted, err := biz.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := module.requestAndSealRuntimeCandidateWithRetry(ctx, restarted, "partial-replay-prefix")
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || sealed.ActiveDispatch == nil || len(sealed.CandidateAttempts) != 3 ||
		sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted ||
		sealed.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStateFailed || sealed.CandidateAttempts[1].Failure == nil ||
		sealed.CandidateAttempts[1].Failure.Code != "candidate_partially_replays_rejected_shape" ||
		sealed.CandidateAttempts[2].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || sealed.Decision.State == runtimecontract.ConvergenceStateFailed {
		var retry runtimecontract.RuntimeCandidateAttemptRecord
		if len(sealed.CandidateAttempts) > 2 {
			retry = sealed.CandidateAttempts[2]
		}
		var sealedCorrection *runtimecontract.RuntimeCandidateCorrectionDirective
		if len(sealed.CandidateAttempts) > 1 {
			sealedCorrection = sealed.CandidateAttempts[1].Correction
		}
		t.Fatalf("partial replay did not converge through typed retry: request-count=%d sealed-correction=%+v retry-state=%q failure=%+v correction=%+v proposal=%+v decision=%q", len(requests), sealedCorrection, retry.State, retry.Failure, retry.Correction, retry.Proposal, sealed.Decision.State)
	}
	if requests[0].StablePrefixHash != requests[1].StablePrefixHash || requests[0].Transaction != requests[1].Transaction {
		t.Fatalf("partial replay retry drifted the sealed request contract: %+v", requests)
	}
}

func TestRuntimeMainlineRestartSkipsPersistedFeasibilityProbe(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })

	workspaceObserver := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{ID: "ws-v2", DeliverableDirectory: t.TempDir(), InternalDirectory: t.TempDir()}}, nil)
	})
	t.Cleanup(func() { workspaceObserver.Unsubscribe(workspaceevents.TopicWorkspaceCommandGet) })
	providerCalls, probeCalls := 0, 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(_ magicevent.Event, _ magicevent.Result) { providerCalls++ })
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	probeObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleFile, hub)
	probeObserver.Subscribe(fileevents.TopicProbeRuntimeOperationFeasibility, func(_ magicevent.Event, _ magicevent.Result) { probeCalls++ })
	t.Cleanup(func() {
		probeObserver.Unsubscribe(fileevents.TopicProbeRuntimeOperationFeasibility)
	})

	facts := runtimeAggregate(t, 1).Facts
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:blocks/file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"target.txt"},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.StepKindFileEdit,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
				runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			},
		}},
		ObservedRevision: "test-revision",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{runtimeOperationAdmissibilityPrerequisite(inventory)}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{runtimeFileAdmissibilityObservation(inventory)}
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "target.txt", ContentHash: "persisted-feasibility-target-v1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	biz := newRuntimeTestOwner()
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	runtimeTestInitializeOperationFixture(t, ctx, biz, runtimeRepository, facts, runtimeDefinition(), nil)
	const prefix = "persisted-feasibility-prefix"
	if _, err := biz.BeginRuntimeCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, prefix); err != nil {
		t.Fatal(err)
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "persisted-feasible-edit", Kind: runtimecontract.StepKindFileEdit, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"}, Parameters: runtimeFileEditParameters("target.txt"),
	}}}}}
	if _, err := biz.RecordRuntimeCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := runtimeCandidateAttemptTestBinding(ctx, biz, "ws-v2", "run-v2", 0)
	if err != nil {
		t.Fatal(err)
	}
	canonicalCandidate := bindRuntimeTestCandidateToManifest(manifest, candidate)
	canonicalCandidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, canonicalCandidate)
	if err != nil || canonicalCandidate.StateChange == nil || len(canonicalCandidate.StateChange.Operations) != 1 {
		t.Fatalf("derive persisted feasibility candidate=%+v err=%v", canonicalCandidate, err)
	}
	operationID := canonicalCandidate.StateChange.Operations[0].Operation.ID
	if _, err := biz.RecordRuntimeCandidateFeasibility(ctx, "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFeasibilityRecord{OperationID: operationID, AdmissibilitySnapshotID: inventory.SnapshotID, Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible}}); err != nil {
		t.Fatal(err)
	}
	restarted, err := biz.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeCandidate(ctx, restarted, prefix, 0)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls != 0 || probeCalls != 0 || sealed.ActiveDispatch == nil || sealed.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted || len(sealed.CandidateAttempts[0].Feasibilities) != 0 {
		t.Fatalf("restart replayed provider/probe or retained unsealed feasibility state: provider=%d probes=%d aggregate=%+v", providerCalls, probeCalls, sealed)
	}
}

func runtimeFileEditParameters(_ string) []runtimecontract.OperationParameter {
	oldText := "old"
	newText := "new"
	return []runtimecontract.OperationParameter{
		{Name: "oldText", String: &oldText},
		{Name: "newText", String: &newText},
	}
}

func TestRuntimeMainlinePreservesProviderProtocolDiagnosticWithoutCandidateAttempt(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, runtimeRunForTest(common.RunStateActive, runtimeDefinition())); err != nil {
		t.Fatal(err)
	}
	runOwner := runmodule.New()
	if err := runOwner.Configure(runOwnerTestDependencies(runs, nil, nil, hub)); err != nil {
		t.Fatal(err)
	}
	if err := runOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runOwner.Stop(ctx) })
	requests := 0
	llmObserver := magicevent.NewSimpleObserver(runtimecontract.RuntimeModuleLLM, hub)
	llmObserver.Subscribe(llmevents.TopicRequestRuntimeCandidate, func(event magicevent.Event, result magicevent.Result) {
		requests++
		command := event.Data().(llmevents.RequestRuntimeCandidateCommand)
		failure := runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassProviderProtocol, Code: "response_byte_limit_exceeded",
			Signature: "response-byte-limit-exceeded", Reason: "provider response violated the protocol contract",
		}
		result.Set(runtimeProviderResultWithAuditForTest(command, llmevents.RequestRuntimeCandidateResult{Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity, ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument), Feedback: runtimeProviderFeedbackForTest(t, command, failure)}), nil)
	})
	t.Cleanup(func() { llmObserver.Unsubscribe(llmevents.TopicRequestRuntimeCandidate) })
	biz := newRuntimeTestOwner()
	runtimeRepository := runtimeStateRepository{hub: hub}
	if err := biz.ConfigureRuntimeRepository(runtimeRepository); err != nil {
		t.Fatal(err)
	}
	initialized := runtimeTestInitializeOperationFixture(t, ctx, biz, runtimeRepository, runtimeCandidateSealFileEditFacts(t), runtimeDefinition(), nil)
	terminal, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, initialized, "stable-prefix-v2")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil ||
		terminal.Decision.Terminal.Code != "provider_protocol_failure" || terminal.ActiveDispatch != nil ||
		len(terminal.CandidateAttempts) != 0 || terminal.Facts.Repair.RepairAttempts != 0 ||
		terminal.Facts.Recovery.ProviderTransportAttempts != 0 || len(terminal.GovernanceRequests) != 0 {
		t.Fatalf("generic provider protocol failure must remain request-session-only: requests=%d terminal=%+v", requests, terminal)
	}
	if len(terminal.CandidateRequestSessions) != 1 {
		t.Fatalf("candidate request sessions = %+v", terminal.CandidateRequestSessions)
	}
	session := terminal.CandidateRequestSessions[0]
	if session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal ||
		session.LastOutcome.Class != runtimecontract.RuntimeCandidateRequestOutcomeProviderProtocol ||
		session.LastOutcome.Code != "response_byte_limit_exceeded" || session.LastOutcome.Signature != "response-byte-limit-exceeded" || session.ProviderSubmitted {
		t.Fatalf("provider protocol session = %+v", session)
	}
	reloaded, err := biz.loadRuntime(ctx, "ws-v2", "run-v2")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Facts.Failure == nil || reloaded.Facts.Failure.OriginalDiagnostic == nil ||
		reloaded.Facts.Failure.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleLLM ||
		reloaded.Facts.Failure.OriginalDiagnostic.OriginalCode != "response_byte_limit_exceeded" ||
		reloaded.Facts.Failure.OriginalDiagnostic.Signature != "response-byte-limit-exceeded" {
		t.Fatalf("persisted provider protocol diagnostic = %+v", reloaded.Facts.Failure)
	}
	replayed, err := newTestBiz(biz, hub, nil).requestAndSealRuntimeCandidateWithRetry(ctx, reloaded, "stable-prefix-v2")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || replayed.StateRevision != reloaded.StateRevision || replayed.Facts.Failure == nil ||
		replayed.Facts.Failure.OriginalDiagnostic == nil || replayed.Facts.Failure.OriginalDiagnostic.OriginalCode != "response_byte_limit_exceeded" {
		t.Fatalf("duplicate terminal drive changed provider diagnostic or replayed request: requests=%d aggregate=%+v", requests, replayed)
	}
}

func TestNextRuntimeCandidateAttemptResumesPersistedPendingOrFailedState(t *testing.T) {
	transaction := runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "snapshot-1", FocusID: "focus-v2", State: runtimecontract.ConvergenceStateNeedStateChange, RequiredOutputHash: "output-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "evidence-v1"}
	base := runtime.RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-v2", RunID: "run-v2"}, Decision: runtimecontract.RuntimeDecision{AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "snapshot-1", FocusID: "focus-v2", State: runtimecontract.ConvergenceStateNeedStateChange, RequiredOutputHash: "output-v2", DefinitionRevision: "definition-v2", EvidenceRevision: "evidence-v1"}}}
	contractID := runtimecontract.StableAuthorityContractID(base.Facts.WorkspaceID, base.Facts.RunID, transaction)
	tests := []struct {
		name     string
		attempts []runtimecontract.RuntimeCandidateAttemptRecord
		want     int
		ok       bool
	}{
		{name: "new", want: 0, ok: true},
		{name: "pending zero", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStatePending}}, want: 0, ok: true},
		{name: "failed zero", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateFailed}}, want: 1, ok: true},
		{name: "pending one", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateFailed}, {AuthorityContractID: contractID, Transaction: transaction, Attempt: 1, State: runtimecontract.RuntimeCandidateAttemptStatePending}}, want: 1, ok: true},
		{name: "accepted", attempts: []runtimecontract.RuntimeCandidateAttemptRecord{{AuthorityContractID: contractID, Transaction: transaction, Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateAccepted}}, ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			aggregate := base
			aggregate.CandidateAttempts = tc.attempts
			got, ok := nextRuntimeCandidateAttempt(aggregate)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("next attempt = %d/%t, want %d/%t", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestNextRuntimeCandidateAttemptRejectsResealRunaway(t *testing.T) {
	aggregate := runtimeAggregate(t, 100)
	transaction := aggregate.Decision.AuthorityTransaction()
	contractID := runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction)
	for idx := 0; idx < 3; idx++ {
		resealed := transaction
		resealed.AuthoritySnapshotID = fmt.Sprintf("resealed-snapshot-%d", idx)
		resealed.Attempt = idx
		aggregate.CandidateAttempts = append(aggregate.CandidateAttempts, runtimecontract.RuntimeCandidateAttemptRecord{
			AuthorityContractID: contractID, Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix-v2",
			State:   runtimecontract.RuntimeCandidateAttemptStateFailed,
			Failure: &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: "invalid candidate"},
		})
	}
	if attempt, ok := nextRuntimeCandidateAttempt(aggregate); ok {
		t.Fatalf("reseal runaway offered candidate attempt %d", attempt)
	}
	if !runtimeCandidateContractExhausted(aggregate) {
		t.Fatal("reseal runaway was not recognized as exhausted")
	}
}

func TestRuntimeStateRecordPreservesSemanticPlanningLedgerAcrossOpaqueReload(t *testing.T) {
	aggregate := runtimeAggregate(t, 7)
	record := runtimeStateRecordFromAggregate(aggregate)
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	reloaded := runtimeAggregateFromRecord(record.Clone())
	if err := reloaded.Validate(); err != nil {
		t.Fatal(err)
	}
	if reloaded.SemanticPlanning.Phase != runtimecontract.RuntimeSemanticPlanningRequired ||
		reloaded.SemanticPlanning.DefinitionSeed.Hash != aggregate.SemanticPlanning.DefinitionSeed.Hash {
		t.Fatalf("semantic planning ledger drifted across opaque reload: before=%+v after=%+v", aggregate.SemanticPlanning, reloaded.SemanticPlanning)
	}
	reloaded.SemanticPlanning.DefinitionSeed.CoverageItems[0].SourceID = "mutated"
	if record.SemanticPlanning.DefinitionSeed.CoverageItems[0].SourceID == "mutated" {
		t.Fatal("runtime state record shared semantic planning backing storage")
	}
}

func TestRuntimeStateRecordPreservesCoverageProofLineageAcrossOpaqueReload(t *testing.T) {
	aggregate := runtimeAggregate(t, 7)
	aggregate.Facts.Focus.TargetRefs = []string{"target.txt"}
	aggregate = runtimeTestAggregateWithSemanticPlanning(t, aggregate)
	lineage := runtimecontract.NormalizeRuntimeCoverageProofLineage(runtimeCoverageProofLineageForRepositoryTest(t, aggregate))
	aggregate.CoverageProofLineage = lineage
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("aggregate with proof lineage: %v", err)
	}
	record := runtimeStateRecordFromAggregate(aggregate)
	if err := record.Validate(); err != nil {
		t.Fatalf("record with proof lineage: %v", err)
	}
	reloaded := runtimeAggregateFromRecord(record.Clone())
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("reloaded aggregate with proof lineage: %v", err)
	}
	if !reflect.DeepEqual(reloaded.CoverageProofLineage, lineage) {
		t.Fatalf("proof lineage drifted across opaque reload: before=%+v after=%+v", lineage, reloaded.CoverageProofLineage)
	}

	record.CoverageProofLineage[0].SourceProofIdentity.TargetRefs[0] = "mutated-record-target"
	if aggregate.CoverageProofLineage[0].SourceProofIdentity.TargetRefs[0] == "mutated-record-target" {
		t.Fatal("runtime state record retained source proof identity storage")
	}
	reloaded.CoverageProofLineage[1].CurrentProofIdentity.TargetRefs[0] = "mutated-reloaded-target"
	if record.CoverageProofLineage[1].CurrentProofIdentity.TargetRefs[0] == "mutated-reloaded-target" {
		t.Fatal("runtime aggregate retained resolution proof identity storage")
	}
}

func TestRuntimeStateRecordPreservesRepairSurfaceConfirmationFenceAcrossOpaqueReload(t *testing.T) {
	ctx := context.Background()
	aggregate := runtimeAggregate(t, 7)
	aggregate.Facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	aggregate.Facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	aggregate.Facts.Focus.ValidationRecipeID = "repair-surface-reload"
	const repairSurfaceCheck = "check/repair-surface-reload"
	aggregate.Facts.Focus.RequiredCheckRefs = []string{repairSurfaceCheck}
	aggregate.Facts.Focus.TargetRefs = []string{"main.go"}
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Capabilities = runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}}
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "repair-surface-reload", Command: "sh", Args: []string{"-c", "true"}}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "admissibility-shell-repair-surface-reload", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"main.go"},
		ValidationRecipeRefs: []string{recipe.ID}, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
		}},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: recipe.ID, OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness: runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "repair-surface-reload-input-v1", InputBindingRef: "repair-surface-reload-input",
			CheckRefs: []string{repairSurfaceCheck}, ProofApplicability: runtimecontract.RuntimeValidationProofDirect,
			EffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		}},
		ObservedRevision: "shell-repair-surface-reload-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	aggregate.Facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{
		ID: inventory.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}}
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	aggregate = runtimeTestAggregateWithSemanticPlanning(t, aggregate)
	repository := &runtimeDriveRepository{aggregate: aggregate}
	controller, err := runtime.NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	var snapshotPrerequisite runtimecontract.RuntimePrerequisite
	for _, pending := range aggregate.Facts.Prerequisites.Pending {
		if pending.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && pending.OwnerModuleID == runtimecontract.RuntimeModuleFile && pending.SubjectRef == "main.go" {
			snapshotPrerequisite = pending
			break
		}
	}
	if snapshotPrerequisite.ID == "" {
		t.Fatalf("repair-surface reload fixture has no File target-snapshot prerequisite: %+v", aggregate.Facts.Prerequisites.Pending)
	}
	snapshot := runtimecontract.TargetSnapshot{TargetRef: "main.go", ContentHash: "repair-surface-main-v1", ContentState: runtimecontract.SnapshotContentStateComplete}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: snapshotPrerequisite.ID, Kind: snapshotPrerequisite.Kind, OwnerModuleID: snapshotPrerequisite.OwnerModuleID,
		ContentHash: snapshot.ContentHash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: snapshot.TargetRef, Revision: snapshot.ContentHash,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	aggregate, err = runtimeTestControllerCommitPrerequisite(controller, ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtime.PrerequisiteResult{
		PrerequisiteID: snapshotPrerequisite.ID, TargetSnapshot: &snapshot, Observation: &observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("repair-surface reload fixture action = %s, want need_validation", aggregate.Decision.State)
	}
	bindingRef := aggregate.Facts.Focus.CoverageEvidenceBindings[0].ID
	recipeID := recipe.ID
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-repair-surface-reload", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		BindingRefs: []string{bindingRef}, TargetRefs: []string{"main.go"},
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}}}}}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(aggregate.Facts.Resources),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	dispatch, err := runtime.SealDispatchWithManifestAndFeasibilities(
		aggregate.Decision,
		candidate,
		aggregate.Facts.Activation,
		manifest,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	recordAcceptedRuntimeDispatchAdmissionForTest(t, &aggregate, dispatch, manifest)
	aggregate.ActiveDispatch = &dispatch
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("sealed repair-surface reload fixture invalid before execution: %v", err)
	}
	repository.aggregate = aggregate
	failed, err := runtimeTestControllerExecuteDispatch(controller, ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID,
		runtimeOperationExecutorFunc(func(ctx context.Context, request runtime.RuntimeOperationExecutionRequest) (runtime.RuntimeOperationExecutionResult, error) {
			validationObservation := runtimecontract.RuntimeValidationFailureObservation{
				WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID,
				DispatchID: request.DispatchID, OperationID: request.Operation.ID,
				FailureSignature: "build-failure", TargetRole: request.TargetRole, DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed,
				CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"}, DiagnosticHash: strings.Repeat("d", 64),
			}
			failure := runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassBuildFailure, Code: "build_failed", Signature: validationObservation.FailureSignature,
				Message: "managed validation failed",
				ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{
					Class: validationObservation.DiagnosticClass, TargetRefs: append([]string(nil), validationObservation.CandidateTargetRefs...),
				},
				ValidationFailureObservation: &validationObservation,
			}
			receipt := runtimecontract.RuntimeOperationReceipt{
				ExecutionKey: request.Execution.ExecutionKey, OperationID: request.Operation.ID,
				Outcome: runtimecontract.RuntimeOperationOutcomeRejected, ObservedEffectScope: runtimecontract.RuntimeEffectUnknown,
				Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "managed validation rejected"},
			}
			feedback, feedbackErr := runtimecontract.RuntimeFeedbackFromFailureFact(runtimecontract.RuntimeFeedbackScope{
				WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.Transaction.AuthoritySnapshotID, DispatchID: request.DispatchID,
			}, runtimecontract.RuntimeFeedbackDomainOperation, runtimecontract.RuntimeFeedbackCategory(receipt.Outcome), runtimecontract.RuntimeModuleShell, failure)
			if feedbackErr != nil {
				return runtime.RuntimeOperationExecutionResult{}, feedbackErr
			}
			feedback.OriginalDiagnostic.OperationID = request.Operation.ID
			receipt.Feedback = &feedback
			descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(request.Operation.Kind)
			if !found || descriptor.OwnerModuleID == "" {
				return runtime.RuntimeOperationExecutionResult{}, fmt.Errorf("unknown operation kind %q", request.Operation.Kind)
			}
			_, committed, commitErr := runtimeTestControllerCommitOperationReceipt(controller, ctx, request.WorkspaceID, request.RunID, request.DispatchID, request.Execution.Fence, descriptor.OwnerModuleID, receipt)
			if commitErr != nil {
				return runtime.RuntimeOperationExecutionResult{}, commitErr
			}
			return runtime.RuntimeOperationExecutionResult{
				EventID: "event-repair-surface-reload", OperationID: request.Operation.ID,
				EvidenceKind: runtimecontract.EvidenceKindValidation, Status: runtime.ExecutionEvidenceStatusRejected,
				Failure: &failure, Evidence: committed.Evidence, Receipt: committed,
			}, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if failed.Facts.Failure == nil || failed.Facts.Failure.ValidationFailureObservation == nil {
		t.Fatalf("rejected managed validation did not persist a validation observation: %+v", failed.Facts.Failure)
	}
	confirmation := runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation:   *failed.Facts.Failure.ValidationFailureObservation,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		Disposition: runtimecontract.RuntimeRepairSurfaceStale, ObservedRevision: "tree-v2", Reason: "changed",
	}
	confirmedEvent, err := controller.ApplyInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:      runtime.RuntimeInternalEventRepairSurfaceConfirmationObserved,
		Scope:     runtime.RuntimeInternalEventScope{WorkspaceID: failed.Facts.WorkspaceID, RunID: failed.Facts.RunID},
		Authority: &runtime.RuntimeInternalAuthorityPayload{RepairSurfaceConfirmation: &confirmation},
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed := confirmedEvent.Aggregate
	if err := confirmed.Validate(); err != nil {
		t.Fatalf("aggregate with confirmation fence: %v", err)
	}
	persisted := runtimeStateRecordFromAggregate(confirmed)
	if err := persisted.Validate(); err != nil {
		t.Fatalf("persisted confirmation fence: %v", err)
	}
	reloaded := runtimeAggregateFromRecord(persisted.Clone())
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("reloaded aggregate with confirmation fence: %v", err)
	}
	if len(reloaded.RepairSurfaceConfirmations) != 1 || !runtimecontract.SameRuntimeRepairSurfaceConfirmation(reloaded.RepairSurfaceConfirmations[0].Confirmation, confirmation) {
		t.Fatalf("repair surface confirmation fence drifted across reload: %+v", reloaded.RepairSurfaceConfirmations)
	}
	reloaded.RepairSurfaceConfirmations[0].Confirmation.Observation.CandidateTargetRefs[0] = "mutated.go"
	if persisted.RepairSurfaceConfirmations[0].Confirmation.Observation.CandidateTargetRefs[0] == "mutated.go" {
		t.Fatal("runtime state record shared repair surface confirmation backing storage")
	}
}

func runtimeAggregate(t *testing.T, revision uint64) runtime.RuntimeAggregate {
	t.Helper()
	// Keep aggregate-only fixtures on the same current-schema activation path as
	// persisted Run fixtures. The fixture deliberately remains before semantic
	// seal so tests can exercise the persisted planning ledger itself.
	bootstrap := runtimeBootstrap()
	facts := runtimecontract.RuntimeFacts{
		Activation:  bootstrap.Activation,
		WorkspaceID: "ws-v2", RunID: "run-v2", DefinitionRevision: "definition-v2", DefinitionSeed: bootstrap.DefinitionSeed, EvidenceRevision: "runtime-evidence-00000000000000000000",
		DefinitionFocuses:     runtimecontract.CloneRuntimeDefinitionFocuses(bootstrap.DefinitionFocuses),
		Focus:                 bootstrap.DefinitionFocuses[0].RuntimeFocusSeed(),
		Coverage:              []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-v2", SliceID: "slice-v2", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities:          runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{runtimecontract.StepKindFileEdit}},
		ConvergencePolicy:     runtimeTestConvergencePolicy(),
		RecoveryPolicy:        runtimecontract.DefaultRuntimeRecoveryPolicy(),
		ExecutionBudgetPolicy: bootstrap.ExecutionBudgetPolicy,
	}
	facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:                bootstrap.Activation.ProviderSelection,
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 8192,
		MaxInputTokens:           119808,
		MaxBytes:                 128 * 1024,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
	}
	initialized, err := runtimecontract.InitializeRuntimeCWILedgerForActivation(facts)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := runtime.ReduceRuntimeState(initialized, nil)
	if err != nil {
		t.Fatal(err)
	}
	semanticPlanning, err := runtimecontract.NewRuntimeSemanticPlanningLedger(bootstrap.DefinitionSeed)
	if err != nil {
		t.Fatal(err)
	}
	executionBudget, err := runtimecontract.NewRuntimeExecutionBudgetLedger(bootstrap.ExecutionBudgetPolicy)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := runtime.RuntimeAggregate{
		StateRevision:    revision,
		Definition:       bootstrap.Definition,
		Facts:            initialized,
		SemanticPlanning: semanticPlanning,
		ExecutionBudget:  executionBudget,
		Decision:         decision,
	}
	if err := aggregate.Validate(); err != nil {
		t.Fatal(err)
	}
	return aggregate
}

func runtimeCoverageProofLineageForRepositoryTest(t *testing.T, aggregate runtime.RuntimeAggregate) []runtimecontract.RuntimeCoverageProofLineage {
	t.Helper()
	if len(aggregate.Facts.Focus.CoverageEvidenceBindings) != 1 {
		t.Fatalf("runtime aggregate fixture requires one current binding: %+v", aggregate.Facts.Focus)
	}
	sourceBinding := aggregate.Facts.Focus.CoverageEvidenceBindings[0].Clone()
	sourceIdentity, err := runtimecontract.DeriveCoverageEvidenceProofIdentity(sourceBinding, runtimecontract.EvidenceKindStateChange)
	if err != nil {
		t.Fatalf("derive source proof identity: %v", err)
	}
	source := runtimecontract.RuntimeCoverageProofLineage{
		Kind:                   runtimecontract.RuntimeCoverageProofLineageKindSource,
		WorkspaceID:            aggregate.Facts.WorkspaceID,
		RunID:                  aggregate.Facts.RunID,
		CoverageWorkItemID:     sourceBinding.CoverageWorkItemID,
		EvidenceKind:           runtimecontract.EvidenceKindStateChange,
		SourceEvidenceID:       "evidence-runtime-lineage",
		SourceDispatchID:       "dispatch-runtime-lineage",
		SourceOperationID:      "operation-runtime-lineage",
		SourceExecutionKey:     "execution-runtime-lineage",
		SourceFence:            1,
		SourceReceiptHash:      "receipt-runtime-lineage",
		SourceBindingRef:       sourceBinding.ID,
		SourceBindingHash:      sourceBinding.Hash,
		SourceManifestHash:     "manifest-runtime-lineage",
		SourceLogicalRequestID: "request-runtime-lineage",
		SourceProofIdentity:    sourceIdentity,
	}
	source.ID = runtimecontract.StableRuntimeCoverageProofLineageSourceID(source.SourceReceiptHash, source.SourceEvidenceID, source.SourceBindingHash, source.SourceProofIdentity.Hash)
	currentBinding := sourceBinding.Clone()
	currentBinding.ID = sourceBinding.ID + "/reacquired"
	currentBinding.PrerequisiteRefs = []string{"operation-admissibility/file/revision-2"}
	currentBinding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(currentBinding)
	currentIdentity, err := runtimecontract.DeriveCoverageEvidenceProofIdentity(currentBinding, runtimecontract.EvidenceKindStateChange)
	if err != nil {
		t.Fatalf("derive current proof identity: %v", err)
	}
	resolution := runtimecontract.RuntimeCoverageProofLineage{
		Kind:                 runtimecontract.RuntimeCoverageProofLineageKindResolution,
		WorkspaceID:          aggregate.Facts.WorkspaceID,
		RunID:                aggregate.Facts.RunID,
		CoverageWorkItemID:   sourceBinding.CoverageWorkItemID,
		EvidenceKind:         runtimecontract.EvidenceKindStateChange,
		SourceLineageID:      source.ID,
		CurrentBindingRef:    currentBinding.ID,
		CurrentBindingHash:   currentBinding.Hash,
		CurrentProofIdentity: currentIdentity,
		Disposition:          runtimecontract.RuntimeCoverageProofLineageDispositionEquivalent,
	}
	resolution.ID = runtimecontract.StableRuntimeCoverageProofLineageResolutionID(resolution.SourceLineageID, resolution.CurrentBindingRef, resolution.CurrentBindingHash)
	if err := runtimecontract.ValidateRuntimeCoverageProofLineage([]runtimecontract.RuntimeCoverageProofLineage{source, resolution}); err != nil {
		t.Fatalf("build runtime proof lineage: %v", err)
	}
	return []runtimecontract.RuntimeCoverageProofLineage{source, resolution}
}

func runtimeCandidateRequestContentID(t *testing.T, aggregate runtime.RuntimeAggregate, stablePrefixHash string, attempt int) common.ArtifactContinuationID {
	t.Helper()
	context := runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, runtime.ActiveRejectedOperationShapes(aggregate))
	manifest, manifestErr := runtime.BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if manifestErr != nil {
		t.Fatal(manifestErr)
	}
	previousFailure, err := runtimePreviousCandidateFailure(aggregate, attempt)
	if err != nil {
		t.Fatal(err)
	}
	previousCorrection, err := runtimePreviousCandidateCorrection(aggregate, attempt)
	if err != nil {
		t.Fatal(err)
	}
	logicalRequestID, _, err := runtimeCandidateRequestIdentity(aggregate, stablePrefixHash, context, manifest, previousFailure, previousCorrection, 1, runtimeCandidateCorrectionInputFromDirective(previousCorrection))
	if err != nil {
		t.Fatal(err)
	}
	return runtimecontract.StableRuntimeCandidateRequestContentID(logicalRequestID)
}

func runtimeFileAdmissibilityObservation(inventory runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisiteObservation {
	inventory = inventory.Normalized()
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: inventory.PrerequisiteID,
		Kind:           runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID:  inventory.OwnerModuleID,
		ContentHash:    inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain:        runtimecontract.RuntimeResourceWorkspaceTree,
			OwnerModuleID: inventory.OwnerModuleID,
			SubjectRef:    "workspace_deliverable",
			Revision:      inventory.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func runtimeRepositoryHasOwnerCatalogRevision(values []runtimecontract.RuntimeOperationAdmissibility, ownerModuleID, revision string) bool {
	for _, value := range values {
		if value.OwnerModuleID == ownerModuleID && value.ObservedRevision == revision {
			return true
		}
	}
	return false
}

func runtimeRepositoryHasOwnerCatalogObservationRevision(values []runtimecontract.RuntimePrerequisiteObservation, ownerModuleID, revision string) bool {
	for _, value := range values {
		if value.OwnerModuleID != ownerModuleID {
			continue
		}
		for _, dependency := range value.Dependencies {
			if dependency.OwnerModuleID == ownerModuleID && dependency.Revision == revision {
				return true
			}
		}
	}
	return false
}

func runtimeOperationAdmissibilityPrerequisite(inventory runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisite {
	inventory = inventory.Normalized()
	return runtimecontract.RuntimePrerequisite{
		ID:                   inventory.PrerequisiteID,
		Kind:                 runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID:        inventory.OwnerModuleID,
		RequiredCapabilities: append([]string(nil), inventory.OperationKinds...),
	}
}

func runtimeDefinition() runtimecontract.RuntimeDefinitionContext {
	return runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: "definition-v2", GoalID: "goal-v2", GoalDefinition: "完成目标",
		DoDID: "dod-v2", DoDDefinition: "产生可验证交付物", CapabilityContract: []string{runtimecontract.StepKindFileEdit},
	}
}

func runtimeBootstrap() runtimecontract.RuntimeBootstrap {
	policy := runtimeTestAcceptanceSlicePolicy()
	bootstrap := runtimecontract.RuntimeBootstrap{
		Definition: runtimeDefinition(),
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{
				OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(),
				StateChangePolicy:         runtimecontract.RuntimeStateChangeAllowedButNotRequired,
				AcceptanceSlicePolicy:     &policy,
			},
		},
		DefinitionFocuses: []runtimecontract.RuntimeDefinitionFocus{{
			ID: "focus-v2", SliceID: "slice-v2", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		}},
		Slices:        []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-v2", Title: "Test", Scope: "test", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll, ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed}},
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-v2", SliceID: "slice-v2"}},
		Requirements: []runtimecontract.RuntimeRequirement{{
			Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			CapabilityClass: runtimecontract.RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{runtimecontract.StepKindFileEdit},
		}},
		ConvergencePolicy: runtimeTestConvergencePolicy(),
		RecoveryPolicy:    runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
	bootstrap.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy)
	return runtimeTestBootstrapWithBindings(bootstrap)
}

// runtimeFileBackedAssessmentBootstrap is the Run-owner bootstrap paired
// with the direct aggregate facts below.  Keeping them identical is material:
// the Run owner rejects an opaque aggregate whose activation contract drifts
// from the activation it persisted for the Run.
func runtimeFileBackedAssessmentBootstrap(coverageIDs []common.CoverageWorkItemID) runtimecontract.RuntimeBootstrap {
	bootstrap := runtimeBootstrap()
	bootstrap.DefinitionFocuses = make([]runtimecontract.RuntimeDefinitionFocus, 0, len(coverageIDs))
	bootstrap.Slices[0].CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), coverageIDs...)
	bootstrap.Slices[0].RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	bootstrap.CoverageItems = make([]runtimecontract.RuntimeCoverageItem, 0, len(coverageIDs))
	for idx, coverageID := range coverageIDs {
		bootstrap.DefinitionFocuses = append(bootstrap.DefinitionFocuses, runtimecontract.RuntimeDefinitionFocus{
			ID: fmt.Sprintf("focus-v2-%03d", idx+1), SliceID: bootstrap.Slices[0].ID,
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{coverageID},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			RequireAssessment:     true,
		})
		bootstrap.CoverageItems = append(bootstrap.CoverageItems, runtimecontract.RuntimeCoverageItem{ID: coverageID, SliceID: bootstrap.Slices[0].ID})
	}
	bootstrap.Activation.EvidencePolicies = append(bootstrap.Activation.EvidencePolicies, runtimeFileBackedManagedValidationEvidencePolicy())
	return runtimeTestBootstrapWithReboundBindings(bootstrap)
}

func runtimeTestAcceptanceSlicePolicy() runtimecontract.RuntimeAcceptanceSlicePolicy {
	return runtimecontract.RuntimeAcceptanceSlicePolicy{
		SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
		MaxWorkItems:  4, MaxFocuses: 16, MaxCandidateContextItems: 4,
		CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll,
		ResultPolicy:     runtimecontract.RuntimeAcceptanceSliceResultAllowed,
		Groups:           []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{ID: "test", Title: "Test", Scope: "test", Priority: 1, MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true}}},
	}
}

func runtimeTestConvergencePolicy() runtimecontract.RuntimeConvergencePolicy {
	policy := runtimecontract.DefaultRuntimeConvergencePolicy()
	policy.MaxAcceptanceSliceWorkItems = 4
	policy.MaxFocuses = 16
	policy.MaxCandidateContextItems = 4
	return policy
}

func runtimeRunForTest(state common.RunState, definition runtimecontract.RuntimeDefinitionContext) *runmodel.Run {
	bootstrap := runtimeBootstrap()
	bootstrap.Definition = definition
	return runtimeRunForTestBootstrap(state, bootstrap)
}

func runtimeRunForTestBootstrap(state common.RunState, bootstrap runtimecontract.RuntimeBootstrap) *runmodel.Run {
	definition := bootstrap.Definition
	return &runmodel.Run{
		ID:                  "run-v2",
		WorkspaceID:         "ws-v2",
		GoalID:              definition.GoalID,
		DoDID:               definition.DoDID,
		DefinitionRevision:  definition.Revision,
		RuntimeBootstrap:    &bootstrap,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun,
		Lineage:             runmodel.RunLineage{LineageState: "active"},
		State:               state,
	}
}
