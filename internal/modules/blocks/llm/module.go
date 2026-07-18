package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmruntimev2 "github.com/muidea/workorch/internal/modules/blocks/llm/biz/runtimev2"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	llmproviderconfig "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/providerconfig"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindWorkspaceExecution = modulecommon.StepKindWorkspaceExecution

type Module struct {
	rawEventHub          magicevent.Hub
	observer             magicevent.SimpleObserver
	prerequisiteObserver magicevent.SimpleObserver
	catalogService       *llmbiz.CatalogService
	candidateProvider    llmruntimev2.CandidateProvider
	topics               []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

type Dependencies struct {
	EventHub          magicevent.Hub
	CatalogService    *llmbiz.CatalogService
	CandidateProvider llmruntimev2.CandidateProvider
}

func (m *Module) Configure(deps ...Dependencies) error {
	if len(deps) > 0 {
		m.rawEventHub = deps[0].EventHub
		m.catalogService = deps[0].CatalogService
		m.candidateProvider = deps[0].CandidateProvider
	}
	if m.catalogService == nil {
		m.catalogService = llmbiz.NewCatalogService()
	}
	if m.candidateProvider == nil {
		m.candidateProvider = m
	}
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.catalogService == nil || m.rawEventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.observer != nil {
		return nil
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.rawEventHub)
	m.topics = []string{
		llmevents.TopicLLMCommandCompleteProvider,
		llmevents.TopicLLMCommandGenerateEmbedding,
		llmevents.TopicLLMCommandGetProviderModelCatalog,
		llmevents.TopicLLMCommandRefreshProviderModelCatalog,
		llmevents.TopicLLMCommandResolveProviderModelTarget,
	}
	for _, topic := range m.topics {
		m.observer.Subscribe(topic, m.handleCommand)
	}
	m.observer.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.topics = append(m.topics, orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	if m.candidateProvider != nil {
		m.observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeCandidate, m.handleRuntimeCandidate)
		m.topics = append(m.topics, orchestratorevents.TopicOrchestratorRequestRuntimeCandidate)
	}
	m.prerequisiteObserver = magicevent.NewSimpleObserverWithMatchID(ModuleID+"/prerequisite", orchestratorevents.PrerequisiteCommandDestination, m.rawEventHub)
	m.prerequisiteObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireProviderCeiling, m.handleProviderCeilingPrerequisite)
	return nil
}

func (m *Module) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindWorkspaceExecution)
	if err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.observer != nil {
		for idx := len(m.topics) - 1; idx >= 0; idx-- {
			m.observer.Unsubscribe(m.topics[idx])
		}
	}
	m.topics = nil
	m.observer = nil
	if m.prerequisiteObserver != nil {
		m.prerequisiteObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireProviderCeiling)
	}
	m.prerequisiteObserver = nil
	m.rawEventHub = nil
	m.catalogService = nil
	m.candidateProvider = nil
	return nil
}

func (m *Module) handleRuntimeCandidate(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.RequestRuntimeCandidateCommand)
	if !ok || command.WorkspaceID == "" || command.RunID == "" || m.candidateProvider == nil ||
		command.Decision.AuthoritySnapshot.WorkspaceID != command.WorkspaceID || command.Decision.AuthoritySnapshot.RunID != command.RunID {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	response, err := llmruntimev2.RequestCandidate(event.Context(), m.candidateProvider, llmmodels.RuntimeCandidateRequest{
		Decision:          command.Decision,
		Transaction:       command.Transaction,
		CandidateAttempt:  command.CandidateAttempt,
		PreviousFailure:   command.PreviousFailure,
		StablePrefixHash:  command.StablePrefixHash,
		DefinitionContext: command.DefinitionContext,
		CandidateContext:  command.CandidateContext,
	})
	// Always attempt to persist sealed request audit when generation produced one.
	// Audit may also ride on CandidateRequestError for failed provider calls.
	audit := response.Audit
	usage := response.Usage
	requestID := response.RequestID
	status := response.Status
	if err != nil {
		if audit == nil {
			if errAudit, errRequestID, errStatus := llmruntimev2.CandidateAuditFromError(err); errAudit != nil {
				audit = errAudit
				requestID = errRequestID
				status = errStatus
			}
		}
		failure := llmruntimev2.CandidateFailureFromError(err)
		if status == "" {
			status = llmmodels.RuntimeCandidateStatusFailed
		}
		m.publishLLMUsageRecorded(event.Context(), audit, usage, status, string(failure.Class))
		result.Set(orchestratorevents.RequestRuntimeCandidateResult{
			Transaction: command.Transaction,
			Failure:     &failure,
			RequestID:   requestID,
			Status:      status,
			Audit:       providerRequestAuditView(audit),
			Usage:       providerUsageView(usage),
		}, nil)
		return
	}
	if status == "" {
		status = llmmodels.RuntimeCandidateStatusCompleted
	}
	m.publishLLMUsageRecorded(event.Context(), audit, usage, status, "")
	result.Set(orchestratorevents.RequestRuntimeCandidateResult{
		Transaction: response.Transaction,
		Candidate:   response.Candidate,
		RequestID:   requestID,
		Status:      status,
		Audit:       providerRequestAuditView(audit),
		Usage:       providerUsageView(usage),
	}, nil)
}

func (m *Module) publishLLMUsageRecorded(ctx context.Context, audit *llmmodels.ProviderRequestAudit, usage *llmmodels.ProviderUsage, status, failureClass string) {
	if m == nil || m.rawEventHub == nil || audit == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if status == "" {
		status = audit.Status
	}
	if failureClass == "" {
		failureClass = audit.FailureClass
	}
	occurredAt := audit.CompletedAt
	if occurredAt == "" {
		occurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data := llmevents.LLMUsageRecordedData{
		RequestID:             audit.RequestID,
		WorkspaceID:           common.WorkspaceID(audit.WorkspaceID),
		RunID:                 common.RunID(audit.RunID),
		StepID:                audit.StepID,
		Target:                audit.Target,
		Operation:             audit.Operation,
		CapabilityRevision:    audit.CapabilityRevision,
		ConfigurationRevision: audit.ConfigurationRevision,
		StablePrefixHash:      audit.StablePrefixHash,
		Kind:                  audit.Kind,
		APISurface:            audit.APISurface,
		Provider:              audit.Provider,
		ProviderSubmitted:     audit.ProviderSubmitted,
		Status:                status,
		FailureClass:          failureClass,
		StartedAt:             audit.StartedAt,
		CompletedAt:           audit.CompletedAt,
		DurationMs:            audit.DurationMs,
		RequestBytes:          audit.RequestBytes,
		ResponseBytes:         audit.ResponseBytes,
		ContextBudget:         providerContextBudgetAudit(audit.ContextBudget),
		OccurredAt:            occurredAt,
	}
	if usage != nil {
		data.InputTokens = usage.InputTokens
		data.OutputTokens = usage.OutputTokens
		data.TotalTokens = usage.TotalTokens
		data.Estimated = false
		data.UsageSource = modulecommon.UsageSourceProviderNative
		data.CacheUsage = providerCacheUsageAudit(usage)
	} else if audit.ContextBudget != nil && audit.ContextBudget.ActualTokens > 0 {
		data.TotalTokens = audit.ContextBudget.ActualTokens
		data.Estimated = true
		data.UsageSource = modulecommon.UsageSourceEstimated
	} else if audit.ContextBudget != nil {
		// No provider usage: still record estimated budget snapshot when available.
		data.Estimated = true
		data.UsageSource = modulecommon.UsageSourceEstimated
	}
	event := magicevent.NewEventWithContext(
		llmevents.TopicLLMUsageRecorded,
		ModuleID,
		llmevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		data,
	)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{
		SubjectType: "llm_usage",
		SubjectID:   audit.RequestID,
		WorkspaceID: audit.WorkspaceID,
		RunID:       audit.RunID,
		StepID:      audit.StepID,
	}))
	m.rawEventHub.Post(event)
}

func providerRequestAuditView(audit *llmmodels.ProviderRequestAudit) *runtimecontract.ProviderRequestAuditView {
	if audit == nil {
		return nil
	}
	return &runtimecontract.ProviderRequestAuditView{
		RequestID:             audit.RequestID,
		WorkspaceID:           audit.WorkspaceID,
		RunID:                 audit.RunID,
		StepID:                audit.StepID,
		Target:                audit.Target,
		Operation:             audit.Operation,
		CapabilityRevision:    audit.CapabilityRevision,
		ConfigurationRevision: audit.ConfigurationRevision,
		StablePrefixHash:      audit.StablePrefixHash,
		Kind:                  audit.Kind,
		APISurface:            audit.APISurface,
		Provider:              audit.Provider,
		ContextBudget:         providerContextBudgetAudit(audit.ContextBudget),
		ProviderSubmitted:     audit.ProviderSubmitted,
		Status:                audit.Status,
		FailureClass:          audit.FailureClass,
		StartedAt:             audit.StartedAt,
		CompletedAt:           audit.CompletedAt,
		DurationMs:            audit.DurationMs,
		RequestBytes:          audit.RequestBytes,
		ResponseBytes:         audit.ResponseBytes,
	}
}

func providerUsageView(usage *llmmodels.ProviderUsage) *runtimecontract.ProviderUsageView {
	if usage == nil {
		return nil
	}
	return &runtimecontract.ProviderUsageView{
		InputTokens:             usage.InputTokens,
		OutputTokens:            usage.OutputTokens,
		TotalTokens:             usage.TotalTokens,
		CacheReadTokens:         usage.CacheReadTokens,
		CacheWriteTokens:        usage.CacheWriteTokens,
		CacheCreationTokens:     usage.CacheCreationTokens,
		CacheableInputTokens:    usage.CacheableInputTokens,
		NonCacheableInputTokens: usage.NonCacheableInputTokens,
	}
}

func providerContextBudgetAudit(budget *llmmodels.ProviderContextBudget) llmaudit.ContextBudget {
	if budget == nil {
		return llmaudit.ContextBudget{}
	}
	ret := llmaudit.ContextBudget{
		ProviderID: budget.ProviderID, ModelID: budget.ModelID,
		CapabilityRevision: budget.CapabilityRevision, ConfigurationRevision: budget.ConfigurationRevision,
		ContextWindowTokens: budget.ContextWindowTokens, ProviderMaxOutputTokens: budget.ProviderMaxOutputTokens,
		EffectiveMaxOutputTokens: budget.EffectiveMaxOutputTokens, MaxBytes: budget.MaxBytes,
		ActualBytes: budget.ActualBytes, MaxTokens: budget.MaxTokens, EstimatedInputTokens: budget.EstimatedInputTokens,
		EstimatedOutputTokens: budget.EstimatedOutputTokens, ActualTokens: budget.ActualTokens,
		MaxRequestBytes: budget.MaxRequestBytes, ProfileMode: budget.ProfileMode, CapabilitySource: budget.CapabilitySource,
		ProviderSubmitted: budget.ProviderSubmitted, OverBudget: budget.OverBudget, Truncated: budget.Truncated,
		ContextGateAction: budget.ContextGateAction, ContextGateReason: budget.ContextGateReason,
		BudgetFailureClass: budget.BudgetFailureClass,
	}
	if budget.CachePlan != nil {
		ret.StablePrefixHash = budget.CachePlan.StablePrefixHash
		ret.RenderedStablePrefixHash = budget.CachePlan.RenderedStablePrefixHash
		ret.CacheStrategy = budget.CachePlan.Strategy
	}
	return ret
}

func providerCacheUsageAudit(usage *llmmodels.ProviderUsage) llmaudit.CacheUsage {
	if usage == nil || (usage.CacheReadTokens == 0 && usage.CacheWriteTokens == 0 && usage.CacheCreationTokens == 0 &&
		usage.CacheableInputTokens == 0 && usage.NonCacheableInputTokens == 0) {
		return llmaudit.CacheUsage{}
	}
	return llmaudit.CacheUsage{
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		CacheCreationTokens: usage.CacheCreationTokens, CacheableInputTokens: usage.CacheableInputTokens,
		NonCacheableInputTokens: usage.NonCacheableInputTokens, Estimated: false, Source: modulecommon.UsageSourceProviderNative,
	}
}

func (m *Module) handleProviderCeilingPrerequisite(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireProviderCeilingCommand)
	if !ok || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	target := command.Target.Normalized()
	if err := target.Validate(); err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	profile, err := m.resolveTarget(event.Context(), target, "chat_completions")
	if err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	ceiling := llmbiz.ProfileToProviderCeiling(profile)
	if command.MinimumBytes > 0 && ceiling.MaxBytes < command.MinimumBytes {
		setPrerequisiteMagicError(result, fmt.Errorf("provider %q model %q request ceiling %d is below required minimum %d", target.ProviderID, target.ModelID, ceiling.MaxBytes, command.MinimumBytes))
		return
	}
	if err := ceiling.Validate(); err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	result.Set(orchestratorevents.AcquireProviderCeilingResult{
		PrerequisiteID: command.PrerequisiteID,
		Ceiling:        ceiling,
	}, nil)
}

func setPrerequisiteMagicError(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	switch ev.ID() {
	case llmevents.TopicLLMCommandGetProviderModelCatalog:
		cmd, ok := ev.Data().(llmevents.GetProviderModelCatalogCommand)
		if !ok {
			setLLMMagicError(result, common.ErrInvalidArgument)
			return
		}
		catalog, err := m.getCatalog(ev.Context(), cmd.ProviderID, false)
		if err != nil {
			setLLMMagicError(result, err)
			return
		}
		result.Set(llmevents.GetProviderModelCatalogResult{Catalog: catalog}, nil)
		return
	case llmevents.TopicLLMCommandRefreshProviderModelCatalog:
		cmd, ok := ev.Data().(llmevents.RefreshProviderModelCatalogCommand)
		if !ok {
			setLLMMagicError(result, common.ErrInvalidArgument)
			return
		}
		catalog, err := m.getCatalog(ev.Context(), cmd.ProviderID, true)
		if err != nil {
			setLLMMagicError(result, err)
			return
		}
		result.Set(llmevents.RefreshProviderModelCatalogResult{Catalog: catalog}, nil)
		return
	case llmevents.TopicLLMCommandResolveProviderModelTarget:
		cmd, ok := ev.Data().(llmevents.ResolveProviderModelTargetCommand)
		if !ok {
			setLLMMagicError(result, common.ErrInvalidArgument)
			return
		}
		profile, err := m.resolveTarget(ev.Context(), cmd.Target, cmd.Operation)
		if err != nil {
			setLLMMagicError(result, err)
			return
		}
		// Response target must match request target identity.
		if !profile.Target.Same(cmd.Target.Normalized()) {
			setLLMMagicError(result, fmt.Errorf("%w: resolved target mismatch", common.ErrInvalidArgument))
			return
		}
		result.Set(llmevents.ResolveProviderModelTargetResult{Profile: profile}, nil)
		return
	}

	response, err := m.executeCommand(ev)
	setLLMMagicResult(result, response, err)
}

func (m *Module) executeCommand(ev magicevent.Event) (llmevents.LLMCommandResult, error) {
	if m == nil {
		return llmevents.LLMCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case llmevents.TopicLLMCommandCompleteProvider:
		cmd, ok := ev.Data().(llmevents.CompleteProviderCommand)
		if !ok {
			return llmevents.LLMCommandResult{}, common.ErrInvalidArgument
		}
		completion, target, err := m.completeProvider(ctx, cmd.Target, cmd.Request)
		if err != nil {
			return llmevents.LLMCommandResult{}, err
		}
		return llmevents.LLMCommandResult{Target: target, Completion: llmCompletionEventData(completion)}, nil
	case llmevents.TopicLLMCommandGenerateEmbedding:
		cmd, ok := ev.Data().(llmevents.GenerateEmbeddingCommand)
		if !ok || strings.TrimSpace(cmd.Input) == "" {
			return llmevents.LLMCommandResult{}, common.ErrInvalidArgument
		}
		embedding, err := m.generateEmbedding(ctx, cmd.Target, cmd.Input)
		if err != nil {
			return llmevents.LLMCommandResult{}, err
		}
		return llmevents.LLMCommandResult{Target: cmd.Target.Normalized(), Embedding: &embedding}, nil
	default:
		return llmevents.LLMCommandResult{}, common.ErrInvalidArgument
	}
}

func llmCompletionEventData(completion llmmodels.ProviderCompletionResult) *llmevents.ProviderCompletionData {
	ret := &llmevents.ProviderCompletionData{Content: string(completion.Payload)}
	if completion.Usage != nil {
		ret.Usage = &llmevents.ProviderUsageData{
			InputTokens: completion.Usage.InputTokens, OutputTokens: completion.Usage.OutputTokens,
			TotalTokens: completion.Usage.TotalTokens, CacheReadTokens: completion.Usage.CacheReadTokens,
			CacheWriteTokens: completion.Usage.CacheWriteTokens, CacheCreationTokens: completion.Usage.CacheCreationTokens,
			CacheableInputTokens: completion.Usage.CacheableInputTokens, NonCacheableInputTokens: completion.Usage.NonCacheableInputTokens,
		}
	}
	return ret
}

func setLLMMagicResult(result magicevent.Result, data llmevents.LLMCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		setLLMMagicError(result, err)
		return
	}
	result.Set(data, nil)
}

func setLLMMagicError(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
}

func (m *Module) completeProvider(ctx context.Context, target runtimecontract.ProviderModelTarget, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, runtimecontract.ProviderModelTarget, error) {
	if m == nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, common.ErrInvalidArgument
	}
	target = target.Normalized()
	if err := target.Validate(); err != nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, err
	}
	if model := strings.TrimSpace(request.Model); model != "" && model != target.ModelID {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, fmt.Errorf("%w: provider request model %q conflicts with target model %q", common.ErrInvalidArgument, model, target.ModelID)
	}
	profile, err := m.resolveTarget(ctx, target, "chat_completions")
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, err
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, err
	}
	runtime, err := llmbiz.ResolveProviderClient(cfg.LLM, target, profile)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, err
	}
	provider := llmbiz.NewProviderFromConfig(runtime)
	if provider == nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}
	request.Model = target.ModelID
	request.Config = llmproviderconfig.RedactedProviderConfig(runtime)
	completion, err := provider.Complete(ctx, request)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, err
	}
	return completion, target, nil
}

func (m *Module) completeProviderWithSealedCeiling(ctx context.Context, ceiling runtimecontract.ProviderCeiling, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	if m == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	ceiling = ceiling.Normalized()
	if err := ceiling.Validate(); err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	if model := strings.TrimSpace(request.Model); model != "" && model != ceiling.Target.ModelID {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: provider request model %q conflicts with sealed ceiling model %q", common.ErrInvalidArgument, model, ceiling.Target.ModelID)
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	runtime, profile, err := llmbiz.ResolveProviderClientForSealedCeiling(cfg.LLM, ceiling)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	_ = profile
	provider := llmbiz.NewProviderFromConfig(runtime)
	if provider == nil {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}
	request.Model = ceiling.Target.ModelID
	request.Config = llmproviderconfig.RedactedProviderConfig(runtime)
	if request.ContextBudget == nil {
		request.ContextBudget = &llmmodels.ProviderContextBudget{}
	}
	// Ensure sealed identity is present on the request audit surface before dispatch.
	request.ContextBudget.ProviderID = ceiling.Target.ProviderID
	request.ContextBudget.ModelID = ceiling.Target.ModelID
	request.ContextBudget.CapabilityRevision = ceiling.CapabilityRevision
	request.ContextBudget.ConfigurationRevision = ceiling.ConfigurationRevision
	request.ContextBudget.CapabilitySource = "sealed_ceiling"
	request.ContextBudget.ProfileMode = "runtime_v2_sealed_ceiling"
	completion, err := provider.Complete(ctx, request)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	request.ContextBudget.ProviderSubmitted = true
	if completion.Usage != nil {
		request.ContextBudget.ActualTokens = completion.Usage.TotalTokens
	}
	return completion, nil
}

func (m *Module) generateEmbedding(ctx context.Context, target runtimecontract.ProviderModelTarget, input string) (llmmodels.ProviderEmbeddingResult, error) {
	target = target.Normalized()
	input = strings.TrimSpace(input)
	if err := target.Validate(); err != nil || input == "" {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	if m == nil {
		return llmmodels.ProviderEmbeddingResult{}, common.ErrInvalidArgument
	}
	profile, err := m.resolveTarget(ctx, target, "embeddings")
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	runtime, err := llmbiz.ResolveProviderClient(cfg.LLM, target, profile)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	return llmbiz.GenerateEmbedding(ctx, target.ProviderID, runtime, input)
}

func (m *Module) resolveTarget(ctx context.Context, target runtimecontract.ProviderModelTarget, operation string) (llmmodels.ResolvedProviderModelProfile, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return llmmodels.ResolvedProviderModelProfile{}, err
	}
	target = target.Normalized()
	var catalog *llmmodels.ProviderModelCatalog
	if target.ProviderID == runtimecontract.BuiltinAIProxyProviderID {
		current, catalogErr := m.getCatalog(ctx, target.ProviderID, false)
		if catalogErr != nil && current.State == llmmodels.ProviderCatalogStateUnavailable {
			return llmmodels.ResolvedProviderModelProfile{}, catalogErr
		}
		catalog = &current
	}
	return llmbiz.ResolveProviderModelTarget(cfg.LLM, catalog, target, operation)
}

func (m *Module) getCatalog(ctx context.Context, providerID string, forceRefresh bool) (llmmodels.ProviderModelCatalog, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		providerID = runtimecontract.BuiltinAIProxyProviderID
	}
	if providerID != runtimecontract.BuiltinAIProxyProviderID {
		return llmmodels.ProviderModelCatalog{}, fmt.Errorf("%w: catalog is only available for built-in ai-proxy", common.ErrInvalidArgument)
	}
	if m == nil || m.catalogService == nil {
		return llmmodels.ProviderModelCatalog{}, common.ErrInvalidArgument
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return llmmodels.ProviderModelCatalog{}, err
	}
	return m.catalogService.GetCatalog(ctx, cfg.LLM, forceRefresh)
}
