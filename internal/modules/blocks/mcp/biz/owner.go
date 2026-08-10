package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	registryevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	workorcheventpayload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindToolCall = modulecommon.StepKindToolCall
const StepKindResourceRead = modulecommon.StepKindResourceRead
const StepKindPromptGet = modulecommon.StepKindPromptGet

type Biz struct {
	basebiz.Base
	runtimeAdapter *RuntimeAdapter
}

type Dependencies struct {
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
	// RuntimeAdapter is an explicit owner-local dependency for focused
	// runtimes and tests. Production leaves it nil so the typed MCP registry
	// remains the sole adapter construction source.
	RuntimeAdapter *RuntimeAdapter
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{
		Base:           basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine),
		runtimeAdapter: NewRuntimeAdapter(mcpEventServerGetter{hub: eventHub}),
	}, nil
}

func NewWithDependencies(deps Dependencies) (*Biz, error) {
	owner, err := New(deps.EventHub, deps.BackgroundRoutine)
	if err != nil {
		return nil, err
	}
	if deps.RuntimeAdapter != nil {
		owner.runtimeAdapter = deps.RuntimeAdapter
	}
	return owner, nil
}

func (m *Biz) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	m.SubscribeFunc(mcpevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.SubscribeFunc(mcpevents.TopicAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility)
	m.SubscribeFunc(mcpevents.TopicProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility)
	m.SubscribeFunc(mcpevents.TopicProbeRuntimeSemanticProofFeasibility, m.handleRuntimeSemanticProofFeasibility)
	m.SubscribeFunc(mcpevents.TopicQueryRuntimeCWIReconciliation, m.handleRuntimeCWIReconciliationQuery)
	m.SubscribeFunc(mcpevents.TopicExecuteOperation, m.handleMCPOperation)
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.UnsubscribeFunc(mcpevents.TopicExecuteOperation)
	m.UnsubscribeFunc(mcpevents.TopicAcquireRuntimeCapabilities)
	m.UnsubscribeFunc(mcpevents.TopicAcquireRuntimeOperationAdmissibility)
	m.UnsubscribeFunc(mcpevents.TopicProbeRuntimeOperationFeasibility)
	m.UnsubscribeFunc(mcpevents.TopicProbeRuntimeSemanticProofFeasibility)
	m.UnsubscribeFunc(mcpevents.TopicQueryRuntimeCWIReconciliation)
	if m.runtimeAdapter != nil {
		m.runtimeAdapter.Close()
	}
	m.runtimeAdapter = nil
}

// handleRuntimeCWIReconciliationQuery reports MCP readiness separately from
// behavior proof. Catalog/endpoint/tool-surface readiness is useful diagnostic
// context, but it is never a satisfied CWI. A behavior proof requires a
// binding-declared executable probe (including its exact input contract),
// which this reconciliation command intentionally does not invent.
func (m *Biz) handleRuntimeCWIReconciliationQuery(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(mcpevents.QueryRuntimeCWIReconciliationCommand)
	if !ok || m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if !runtimeCWIReconciliationBindingHasMCPProof(command.Binding) {
		observation := mcpReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, command.WorkspaceBaselineRevision, nil, nil, "binding has no MCP behavior recipe", "behavior_probe_unavailable")
		result.Set(mcpevents.QueryRuntimeCWIReconciliationResult{Observation: observation}, nil)
		return
	}
	serverIDs, _, revision, err := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if err != nil {
		observation := mcpReconciliationObservation(command, runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, command.WorkspaceBaselineRevision, nil, nil, "MCP server catalog is unavailable", "mcp_catalog_unavailable")
		result.Set(mcpevents.QueryRuntimeCWIReconciliationResult{Observation: observation}, nil)
		return
	}
	serverSet := make(map[string]struct{}, len(serverIDs))
	for _, id := range serverIDs {
		serverSet[strings.TrimSpace(id)] = struct{}{}
	}
	var readyTargets []string
	var readyChecks []string
	allSurfaceReady := true
	for _, recipeID := range command.Binding.ValidationRecipeIDs {
		serverID, tool, parseErr := parseMCPReconciliationRecipe(recipeID)
		if parseErr != nil {
			allSurfaceReady = false
			continue
		}
		if _, found := serverSet[serverID]; !found {
			allSurfaceReady = false
			continue
		}
		probeErr := m.runtimeAdapter.ProbeOperation(event.Context(), RuntimeOperationRequest{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID,
			StepID:                        common.StepID("reconciliation/" + command.RequestID + "/" + command.Binding.BindingID + "/" + recipeID),
			Kind:                          runtimecontract.StepKindMCPToolCall,
			WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
			TargetRefs:                    command.Binding.TargetRefs,
			Data:                          mcpevents.OperationData{ServerID: serverID, Tool: tool},
		})
		if probeErr != nil {
			allSurfaceReady = false
			continue
		}
		readyTargets = append(readyTargets, serverID+"/"+tool)
		readyChecks = append(readyChecks, recipeID)
	}
	status := runtimecontract.RuntimeCWIReconciliationObservationUnverifiable
	message, code := "MCP endpoint/tool surface is ready but no behavior proof was executed", "behavior_probe_unavailable"
	if !allSurfaceReady {
		message, code = "MCP binding tool surface is unavailable", "mcp_tool_surface_unavailable"
	}
	// Preserve the readiness facts in the observation while keeping Status
	// unverifiable. Orchestrator may use them for prerequisite diagnostics but
	// can never convert them into CWI credit.
	observation := mcpReconciliationObservation(command, status, revision, readyTargets, readyChecks, message, code)
	result.Set(mcpevents.QueryRuntimeCWIReconciliationResult{Observation: observation}, nil)
}

func runtimeCWIReconciliationBindingHasMCPProof(binding runtimecontract.RuntimeCWIReconciliationBindingRef) bool {
	for _, kind := range binding.AllowedOperationKinds {
		switch runtimecontract.CanonicalRuntimeBlockKind(kind) {
		case runtimecontract.StepKindMCPToolCall, runtimecontract.StepKindMCPResourceRead, runtimecontract.StepKindMCPPromptGet:
			return true
		}
	}
	return len(binding.ValidationRecipeIDs) != 0
}

func parseMCPReconciliationRecipe(recipeID string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(recipeID), "/", 4)
	if len(parts) != 4 || parts[0] != "mcp" || parts[1] != "tool_call" || parts[2] == "" || parts[3] == "" {
		return "", "", fmt.Errorf("MCP reconciliation recipe %q is not an executable tool probe", recipeID)
	}
	return parts[2], parts[3], nil
}

func mcpReconciliationObservation(command mcpevents.QueryRuntimeCWIReconciliationCommand, status runtimecontract.RuntimeCWIReconciliationObservationStatus, revision string, targetRefs, checkRefs []string, message, code string) runtimecontract.RuntimeCWIReconciliationObservation {
	if strings.TrimSpace(revision) == "" {
		revision = command.WorkspaceBaselineRevision
	}
	value := runtimecontract.RuntimeCWIReconciliationObservation{
		RequestID: command.RequestID, WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		Generation: command.Generation, CoverageWorkItemID: command.Binding.CoverageWorkItemID,
		BindingID: command.Binding.BindingID, BindingHash: command.Binding.BindingHash,
		OwnerModuleID: ModuleID, ObservedRevision: revision, Status: status,
		TargetRefs: append([]string(nil), targetRefs...), CheckRefs: append([]string(nil), checkRefs...),
	}
	if status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
		value.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: ModuleID,
			OriginalClass: "reconciliation", OriginalCode: code, OriginalMessage: message,
			Signature: "mcp-reconciliation/" + code, TargetRefs: append([]string(nil), targetRefs...), ObservedRevision: revision,
		}
	}
	return value.Normalized()
}

func (m *Biz) handleMCPOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(mcpevents.ExecuteOperationCommand)
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
		result.Set(mcpevents.ExecuteOperationResult{Receipt: receipt}, nil)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.ExecuteOperation(event.Context(), RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		Kind: command.OperationKind, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory: command.WorkspaceInternalDirectory, TargetRefs: command.TargetRefs,
		TargetRole: command.TargetRole,
		Data:       command.Data, EvidencePolicies: command.EvidencePolicies,
	})
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	evidence := operationResult.Evidence
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		failure = orchestratorevents.RuntimeOperationFailureWithTargetObservation(command.Transaction, command.OperationKind, operationResult.FailureCode, operationResult.FailureMessage, mcpFailureTargets(operationResult), mcpFailureTargetObservation(operationResult))
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectUnknown}
	if status == runtimecontract.RuntimeOperationExecutionStatusAccepted {
		if capabilities, capabilityErr := m.runtimeAdapter.EffectiveCapabilities(event.Context()); capabilityErr == nil {
			if capability, found := findEffectiveMCPCapability(capabilities, command.Data.ServerID, command.Data.Tool); found {
				observation.ObservedEffectScope = runtimecontract.NormalizeRuntimeEffectScope(runtimecontract.RuntimeEffectScope(capability.EffectScope))
			}
		}
		observation.Changed = runtimecontract.RuntimeEffectIsDurable(observation.ObservedEffectScope)
	}
	attachMCPToolValidationObservation(&evidence, status, command.OperationKind, command.EvidenceKind, command.Data.ServerID, command.Data.Tool)
	receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, status, operationResult.TargetRefs, evidence, failure, nil, observation)
	committed, err := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(mcpevents.ExecuteOperationResult{Receipt: committed}, nil)
}

// mcpFailureTargets preserves only the bounded failure surface actually
// observed by the MCP owner. An operation target identifies what was queried;
// without an explicit owner observation there is no related failure target.
func mcpFailureTargets(result RuntimeOperationResult) []string {
	return normalizeMCPTargetRefs(result.Diagnostics.FailureTargets)
}

func mcpFailureTargetObservation(result RuntimeOperationResult) *runtimecontract.RuntimeFailureTargetObservation {
	targets := mcpFailureTargets(result)
	observation := runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: ModuleID, SubjectRef: result.ServerID, TargetRole: result.Diagnostics.TargetRole,
		ObservedRevision: result.OwnerRevision, TargetRefs: targets,
	}.Normalized()
	if err := observation.Validate(); err != nil {
		return nil
	}
	return &observation
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(mcpevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := mcpevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Biz) handleRuntimeOperationAdmissibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(mcpevents.AcquireRuntimeOperationAdmissibilityCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := mcpevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	serverIDs, recipeRefs, revision, err := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(command.PrerequisiteID, ModuleID, command.RequiredKinds, nil, serverIDs, []string{"configured-mcp-server", "owner-execution-policy"}, revision)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory.ValidationRecipeRefs = recipeRefs
	if capabilities, capabilityErr := m.runtimeAdapter.EffectiveCapabilities(event.Context()); capabilityErr == nil {
		inventory.MCPCapabilities = runtimeMCPCapabilityContexts(capabilities)
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := mcpAdmissibilityObservation(command.PrerequisiteID, inventory)
	result.Set(mcpevents.AcquireRuntimeOperationAdmissibilityResult{Acquisition: runtimecontract.RuntimeOperationAdmissibilityAcquisition{WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID, Admissibility: inventory, Observation: observation}}, nil)
}

func runtimeMCPCapabilityContexts(items []*mcpmodel.Capability) []runtimecontract.RuntimeMCPCapabilityContext {
	ret := make([]runtimecontract.RuntimeMCPCapabilityContext, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, runtimecontract.SealMCPCapabilityEffect(runtimecontract.RuntimeMCPCapabilityContext{
			ID: item.ID, ServerID: item.ServerID, Kind: item.Kind, Name: item.Name, DisplayName: item.DisplayName,
			Description: item.Description, Category: item.Category, Tags: append([]string(nil), item.Tags...), UsageHint: item.UsageHint,
			InputSummary: item.InputSummary, OutputSummary: item.OutputSummary, StageHints: append([]string(nil), item.StageHints...),
			RiskLevel: item.RiskLevel, EffectScope: item.EffectScope, IdempotencyClass: item.IdempotencyClass,
			URI: item.URI, Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy, Status: item.Status,
		}))
	}
	return ret
}

func setRuntimeOperationFeasibilityResult(result magicevent.Result, command mcpevents.ProbeRuntimeOperationFeasibilityCommand, failure *runtimecontract.FailureFact, stale *runtimecontract.RuntimePrerequisiteStale) {
	category := ownerFeasibilityCategory(failure)
	observation, err := mcpevents.NewProbeRuntimeOperationFeasibilityResult(command, category, failure, stale, nil, nil)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(observation, nil)
}

func (m *Biz) handleRuntimeOperationFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(mcpevents.ProbeRuntimeOperationFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := mcpevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	operation := command.Operation.Normalized()
	serverIDs, _, revision, revisionErr := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if revisionErr != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, mcpOperationCatalogUnavailableCode(revisionErr), revisionErr.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	if revision != command.Admissibility.ObservedRevision {
		stale := mcpAdmissibilityStale(command.Observation, command.Admissibility, revision)
		if stale != nil {
			setRuntimeOperationFeasibilityResult(result, command, nil, stale)
			return
		}
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "operation_admissibility_stale", "runtime mcp operation admissibility snapshot is stale")
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	data, err := mcpevents.OperationDataFromRuntimeOperation(operation)
	if err != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "mcp_operation_invalid", err.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	if !containsMCPAdmissibleServer(serverIDs, data.ServerID) {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "mcp_server_not_admissible", "runtime mcp operation server is absent from the owner admissibility inventory")
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	err = m.runtimeAdapter.ProbeOperation(event.Context(), RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID("probe/" + operation.ID), Kind: operation.Kind,
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory, TargetRefs: operation.TargetRefs, Data: data,
	})
	if err != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "mcp_prerequisite_unavailable", err.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	setRuntimeOperationFeasibilityResult(result, command, nil, nil)
}

func (m *Biz) handleRuntimeSemanticProofFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(mcpevents.ProbeRuntimeSemanticProofFeasibilityCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := mcpevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	serverIDs, recipeRefs, revision, err := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if err != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, command.OwnerCatalog.ObservedRevision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, mcpSemanticCatalogUnavailableCode(err), err.Error(), command.OwnerCatalog.ObservedRevision, "", nil))
		return
	}
	if revision != command.OwnerCatalog.ObservedRevision {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityStale, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_catalog_stale", "runtime MCP semantic catalog is stale", command.OwnerCatalog.ObservedRevision, revision, nil))
		return
	}
	if !containsMCPAdmissibleServer(recipeRefs, command.Proposal.ValidationRecipeID) {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_recipe_not_admissible", "runtime semantic MCP validation recipe is absent from the owner catalog", "", revision, nil))
		return
	}
	capabilities, capabilityErr := m.runtimeAdapter.EffectiveCapabilities(event.Context())
	if capabilityErr != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_capability_catalog_unavailable", capabilityErr.Error(), "", revision, nil))
		return
	}
	serverID, tool, parseErr := parseMCPValidationRecipe(command.Proposal.ValidationRecipeID)
	if parseErr != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_recipe_invalid", parseErr.Error(), "", revision, nil))
		return
	}
	capability, found := findEffectiveMCPCapability(capabilities, serverID, tool)
	if !found {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_recipe_not_admissible", fmt.Sprintf("recipe %q is not present in the effective owner capability catalog", command.Proposal.ValidationRecipeID), "", revision, []string{command.Proposal.ValidationRecipeID}))
		return
	}
	// A semantic behavior probe is intentionally narrower than an operation
	// dispatch. It may invoke a tool only when the MCP registry owner has
	// explicitly declared that invocation non-durable and safe to replay. This
	// keeps probe execution owner-local without creating a hidden durable effect
	// or a second operation-receipt path before SM-2 seals a binding.
	effect := runtimecontract.NormalizeRuntimeEffectScope(runtimecontract.RuntimeEffectScope(capability.EffectScope))
	idempotency := runtimecontract.NormalizeRuntimeOperationIdempotencyClass(runtimecontract.RuntimeOperationIdempotencyClass(capability.IdempotencyClass))
	if !runtimecontract.RuntimeEffectIsNonDurable(effect) || idempotency != runtimecontract.RuntimeOperationIdempotencySafeToReplay {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_semantic_proof_unavailable", "MCP semantic behavior probe requires a non-durable safe-to-replay capability", "", revision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal)))
		return
	}
	if schemaErr := validateMCPBehaviorProbeSchema(capability.Schema); schemaErr != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_semantic_proof_unavailable", schemaErr.Error(), "", revision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal)))
		return
	}
	for _, selector := range command.Proposal.Target.OwnerSelectorRefs {
		if !containsMCPAdmissibleServer(serverIDs, selector) {
			setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
				runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_selector_not_admissible", "runtime semantic MCP selector is absent from the owner catalog", "", revision, []string{selector}))
			return
		}
	}
	// OperationAdmissibility and EffectiveCapabilities are owner-local typed
	// reads. Re-check the catalog revision immediately before the behavior
	// probe so a registry update between those reads cannot produce a feasible
	// fact from mixed snapshots (and, importantly, cannot invoke the endpoint).
	_, _, verifiedRevision, verifyErr := m.runtimeAdapter.OperationAdmissibility(event.Context())
	if verifyErr != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, mcpSemanticCatalogUnavailableCode(verifyErr), verifyErr.Error(), revision, "", nil))
		return
	}
	if verifiedRevision != revision {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityStale, verifiedRevision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_catalog_stale", "runtime MCP semantic catalog changed during owner proof acquisition", revision, verifiedRevision, nil))
		return
	}
	if recipeID := strings.TrimSpace(command.Proposal.ValidationRecipeID); recipeID != "" {
		probeErr := m.probeSemanticValidationRecipe(event.Context(), command, serverID, tool)
		if probeErr != nil {
			setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
				runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_semantic_proof_unavailable", probeErr.Error(), "", revision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal)))
			return
		}
		// The probe itself is owner-local and side-effect bounded, but its receipt
		// must still be tied to the same catalog revision that authorized it.
		_, _, observedAfterProbe, revisionErr := m.runtimeAdapter.OperationAdmissibility(event.Context())
		if revisionErr != nil {
			setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
				runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_catalog_unavailable", revisionErr.Error(), revision, "", nil))
			return
		}
		if observedAfterProbe != revision {
			setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityStale, observedAfterProbe, nil,
				runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_catalog_stale", "runtime MCP semantic catalog changed during owner behavior probe", revision, observedAfterProbe, nil))
			return
		}
	}
	setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityFeasible, revision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal), nil)
}

func mcpOperationCatalogUnavailableCode(err error) string {
	var unavailable *runtimecontract.RuntimeCapabilityCatalogUnavailableError
	if errors.As(err, &unavailable) {
		return "mcp_capability_catalog_unavailable"
	}
	return "mcp_server_catalog_unavailable"
}

func mcpSemanticCatalogUnavailableCode(err error) string {
	var unavailable *runtimecontract.RuntimeCapabilityCatalogUnavailableError
	if errors.As(err, &unavailable) {
		return "mcp_capability_catalog_unavailable"
	}
	return "mcp_catalog_unavailable"
}

// validateMCPBehaviorProbeSchema is deliberately conservative.  A semantic
// behavior probe has no provider-authored input values, so it may only invoke
// a capability whose sealed schema is a well-formed object and whose complete
// schema (including simple combinators) has no required input.  This is an
// owner-local safety gate: malformed or input-requiring schemas are reported as
// typed unavailable facts and never reach the MCP transport.
func validateMCPBehaviorProbeSchema(schema workorcheventpayload.Payload) error {
	if schemaType, declared := schema.String("type"); !declared || strings.TrimSpace(schemaType) != "object" {
		return fmt.Errorf("MCP semantic behavior probe requires a valid object input schema")
	}
	return validateMCPBehaviorProbeSchemaNode(schema, "$")
}

func validateMCPBehaviorProbeSchemaNode(schema workorcheventpayload.Payload, path string) error {
	if schemaType, declared := schema.String("type"); declared && strings.TrimSpace(schemaType) != "object" {
		return fmt.Errorf("MCP semantic behavior probe schema %s has unsupported type %q", path, schemaType)
	}
	if _, declared := schema.Field("required"); declared {
		required, ok := schema.StringSlice("required")
		if !ok {
			return fmt.Errorf("MCP semantic behavior probe schema %s.required must be a string array", path)
		}
		seen := make(map[string]struct{}, len(required))
		for _, name := range required {
			name = strings.TrimSpace(name)
			if name == "" {
				return fmt.Errorf("MCP semantic behavior probe schema %s.required contains an empty name", path)
			}
			if _, duplicate := seen[name]; duplicate {
				return fmt.Errorf("MCP semantic behavior probe schema %s.required contains duplicate %q", path, name)
			}
			seen[name] = struct{}{}
		}
		if len(required) != 0 {
			return fmt.Errorf("MCP semantic behavior probe cannot invent required tool parameters at %s.required", path)
		}
	}
	if properties, declared := schema.Field("properties"); declared && properties.Kind != workorcheventpayload.ValueObject {
		return fmt.Errorf("MCP semantic behavior probe schema %s.properties must be an object", path)
	}
	if additional, declared := schema.Field("additionalProperties"); declared && additional.Kind != workorcheventpayload.ValueBool && additional.Kind != workorcheventpayload.ValueObject {
		return fmt.Errorf("MCP semantic behavior probe schema %s.additionalProperties must be a boolean or object", path)
	}
	for _, fieldName := range []string{"anyOf", "allOf", "oneOf"} {
		value, declared := schema.Field(fieldName)
		if !declared {
			continue
		}
		if value.Kind != workorcheventpayload.ValueArray {
			return fmt.Errorf("MCP semantic behavior probe schema %s.%s must be an array", path, fieldName)
		}
		if len(value.Array) == 0 {
			return fmt.Errorf("MCP semantic behavior probe schema %s.%s must not be empty", path, fieldName)
		}
		for index, item := range value.Array {
			if item.Kind != workorcheventpayload.ValueObject {
				return fmt.Errorf("MCP semantic behavior probe schema %s.%s[%d] must be an object", path, fieldName, index)
			}
			if err := validateMCPBehaviorProbeSchemaNode(workorcheventpayload.Payload{Fields: item.Object}, fmt.Sprintf("%s.%s[%d]", path, fieldName, index)); err != nil {
				return err
			}
		}
	}
	return nil
}

// probeSemanticValidationRecipe executes one bounded, owner-local MCP behavior
// probe. It is deliberately not an EventHub operation and does not persist a
// receipt: this is a factual feasibility observation before SM-2 binding seal.
// Its caller has already fenced effect/idempotency and required input schema.
func (m *Biz) probeSemanticValidationRecipe(ctx context.Context, command mcpevents.ProbeRuntimeSemanticProofFeasibilityCommand, serverID, tool string) error {
	if m == nil || m.runtimeAdapter == nil || strings.TrimSpace(serverID) == "" || strings.TrimSpace(tool) == "" {
		return common.ErrInvalidArgument
	}
	request := RuntimeOperationRequest{
		WorkspaceID:                   command.WorkspaceID,
		RunID:                         command.RunID,
		StepID:                        common.StepID("semantic-probe/" + command.FeasibilityBatchID + "/" + string(command.Proposal.CoverageWorkItemID)),
		Kind:                          runtimecontract.StepKindMCPToolCall,
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		TargetRefs:                    runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal),
		Data: mcpevents.OperationData{
			ServerID: serverID,
			Tool:     tool,
		},
	}
	if err := m.runtimeAdapter.ProbeOperation(ctx, request); err != nil {
		return fmt.Errorf("MCP behavior probe preflight failed: %w", err)
	}
	operationResult, err := m.runtimeAdapter.ExecuteOperation(ctx, request)
	evidence := operationResult.Evidence
	status := runtimecontract.RuntimeOperationExecutionStatusRejected
	if err == nil && operationResult.Status == "completed" {
		status = runtimecontract.RuntimeOperationExecutionStatusAccepted
	}
	attachMCPToolValidationObservation(&evidence, status, runtimecontract.StepKindMCPToolCall, runtimecontract.EvidenceKindValidation, serverID, tool)
	if err != nil {
		return fmt.Errorf("MCP behavior probe failed: %w", err)
	}
	if operationResult.Status != "completed" || evidence.Validation == nil {
		return fmt.Errorf("MCP behavior probe did not produce an accepted validation observation")
	}
	// Check identity is owner-derived by attachMCPToolValidationObservation
	// (the executable recipe itself).  The provider proposal is intentionally
	// not consulted here: RequiredCheckRefs is not provider authority and must
	// never be echoed into a feasibility decision.
	return nil
}

func parseMCPValidationRecipe(recipe string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(recipe), "/", 4)
	if len(parts) != 4 || parts[0] != "mcp" || parts[1] != "tool_call" || parts[2] == "" || parts[3] == "" {
		return "", "", fmt.Errorf("MCP validation recipe %q is not an executable tool recipe", recipe)
	}
	return parts[2], parts[3], nil
}

func findEffectiveMCPCapability(items []*mcpmodel.Capability, serverID, tool string) (*mcpmodel.Capability, bool) {
	for _, item := range items {
		if item != nil && item.Enabled && item.Kind == mcpmodel.CapabilityKindTool && string(item.ServerID) == serverID && item.Name == tool {
			return item, true
		}
	}
	return nil, false
}

func setRuntimeSemanticProofFeasibilityResult(result magicevent.Result, command mcpevents.ProbeRuntimeSemanticProofFeasibilityCommand, status runtimecontract.RuntimeSemanticFeasibilityStatus, revision string, targetRefs []string, diagnostic *runtimecontract.RuntimeOriginalDiagnostic) {
	response, err := mcpevents.NewRuntimeSemanticProofFeasibilityResult(command, status, revision, targetRefs, diagnostic)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func mcpAdmissibilityObservation(prerequisiteID string, inventory runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: ModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: ModuleID, SubjectRef: "mcp_server_catalog", Revision: inventory.ObservedRevision}}}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func mcpAdmissibilityStale(observation *runtimecontract.RuntimePrerequisiteObservation, inventory runtimecontract.RuntimeOperationAdmissibility, observedRevision string) *runtimecontract.RuntimePrerequisiteStale {
	if observation == nil {
		return nil
	}
	value := observation.Normalized()
	for _, dependency := range value.Dependencies {
		if dependency.Domain == runtimecontract.RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == ModuleID && dependency.SubjectRef == "mcp_server_catalog" && dependency.Revision == inventory.ObservedRevision {
			return &runtimecontract.RuntimePrerequisiteStale{PrerequisiteID: value.PrerequisiteID, ObservationID: value.ObservationID, Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef, ExpectedRevision: dependency.Revision, ObservedRevision: observedRevision, Reason: "runtime mcp server catalog changed"}
		}
	}
	return nil
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
		registryevents.TopicMCPCommandListServers,
		ModuleID,
		registryevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		registryevents.ListServersCommand{},
	)
	result := g.hub.Send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return nil, resultErr
	}
	response, ok := data.(registryevents.MCPCommandResult)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	servers := make([]*mcpmodel.Server, 0, len(response.Servers))
	for _, item := range response.Servers {
		server, err := item.Model()
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func (g mcpEventServerGetter) GetServer(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if g.hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(
		registryevents.TopicMCPCommandGetServer,
		ModuleID,
		registryevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		registryevents.GetServerCommand{ServerID: id},
	)
	result := g.hub.Send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return nil, resultErr
	}
	response, ok := data.(registryevents.MCPCommandResult)
	if !ok || response.Server == nil {
		return nil, common.ErrNotFound
	}
	return response.Server.Model()
}

func (g mcpEventServerGetter) ListCapabilities(ctx context.Context, query registryevents.CapabilityQuery) ([]*mcpmodel.Capability, error) {
	if g.hub == nil {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(registryevents.TopicMCPCommandListCapabilities, ModuleID, registryevents.CommandDestination, magicevent.NewHeader(), ctx, registryevents.ListCapabilitiesCommand{Query: query})
	result := g.hub.Send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return nil, resultErr
	}
	response, ok := data.(registryevents.MCPCommandResult)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return mcpmodel.CloneCapabilities(response.Capabilities), nil
}
