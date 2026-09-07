package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// TestShellOwnerObservedFailureTargetBindsThroughEventHubAndFileSnapshot
// exercises the real Shell owner boundary for the closed failure-target
// protocol. A rejected validation may retarget SM-2 only after the owner
// receipt is CAS-committed and the File owner confirms the diagnostic
// candidate; the resulting target must then be snapshotted before it can enter
// another candidate authority.
func TestShellOwnerObservedFailureTargetBindsThroughEventHubAndFileSnapshot(t *testing.T) {
	for _, entry := range []string{"implementation.go", "validation_entry_test.go"} {
		t.Run(entry, func(t *testing.T) {
			t.Run("fresh validation succeeds", func(t *testing.T) { testObservedShellRepair(t, entry, false, true) })
			t.Run("second repair succeeds", func(t *testing.T) { testObservedShellRepair(t, entry, true, true) })
			t.Run("second repair exhausts budget", func(t *testing.T) { testObservedShellRepair(t, entry, true, false) })
		})
	}
}

func testObservedShellRepair(t *testing.T, validationEntry string, failsAgain, secondSucceeds bool) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })

	workspaceDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "go.mod"), []byte("module example.com/workorch-observed-target\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The Shell owner derives the repair candidate from its native Go
	// diagnostic. Raw stdout markers are deliberately not proof authority.
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "implementation.go"), []byte("package fixture\n\nvar _ = workorchMissingImplementation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "validation_entry_test.go"), []byte("package fixture\n\nimport \"testing\"\n\nfunc TestValidationEntry(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "validation-entry.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	fileOwner, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fileOwner.Teardown(ctx) })
	shellOwner, err := shellbiz.New(hub, nil, shellbiz.Dependencies{RuntimeAdapter: shellbiz.NewRuntimeAdapter()})
	if err != nil {
		t.Fatal(err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shellOwner.Teardown(ctx) })

	policy := runtimeManagedValidationEvidencePolicy()
	policy.CommandName = "go"
	policy.RecipeArgs = []string{"test", "-v", "./..."}
	ownerPreflight := validationOnlyAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Focus.RequiredCheckRefs = []string{"workorch-cwi-check-observedfailuretarget"}
	})
	ownerPreflight.Facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{policy}
	shellCatalog := addObservedShellFailureTargetFreshness(t, ctx, hub, &ownerPreflight, workspaceDirectory, false)
	if len(shellCatalog.SelectorRefs) == 0 {
		t.Fatalf("Shell observed-target catalog has no owner selector: %+v", shellCatalog)
	}
	shellSelectorRef := shellCatalog.SelectorRefs[0]
	aggregate := validationOnlyAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{policy}
		aggregate.Facts.Focus.RequiredCheckRefs = []string{"workorch-cwi-check-observedfailuretarget"}
		aggregate.Facts.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus = 2
		aggregate.Definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindManagedValidationSession}
		aggregate.Facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
		aggregate.Facts.Focus.AcceptedStepKinds = []string{
			runtimecontract.RuntimeBlockKindPatchApply,
			runtimecontract.RuntimeBlockKindManagedValidationSession,
		}
		aggregate.Facts.Focus.TargetRefs = nil
		aggregate.Facts.Focus.TargetRole = "implementation"
		aggregate.Facts.Focus.TargetSelectorRef = shellSelectorRef
		aggregate.Facts.Focus.TargetSelectorOwnerRef = runtimecontract.RuntimeModuleShell
		aggregate.Facts.Focus.TargetSelectorSubjectRef = "implementation"
		aggregate.Facts.Focus.TargetSelectorCandidateRefs = []string{validationEntry}
		aggregate.Facts.Focus.RequireTargetGrounding = true
		runtimeTestInstallObservedShellPreflight(aggregate, ownerPreflight, shellCatalog)
		route := runtimeValidationFailureRepairRouteForTest("observed-build-repair", runtimecontract.ValidationDiagnosticBuildFailed, runtimecontract.ValidationEvidenceTargetFailureSurface)
		route.TriggerOwnerDiagnosticCode = "source_compile_failed"
		route.RepairOperationKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
		route.RepairTargetPolicy = runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope
		route.RepairOperationTargetSelectorRef = "workspace-relative-existing-file-target"
		convergence := runtimecontract.RuntimeValidationFailureRepairConvergence{ID: "observed-validation-repair", Routes: []runtimecontract.RuntimeValidationFailureRepairRoute{route}, RequireFreshValidation: true, MaxAttempts: 2}
		declaration := runtimecontract.CompileRuntimeDefinitionFocuses([]runtimecontract.CoverageFocus{aggregate.Facts.Focus})[0]
		declaration.ValidationFailureRepairConvergence = &convergence
		aggregate.Facts.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{declaration}
	})
	shellCatalog = runtimeTestOperationAdmissibilityForOwner(t, aggregate.Facts, runtimecontract.RuntimeModuleShell)
	// Replace synthetic File preflight with current facts from the real owner.
	currentCatalogs := aggregate.Facts.Resources.OperationAdmissibilities[:0]
	for _, catalog := range aggregate.Facts.Resources.OperationAdmissibilities {
		if catalog.Normalized().OwnerModuleID != runtimecontract.RuntimeModuleFile {
			currentCatalogs = append(currentCatalogs, catalog)
		}
	}
	aggregate.Facts.Resources.OperationAdmissibilities = currentCatalogs
	currentObservations := aggregate.Facts.Resources.PrerequisiteObservations[:0]
	for _, observation := range aggregate.Facts.Resources.PrerequisiteObservations {
		if observation.Normalized().OwnerModuleID != runtimecontract.RuntimeModuleFile || observation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility {
			currentObservations = append(currentObservations, observation)
		}
	}
	aggregate.Facts.Resources.PrerequisiteObservations = currentObservations
	currentSatisfied := aggregate.Facts.Prerequisites.Satisfied[:0]
	for _, prerequisite := range aggregate.Facts.Prerequisites.Satisfied {
		if strings.TrimSpace(prerequisite.OwnerModuleID) != runtimecontract.RuntimeModuleFile || prerequisite.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility {
			currentSatisfied = append(currentSatisfied, prerequisite)
		}
	}
	aggregate.Facts.Prerequisites.Satisfied = currentSatisfied
	postStateManifestPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: "manifest:shell-observed-target-post-state-change", Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	postStateManifest := sendObservedManifest(t, ctx, hub, aggregate, postStateManifestPrerequisite, workspaceDirectory)
	aggregate.Facts.Resources.Manifest = &postStateManifest.Manifest
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, postStateManifest.Observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, postStateManifestPrerequisite)
	filePrerequisite := runtimecontract.RuntimePrerequisite{ID: "confirmed-edit-admissibility", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply}}
	initialFileCatalog := sendObservedShellFileAdmissibility(t, ctx, hub, aggregate, filePrerequisite, workspaceDirectory)
	aggregate.Facts.Resources.OperationAdmissibilities = append(aggregate.Facts.Resources.OperationAdmissibilities, initialFileCatalog.Acquisition.Admissibility)
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, initialFileCatalog.Acquisition.Observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, filePrerequisite)
	if err := aggregate.Validate(); err != nil {
		t.Fatalf("install post-state-change File manifest: %v", err)
	}
	decision, err := runtimecontract.SealRuntimeDecision(aggregate.Facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedValidation,
		Focus: aggregate.Facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession},
			CoverageWorkItemIDs:  aggregate.Facts.Focus.CoverageWorkItemIDs,
			TargetRefs:           append([]string(nil), aggregate.Facts.Focus.TargetRefs...),
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	aggregate.Decision = decision
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	registerRuntimeOperationReceiptOwner(t, hub, controller)

	recipeID := policy.ID
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-with-observed-target", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		TargetRefs:          []string{validationEntry},
		Parameters:          []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}}}}}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	canonicalOperation := candidate.Validation.Operations[0].Operation
	sealedAggregate := sealCandidateWithFeasibilitiesForTest(t, repository, candidate, []runtimecontract.RuntimeCandidateFeasibilityRecord{{
		OperationID: canonicalOperation.ID, AdmissibilitySnapshotID: shellCatalog.SnapshotID,
		AcquisitionBinding: shellCatalog.AcquisitionBinding,
		Observation:        &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
	}})
	sealedExecution := sealedAggregate.ActiveDispatch.OperationExecutions[0]
	sealedOperations, err := orderedCandidateOperations(sealedAggregate.ActiveDispatch.Candidate)
	if err != nil || len(sealedOperations) != 1 {
		t.Fatalf("sealed validation operations = %+v err=%v", sealedOperations, err)
	}
	sealedOperation := sealedOperations[0]
	sealedValidationScope, err := runtimeValidationScopeTargetRefs(*sealedAggregate.ActiveDispatch, sealedOperation)
	if err != nil || !slices.Equal(sealedValidationScope, shellCatalog.TargetRefs) || len(sealedValidationScope) <= 1 ||
		len(sealedOperation.TargetRefs) != 1 || sealedOperation.TargetRefs[0] != validationEntry {
		t.Fatalf("exact validation seal = entry=%v scope=%v err=%v", sealedOperation.TargetRefs, sealedValidationScope, err)
	}
	if selected := runtimeOwnerCatalogForOperation(
		sealedAggregate.Facts.Resources.OperationAdmissibilities,
		sealedAggregate.Facts.Resources.PrerequisiteObservations,
		candidate.Validation.Operations[0].Operation,
		sealedExecution.AdmissibilitySnapshotID,
		sealedExecution.AcquisitionBinding,
	); selected == nil {
		t.Fatalf("sealed Shell execution cannot recover exact owner catalog: execution=%+v catalog=%+v", sealedExecution, shellCatalog)
	}
	persisted, err := controller.executeActiveDispatch(ctx, "ws-1", "run-1", runtimeObservedShellExecutor{hub: hub, deliverableDirectory: workspaceDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Facts.Failure == nil || persisted.Facts.Failure.ValidationFailureObservation == nil ||
		persisted.Facts.Resources.FailureTarget != nil {
		t.Fatalf("validation-owner receipt bypassed File repair-surface confirmation: failure=%+v target=%+v", persisted.Facts.Failure, persisted.Facts.Resources.FailureTarget)
	}
	receipt := runtimecontract.RuntimeOperationReceipt{}
	found := false
	for _, candidateReceipt := range persisted.OperationReceipts {
		if candidateReceipt.ExecutionKey == sealedExecution.ExecutionKey {
			receipt, found = candidateReceipt, true
			break
		}
	}
	if !found {
		t.Fatalf("Shell failure receipt is missing: execution=%+v receipts=%+v", sealedExecution, persisted.OperationReceipts)
	}
	receipt = receipt.Normalized()
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.Evidence.ZeroWork ||
		receipt.Evidence.Quality == common.EvidenceQualityInconclusive || receipt.Evidence.EvidenceReason == "configured zero-work marker matched" ||
		len(receipt.Evidence.EvidenceTags) != 0 {
		t.Fatalf("mixed aggregate failure receipt = %+v, want rejected non-zero-work evidence", receipt)
	}
	if receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil ||
		receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic == nil ||
		receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic.Class != runtimecontract.ValidationDiagnosticBuildFailed {
		t.Fatalf("mixed aggregate failure diagnostic = %+v, want validation build failure", receipt.Feedback)
	}
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	persistedReceiptHash := ""
	for _, dispatch := range persisted.CompletedDispatches {
		if dispatch.ID != sealedAggregate.ActiveDispatch.ID {
			continue
		}
		for _, execution := range dispatch.OperationExecutions {
			if execution.ExecutionKey == receipt.ExecutionKey && execution.Fence == receipt.Fence {
				persistedReceiptHash = execution.OwnerReceiptHash
			}
		}
	}
	if receiptHash == "" || persistedReceiptHash != receiptHash {
		t.Fatalf("Shell owner receipt hash was not exact-persisted: receipt=%+v dispatches=%+v", receipt, persisted.CompletedDispatches)
	}
	failureObservation := persisted.Facts.Failure.ValidationFailureObservation.Clone()
	if !slices.Equal(failureObservation.CandidateTargetRefs, []string{"implementation.go"}) ||
		!slices.Equal(failureObservation.ValidationScopeTargetRefs, sealedValidationScope) {
		t.Fatalf("Shell observation lost exact sealed scope: %+v", failureObservation)
	}
	if _, err := controller.recordValidationFailureObservation(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, failureObservation); err != nil {
		t.Fatalf("validation observation identity fence: %v", err)
	}
	// The semantic helper snapshots only the pre-validation target set. The
	// later owner confirmation is a new owner revision and must not inherit those
	// fixture snapshots as repair authority.
	repository.aggregate.Facts.Resources.TargetSnapshots = nil
	persisted.Facts.Resources.TargetSnapshots = nil
	confirmationPlan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(persisted)
	if err != nil || !confirmationPlan.Required() {
		t.Fatalf("repair-surface confirmation plan = %+v err=%v", confirmationPlan, err)
	}
	confirmationCommand := fileevents.ConfirmRuntimeRepairSurfaceCommand{
		RequestID: confirmationPlan.RequestID, IssuedAtStateRevision: confirmationPlan.IssuedAtStateRevision, ExpectedSubjectRevision: confirmationPlan.ExpectedSubjectRevision,
		WorkspaceID: persisted.Facts.WorkspaceID, RunID: persisted.Facts.RunID,
		WorkspaceDeliverableDirectory: workspaceDirectory, Observation: failureObservation,
	}
	oldConfirmationResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicConfirmRuntimeRepairSurface, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx, confirmationCommand,
	))
	if oldConfirmationResult == nil {
		t.Fatal("File owner returned no repair-surface confirmation")
	}
	oldConfirmationData, oldConfirmationErr := oldConfirmationResult.Get()
	if oldConfirmationErr != nil {
		t.Fatalf("File current repair-surface confirmation error: %v", oldConfirmationErr)
	}
	oldConfirmationResponse, ok := oldConfirmationData.(fileevents.ConfirmRuntimeRepairSurfaceResult)
	if !ok || oldConfirmationResponse.Confirmation.Disposition != runtimecontract.RuntimeRepairSurfaceConfirmed ||
		oldConfirmationResponse.Confirmation.ObservedRevision != confirmationPlan.ExpectedSubjectRevision {
		t.Fatalf("File current repair-surface confirmation = %#v", oldConfirmationData)
	}
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "concurrent-change.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleConfirmationResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicConfirmRuntimeRepairSurface, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx, confirmationCommand,
	))
	if staleConfirmationResult == nil {
		t.Fatal("File owner returned no stale repair-surface confirmation")
	}
	staleConfirmationData, staleConfirmationErr := staleConfirmationResult.Get()
	if staleConfirmationErr != nil {
		t.Fatalf("File stale repair-surface confirmation error: %v", staleConfirmationErr)
	}
	staleConfirmationResponse, ok := staleConfirmationData.(fileevents.ConfirmRuntimeRepairSurfaceResult)
	if !ok || staleConfirmationResponse.Confirmation.Disposition != runtimecontract.RuntimeRepairSurfaceStale ||
		staleConfirmationResponse.Confirmation.ExpectedSubjectRevision != confirmationPlan.ExpectedSubjectRevision ||
		staleConfirmationResponse.Confirmation.ObservedRevision == confirmationPlan.ExpectedSubjectRevision ||
		len(staleConfirmationResponse.Confirmation.ConfirmedTargetRefs) != 0 {
		t.Fatalf("File stale repair-surface confirmation = %#v", staleConfirmationData)
	}
	persisted, err = controller.recordRepairSurfaceConfirmation(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, staleConfirmationResponse.Confirmation)
	if err != nil || persisted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(persisted.PrerequisiteInvalidations) == 0 || persisted.Facts.Recovery.PrerequisiteRefreshes == 0 {
		t.Fatalf("stale File confirmation did not atomically reopen owner prerequisites: aggregate=%+v err=%v", persisted, err)
	}
	persisted = reacquireObservedShellRepairPrerequisites(t, ctx, hub, controller, persisted, workspaceDirectory)
	if persisted.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("stale File prerequisite refresh did not return to validation: %+v", persisted.Decision)
	}
	reopenedPlan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(persisted)
	if err != nil || !reopenedPlan.Required() || reopenedPlan.RequestID == confirmationPlan.RequestID ||
		reopenedPlan.ExpectedSubjectRevision == confirmationPlan.ExpectedSubjectRevision {
		t.Fatalf("stale File refresh did not issue a new revision-bound plan: old=%+v new=%+v err=%v", confirmationPlan, reopenedPlan, err)
	}
	beforeLateRevision := persisted.StateRevision
	if _, err := controller.recordRepairSurfaceConfirmation(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, oldConfirmationResponse.Confirmation); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("old File confirmation crossed reopened request window: %v", err)
	}
	if repository.aggregate.StateRevision != beforeLateRevision || len(repository.aggregate.RepairSurfaceConfirmations) != 1 {
		t.Fatalf("old File confirmation changed reopened authority: %+v", repository.aggregate)
	}
	confirmationCommand.RequestID = reopenedPlan.RequestID
	confirmationCommand.IssuedAtStateRevision = reopenedPlan.IssuedAtStateRevision
	confirmationCommand.ExpectedSubjectRevision = reopenedPlan.ExpectedSubjectRevision
	confirmationResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicConfirmRuntimeRepairSurface, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx, confirmationCommand,
	))
	confirmationData, confirmationErr := confirmationResult.Get()
	if confirmationErr != nil {
		t.Fatalf("File repair-surface confirmation error: %v", confirmationErr)
	}
	confirmationResponse, ok := confirmationData.(fileevents.ConfirmRuntimeRepairSurfaceResult)
	if !ok || confirmationResponse.Confirmation.Disposition != runtimecontract.RuntimeRepairSurfaceConfirmed ||
		!slices.Equal(confirmationResponse.Confirmation.ConfirmedTargetRefs, []string{"implementation.go"}) {
		t.Fatalf("File repair-surface confirmation = %#v", confirmationData)
	}
	persisted, err = controller.recordRepairSurfaceConfirmation(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, confirmationResponse.Confirmation)
	if err != nil || persisted.Facts.Resources.FailureTarget == nil || persisted.Facts.Resources.FailureTarget.TargetObservation == nil ||
		!slices.Equal(persisted.Facts.Resources.FailureTarget.TargetRefs, []string{"implementation.go"}) {
		t.Fatalf("owner confirmation did not CAS-bind the refreshed failure target: state=%s terminal=%+v target=%+v err=%v", persisted.Decision.State, persisted.Decision.Terminal, persisted.Facts.Resources.FailureTarget, err)
	}
	// A distinct repair target needs its own File snapshot; the validation
	// entry's existing snapshot cannot authorize that effect.
	persisted = reacquireObservedShellRepairPrerequisites(t, ctx, hub, controller, persisted, workspaceDirectory)
	if persisted.Decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("confirmed repair target did not become executable after owner refresh: %s", persisted.Decision.State)
	}
	var refreshedTargetSnapshot runtimecontract.TargetSnapshot
	for _, snapshot := range persisted.Facts.Resources.TargetSnapshots {
		if snapshot.TargetRef == "implementation.go" {
			refreshedTargetSnapshot = snapshot
			break
		}
	}
	if !refreshedTargetSnapshot.Complete() {
		t.Fatalf("owner confirmation did not reuse the current File snapshot acquired by stale refresh: %+v", persisted.Facts.Resources.TargetSnapshots)
	}

	// A fresh controller can only see the committed owner observation and
	// snapshot; it must not reconstruct a target from the validation entry.
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := controller.Load(ctx, "ws-1", "run-1")
	if err != nil || loaded.Facts.Resources.FailureTarget == nil || loaded.Facts.Resources.FailureTarget.TargetObservation == nil ||
		len(loaded.Facts.Resources.FailureTarget.TargetRefs) != 1 || loaded.Facts.Resources.FailureTarget.TargetRefs[0] != "implementation.go" {
		t.Fatalf("restart lost committed Shell target authority: aggregate=%+v err=%v", loaded, err)
	}

	// Replaying the exact sealed execution must query the committed receipt rather
	// than execute again or manufacture a new observation.
	dispatch := SealedDispatch{}
	for _, candidate := range loaded.CompletedDispatches {
		if candidate.ID == sealedAggregate.ActiveDispatch.ID {
			dispatch = candidate
			break
		}
	}
	if dispatch.ID == "" {
		t.Fatalf("completed Shell dispatch %q is missing: %+v", sealedAggregate.ActiveDispatch.ID, loaded.CompletedDispatches)
	}
	execution := dispatch.OperationExecutions[0]
	operations, err := orderedCandidateOperations(dispatch.Candidate)
	if err != nil || len(operations) != 1 {
		t.Fatalf("completed Shell operations = %+v err=%v", operations, err)
	}
	operation := operations[0]
	catalog := runtimeOwnerCatalogForOperation(dispatch.OperationAdmissibilities, dispatch.OperationAdmissibilityObservations, operation, execution.AdmissibilitySnapshotID, execution.AcquisitionBinding)
	observation := runtimeOwnerCatalogObservationForOperation(dispatch.OperationAdmissibilityObservations, catalog)
	if catalog == nil || observation == nil {
		t.Fatalf("completed Shell dispatch lost owner catalog binding: %+v", loaded.Facts.Resources)
	}
	validationScopeTargetRefs, err := runtimeValidationScopeTargetRefs(dispatch, operation)
	if err != nil {
		t.Fatal(err)
	}
	validationProofBinding, err := runtimeValidationProofBindingForOperation(dispatch, operation, catalog)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := shellevents.SealRuntimeOperation(operation, *catalog, shellevents.RuntimeExecutionSealInput{
		CatalogObservation: *observation, TargetRole: dispatch.FocusTargetRole,
		ValidationScopeTargetRefs: validationScopeTargetRefs,
		EvidencePolicies:          loaded.Facts.Activation.EvidencePolicies,
		ValidationProofBinding:    validationProofBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed := sendObservedShellExecution(t, ctx, hub, shellevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution:   runtimeOperationExecutionEnvelopeForTest(execution, nil),
			WorkspaceID: loaded.Facts.WorkspaceID, RunID: loaded.Facts.RunID, DispatchID: dispatch.ID, Transaction: dispatch.Transaction,
			OperationID: operation.ID, CoverageWorkItemIDs: operation.CoverageWorkItemIDs, EvidenceKind: runtimecontract.EvidenceKindValidation,
			WorkspaceDeliverableDirectory: workspaceDirectory, TargetRefs: operation.TargetRefs,
		},
		SealedOperation: sealed,
	})
	diagnostic := replayed.Receipt.Feedback.OriginalDiagnostic
	if replayed.Receipt.ExecutionKey != execution.ExecutionKey || diagnostic == nil || diagnostic.TargetObservation != nil ||
		diagnostic.ValidationFailureObservation == nil ||
		!slices.Equal(diagnostic.ValidationFailureObservation.CandidateTargetRefs, []string{"implementation.go"}) ||
		!slices.Equal(diagnostic.ValidationFailureObservation.ValidationScopeTargetRefs, sealedValidationScope) {
		t.Fatalf("duplicate Shell execution did not replay the committed receipt: %+v", replayed)
	}

	// The owner-confirmed target is now the exact repair surface. Reacquire all
	// current File/Shell facts, seal one target-bound File operation and execute
	// it through the real File owner/EventHub receipt route. The operation's own
	// workspace change must invalidate old owner facts without erasing the
	// state-change -> fresh-validation closure in the same evidence CAS.
	loaded = reacquireObservedShellRepairPrerequisites(t, ctx, hub, controller, loaded, workspaceDirectory)
	if loaded.Decision.State != runtimecontract.ConvergenceStateNeedStateChange || loaded.Facts.RepairDirective == nil ||
		loaded.Facts.RepairDirective.Closure == nil || loaded.Facts.Resources.FailureTarget == nil {
		t.Fatalf("confirmed Shell/File surface did not produce a repair closure: %+v", loaded)
	}
	repairClosureID := loaded.Facts.RepairDirective.Closure.ID
	var currentSnapshot runtimecontract.TargetSnapshot
	for _, candidateSnapshot := range loaded.Facts.Resources.TargetSnapshots {
		if candidateSnapshot.TargetRef == "implementation.go" {
			currentSnapshot = candidateSnapshot
			break
		}
	}
	if !currentSnapshot.Complete() {
		t.Fatalf("confirmed repair target has no current File snapshot: %+v", loaded.Facts.Resources.TargetSnapshots)
	}
	fileCatalog := runtimeTestOperationAdmissibilityForOwner(t, loaded.Facts, runtimecontract.RuntimeModuleFile)
	fileObservation, found := runtimecontract.RuntimePrerequisiteObservationForID(loaded.Facts.Resources.PrerequisiteObservations, fileCatalog.PrerequisiteID)
	if !found {
		t.Fatal("confirmed repair lost current File observation")
	}
	patchType := "text_replace"
	oldText := currentSnapshot.Content
	repairedContent := "package fixture\n// repaired through confirmed surface\n"
	if failsAgain {
		repairedContent += "var _ = anotherMissingImplementation\n"
	}
	baseHash := currentSnapshot.ContentHash
	repairCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		Kind:                runtimecontract.RuntimeBlockKindPatchApply,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), loaded.Facts.Focus.CoverageWorkItemIDs...),
		TargetRefs:          []string{"implementation.go"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldText},
			{Name: "newText", String: &repairedContent},
			{Name: "baseHash", String: &baseHash},
		},
	}}}}}
	repairCandidate = runtimeTestBindCandidateToCurrentFocus(t, loaded, repairCandidate)
	repairCandidate.StateChange.Operations[0].Operation.BindingRefs = []string{loaded.Facts.Focus.CoverageEvidenceBindings[0].ID}
	repairCandidate.StateChange.Operations[0].Operation.CoverageWorkItemIDs = nil
	repairManifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		loaded.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(loaded.Facts, loaded.Decision, nil),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	repairCandidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(repairManifest, repairCandidate)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce a model that supplies already-corrected text as oldText while
	// the owner revision is unchanged. It must be corrected before dispatch,
	// without retiring the confirmed surface or creating a File receipt.
	badCandidate := runtimecontract.CanonicalRuntimeCandidate(repairCandidate)
	badOldText := "package fixture\nvar _ = alreadyCorrectedImplementation\n"
	for index := range badCandidate.StateChange.Operations[0].Operation.Parameters {
		if badCandidate.StateChange.Operations[0].Operation.Parameters[index].Name == "oldText" {
			badCandidate.StateChange.Operations[0].Operation.Parameters[index].String = &badOldText
		}
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, loaded.Facts.WorkspaceID, loaded.Facts.RunID, 0, "confirmed-edit-prefix"); err != nil {
		t.Fatal(err)
	}
	proposal, err := controller.AdmitCandidateProposalForTest(ctx, loaded.Facts.WorkspaceID, loaded.Facts.RunID, 0, badCandidate)
	if err != nil || !proposal.Admission.Accepted() {
		t.Fatalf("bad-text proposal: failure=%+v correction=%+v err=%v", proposal.Admission.Failure, proposal.Admission.Correction, err)
	}
	probe := func(aggregate RuntimeAggregate, attempt int) runtimecontract.RuntimeCandidateFeasibilityRecord {
		t.Helper()
		record := aggregate.CandidateAttempts[len(aggregate.CandidateAttempts)-1]
		if record.Attempt != attempt || record.Proposal == nil {
			t.Fatal("repair probe has no current persisted proposal")
		}
		operation := record.Proposal.StateChange.Operations[0].Operation
		var authorization runtimecontract.RuntimeCandidateOperationManifest
		for _, value := range repairManifest.Operations {
			if value.Kind == operation.Kind {
				authorization = value
			}
		}
		response := hub.Send(magicevent.NewEventWithContext(fileevents.TopicProbeRuntimeOperationFeasibility,
			runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile, magicevent.NewHeader(), ctx,
			fileevents.ProbeRuntimeOperationFeasibilityCommand(runtimecontract.RuntimeOperationFeasibilityRequest{
				WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, Transaction: record.Transaction,
				WorkspaceDeliverableDirectory: workspaceDirectory, Operation: operation, CandidateManifestHash: repairManifest.Hash,
				OperationAuthorization: authorization, Admissibility: fileCatalog,
				Observation: &fileObservation, CurrentTargetSnapshot: &currentSnapshot,
			})))
		data, err := response.Get()
		if err != nil {
			t.Fatal(err)
		}
		owner := runtimecontract.RuntimeOperationFeasibilityObservation(data.(fileevents.ProbeRuntimeOperationFeasibilityResult))
		fact, factErr := runtimecontract.RuntimeOwnerFeasibilityObservationFromOperation(owner)
		if factErr != nil {
			t.Fatal(factErr)
		}
		return runtimecontract.RuntimeCandidateFeasibilityRecord{OperationID: operation.ID, CandidateManifestHash: repairManifest.Hash,
			AdmissibilitySnapshotID: owner.AdmissibilitySnapshotID, AcquisitionBinding: owner.CapabilityStateBinding,
			Observation: &fact, ReplayProof: owner.ReplayProof}
	}
	firstOwnerFact := probe(proposal.Aggregate, 0)
	closed, err := controller.candidateDispatch.recordCandidateFeasibility(ctx, loaded.Facts.WorkspaceID, loaded.Facts.RunID, 0, firstOwnerFact)
	if err != nil {
		t.Fatal(err)
	}
	firstAttempt := closed.CandidateAttempts[len(closed.CandidateAttempts)-1]
	if firstAttempt.Correction == nil || firstAttempt.Correction.Disposition != runtimecontract.RuntimeCandidateCorrectionCorrectableOnce ||
		closed.ActiveDispatch != nil || len(closed.OperationReceipts) != len(loaded.OperationReceipts) ||
		closed.Facts.RepairDirective == nil || closed.Facts.RepairDirective.Closure.ID != repairClosureID ||
		closed.Facts.Repair.RepairAttempts != loaded.Facts.Repair.RepairAttempts {
		t.Fatalf("candidate mismatch escaped bounded SM-3 repair correction: %+v", firstAttempt)
	}
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, loaded.Facts.WorkspaceID, loaded.Facts.RunID, 1, "confirmed-edit-prefix"); err != nil {
		t.Fatal(err)
	}
	corrected, err := controller.AdmitCandidateProposalForTest(ctx, loaded.Facts.WorkspaceID, loaded.Facts.RunID, 1, repairCandidate)
	if err != nil || !corrected.Admission.Accepted() {
		t.Fatalf("corrected edit proposal: %+v err=%v", corrected.Admission, err)
	}
	secondOwnerFact := probe(corrected.Aggregate, 1)
	if _, err := controller.candidateDispatch.recordCandidateFeasibility(ctx, loaded.Facts.WorkspaceID, loaded.Facts.RunID, 1, secondOwnerFact); err != nil {
		t.Fatal(err)
	}
	sealedRepair, err := controller.AcceptCandidateAttempt(ctx, loaded.Facts.WorkspaceID, loaded.Facts.RunID, 1, repairCandidate)
	if err != nil {
		t.Fatal(err)
	}
	repairOperation := sealedRepair.ActiveDispatch.Candidate.StateChange.Operations[0].Operation
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	repaired, err := controller.executeActiveDispatch(ctx, loaded.Facts.WorkspaceID, loaded.Facts.RunID, runtimeObservedFilePatchExecutor{hub: hub, deliverableDirectory: workspaceDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if repaired.ActiveDispatch != nil || repaired.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite ||
		repaired.Facts.RepairDirective == nil || repaired.Facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation ||
		repaired.Facts.RepairDirective.Closure == nil || repaired.Facts.RepairDirective.Closure.ID != repairClosureID ||
		repaired.Facts.Resources.FailureTarget != nil || len(repaired.Facts.Prerequisites.Pending) == 0 ||
		len(repaired.PrerequisiteInvalidations) == 0 {
		t.Fatalf("accepted confirmed owner repair lost closure or bypassed freshness: %+v", repaired)
	}
	if repaired.Facts.RepairDirective.CurrentFailure != nil ||
		!slices.Equal(repaired.Facts.RepairDirective.Closure.GroundedTargetRefs, []string{"implementation.go"}) {
		t.Fatalf("accepted confirmed owner repair retained stale target authority or lost immutable closure: %+v", repaired.Facts.RepairDirective)
	}
	content, readErr := os.ReadFile(filepath.Join(workspaceDirectory, "implementation.go"))
	if readErr != nil || string(content) != repairedContent {
		t.Fatalf("File owner did not apply confirmed repair: content=%q err=%v", content, readErr)
	}

	// Fresh load must accept the closed transaction but not resurrect its mutable
	// target. Both owner receipt replay and evidence replay remain exact no-ops.
	controller, err = NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := controller.Load(ctx, repaired.Facts.WorkspaceID, repaired.Facts.RunID)
	if err != nil || restarted.StateRevision != repaired.StateRevision || restarted.Facts.Resources.FailureTarget != nil ||
		restarted.Facts.RepairDirective == nil || restarted.Facts.RepairDirective.Closure == nil ||
		restarted.Facts.RepairDirective.Closure.ID != repairClosureID {
		t.Fatalf("restart lost confirmed owner repair transaction: aggregate=%+v err=%v", restarted, err)
	}
	repairDispatch := restarted.CompletedDispatches[len(restarted.CompletedDispatches)-1]
	repairExecution, found := operationExecutionRecord(repairDispatch.OperationExecutions, repairOperation.ID)
	if !found {
		t.Fatalf("completed confirmed repair has no execution: %+v", repairDispatch.OperationExecutions)
	}
	repairReceipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(restarted.OperationReceipts, repairExecution.ExecutionKey, repairExecution.Fence)
	if !found {
		t.Fatalf("completed confirmed repair has no owner receipt: %+v", restarted.OperationReceipts)
	}
	sealedFileOperation, err := fileevents.SealRuntimeOperation(repairOperation)
	if err != nil {
		t.Fatal(err)
	}
	ownerReplayResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicExecutePatchApplyOperation,
		runtimecontract.RuntimeModuleOrchestrator,
		runtimecontract.RuntimeModuleFile,
		magicevent.NewHeader(),
		ctx,
		fileevents.ExecutePatchApplyOperationCommand{
			RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
				Execution:   runtimeOperationExecutionEnvelopeForTest(repairExecution, nil),
				WorkspaceID: restarted.Facts.WorkspaceID, RunID: restarted.Facts.RunID,
				DispatchID: repairDispatch.ID, Transaction: repairDispatch.Transaction,
				OperationID: repairOperation.ID, CoverageWorkItemIDs: repairOperation.CoverageWorkItemIDs,
				TargetRefs: repairOperation.TargetRefs, EvidenceKind: runtimecontract.EvidenceKindStateChange,
				WorkspaceDeliverableDirectory: workspaceDirectory,
			},
			SealedOperation: sealedFileOperation,
		},
	))
	if ownerReplayResult == nil {
		t.Fatal("File owner returned no duplicate confirmed-repair result")
	}
	ownerReplayData, ownerReplayErr := ownerReplayResult.Get()
	ownerReplay, ownerReplayOK := ownerReplayData.(fileevents.ExecuteOperationResult)
	if ownerReplayErr != nil || !ownerReplayOK ||
		runtimecontract.StableRuntimeOperationReceiptHash(ownerReplay.Receipt) != runtimecontract.StableRuntimeOperationReceiptHash(repairReceipt) {
		t.Fatalf("File owner duplicate did not replay exact receipt: result=%#v err=%v", ownerReplayData, ownerReplayErr)
	}
	duplicateEvidence := ExecutionEvidence{
		EventID:     orchestratorevents.RuntimeOperationEventID(repairDispatch.ID, repairOperation.ID),
		WorkspaceID: restarted.Facts.WorkspaceID, RunID: restarted.Facts.RunID,
		DispatchID: repairDispatch.ID, AuthoritySnapshotID: repairDispatch.Transaction.AuthoritySnapshotID,
		OperationID: repairOperation.ID, Kind: runtimecontract.EvidenceKindStateChange,
		Status: ExecutionEvidenceStatusAccepted, Receipt: &repairReceipt,
	}
	beforeDuplicateRevision := restarted.StateRevision
	duplicate, err := controller.commitEvidence(ctx, duplicateEvidence)
	if err != nil || duplicate.Disposition != CommitDispositionDuplicate || duplicate.Aggregate.StateRevision != beforeDuplicateRevision ||
		duplicate.Aggregate.Facts.Resources.FailureTarget != nil || duplicate.Aggregate.Facts.RepairDirective == nil ||
		duplicate.Aggregate.Facts.RepairDirective.Closure == nil || duplicate.Aggregate.Facts.RepairDirective.Closure.ID != repairClosureID {
		t.Fatalf("duplicate confirmed-repair evidence changed handoff: result=%+v err=%v", duplicate, err)
	}

	final := reacquireObservedShellRepairPrerequisites(t, ctx, hub, controller, duplicate.Aggregate, workspaceDirectory)
	if final.Decision.State != runtimecontract.ConvergenceStateNeedValidation || len(final.Facts.Prerequisites.Pending) != 0 ||
		final.Facts.Resources.FailureTarget != nil || final.Facts.RepairDirective == nil ||
		final.Facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation ||
		final.Facts.RepairDirective.CurrentFailure != nil || len(final.Facts.RepairDirective.TargetRefs) != 0 ||
		final.Facts.RepairDirective.Closure == nil || final.Facts.RepairDirective.Closure.ID != repairClosureID {
		t.Fatalf("fresh owner facts did not retain the confirmed repair closure for validation: %+v", final)
	}
	restartedFinal, err := controller.Load(ctx, final.Facts.WorkspaceID, final.Facts.RunID)
	if err != nil || restartedFinal.Facts.RepairDirective == nil ||
		restartedFinal.Facts.RepairDirective.CurrentFailure != nil || len(restartedFinal.Facts.RepairDirective.TargetRefs) != 0 ||
		restartedFinal.Facts.RepairDirective.Closure == nil || restartedFinal.Facts.RepairDirective.Closure.ID != repairClosureID {
		t.Fatalf("restart resurrected confirmed File target authority: aggregate=%+v err=%v", restartedFinal, err)
	}
	// The edit receipt is causation-only. Only a new real Shell validation may
	// satisfy the original validation binding after owner facts are refreshed.
	for _, fact := range restartedFinal.Facts.Evidence.Accepted {
		if fact.DispatchID == repairDispatch.ID && len(fact.CoverageWorkItemIDs) != 0 {
			t.Fatal("repair edit acquired terminal validation credit")
		}
	}
	freshCandidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, TargetRefs: []string{validationEntry},
		BindingRefs: []string{restartedFinal.Facts.Focus.CoverageEvidenceBindings[0].ID},
		Parameters:  []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}}}}}
	freshCandidate = runtimeTestMustDeriveCandidateAuthority(t, restartedFinal, freshCandidate)
	testObservedFreshValidationAdmission(t, restartedFinal, freshCandidate)
	freshCatalog := runtimeTestOperationAdmissibilityForOwner(t, restartedFinal.Facts, runtimecontract.RuntimeModuleShell)
	freshSeal := testSealObservedFreshValidation(t, ctx, controller, restartedFinal, freshCandidate)
	validated, err := controller.executeActiveDispatch(ctx, final.Facts.WorkspaceID, final.Facts.RunID, runtimeObservedShellExecutor{hub: hub, deliverableDirectory: workspaceDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if failsAgain {
		if freshCatalog.SnapshotID == shellCatalog.SnapshotID ||
			validated.Facts.Focus.CoverageEvidenceBindings[0].Hash != restartedFinal.Facts.Focus.CoverageEvidenceBindings[0].Hash ||
			validated.Definition.StartupInputHash != restartedFinal.Definition.StartupInputHash {
			t.Fatal("catalog refresh must not rewrite the execution-locked binding or sealed startup input")
		}
		if len(ActiveRejectedOperationShapes(validated)) == 0 {
			t.Fatal("new rejected validation lost its replay guard before another confirmed repair")
		}
		plan, planErr := CurrentRuntimeRepairSurfaceConfirmationPlan(validated)
		if planErr != nil || !plan.Required() || validated.Decision.State != runtimecontract.ConvergenceStateNeedValidation {
			t.Fatalf("new rejected validation lost owner confirmation after catalog refresh: state=%s terminal=%+v plan=%+v err=%v", validated.Decision.State, validated.Decision.Terminal, plan, planErr)
		}
		if plan.RequestID == reopenedPlan.RequestID || runtimecontract.SameRuntimeValidationFailureObservation(plan.Observation, failureObservation) ||
			plan.ExpectedSubjectRevision == reopenedPlan.ExpectedSubjectRevision || validated.Facts.Resources.FailureTarget != nil || validated.Facts.RepairDirective != nil {
			t.Fatalf("new rejected validation inherited old repair authority: plan=%+v directive=%+v", plan, validated.Facts.RepairDirective)
		}
		for _, documentKind := range []runtimecontract.RuntimeProviderDocumentKind{runtimecontract.RuntimeProviderDocumentOperations, runtimecontract.RuntimeProviderDocumentSemanticPlan} {
			if err := candidateRequestSessionActionAllowed(validated, documentKind); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("pending confirmation admitted %s provider work: %v", documentKind, err)
			}
		}
		if err := validateCandidateAttemptBeginAuthority(validated); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("pending confirmation admitted candidate work: %v", err)
		}
		for name, mutate := range map[string]func(*RuntimeAggregate){
			"missing fresh rejected evidence": func(a *RuntimeAggregate) {
				for index, fact := range a.Facts.Evidence.Rejected {
					if fact.DispatchID == freshSeal.ActiveDispatch.ID {
						a.Facts.Evidence.Rejected = append(a.Facts.Evidence.Rejected[:index], a.Facts.Evidence.Rejected[index+1:]...)
						break
					}
				}
			},
			"old continuation restored": func(a *RuntimeAggregate) { a.Facts.RepairDirective = restartedFinal.Facts.RepairDirective },
			"fresh receipt fence drift": func(a *RuntimeAggregate) {
				for index := range a.OperationReceipts {
					if feedback := a.OperationReceipts[index].Feedback; feedback != nil && feedback.DispatchID == freshSeal.ActiveDispatch.ID {
						a.OperationReceipts[index].Fence++
					}
				}
			},
		} {
			t.Run(name, func(t *testing.T) {
				changed := cloneRuntimeAggregateForMCPProvenanceTest(t, validated)
				mutate(&changed)
				if err := validateConfirmedRepairFreshValidationContinuation(changed); err == nil {
					t.Fatal("invalid fresh-validation consumption passed reverse proof")
				}
			})
		}
		// Exact old response replay is harmless even though this is now a new
		// observation sequence. It cannot restore the first target or directive.
		replayed, err := controller.recordRepairSurfaceConfirmation(ctx, validated.Facts.WorkspaceID, validated.Facts.RunID, confirmationResponse.Confirmation)
		if err != nil || replayed.StateRevision != validated.StateRevision || replayed.Facts.Resources.FailureTarget != nil {
			t.Fatalf("old confirmation replay changed the new obligation: %v", err)
		}
		testContinueObservedShellRepair(t, ctx, hub, repository, validated, workspaceDirectory, validationEntry, secondSucceeds)
		return
	}
	acceptedValidation := false
	for _, fact := range validated.Facts.Evidence.Accepted {
		if fact.DispatchID == freshSeal.ActiveDispatch.ID && fact.Kind == runtimecontract.EvidenceKindValidation && len(fact.CoverageWorkItemIDs) == 1 {
			acceptedValidation = true
		}
	}
	if !acceptedValidation {
		t.Fatalf("fresh real validation did not prove the original CWI: state=%s failure=%+v", validated.Decision.State, validated.Facts.Failure)
	}
	if err := validated.Validate(); err != nil {
		t.Fatalf("fresh validation reload: %v", err)
	}
}

// Final sealing alone does not exercise the production pre-probe evaluator.
// The exact same manifest must admit validation of the Definition entry even
// when the completed File repair targeted a different implementation file.
func testObservedFreshValidationAdmission(t *testing.T, aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) {
	t.Helper()
	manifest, err := BuildRuntimeCandidateContractManifestForAggregate(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if admission := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, candidate); !admission.Accepted() {
		t.Fatalf("fresh validation rejected before owner probe: failure=%+v diagnostic=%+v correction=%+v", admission.Failure, admission.Failure.Diagnostic, admission.Correction)
	}
	for name, mutate := range map[string]func(*RuntimeAggregate){
		"missing confirmation": func(a *RuntimeAggregate) { a.RepairSurfaceConfirmations = nil },
		"orphan handoff":       func(a *RuntimeAggregate) { a.CompletedDispatches = nil },
		"missing receipts":     func(a *RuntimeAggregate) { a.OperationReceipts = nil },
		"missing continuation": func(a *RuntimeAggregate) { a.Facts.RepairDirective = nil },
		"restored repair target": func(a *RuntimeAggregate) {
			a.Facts.RepairDirective.TargetRefs = append([]string(nil), a.Facts.RepairDirective.Closure.GroundedTargetRefs...)
		},
		"stale evidence fence": func(a *RuntimeAggregate) {
			a.Facts.RepairDirective.SupersedesEvidenceRevision = "runtime-evidence-99999999999999999999"
		},
		"different closure": func(a *RuntimeAggregate) { a.Facts.RepairDirective.Closure.ID = "other-closure" },
	} {
		t.Run("admission rejects "+name, func(t *testing.T) {
			changed := cloneRuntimeAggregateForMCPProvenanceTest(t, aggregate)
			mutate(&changed)
			proof := candidateRepairClosureFacts(changed)
			if proof.LineageError == "" || proof.CompletedRepairEvidenceRevision != "" {
				t.Fatalf("unproven repair handoff acquired a new validation baseline: %+v", proof)
			}
			admission := EvaluateCandidateAdmissionWithManifest(changed, manifest, candidate)
			if admission.Accepted() || admission.Failure.Code != "runtime_contract_defect" || admission.Correction.Witness != nil {
				t.Fatalf("invalid lineage admitted correction/dispatch: failure=%+v correction=%+v", admission.Failure, admission.Correction)
			}
		})
	}
	for name, mutate := range map[string]func(*runtimecontract.RuntimeOperation){
		"unsealed target": func(op *runtimecontract.RuntimeOperation) { op.TargetRefs = []string{"unsealed-target"} },
		"stale binding":   func(op *runtimecontract.RuntimeOperation) { op.BindingRefs = []string{"stale-binding"} },
		"different owner": func(op *runtimecontract.RuntimeOperation) { op.Kind = runtimecontract.StepKindFileRead },
	} {
		t.Run("admission rejects "+name, func(t *testing.T) {
			changed := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{
				Operations: append([]runtimecontract.ValidationOperation(nil), candidate.Validation.Operations...),
			}}
			mutate(&changed.Validation.Operations[0].Operation)
			if admission := EvaluateCandidateAdmissionWithManifest(aggregate, manifest, changed); admission.Accepted() {
				t.Fatal("fresh validation bypassed the sealed manifest")
			}
		})
	}
}

// The provider document and read-only feasible responses are fixture inputs;
// proposal admission and dispatch sealing use the real SM-3 owner transitions.
// The external effect still runs through the real Shell owner below.
func testSealObservedFreshValidation(t *testing.T, ctx context.Context, controller *RuntimeController, aggregate RuntimeAggregate, candidate runtimecontract.RuntimeCandidate) RuntimeAggregate {
	t.Helper()
	workspaceID, runID := aggregate.Facts.WorkspaceID, aggregate.Facts.RunID
	if _, err := controller.BeginCandidateAttemptForTest(ctx, workspaceID, runID, 0, "fresh-validation-prefix"); err != nil {
		t.Fatal(err)
	}
	if err := RecordFixtureCandidateFeasibilities(ctx, controller, workspaceID, runID, 0, candidate); err != nil {
		t.Fatal(err)
	}
	pending, manifest, requestID, err := candidateAttemptTestBinding(ctx, controller, workspaceID, runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	testObservedFreshValidationRejectionCAS(t, pending)
	seal := RuntimeInternalEvent{
		Kind: RuntimeInternalEventCandidateSealed, Scope: RuntimeInternalEventScope{WorkspaceID: workspaceID, RunID: runID},
		Transaction: &RuntimeInternalTransactionPayload{Attempt: 0, Manifest: &manifest, LogicalRequestID: requestID, Candidate: &candidate},
	}
	sealed, err := controller.ApplyInternalEvent(ctx, seal)
	if err != nil || sealed.Aggregate.ActiveDispatch == nil || sealed.Aggregate.StateRevision != pending.StateRevision+1 ||
		sealed.Aggregate.ActiveDispatch.RepairCommitment != nil {
		t.Fatalf("fresh validation did not seal in one owner CAS without new repair authority: %v", err)
	}
	duplicate, err := controller.ApplyInternalEvent(ctx, seal)
	if err != nil || duplicate.Aggregate.StateRevision != sealed.Aggregate.StateRevision ||
		duplicate.Aggregate.ActiveDispatch.ID != sealed.Aggregate.ActiveDispatch.ID {
		t.Fatalf("duplicate fresh validation seal changed the transaction: %v", err)
	}
	loaded, err := controller.Load(ctx, workspaceID, runID)
	if err != nil || loaded.ActiveDispatch.ID != sealed.Aggregate.ActiveDispatch.ID {
		t.Fatalf("fresh validation seal cannot be reloaded: %v", err)
	}
	return loaded
}

func testContinueObservedShellRepair(t *testing.T, ctx context.Context, hub magicevent.Hub, repository *durableRuntimeStateRepository, pending RuntimeAggregate, workspaceDirectory, validationEntry string, succeeds bool) {
	t.Helper()
	// Reload the new failure without altering owner resources or old seals.
	repository.aggregate = cloneRuntimeAggregateForMCPProvenanceTest(t, pending)
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(repository.aggregate)
	if err != nil || !plan.Required() {
		t.Fatalf("reload lost new confirmation obligation: %v", err)
	}
	response := hub.Send(magicevent.NewEventWithContext(fileevents.TopicConfirmRuntimeRepairSurface, runtimecontract.RuntimeModuleOrchestrator,
		orchestratorevents.PrerequisiteCommandDestination, magicevent.NewHeader(), ctx, fileevents.ConfirmRuntimeRepairSurfaceCommand{
			RequestID: plan.RequestID, IssuedAtStateRevision: plan.IssuedAtStateRevision, ExpectedSubjectRevision: plan.ExpectedSubjectRevision,
			WorkspaceID: pending.Facts.WorkspaceID, RunID: pending.Facts.RunID, WorkspaceDeliverableDirectory: workspaceDirectory, Observation: plan.Observation,
		}))
	data, ownerErr := response.Get()
	if ownerErr != nil {
		t.Fatal(ownerErr)
	}
	confirmation := data.(fileevents.ConfirmRuntimeRepairSurfaceResult).Confirmation
	confirmed, err := controller.recordRepairSurfaceConfirmation(ctx, pending.Facts.WorkspaceID, pending.Facts.RunID, confirmation)
	if err != nil || confirmed.StateRevision != pending.StateRevision+1 || len(confirmed.RepairSurfaceConfirmations) != len(pending.RepairSurfaceConfirmations)+1 {
		t.Fatalf("second confirmation was not one owner CAS: %v", err)
	}
	loaded := reacquireObservedShellRepairPrerequisites(t, ctx, hub, controller, confirmed, workspaceDirectory)
	if loaded.Facts.RepairDirective == nil || loaded.Facts.RepairDirective.CurrentFailure == nil ||
		loaded.Facts.RepairDirective.CurrentFailure.Signature != pending.Facts.Failure.Signature {
		t.Fatalf("second repair did not bind the new rejected receipt: state=%s terminal=%+v failure=%+v directive=%+v repair=%+v", loaded.Decision.State, loaded.Decision.Terminal, loaded.Facts.Failure, loaded.Facts.RepairDirective, loaded.Facts.Repair)
	}
	var snapshot runtimecontract.TargetSnapshot
	for _, value := range loaded.Facts.Resources.TargetSnapshots {
		if value.TargetRef == "implementation.go" {
			snapshot = value
		}
	}
	patchType, oldText, baseHash := "text_replace", snapshot.Content, snapshot.ContentHash
	newText := "package fixture\n// second confirmed repair\n"
	if !succeeds {
		newText += "var _ = finalMissingImplementation\n"
	}
	candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"implementation.go"}, BindingRefs: []string{loaded.Facts.Focus.CoverageEvidenceBindings[0].ID},
		Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}, {Name: "baseHash", String: &baseHash}},
	}}}}}
	candidate = runtimeTestMustDeriveCandidateAuthority(t, loaded, candidate)
	catalog := runtimeTestOperationAdmissibilityForOwner(t, loaded.Facts, runtimecontract.RuntimeModuleFile)
	sealCandidateWithFeasibilitiesForTest(t, repository, candidate, []runtimecontract.RuntimeCandidateFeasibilityRecord{{
		OperationID: candidate.StateChange.Operations[0].Operation.ID, AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
		Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
	}})
	repaired, err := controller.executeActiveDispatch(ctx, loaded.Facts.WorkspaceID, loaded.Facts.RunID, runtimeObservedFilePatchExecutor{hub: hub, deliverableDirectory: workspaceDirectory})
	if err != nil {
		t.Fatal(err)
	}
	ready := reacquireObservedShellRepairPrerequisites(t, ctx, hub, controller, repaired, workspaceDirectory)
	recipeID := ready.Facts.Focus.ValidationRecipeID
	fresh := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, TargetRefs: []string{validationEntry}, BindingRefs: []string{ready.Facts.Focus.CoverageEvidenceBindings[0].ID},
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}}}}}
	fresh = runtimeTestMustDeriveCandidateAuthority(t, ready, fresh)
	testObservedFreshValidationAdmission(t, ready, fresh)
	testSealObservedFreshValidation(t, ctx, controller, ready, fresh)
	final, err := controller.executeActiveDispatch(ctx, ready.Facts.WorkspaceID, ready.Facts.RunID, runtimeObservedShellExecutor{hub: hub, deliverableDirectory: workspaceDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if succeeds {
		if !hasAcceptedCurrentFocusValidationEvidence(final.Facts, final.CoverageProofLineage) || final.Decision.State == runtimecontract.ConvergenceStateFailed {
			t.Fatalf("second repair lost new validation proof: decision=%+v failure=%+v", final.Decision, final.Facts.Failure)
		}
	} else if final.Decision.State != runtimecontract.ConvergenceStateFailed || final.Decision.Terminal == nil || final.Decision.Terminal.Code != "validation_failure_repair_exhausted" {
		t.Fatalf("third rejection did not close sealed Profile budget: decision=%+v repair=%+v", final.Decision, final.Facts.Repair)
	}
	if err := final.Validate(); err != nil {
		t.Fatalf("final reload: %v", err)
	}
	if plan, err := CurrentRuntimeRepairSurfaceConfirmationPlan(final); err != nil || plan.Required() {
		t.Fatalf("settled sequence issued another owner query: %+v %v", plan, err)
	}
}

// TestShellOwnerSelectorOnlyValidationNeverCreatesRepairObservation proves the
// owner boundary for an executable selector-only managed-validation seal. A
// closed failure-target marker remains only a diagnostic hint when the SM-3
// authorization has no finite scope; neither first delivery nor receipt replay
// may turn the current File manifest into historical repair authority.
func TestShellOwnerSelectorOnlyValidationNeverCreatesRepairObservation(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })

	workspaceDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "go.mod"), []byte("module example.com/workorch-selector-only\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fileOwner, err := filebiz.New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := fileOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fileOwner.Teardown(ctx) })
	shellOwner, err := shellbiz.New(hub, nil, shellbiz.Dependencies{RuntimeAdapter: shellbiz.NewRuntimeAdapter()})
	if err != nil {
		t.Fatal(err)
	}
	if err := shellOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shellOwner.Teardown(ctx) })

	policy := runtimeManagedValidationEvidencePolicy()
	policy.CommandName = "go"
	policy.RecipeArgs = []string{"test", "-v", "./..."}
	ownerPreflight := validationAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Focus.RequiredCheckRefs = []string{"workorch-cwi-check-selectoronly"}
	})
	ownerPreflight.Facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{policy}
	shellCatalog := addSelectorOnlyShellFailureTargetFreshness(t, ctx, hub, &ownerPreflight, workspaceDirectory)
	aggregate := validationOnlyAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{policy}
		aggregate.Facts.Focus.RequiredCheckRefs = []string{"workorch-cwi-check-selectoronly"}
		aggregate.Definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindManagedValidationSession}
		aggregate.Facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
		aggregate.Facts.Focus.TargetRefs = nil
		aggregate.Facts.Focus.TargetRole = "implementation"
		aggregate.Facts.Focus.TargetSelectorRef = "workspace-validation-target"
		aggregate.Facts.Focus.TargetSelectorOwnerRef = runtimecontract.RuntimeModuleShell
		aggregate.Facts.Focus.TargetSelectorSubjectRef = "implementation"
		aggregate.Facts.Focus.RequireTargetGrounding = true
		runtimeTestInstallObservedShellPreflight(aggregate, ownerPreflight, shellCatalog)
	})
	shellCatalog = runtimeTestOperationAdmissibilityForOwner(t, aggregate.Facts, runtimecontract.RuntimeModuleShell)
	repository := &durableRuntimeStateRepository{aggregate: aggregate}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	registerRuntimeOperationReceiptOwner(t, hub, controller)

	recipeID := policy.ID
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-selector-only", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Parameters:          []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}}}}}
	candidate = runtimeTestBindCandidateToCurrentFocus(t, aggregate, candidate)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(
		aggregate.Decision,
		runtimecontract.RuntimeCandidateManifestOwnerFactsFromContext(
			runtimecontract.RuntimeCandidateContextForDecision(aggregate.Facts, aggregate.Decision, nil),
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err = runtimecontract.DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
	if err != nil {
		t.Fatal(err)
	}
	canonicalOperation := candidate.Validation.Operations[0].Operation
	sealedAggregate := sealCandidateWithFeasibilitiesForTest(t, repository, candidate, []runtimecontract.RuntimeCandidateFeasibilityRecord{{
		OperationID: canonicalOperation.ID, AdmissibilitySnapshotID: shellCatalog.SnapshotID,
		AcquisitionBinding: shellCatalog.AcquisitionBinding,
		Observation:        &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
	}})
	if sealedAggregate.ActiveDispatch == nil || len(sealedAggregate.ActiveDispatch.OperationTargetAuthorizations) != 1 {
		t.Fatalf("selector-only dispatch seal = %+v", sealedAggregate.ActiveDispatch)
	}
	targetAuthorization := sealedAggregate.ActiveDispatch.OperationTargetAuthorizations[0].Target.Normalized()
	if targetAuthorization.Policy != runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope || len(targetAuthorization.OwnerScopeTargetRefs) != 0 ||
		!slices.Contains(targetAuthorization.OwnerSelectorRefs, "workspace-validation-target") || len(targetAuthorization.AllowedTargetRefs()) != 0 {
		t.Fatalf("managed validation was not sealed selector-only: %+v", targetAuthorization)
	}
	sealedScope, err := runtimeValidationScopeTargetRefs(*sealedAggregate.ActiveDispatch, canonicalOperation)
	if err != nil || len(sealedScope) != 0 {
		t.Fatalf("selector-only seal invented finite validation scope: scope=%v err=%v", sealedScope, err)
	}

	persisted, err := controller.executeActiveDispatch(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, runtimeObservedShellExecutor{hub: hub, deliverableDirectory: workspaceDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Facts.Failure == nil || persisted.Facts.Failure.ValidationFailureObservation != nil || persisted.Facts.Failure.TargetObservation != nil ||
		len(persisted.Facts.Failure.TargetRefs) != 0 || persisted.Facts.Resources.FailureTarget != nil ||
		persisted.Facts.Failure.OriginalDiagnostic == nil || persisted.Facts.Failure.OriginalDiagnostic.ValidationFailureObservation != nil ||
		persisted.Facts.Failure.OriginalDiagnostic.TargetObservation != nil {
		t.Fatalf("selector-only Shell marker created repair authority: %+v", persisted.Facts.Failure)
	}
	completedDispatchID := sealedAggregate.ActiveDispatch.ID
	completed := false
	for _, dispatch := range persisted.CompletedDispatches {
		if dispatch.ID == completedDispatchID {
			completed = true
			break
		}
	}
	if !completed {
		t.Fatalf("selector-only rejected validation did not complete its dispatch: %+v", persisted.CompletedDispatches)
	}

	// Refresh mutable File facts after the completed selector-only dispatch. The
	// new finite manifest was never part of the SM-3 operation authorization;
	// fresh load and duplicate owner delivery must not use it as repair scope.
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "implementation.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "validation-entry.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: "manifest:shell-selector-only", Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	refreshedManifest := sendObservedManifest(t, ctx, hub, persisted, manifestPrerequisite, workspaceDirectory)
	repository.aggregate.Facts.Resources.Manifest = &refreshedManifest.Manifest
	for index := range repository.aggregate.Facts.Resources.PrerequisiteObservations {
		observation := repository.aggregate.Facts.Resources.PrerequisiteObservations[index]
		if observation.PrerequisiteID == manifestPrerequisite.ID && observation.Kind == runtimecontract.PrerequisiteKindManifest {
			repository.aggregate.Facts.Resources.PrerequisiteObservations[index] = refreshedManifest.Observation
		}
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Facts.Resources.Manifest == nil || len(loaded.Facts.Resources.Manifest.TargetRefs) == 0 {
		t.Fatalf("selector-only restart fixture lost its finite current manifest: %+v", loaded.Facts.Resources.Manifest)
	}
	dispatch := SealedDispatch{}
	for _, candidate := range loaded.CompletedDispatches {
		if candidate.ID == completedDispatchID {
			dispatch = candidate
			break
		}
	}
	if dispatch.ID == "" {
		t.Fatalf("selector-only completed dispatch %q is missing: %+v", completedDispatchID, loaded.CompletedDispatches)
	}
	operations, err := orderedCandidateOperations(dispatch.Candidate)
	if err != nil || len(operations) != 1 {
		t.Fatalf("selector-only completed operations = %+v err=%v", operations, err)
	}
	operation := operations[0]
	reloadedScope, err := runtimeValidationScopeTargetRefs(dispatch, operation)
	if err != nil || len(reloadedScope) != 0 {
		t.Fatalf("restart supplemented selector-only scope from current manifest: scope=%v err=%v", reloadedScope, err)
	}
	execution := dispatch.OperationExecutions[0]
	catalog := runtimeOwnerCatalogForOperation(dispatch.OperationAdmissibilities, dispatch.OperationAdmissibilityObservations, operation, execution.AdmissibilitySnapshotID, execution.AcquisitionBinding)
	if catalog == nil {
		t.Fatalf("selector-only completed dispatch lost Shell catalog: %+v", dispatch.OperationAdmissibilities)
	}
	catalogObservation := runtimeOwnerCatalogObservationForOperation(dispatch.OperationAdmissibilityObservations, catalog)
	if catalogObservation == nil {
		t.Fatalf("selector-only completed dispatch lost Shell catalog observation: %+v", dispatch.OperationAdmissibilityObservations)
	}
	validationProofBinding, err := runtimeValidationProofBindingForOperation(dispatch, operation, catalog)
	if err != nil {
		t.Fatal(err)
	}
	sealedOperation, err := shellevents.SealRuntimeOperation(operation, *catalog, shellevents.RuntimeExecutionSealInput{
		CatalogObservation: *catalogObservation, TargetRole: dispatch.FocusTargetRole,
		ValidationScopeTargetRefs: reloadedScope, EvidencePolicies: loaded.Facts.Activation.EvidencePolicies,
		ValidationProofBinding: validationProofBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed := sendObservedShellExecution(t, ctx, hub, shellevents.ExecuteOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution:   runtimeOperationExecutionEnvelopeForTest(execution, nil),
			WorkspaceID: loaded.Facts.WorkspaceID, RunID: loaded.Facts.RunID, DispatchID: dispatch.ID, Transaction: dispatch.Transaction,
			OperationID: operation.ID, CoverageWorkItemIDs: operation.CoverageWorkItemIDs, EvidenceKind: runtimecontract.EvidenceKindValidation,
			WorkspaceDeliverableDirectory: workspaceDirectory, TargetRefs: operation.TargetRefs,
		},
		SealedOperation: sealedOperation,
	})
	diagnostic := replayed.Receipt.Feedback.OriginalDiagnostic
	if diagnostic == nil || diagnostic.ValidationFailureObservation != nil || diagnostic.TargetObservation != nil || len(diagnostic.TargetRefs) != 0 {
		t.Fatalf("selector-only duplicate/restart created repair authority: %+v", replayed)
	}
}

// Direct Shell failure-target observations are not a legal authority shape.
// Fresh-load/initialization rejects them without reconstructing owner facts.
func TestRuntimeAggregateRejectsDirectShellFailureTargetAuthority(t *testing.T) {
	facts := controllerFacts()
	observation := &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", TargetRole: "implementation",
		ObservedRevision: "shell-policy-v1", TargetRefs: []string{"implementation.go"},
		DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed,
		ContentHash:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EvidenceRefs:    []string{"workorch://shell/failure-target/sha256/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
	}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassBuildFailure, Signature: "shell-failure", Message: "retired direct Shell authority",
		TargetRefs: []string{"implementation.go"}, TargetObservation: observation,
	}
	facts.Resources.FailureTarget = &runtimecontract.FailureTarget{
		Signature: "shell-failure", TargetRefs: []string{"implementation.go"}, TargetObservation: observation,
	}
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.initialize(context.Background(), facts, controllerDefinition(), nil); err == nil ||
		!strings.Contains(err.Error(), "receipt-evidence target observation copies") {
		t.Fatalf("direct Shell failure target initialization error = %v", err)
	}
	if repository.aggregate != nil || repository.saves != 0 {
		t.Fatalf("invalid direct Shell target authority was persisted: %+v", repository.aggregate)
	}
}

func addObservedShellFailureTargetFreshness(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate *RuntimeAggregate, workspaceDirectory string, nativeCheckFails bool) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	manifestPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: "manifest:shell-observed-target", Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "operation_admissibility:shell-observed-target", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	requirements := runtimeTestShellValidationProofRequirements(t, aggregate.Facts)
	initialManifest := sendObservedManifest(t, ctx, hub, *aggregate, manifestPrerequisite, workspaceDirectory)
	initial := acquireObservedShellValidationCatalog(
		t, ctx, hub, *aggregate, prerequisite, workspaceDirectory, initialManifest,
		initialManifest.Manifest.TargetRefs, requirements,
	)
	runtimeTestInstallShellNativeValidationChecks(t, workspaceDirectory, initial.Acquisition.Admissibility, requirements, nativeCheckFails)
	manifest := sendObservedManifest(t, ctx, hub, *aggregate, manifestPrerequisite, workspaceDirectory)
	response := acquireObservedShellValidationCatalog(
		t, ctx, hub, *aggregate, prerequisite, workspaceDirectory, manifest,
		manifest.Manifest.TargetRefs, requirements,
	)
	for _, recipe := range runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(aggregate.Facts.Activation.EvidencePolicies) {
		if !slices.Contains(response.Acquisition.Admissibility.ValidationRecipeRefs, recipe.ID) {
			t.Fatalf("Shell operation admissibility did not seal recipe %q: %+v", recipe.ID, response.Acquisition.Admissibility)
		}
		if _, found := runtimecontract.RuntimeValidationRecipeForID(response.Acquisition.Admissibility.ValidationRecipes, recipe.ID); !found {
			t.Fatalf("Shell operation admissibility omitted executable recipe %q: %+v", recipe.ID, response.Acquisition.Admissibility)
		}
	}
	aggregate.Facts.Resources.OperationAdmissibilities = append(aggregate.Facts.Resources.OperationAdmissibilities, response.Acquisition.Admissibility)
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, response.Acquisition.Observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
	return response.Acquisition.Admissibility
}

// runtimeTestInstallObservedShellPreflight moves only the real File-manifest
// and Shell-catalog observations obtained through EventHub into the aggregate
// that will subsequently seal its semantic contract. The disposable source
// aggregate supplies a valid SM-2 authority snapshot for the owner queries;
// none of its synthetic semantic bindings are copied into the final fixture.
func runtimeTestInstallObservedShellPreflight(aggregate *RuntimeAggregate, observed RuntimeAggregate, catalog runtimecontract.RuntimeOperationAdmissibility) {
	if aggregate == nil {
		return
	}
	if manifest := observed.Facts.Resources.Manifest; manifest != nil {
		aggregate.Facts.Resources.Manifest = &runtimecontract.WorkspaceManifest{
			Revision: manifest.Revision, TargetRefs: append([]string(nil), manifest.TargetRefs...),
		}
	}
	catalog = catalog.Normalized()
	fileCatalog := runtimeValidationFailureRepairFileCatalogForTest()
	fileCatalog.PrerequisiteID = "operation_admissibility:file-observed-workspace"
	if aggregate.Facts.Resources.Manifest != nil {
		fileCatalog.ObservedRevision = aggregate.Facts.Resources.Manifest.Revision
		fileCatalog.TargetRefs = append([]string(nil), aggregate.Facts.Resources.Manifest.TargetRefs...)
	}
	fileCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog, fileCatalog}
	aggregate.Facts.Resources.PrerequisiteObservations = nil
	for _, rawObservation := range observed.Facts.Resources.PrerequisiteObservations {
		observation := rawObservation.Normalized()
		if observation.PrerequisiteID == catalog.PrerequisiteID ||
			(observation.Kind == runtimecontract.PrerequisiteKindManifest && observation.OwnerModuleID == runtimecontract.RuntimeModuleFile) {
			aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, observation)
		}
	}
	fileObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: fileCatalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: fileCatalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: runtimecontract.RuntimeRepairSurfaceFileWorkspaceSubjectRef, Revision: fileCatalog.ObservedRevision,
		}},
	}
	fileObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(fileObservation)
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, fileObservation)
	aggregate.Facts.Prerequisites.Satisfied = nil
	for _, prerequisite := range observed.Facts.Prerequisites.Satisfied {
		if prerequisite.ID == catalog.PrerequisiteID ||
			(prerequisite.Kind == runtimecontract.PrerequisiteKindManifest && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleFile) {
			aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
		}
	}
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
		ID: fileCatalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		Scope: runtimecontract.RuntimePrerequisiteScopeGlobal, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		RequiredCapabilities: append([]string(nil), fileCatalog.OperationKinds...), Reason: "real File workspace preflight fixture",
	})
}

func addSelectorOnlyShellFailureTargetFreshness(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate *RuntimeAggregate, workspaceDirectory string) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	manifestPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: "manifest:shell-selector-only", Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "operation_admissibility:shell-selector-only", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	requirements := runtimeTestShellValidationProofRequirements(t, aggregate.Facts)
	initialManifest := sendObservedManifest(t, ctx, hub, *aggregate, manifestPrerequisite, workspaceDirectory)
	initial := acquireObservedShellValidationCatalog(t, ctx, hub, *aggregate, prerequisite, workspaceDirectory, initialManifest, nil, requirements)
	runtimeTestInstallShellNativeValidationChecks(t, workspaceDirectory, initial.Acquisition.Admissibility, requirements, true)
	manifest := sendObservedManifest(t, ctx, hub, *aggregate, manifestPrerequisite, workspaceDirectory)
	response := acquireObservedShellValidationCatalog(t, ctx, hub, *aggregate, prerequisite, workspaceDirectory, manifest, nil, requirements)
	catalog := response.Acquisition.Admissibility.Normalized()
	if len(catalog.TargetRefs) != 0 || !slices.Contains(catalog.SelectorRefs, "workspace-validation-target") {
		t.Fatalf("Shell owner did not retain selector-only admissibility: %+v", catalog)
	}
	for _, recipe := range runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(aggregate.Facts.Activation.EvidencePolicies) {
		if _, found := runtimecontract.RuntimeValidationRecipeForID(catalog.ValidationRecipes, recipe.ID); !found {
			t.Fatalf("Shell selector-only admissibility omitted executable recipe %q: %+v", recipe.ID, catalog)
		}
	}
	aggregate.Facts.Resources.OperationAdmissibilities = append(aggregate.Facts.Resources.OperationAdmissibilities, catalog)
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, response.Acquisition.Observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
	return catalog
}

func runtimeTestShellValidationProofRequirements(t *testing.T, facts runtimecontract.RuntimeFacts) []runtimecontract.RuntimeValidationProofRequirement {
	t.Helper()
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(
		facts.DefinitionRevision,
		facts.DefinitionSeed.CoverageItems,
		facts.DefinitionFocuses,
	)
	if err != nil {
		t.Fatalf("compile exact Shell proof requirements: %v", err)
	}
	if len(requirements) == 0 {
		t.Fatal("managed-validation integration fixture has no exact CWI proof requirement")
	}
	return requirements
}

func acquireObservedShellValidationCatalog(
	t *testing.T,
	ctx context.Context,
	hub magicevent.Hub,
	aggregate RuntimeAggregate,
	prerequisite runtimecontract.RuntimePrerequisite,
	workspaceDirectory string,
	manifest fileevents.AcquireWorkspaceManifestResult,
	requiredTargetRefs []string,
	requirements []runtimecontract.RuntimeValidationProofRequirement,
) shellevents.AcquireRuntimeOperationAdmissibilityResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicAcquireRuntimeOperationAdmissibility,
		runtimecontract.RuntimeModuleOrchestrator,
		runtimecontract.RuntimeModuleShell,
		magicevent.NewHeader(),
		ctx,
		shellevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			PrerequisiteID:      prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID,
			WorkspaceDeliverableDirectory: workspaceDirectory,
			RequiredKinds:                 prerequisite.RequiredCapabilities,
			RequiredTargetRefs:            append([]string(nil), requiredTargetRefs...),
			ValidationRecipes: runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(
				aggregate.Facts.Activation.EvidencePolicies,
			),
			ValidationProofRequirements: runtimecontract.NormalizeRuntimeValidationProofRequirements(requirements),
			WorkspaceManifestBinding: runtimecontract.RuntimeWorkspaceManifestBinding{
				Manifest: manifest.Manifest, Observation: manifest.Observation,
			},
			MaxItems: runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems,
		},
	))
	if result == nil {
		t.Fatal("Shell owner returned no operation admissibility result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(shellevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Acquisition.Observation.ObservationID == "" || response.Acquisition.Admissibility.ObservedRevision == "" {
		t.Fatalf("Shell operation admissibility response = %#v", data)
	}
	return response
}

func runtimeTestInstallShellNativeValidationChecks(
	t *testing.T,
	workspaceDirectory string,
	catalog runtimecontract.RuntimeOperationAdmissibility,
	requirements []runtimecontract.RuntimeValidationProofRequirement,
	fail bool,
) {
	t.Helper()
	moduleData, err := os.ReadFile(filepath.Join(workspaceDirectory, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	modulePath := ""
	for _, line := range strings.Split(string(moduleData), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			modulePath = strings.Trim(fields[1], "\"")
			break
		}
	}
	if modulePath == "" {
		t.Fatal("native owner fixture requires a module")
	}
	// An independent, fixed subject exercises the native behavior protocol.
	// The surrounding fixtures test Runtime transitions, not template semantics.
	subjectDirectory := filepath.Join(workspaceDirectory, "runtime_proof_subject")
	if err := os.MkdirAll(subjectDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subjectDirectory, "subject.go"), []byte("package subject\nfunc Value() int { return 4 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	preparationByRequirement := make(map[string]runtimecontract.RuntimeValidationProofPreparation)
	for _, surface := range catalog.ValidationSurfaces {
		for _, preparation := range surface.ProofRequirementPreparations {
			preparationByRequirement[preparation.RequirementHash] = preparation
		}
	}
	for _, requirement := range requirements {
		preparation, found := preparationByRequirement[requirement.Hash]
		if !found || preparation.TargetRef == "" || len(preparation.CheckAttestations) == 0 {
			t.Fatalf("Shell catalog has no exact native preparation for requirement %q: %+v", requirement.Hash, catalog)
		}
		var source strings.Builder
		fmt.Fprintf(&source, "package workorch_validation\n\nimport (\"testing\"; subject %q)\n\n", modulePath+"/runtime_proof_subject")
		for _, attestation := range preparation.CheckAttestations {
			fmt.Fprintf(&source, "func %s(t *testing.T) {\n", attestation.NativeCheckRef)
			if fail {
				source.WriteString("\tif subject.Value() != 5 { t.Fatal(\"owner-native validation assertion failed\") }\n")
			} else {
				source.WriteString("\tif subject.Value() != 4 { t.Fatal(\"owner-native validation assertion failed\") }\n")
			}
			source.WriteString("}\n\n")
		}
		path := filepath.Join(workspaceDirectory, filepath.FromSlash(preparation.TargetRef))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func reacquireObservedShellRepairPrerequisites(t *testing.T, ctx context.Context, hub magicevent.Hub, controller *RuntimeController, aggregate RuntimeAggregate, workspaceDirectory string) RuntimeAggregate {
	t.Helper()
	priority := func(prerequisite runtimecontract.RuntimePrerequisite) int {
		switch {
		case prerequisite.Kind == runtimecontract.PrerequisiteKindManifest && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleFile:
			return 0
		case prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleFile:
			return 1
		case prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleFile:
			return 2
		case prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleShell:
			return 3
		default:
			return 100
		}
	}
	for step := 0; step < 12 && aggregate.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite; step++ {
		if len(aggregate.Facts.Prerequisites.Pending) == 0 {
			t.Fatalf("repair prerequisite decision has no pending requirement: %+v", aggregate.Decision)
		}
		prerequisite := aggregate.Facts.Prerequisites.Pending[0]
		for _, candidate := range aggregate.Facts.Prerequisites.Pending[1:] {
			if priority(candidate) < priority(prerequisite) {
				prerequisite = candidate
			}
		}
		var result PrerequisiteResult
		switch {
		case prerequisite.Kind == runtimecontract.PrerequisiteKindManifest && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleFile:
			response := sendObservedManifest(t, ctx, hub, aggregate, prerequisite, workspaceDirectory)
			result = PrerequisiteResult{PrerequisiteID: prerequisite.ID, Manifest: &response.Manifest, Observation: &response.Observation}
		case prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleFile:
			response := sendObservedTargetSnapshot(t, ctx, hub, aggregate, prerequisite, workspaceDirectory)
			result = PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSnapshot: &response.Snapshot, Observation: &response.Observation}
		case prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleFile:
			response := sendObservedShellFileAdmissibility(t, ctx, hub, aggregate, prerequisite, workspaceDirectory)
			result = PrerequisiteResult{PrerequisiteID: prerequisite.ID, OperationAdmissibility: &response.Acquisition.Admissibility, Observation: &response.Acquisition.Observation}
		case prerequisite.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleShell:
			if aggregate.Facts.Resources.Manifest == nil {
				t.Fatalf("Shell admissibility refresh has no current File manifest: %+v", aggregate.Facts.Prerequisites.Pending)
			}
			requirements := runtimeTestShellValidationProofRequirements(t, aggregate.Facts)
			var manifestObservation *runtimecontract.RuntimePrerequisiteObservation
			for index := range aggregate.Facts.Resources.PrerequisiteObservations {
				candidate := aggregate.Facts.Resources.PrerequisiteObservations[index]
				if candidate.Kind == runtimecontract.PrerequisiteKindManifest && candidate.OwnerModuleID == runtimecontract.RuntimeModuleFile &&
					candidate.ContentHash == aggregate.Facts.Resources.Manifest.Revision {
					value := candidate
					manifestObservation = &value
					break
				}
			}
			if manifestObservation == nil {
				t.Fatalf("Shell admissibility refresh has no matching manifest observation: %+v", aggregate.Facts.Resources.PrerequisiteObservations)
			}
			eventResult := hub.Send(magicevent.NewEventWithContext(
				shellevents.TopicAcquireRuntimeOperationAdmissibility,
				runtimecontract.RuntimeModuleOrchestrator,
				runtimecontract.RuntimeModuleShell,
				magicevent.NewHeader(),
				ctx,
				shellevents.AcquireRuntimeOperationAdmissibilityCommand{
					WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
					AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
					PrerequisiteID:      prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID,
					WorkspaceDeliverableDirectory: workspaceDirectory, RequiredKinds: prerequisite.RequiredCapabilities,
					RequiredTargetRefs: append([]string(nil), aggregate.Facts.Resources.Manifest.TargetRefs...),
					ValidationRecipes: runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(
						aggregate.Facts.Activation.EvidencePolicies,
					),
					ValidationProofRequirements: runtimecontract.NormalizeRuntimeValidationProofRequirements(requirements),
					WorkspaceManifestBinding: runtimecontract.RuntimeWorkspaceManifestBinding{
						Manifest: *aggregate.Facts.Resources.Manifest, Observation: *manifestObservation,
					},
					MaxItems: 8,
				},
			))
			if eventResult == nil {
				t.Fatal("Shell owner returned no refreshed admissibility result")
			}
			data, eventErr := eventResult.Get()
			if eventErr != nil {
				t.Fatal(eventErr)
			}
			response, ok := data.(shellevents.AcquireRuntimeOperationAdmissibilityResult)
			if !ok || response.Acquisition.Observation.ObservationID == "" {
				t.Fatalf("Shell refreshed admissibility response = %#v", data)
			}
			result = PrerequisiteResult{
				PrerequisiteID:         prerequisite.ID,
				OperationAdmissibility: &response.Acquisition.Admissibility,
				Observation:            &response.Acquisition.Observation,
			}
		default:
			t.Fatalf("unexpected confirmed-repair prerequisite: %+v", prerequisite)
		}
		var err error
		aggregate, err = controller.commitPrerequisite(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, result)
		if err != nil {
			t.Fatal(err)
		}
	}
	if aggregate.Decision.State == runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("confirmed-repair prerequisite refresh did not converge: %+v", aggregate.Facts.Prerequisites.Pending)
	}
	return aggregate
}

func sendObservedShellFileAdmissibility(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate RuntimeAggregate, prerequisite runtimecontract.RuntimePrerequisite, directory string) fileevents.AcquireRuntimeOperationAdmissibilityResult {
	t.Helper()
	requiredTargets := GroundedRepairTargets(aggregate.Facts)
	result := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleFile,
		magicevent.NewHeader(), ctx, fileevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
			AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID, PrerequisiteID: prerequisite.ID,
			OwnerModuleID: prerequisite.OwnerModuleID, WorkspaceDeliverableDirectory: directory,
			RequiredKinds: prerequisite.RequiredCapabilities, RequiredTargetRefs: requiredTargets, MaxItems: 4,
		},
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(fileevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Acquisition.Observation.ObservationID == "" {
		t.Fatalf("Shell repair File admissibility refresh response = %#v", data)
	}
	for _, targetRef := range requiredTargets {
		if !slices.Contains(response.Acquisition.Admissibility.TargetRefs, targetRef) {
			t.Fatalf("Shell repair File admissibility omitted required target %q: %#v", targetRef, data)
		}
	}
	return response
}

func registerRuntimeOperationReceiptOwner(t *testing.T, hub magicevent.Hub, controller *RuntimeController) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found, err := controller.QueryOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.ExecutionKey, command.Fence, event.Source(), command.GovernanceGrant)
		if err != nil {
			result.Set(nil, runtimeMagicFailure(err))
			return
		}
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		next, receipt, err := controller.commitOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Fence, event.Source(), command.Receipt)
		if err != nil {
			result.Set(nil, runtimeMagicFailure(err))
			return
		}
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt, StateRevision: next.StateRevision}, nil)
	})
	t.Cleanup(func() {
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
		observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
	})
}

type runtimeObservedShellExecutor struct {
	hub                  magicevent.Hub
	deliverableDirectory string
}

func (e runtimeObservedShellExecutor) ExecuteRuntimeOperation(ctx context.Context, request RuntimeOperationExecutionRequest) (RuntimeOperationExecutionResult, error) {
	if request.OwnerCatalog == nil || request.OwnerCatalogObservation == nil {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("test Shell executor requires persisted catalog=%t observation=%t", request.OwnerCatalog != nil, request.OwnerCatalogObservation != nil)
	}
	sealed, err := shellevents.SealRuntimeOperation(request.Operation, *request.OwnerCatalog, shellevents.RuntimeExecutionSealInput{
		CatalogObservation: *request.OwnerCatalogObservation, TargetRole: request.TargetRole,
		ValidationScopeTargetRefs: request.ValidationScopeTargetRefs, EvidencePolicies: request.EvidencePolicies,
		ValidationProofBinding: request.ValidationProofBinding,
	})
	if err != nil {
		return RuntimeOperationExecutionResult{}, fmt.Errorf("seal observed Shell operation: catalog=%s observation=%s content=%s: %w", request.OwnerCatalog.SnapshotID, request.OwnerCatalogObservation.ObservationID, request.OwnerCatalogObservation.ContentHash, err)
	}
	result := e.hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleShell,
		magicevent.NewHeader(), ctx, shellevents.ExecuteOperationCommand{
			RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
				Execution:   runtimeOperationExecutionEnvelopeForTest(request.Execution, request.GovernanceGrant),
				WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
				OperationID: request.Operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
				TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind,
				EvidencePolicies: append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...), WorkspaceDeliverableDirectory: e.deliverableDirectory,
			},
			SealedOperation: sealed,
		},
	))
	if result == nil {
		return RuntimeOperationExecutionResult{}, cd.NewError(cd.Unexpected, "Shell owner returned no execution result")
	}
	responseData, resultErr := result.Get()
	if resultErr != nil {
		return RuntimeOperationExecutionResult{}, resultErr
	}
	response, ok := responseData.(shellevents.ExecuteOperationResult)
	if !ok {
		return RuntimeOperationExecutionResult{}, cd.NewError(cd.Unexpected, "Shell owner returned unexpected execution result")
	}
	status, evidence, governance := orchestratorevents.RuntimeOperationResultFromReceipt(response.Receipt)
	failure := failureFactFromOperationReceiptFeedback(response.Receipt)
	return RuntimeOperationExecutionResult{EventID: orchestratorevents.RuntimeOperationEventID(request.DispatchID, request.Operation.ID), OperationID: request.Operation.ID, EvidenceKind: request.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(request.DispatchID, request.Operation.ID), Failure: failure, Evidence: evidence, Receipt: response.Receipt, Governance: governance}, nil
}

func sendObservedShellExecution(t *testing.T, ctx context.Context, hub magicevent.Hub, command shellevents.ExecuteOperationCommand) shellevents.ExecuteOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(shellevents.TopicExecuteOperation, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleShell, magicevent.NewHeader(), ctx, command))
	if result == nil {
		t.Fatal("Shell owner returned no execution result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(shellevents.ExecuteOperationResult)
	if !ok {
		t.Fatalf("Shell owner result type = %T", data)
	}
	return response
}
