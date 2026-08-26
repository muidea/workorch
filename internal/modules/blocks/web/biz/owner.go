package web

import (
	"context"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
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
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine), runtimeAdapter: NewRuntimeAdapterFromConfigLoaderWithRevision(loadProviderConfigsFromWorkorchConfig, workorchconfig.Revision)}, nil
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
	m.SubscribeFunc(webevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(webevents.TopicAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(webevents.TopicProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.SubscribeFunc(webevents.TopicProbeRuntimeSemanticProofFeasibility, m.handleRuntimeSemanticProofFeasibility)
	m.SubscribeFunc(webevents.TopicExecuteOperation, m.handleWebOperation)
	m.started = true
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.UnsubscribeFunc(webevents.TopicExecuteOperation)
	m.UnsubscribeFunc(webevents.TopicAcquireRuntimeCapabilities)
	m.UnsubscribeFunc(webevents.TopicAcquireRuntimeOperationAdmissibility)
	m.UnsubscribeFunc(webevents.TopicProbeRuntimeOperationFeasibility)
	m.UnsubscribeFunc(webevents.TopicProbeRuntimeSemanticProofFeasibility)
	m.runtimeAdapter = nil
	m.started = false
}

func (m *Biz) handleWebOperation(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator {
		setCapabilityError(result, fmt.Errorf("runtime Web operation requires Orchestrator source"))
		return
	}
	command, ok := event.Data().(webevents.ExecuteOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	if strings.TrimSpace(command.WorkspaceInternalDirectory) == "" {
		setCapabilityError(result, fmt.Errorf("runtime Web operation requires workspace internal directory"))
		return
	}
	if err := command.Execution.Validate(); err != nil {
		setCapabilityError(result, err)
		return
	}
	operation, authorization, err := command.SealedOperation.ValidateForExecution(command.RuntimeOperationExecutionContext)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if command.Execution.ExpectedEffectScope != runtimecontract.RuntimeEffectNone {
		setCapabilityError(result, fmt.Errorf("runtime Web operation requires sealed none effect"))
		return
	}
	data, err := webevents.OperationDataFromRuntimeOperation(operation)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil {
		setCapabilityError(result, err)
		return
	} else if found {
		result.Set(webevents.ExecuteOperationResult{Receipt: receipt}, nil)
		return
	}
	runtimeRequest := RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		Kind: operation.Kind, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory: command.WorkspaceInternalDirectory,
		ProviderID:                 data.ProviderID, Query: data.Query, Locale: data.Locale, Recency: data.Recency,
		Domains: data.Domains, ExcludeDomains: data.ExcludeDomains, MaxResults: data.MaxResults,
		SafeSearch: data.SafeSearch, URL: data.URL,
		GovernanceGrant: command.Execution.GovernanceGrant,
		MaxBytes:        data.MaxBytes, ExtractMode: data.ExtractMode, TargetRefs: command.TargetRefs,
		EvidencePolicies: command.EvidencePolicies,
	}
	operationResult, currentRevision, executeErr := m.runtimeAdapter.ExecuteOperationAtOwnerRevision(event.Context(), runtimeRequest, authorization.OwnerRevision)
	if executeErr != nil && currentRevision == "" {
		setCapabilityError(result, executeErr)
		return
	}
	if currentRevision != authorization.OwnerRevision {
		message := "Web provider configuration changed after operation seal"
		failure := &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "operation_admissibility_stale",
			Signature: "web-owner-catalog-stale:" + authorization.CatalogObservation.ObservationID, Message: message,
		}
		receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusRejected, nil,
			runtimecontract.RuntimeOperationEvidence{Summary: message}, failure, nil,
			orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone})
		receipt.PrerequisiteStale = &runtimecontract.RuntimePrerequisiteStale{
			PrerequisiteID: authorization.PrerequisiteID, ObservationID: authorization.CatalogObservation.ObservationID,
			Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: ModuleID, SubjectRef: "configured_web_providers",
			ExpectedRevision: authorization.OwnerRevision, ObservedRevision: currentRevision, Reason: message,
		}
		committed, commitErr := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
		if commitErr != nil {
			setCapabilityError(result, commitErr)
			return
		}
		result.Set(webevents.ExecuteOperationResult{Receipt: committed}, nil)
		return
	}
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	evidence := operationResult.Evidence
	var failure *runtimecontract.FailureFact
	governance := operationResult.Governance
	if executeErr != nil && governance == nil {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		message := safeWebOperationFailureMessage(operationResult.FailureCode)
		failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, operation.Kind, operationResult.FailureCode, message, nil)
		evidence = runtimecontract.RuntimeOperationEvidence{Summary: message}
	}
	if governance != nil {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		evidence = runtimecontract.RuntimeOperationEvidence{Summary: "Web operation requires exact approval before external access"}
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectNone}
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, operationResult.TargetRefs, evidence, failure, governance, observation)
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(webevents.ExecuteOperationResult{Receipt: committed}, nil)
}

func safeWebOperationFailureMessage(code string) string {
	switch strings.TrimSpace(code) {
	case modulecommon.ErrorTimeout:
		return "Web owner request timed out"
	case modulecommon.ErrorProviderUnavailable:
		return "Web owner provider is unavailable"
	case modulecommon.ErrorContentTooLarge:
		return "Web owner response exceeded its byte bound"
	case modulecommon.ErrorPolicyDenied:
		return "Web owner network policy denied the request"
	case modulecommon.ErrorInvalidRequest:
		return "Web owner rejected an invalid bounded request"
	default:
		return "Web owner request failed"
	}
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(webevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := webevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(webevents.AcquireRuntimeOperationAdmissibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := webevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	providerIDs, revision, err := m.runtimeAdapter.OperationAdmissibility()
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(command.PrerequisiteID, ModuleID, command.RequiredKinds, nil, providerIDs, []string{"configured-web-provider", "owner-network-preflight", "owner-evidence-ref", "owner-execution-policy"}, revision)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory.OperationPolicies = []runtimecontract.RuntimeOperationPolicyFact{
		{
			ID: "operation-policy/web-search", OwnerModuleID: ModuleID, OperationKind: runtimecontract.StepKindWebSearch,
			SubjectRef: "configured-web-provider", EffectScope: runtimecontract.RuntimeEffectNone,
			IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		},
		{
			ID: "operation-policy/web-fetch", OwnerModuleID: ModuleID, OperationKind: runtimecontract.StepKindWebFetch,
			SubjectRef: "owner-controlled-network-target", EffectScope: runtimecontract.RuntimeEffectNone,
			IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		},
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := webAdmissibilityObservation(command.PrerequisiteID, inventory)
	result.Set(webevents.AcquireRuntimeOperationAdmissibilityResult{Acquisition: runtimecontract.RuntimeOperationAdmissibilityAcquisition{WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID, Admissibility: inventory, Observation: observation}}, nil)
}

func setRuntimeOperationFeasibilityResult(result magicevent.Result, command webevents.ProbeRuntimeOperationFeasibilityCommand, failure *runtimecontract.FailureFact, stale *runtimecontract.RuntimePrerequisiteStale) {
	category := ownerFeasibilityCategory(failure)
	observation, err := webevents.NewProbeRuntimeOperationFeasibilityResult(command, category, failure, stale, nil, nil)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(observation, nil)
}

func (m *Biz) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(webevents.ProbeRuntimeOperationFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := webevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	operation := command.Operation.Normalized()
	_, revision, revisionErr := m.runtimeAdapter.OperationAdmissibility()
	if revisionErr != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "web_provider_configuration_unavailable", revisionErr.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	if revision != command.Admissibility.ObservedRevision {
		stale := webAdmissibilityStale(command.Observation, command.Admissibility, revision)
		if stale != nil {
			setRuntimeOperationFeasibilityResult(result, command, nil, stale)
			return
		}
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime web operation admissibility snapshot is stale")
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	data, err := webevents.OperationDataFromRuntimeOperation(operation)
	if err != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "web_operation_invalid", err.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	err = m.runtimeAdapter.ProbeOperation(event.Context(), RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID("probe/" + operation.ID), Kind: operation.Kind,
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		ProviderID:                    data.ProviderID, Query: data.Query, Locale: data.Locale, Recency: data.Recency,
		Domains: data.Domains, ExcludeDomains: data.ExcludeDomains, MaxResults: data.MaxResults, SafeSearch: data.SafeSearch,
		URL: data.URL, MaxBytes: data.MaxBytes, ExtractMode: data.ExtractMode, TargetRefs: operation.TargetRefs,
	})
	if err != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "web_operation_infeasible", "runtime Web owner rejected the bounded operation preflight")
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	setRuntimeOperationFeasibilityResult(result, command, nil, nil)
}

func (m *Biz) handleRuntimeSemanticProofFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(webevents.ProbeRuntimeSemanticProofFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := webevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	providerIDs, revision, err := m.runtimeAdapter.OperationAdmissibility()
	if err != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, command.OwnerCatalog.ObservedRevision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "web_catalog_unavailable", err.Error(), command.OwnerCatalog.ObservedRevision, "", nil))
		return
	}
	if revision != command.OwnerCatalog.ObservedRevision {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityStale, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_catalog_stale", "runtime Web semantic catalog is stale", command.OwnerCatalog.ObservedRevision, revision, nil))
		return
	}
	for _, selector := range command.Proposal.Target.OwnerSelectorRefs {
		if !containsWebProvider(providerIDs, selector) {
			setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
				runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "web_selector_not_admissible", "runtime semantic Web selector is absent from the owner catalog", "", revision, []string{selector}))
			return
		}
	}
	if strings.TrimSpace(command.Proposal.ValidationRecipeID) != "" || len(command.Proposal.RequiredCheckRefs) != 0 {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "web_semantic_proof_unavailable", "Web provider availability is not an independent semantic validation proof", "", revision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal)))
		return
	}
	setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityFeasible, revision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal), nil)
}

func setRuntimeSemanticProofFeasibilityResult(result magicevent.Result, command webevents.ProbeRuntimeSemanticProofFeasibilityCommand, status runtimecontract.RuntimeSemanticFeasibilityStatus, revision string, targetRefs []string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) {
	response, err := webevents.NewRuntimeSemanticProofFeasibilityResult(command, status, revision, targetRefs, diagnostic)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func containsWebProvider(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
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
