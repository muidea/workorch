package runtime

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/semanticplan"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeValidationFailureRepairConvergenceForTest(maxAttempts int) runtimecontract.RuntimeValidationFailureRepairConvergence {
	return runtimecontract.RuntimeValidationFailureRepairConvergence{
		ID: "test-validation-failure-repair",
		Routes: []runtimecontract.RuntimeValidationFailureRepairRoute{
			runtimeValidationFailureRepairRouteForTest("build-failure-to-file", runtimecontract.ValidationDiagnosticBuildFailed, runtimecontract.ValidationEvidenceTargetFailureSurface),
			runtimeValidationFailureRepairRouteForTest("test-failure-to-file", runtimecontract.ValidationDiagnosticTestFailed, runtimecontract.ValidationEvidenceTargetBehaviorEvidence),
		},
		RequireFreshValidation: true,
		MaxAttempts:            maxAttempts,
	}
}

func runtimeValidationFailureRepairRouteForTest(id, diagnosticClass, targetType string) runtimecontract.RuntimeValidationFailureRepairRoute {
	return runtimecontract.RuntimeValidationFailureRepairRoute{
		ID:                                        id,
		TriggerOwnerModuleID:                      runtimecontract.RuntimeModuleShell,
		TriggerOperationKind:                      runtimecontract.RuntimeBlockKindManagedValidationSession,
		TriggerDiagnosticClass:                    diagnosticClass,
		TriggerEvidenceTargetType:                 targetType,
		TriggerOwnerDiagnosticCode:                "shell_exit_nonzero",
		RepairEvidenceKind:                        runtimecontract.EvidenceKindStateChange,
		RepairOwnerModuleID:                       runtimecontract.RuntimeModuleFile,
		RepairOperationKinds:                      []string{runtimecontract.StepKindFileWrite},
		RepairTargetPolicy:                        runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
		RepairTargetRole:                          "implementation",
		RepairDeclarationTargetSelectorRef:        "profile/workspace-deliverable",
		RepairDeclarationTargetSelectorOwnerRef:   "test-profile",
		RepairDeclarationTargetSelectorSubjectRef: "dodItem",
		RepairOperationTargetSelectorRef:          "file/workspace-relative-new-target",
	}
}

func runtimeValidationFailureRepairAggregateForTest(t *testing.T, convergence runtimecontract.RuntimeValidationFailureRepairConvergence, prepare ...func(*RuntimeAggregate)) RuntimeAggregate {
	t.Helper()
	// Install the repair policy and owner catalogs before the baseline SM-2
	// generation is sealed. Rewriting immutable Definition inputs after the
	// baseline state-change dispatch would leave its request/dispatch copies
	// without the exact provider/owner semantic preimage required on restart.
	return validationOnlyAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		focus := aggregate.Facts.Focus.Normalized()
		focus.ID = "definition-validation-repair/cwi-1"
		focus.AssessmentScope = "dodItem"
		focus.AssessmentID = "dod-item-1"
		focus.AssessmentText = "validation succeeds"
		focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
		focus.ValidationRecipeID = "test-managed-validation"
		aggregate.Facts.Focus = focus
		declaration := runtimecontract.CompileRuntimeDefinitionFocuses([]runtimecontract.CoverageFocus{focus})[0]
		declaration.EvidenceDeclarationRef = runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-item-1")
		declaration.ValidationFailureRepairConvergence = &convergence
		aggregate.Facts.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{declaration}
		aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{
			runtimeValidationFailureRepairFileCatalogForTest(),
			runtimeValidationFailureRepairShellCatalogForTest(),
		}
		for _, route := range convergence.Routes {
			aggregate.Facts.Capabilities.StateChangeKinds = append(aggregate.Facts.Capabilities.StateChangeKinds, route.RepairOperationKinds...)
			aggregate.Definition.CapabilityContract = append(aggregate.Definition.CapabilityContract, route.RepairOperationKinds...)
		}
		for _, hook := range prepare {
			hook(aggregate)
		}
	})
}

func runtimeValidationFailureRepairFileCatalogForTest() runtimecontract.RuntimeOperationAdmissibility {
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "file-repair-catalog", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply},
		SelectorRefs:   []string{"file/workspace-relative-existing-target", "file/workspace-relative-new-target"},
		ConstraintRefs: []string{"workspace-relative-path", "existing-target-must-be-listed", "new-target-requires-owner-selector"},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{
			{
				Kind: runtimecontract.StepKindFileWrite,
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
					runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
				},
				SelectorRefs: []string{"file/workspace-relative-new-target"}, ConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"},
			},
			{
				Kind: runtimecontract.StepKindFileEdit,
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
					runtimecontract.RuntimeOperationTargetPolicyExactTarget,
					runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
				},
				SelectorRefs:   []string{"file/workspace-relative-existing-target"},
				ConstraintRefs: []string{"workspace-relative-path", "existing-target-must-be-listed"},
			},
			{
				Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write",
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
					runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
				},
				SelectorRefs: []string{"file/workspace-relative-new-target"}, ConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"},
			},
		},
		ObservedRevision: "file-repair-revision",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	return catalog
}

func runtimeValidationFailureRepairShellCatalogForTest() runtimecontract.RuntimeOperationAdmissibility {
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-validation-catalog", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"main.go"},
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{{ID: "test-managed-validation", Command: "true"}},
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
			ID: "test-managed-validation", OwnerModuleID: runtimecontract.RuntimeModuleShell,
			OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
			Readiness:     runtimecontract.RuntimeValidationSurfaceReady, InputSchemaHash: "test-validation-input-v1",
			InputBindingRef: "test-validation-input", CheckRefs: []string{"check/test-validation"},
			ProofApplicability: runtimecontract.RuntimeValidationProofDirect, EffectScope: runtimecontract.RuntimeEffectEphemeralSession,
			IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		}, runtimeTestUnavailableAlternativeValidationSurface()},
		ObservedRevision: "shell-validation-revision",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	return catalog
}

func runtimeRejectedValidationEvidenceForTest(t *testing.T, sealed RuntimeAggregate, eventID string, diagnosticClass string) ExecutionEvidence {
	t.Helper()
	operationID := runtimeTestCandidateOperationID(t, sealed.ActiveDispatch.Candidate, 0)
	targetType := runtimecontract.ValidationEvidenceTargetFailureSurface
	if diagnosticClass == runtimecontract.ValidationDiagnosticTestFailed {
		targetType = runtimecontract.ValidationEvidenceTargetBehaviorEvidence
	}
	return ExecutionEvidence{
		EventID: eventID, WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: operationID, Kind: runtimecontract.EvidenceKindValidation, Status: ExecutionEvidenceStatusRejected,
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassBuildFailure, Code: "validation_failed", Signature: "validation-failure/typed/1", Message: "typed validation failed",
			ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{
				Class: diagnosticClass, RecommendedEvidenceTargetType: targetType, OwnerCode: "shell_exit_nonzero",
				Operation: runtimecontract.RuntimeBlockKindManagedValidationSession,
			},
		},
	}
}

func TestRejectedTypedValidationWithoutFailureTargetOpensProfileRepairGeneration(t *testing.T) {
	aggregate := runtimeValidationFailureRepairAggregateForTest(t, runtimeValidationFailureRepairConvergenceForTest(2))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	// The sealed Profile, not an Orchestrator capability-name branch, is the
	// only authority that admits this target-free implementation rejection into
	// the bounded repair generation.
	evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "event-validation-profile-repair", runtimecontract.ValidationDiagnosticBuildFailed)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	input, required, err := CurrentRuntimeValidationFailureRepairGenerationInput(committed.Aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if !required || input.CoverageWorkItemID != "cwi-1" || input.FailedFocusID != committed.Aggregate.Facts.DefinitionFocuses[0].ID ||
		input.TriggerOwnerModuleID != runtimecontract.RuntimeModuleShell || input.TriggerOperationKind != runtimecontract.RuntimeBlockKindManagedValidationSession ||
		input.TriggerDiagnosticClass != runtimecontract.ValidationDiagnosticBuildFailed || input.TriggerEvidenceTargetType != runtimecontract.ValidationEvidenceTargetFailureSurface ||
		input.TriggerOwnerDiagnosticCode != "shell_exit_nonzero" {
		t.Fatalf("persisted repair generation input = %+v required=%v", input, required)
	}
	item := committed.Aggregate.Facts.Coverage[0]
	if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened || item.CurrentBindingRef != nil ||
		runtimeValidationFailureRepairReopenReason(item) != runtimeCWIReopenReasonValidationFailureRepairRequired ||
		committed.Aggregate.Facts.RepairDirective == nil || committed.Aggregate.Facts.RepairDirective.Closure != nil ||
		len(committed.Aggregate.Facts.RepairDirective.TargetRefs) != 0 || committed.Aggregate.DefinitionClarification != nil {
		t.Fatalf("validation failure did not open target-free repair generation: %+v", committed.Aggregate)
	}

	// Duplicate delivery is fenced before SM-2 and cannot consume another
	// repair attempt or append another CWI reopen lineage record.
	revision := committed.Aggregate.StateRevision
	duplicate, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != revision ||
		duplicate.Aggregate.Facts.Repair.RepairAttempts != 1 || len(duplicate.Aggregate.Facts.Coverage[0].ReopenLineage) != 1 {
		t.Fatalf("duplicate rejected validation changed repair generation: result=%+v err=%v", duplicate, err)
	}

	// Repository load is the process restart boundary. The compiler input must
	// be reconstructed from persisted rejected fact + dispatch lineage only.
	reloaded, err := repository.Load(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	reloadedInput, reloadedRequired, err := CurrentRuntimeValidationFailureRepairGenerationInput(reloaded)
	if err != nil || !reloadedRequired || reloadedInput != input {
		t.Fatalf("restart repair generation input = %+v required=%v err=%v", reloadedInput, reloadedRequired, err)
	}
}

func TestRejectedTypedValidationWithoutStructuredTargetTerminatesExistingScopeRoute(t *testing.T) {
	convergence := runtimeValidationFailureRepairConvergenceForTest(2)
	convergence.Routes = convergence.Routes[:1]
	convergence.Routes[0].RepairOperationKinds = []string{runtimecontract.StepKindFileEdit}
	convergence.Routes[0].RepairTargetPolicy = runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope
	convergence.Routes[0].RepairOperationTargetSelectorRef = "file/workspace-relative-existing-target"
	aggregate := runtimeValidationFailureRepairAggregateForTest(t, convergence)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "event-validation-existing-scope-no-target", runtimecontract.ValidationDiagnosticBuildFailed)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || committed.Aggregate.Decision.Terminal == nil ||
		committed.Aggregate.Decision.Terminal.Code != "repair_surface_unavailable" || committed.Aggregate.Facts.Failure == nil ||
		committed.Aggregate.Facts.Failure.Class != runtimecontract.FailureClassCandidateInfeasible ||
		committed.Aggregate.Facts.Failure.Code != "repair_surface_unavailable" || committed.Aggregate.Facts.Repair.Exhausted ||
		committed.Aggregate.DefinitionClarification != nil || committed.Aggregate.Facts.RepairDirective != nil {
		t.Fatalf("existing-scope rejection without owner target was not closed in receipt CAS: %+v", committed.Aggregate)
	}
	if _, required, inputErr := CurrentRuntimeValidationFailureRepairGenerationInput(committed.Aggregate); inputErr != nil || required {
		t.Fatalf("existing-scope rejection opened semantic target selection: required=%v err=%v", required, inputErr)
	}
}

func TestRejectedTypedValidationWithoutExactProfileRouteTerminatesWithoutRetry(t *testing.T) {
	for _, diagnostic := range []struct{ name, class, target, code string }{
		{"dependency", runtimecontract.ValidationDiagnosticDependencyMismatch, runtimecontract.ValidationEvidenceTargetDependencyContract, "dependency_catalog_missing"},
		// Owner-private codes remain opaque to the reducer. A test failure
		// without attribution must not borrow an implementation repair route.
		{"unattributed test", runtimecontract.ValidationDiagnosticTestFailed, runtimecontract.ValidationEvidenceTargetValidationOutput, "owner_test_attribution_unavailable"},
	} {
		t.Run(diagnostic.name, func(t *testing.T) {
			aggregate := runtimeValidationFailureRepairAggregateForTest(t, runtimeValidationFailureRepairConvergenceForTest(2))
			repository := &durableRuntimeStateRepository{aggregate: aggregate}
			coordinator, err := NewDispatchCoordinator(repository)
			if err != nil {
				t.Fatal(err)
			}
			sealed := sealCandidateForTest(t, repository, validationCandidate())
			evidence := runtimeRejectedValidationEvidenceForTest(
				t, sealed, "event-validation-route-unavailable", diagnostic.class,
			)
			evidence.Failure.ValidationDiagnostic.RecommendedEvidenceTargetType = diagnostic.target
			evidence.Failure.ValidationDiagnostic.OwnerCode = diagnostic.code
			attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)

			committed, err := coordinator.commitEvidence(context.Background(), evidence)
			if err != nil {
				t.Fatal(err)
			}
			if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || committed.Aggregate.Decision.Terminal == nil ||
				committed.Aggregate.Decision.Terminal.Code != "validation_failure_repair_route_unavailable" ||
				committed.Aggregate.Facts.Failure == nil || committed.Aggregate.Facts.Failure.Class != runtimecontract.FailureClassNoProgressExhausted ||
				committed.Aggregate.Facts.Failure.Code != "validation_failure_repair_route_unavailable" || !committed.Aggregate.Facts.Repair.Exhausted ||
				committed.Aggregate.Facts.Repair.RepairAttempts != 1 || committed.Aggregate.Facts.Repair.ValidationOnlyRetries != 0 ||
				len(committed.Aggregate.Facts.Coverage[0].ReopenLineage) != 0 || committed.Aggregate.DefinitionClarification != nil {
				t.Fatalf("unroutable owner diagnostic did not terminate in its receipt CAS: %+v", committed.Aggregate)
			}
			if _, required, inputErr := CurrentRuntimeValidationFailureRepairGenerationInput(committed.Aggregate); inputErr != nil || required {
				t.Fatalf("unroutable diagnostic opened semantic repair generation: required=%v err=%v", required, inputErr)
			}

			revision := committed.Aggregate.StateRevision
			restarted, err := NewDispatchCoordinator(repository)
			if err != nil {
				t.Fatal(err)
			}
			duplicate, err := restarted.commitEvidence(context.Background(), evidence)
			if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != revision ||
				duplicate.Aggregate.Facts.Repair.RepairAttempts != 1 || duplicate.Aggregate.Facts.Repair.ValidationOnlyRetries != 0 {
				t.Fatalf("duplicate unroutable rejection changed terminal authority: result=%+v err=%v", duplicate, err)
			}
		})
	}
}

func TestRejectedStructuredValidationWithoutExactProfileRouteTerminatesInReceiptCAS(t *testing.T) {
	aggregate := runtimeValidationFailureRepairAggregateForTest(t, runtimeValidationFailureRepairConvergenceForTest(2))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	operationID := runtimeTestCandidateOperationID(t, sealed.ActiveDispatch.Candidate, 0)
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID:          sealed.ActiveDispatch.ID, OperationID: operationID,
		FailureSignature:    "validation-failure/structured-no-route/1",
		TargetRole:          sealed.ActiveDispatch.FocusTargetRole,
		DiagnosticClass:     runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"},
		DiagnosticHash: strings.Repeat("7", 64),
	}
	evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "event-validation-structured-route-unavailable", runtimecontract.ValidationDiagnosticBuildFailed)
	evidence.Failure.Signature = observation.FailureSignature
	evidence.Failure.ValidationDiagnostic.OwnerCode = "unmapped_owner_code"
	evidence.Failure.ValidationDiagnostic.TargetRefs = append([]string(nil), observation.CandidateTargetRefs...)
	observation.DiagnosticHash = runtimecontract.StableRuntimeValidationFailureObservationDiagnosticHash(*evidence.Failure.ValidationDiagnostic, observation.ValidationScopeTargetRefs)
	evidence.Failure.ValidationFailureObservation = &observation
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)

	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateFailed || committed.Aggregate.Decision.Terminal == nil ||
		committed.Aggregate.Decision.Terminal.Code != "validation_failure_repair_route_unavailable" ||
		committed.Aggregate.Facts.Resources.FailureTarget != nil || committed.Aggregate.Facts.RepairDirective != nil ||
		len(committed.Aggregate.RepairSurfaceConfirmations) != 0 || !committed.Aggregate.Facts.Repair.Exhausted {
		t.Fatalf("structured unroutable diagnostic escaped same-CAS terminal closure: %+v", committed.Aggregate)
	}
	revision := committed.Aggregate.StateRevision
	restarted, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := restarted.commitEvidence(context.Background(), evidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != revision ||
		len(duplicate.Aggregate.RepairSurfaceConfirmations) != 0 {
		t.Fatalf("structured unroutable duplicate changed terminal authority: result=%+v err=%v", duplicate, err)
	}
}

func TestRejectedValidationObservationWaitsForFileConfirmationBeforeSemanticRepair(t *testing.T) {
	aggregate := runtimeValidationFailureRepairAggregateForTest(t, runtimeValidationFailureRepairConvergenceForTest(2))
	aggregate.Facts.Focus.TargetRole = "semantic_proof_target"
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, nil, aggregate.CoverageProofLineage)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	originalFocus := aggregate.Facts.Focus.Normalized()
	originalPlanning := aggregate.SemanticPlanning.Clone()
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	observation := runtimecontract.RuntimeValidationFailureObservation{
		WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		DispatchID:          sealed.ActiveDispatch.ID,
		OperationID:         runtimeTestCandidateOperationID(t, sealed.ActiveDispatch.Candidate, 0),
		FailureSignature:    "validation-failure/owner-observation/1", TargetRole: "semantic_proof_target",
		DiagnosticClass:     runtimecontract.ValidationDiagnosticBuildFailed,
		CandidateTargetRefs: []string{"main.go"}, ValidationScopeTargetRefs: []string{"main.go"},
		DiagnosticHash: strings.Repeat("d", 64),
	}
	evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "event-validation-owner-observation", runtimecontract.ValidationDiagnosticBuildFailed)
	evidence.Failure.Signature = observation.FailureSignature
	evidence.Failure.ValidationDiagnostic.TargetRefs = append([]string(nil), observation.CandidateTargetRefs...)
	observation.DiagnosticHash = runtimecontract.StableRuntimeValidationFailureObservationDiagnosticHash(*evidence.Failure.ValidationDiagnostic, observation.ValidationScopeTargetRefs)
	evidence.Failure.ValidationFailureObservation = &observation
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	var validationDispatch *SealedDispatch
	for index := range committed.Aggregate.CompletedDispatches {
		if committed.Aggregate.CompletedDispatches[index].ID == sealed.ActiveDispatch.ID {
			validationDispatch = &committed.Aggregate.CompletedDispatches[index]
			break
		}
	}
	if committed.Aggregate.ActiveDispatch != nil || validationDispatch == nil || validationDispatch.FocusTargetRole != observation.TargetRole {
		t.Fatalf("completed validation dispatch did not retain sealed focus role: %+v", committed.Aggregate)
	}
	if !reflect.DeepEqual(originalFocus, committed.Aggregate.Facts.Focus.Normalized()) {
		t.Fatalf("owner observation reopened or replaced focus before owner confirmation: before=%+v after=%+v", originalFocus, committed.Aggregate.Facts.Focus)
	}
	currentPlanning := committed.Aggregate.SemanticPlanning
	if currentPlanning.Phase != originalPlanning.Phase || currentPlanning.Attempt != originalPlanning.Attempt ||
		currentPlanning.LogicalRequestID != originalPlanning.LogicalRequestID || currentPlanning.DocumentHash != originalPlanning.DocumentHash ||
		currentPlanning.SealedBindingSetHash != originalPlanning.SealedBindingSetHash || len(currentPlanning.Batches) != len(originalPlanning.Batches) ||
		len(currentPlanning.History) != len(originalPlanning.History) {
		t.Fatalf("owner observation opened semantic planning before owner confirmation: before=%+v after=%+v", originalPlanning, committed.Aggregate.SemanticPlanning)
	}
	if committed.Aggregate.Facts.Repair.RepairAttempts != 1 || committed.Aggregate.Facts.Repair.ValidationOnlyRetries != 0 {
		t.Fatalf("pending owner confirmation created extra repair or validation-retry authority: %+v", committed.Aggregate.Facts.Repair)
	}
	if _, required, inputErr := CurrentRuntimeValidationFailureRepairGenerationInput(committed.Aggregate); inputErr != nil || required {
		t.Fatalf("pending owner confirmation exposed semantic repair input: required=%v err=%v", required, inputErr)
	}
	if committed.Aggregate.Decision.State != runtimecontract.ConvergenceStateNeedValidation || committed.Aggregate.Facts.Resources.FailureTarget != nil || committed.Aggregate.Facts.RepairDirective != nil {
		t.Fatalf("pending owner confirmation selected premature repair authority: decision=%+v target=%+v directive=%+v", committed.Aggregate.Decision, committed.Aggregate.Facts.Resources.FailureTarget, committed.Aggregate.Facts.RepairDirective)
	}

	// A later focus projection cannot rewrite the completed validation's
	// immutable diagnostic causation. This models the exact restart-era drift
	// that previously compared the receipt against mutable current Focus.
	drifted := committed.Aggregate
	drifted.Facts.Focus.TargetRole = "prospective_implementation"
	if !sealedValidationFailureObservationMatches(drifted, observation) {
		t.Fatal("completed dispatch causation changed with mutable current focus")
	}
	forged := observation.Clone()
	forged.TargetRole = drifted.Facts.Focus.TargetRole
	if sealedValidationFailureObservationMatches(drifted, forged) {
		t.Fatal("mutable current focus role was accepted as completed dispatch causation")
	}

	persistedRevision := committed.Aggregate.StateRevision
	duplicate, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != persistedRevision ||
		duplicate.Aggregate.Facts.Repair.RepairAttempts != 1 || duplicate.Aggregate.Facts.Repair.ValidationOnlyRetries != 0 {
		t.Fatalf("duplicate owner observation changed pending confirmation authority: result=%+v err=%v", duplicate, err)
	}
	reloaded, err := repository.Load(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, required, inputErr := CurrentRuntimeValidationFailureRepairGenerationInput(reloaded); inputErr != nil || required ||
		!sealedValidationFailureObservationMatches(reloaded, observation) {
		t.Fatalf("restart changed pending owner confirmation: required=%v err=%v aggregate=%+v", required, inputErr, reloaded)
	}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	confirmation := bindRuntimeRepairSurfaceConfirmationRequestForTest(t, reloaded, runtimecontract.RuntimeRepairSurfaceConfirmation{
		Observation: observation, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		Disposition: runtimecontract.RuntimeRepairSurfaceConfirmed, ConfirmedTargetRefs: []string{"main.go"}, ObservedRevision: "tree-v1",
	})
	confirmed, err := controller.recordRepairSurfaceConfirmation(context.Background(), reloaded.Facts.WorkspaceID, reloaded.Facts.RunID, confirmation)
	if err != nil {
		t.Fatal(err)
	}
	confirmed = satisfyConfirmedRepairTargetSnapshotsForTest(t, context.Background(), controller, confirmed, "tree-v1")
	if confirmed.Facts.RepairDirective == nil || confirmed.Facts.RepairDirective.Purpose != runtimecontract.RuntimeDispatchRepairPurposeConfirmedValidation ||
		!reflect.DeepEqual(confirmed.Facts.RepairDirective.AllowedOperationKinds, []string{runtimecontract.StepKindFileWrite}) {
		t.Fatalf("confirmed repair directive escaped exact Profile operation route: %+v", confirmed.Facts.RepairDirective)
	}
}

func runtimeValidationFailureRepairSemanticIdentityForTest(t *testing.T, aggregate RuntimeAggregate) semanticplan.ManifestIdentity {
	t.Helper()
	input, required, err := CurrentRuntimeValidationFailureRepairGenerationInput(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	identity := semanticplan.ManifestIdentity{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
		Definition: aggregate.Definition, DefinitionSeed: aggregate.Facts.DefinitionSeed,
		DefinitionFocuses: aggregate.Facts.DefinitionFocuses,
		Execution:         aggregate.Facts.Activation.ExecutionContract,
		Admissibilities:   aggregate.Facts.Resources.OperationAdmissibilities,
		EligibleCoverage:  runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts),
	}
	if required {
		identity.EligibleCoverage = []common.CoverageWorkItemID{input.CoverageWorkItemID}
		identity.ValidationFailureRepair = &input
	}
	return identity
}

func runtimeValidationFailureRepairCatalogObservationForTest(t *testing.T, aggregate RuntimeAggregate, catalog runtimecontract.RuntimeOperationAdmissibility) string {
	t.Helper()
	for _, raw := range aggregate.Facts.Resources.PrerequisiteObservations {
		observation := raw.Normalized()
		if observation.ContentHash == catalog.SnapshotID && observation.OwnerModuleID == catalog.OwnerModuleID {
			return observation.ObservationID
		}
	}
	t.Fatalf("catalog %s/%s has no current observation", catalog.OwnerModuleID, catalog.SnapshotID)
	return ""
}

func runtimeValidationFailureRepairCatalogForOwnerTest(t *testing.T, manifest runtimecontract.RuntimeSemanticPlanManifest, owner string) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	for _, raw := range manifest.OwnerCatalogs {
		catalog := raw.Normalized()
		if catalog.OwnerModuleID == owner {
			return catalog
		}
	}
	t.Fatalf("semantic manifest has no catalog for owner %q: %+v", owner, manifest.OwnerCatalogs)
	return runtimecontract.RuntimeOperationAdmissibility{}
}

// runtimeSealValidationFailureRepairSemanticProofForTest drives the actual
// private semantic transaction. It deliberately does not install a focus or
// binding by hand: the production seal must persist the manifest lineage used
// later by completeEvidenceDispatch.
func runtimeSealValidationFailureRepairSemanticProofForTest(
	t *testing.T,
	repository *durableRuntimeStateRepository,
	manifest runtimecontract.RuntimeSemanticPlanManifest,
	proposal runtimecontract.RuntimeSemanticProofProposal,
	feasibility runtimecontract.RuntimeSemanticProofFeasibility,
) RuntimeAggregate {
	t.Helper()
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := repository.aggregate
	apply := func(event RuntimeInternalEvent) {
		t.Helper()
		event.Scope.WorkspaceID = aggregate.Facts.WorkspaceID
		event.Scope.RunID = aggregate.Facts.RunID
		result, applyErr := controller.ApplyInternalEvent(context.Background(), event)
		if applyErr != nil {
			t.Fatalf("apply %s: %v", event.Kind, applyErr)
		}
		aggregate = result.Aggregate
	}
	order := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder,
		CoverageOrder: []common.CoverageWorkItemID{proposal.CoverageWorkItemID},
	}
	if err := order.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	orderLogicalID := "semantic-order/" + manifest.Hash
	proofLogicalID := "semantic-proof/" + manifest.Hash
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanRequestStarted, Transaction: &RuntimeInternalTransactionPayload{
		SemanticManifest: &manifest, StablePrefixHash: "validation-failure-repair-prefix", LogicalRequestID: orderLogicalID,
	}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{
		SemanticDocument: &order, LogicalRequestID: orderLogicalID,
	}})
	batch, err := semanticplan.CompileFocusBatch(
		manifest, order, 0, proofLogicalID, aggregate.Facts.Focus.CoverageWorkItemIDs,
		aggregate.SemanticPlanning.PendingCoverageIDs, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err = controller.compileSemanticPlanBatch(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, batch)
	if err != nil {
		t.Fatal(err)
	}
	proof := runtimecontract.RuntimeSemanticPlanDocument{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch,
		BatchID:  batch.BatchID, BatchSequence: batch.Sequence,
		BatchCoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), batch.CoverageWorkItemIDs...),
		Proposals:                []runtimecontract.RuntimeSemanticProofProposal{proposal},
	}
	if err := proof.ValidateAgainstManifest(manifest); err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanEvaluated, Transaction: &RuntimeInternalTransactionPayload{
		SemanticDocument: &proof, LogicalRequestID: batch.RequestLogicalID,
	}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityRequired, Authority: &RuntimeInternalAuthorityPayload{}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticFeasibilityObserved, Authority: &RuntimeInternalAuthorityPayload{
		SemanticBatchID: aggregate.SemanticPlanning.FeasibilityBatchID, SemanticFeasibility: &feasibility,
	}})
	accepted := aggregate.SemanticPlanning.Batches[0]
	merged, err := runtimecontract.MergeRuntimeSemanticPlanFocusBatchDocument(manifest, accepted, proof)
	if err != nil {
		t.Fatal(err)
	}
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticPlanBatchDocumentsMerged, Transaction: &RuntimeInternalTransactionPayload{SemanticPlanMergedDocument: &merged}})
	apply(RuntimeInternalEvent{Kind: RuntimeInternalEventSemanticContractSealed, Authority: &RuntimeInternalAuthorityPayload{}})
	return aggregate
}

func runtimeValidationFailureRepairProofForTest(
	t *testing.T,
	aggregate RuntimeAggregate,
	targets []string,
) (runtimecontract.RuntimeSemanticPlanManifest, runtimecontract.RuntimeSemanticProofProposal, runtimecontract.RuntimeSemanticProofFeasibility) {
	t.Helper()
	identity := runtimeValidationFailureRepairSemanticIdentityForTest(t, aggregate)
	if identity.ValidationFailureRepair == nil {
		t.Fatal("repair semantic proof requested without persisted generation input")
	}
	manifest, err := semanticplan.CompileManifest(identity)
	if err != nil {
		t.Fatal(err)
	}
	obligation, found, err := runtimeSemanticObligationForCoverage(manifest, identity.ValidationFailureRepair.CoverageWorkItemID)
	if err != nil || !found || obligation.ValidationFailureRepair == nil {
		t.Fatalf("repair obligation = %+v found=%v err=%v", obligation, found, err)
	}
	repair := obligation.ValidationFailureRepair.Normalized()
	catalog := runtimeValidationFailureRepairCatalogForOwnerTest(t, manifest, repair.RepairOwnerModuleID)
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: identity.ValidationFailureRepair.CoverageWorkItemID,
		OwnerModuleID:      repair.RepairOwnerModuleID, OperationKind: repair.RepairOperationKinds[0],
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: repair.RepairTargetPolicy, OwnerSelectorRefs: []string{repair.RepairOperationTargetSelectorRef},
			ProspectiveConstraintRefs: append([]string(nil), repair.RepairConstraintRefs...),
			ProspectiveTargetRefs:     append([]string(nil), targets...),
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		DeclarationRef:        repair.ConvergenceID,
	}.Normalized()
	feasibility := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
		OwnerModuleID: catalog.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID,
		CatalogObservationID: runtimeValidationFailureRepairCatalogObservationForTest(t, aggregate, catalog),
		Status:               runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision,
		TargetRefs: append([]string(nil), targets...),
	}.Normalized()
	return manifest, proposal, feasibility
}

func runtimeValidationFailureFreshValidationProofForTest(
	t *testing.T,
	aggregate RuntimeAggregate,
) (runtimecontract.RuntimeSemanticPlanManifest, runtimecontract.RuntimeSemanticProofProposal, runtimecontract.RuntimeSemanticProofFeasibility) {
	t.Helper()
	identity := runtimeValidationFailureRepairSemanticIdentityForTest(t, aggregate)
	if identity.ValidationFailureRepair != nil {
		t.Fatal("fresh Definition proof retained repair-purpose generation input")
	}
	manifest, err := semanticplan.CompileManifest(identity)
	if err != nil {
		t.Fatal(err)
	}
	obligation, found, err := runtimeSemanticObligationForCoverage(manifest, "cwi-1")
	if err != nil || !found {
		t.Fatalf("fresh validation obligation found=%t err=%v", found, err)
	}
	catalog := runtimeValidationFailureRepairCatalogForOwnerTest(t, manifest, runtimecontract.RuntimeModuleShell)
	const recipeID = "test-managed-validation"
	if obligation.ValidationProofRequirement == nil {
		t.Fatal("fresh validation obligation has no exact proof requirement")
	}
	_, exactBinding, exact := runtimecontract.RuntimeValidationSurfaceForProofRequirement(catalog.ValidationSurfaces, *obligation.ValidationProofRequirement)
	if !exact || exactBinding.ValidationRecipeID != recipeID {
		t.Fatalf("fresh validation catalog has no exact recipe binding: %+v", catalog.ValidationSurfaces)
	}
	proposal := runtimecontract.RuntimeSemanticProofProposal{
		CoverageWorkItemID: "cwi-1", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Target: runtimecontract.RuntimeOperationTargetAuthorization{
			Policy: runtimecontract.RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"main.go"},
		},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession},
		ValidationRecipeID:    recipeID, DeclarationRef: obligation.DeclarationRef,
	}.Normalized()
	feasibility := runtimecontract.RuntimeSemanticProofFeasibility{
		SchemaVersion: runtimecontract.RuntimeSemanticFeasibilitySchemaCurrent,
		ManifestHash:  manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalHash: runtimecontract.StableRuntimeSemanticProofProposalHash(proposal),
		OwnerModuleID: catalog.OwnerModuleID, CatalogSnapshotID: catalog.SnapshotID,
		CatalogObservationID: runtimeValidationFailureRepairCatalogObservationForTest(t, aggregate, catalog),
		Status:               runtimecontract.RuntimeSemanticFeasibilityFeasible, ObservedRevision: catalog.ObservedRevision,
		TargetRefs: []string{"main.go"}, ValidationRecipeID: recipeID, RequiredCheckRefs: append([]string(nil), exactBinding.RequiredCheckRefs...),
	}.Normalized()
	return manifest, proposal, feasibility
}

func TestValidationFailureRepairManifestIdentityIsRepairAwareAndFailClosed(t *testing.T) {
	aggregate := runtimeValidationFailureRepairAggregateForTest(t, runtimeValidationFailureRepairConvergenceForTest(1))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	evidence := runtimeRejectedValidationEvidenceForTest(t, sealed, "event-validation-repair-identity", runtimecontract.ValidationDiagnosticBuildFailed)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	opened, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatal(err)
	}
	identity := runtimeValidationFailureRepairSemanticIdentityForTest(t, opened.Aggregate)
	manifest, err := semanticplan.CompileManifest(identity)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := semanticplan.CompileManifest(identity)
	if err != nil || replayed.Hash != manifest.Hash {
		t.Fatalf("persisted repair generation did not replay one manifest: first=%s second=%s err=%v", manifest.Hash, replayed.Hash, err)
	}
	if err := validateSemanticManifestIdentity(opened.Aggregate, manifest); err != nil {
		t.Fatalf("repair manifest identity rejected: %v", err)
	}

	normalIdentity := identity
	normalIdentity.ValidationFailureRepair = nil
	normalManifest, err := semanticplan.CompileManifest(normalIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSemanticManifestIdentity(opened.Aggregate, normalManifest); err == nil {
		t.Fatal("normal Definition manifest replaced a persisted repair generation")
	}

	staleFailure := opened.Aggregate
	staleFailure.Facts.Failure.Signature = "stale-failure"
	if err := validateSemanticManifestIdentity(staleFailure, manifest); err == nil {
		t.Fatal("repair manifest survived stale failure identity")
	}
	staleCatalog := opened.Aggregate
	for index := range staleCatalog.Facts.Resources.OperationAdmissibilities {
		catalog := &staleCatalog.Facts.Resources.OperationAdmissibilities[index]
		if catalog.OwnerModuleID != runtimecontract.RuntimeModuleFile {
			continue
		}
		catalog.ObservedRevision = "stale-file-revision"
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	}
	if err := validateSemanticManifestIdentity(staleCatalog, manifest); err == nil {
		t.Fatal("repair manifest survived stale owner catalog identity")
	}
}

func TestValidationFailureRepairProductionReceiptReopensFreshValidationAndProfileBudgetExhausts(t *testing.T) {
	// MaxAttempts=1 means the first rejection is the final allowed Profile
	// repair cycle. Equality with the bound must remain admissible.
	aggregate := runtimeValidationFailureRepairAggregateForTest(t, runtimeValidationFailureRepairConvergenceForTest(1))
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealedValidation := sealCandidateForTest(t, repository, validationCandidate())
	firstRejection := runtimeRejectedValidationEvidenceForTest(t, sealedValidation, "event-validation-repair-cycle-1", runtimecontract.ValidationDiagnosticBuildFailed)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &firstRejection)
	opened, err := coordinator.commitEvidence(context.Background(), firstRejection)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Aggregate.Facts.Repair.RepairAttempts != 1 || opened.Aggregate.Facts.Repair.Exhausted {
		t.Fatalf("last allowed Profile repair was rejected: %+v", opened.Aggregate.Facts.Repair)
	}

	repairTargets := []string{"docs/lsp_tools.md", "lsp_tools.go"}
	repairManifest, repairProposal, repairFeasibility := runtimeValidationFailureRepairProofForTest(t, opened.Aggregate, repairTargets)
	sealedRepairSemantic := runtimeSealValidationFailureRepairSemanticProofForTest(t, repository, repairManifest, repairProposal, repairFeasibility)
	binding, found := coverageEvidenceBindingForCoverage(sealedRepairSemantic.Facts.Focus, "cwi-1")
	if !found || !sameRuntimeTargetRefs(binding.ProspectiveTargetRefs, repairTargets) {
		t.Fatalf("repair semantic seal did not bind owner-confirmed target: %+v", sealedRepairSemantic.Facts.Focus)
	}
	repair, repairPurpose, err := currentRuntimeSemanticValidationFailureRepair(sealedRepairSemantic)
	if err != nil || !repairPurpose || repair.FailureSignature != "validation-failure/typed/1" {
		t.Fatalf("repair purpose was not resolved from current manifest lineage: repair=%+v found=%v err=%v", repair, repairPurpose, err)
	}
	if sealedRepairSemantic.Facts.RepairDirective == nil || sealedRepairSemantic.Facts.RepairDirective.Closure == nil ||
		!sameRuntimeTargetRefs(sealedRepairSemantic.Facts.RepairDirective.TargetRefs, repairTargets) {
		t.Fatalf("repair semantic seal did not install exact directive: %+v", sealedRepairSemantic.Facts.RepairDirective)
	}
	targetAuthority := runtimeTargetAuthorityPlanForFacts(sealedRepairSemantic.Facts)
	if !targetAuthority.hasProspectiveTargets() || len(targetAuthority.existingTargets()) != 0 ||
		!sameRuntimeTargetRefs(targetAuthority.prospectiveTargets(), repairTargets) {
		t.Fatalf("repair semantic targets were not classified as prospective: plan=%+v focus=%+v binding=%+v", targetAuthority, sealedRepairSemantic.Facts.Focus, binding)
	}
	for _, prerequisite := range sealedRepairSemantic.Facts.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && contains(repairTargets, prerequisite.SubjectRef) {
			t.Fatalf("binding-sealed prospective repair target requested a pre-create snapshot: %+v", prerequisite)
		}
	}
	if sealedRepairSemantic.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("repair semantic seal selected %s, pending=%+v", sealedRepairSemantic.Decision.State, sealedRepairSemantic.Facts.Prerequisites.Pending)
	}

	requiredSequence := sealedRepairSemantic.Decision.RequiredOutput.RequiredOperationSequence
	if len(requiredSequence) != len(repairTargets) {
		t.Fatalf("multi-target repair has no all-of operation sequence: %+v", requiredSequence)
	}
	operationIDs := make(map[string]string, len(requiredSequence))
	for index, requirement := range requiredSequence {
		operationIDs[requirement.RequirementID] = fmt.Sprintf("repair-write-%d", index+1)
	}
	repairOperations := make([]runtimecontract.RuntimeOperation, 0, len(requiredSequence))
	repairCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{}}
	fileCatalog := runtimeValidationFailureRepairCatalogForOwnerTest(t, repairManifest, runtimecontract.RuntimeModuleFile)
	repairFeasibilities := make([]runtimecontract.RuntimeCandidateFeasibilityRecord, 0, len(requiredSequence))
	for _, requirement := range requiredSequence {
		operation := runtimecontract.RuntimeOperation{
			ID: operationIDs[requirement.RequirementID], Kind: runtimecontract.StepKindFileWrite,
			BindingRefs: []string{binding.ID}, TargetRefs: []string{requirement.TargetRef},
			RequiredOperationRef: requirement.RequirementID, Parameters: runtimeFileWriteParameters("repaired"),
		}
		for _, dependencyRef := range requirement.DependsOnRequirementIDs {
			operation.DependsOn = append(operation.DependsOn, operationIDs[dependencyRef])
		}
		repairOperations = append(repairOperations, operation)
		repairCandidate.StateChange.Operations = append(repairCandidate.StateChange.Operations, runtimecontract.StateChangingOperation{Operation: operation})
		repairFeasibilities = append(repairFeasibilities, runtimecontract.RuntimeCandidateFeasibilityRecord{
			OperationID: operation.ID, AdmissibilitySnapshotID: fileCatalog.SnapshotID, AcquisitionBinding: fileCatalog.AcquisitionBinding,
			Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
		})
	}
	sealedRepairDispatch := sealCandidateWithFeasibilitiesForTest(t, repository, repairCandidate, repairFeasibilities)
	repairOperations = candidateRuntimeOperations(sealedRepairDispatch.ActiveDispatch.Candidate)
	if len(repairOperations) != len(requiredSequence) {
		t.Fatalf("sealed repair operation count = %d, want %d", len(repairOperations), len(requiredSequence))
	}
	repairEvidence := func(operation runtimecontract.RuntimeOperation, eventID string) ExecutionEvidence {
		return ExecutionEvidence{
			EventID: eventID, WorkspaceID: sealedRepairDispatch.Facts.WorkspaceID, RunID: sealedRepairDispatch.Facts.RunID,
			DispatchID: sealedRepairDispatch.ActiveDispatch.ID, AuthoritySnapshotID: sealedRepairDispatch.ActiveDispatch.Transaction.AuthoritySnapshotID,
			OperationID: operation.ID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
			Evidence: runtimecontract.RuntimeOperationEvidence{Quality: common.EvidenceQualityValid},
		}
	}
	firstRepairEvidence := repairEvidence(repairOperations[0], "event-validation-repair-state-change-1")
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &firstRepairEvidence)
	repairCoordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	partial, err := repairCoordinator.commitEvidence(context.Background(), firstRepairEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if partial.DispatchCompleted || partial.Aggregate.ActiveDispatch == nil ||
		partial.Aggregate.Facts.Coverage[0].CurrentBindingRef == nil ||
		runtimeValidationFailureRepairReopenReason(partial.Aggregate.Facts.Coverage[0]) != runtimeCWIReopenReasonValidationFailureRepairRequired {
		t.Fatalf("partial multi-target repair receipt settled generation: %+v", partial.Aggregate)
	}
	partialRevision := partial.Aggregate.StateRevision
	restartedCoordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	partialDuplicate, err := restartedCoordinator.commitEvidence(context.Background(), firstRepairEvidence)
	if err != nil || partialDuplicate.Disposition != CommitDispositionDuplicate || partialDuplicate.Aggregate.StateRevision != partialRevision {
		t.Fatalf("partial multi-target repair duplicate changed authority: result=%+v err=%v", partialDuplicate, err)
	}
	staleRepairEvidence := ExecutionEvidence{
		EventID: "event-validation-repair-state-change-stale", WorkspaceID: repository.aggregate.Facts.WorkspaceID, RunID: repository.aggregate.Facts.RunID,
		DispatchID: repository.aggregate.ActiveDispatch.ID, AuthoritySnapshotID: "stale-repair-authority",
		OperationID: repairOperations[1].ID, Kind: runtimecontract.EvidenceKindStateChange, Status: ExecutionEvidenceStatusAccepted,
	}
	stale, err := restartedCoordinator.commitEvidence(context.Background(), staleRepairEvidence)
	if err != nil || stale.Disposition != CommitDispositionStale || stale.Aggregate.ActiveDispatch == nil ||
		stale.Aggregate.Facts.Repair.RepairAttempts != 1 || len(stale.Aggregate.Facts.Coverage[0].ReopenLineage) != 1 {
		t.Fatalf("stale multi-target repair evidence changed generation: result=%+v err=%v", stale, err)
	}
	secondRepairEvidence := repairEvidence(repairOperations[1], "event-validation-repair-state-change-2")
	// The active dispatch is persisted after the first receipt; attach the
	// second owner fact to that restarted aggregate before completing all-of.
	secondRepairEvidence.DispatchID = repository.aggregate.ActiveDispatch.ID
	secondRepairEvidence.AuthoritySnapshotID = repository.aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &secondRepairEvidence)
	settled, err := restartedCoordinator.commitEvidence(context.Background(), secondRepairEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if !settled.DispatchCompleted || settled.Aggregate.ActiveDispatch != nil {
		t.Fatalf("complete multi-target repair dispatch remained active: %+v", settled.Aggregate)
	}
	settledRevision := settled.Aggregate.StateRevision
	restartedAfterSettlement, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	settledDuplicate, err := restartedAfterSettlement.commitEvidence(context.Background(), secondRepairEvidence)
	if err != nil || settledDuplicate.Disposition != CommitDispositionDuplicate || settledDuplicate.Aggregate.StateRevision != settledRevision {
		t.Fatalf("complete multi-target repair duplicate reopened another generation: result=%+v err=%v", settledDuplicate, err)
	}
	item := settled.Aggregate.Facts.Coverage[0]
	if item.State != runtimecontract.RuntimeCoverageItemStateOpen || item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened ||
		item.CurrentBindingRef != nil || runtimeValidationFailureRepairReopenReason(item) != runtimeCWIReopenReasonValidationFailureRepairSettled ||
		settled.Aggregate.Facts.RepairDirective == nil || settled.Aggregate.Facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation ||
		settled.Aggregate.Facts.Repair.LastProgressRevision != "" {
		t.Fatalf("repair receipt did not automatically reopen fresh validation without terminal credit: %+v", settled.Aggregate.Facts)
	}
	if len(settled.Aggregate.Facts.Evidence.Accepted) == 0 || len(settled.Aggregate.Facts.Evidence.Accepted[len(settled.Aggregate.Facts.Evidence.Accepted)-1].CoverageWorkItemIDs) != 1 {
		t.Fatalf("repair receipt audit fact was not retained: %+v", settled.Aggregate.Facts.Evidence.Accepted)
	}
	if _, required, err := CurrentRuntimeValidationFailureRepairGenerationInput(settled.Aggregate); err != nil || required {
		t.Fatalf("settled repair still selected repair-purpose generation: required=%v err=%v", required, err)
	}
	if len(settled.Aggregate.SemanticPlanning.History) == 0 ||
		settled.Aggregate.SemanticPlanning.History[len(settled.Aggregate.SemanticPlanning.History)-1].Snapshot.Manifest.Hash != repairManifest.Hash {
		t.Fatalf("repair manifest was not retained as immutable binding lineage: %+v", settled.Aggregate.SemanticPlanning)
	}

	// Restart at the fresh-validation boundary and seal the original Definition
	// proof, not another repair-purpose manifest.
	freshManifest, freshProposal, freshFeasibility := runtimeValidationFailureFreshValidationProofForTest(t, settled.Aggregate)
	sealedFreshValidation := runtimeSealValidationFailureRepairSemanticProofForTest(t, repository, freshManifest, freshProposal, freshFeasibility)
	if sealedFreshValidation.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
		var snapshotID string
		for _, prerequisite := range sealedFreshValidation.Facts.Prerequisites.Pending {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.SubjectRef == "main.go" {
				snapshotID = prerequisite.ID
				break
			}
		}
		if snapshotID == "" {
			t.Fatalf("fresh validation target prerequisite is not a typed File snapshot: %+v", sealedFreshValidation.Facts.Prerequisites.Pending)
		}
		controller, controllerErr := NewRuntimeController(repository)
		if controllerErr != nil {
			t.Fatal(controllerErr)
		}
		sealedFreshValidation, err = controller.commitPrerequisite(context.Background(), sealedFreshValidation.Facts.WorkspaceID, sealedFreshValidation.Facts.RunID, PrerequisiteResult{
			PrerequisiteID: snapshotID,
			TargetSnapshot: &runtimecontract.TargetSnapshot{TargetRef: "main.go", Content: "package main\n", ContentHash: "main-after-repair-v1", ContentState: runtimecontract.SnapshotContentStateComplete},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, repairPurpose, err := currentRuntimeSemanticValidationFailureRepair(sealedFreshValidation); err != nil || repairPurpose {
		t.Fatalf("fresh Definition binding retained repair purpose: found=%v err=%v", repairPurpose, err)
	}
	freshBinding, found := coverageEvidenceBindingForCoverage(sealedFreshValidation.Facts.Focus, "cwi-1")
	if !found {
		t.Fatalf("fresh validation binding missing: %+v", sealedFreshValidation.Facts.Focus)
	}
	validationProof, found := runtimecontract.CoverageEvidenceProofForKind(freshBinding, runtimecontract.EvidenceKindValidation)
	if !found || validationProof.ValidationRecipeID != "test-managed-validation" ||
		sealedFreshValidation.Facts.RepairDirective == nil || sealedFreshValidation.Facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("fresh Definition proof was not restored: binding=%+v directive=%+v", freshBinding, sealedFreshValidation.Facts.RepairDirective)
	}
	if sealedFreshValidation.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("fresh Definition seal selected %s, pending=%+v", sealedFreshValidation.Decision.State, sealedFreshValidation.Facts.Prerequisites.Pending)
	}

	recipeID := validationProof.ValidationRecipeID
	secondValidationCandidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-repair-cycle-2", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		BindingRefs: []string{freshBinding.ID}, TargetRefs: []string{"main.go"},
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}}}}}
	sealedSecondValidation := sealCandidateForTest(t, repository, secondValidationCandidate)
	secondRejection := runtimeRejectedValidationEvidenceForTest(t, sealedSecondValidation, "event-validation-repair-cycle-2", runtimecontract.ValidationDiagnosticBuildFailed)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &secondRejection)
	historyCount := len(repository.aggregate.SemanticPlanning.History)
	sessionCount := len(repository.aggregate.CandidateRequestSessions)
	receiptCount := len(repository.aggregate.OperationReceipts)
	secondCoordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	exhausted, err := secondCoordinator.commitEvidence(context.Background(), secondRejection)
	if err != nil {
		t.Fatal(err)
	}
	if !exhausted.Aggregate.Facts.Repair.Exhausted || exhausted.Aggregate.Facts.Repair.RepairAttempts != 2 ||
		exhausted.Aggregate.Facts.Failure == nil || exhausted.Aggregate.Facts.Failure.Code != "validation_failure_repair_exhausted" ||
		exhausted.Aggregate.Decision.Terminal == nil || exhausted.Aggregate.DefinitionClarification != nil ||
		exhausted.Aggregate.SemanticPlanning.Manifest == nil || exhausted.Aggregate.SemanticPlanning.Manifest.Hash != freshManifest.Hash ||
		len(exhausted.Aggregate.SemanticPlanning.History) != historyCount || len(exhausted.Aggregate.CandidateRequestSessions) != sessionCount ||
		len(exhausted.Aggregate.OperationReceipts) != receiptCount {
		t.Fatalf("post-budget validation did not terminate in the same CAS without repair side effects: %+v", exhausted.Aggregate)
	}

	// Duplicate delivery after restart cannot count a third cycle, retire a
	// semantic generation, or emit another owner/provider effect.
	exhaustedRevision := exhausted.Aggregate.StateRevision
	restartedAfterExhaustion, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := restartedAfterExhaustion.commitEvidence(context.Background(), secondRejection)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != exhaustedRevision ||
		duplicate.Aggregate.Facts.Repair.RepairAttempts != 2 || len(duplicate.Aggregate.SemanticPlanning.History) != historyCount ||
		len(duplicate.Aggregate.CandidateRequestSessions) != sessionCount || len(duplicate.Aggregate.OperationReceipts) != receiptCount {
		t.Fatalf("duplicate exhausted validation changed authority/effects: result=%+v err=%v", duplicate, err)
	}
}

func TestValidationFailureRepairTriggerFailsClosedAndExactFailureTargetRetainsPriority(t *testing.T) {
	convergence := runtimeValidationFailureRepairConvergenceForTest(1)
	base := runtimeValidationFailureRepairAggregateForTest(t, convergence)
	operation := runtimecontract.RuntimeOperation{ID: "validate-1", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}
	fact := runtimecontract.EvidenceFact{
		ID: "rejected-validation", Kind: runtimecontract.EvidenceKindValidation, OperationID: operation.ID,
		FailureSignature: "validation-failure/typed/1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "runtime-evidence-1",
	}
	original := runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationID: operation.ID, OriginalClass: "operation", OriginalCode: "validation_failed", OriginalMessage: "failed", Signature: fact.FailureSignature,
	}
	typedFailure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassBuildFailure, Code: "validation_failed", Signature: fact.FailureSignature, Message: "failed",
		OriginalDiagnostic: &original, ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{
			Class: runtimecontract.ValidationDiagnosticBuildFailed, RecommendedEvidenceTargetType: runtimecontract.ValidationEvidenceTargetFailureSurface,
			OwnerCode: "shell_exit_nonzero",
		},
	}

	t.Run("exact owner target", func(t *testing.T) {
		aggregate := base
		aggregate.Facts.Failure = &typedFailure
		aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: fact.FailureSignature, TargetRefs: []string{"owner-grounded.go"}}
		if _, matched, err := matchRuntimeValidationFailureRepairTrigger(aggregate, fact, operation); err != nil || matched {
			t.Fatalf("Profile fallback displaced exact FailureTarget: matched=%v err=%v", matched, err)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(*runtimecontract.FailureFact)
	}{
		{name: "raw diagnostic", mutate: func(value *runtimecontract.FailureFact) { value.ValidationDiagnostic = nil }},
		{name: "unmatched class", mutate: func(value *runtimecontract.FailureFact) {
			value.ValidationDiagnostic.Class = runtimecontract.ValidationDiagnosticTimeout
		}},
		{name: "dependency contract has no implementation repair mapping", mutate: func(value *runtimecontract.FailureFact) {
			value.ValidationDiagnostic.Class = runtimecontract.ValidationDiagnosticDependencyMismatch
		}},
		{name: "unmatched owner", mutate: func(value *runtimecontract.FailureFact) {
			value.OriginalDiagnostic.OwnerModuleID = runtimecontract.RuntimeModuleMCP
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			aggregate := base
			failure := typedFailure.Clone()
			tc.mutate(&failure)
			aggregate.Facts.Failure = &failure
			if _, matched, err := matchRuntimeValidationFailureRepairTrigger(aggregate, fact, operation); err != nil || matched {
				t.Fatalf("invalid typed trigger opened repair: matched=%v err=%v failure=%+v", matched, err, failure)
			}
			if aggregate.DefinitionClarification != nil {
				t.Fatalf("failed-closed Profile mismatch created clarification: %+v", aggregate.DefinitionClarification)
			}
		})
	}
}

func TestValidationFailureRepairMultiTargetSettlementUsesOneCompletedAllOfDispatch(t *testing.T) {
	repository, coordinator, operations := prospectiveAllOfDispatchFixture(t, true)
	binding := repository.aggregate.Facts.Focus.CoverageEvidenceBindings[0].Clone()
	if len(binding.ProspectiveTargetRefs) != 2 {
		t.Fatalf("repair collective fixture binding = %+v", binding)
	}

	first := prospectiveAllOfEvidence(repository.aggregate, operations[0], "repair-collective-first", ExecutionEvidenceStatusAccepted, nil)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &first)
	partial, err := coordinator.commitEvidence(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if partial.DispatchCompleted || partial.Aggregate.ActiveDispatch == nil {
		t.Fatalf("partial repair dispatch completed: %+v", partial.Aggregate)
	}
	if dispatchID, complete := runtimeProspectiveCoverageKindComplete(
		partial.Aggregate, binding, "cwi-1", runtimecontract.EvidenceKindStateChange, partial.Aggregate.Facts.Evidence.Accepted,
	); complete || dispatchID != "" {
		t.Fatalf("partial repair receipt set settled: dispatch=%q complete=%v", dispatchID, complete)
	}

	// Restart reconstructs collective completeness exclusively from the
	// persisted operation executions, owner receipts and accepted evidence.
	restarted, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	revision := repository.aggregate.StateRevision
	duplicate, err := restarted.commitEvidence(context.Background(), first)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != revision {
		t.Fatalf("partial repair duplicate after restart = %+v err=%v", duplicate, err)
	}

	second := prospectiveAllOfEvidence(repository.aggregate, operations[1], "repair-collective-second", ExecutionEvidenceStatusAccepted, nil)
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &second)
	completed, err := restarted.commitEvidence(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	if !completed.DispatchCompleted || completed.Aggregate.ActiveDispatch != nil || len(completed.Aggregate.CompletedDispatches) != 1 {
		t.Fatalf("complete repair dispatch was not persisted: %+v", completed.Aggregate)
	}
	lastFact := completed.Aggregate.Facts.Evidence.Accepted[len(completed.Aggregate.Facts.Evidence.Accepted)-1]
	dispatchID, complete := runtimeProspectiveCoverageKindComplete(
		completed.Aggregate, binding, "cwi-1", runtimecontract.EvidenceKindStateChange, completed.Aggregate.Facts.Evidence.Accepted,
	)
	if !complete || dispatchID != lastFact.DispatchID {
		t.Fatalf("complete repair all-of set = dispatch=%q complete=%v last=%+v", dispatchID, complete, lastFact)
	}

	repair := runtimeSemanticValidationFailureRepairForTest(runtimecontract.RuntimeBlockKindPatchApply, 2)
	installRuntimeValidationFailureRepairDirectiveForTest(t, &completed.Aggregate, repair, binding, lastFact.Revision)
	settled, err := settleRuntimeValidationFailureRepair(&completed.Aggregate, lastFact, repair)
	if err != nil {
		t.Fatal(err)
	}
	if !settled || completed.Aggregate.Facts.Coverage[0].CurrentBindingRef != nil ||
		completed.Aggregate.Facts.Coverage[0].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened ||
		runtimeValidationFailureRepairReopenReason(completed.Aggregate.Facts.Coverage[0]) != runtimeCWIReopenReasonValidationFailureRepairSettled ||
		completed.Aggregate.Facts.RepairDirective == nil || completed.Aggregate.Facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation ||
		completed.Aggregate.Facts.RepairDirective.Purpose != "" ||
		completed.Aggregate.Facts.RepairDirective.Closure == nil || len(completed.Aggregate.Facts.RepairDirective.TargetRefs) != 0 ||
		completed.Aggregate.Facts.Repair.LastProgressRevision != "" {
		t.Fatalf("complete repair all-of set did not reopen for fresh validation: %+v", completed.Aggregate.Facts)
	}

	// The final owner event is still idempotent at the normal event ingress; a
	// second completed-dispatch event cannot create another repair generation.
	repository.aggregate = completed.Aggregate
	restartedAfterSettlement, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	settledRevision := repository.aggregate.StateRevision
	completedDuplicate, err := restartedAfterSettlement.commitEvidence(context.Background(), second)
	if err != nil || completedDuplicate.Disposition != CommitDispositionDuplicate || completedDuplicate.Aggregate.StateRevision != settledRevision ||
		len(completedDuplicate.Aggregate.Facts.Coverage[0].ReopenLineage) != 1 {
		t.Fatalf("complete repair duplicate changed fresh-validation reopen: %+v err=%v", completedDuplicate, err)
	}
}

func runtimeSemanticValidationFailureRepairForTest(operationKind string, maxAttempts int) runtimecontract.RuntimeSemanticValidationFailureRepair {
	convergence := runtimeValidationFailureRepairConvergenceForTest(maxAttempts)
	for index := range convergence.Routes {
		if convergence.Routes[index].TriggerDiagnosticClass == runtimecontract.ValidationDiagnosticBuildFailed {
			convergence.Routes[index].RepairOperationKinds = []string{operationKind}
		}
	}
	route, found := convergence.RouteForTrigger(
		runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeBlockKindManagedValidationSession,
		runtimecontract.ValidationDiagnosticBuildFailed, runtimecontract.ValidationEvidenceTargetFailureSurface, "shell_exit_nonzero",
	)
	if !found {
		panic("test validation repair route is invalid")
	}
	return runtimecontract.RuntimeSemanticValidationFailureRepair{
		FailedFocusID: "definition-validation-repair/cwi-1", ConvergenceID: convergence.ID,
		ConvergenceHash: runtimecontract.StableRuntimeValidationFailureRepairConvergenceHash(convergence),
		RouteID:         route.ID, RouteHash: runtimecontract.StableRuntimeValidationFailureRepairRouteHash(route),
		FailureSignature: "validation-failure/typed/1", TriggerOwnerModuleID: route.TriggerOwnerModuleID,
		TriggerOperationKind: route.TriggerOperationKind, TriggerDiagnosticClass: route.TriggerDiagnosticClass,
		TriggerEvidenceTargetType: route.TriggerEvidenceTargetType, TriggerOwnerDiagnosticCode: route.TriggerOwnerDiagnosticCode,
		RepairEvidenceKind: route.RepairEvidenceKind, RepairOwnerModuleID: route.RepairOwnerModuleID,
		RepairOperationKinds: append([]string(nil), route.RepairOperationKinds...), RepairTargetPolicy: route.RepairTargetPolicy,
		RepairTargetRole:                          route.RepairTargetRole,
		RepairDeclarationTargetSelectorRef:        route.RepairDeclarationTargetSelectorRef,
		RepairDeclarationTargetSelectorOwnerRef:   route.RepairDeclarationTargetSelectorOwnerRef,
		RepairDeclarationTargetSelectorSubjectRef: route.RepairDeclarationTargetSelectorSubjectRef,
		RepairOperationTargetSelectorRef:          route.RepairOperationTargetSelectorRef,
		RepairConstraintRefs:                      []string{"workspace-relative-path"}, OwnerCatalogSnapshotID: "file-repair-catalog",
		OwnerCatalogObservedRevision: "file-repair-revision", RequireFreshValidation: true, MaxAttempts: maxAttempts,
	}.Normalized()
}

func installRuntimeValidationFailureRepairDirectiveForTest(
	t *testing.T,
	aggregate *RuntimeAggregate,
	repair runtimecontract.RuntimeSemanticValidationFailureRepair,
	binding runtimecontract.CoverageEvidenceBinding,
	baselineRevision string,
) {
	t.Helper()
	if err := repair.Validate(); err != nil {
		t.Fatalf("repair fixture contract: %v", err)
	}
	failure := runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassBuildFailure, Code: "validation_failed", Signature: repair.FailureSignature, Message: "typed validation failed",
		ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{
			Class: repair.TriggerDiagnosticClass, RecommendedEvidenceTargetType: repair.TriggerEvidenceTargetType, OwnerCode: repair.TriggerOwnerDiagnosticCode,
		},
	}
	closure := runtimecontract.RuntimeRepairClosure{
		GapSignature: repair.FailureSignature, BaselineEvidenceRevision: baselineRevision,
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{binding.CoverageWorkItemID},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		GroundedTargetRefs:    append([]string(nil), binding.ProspectiveTargetRefs...), RequireValidationAfterStateChange: true,
	}
	closure.ID = runtimecontract.StableRuntimeRepairClosureID(closure)
	directive := runtimecontract.RuntimeRepairDirective{
		ID: "validation-failure-repair/test", FocusID: aggregate.Facts.Focus.ID,
		Trigger: runtimecontract.RuntimeRepairTriggerRejectedValidation, CurrentFailure: &failure,
		RequiredState:         runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AllowedOperationKinds: append([]string(nil), repair.RepairOperationKinds...),
		CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID}, TargetRefs: append([]string(nil), binding.ProspectiveTargetRefs...),
		Closure: &closure, SupersedesEvidenceRevision: baselineRevision, RepairAttempt: 1,
	}.Normalized()
	if err := directive.Validate(); err != nil {
		t.Fatalf("repair fixture directive: %v", err)
	}
	aggregate.Facts.Failure = &failure
	aggregate.Facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: aggregate.Facts.Focus.ID, SliceID: aggregate.Facts.Focus.SliceID, RepairAttempts: 1,
		LastCycleID: "repair-cycle-1", ActiveGap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID}, FailureSignature: repair.FailureSignature,
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, Reason: "typed validation failed",
		},
	}.Normalized()
	aggregate.Facts.RepairDirective = &directive
}
