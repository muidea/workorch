package mcp

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindToolCall = modulecommon.StepKindToolCall
const StepKindResourceRead = modulecommon.StepKindResourceRead
const StepKindPromptGet = modulecommon.StepKindPromptGet

type Dependencies struct {
	EventHub magicevent.Hub
}

type Module struct {
	runtimeAdapter     *mcpbiz.RuntimeAdapter
	capabilityObserver magicevent.SimpleObserver
	eventHub           magicevent.Hub
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.runtimeAdapter = mcpbiz.NewRuntimeAdapter(mcpEventServerGetter{hub: deps.EventHub})
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
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteMCPOperation, m.handleMCPOperation)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.capabilityObserver != nil {
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteMCPOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	}
	m.capabilityObserver = nil
	if m.runtimeAdapter != nil {
		m.runtimeAdapter.Close()
	}
	m.runtimeAdapter = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleMCPOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteMCPOperationCommand)
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
		result.Set(orchestratorevents.ExecuteMCPOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), mcpbiz.RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		Kind: command.OperationKind, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory: command.WorkspaceInternalDirectory, TargetRefs: command.TargetRefs,
		Data: command.Data, EvidencePolicies: command.EvidencePolicies,
	})
	response := orchestratorevents.ExecuteMCPOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimecontract.RuntimeOperationExecutionStatusAccepted,
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: operationResult.Evidence,
		Data: orchestratorevents.MCPOperationResultData{
			Status: operationResult.Status, Stage: operationResult.Stage, Operation: operationResult.Operation,
			ServerID: operationResult.ServerID, Tool: operationResult.Tool, URI: operationResult.URI, Prompt: operationResult.Prompt,
			Summary: operationResult.Summary, Transport: operationResult.Transport, Endpoint: operationResult.Endpoint,
			DurationMillis: operationResult.DurationMillis, StatusCode: operationResult.StatusCode, Stderr: operationResult.Stderr,
			FailureCode: operationResult.FailureCode, FailureMessage: operationResult.FailureMessage, TargetRefs: operationResult.TargetRefs,
			Diagnostics: orchestratorevents.MCPFailureDiagnosticsData{
				Operation: operationResult.Diagnostics.Operation, ServerID: operationResult.Diagnostics.ServerID, Tool: operationResult.Diagnostics.Tool,
				URI: operationResult.Diagnostics.URI, Prompt: operationResult.Diagnostics.Prompt, TargetRef: operationResult.Diagnostics.TargetRef,
				FailureTargets: append([]string(nil), operationResult.Diagnostics.FailureTargets...),
			},
		},
	}
	if executeErr != nil {
		response.Status = runtimecontract.RuntimeOperationExecutionStatusRejected
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, command.OperationKind, operationResult.FailureCode, operationResult.FailureMessage, operationResult.TargetRefs)
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectUnknown}
	if response.Status == runtimecontract.RuntimeOperationExecutionStatusAccepted {
		observation.ObservedEffectScope = command.Execution.ExpectedEffectScope
		observation.Changed = runtimecontract.RuntimeEffectIsDurable(command.Execution.ExpectedEffectScope)
	}
	receipt := orchestratorevents.NewRuntimeOperationReceiptWithObservation(command.Execution, command.OperationID, response.Status, operationResult.TargetRefs, response.Evidence, response.Failure, nil, observation)
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
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindToolCall, StepKindResourceRead, StepKindPromptGet)
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
	serverIDs, revision, err := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory, err := orchestratorevents.NewRuntimeOperationAdmissibility(command.PrerequisiteID, ModuleID, command.RequiredKinds, nil, serverIDs, []string{"configured-mcp-server", "owner-execution-policy"}, revision)
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
	serverIDs, revision, revisionErr := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if revisionErr != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "mcp_server_catalog_unavailable", revisionErr.Error())
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	if revision != command.Admissibility.ObservedRevision {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime mcp operation admissibility snapshot is stale")
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	data, err := orchestratorevents.MCPOperationDataFromRuntimeOperation(operation)
	if err != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "mcp_operation_invalid", err.Error())
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	if !containsMCPAdmissibleServer(serverIDs, data.ServerID) {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "mcp_server_not_admissible", "runtime mcp operation server is absent from the owner admissibility inventory")
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	err = m.runtimeAdapter.ProbeOperation(event.Context(), mcpbiz.RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID("probe/" + operation.ID), Kind: operation.Kind,
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory, TargetRefs: operation.TargetRefs, Data: data,
	})
	if err != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "mcp_prerequisite_unavailable", err.Error())
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: true}, nil)
}

func containsMCPAdmissibleServer(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}

type mcpEventServerGetter struct {
	hub magicevent.Hub
}

func (g mcpEventServerGetter) ListServers(ctx context.Context) ([]*mcpmodel.Server, error) {
	if g.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(
		mcpevents.TopicMCPCommandListServers,
		ModuleID,
		mcpevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		mcpevents.ListServersCommand{},
	)
	result := g.hub.Send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, err := result.Get()
	if err != nil {
		return nil, err
	}
	response, ok := data.(mcpevents.MCPCommandResult)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return append([]*mcpmodel.Server(nil), response.Servers...), nil
}

func (g mcpEventServerGetter) GetServer(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g.hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(
		mcpevents.TopicMCPCommandGetServer,
		ModuleID,
		mcpevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		mcpevents.GetServerCommand{ServerID: id},
	)
	result := g.hub.Send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, err := result.Get()
	if err != nil {
		return nil, err
	}
	response, ok := data.(mcpevents.MCPCommandResult)
	if !ok || response.Server == nil {
		return nil, common.ErrNotFound
	}
	return response.Server, nil
}
