package ssh

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	sshbiz "github.com/muidea/workorch/internal/modules/blocks/ssh/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindCommand = modulecommon.StepKindCommand

type Module struct {
	runtimeAdapter     *sshbiz.RuntimeAdapter
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
	m.runtimeAdapter = sshbiz.NewRuntimeAdapter()
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
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteSSHOperation, m.handleSSHOperation)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.capabilityObserver != nil {
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteSSHOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	}
	m.capabilityObserver = nil
	m.runtimeAdapter = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleSSHOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteSSHOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	if receipt, found, err := orchestratorevents.QueryPersistedRuntimeOperationReceipt(event.Context(), m.eventHub, ModuleID, command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setCapabilityError(result, err)
		return
	} else if found {
		status, evidence, failure, _ := orchestratorevents.RuntimeOperationResultFromReceipt(receipt)
		result.Set(orchestratorevents.ExecuteSSHOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), sshbiz.RuntimeCommandRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory:    command.WorkspaceInternalDirectory,
		Host:                          command.Data.Host, Port: command.Data.Port, Username: command.Data.Username,
		Password: command.Data.Password, PrivateKey: command.Data.PrivateKey, PrivateKeyPath: command.Data.PrivateKeyPath,
		Command: command.Data.Command, TimeoutMillis: command.Data.TimeoutMillis, EvidencePolicies: command.EvidencePolicies,
	})
	response := orchestratorevents.ExecuteSSHOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimecontract.RuntimeOperationExecutionStatusAccepted,
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: operationResult.Evidence,
		Data: orchestratorevents.SSHOperationResultData{
			Status: operationResult.Status, Host: operationResult.Host, Port: operationResult.Port, Username: operationResult.Username,
			Command: operationResult.Command, Stdout: operationResult.Stdout, Stderr: operationResult.Stderr, ExitCode: operationResult.ExitCode,
			DurationMillis: operationResult.DurationMillis, TimeoutMillis: operationResult.TimeoutMillis, AuthMethod: operationResult.AuthMethod,
			OutputTruncated: operationResult.OutputTruncated, StdoutTruncated: operationResult.StdoutTruncated, StderrTruncated: operationResult.StderrTruncated,
			StdoutBytes: operationResult.StdoutBytes, StderrBytes: operationResult.StderrBytes,
			FailureCode: operationResult.FailureCode, FailureMessage: operationResult.FailureMessage,
		},
	}
	if executeErr != nil {
		response.Status = runtimecontract.RuntimeOperationExecutionStatusRejected
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, runtimecontract.StepKindSSHCommand, operationResult.FailureCode, operationResult.FailureMessage, command.TargetRefs)
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectUnknown}
	if response.Status == runtimecontract.RuntimeOperationExecutionStatusAccepted {
		observation.ObservedEffectScope = command.Execution.ExpectedEffectScope
		observation.Changed = runtimecontract.RuntimeEffectIsDurable(command.Execution.ExpectedEffectScope)
	}
	receipt := orchestratorevents.NewRuntimeOperationReceiptWithObservation(command.Execution, command.OperationID, response.Status, command.TargetRefs, response.Evidence, response.Failure, nil, observation)
	committed, err := orchestratorevents.CommitPersistedRuntimeOperationReceipt(event.Context(), m.eventHub, ModuleID, command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	response.Receipt = committed
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

func (m *Module) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory, err := orchestratorevents.NewRuntimeOperationAdmissibility(command.PrerequisiteID, ModuleID, command.RequiredKinds, nil, nil, []string{"remote-host-parameter", "owner-execution-policy"}, "ssh-owner")
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{PrerequisiteID: command.PrerequisiteID, Admissibility: inventory}, nil)
}

func (m *Module) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ProbeRuntimeOperationFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	operation := command.Operation.Normalized()
	data, err := orchestratorevents.SSHOperationDataFromRuntimeOperation(operation)
	if err != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "ssh_operation_invalid", err.Error())
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	err = m.runtimeAdapter.ProbeOperation(sshbiz.RuntimeCommandRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID("probe/" + operation.ID),
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		Host:                          data.Host, Port: data.Port, Username: data.Username, Password: data.Password,
		PrivateKey: data.PrivateKey, PrivateKeyPath: data.PrivateKeyPath, Command: data.Command, TimeoutMillis: data.TimeoutMillis,
	})
	if err != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "ssh_prerequisite_unavailable", err.Error())
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: true}, nil)
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}
