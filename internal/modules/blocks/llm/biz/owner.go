package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmruntime "github.com/muidea/workorch/internal/modules/blocks/llm/biz/runtime"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	llmproviderconfig "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/providerconfig"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindWorkspaceExecution = modulecommon.StepKindWorkspaceExecution

type Biz struct {
	basebiz.Base
	catalogService    *CatalogService
	candidateProvider llmruntime.CandidateProvider
	topics            []string
}

type Dependencies struct {
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
	CatalogService    *CatalogService
	CandidateProvider llmruntime.CandidateProvider
}

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine, deps ...Dependencies) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	m := &Biz{Base: basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine)}
	if len(deps) > 0 {
		m.catalogService = deps[0].CatalogService
		m.candidateProvider = deps[0].CandidateProvider
	}
	if m.catalogService == nil {
		m.catalogService = NewCatalogService()
	}
	if m.candidateProvider == nil {
		m.candidateProvider = m
	}
	return m, nil
}

func (m *Biz) Start() error {
	if m == nil || m.catalogService == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	m.topics = []string{
		llmevents.TopicLLMCommandCompleteProvider,
		llmevents.TopicLLMCommandGenerateEmbedding,
		llmevents.TopicLLMCommandGetProviderModelCatalog,
		llmevents.TopicLLMCommandRefreshProviderModelCatalog,
		llmevents.TopicLLMCommandValidateInferenceSelection,
	}
	for _, topic := range m.topics {
		m.SubscribeFunc(topic, m.handleCommand)
	}
	m.SubscribeFunc(llmevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.topics = append(m.topics, llmevents.TopicAcquireRuntimeCapabilities)
	m.SubscribeFunc(llmevents.TopicPreflightRuntimeCandidate, m.handleRuntimeCandidatePreflight)
	m.topics = append(m.topics, llmevents.TopicPreflightRuntimeCandidate)
	if m.candidateProvider != nil {
		m.SubscribeFunc(llmevents.TopicRequestRuntimeCandidate, m.handleRuntimeCandidate)
		m.topics = append(m.topics, llmevents.TopicRequestRuntimeCandidate)
	}
	m.SubscribeMatchedFunc(ModuleID+"/prerequisite", orchestratorevents.PrerequisiteCommandDestination, llmevents.TopicAcquireProviderCeiling, m.handleProviderCeilingPrerequisite)
	return nil
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(llmevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	response, err := llmevents.ResolveRuntimeCapabilities(command, ModuleID, modulecommon.RuntimeCapabilityKinds()...)
	if err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.topics = nil
	m.UnsubscribeMatchedFunc(ModuleID+"/prerequisite", llmevents.TopicAcquireProviderCeiling)
	m.catalogService = nil
	m.candidateProvider = nil
}

// handleRuntimeCandidatePreflight renders the sealed C0-C5 request and runs
// its integrity/ceiling gate without contacting the configured provider. The
// Orchestrator owns the resulting acquire/terminal transition; this block only
// reports a bounded, secret-free rendering outcome.
func (m *Biz) handleRuntimeCandidatePreflight(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
	if !ok || !runtimeCandidateCommandScopeValid(command.DocumentKind, command.WorkspaceID, command.RunID, command.Decision, command.SemanticContext) {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	request := llmmodels.RuntimeCandidateRequest{
		DocumentKind: command.DocumentKind,
		Decision:     command.Decision, Transaction: command.Transaction,
		CandidateAttempt: command.CandidateAttempt, PreviousFailure: command.PreviousFailure,
		StablePrefixHash: command.StablePrefixHash, DefinitionContext: command.DefinitionContext,
		CandidateManifest: command.CandidateManifest, CandidateContext: command.CandidateContext,
		SemanticManifest: command.SemanticManifest, SemanticContext: command.SemanticContext,
		PreviousSemanticRejection: command.PreviousSemanticRejection,
	}
	startedAt := time.Now().UTC()
	providerRequest, preflightErr := runtimeProviderRequest(request)
	completedAt := time.Now().UTC()
	budget := providerRequest.ContextBudget
	if budget != nil {
		budget.ProviderSubmitted = false
	}
	status := llmmodels.RuntimeCandidateStatusCompleted
	failureClass := ""
	if preflightErr != nil {
		status = llmmodels.RuntimeCandidateStatusFailed
		failureClass = "context_integrity"
	}
	audit := sealedAuditFromRequest(request, newProviderRequestID(request), budget, startedAt, completedAt, false, status, failureClass, 0)
	if audit != nil {
		audit.Kind = "runtime_candidate_preflight"
		audit.Operation = "runtime_candidate_preflight"
		if command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
			audit.Kind = "runtime_semantic_plan_preflight"
			audit.Operation = "runtime_semantic_plan_preflight"
		}
	}
	m.publishLLMUsageRecorded(event.Context(), audit, nil, status, failureClass)
	feedback, feedbackErr := runtimeProviderPreflightFeedback(command, preflightErr)
	if feedbackErr != nil {
		setPrerequisiteMagicError(result, feedbackErr)
		return
	}
	result.Set(llmevents.PreflightRuntimeCandidateResult{
		Transaction:  command.Transaction,
		DocumentKind: command.DocumentKind,
		Feedback:     feedback,
		Audit:        providerRequestAuditView(audit),
	}, nil)
}

func runtimeProviderPreflightFeedback(command llmevents.PreflightRuntimeCandidateCommand, preflightErr error) (runtimecontract.RuntimeFeedback, error) {
	scope := runtimecontract.RuntimeFeedbackScope{WorkspaceID: command.WorkspaceID, RunID: command.RunID}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		scope.AuthoritySnapshotID = command.Transaction.AuthoritySnapshotID
	}
	feedback := runtimecontract.RuntimeFeedback{
		Domain: runtimecontract.RuntimeFeedbackDomainProviderPreflight, StableCategory: runtimecontract.RuntimeFeedbackCategoryAccepted,
		WorkspaceID: scope.WorkspaceID, RunID: scope.RunID, AuthoritySnapshotID: scope.AuthoritySnapshotID,
	}
	if preflightErr != nil {
		reason := runtimeCandidatePreflightReason(preflightErr)
		h := sha256.Sum256([]byte(string(command.DocumentKind) + "\x00" + command.StablePrefixHash + "\x00" + reason))
		feedback.StableCategory = runtimecontract.RuntimeFeedbackCategoryContextIntegrity
		feedback.OriginalDiagnostic = &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
			OriginalClass: "context_integrity", OriginalCode: "candidate_context_integrity", OriginalMessage: reason,
			Signature: "candidate-context-integrity-" + hex.EncodeToString(h[:])[:20],
		}
	}
	if err := feedback.Validate(); err != nil {
		return runtimecontract.RuntimeFeedback{}, err
	}
	return feedback, nil
}

func runtimeCandidatePreflightReason(err error) string {
	if err == nil {
		return ""
	}
	const maxBytes = 1024
	reason := strings.TrimSpace(err.Error())
	if len(reason) <= maxBytes {
		return reason
	}
	return reason[:maxBytes]
}

func (m *Biz) handleRuntimeCandidate(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(llmevents.RequestRuntimeCandidateCommand)
	if !ok || m.candidateProvider == nil || !runtimeCandidateCommandScopeValid(command.DocumentKind, command.WorkspaceID, command.RunID, command.Decision, command.SemanticContext) {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	if err := command.RequestIdentity.Validate(); err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	response, err := llmruntime.RequestCandidate(event.Context(), m.candidateProvider, llmmodels.RuntimeCandidateRequest{
		DocumentKind:              command.DocumentKind,
		RequestIdentity:           command.RequestIdentity,
		Decision:                  command.Decision,
		Transaction:               command.Transaction,
		CandidateAttempt:          command.CandidateAttempt,
		PreviousFailure:           command.PreviousFailure,
		StablePrefixHash:          command.StablePrefixHash,
		CandidateManifest:         command.CandidateManifest,
		DefinitionContext:         command.DefinitionContext,
		CandidateContext:          command.CandidateContext,
		SemanticManifest:          command.SemanticManifest,
		SemanticContext:           command.SemanticContext,
		PreviousSemanticRejection: command.PreviousSemanticRejection,
		OutputContinuation:        command.OutputContinuation,
	})
	// Always attempt to persist sealed request audit when generation produced one.
	// Audit may also ride on CandidateRequestError for failed provider calls.
	audit := response.Audit
	usage := response.Usage
	status := ""
	if audit != nil {
		status = audit.Status
	}
	if err != nil {
		if audit == nil {
			if errAudit, _, errStatus := llmruntime.CandidateAuditFromError(err); errAudit != nil {
				audit = errAudit
				status = errStatus
			}
		}
		if status == "" {
			status = llmmodels.RuntimeCandidateStatusFailed
		}
		if stale := llmruntime.CandidatePrerequisiteStaleFromError(err); stale != nil {
			m.publishLLMUsageRecorded(event.Context(), audit, usage, status, "")
			result.Set(llmevents.RequestRuntimeCandidateResult{
				Transaction:     command.Transaction,
				DocumentKind:    command.DocumentKind,
				RequestIdentity: command.RequestIdentity,
				Stale:           stale,
				Audit:           providerRequestAuditView(audit),
				Usage:           usage,
			}, nil)
			return
		}
		failure := llmruntime.CandidateFailureFromError(err)
		m.publishLLMUsageRecorded(event.Context(), audit, usage, status, string(failure.Class))
		feedback, feedbackErr := runtimeProviderRequestFeedback(command, failure)
		if feedbackErr != nil {
			setPrerequisiteMagicError(result, feedbackErr)
			return
		}
		result.Set(llmevents.RequestRuntimeCandidateResult{
			Transaction:     command.Transaction,
			DocumentKind:    command.DocumentKind,
			RequestIdentity: command.RequestIdentity,
			Feedback:        &feedback,
			Audit:           providerRequestAuditView(audit),
			Usage:           usage,
		}, nil)
		return
	}
	if status == "" {
		status = llmmodels.RuntimeCandidateStatusCompleted
	}
	m.publishLLMUsageRecorded(event.Context(), audit, usage, status, "")
	result.Set(llmevents.RequestRuntimeCandidateResult{
		Transaction:     response.Transaction,
		DocumentKind:    response.DocumentKind,
		RequestIdentity: command.RequestIdentity,
		DocumentContent: append([]byte(nil), response.DocumentContent...),
		OutputFragment:  response.OutputFragment,
		Audit:           providerRequestAuditView(audit),
		Usage:           usage,
	}, nil)
}

func runtimeProviderRequestFeedback(command llmevents.RequestRuntimeCandidateCommand, failure runtimecontract.RuntimeCandidateFailure) (runtimecontract.RuntimeFeedback, error) {
	category := runtimecontract.RuntimeFeedbackCategoryProviderProtocol
	switch failure.Class {
	case runtimecontract.RuntimeCandidateFailureClassProviderAuthentication:
		category = runtimecontract.RuntimeFeedbackCategoryProviderAuthentication
	case runtimecontract.RuntimeCandidateFailureClassProviderConfiguration:
		category = runtimecontract.RuntimeFeedbackCategoryProviderConfiguration
	case runtimecontract.RuntimeCandidateFailureClassProviderTransient:
		category = runtimecontract.RuntimeFeedbackCategoryProviderTransient
	case runtimecontract.RuntimeCandidateFailureClassProvider,
		runtimecontract.RuntimeCandidateFailureClassSchema,
		runtimecontract.RuntimeCandidateFailureClassContract,
		runtimecontract.RuntimeCandidateFailureClassInternal:
		// These are owner-side request/protocol failures. A complete document's
		// schema or contract rejection is never returned through this branch.
	default:
		failure.Class = runtimecontract.RuntimeCandidateFailureClassInternal
	}
	if strings.TrimSpace(failure.Code) == "" {
		failure.Code = string(category)
	}
	if strings.TrimSpace(failure.Signature) == "" {
		h := sha256.New()
		_, _ = h.Write([]byte(command.RequestIdentity.LogicalRequestID))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(fmt.Sprintf("%d", command.RequestIdentity.Sequence)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(command.RequestIdentity.PhysicalCallID))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(failure.Class))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(failure.Code))
		failure.Signature = "provider-request-" + hex.EncodeToString(h.Sum(nil))[:20]
	}
	scope := runtimecontract.RuntimeFeedbackScope{WorkspaceID: command.WorkspaceID, RunID: command.RunID}
	if command.DocumentKind == runtimecontract.RuntimeProviderDocumentOperations {
		scope.AuthoritySnapshotID = command.Transaction.AuthoritySnapshotID
	}
	return runtimecontract.RuntimeFeedbackFromCandidateFailure(scope, runtimecontract.RuntimeFeedbackDomainProviderRequest, category, runtimecontract.RuntimeModuleLLM, failure)
}

func runtimeCandidateCommandScopeValid(kind runtimecontract.RuntimeProviderDocumentKind, workspaceID common.WorkspaceID, runID common.RunID, decision runtimecontract.RuntimeDecision, semanticContext runtimecontract.RuntimeSemanticPlanRequestContext) bool {
	if workspaceID == "" || runID == "" {
		return false
	}
	switch kind {
	case runtimecontract.RuntimeProviderDocumentOperations:
		return decision.AuthoritySnapshot.WorkspaceID == workspaceID && decision.AuthoritySnapshot.RunID == runID
	case runtimecontract.RuntimeProviderDocumentSemanticPlan:
		return semanticContext.WorkspaceID == workspaceID && semanticContext.RunID == runID
	default:
		return false
	}
}

func (m *Biz) publishLLMUsageRecorded(ctx context.Context, audit *llmmodels.ProviderRequestAudit, usage *llmaudit.ProviderUsage, status, failureClass string) {
	if m == nil || m.EventHub() == nil || audit == nil {
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
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(llmevents.Contracts(), event.ID(), eventcontract.LaneScope{
		SubjectType: "llm_usage",
		SubjectID:   audit.RequestID,
		WorkspaceID: audit.WorkspaceID,
		RunID:       audit.RunID,
		StepID:      audit.StepID,
	}))
	m.PostEvent(event)
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
		IncludedSections:   boundedLLMAuditStrings(budget.IncludedSections),
		NormalizedSections: boundedLLMAuditStrings(budget.NormalizedSections),
		CompressedSections: boundedLLMAuditStrings(budget.CompressedSections),
		OmittedSections:    boundedLLMAuditStrings(budget.OmittedSections),
		RedactedPaths:      boundedLLMAuditStrings(budget.RedactedPaths),
		ProtectedFields:    boundedLLMAuditStrings(budget.ProtectedFields),
		IntegrityWarnings:  boundedLLMAuditStrings(budget.IntegrityWarnings),
		SourceRefs:         boundedLLMAuditStrings(budget.SourceRefs),
	}
	if budget.CachePlan != nil {
		ret.StablePrefixHash = budget.CachePlan.StablePrefixHash
		ret.RenderedStablePrefixHash = budget.CachePlan.RenderedStablePrefixHash
		ret.CacheStrategy = budget.CachePlan.Strategy
		ret.SectionHashes = boundedLLMAuditSections(budget.CachePlan.SectionHashes)
	}
	return ret
}

const (
	llmAuditSliceLimit      = 24
	llmAuditStringByteLimit = 512
)

// boundedLLMAuditStrings prevents an owner-provided diagnostic, path, or ref
// from turning the audit event into a prompt-content transport. The original
// provider request remains private to the LLM adapter; cross-owner audit only
// carries short identifiers and warnings.
func boundedLLMAuditStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, min(len(values), llmAuditSliceLimit))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > llmAuditStringByteLimit {
			value = value[:llmAuditStringByteLimit] + "...[truncated]"
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
		if len(ret) == llmAuditSliceLimit {
			break
		}
	}
	return ret
}

func boundedLLMAuditSections(values []llmmodels.ProviderCacheSectionHash) []llmaudit.ContextSection {
	if len(values) == 0 {
		return nil
	}
	ret := make([]llmaudit.ContextSection, 0, min(len(values), llmAuditSliceLimit))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value.SectionID)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		ret = append(ret, llmaudit.ContextSection{
			ID: id, Hash: strings.TrimSpace(value.Hash), SizeBytes: value.SizeBytes,
			SourceRefs: boundedLLMAuditStrings(value.SourceRefs),
		})
		if len(ret) == llmAuditSliceLimit {
			break
		}
	}
	return ret
}

func providerCacheUsageAudit(usage *llmaudit.ProviderUsage) llmaudit.CacheUsage {
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

func (m *Biz) handleProviderCeilingPrerequisite(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(llmevents.AcquireProviderCeilingCommand)
	if !ok || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	selection := command.Selection.Normalized()
	if err := selection.Validate(); err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	target := selection.Target
	profile, err := m.resolveTarget(event.Context(), target, workorchconfig.LLMOperationResponses)
	if err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	ceiling := ProfileToProviderCeiling(profile)
	ceiling.Selection = selection
	if command.MinimumBytes > 0 && ceiling.MaxBytes < command.MinimumBytes {
		setPrerequisiteMagicError(result, fmt.Errorf("provider %q model %q request ceiling %d is below required minimum %d", target.ProviderID, target.ModelID, ceiling.MaxBytes, command.MinimumBytes))
		return
	}
	if err := ceiling.Validate(); err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	observation := providerCeilingObservation(command.PrerequisiteID, ceiling)
	if err := observation.Validate(); err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	result.Set(llmevents.AcquireProviderCeilingResult{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID, AuthoritySnapshotID: command.AuthoritySnapshotID,
		Ceiling: ceiling, Observation: observation,
	}, nil)
}

// providerCeilingObservation records the exact configuration revision used to
// acquire a sealed provider ceiling. The target is immutable for the Run; a
// later configuration revision is a prerequisite freshness event, never a
// silent provider/model substitution.
func providerCeilingObservation(prerequisiteID string, ceiling runtimecontract.ProviderCeiling) runtimecontract.RuntimePrerequisiteObservation {
	ceiling = ceiling.Normalized()
	subject := ceiling.Selection.Target.ProviderID + "/" + ceiling.Selection.Target.ModelID
	contentHash := fmt.Sprintf("provider-ceiling:%s:%s:%s:%s:%d:%d:%d:%d:%d",
		ceiling.Selection.Target.ProviderID, ceiling.Selection.Target.ModelID, ceiling.CapabilityRevision, ceiling.ConfigurationRevision,
		ceiling.ContextWindowTokens, ceiling.ProviderMaxOutputTokens, ceiling.EffectiveMaxOutputTokens, ceiling.MaxInputTokens, ceiling.MaxBytes)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID,
		Kind:           runtimecontract.PrerequisiteKindProviderCeiling,
		OwnerModuleID:  ModuleID,
		ContentHash:    contentHash,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: ModuleID,
			SubjectRef: subject, Revision: ceiling.ConfigurationRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func setPrerequisiteMagicError(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
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
	case llmevents.TopicLLMCommandValidateInferenceSelection:
		cmd, ok := ev.Data().(llmevents.ValidateProviderInferenceSelectionCommand)
		if !ok {
			setLLMMagicError(result, common.ErrInvalidArgument)
			return
		}
		selection, revision, err := m.validateInferenceSelection(ev.Context(), cmd.Selection, cmd.Operation)
		if err != nil {
			setLLMMagicError(result, err)
			return
		}
		result.Set(llmevents.ValidateProviderInferenceSelectionResult{Selection: selection, CapabilityRevision: revision}, nil)
		return
	}

	response, err := m.executeCommand(ev)
	setLLMMagicResult(result, response, err)
}

func (m *Biz) validateInferenceSelection(ctx context.Context, selection runtimecontract.ProviderInferenceSelection, operation string) (runtimecontract.ProviderInferenceSelection, string, error) {
	selection = selection.Normalized()
	if err := selection.Validate(); err != nil {
		return runtimecontract.ProviderInferenceSelection{}, "", err
	}
	profile, err := m.resolveTarget(ctx, selection.Target, operation)
	if err != nil {
		return runtimecontract.ProviderInferenceSelection{}, "", err
	}
	if selection.ReasoningEffort == "" {
		return selection, profile.CapabilityRevision, nil
	}
	catalog, err := m.getCatalog(ctx, selection.Target.ProviderID, false)
	if err != nil {
		return runtimecontract.ProviderInferenceSelection{}, "", err
	}
	for _, descriptor := range catalog.Models {
		if descriptor.Target.Same(selection.Target) {
			if !descriptor.Reasoning.Allows(selection.ReasoningEffort) {
				return runtimecontract.ProviderInferenceSelection{}, "", fmt.Errorf("%w: reasoning effort %q is not allowed for %s/%s", common.ErrInvalidArgument, selection.ReasoningEffort, selection.Target.ProviderID, selection.Target.ModelID)
			}
			return selection, descriptor.CapabilityRevision, nil
		}
	}
	return runtimecontract.ProviderInferenceSelection{}, "", common.ErrNotFound
}

func (m *Biz) executeCommand(ev magicevent.Event) (llmevents.LLMCommandResult, error) {
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
		request, err := cmd.Request.ToModel()
		if err != nil {
			return llmevents.LLMCommandResult{}, err
		}
		completion, target, err := m.completeProvider(ctx, cmd.Target, request)
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
		usage := *completion.Usage
		ret.Usage = &usage
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

func (m *Biz) completeProvider(ctx context.Context, target runtimecontract.ProviderModelTarget, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, runtimecontract.ProviderModelTarget, error) {
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
	profile, err := m.resolveTarget(ctx, target, workorchconfig.LLMOperationResponses)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, err
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, err
	}
	runtime, err := ResolveProviderClient(cfg.LLM, target, profile)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, err
	}
	provider := NewProviderFromConfig(runtime)
	if provider == nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}
	request.Model = target.ModelID
	request.Config = llmproviderconfig.RedactedProviderConfig(runtime)
	var completion llmmodels.ProviderCompletionResult
	if runtime.Capabilities.Streaming {
		streaming, ok := provider.(StreamingProvider)
		if !ok {
			return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, fmt.Errorf("%w: provider does not implement streaming", common.ErrInvalidArgument)
		}
		completion, err = streaming.StreamComplete(ctx, request, func(llmmodels.ProviderStreamEvent) error { return nil })
	} else {
		completion, err = provider.Complete(ctx, request)
	}
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, runtimecontract.ProviderModelTarget{}, err
	}
	return completion, target, nil
}

func (m *Biz) completeProviderWithSealedCeiling(ctx context.Context, ceiling runtimecontract.ProviderCeiling, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, error) {
	if m == nil {
		return llmmodels.ProviderCompletionResult{}, common.ErrInvalidArgument
	}
	ceiling = ceiling.Normalized()
	if err := ceiling.Validate(); err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	if model := strings.TrimSpace(request.Model); model != "" && model != ceiling.Selection.Target.ModelID {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: provider request model %q conflicts with sealed ceiling model %q", common.ErrInvalidArgument, model, ceiling.Selection.Target.ModelID)
	}
	cfg, err := workorchconfig.Current()
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	runtime, profile, err := ResolveProviderClientForSealedCeiling(cfg.LLM, ceiling)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, err
	}
	_ = profile
	if request.MaxOutputTokens > 0 && request.MaxOutputTokens < runtime.InferenceProfile.MaxOutputTokens {
		runtime.InferenceProfile.MaxOutputTokens = request.MaxOutputTokens
	}
	provider := NewProviderFromConfig(runtime)
	if provider == nil {
		return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}
	request.Model = ceiling.Selection.Target.ModelID
	request.ReasoningEffort = ceiling.Selection.ReasoningEffort
	request.Config = llmproviderconfig.RedactedProviderConfig(runtime)
	if request.ContextBudget == nil {
		request.ContextBudget = &llmmodels.ProviderContextBudget{}
	}
	// Ensure sealed identity is present on the request audit surface before dispatch.
	request.ContextBudget.ProviderID = ceiling.Selection.Target.ProviderID
	request.ContextBudget.ModelID = ceiling.Selection.Target.ModelID
	request.ContextBudget.CapabilityRevision = ceiling.CapabilityRevision
	request.ContextBudget.ConfigurationRevision = ceiling.ConfigurationRevision
	request.ContextBudget.CapabilitySource = "sealed_ceiling"
	request.ContextBudget.ProfileMode = "runtime_sealed_ceiling"
	var completion llmmodels.ProviderCompletionResult
	if runtime.Capabilities.Streaming {
		streaming, ok := provider.(StreamingProvider)
		if !ok {
			return llmmodels.ProviderCompletionResult{}, fmt.Errorf("%w: provider does not implement streaming", common.ErrInvalidArgument)
		}
		completion, err = streaming.StreamComplete(ctx, request, func(llmmodels.ProviderStreamEvent) error { return nil })
	} else {
		completion, err = provider.Complete(ctx, request)
	}
	if err != nil {
		return completion, err
	}
	request.ContextBudget.ProviderSubmitted = true
	if completion.Usage != nil {
		request.ContextBudget.ActualTokens = completion.Usage.TotalTokens
	}
	return completion, nil
}

func (m *Biz) generateEmbedding(ctx context.Context, target runtimecontract.ProviderModelTarget, input string) (llmmodels.ProviderEmbeddingResult, error) {
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
	runtime, err := ResolveProviderClient(cfg.LLM, target, profile)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, err
	}
	return GenerateEmbedding(ctx, target.ProviderID, runtime, input)
}

func (m *Biz) resolveTarget(ctx context.Context, target runtimecontract.ProviderModelTarget, operation string) (llmmodels.ResolvedProviderModelProfile, error) {
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
	return ResolveProviderModelTarget(cfg.LLM, catalog, target, operation)
}

func (m *Biz) getCatalog(ctx context.Context, providerID string, forceRefresh bool) (llmmodels.ProviderModelCatalog, error) {
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
