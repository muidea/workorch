package shell

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	shellbiz "github.com/muidea/workorch/internal/modules/blocks/shell/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindCommand = modulecommon.StepKindCommand

type Module struct {
	runtimeAdapter     *shellbiz.RuntimeAdapter
	capabilityObserver magicevent.SimpleObserver
	eventHub           magicevent.Hub
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

type Dependencies struct {
	EventHub magicevent.Hub
}

func (m *Module) Configure(deps ...Dependencies) error {
	if len(deps) > 0 {
		m.eventHub = deps[0].EventHub
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return err
	}
	adapter, err := shellbiz.NewRuntimeAdapterFromTimeoutString(cfg.Runtime.ShellCommandTimeout)
	if err != nil {
		return err
	}
	m.runtimeAdapter = adapter
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.capabilityObserver != nil {
		return nil
	}
	m.capabilityObserver = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteShellOperation, m.handleShellOperation)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.capabilityObserver != nil {
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteShellOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	}
	m.capabilityObserver = nil
	m.runtimeAdapter = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleShellOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteShellOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), shellbiz.RuntimeCommandRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		StepID:                        common.StepID(command.DispatchID + "/" + command.OperationID),
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory:    command.WorkspaceInternalDirectory,
		Command:                       command.Data.Command, Args: command.Data.Args, TimeoutMillis: command.Data.TimeoutMillis,
		StateChanging: command.Data.StateChanging, TargetRef: command.Data.TargetRef, TargetRefs: command.Data.TargetRefs,
		RelatedFailureTargets: command.Data.RelatedFailureTargets, VerificationScope: command.Data.VerificationScope,
		BoundedReason: command.Data.BoundedReason, ValidationScratchDirectory: command.Data.ValidationScratchDirectory,
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
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindShellCommand, operationResult.FailureCode, operationResult.FailureMessage, operationResult.TargetRefs)
	}
	result.Set(response, nil)
}

func (m *Module) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindCommand)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}
