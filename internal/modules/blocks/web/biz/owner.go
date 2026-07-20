package web

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindSearch = modulecommon.StepKindSearch
const StepKindFetch = modulecommon.StepKindFetch

type Biz struct {
	basebiz.Base
	runtimeAdapter *RuntimeAdapter
	started        bool
}

type Dependencies struct {
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine), runtimeAdapter: NewRuntimeAdapterFromConfigLoaderWithRevision(loadProviderConfigsFromWorkorchConfig, workorchconfig.Revision, webApprovalGetter{hub: eventHub})}, nil
}

func NewWithDependencies(deps Dependencies) (*Biz, error) {
	return New(deps.EventHub, deps.BackgroundRoutine)
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.SubscribeFunc(orchestratorevents.TopicOrchestratorExecuteWebOperation, m.handleWebOperation)
	m.started = true
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorExecuteWebOperation)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorAcquireRuntimeOperationAdmissibility)
	m.UnsubscribeFunc(orchestratorevents.TopicOrchestratorProbeRuntimeOperationFeasibility)
	m.runtimeAdapter = nil
	m.started = false
}

func (m *Biz) handleWebOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteWebOperationCommand)
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
		result.Set(orchestratorevents.ExecuteWebOperationResult{Receipt: receipt, EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID, EvidenceKind: command.EvidenceKind, Status: status, SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: evidence, Failure: failure}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		Kind: command.OperationKind, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory: command.WorkspaceInternalDirectory,
		ProviderID:                 command.Data.ProviderID, Query: command.Data.Query, Locale: command.Data.Locale, Recency: command.Data.Recency,
		Domains: command.Data.Domains, ExcludeDomains: command.Data.ExcludeDomains, MaxResults: command.Data.MaxResults,
		SafeSearch: command.Data.SafeSearch, ApprovalID: command.Data.ApprovalID, URL: command.Data.URL,
		MaxBytes: command.Data.MaxBytes, ExtractMode: command.Data.ExtractMode, TargetRefs: command.TargetRefs,
		EvidencePolicies: command.EvidencePolicies,
	})
	response := orchestratorevents.ExecuteWebOperationResult{
		EventID: orchestratorevents.RuntimeOperationEventID(command.DispatchID, command.OperationID), OperationID: command.OperationID,
		EvidenceKind: command.EvidenceKind, Status: runtimecontract.RuntimeOperationExecutionStatusAccepted,
		SourceRef: orchestratorevents.RuntimeOperationSourceRef(command.DispatchID, command.OperationID), Evidence: operationResult.Evidence,
		Data: orchestratorevents.WebOperationResultData{
			Status: operationResult.Status, Stage: operationResult.Stage, ProviderID: operationResult.ProviderID,
			Query: operationResult.Query, URL: operationResult.URL, FinalURL: operationResult.FinalURL,
			Title: operationResult.Title, Summary: operationResult.Summary, ContentType: operationResult.ContentType,
			ContentHash: operationResult.ContentHash, RetrievedAt: operationResult.RetrievedAt, PublishedAt: operationResult.PublishedAt,
			Source: operationResult.Source, Size: operationResult.Size, ResultCount: operationResult.ResultCount,
			Truncated: operationResult.Truncated, ApprovalID: operationResult.ApprovalID, ApprovalNeeded: operationResult.ApprovalNeeded,
			DurationMillis: operationResult.DurationMillis, FailureCode: operationResult.FailureCode,
			FailureMessage: operationResult.FailureMessage, TargetRefs: operationResult.TargetRefs,
		},
	}
	if executeErr != nil {
		response.Status = runtimecontract.RuntimeOperationExecutionStatusRejected
		response.Failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, command.OperationKind, operationResult.FailureCode, operationResult.FailureMessage, operationResult.TargetRefs)
	}
	var governance *runtimecontract.RuntimeGovernanceRequirement
	if operationResult.ApprovalNeeded || (response.Failure != nil && response.Failure.Code == "approval_required") {
		subject := operationResult.URL
		if subject == "" {
			subject = operationResult.Query
		}
		if subject == "" {
			subject = command.OperationKind
		}
		reason := operationResult.FailureMessage
		if reason == "" {
			reason = "web operation requires approval before side effects"
		}
		governance = &runtimecontract.RuntimeGovernanceRequirement{Kind: runtimecontract.GovernanceApproval, SubjectRef: subject, Reason: reason, RequestedAction: command.OperationKind, RiskLevel: "external"}
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone}
	if runtimecontract.RuntimeEffectIsDurable(command.Execution.ExpectedEffectScope) {
		observation.ObservedEffectScope = runtimecontract.RuntimeEffectUnknown
		if response.Status == runtimecontract.RuntimeOperationExecutionStatusAccepted {
			observation.ObservedEffectScope = command.Execution.ExpectedEffectScope
			observation.Changed = true
		}
	}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, response.Status, operationResult.TargetRefs, response.Evidence, response.Failure, governance, observation)
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	response.Receipt = committed
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindSearch, StepKindFetch)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeOperationAdmissibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	providerIDs, revision, err := m.runtimeAdapter.OperationAdmissibility()
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory, err := orchestratorevents.NewRuntimeOperationAdmissibility(command.PrerequisiteID, ModuleID, command.RequiredKinds, nil, providerIDs, []string{"configured-web-provider", "owner-execution-policy"}, revision)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := webAdmissibilityObservation(command.PrerequisiteID, inventory)
	result.Set(orchestratorevents.AcquireRuntimeOperationAdmissibilityResult{PrerequisiteID: command.PrerequisiteID, Admissibility: inventory, Observation: &observation}, nil)
}

func (m *Biz) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
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
	_, revision, revisionErr := m.runtimeAdapter.OperationAdmissibility()
	if revisionErr != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "web_provider_configuration_unavailable", revisionErr.Error())
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	if revision != command.Admissibility.ObservedRevision {
		stale := webAdmissibilityStale(command.Observation, command.Admissibility, revision)
		if stale != nil {
			result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Stale: stale}, nil)
			return
		}
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime web operation admissibility snapshot is stale")
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	data, err := orchestratorevents.WebOperationDataFromRuntimeOperation(operation)
	if err != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "web_operation_invalid", err.Error())
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	err = m.runtimeAdapter.ProbeOperation(event.Context(), RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID("probe/" + operation.ID), Kind: operation.Kind,
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		ProviderID:                    data.ProviderID, Query: data.Query, Locale: data.Locale, Recency: data.Recency,
		Domains: data.Domains, ExcludeDomains: data.ExcludeDomains, MaxResults: data.MaxResults, SafeSearch: data.SafeSearch,
		ApprovalID: data.ApprovalID, URL: data.URL, MaxBytes: data.MaxBytes, ExtractMode: data.ExtractMode, TargetRefs: operation.TargetRefs,
	})
	if err != nil {
		failure := orchestratorevents.RuntimeOperationInfeasibleFailure(operation, "web_operation_infeasible", err.Error())
		result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: false, Failure: failure}, nil)
		return
	}
	result.Set(orchestratorevents.ProbeRuntimeOperationFeasibilityResult{OperationID: operation.ID, AdmissibilitySnapshotID: command.Admissibility.SnapshotID, Feasible: true}, nil)
}

func webAdmissibilityObservation(prerequisiteID string, inventory runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: ModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: ModuleID, SubjectRef: "configured_web_providers", Revision: inventory.ObservedRevision}}}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func webAdmissibilityStale(observation *runtimecontract.RuntimePrerequisiteObservation, inventory runtimecontract.RuntimeOperationAdmissibility, observedRevision string) *runtimecontract.RuntimePrerequisiteStale {
	if observation == nil {
		return nil
	}
	value := observation.Normalized()
	for _, dependency := range value.Dependencies {
		if dependency.Domain == runtimecontract.RuntimeResourceProviderConfig && dependency.OwnerModuleID == ModuleID && dependency.SubjectRef == "configured_web_providers" && dependency.Revision == inventory.ObservedRevision {
			return &runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: value.PrerequisiteID, ObservationID: value.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: observedRevision, Reason: "runtime web provider configuration changed"}
		}
	}
	return nil
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}

func loadProviderConfigsFromWorkorchConfig() (string, map[string]webmodels.ProviderConfig, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return "", nil, err
	}
	return webproviderconfig.LoadProviderConfigs(cfg.WebSearch.DefaultProvider, cfg.WebSearchProviderValueSets())
}

type webApprovalGetter struct {
	hub magicevent.Hub
}

func (g webApprovalGetter) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	return g.GetForRun(ctx, id, "")
}

func (g webApprovalGetter) GetForRun(ctx context.Context, id common.ApprovalID, runID common.RunID) (*approvalmodel.Approval, error) {
	if g.hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(
		approvalevents.TopicApprovalCommandGet,
		ModuleID,
		approvalevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		approvalevents.GetApprovalCommand{RunID: runID, ApprovalID: id},
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
	response, ok := data.(approvalevents.ApprovalCommandResult)
	if !ok || response.Approval == nil {
		return nil, common.ErrNotFound
	}
	return response.Approval, nil
}
