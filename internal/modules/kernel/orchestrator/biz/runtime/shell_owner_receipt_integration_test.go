package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// TestShellOwnerObservedFailureTargetBindsThroughEventHubAndFileSnapshot
// exercises the real Shell owner boundary for the closed failure-target
// protocol. A rejected validation may retarget SM-2 only after the owner
// receipt is CAS-committed, and the resulting target must be reacquired from
// the File owner before it can enter another candidate authority.
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

	aggregate := validationAggregate(t)
	// This fixture exercises the unrestricted shell/command owner boundary,
	// not managed validation.  Remove the managed-recipe declaration inherited
	// from the shared validation fixture so its deliberately generated failure
	// record remains a valid non-recipe Shell operation.
	aggregate.Facts.Activation.EvidencePolicies = nil
	aggregate.Definition.CapabilityContract = []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand}
	aggregate.Facts.Capabilities.ValidationKinds = []string{runtimecontract.StepKindShellCommand}
	aggregate.Facts.Focus.TargetRefs = nil
	aggregate.Facts.Focus.TargetRole = "implementation"
	aggregate.Facts.Focus.RequireTargetGrounding = true
	addObservedShellFailureTargetFreshness(t, ctx, hub, &aggregate, workspaceDirectory)
	decision, err := runtimecontract.SealRuntimeDecision(aggregate.Facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedValidation,
		Focus: aggregate.Facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.StepKindShellCommand},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
			CoverageWorkItemIDs:  aggregate.Facts.Focus.CoverageWorkItemIDs,
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

	record := `WORKORCH_FAILURE_TARGET_V1={"schemaVersion":"workorch.shell.failure_target.v1","targetRole":"implementation","targetRefs":["implementation.go"],"diagnosticClass":"validation_build_failed"}`
	command := "printf '%s\\n' '" + record + "'; exit 7"
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: runtimecontract.RuntimeOperation{
		ID: "validate-with-observed-target", Kind: runtimecontract.StepKindShellCommand,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"validation-entry.sh"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "command", String: runtimeStringPointer("sh")},
			{Name: "args", StringList: []string{"-c", command}},
			{Name: "stateChanging", Boolean: runtimeBoolPointer(false)},
		},
	}}}}}
	sealCandidateForTest(t, repository, candidate)

	persisted, err := controller.ExecuteActiveDispatch(ctx, "ws-1", "run-1", runtimeObservedShellExecutor{hub: hub, deliverableDirectory: workspaceDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Facts.Resources.FailureTarget == nil || persisted.Facts.Resources.FailureTarget.TargetObservation == nil ||
		len(persisted.Facts.Resources.FailureTarget.TargetRefs) != 1 || persisted.Facts.Resources.FailureTarget.TargetRefs[0] != "implementation.go" ||
		persisted.Decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("Shell receipt did not CAS-bind failure target before snapshot acquisition: %+v", persisted)
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
	persisted, err = controller.CommitPrerequisite(ctx, persisted.Facts.WorkspaceID, persisted.Facts.RunID, PrerequisiteResult{
		PrerequisiteID: snapshotPrerequisite.ID, TargetSnapshot: &snapshot.Snapshot, Observation: snapshot.Observation,
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

	// Replaying the exact execution key must query the committed receipt rather
	// than execute the replacement command or manufacture a new observation.
	dispatch := loaded.CompletedDispatches[0]
	execution := dispatch.OperationExecutions[0]
	replayed := sendObservedShellExecution(t, ctx, hub, orchestratorevents.ExecuteShellOperationCommand{
		Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, ExpectedEffectScope: execution.ExpectedEffectScope},
		WorkspaceID: loaded.Facts.WorkspaceID, RunID: loaded.Facts.RunID, DispatchID: dispatch.ID, Transaction: dispatch.Transaction,
		OperationID: "validate-with-observed-target", EvidenceKind: runtimecontract.EvidenceKindValidation, TargetRole: "implementation",
		WorkspaceDeliverableDirectory: workspaceDirectory, TargetRefs: []string{"replacement-entry.sh"},
		Data: orchestratorevents.ShellOperationData{Command: "sh", Args: []string{"-c", "exit 0"}},
	})
	if replayed.Receipt.ExecutionKey != execution.ExecutionKey || replayed.Receipt.Failure == nil || replayed.Receipt.Failure.TargetObservation == nil ||
		len(replayed.Receipt.Failure.TargetRefs) != 1 || replayed.Receipt.Failure.TargetRefs[0] != "implementation.go" {
		t.Fatalf("duplicate Shell execution did not replay the committed receipt: %+v", replayed)
	}
}

// TestShellOwnerCatalogStaleRestartDoesNotReuseFailureTarget proves that the
// owner-catalog revision which authorizes a Shell observation is a real
// freshness fence. A policy revision cannot leave a previously observed
// implementation target active across a candidate retry or process restart.
func TestShellOwnerCatalogStaleRestartDoesNotReuseFailureTarget(t *testing.T) {
	ctx := context.Background()
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "operation_admissibility:shell-policy", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand},
	}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: prerequisite.ID, OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.StepKindShellCommand}, ObservedRevision: "shell-policy-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", Revision: inventory.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)

	facts := controllerFacts()
	facts.RecoveryPolicy.MaxPrerequisiteRefreshes = 2
	targetObservation := &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", TargetRole: "implementation",
		ObservedRevision: inventory.ObservedRevision, TargetRefs: []string{"implementation.go"},
		DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed, ContentHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		EvidenceRefs: []string{"workorch://shell/failure-target/sha256/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
	}
	facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Signature: "shell-failure", Message: "observed failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation}
	facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "shell-failure", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation}
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Initialize(ctx, facts, controllerDefinition(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
		t.Fatal(err)
	}
	stale := runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID, Domain: runtimecontract.RuntimeResourceOwnerCatalog,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", ExpectedRevision: inventory.ObservedRevision,
		ObservedRevision: "shell-policy-revoked", Reason: "runtime shell operation policy changed",
	}
	invalidated, err := controller.InvalidateCandidateAttemptForPrerequisiteStale(ctx, "ws-v2", "run-v2", 0, stale)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restarted.Load(ctx, "ws-v2", "run-v2")
	if err != nil || loaded.StateRevision != invalidated.StateRevision || loaded.Facts.Resources.FailureTarget != nil || loaded.Facts.Failure == nil ||
		len(loaded.Facts.Failure.TargetRefs) != 0 || len(loaded.Facts.Prerequisites.Pending) != 1 || loaded.Facts.Prerequisites.Pending[0].ID != prerequisite.ID ||
		loaded.Facts.RepairDirective != nil || len(invalidated.CandidateAttempts) != 1 || invalidated.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateInvalidated {
		t.Fatalf("restart retained stale Shell failure authority: loaded=%+v invalidated=%+v err=%v", loaded, invalidated, err)
	}
}

func runtimeBoolPointer(value bool) *bool { return &value }

func addObservedShellFailureTargetFreshness(t *testing.T, ctx context.Context, hub magicevent.Hub, aggregate *RuntimeAggregate, workspaceDirectory string) {
	t.Helper()
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "operation_admissibility:shell-observed-target", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleShell,
		magicevent.NewHeader(), ctx, orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand{
			WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, AuthoritySnapshotID: aggregate.Decision.AuthoritySnapshot.ID,
			PrerequisiteID: prerequisite.ID, OwnerModuleID: prerequisite.OwnerModuleID, WorkspaceDeliverableDirectory: workspaceDirectory,
			RequiredKinds: prerequisite.RequiredCapabilities, MaxItems: 4,
		},
	))
	if result == nil {
		t.Fatal("Shell owner returned no operation admissibility result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult)
	if !ok || response.Observation == nil || response.Admissibility.ObservedRevision == "" {
		t.Fatalf("Shell operation admissibility response = %#v", data)
	}
	aggregate.Facts.Resources.OperationAdmissibilities = append(aggregate.Facts.Resources.OperationAdmissibilities, response.Admissibility)
	aggregate.Facts.Resources.PrerequisiteObservations = append(aggregate.Facts.Resources.PrerequisiteObservations, *response.Observation)
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
}

func registerRuntimeOperationReceiptOwner(t *testing.T, hub magicevent.Hub, controller *RuntimeController) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found, err := controller.QueryOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.ExecutionKey, command.Fence, command.GovernanceGrant)
		if err != nil {
			result.Set(nil, runtimeMagicFailure(err))
			return
		}
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found, Receipt: &receipt}, nil)
	})
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		next, receipt, err := controller.CommitOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Fence, command.Receipt)
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
	data, err := orchestratorevents.ShellOperationDataFromRuntimeOperation(request.Operation)
	if err != nil {
		return RuntimeOperationExecutionResult{}, err
	}
	result := e.hub.Send(magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorExecuteShellOperation, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleShell,
		magicevent.NewHeader(), ctx, orchestratorevents.ExecuteShellOperationCommand{
			Execution:   orchestratorevents.RuntimeOperationExecutionEnvelope{ExecutionKey: request.Execution.ExecutionKey, Fence: request.Execution.Fence, ExpectedEffectScope: request.Execution.ExpectedEffectScope, GovernanceGrant: request.GovernanceGrant},
			WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: request.DispatchID, Transaction: request.Transaction,
			OperationID: request.Operation.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), request.Operation.CoverageWorkItemIDs...),
			TargetRefs: append([]string(nil), request.Operation.TargetRefs...), EvidenceKind: request.EvidenceKind, TargetRole: request.TargetRole,
			EvidencePolicies: append([]runtimecontract.RuntimeEvidencePolicy(nil), request.EvidencePolicies...), WorkspaceDeliverableDirectory: e.deliverableDirectory, Data: data,
		},
	))
	if result == nil {
		return RuntimeOperationExecutionResult{}, cd.NewError(cd.Unexpected, "Shell owner returned no execution result")
	}
	responseData, resultErr := result.Get()
	if resultErr != nil {
		return RuntimeOperationExecutionResult{}, resultErr
	}
	response, ok := responseData.(orchestratorevents.ExecuteShellOperationResult)
	if !ok {
		return RuntimeOperationExecutionResult{}, cd.NewError(cd.Unexpected, "Shell owner returned unexpected execution result")
	}
	status, evidence, failure, governance := orchestratorevents.RuntimeOperationResultFromReceipt(response.Receipt)
	return RuntimeOperationExecutionResult{EventID: response.EventID, OperationID: response.OperationID, EvidenceKind: response.EvidenceKind, Status: status, SourceRef: response.SourceRef, Failure: failure, Evidence: evidence, Receipt: response.Receipt, Governance: governance}, nil
}

func sendObservedShellExecution(t *testing.T, ctx context.Context, hub magicevent.Hub, command orchestratorevents.ExecuteShellOperationCommand) orchestratorevents.ExecuteShellOperationResult {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorExecuteShellOperation, runtimecontract.RuntimeModuleOrchestrator, runtimecontract.RuntimeModuleShell, magicevent.NewHeader(), ctx, command))
	if result == nil {
		t.Fatal("Shell owner returned no execution result")
	}
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(orchestratorevents.ExecuteShellOperationResult)
	if !ok {
		t.Fatalf("Shell owner result type = %T", data)
	}
	return response
}
