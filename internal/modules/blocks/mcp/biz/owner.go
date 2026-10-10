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
	lifecycle        basebiz.Lifecycle
	runtimeAdapter   *RuntimeAdapter
	started          bool
	subscribedTopics []string
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
	leaveStart, err := m.lifecycle.Enter()
	if err != nil {
		return err
	}
	defer leaveStart()
	if m.started {
		return nil
	}
	subscriptions := []struct {
		topic   string
		handler magicevent.ObserverFunc
	}{
		{mcpevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities},
		{mcpevents.TopicAcquireRuntimeOperationAdmissibility, m.handleRuntimeOperationAdmissibility},
		{mcpevents.TopicProbeRuntimeOperationFeasibility, m.handleRuntimeOperationFeasibility},
		{mcpevents.TopicProbeRuntimeSemanticProofFeasibility, m.handleRuntimeSemanticProofFeasibility},
		{mcpevents.TopicQueryRuntimeCWIReconciliation, m.handleRuntimeCWIReconciliationQuery},
		{mcpevents.TopicExecuteOperation, m.handleMCPOperation},
	}
	// A failed Start keeps every completed subscription reachable. Retrying
	// resumes at the first unfinished topic rather than registering duplicates.
	for _, subscription := range subscriptions[len(m.subscribedTopics):] {
		if err := m.SubscribeFuncChecked(subscription.topic, m.lifecycleHandler(subscription.handler, subscription.topic == mcpevents.TopicExecuteOperation)); err != nil {
			return err
		}
		m.subscribedTopics = append(m.subscribedTopics, subscription.topic)
	}
	m.started = true
	return nil
}

func (m *Biz) Teardown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// 最终释放仍由应用共享 Hub/任务屏障后的调用方授权；本地等待只是防御。
	if err := m.BeginShutdown(ctx); err != nil {
		return err
	}
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	// Lifecycle calls are serialized by their caller. Do not hold an owner
	// lock across Hub control actions. Unsubscribe completion is a resource
	// receipt, not proof of handler quiescence; the caller must drain first.
	for len(m.subscribedTopics) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(m.subscribedTopics) - 1
		if err := m.UnsubscribeFuncChecked(m.subscribedTopics[last]); err != nil {
			return err
		}
		m.subscribedTopics = m.subscribedTopics[:last]
		m.started = false
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.started = false
	if m.runtimeAdapter != nil {
		if err := m.runtimeAdapter.CloseChecked(ctx); err != nil {
			return err
		}
		m.runtimeAdapter = nil
	}
	return ctx.Err()
}

// handleRuntimeCWIReconciliationQuery reports MCP readiness separately from
// behavior proof. Catalog/endpoint/tool-surface readiness is useful diagnostic
// context, but it is never a satisfied CWI. A behavior proof requires a
// binding-declared executable probe (including its exact input contract),
// which this reconciliation command intentionally does not invent.
func (m *Biz) handleRuntimeCWIReconciliationQuery(event magicevent.Event, result magicevent.Result) {
	command, ok := mcpRuntimeIngressCommand[mcpevents.QueryRuntimeCWIReconciliationCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil {
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
	command, ok := mcpRuntimeIngressCommand[mcpevents.ExecuteOperationCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	if err := command.Execution.Validate(); err != nil {
		setCapabilityError(result, err)
		return
	}
	operation, sealedAuthorization, err := command.SealedOperation.ValidateForExecution(command.RuntimeOperationExecutionContext)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	data, err := mcpevents.OperationDataFromRuntimeOperation(operation)
	if err != nil {
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
	// 未命中的回执查询不能借 drain 准入产生新 effect 或新 receipt。
	leaveFresh, admissionErr := m.lifecycle.Enter()
	if admissionErr != nil {
		setCapabilityError(result, admissionErr)
		return
	}
	defer leaveFresh()

	request := RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID(command.DispatchID + "/" + command.OperationID),
		Kind: operation.Kind, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		WorkspaceInternalDirectory: command.WorkspaceInternalDirectory, TargetRefs: command.TargetRefs,
		TargetRole: command.TargetRole,
		Data:       data, EvidencePolicies: command.EvidencePolicies,
	}
	authorized, err := m.runtimeAdapter.authorizeRuntimeOperation(event.Context(), request, sealedAuthorization)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if err := m.runtimeAdapter.probeAuthorizedOperation(request, authorized); err != nil {
		setCapabilityError(result, err)
		return
	}
	governance, err := mcpApprovalRequirement(operation, command.SealedOperation.PayloadHash, sealedAuthorization, authorized)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if governance != nil && command.Execution.GovernanceGrant == nil {
		receipt := m.newRuntimeOperationReceipt(command.Execution, command.OperationID, runtimecontract.RuntimeOperationExecutionStatusRejected, command.TargetRefs,
			runtimecontract.RuntimeOperationEvidence{Summary: "MCP capability requires approval before execution"}, nil, governance,
			orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectGovernanceOnly})
		committed, commitErr := m.commitRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.Transaction.AuthoritySnapshotID, command.DispatchID, command.Execution, receipt)
		if commitErr != nil {
			setCapabilityError(result, commitErr)
			return
		}
		result.Set(mcpevents.ExecuteOperationResult{Receipt: committed}, nil)
		return
	}
	if governance != nil {
		if err := validateMCPApprovalGrant(command.Execution.GovernanceGrant, *governance); err != nil {
			setCapabilityError(result, err)
			return
		}
	}
	if err := prepareMCPEvidenceStore(command.WorkspaceInternalDirectory); err != nil {
		setCapabilityError(result, err)
		return
	}
	operationResult, executeErr := m.runtimeAdapter.executeAuthorizedOperation(event.Context(), request, authorized)
	status := runtimecontract.RuntimeOperationExecutionStatusAccepted
	evidence, evidenceErr := buildMCPOperationEvidence(command.WorkspaceInternalDirectory, operation.Kind, sealedAuthorization, operationResult)
	if evidenceErr != nil && executeErr == nil {
		executeErr = evidenceErr
		operationResult.FailureCode = "mcp_evidence_persist_failed"
	}
	if evidenceErr != nil {
		evidence = runtimecontract.RuntimeOperationEvidence{Summary: "MCP operation did not produce persistable owner evidence"}
	}
	var failure *runtimecontract.FailureFact
	if executeErr != nil {
		status = runtimecontract.RuntimeOperationExecutionStatusRejected
		targetObservation := mcpFailureTargetObservation(operationResult, evidence)
		var failureTargets []string
		if targetObservation != nil {
			failureTargets = append([]string(nil), targetObservation.TargetRefs...)
		}
		failure = orchestratorevents.RuntimeOperationFailureWithTargetObservation(command.Transaction, operation.Kind, operationResult.FailureCode, safeMCPFailureMessage(operationResult.FailureCode), failureTargets, targetObservation)
	}
	observation := orchestratorevents.RuntimeOperationReceiptObservation{ObservedEffectScope: runtimecontract.RuntimeEffectUnknown}
	if status == runtimecontract.RuntimeOperationExecutionStatusAccepted {
		scope, changed, beforeHash, afterHash, resourceChanges, effectErr := mcpReceiptEffectObservation(operationResult, sealedAuthorization)
		if effectErr != nil {
			status = runtimecontract.RuntimeOperationExecutionStatusRejected
			operationResult.FailureCode = "mcp_effect_observation_invalid"
			failure = orchestratorevents.RuntimeOperationFailure(command.Transaction, operation.Kind, operationResult.FailureCode, safeMCPFailureMessage(operationResult.FailureCode), command.TargetRefs)
		} else {
			observation.ObservedEffectScope = scope
			observation.Changed = changed
			observation.NoOp = runtimecontract.RuntimeEffectIsDurable(sealedAuthorization.Policy.EffectScope) && !changed
			observation.BeforeHash = beforeHash
			observation.AfterHash = afterHash
			observation.ResourceChanges = resourceChanges
		}
	}
	attachMCPToolValidationObservation(&evidence, status, operation.Kind, command.EvidenceKind, data.ServerID, data.Tool, sealedAuthorization.ValidationSurface)
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

func mcpFailureTargetObservation(result RuntimeOperationResult, evidence runtimecontract.RuntimeOperationEvidence) *runtimecontract.RuntimeFailureTargetObservation {
	if evidence.ContentAddress == nil || evidence.ContentAddress.Validate() != nil ||
		evidence.ContentAddress.Digest != evidence.ContentHash || !containsExactMCPValue(evidence.EvidenceRefs, evidence.ContentAddress.Ref) {
		return nil
	}
	targets := mcpFailureTargets(result)
	observation := runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: ModuleID, SubjectRef: result.ServerID, TargetRole: result.Diagnostics.TargetRole,
		ObservedRevision: result.OwnerRevision, TargetRefs: targets,
		CatalogDependency: &runtimecontract.RuntimeFreshnessDependency{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: ModuleID,
			SubjectRef: "mcp_server_catalog", Revision: result.OwnerRevision,
		},
		DiagnosticClass: runtimecontract.RuntimeFailureTargetDiagnosticReceiptEvidence,
		EvidenceRefs:    append([]string(nil), evidence.EvidenceRefs...),
		ContentHash:     evidence.ContentHash,
	}.Normalized()
	if err := observation.Validate(); err != nil {
		return nil
	}
	return &observation
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := mcpRuntimeIngressCommand[mcpevents.AcquireRuntimeCapabilitiesCommand](event, result)
	if !ok {
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
	command, ok := mcpRuntimeIngressCommand[mcpevents.AcquireRuntimeOperationAdmissibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := mcpevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	catalogSnapshot, err := m.runtimeAdapter.runtimeOperationAdmissibilitySnapshot(event.Context())
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	inventory, err := buildRuntimeMCPAdmissibility(command.PrerequisiteID, command.RequiredKinds, catalogSnapshot)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	observation := mcpAdmissibilityObservation(command.PrerequisiteID, inventory)
	result.Set(mcpevents.AcquireRuntimeOperationAdmissibilityResult{Acquisition: runtimecontract.RuntimeOperationAdmissibilityAcquisition{WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID, Admissibility: inventory, Observation: observation}}, nil)
}

func buildRuntimeMCPAdmissibility(prerequisiteID string, requiredKinds []string, catalogSnapshot runtimeMCPAdmissibilitySnapshot) (runtimecontract.RuntimeOperationAdmissibility, error) {
	inventory, err := runtimecontract.NewRuntimeOperationAdmissibility(prerequisiteID, ModuleID, requiredKinds, nil, catalogSnapshot.ServerIDs, []string{"configured-mcp-server", "owner-execution-policy"}, catalogSnapshot.Revision)
	if err != nil {
		return runtimecontract.RuntimeOperationAdmissibility{}, err
	}
	inventory.ValidationRecipeRefs = append([]string(nil), catalogSnapshot.ValidationRecipes...)
	inventory.ValidationSurfaces = runtimecontract.NormalizeRuntimeValidationSurfaces(catalogSnapshot.ValidationSurfaces)
	inventory.MCPCapabilities = runtimeMCPCapabilityContexts(catalogSnapshot.Capabilities)
	inventory.OperationPolicies = runtimeMCPOperationPolicies(inventory.MCPCapabilities, inventory.OperationKinds)
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		return runtimecontract.RuntimeOperationAdmissibility{}, err
	}
	return inventory, nil
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
			URI: item.URI, Enabled: item.Enabled, ApprovalPolicy: item.ApprovalPolicy, Status: mcpCapabilityAvailability(item.Enabled),
		}))
	}
	return ret
}

func mcpCapabilityAvailability(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func runtimeMCPOperationPolicies(capabilities []runtimecontract.RuntimeMCPCapabilityContext, operationKinds []string) []runtimecontract.RuntimeOperationPolicyFact {
	allowedKinds := make(map[string]struct{}, len(operationKinds))
	for _, kind := range operationKinds {
		allowedKinds[runtimecontract.CanonicalRuntimeBlockKind(kind)] = struct{}{}
	}
	ret := make([]runtimecontract.RuntimeOperationPolicyFact, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = runtimecontract.SealMCPCapabilityEffect(capability)
		if !capability.Enabled || capability.ID == "" || capability.ServerID == "" || strings.TrimSpace(capability.Name) == "" {
			continue
		}
		if _, allowed := allowedKinds[capability.OperationKind]; !allowed {
			continue
		}
		serverID := string(capability.ServerID)
		appendPolicy := func(suffix, field, value string) {
			value = strings.TrimSpace(value)
			if value == "" {
				return
			}
			id := "operation-policy/" + string(capability.ID)
			if suffix != "" {
				id += "/" + suffix
			}
			ret = append(ret, runtimecontract.RuntimeOperationPolicyFact{
				ID: id, OwnerModuleID: ModuleID, OperationKind: capability.OperationKind, SubjectRef: string(capability.ID),
				ParameterMatches: []runtimecontract.RuntimeOperationParameterMatch{{Name: "serverId", String: &serverID}, {Name: field, String: &value}},
				EffectScope:      runtimecontract.RuntimeEffectScope(capability.EffectScope), IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyClass(capability.IdempotencyClass),
			})
		}
		switch capability.OperationKind {
		case runtimecontract.StepKindMCPToolCall:
			appendPolicy("", "tool", capability.Name)
		case runtimecontract.StepKindMCPResourceRead:
			resource := capability.URI
			if strings.TrimSpace(resource) == "" {
				resource = capability.Name
			}
			appendPolicy("uri", "uri", resource)
			appendPolicy("resource-uri", "resourceUri", resource)
		case runtimecontract.StepKindMCPPromptGet:
			appendPolicy("prompt", "prompt", capability.Name)
			appendPolicy("name", "name", capability.Name)
		}
	}
	return runtimecontract.NormalizeRuntimeOperationPolicyFacts(ret)
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
	command, ok := mcpRuntimeIngressCommand[mcpevents.ProbeRuntimeOperationFeasibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
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
	if !containsExactMCPValue(serverIDs, data.ServerID) {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "mcp_server_not_admissible", "runtime mcp operation server is absent from the owner admissibility inventory")
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	sealed, err := mcpevents.SealRuntimeOperation(operation, command.Admissibility)
	if err != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "mcp_capability_not_admissible", err.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	request := RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID("probe/" + operation.ID), Kind: operation.Kind,
		WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory, TargetRefs: operation.TargetRefs, Data: data,
	}
	authorized, err := m.runtimeAdapter.authorizeRuntimeOperation(event.Context(), request, sealed.Authorization)
	if err == nil {
		err = m.runtimeAdapter.probeAuthorizedOperation(request, authorized)
	}
	if err != nil {
		failure := runtimecontract.RuntimeOperationInfeasibleFailure(operation, "mcp_prerequisite_unavailable", err.Error())
		setRuntimeOperationFeasibilityResult(result, command, failure, nil)
		return
	}
	setRuntimeOperationFeasibilityResult(result, command, nil, nil)
}

func (m *Biz) handleRuntimeSemanticProofFeasibility(event magicevent.Event, result magicevent.Result) {
	command, ok := mcpRuntimeIngressCommand[mcpevents.ProbeRuntimeSemanticProofFeasibilityCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	if err := mcpevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil {
		setCapabilityError(result, err)
		return
	}
	catalogSnapshot, err := m.runtimeAdapter.runtimeOperationAdmissibilitySnapshot(event.Context())
	if err != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, command.OwnerCatalog.ObservedRevision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, mcpSemanticCatalogUnavailableCode(err), err.Error(), command.OwnerCatalog.ObservedRevision, "", nil))
		return
	}
	serverIDs, recipeRefs, revision := catalogSnapshot.ServerIDs, catalogSnapshot.ValidationRecipes, catalogSnapshot.Revision
	if revision != command.OwnerCatalog.ObservedRevision {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityStale, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_catalog_stale", "runtime MCP semantic catalog is stale", command.OwnerCatalog.ObservedRevision, revision, nil))
		return
	}
	serverID, tool, parseErr := parseMCPValidationRecipe(command.Proposal.ValidationRecipeID)
	if parseErr != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_recipe_invalid", parseErr.Error(), "", revision, nil))
		return
	}
	capability, found := findEffectiveMCPCapability(catalogSnapshot.Capabilities, serverID, tool)
	if !found {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_recipe_not_admissible", fmt.Sprintf("recipe %q is not present in the effective owner capability catalog", command.Proposal.ValidationRecipeID), "", revision, []string{command.Proposal.ValidationRecipeID}))
		return
	}
	surface, surfaceFound := runtimecontract.RuntimeValidationSurfaceForID(catalogSnapshot.ValidationSurfaces, command.Proposal.ValidationRecipeID)
	if surfaceFound && surface.Readiness == runtimecontract.RuntimeValidationSurfacePrerequisiteRequired {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityPrerequisiteRequired, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_validation_input_binding_required", "MCP semantic validation recipe has no ready owner-sealed input binding", "", revision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal)))
		return
	}
	if !surfaceFound || surface.Readiness != runtimecontract.RuntimeValidationSurfaceReady || surface.ProofApplicability != runtimecontract.RuntimeValidationProofDirect || surface.InputBindingRef == "" ||
		!containsExactMCPValue(recipeRefs, command.Proposal.ValidationRecipeID) {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_recipe_not_admissible", "runtime semantic MCP validation recipe is absent from the ready direct-proof owner catalog", "", revision, nil))
		return
	}
	// SM-2 feasibility is a catalog/schema/input-binding fact only. Effect and
	// replay declarations constrain whether the surface may be sealed, but never
	// authorize a transport call before an SM-3 dispatch exists.
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
		if !containsExactMCPValue(serverIDs, selector) {
			setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
				runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_selector_not_admissible", "runtime semantic MCP selector is absent from the owner catalog", "", revision, []string{selector}))
			return
		}
	}
	if probeErr := m.runtimeAdapter.ProbeOperation(event.Context(), RuntimeOperationRequest{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		StepID: common.StepID("semantic-preflight/" + command.FeasibilityBatchID + "/" + string(command.Proposal.CoverageWorkItemID)),
		Kind:   runtimecontract.StepKindMCPToolCall, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory,
		TargetRefs: runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal),
		Data:       mcpevents.OperationData{ServerID: serverID, Tool: tool},
	}); probeErr != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "mcp_semantic_proof_unavailable", "MCP semantic transport preflight is unavailable", "", revision, runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal)))
		return
	}
	// Re-check the owner-local catalog revision immediately before publishing the
	// fact so a registry update cannot produce a feasible observation from a
	// stale snapshot. No endpoint or process is touched in this path.
	verifiedSnapshot, verifyErr := m.runtimeAdapter.runtimeOperationAdmissibilitySnapshot(event.Context())
	if verifyErr != nil {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityInfeasible, revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, mcpSemanticCatalogUnavailableCode(verifyErr), verifyErr.Error(), revision, "", nil))
		return
	}
	if verifiedSnapshot.Revision != revision {
		setRuntimeSemanticProofFeasibilityResult(result, command, runtimecontract.RuntimeSemanticFeasibilityStale, verifiedSnapshot.Revision, nil,
			runtimecontract.RuntimeSemanticFeasibilityDiagnostic(ModuleID, "semantic_catalog_stale", "runtime MCP semantic catalog changed during owner proof acquisition", revision, verifiedSnapshot.Revision, nil))
		return
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
// behavior probe has no provider-authored input values, so it may only admit
// a capability whose sealed schema is a well-formed object and whose complete
// schema (including simple combinators) has no required input.  This is an
// owner-local safety gate: malformed schemas become typed unavailable facts,
// while input-requiring schemas remain callable but become typed prerequisites;
// neither reaches the MCP transport without an owner-sealed input binding.
func validateMCPBehaviorProbeSchema(schema workorcheventpayload.Payload) error {
	requiresInput, err := inspectMCPBehaviorProbeSchema(schema)
	if err != nil {
		return err
	}
	if requiresInput {
		return fmt.Errorf("MCP semantic behavior probe requires an owner-sealed input binding")
	}
	return nil
}

// inspectMCPBehaviorProbeSchema validates the bounded schema vocabulary and
// reports whether invocation needs values that are not present in the empty
// owner input binding. Required-input surfaces remain callable, but catalog
// publication marks them prerequisite_required and keeps them out of the
// provider-visible validation recipe set.
func inspectMCPBehaviorProbeSchema(schema workorcheventpayload.Payload) (bool, error) {
	if schemaType, declared := schema.String("type"); !declared || strings.TrimSpace(schemaType) != "object" {
		return false, fmt.Errorf("MCP semantic behavior probe requires a valid object input schema")
	}
	return inspectMCPBehaviorProbeSchemaNode(schema, "$")
}

func inspectMCPBehaviorProbeSchemaNode(schema workorcheventpayload.Payload, path string) (bool, error) {
	requiresInput := false
	if schemaType, declared := schema.String("type"); declared && strings.TrimSpace(schemaType) != "object" {
		return false, fmt.Errorf("MCP semantic behavior probe schema %s has unsupported type %q", path, schemaType)
	}
	if _, declared := schema.Field("required"); declared {
		required, ok := schema.StringSlice("required")
		if !ok {
			return false, fmt.Errorf("MCP semantic behavior probe schema %s.required must be a string array", path)
		}
		seen := make(map[string]struct{}, len(required))
		for _, name := range required {
			name = strings.TrimSpace(name)
			if name == "" {
				return false, fmt.Errorf("MCP semantic behavior probe schema %s.required contains an empty name", path)
			}
			if _, duplicate := seen[name]; duplicate {
				return false, fmt.Errorf("MCP semantic behavior probe schema %s.required contains duplicate %q", path, name)
			}
			seen[name] = struct{}{}
		}
		if len(required) != 0 {
			requiresInput = true
		}
	}
	if properties, declared := schema.Field("properties"); declared && properties.Kind != workorcheventpayload.ValueObject {
		return false, fmt.Errorf("MCP semantic behavior probe schema %s.properties must be an object", path)
	}
	if additional, declared := schema.Field("additionalProperties"); declared && additional.Kind != workorcheventpayload.ValueBool && additional.Kind != workorcheventpayload.ValueObject {
		return false, fmt.Errorf("MCP semantic behavior probe schema %s.additionalProperties must be a boolean or object", path)
	}
	for _, fieldName := range []string{"anyOf", "allOf", "oneOf"} {
		value, declared := schema.Field(fieldName)
		if !declared {
			continue
		}
		if value.Kind != workorcheventpayload.ValueArray {
			return false, fmt.Errorf("MCP semantic behavior probe schema %s.%s must be an array", path, fieldName)
		}
		if len(value.Array) == 0 {
			return false, fmt.Errorf("MCP semantic behavior probe schema %s.%s must not be empty", path, fieldName)
		}
		for index, item := range value.Array {
			if item.Kind != workorcheventpayload.ValueObject {
				return false, fmt.Errorf("MCP semantic behavior probe schema %s.%s[%d] must be an object", path, fieldName, index)
			}
			nestedRequiresInput, err := inspectMCPBehaviorProbeSchemaNode(workorcheventpayload.Payload{Fields: item.Object}, fmt.Sprintf("%s.%s[%d]", path, fieldName, index))
			if err != nil {
				return false, err
			}
			requiresInput = requiresInput || nestedRequiresInput
		}
	}
	return requiresInput, nil
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

func containsExactMCPValue(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// mcpRuntimeIngressCommand is the owner-local strong-call gate for every
// Runtime topic subscribed by MCP. Post is rejected before DTO inspection;
// only the exact Orchestrator source may reach registry queries, tool/resource
// calls, receipt lookup, or evidence paths.
func mcpRuntimeIngressCommand[T any](event magicevent.Event, result magicevent.Result) (T, bool) {
	var zero T
	if result == nil {
		return zero, false
	}
	if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator {
		setMCPRuntimeIngressInvalid(result, "runtime MCP ingress requires exact Orchestrator source")
		return zero, false
	}
	command, ok := event.Data().(T)
	if !ok {
		setMCPRuntimeIngressInvalid(result, "runtime MCP ingress payload has an invalid concrete type")
		return zero, false
	}
	return command, true
}

func setMCPRuntimeIngressInvalid(result magicevent.Result, message string) {
	if result == nil {
		return
	}
	err := fmt.Errorf("%w: %s", common.ErrInvalidArgument, message)
	result.Set(nil, &cd.Error{Code: cd.IllegalParam, Message: err.Error(), Cause: err})
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
}

type mcpEventServerGetter struct {
	hub magicevent.Hub
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
	response, ok := data.(registryevents.GetServerResult)
	if !ok || response.Server == nil {
		return nil, common.ErrNotFound
	}
	return response.Server.Model()
}

func (g mcpEventServerGetter) RegistrySnapshot(ctx context.Context) (*mcpmodel.RegistrySnapshot, string, error) {
	if g.hub == nil {
		return nil, "", common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(registryevents.TopicMCPCommandReadRegistrySnapshot, ModuleID, registryevents.CommandDestination, magicevent.NewHeader(), ctx, registryevents.ReadRegistrySnapshotCommand{})
	result := g.hub.Send(ev)
	if result == nil {
		return nil, "", common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, "", err
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return nil, "", resultErr
	}
	response, ok := data.(registryevents.RegistrySnapshotResult)
	if !ok {
		return nil, "", common.ErrInvalidArgument
	}
	servers := make([]*mcpmodel.Server, 0, len(response.Servers))
	for _, item := range response.Servers {
		server, err := item.Model()
		if err != nil {
			return nil, "", err
		}
		servers = append(servers, server)
	}
	return &mcpmodel.RegistrySnapshot{Servers: servers, Capabilities: mcpmodel.CloneCapabilities(response.Capabilities)}, response.RegistryRevision, nil
}

// BeginShutdown 关停新 root，不取消已接纳 handler；其 nested receipt/query
// 仍保留真实调用 context，只有实际返回时才释放在途记账。
func (m *Biz) BeginShutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.lifecycle.BeginShutdown()
	if m.runtimeAdapter != nil {
		m.runtimeAdapter.BeginShutdown()
	}
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func (m *Biz) Quiesce(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if err := m.lifecycle.Quiesce(ctx); err != nil {
		return err
	}
	if m.runtimeAdapter != nil {
		return m.runtimeAdapter.Quiesce(ctx)
	}
	return nil
}

// Execute 的 drain 入口仅执行 sealed command 校验与 exact persisted receipt
// 查询；receipt 未命中必须另取 fresh 票，不能回退扩大停机期间的执行权限。
func (m *Biz) lifecycleHandler(handler magicevent.ObserverFunc, receiptDrain bool) magicevent.ObserverFunc {
	return func(ev magicevent.Event, result magicevent.Result) {
		if result == nil || ev == nil {
			return
		}
		if ev.Source() != runtimecontract.RuntimeModuleOrchestrator {
			setMCPRuntimeIngressInvalid(result, "runtime command source must be kernel/orchestrator")
			return
		}
		var leave func()
		var err error
		if receiptDrain {
			leave, err = m.lifecycle.EnterDrain()
		} else {
			leave, err = m.lifecycle.Enter()
		}
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		defer leave()
		handler(ev, result)
	}
}
