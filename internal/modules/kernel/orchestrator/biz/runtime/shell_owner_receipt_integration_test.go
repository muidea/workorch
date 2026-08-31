package runtime

import (
	"context"
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
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })

	workspaceDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDirectory, "implementation.go"), []byte("package fixture\n"), 0o644); err != nil {
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

	record := `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":["implementation.go"],"diagnosticClass":"validation_build_failed"}`
	script := "printf '%s\\n' '" + record + "' '? github.com/user/lsp-bridge/internal/config [no test files]'; exit 7"
	policy := runtimeManagedValidationEvidencePolicy()
	policy.CommandName = "sh"
	policy.RecipeArgs = []string{"-c", script}
	policy.ZeroWorkMarkers = []string{"[no test files]"}
	policy.WorkEvidenceMarkers = []string{"=== RUN", "--- PASS:", "ok  "}
	policy.QualityOnZeroWork = string(common.EvidenceQualityInconclusive)
	policy.EvidenceReason = "configured zero-work marker matched"
	policy.EvidenceTags = []string{"validation", "zero_work"}
	ownerPreflight := validationAggregate(t)
	ownerPreflight.Facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{policy}
	shellCatalog := addObservedShellFailureTargetFreshness(t, ctx, hub, &ownerPreflight, workspaceDirectory)
	if len(shellCatalog.SelectorRefs) == 0 {
		t.Fatalf("Shell observed-target catalog has no owner selector: %+v", shellCatalog)
	}
	shellSelectorRef := shellCatalog.SelectorRefs[0]
	aggregate := validationAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{policy}
		aggregate.Definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindManagedValidationSession}
		aggregate.Facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
		aggregate.Facts.Focus.TargetRefs = nil
		aggregate.Facts.Focus.TargetRole = "implementation"
		aggregate.Facts.Focus.TargetSelectorRef = shellSelectorRef
		aggregate.Facts.Focus.TargetSelectorOwnerRef = runtimecontract.RuntimeModuleShell
		aggregate.Facts.Focus.TargetSelectorSubjectRef = "implementation"
		aggregate.Facts.Focus.RequireTargetGrounding = true
		runtimeTestInstallObservedShellPreflight(aggregate, ownerPreflight, shellCatalog)
	})
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
		TargetRefs:          []string{"validation-entry.sh"},
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
	if err != nil || len(sealedValidationScope) != 2 || !slices.Contains(sealedValidationScope, "implementation.go") || !slices.Contains(sealedValidationScope, "validation-entry.sh") ||
		len(sealedOperation.TargetRefs) != 1 || sealedOperation.TargetRefs[0] != "validation-entry.sh" {
		t.Fatalf("asymmetric validation seal = entry=%v scope=%v err=%v", sealedOperation.TargetRefs, sealedValidationScope, err)
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
		t.Fatalf("validation-owner receipt bypassed File repair-surface confirmation: %+v", persisted)
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
		!slices.Equal(failureObservation.ValidationScopeTargetRefs, []string{"implementation.go", "validation-entry.sh"}) {
		t.Fatalf("Shell observation lost asymmetric sealed scope: %+v", failureObservation)
	}
	if _, err := controller.recordValidationFailureObservation(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, failureObservation); err != nil {
		t.Fatalf("validation observation identity fence: %v", err)
	}
	// The semantic helper snapshots only the pre-validation target set. The
	// later owner confirmation is a new owner revision and must not inherit those
	// fixture snapshots as repair authority.
	repository.aggregate.Facts.Resources.TargetSnapshots = nil
	persisted.Facts.Resources.TargetSnapshots = nil
	confirmationResult := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicConfirmRuntimeRepairSurface, runtimecontract.RuntimeModuleOrchestrator, orchestratorevents.PrerequisiteCommandDestination,
		magicevent.NewHeader(), ctx, fileevents.ConfirmRuntimeRepairSurfaceCommand{
			WorkspaceID: persisted.Facts.WorkspaceID, RunID: persisted.Facts.RunID,
			WorkspaceDeliverableDirectory: workspaceDirectory, Observation: failureObservation,
		},
	))
	if confirmationResult == nil {
		t.Fatal("File owner returned no repair-surface confirmation")
	}
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
		!slices.Equal(persisted.Facts.Resources.FailureTarget.TargetRefs, []string{"implementation.go"}) ||
		persisted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("owner confirmation did not CAS-bind failure target before snapshot acquisition: aggregate=%+v err=%v", persisted, err)
	}
	var snapshotPrerequisite runtimecontract.RuntimePrerequisite
	for _, pending := range persisted.Facts.Prerequisites.Pending {
		if pending.Kind == runtimecontract.PrerequisiteKindTargetSnapshot && pending.OwnerModuleID == runtimecontract.RuntimeModuleFile && pending.SubjectRef == "implementation.go" {
			snapshotPrerequisite = pending
			break
		}
	}
	if snapshotPrerequisite.ID == "" {
		t.Fatalf("observed Shell target did not request File snapshot: %+v", persisted.Facts.Prerequisites.Pending)
	}
	snapshot := sendObservedTargetSnapshot(t, ctx, hub, persisted, snapshotPrerequisite, workspaceDirectory)
	persisted, err = controller.commitPrerequisite(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: snapshotPrerequisite.ID, TargetSnapshot: &snapshot.Snapshot, Observation: &snapshot.Observation,
	})
	if err != nil || len(persisted.Facts.Resources.TargetSnapshots) != 1 || persisted.Facts.Resources.TargetSnapshots[0].TargetRef != "implementation.go" {
		t.Fatalf("File snapshot was not CAS-bound after observed Shell target: aggregate=%+v err=%v", persisted, err)
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
	catalog := runtimeOwnerCatalogForOperation(loaded.Facts.Resources.OperationAdmissibilities, loaded.Facts.Resources.PrerequisiteObservations, operation, execution.AdmissibilitySnapshotID, execution.AcquisitionBinding)
	observation := runtimeOwnerCatalogObservationForOperation(loaded.Facts.Resources.PrerequisiteObservations, catalog)
	if catalog == nil || observation == nil {
		t.Fatalf("completed Shell dispatch lost owner catalog binding: %+v", loaded.Facts.Resources)
	}
	validationScopeTargetRefs, err := runtimeValidationScopeTargetRefs(dispatch, operation)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := shellevents.SealRuntimeOperation(operation, *catalog, shellevents.RuntimeExecutionSealInput{
		CatalogObservation: *observation, TargetRole: dispatch.FocusTargetRole,
		ValidationScopeTargetRefs: validationScopeTargetRefs,
		EvidencePolicies:          loaded.Facts.Activation.EvidencePolicies,
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
		!slices.Equal(diagnostic.ValidationFailureObservation.ValidationScopeTargetRefs, []string{"implementation.go", "validation-entry.sh"}) {
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
	patchType := "file_write"
	repairedContent := "package fixture\n// repaired through confirmed surface\n"
	baseHash := currentSnapshot.ContentHash
	repairCandidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{Operations: []runtimecontract.StateChangingOperation{{Operation: runtimecontract.RuntimeOperation{
		Kind:                runtimecontract.RuntimeBlockKindPatchApply,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), loaded.Facts.Focus.CoverageWorkItemIDs...),
		TargetRefs:          []string{"implementation.go"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &repairedContent},
			{Name: "baseHash", String: &baseHash},
		},
	}}}}}
	repairCandidate = runtimeTestBindCandidateToCurrentFocus(t, loaded, repairCandidate)
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
	repairOperation := repairCandidate.StateChange.Operations[0].Operation
	// patch/apply has a static File-owner convergence declaration, so this
	// fixture exercises the legal static seal path instead of inventing a
	// mutable File catalog that SM-2 did not request. The actual effect and
	// receipt still cross the real File/EventHub boundary below.
	sealedRepair := sealCandidateWithFeasibilitiesForTest(t, repository, repairCandidate, nil)
	repairOperation = sealedRepair.ActiveDispatch.Candidate.StateChange.Operations[0].Operation
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
}

// TestShellOwnerSelectorOnlyValidationNeverCreatesRepairObservation proves the
// owner boundary for an executable selector-only managed-validation seal. A
// closed failure-target marker remains only a diagnostic hint when the SM-3
// authorization has no finite scope; neither first delivery nor receipt replay
// may turn the current File manifest into historical repair authority.
func TestShellOwnerSelectorOnlyValidationNeverCreatesRepairObservation(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })

	workspaceDirectory := t.TempDir()
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

	record := `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.shell.failure_target","targetRole":"implementation","targetRefs":["implementation.go"],"diagnosticClass":"validation_build_failed"}`
	policy := runtimeManagedValidationEvidencePolicy()
	policy.CommandName = "sh"
	policy.RecipeArgs = []string{"-c", "printf '%s\\n' '" + record + "'; exit 7"}
	ownerPreflight := validationAggregate(t)
	ownerPreflight.Facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{policy}
	shellCatalog := addSelectorOnlyShellFailureTargetFreshness(t, ctx, hub, &ownerPreflight, workspaceDirectory)
	aggregate := validationOnlyAggregateWithPreparation(t, func(aggregate *RuntimeAggregate) {
		aggregate.Facts.Activation.EvidencePolicies = []runtimecontract.RuntimeEvidencePolicy{policy}
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
	catalog := runtimeOwnerCatalogForOperation(loaded.Facts.Resources.OperationAdmissibilities, loaded.Facts.Resources.PrerequisiteObservations, operation, execution.AdmissibilitySnapshotID, execution.AcquisitionBinding)
	if catalog == nil {
		t.Fatalf("selector-only completed dispatch lost Shell catalog: %+v", loaded.Facts.Resources.OperationAdmissibilities)
	}
	catalogObservation := runtimeOwnerCatalogObservationForOperation(loaded.Facts.Resources.PrerequisiteObservations, catalog)
	if catalogObservation == nil {
		t.Fatalf("selector-only completed dispatch lost Shell catalog observation: %+v", loaded.Facts.Resources.PrerequisiteObservations)
	}
	sealedOperation, err := shellevents.SealRuntimeOperation(operation, *catalog, shellevents.RuntimeExecutionSealInput{
		CatalogObservation: *catalogObservation, TargetRole: dispatch.FocusTargetRole,
		ValidationScopeTargetRefs: reloadedScope, EvidencePolicies: loaded.Facts.Activation.EvidencePolicies,
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

func addObservedShellFailureTargetFreshness(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate *RuntimeAggregate, workspaceDirectory string) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	manifestPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: "manifest:shell-observed-target", Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	manifest := sendObservedManifest(t, ctx, hub, *aggregate, manifestPrerequisite, workspaceDirectory)
	aggregate.Facts.Resources.Manifest = &manifest.Manifest
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, manifest.Observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, manifestPrerequisite)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "operation_admissibility:shell-observed-target", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleShell,
		magicevent.NewHeader(), ctx, shellevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			PrerequisiteID: prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID, WorkspaceDeliverableDirectory: workspaceDirectory,
			RequiredKinds:      prerequisite.RequiredCapabilities,
			RequiredTargetRefs: append([]string(nil), manifest.Manifest.TargetRefs...),
			ValidationRecipes: runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(
				aggregate.Facts.Activation.EvidencePolicies,
			),
			WorkspaceManifestBinding: runtimecontract.RuntimeWorkspaceManifestBinding{Manifest: manifest.Manifest, Observation: manifest.Observation},
			MaxItems:                 4,
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
	aggregate.Facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	aggregate.Facts.Resources.PrerequisiteObservations = nil
	for _, rawObservation := range observed.Facts.Resources.PrerequisiteObservations {
		observation := rawObservation.Normalized()
		if observation.PrerequisiteID == catalog.PrerequisiteID ||
			(observation.Kind == runtimecontract.PrerequisiteKindManifest && observation.OwnerModuleID == runtimecontract.RuntimeModuleFile) {
			aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, observation)
		}
	}
	aggregate.Facts.Prerequisites.Satisfied = nil
	for _, prerequisite := range observed.Facts.Prerequisites.Satisfied {
		if prerequisite.ID == catalog.PrerequisiteID ||
			(prerequisite.Kind == runtimecontract.PrerequisiteKindManifest && prerequisite.OwnerModuleID == runtimecontract.RuntimeModuleFile) {
			aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
		}
	}
}

func addSelectorOnlyShellFailureTargetFreshness(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate *RuntimeAggregate, workspaceDirectory string) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	manifestPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: "manifest:shell-selector-only", Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	manifest := sendObservedManifest(t, ctx, hub, *aggregate, manifestPrerequisite, workspaceDirectory)
	aggregate.Facts.Resources.Manifest = &manifest.Manifest
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, manifest.Observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, manifestPrerequisite)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "operation_admissibility:shell-selector-only", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleShell,
		magicevent.NewHeader(), ctx, shellevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			PrerequisiteID: prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID, WorkspaceDeliverableDirectory: workspaceDirectory,
			RequiredKinds: prerequisite.RequiredCapabilities,
			ValidationRecipes: runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(
				aggregate.Facts.Activation.EvidencePolicies,
			),
			WorkspaceManifestBinding: runtimecontract.RuntimeWorkspaceManifestBinding{Manifest: manifest.Manifest, Observation: manifest.Observation},
			MaxItems:                 4,
		},
	))
	if result == nil {
		t.Fatal("Shell owner returned no selector-only operation admissibility result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(shellevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Acquisition.Observation.ObservationID == "" || response.Acquisition.Admissibility.ObservedRevision == "" {
		t.Fatalf("Shell selector-only operation admissibility response = %#v", data)
	}
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
	})
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
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
