package shell

import (
	"context"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindCommand = modulecommon.StepKindCommand

type Biz struct {
	basebiz.Base
	runtimeAdapter *RuntimeAdapter
}

type Dependencies struct {
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
	// RuntimeAdapter is an explicit owner-local dependency used by focused
	// runtimes and tests. Production leaves it nil so configuration remains the
	// sole construction source.
	RuntimeAdapter *RuntimeAdapter
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine, deps ...Dependencies) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	m := &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine)}
	if len(deps) > 0 && deps[0].RuntimeAdapter != nil {
		m.runtimeAdapter = deps[0].RuntimeAdapter
		return m, nil
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return nil, err
	}
	adapter, err := NewRuntimeAdapterFromTimeoutString(cfg.Runtime.ShellCommandTimeout)
	if err != nil {
		return nil, err
	}
	m.runtimeAdapter = adapter
	return m, nil
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorExecuteShellOperation, m.handleShellOperation)
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorExecuteShellOperation)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	m.runtimeAdapter = nil
}

func (m *Biz) handleShellOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteShellOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setCapabilityError(result, err)
		return
	} else if found {
		status, evidence, failure, _ := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
		result.Set(orchestratorevents.ExecuteShellOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), RuntimeCommandRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		StepID:                        common.StepID(command.DispatchID + "/" + command.OperationID),
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory:    command.WorkspaceInternalDirectory,
		Command:                       command.Data.Command, Args: command.Data.Args, TimeoutMillis: command.Data.TimeoutMillis,
		StateChanging: command.Data.StateChanging, TargetRef: command.Data.TargetRef, TargetRefs: command.Data.TargetRefs,
		VerificationScope: command.Data.VerificationScope,
		BoundedReason:     command.Data.BoundedReason, ValidationScratchDirectory: command.Data.ValidationScratchDirectory,
		ValidationScratchAutoCleanup:    command.Data.ValidationScratchAutoCleanup,
		ValidationScratchAutoCleanupSet: command.Data.ValidationScratchAutoCleanupSet,
		ValidationEvidence:              command.Data.ValidationEvidence, ManagedValidation: command.Data.ManagedValidation,
		EvidencePolicies: command.EvidencePolicies,
	})
	response := orchestratorevents.ExecuteShellOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimecontract.RuntimeOperationExecutionStatusAccepted,
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: operationResult.Evidence,
		Data: orchestratorevents.ShellOperationResultData{
			Status: operationResult.Status, Stage: operationResult.Stage, Command: operationResult.Command, Args: operationResult.Args,
			ActualCommand: operationResult.ActualCommand, WorkingDirectory: operationResult.WorkingDirectory,
			Stdout: operationResult.Stdout, Stderr: operationResult.Stderr, ExitCode: operationResult.ExitCode,
			DurationMillis: operationResult.DurationMillis, TimeoutMillis: operationResult.TimeoutMillis,
			OutputTruncated: operationResult.OutputTruncated, StdoutTruncated: operationResult.StdoutTruncated, StderrTruncated: operationResult.StderrTruncated,
			StdoutBytes: operationResult.StdoutBytes, StderrBytes: operationResult.StderrBytes,
			FailureCode: operationResult.FailureCode, FailureMessage: operationResult.FailureMessage,
			Summary: operationResult.Summary, Reason: operationResult.Reason, FailureReason: operationResult.FailureReason,
			UnavailableCommand: operationResult.UnavailableCommand, CapabilityMissing: operationResult.CapabilityMissing,
			ZeroWork: operationResult.ZeroWork, EvidenceQuality: operationResult.EvidenceQuality, EvidenceReason: operationResult.EvidenceReason,
			EvidenceTags: operationResult.EvidenceTags, TargetRefs: operationResult.TargetRefs,
			CommandNormalization: operationResult.CommandNormalization, MaxOutputBytes: operationResult.MaxOutputBytes,
			ValidationEvidence: operationResult.ValidationEvidence, ManagedValidation: operationResult.ManagedValidation,
			ValidationResources: operationResult.ValidationResources, RuntimeDiagnostics: operationResult.RuntimeDiagnostics,
			ValidationFailureDiagnostic: operationResult.ValidationFailureDiagnostic,
		},
	}
	if executeErr != nil {
		response.Status = runtimecontract.RuntimeOperationExecutionStatusRejected
		// Shell currently has no structured protocol that can bind a related
		// failure target to an owner, subject, role and revision. The command
		// target remains execution lineage only; it must not become repair
		// authority through a local heuristic.
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindShellCommand, operationResult.FailureCode, operationResult.FailureMessage, nil)
		if operationResult.ValidationFailureDiagnostic != nil {
			diagnostic := operationResult.ValidationFailureDiagnostic.Clone()
			response.Failure.ValidationDiagnostic = &diagnostic
		}
	}
	observation := shellMutationReceiptObservation(command.Data.StateChanging, response.Status, operationResult)
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, response.Status, operationResult.TargetRefs, response.Evidence, response.Failure, nil, observation)
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	response.Receipt = committed
	result.Set(response, nil)
}

// shellMutationReceiptObservation turns only the Shell owner's bounded,
// before/after observations into resource changes. The resource identity uses
// File as the canonical owner of workspace targets/tree: Shell observes the
// mutation, while Orchestrator performs the neutral intersection against File
// and Shell prerequisites. No File service/repository is consulted here.
func shellMutationReceiptObservation(stateChanging bool, status runtimecontract.RuntimeOperationExecutionStatus, result RuntimeCommandResult) orchestratorevents.RuntimeOperationReceiptObservation {
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone}
	if !stateChanging {
		return observation
	}
	observation.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
	if status != runtimecontract.RuntimeOperationExecutionStatusAccepted {
		return observation
	}
	changes := make([]runtimecontract.RuntimeResourceChange, 0, len(result.WorkspaceTargetChanges)+1)
	for _, change := range result.WorkspaceTargetChanges {
		targetRef := strings.TrimSpace(change.TargetRef)
		if targetRef == "" || change.BeforeRevision == change.AfterRevision || strings.TrimSpace(change.AfterRevision) == "" {
			continue
		}
		changes = append(changes, runtimecontract.RuntimeResourceChange{
			Domain: runtimecontract.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: targetRef, BeforeRevision: change.BeforeRevision, AfterRevision: change.AfterRevision,
		})
	}
	if fingerprint := result.WorkspaceStateChangeFingerprint; strings.TrimSpace(fingerprint.Before) != "" &&
		strings.TrimSpace(fingerprint.After) != "" && fingerprint.Before != fingerprint.After {
		changes = append(changes, runtimecontract.RuntimeResourceChange{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", BeforeRevision: fingerprint.Before, AfterRevision: fingerprint.After,
		})
	}
	if len(changes) == 0 {
		observation.NoOp = true
		return observation
	}
	observation.ObservedEffectScope = runtimecontract.RuntimeEffectWorkspaceDurable
	observation.Changed = true
	observation.ResourceChanges = changes
	return observation
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindCommand, runtimecontract.RuntimeBlockKindManagedValidationSession)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand)
	if !ok || m.runtimeAdapter == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" ||
		command.PrerequisiteID == "" || command.OwnerModuleID != ModuleID || command.WorkspaceDeliverableDirectory == "" || len(command.RequiredKinds) == 0 {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	inventory, err := m.runtimeAdapter.AcquireRuntimeOperationAdmissibility(command.WorkspaceDeliverableDirectory, command.PrerequisiteID, command.RequiredKinds)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := m.runtimeAdapter.runtimeAdmissibilityObservation(command.WorkspaceDeliverableDirectory, command.PrerequisiteID, inventory, command.RequiredTargetRefs)
	result.Set(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{PrerequisiteID: command.PrerequisiteID, Admissibility: inventory, Observation: &observation}, nil)
}

func (m *Biz) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil || command.WorkspaceID == "" || command.RunID == "" || command.Transaction.AuthoritySnapshotID == "" || command.WorkspaceDeliverableDirectory == "" {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	operation := command.Operation.Normalized()
	descriptor, known := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !known || descriptor.OwnerModuleID != ModuleID || command.Admissibility.OwnerModuleID != ModuleID {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	failure, stale := m.runtimeAdapter.ProbeRuntimeOperationFeasibilityWithObservation(command.WorkspaceDeliverableDirectory, command.Admissibility, operation, command.Observation)
	result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{
		OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID,
		Feasible: failure == nil && stale == nil, Failure: failure, Stale: stale,
	}, nil)
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}
