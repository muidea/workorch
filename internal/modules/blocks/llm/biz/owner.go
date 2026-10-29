package llm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
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
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID
const StepKindWorkspaceExecution = modulecommon.StepKindWorkspaceExecution

const (
	llmApplicationOwnerSource     = "application/workorchd"
	llmSetupDraftAppserviceSource = "application/workorchd/appservice"
)

type Biz struct {
	basebiz.Base
	catalogService                    *CatalogService
	candidateProvider                 llmruntime.CandidateProvider
	loadLLMConfig                     func() (workorchconfig.LLMConfig, error)
	activeLLMConfig                   workorchconfig.LLMConfig
	activeLLMConfigurationRevision    string
	activeLLMConfigurationInitialized bool
	preparationConfigurationEpoch     string
	physicalSubmissionGuard           *runtimePhysicalSubmissionGuard
	subscriptions                     []ownerSubscription
	started                           bool
	lifecycle                         basebiz.Lifecycle
	shutdownMu                        sync.Mutex
	releasing                         bool
	released                          bool
}

type Dependencies struct {
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
	CatalogService    *CatalogService
	CandidateProvider llmruntime.CandidateProvider
	LoadLLMConfig     func() (workorchconfig.LLMConfig, error)
}

type providerExecutionMetadata struct {
	Target               runtimecontract.ProviderModelTarget
	Profile              llmmodels.ResolvedProviderModelProfile
	PhysicalResponseMode llmaudit.PhysicalResponseMode
	Submitted            bool
}

type providerCommandExecution struct {
	Result any
	Audit  *llmmodels.ProviderRequestAudit
	Usage  *llmaudit.ProviderUsage
}

var providerCommandSequence atomic.Uint64

func New(eventHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine, deps ...Dependencies) (*Biz, error) {
	if eventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	m := &Biz{
		Base:                    basebiz.New(modulecommon.ModuleID, eventHub, backgroundRoutine),
		physicalSubmissionGuard: newRuntimePhysicalSubmissionGuard(defaultRuntimePhysicalSubmissionGuardLimit),
	}
	if len(deps) > 0 {
		m.catalogService = deps[0].CatalogService
		m.candidateProvider = deps[0].CandidateProvider
		m.loadLLMConfig = deps[0].LoadLLMConfig
	}
	if m.catalogService == nil {
		m.catalogService = NewCatalogService()
	}
	if m.candidateProvider == nil {
		m.candidateProvider = m
	}
	if m.loadLLMConfig == nil {
		m.loadLLMConfig = func() (workorchconfig.LLMConfig, error) {
			cfg, err := workorchconfig.Current()
			if err != nil {
				return workorchconfig.LLMConfig{}, err
			}
			return cfg.LLM, nil
		}
	}
	return m, nil
}

func (m *Biz) Start() error {
	if m == nil || m.catalogService == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	leave, err := m.enterOwner(false)
	if err != nil {
		return err
	}
	defer leave()
	if m.started {
		return nil
	}
	if len(m.subscriptions) != 0 {
		return common.ErrConflict
	}
	if !m.activeLLMConfigurationInitialized {
		if err := m.captureActiveLLMConfiguration(); err != nil {
			return err
		}
	}
	topics := []string{
		llmevents.TopicLLMCommandCompleteProvider,
		llmevents.TopicLLMCommandCompleteProviderDocument,
		llmevents.TopicLLMCommandGenerateEmbedding,
		llmevents.TopicLLMCommandGetActiveConfiguration,
		llmevents.TopicLLMCommandGetProviderModelCatalog,
		llmevents.TopicLLMCommandRefreshProviderModelCatalog,
		llmevents.TopicLLMCommandValidateInferenceSelection,
		llmevents.TopicObserveProviderCatalogCache,
	}
	for _, topic := range topics {
		if err := m.subscribeOwner(topic, m.handleCommand, false); err != nil {
			return err
		}
	}
	if err := m.subscribeOwner(llmevents.TopicAcquireRuntimeCapabilities, m.handleRuntimeCapabilities, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(llmevents.TopicPreflightRuntimeCandidate, m.handleRuntimeCandidatePreflight, false); err != nil {
		return err
	}
	if m.candidateProvider != nil {
		if err := m.subscribeOwner(llmevents.TopicRequestRuntimeCandidate, m.handleRuntimeCandidate, false); err != nil {
			return err
		}
	}
	if err := m.subscribeOwner(llmevents.TopicAcquireProviderCeiling, m.handleProviderCeilingPrerequisite, true); err != nil {
		return err
	}
	m.started = true
	return nil
}

// captureActiveLLMConfiguration freezes the provider settings for this LLM
// owner lifecycle. The framework configuration watcher may reload TOML later,
// but it cannot rotate credentials, models, or budget policy underneath an
// active owner or a sealed Runtime request.
func (m *Biz) captureActiveLLMConfiguration() error {
	if m == nil || m.loadLLMConfig == nil {
		return common.ErrInvalidArgument
	}
	configured, err := m.loadLLMConfig()
	if err != nil {
		return err
	}
	configured, err = workorchconfig.NormalizeLLMConfig(configured)
	if err != nil {
		return err
	}
	revision, err := workorchconfig.LLMConfigurationRevision(configured)
	if err != nil {
		return err
	}
	m.activeLLMConfig = workorchconfig.CloneLLMConfig(configured)
	m.activeLLMConfigurationRevision = revision
	m.preparationConfigurationEpoch = "llm-preparation-epoch:" + rand.Text()
	m.activeLLMConfigurationInitialized = true
	return nil
}

func (m *Biz) activeConfiguration() (workorchconfig.LLMConfig, error) {
	if m == nil || !m.activeLLMConfigurationInitialized || m.activeLLMConfigurationRevision == "" {
		return workorchconfig.LLMConfig{}, fmt.Errorf("%w: llm owner configuration is not initialized", common.ErrInvalidArgument)
	}
	return workorchconfig.CloneLLMConfig(m.activeLLMConfig), nil
}

func (m *Biz) activeConfigurationStatus() (llmevents.GetActiveConfigurationResult, error) {
	configured, err := m.activeConfiguration()
	if err != nil {
		return llmevents.GetActiveConfigurationResult{}, err
	}
	ret := llmevents.GetActiveConfigurationResult{
		ProviderID:            runtimecontract.BuiltinAIProxyProviderID,
		ConfigurationRevision: m.activeLLMConfigurationRevision,
		DefaultChat:           configured.DefaultChat.ToRuntimeTarget(),
	}
	if configured.DefaultEmbedding != nil {
		value := configured.DefaultEmbedding.ToRuntimeTarget()
		ret.DefaultEmbedding = &value
	}
	return ret, nil
}

func (m *Biz) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if !runtimeCommandHasExactOrchestratorSource(event) {
		setPrerequisiteMagicError(result, fmt.Errorf("%w: runtime LLM capability acquisition requires the exact Orchestrator source", common.ErrInvalidArgument))
		return
	}
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

func (m *Biz) Teardown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.BeginShutdown(ctx)
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	m.shutdownMu.Lock()
	if m.releasing {
		m.shutdownMu.Unlock()
		return common.ErrConflict
	}
	m.releasing = true
	m.shutdownMu.Unlock()
	defer func() {
		m.shutdownMu.Lock()
		m.releasing = false
		m.shutdownMu.Unlock()
	}()
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	for len(m.subscriptions) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(m.subscriptions) - 1
		entry := m.subscriptions[last]
		var err error
		if entry.matched {
			err = m.UnsubscribeMatchedFuncChecked(ModuleID+"/prerequisite", entry.topic)
		} else {
			err = m.UnsubscribeFuncChecked(entry.topic)
		}
		if err != nil {
			return err
		}
		m.subscriptions = m.subscriptions[:last]
	}
	m.catalogService = nil
	m.candidateProvider = nil
	m.activeLLMConfig = workorchconfig.LLMConfig{}
	m.activeLLMConfigurationRevision = ""
	m.preparationConfigurationEpoch = ""
	m.activeLLMConfigurationInitialized = false
	m.physicalSubmissionGuard.clear()
	m.started = false
	m.shutdownMu.Lock()
	m.released = true
	m.shutdownMu.Unlock()
	return nil
}

// handleRuntimeCandidatePreflight renders the sealed C0-C5 request and runs
// its integrity/ceiling gate without contacting the configured provider. The
// Orchestrator owns the resulting acquire/terminal transition; this block only
// reports a bounded, secret-free rendering outcome.
func (m *Biz) handleRuntimeCandidatePreflight(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if !runtimeCommandHasExactOrchestratorSource(event) {
		setPrerequisiteMagicError(result, fmt.Errorf("%w: runtime LLM preflight requires the exact Orchestrator source", common.ErrInvalidArgument))
		return
	}
	command, ok := event.Data().(llmevents.PreflightRuntimeCandidateCommand)
	if !ok || !runtimeCandidateCommandScopeValid(command.DocumentKind, command.WorkspaceID, command.RunID, command.Decision, command.SemanticContext) {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	request := llmmodels.RuntimeCandidateRequest{
		DocumentKind: command.DocumentKind,
		Decision:     command.Decision, Transaction: command.Transaction,
		CandidateAttempt: command.CandidateAttempt, CandidateCorrectionObservation: cloneRuntimeCandidateCorrectionObservation(command.CandidateCorrectionObservation), CandidateCorrection: command.CandidateCorrection.Clone(),
		StablePrefixHash: command.StablePrefixHash, DefinitionContext: command.DefinitionContext,
		CandidateManifest: command.CandidateManifest, CandidateContext: command.CandidateContext,
		SemanticManifest: command.SemanticManifest, SemanticContext: command.SemanticContext,
		SemanticCorrectionObservation: cloneRuntimeSemanticCorrectionObservation(command.SemanticCorrectionObservation),
		SemanticCorrectionDirective:   cloneRuntimeSemanticCorrectionDirective(command.SemanticCorrectionDirective),
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
	if err := m.publishLLMUsageRecorded(event.Context(), audit, nil, status, failureClass); err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
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
	// Candidate generation is a paid synchronous execute contract. A Post has
	// no acknowledgement channel and must not reach the provider.
	if result == nil {
		return
	}
	if !runtimeCommandHasExactOrchestratorSource(event) {
		setPrerequisiteMagicError(result, fmt.Errorf("%w: runtime LLM request requires the exact Orchestrator source", common.ErrInvalidArgument))
		return
	}
	command, ok := event.Data().(llmevents.RequestRuntimeCandidateCommand)
	if !ok || m.candidateProvider == nil || m.physicalSubmissionGuard == nil || !runtimeCandidateCommandScopeValid(command.DocumentKind, command.WorkspaceID, command.RunID, command.Decision, command.SemanticContext) {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	if err := command.RequestIdentity.Validate(); err != nil {
		setPrerequisiteMagicError(result, err)
		return
	}
	if command.RequestIdentity.Normalized() != command.RequestIdentity {
		setPrerequisiteMagicError(result, common.ErrInvalidArgument)
		return
	}
	response, err := llmruntime.RequestCandidate(event.Context(), runtimePhysicalSubmissionGuardedProvider{
		guard: m.physicalSubmissionGuard, delegate: m.candidateProvider,
	}, llmmodels.RuntimeCandidateRequest{
		DocumentKind:                   command.DocumentKind,
		RequestIdentity:                command.RequestIdentity,
		ExpectedDocument:               command.ExpectedDocument,
		Decision:                       command.Decision,
		Transaction:                    command.Transaction,
		CandidateAttempt:               command.CandidateAttempt,
		CandidateCorrectionObservation: cloneRuntimeCandidateCorrectionObservation(command.CandidateCorrectionObservation),
		CandidateCorrection:            command.CandidateCorrection.Clone(),
		StablePrefixHash:               command.StablePrefixHash,
		CandidateManifest:              command.CandidateManifest,
		DefinitionContext:              command.DefinitionContext,
		CandidateContext:               command.CandidateContext,
		SemanticManifest:               command.SemanticManifest,
		SemanticContext:                command.SemanticContext,
		SemanticCorrectionObservation:  cloneRuntimeSemanticCorrectionObservation(command.SemanticCorrectionObservation),
		SemanticCorrectionDirective:    cloneRuntimeSemanticCorrectionDirective(command.SemanticCorrectionDirective),
	})
	if isRuntimePhysicalSubmissionGuardError(err) {
		setPrerequisiteMagicError(result, err)
		return
	}
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
			if auditErr := m.publishLLMUsageRecorded(event.Context(), audit, usage, status, ""); auditErr != nil {
				setPrerequisiteMagicError(result, auditErr)
				return
			}
			candidateResult := llmevents.RequestRuntimeCandidateResult{
				WorkspaceID: command.WorkspaceID, RunID: command.RunID,
				Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
				ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
				Stale:                stale,
				Audit:                providerRequestAuditView(audit),
				Usage:                usage,
			}
			if validateErr := candidateResult.Validate(); validateErr != nil {
				setPrerequisiteMagicError(result, validateErr)
				return
			}
			result.Set(candidateResult, nil)
			return
		}
		failure := llmruntime.CandidateFailureFromError(err)
		if auditErr := m.publishLLMUsageRecorded(event.Context(), audit, usage, status, string(failure.Class)); auditErr != nil {
			setPrerequisiteMagicError(result, auditErr)
			return
		}
		feedback, feedbackErr := runtimeProviderRequestFeedback(command, failure)
		if feedbackErr != nil {
			setPrerequisiteMagicError(result, feedbackErr)
			return
		}
		candidateResult := llmevents.RequestRuntimeCandidateResult{
			WorkspaceID: command.WorkspaceID, RunID: command.RunID,
			Transaction: command.Transaction, DocumentKind: command.DocumentKind, RequestIdentity: command.RequestIdentity,
			ExpectedDocumentHash: runtimeExpectedDocumentHash(command.ExpectedDocument),
			Feedback:             &feedback,
			Audit:                providerRequestAuditView(audit),
			Usage:                usage,
		}
		if validateErr := candidateResult.Validate(); validateErr != nil {
			setPrerequisiteMagicError(result, validateErr)
			return
		}
		result.Set(candidateResult, nil)
		return
	}
	if status == "" {
		status = llmmodels.RuntimeCandidateStatusCompleted
	}
	if auditErr := m.publishLLMUsageRecorded(event.Context(), audit, usage, status, ""); auditErr != nil {
		setPrerequisiteMagicError(result, auditErr)
		return
	}
	var documentOutcome *llmevents.RuntimeProviderDocumentOutcome
	if response.DocumentOutcome != nil {
		observation := response.DocumentOutcome.Normalized()
		value := llmevents.RuntimeProviderDocumentOutcome{
			Framing: observation.Framing,
			Content: append([]byte(nil), observation.Content...),
		}
		documentOutcome = &value
	}
	candidateResult := llmevents.RequestRuntimeCandidateResult{
		WorkspaceID: command.WorkspaceID, RunID: command.RunID,
		Transaction:          response.Transaction,
		DocumentKind:         response.DocumentKind,
		RequestIdentity:      command.RequestIdentity,
		ExpectedDocumentHash: response.ExpectedDocumentHash,
		DocumentOutcome:      documentOutcome,
		Audit:                providerRequestAuditView(audit),
		Usage:                usage,
	}
	if validateErr := candidateResult.Validate(); validateErr != nil {
		setPrerequisiteMagicError(result, validateErr)
		return
	}
	result.Set(candidateResult, nil)
}

func cloneRuntimeSemanticCorrectionDirective(value *runtimecontract.RuntimeSemanticCorrectionDirective) *runtimecontract.RuntimeSemanticCorrectionDirective {
	if value == nil {
		return nil
	}
	clone := value.Clone()
	return &clone
}

func cloneRuntimeCandidateCorrectionObservation(value *runtimecontract.RuntimeCandidateCorrectionObservation) *runtimecontract.RuntimeCandidateCorrectionObservation {
	if value == nil {
		return nil
	}
	clone := value.Clone()
	return &clone
}

func cloneRuntimeSemanticCorrectionObservation(value *runtimecontract.RuntimeSemanticCorrectionObservation) *runtimecontract.RuntimeSemanticCorrectionObservation {
	if value == nil {
		return nil
	}
	clone := value.Clone()
	return &clone
}

func cloneRuntimeOriginalDiagnostic(value *runtimecontract.RuntimeOriginalDiagnostic) *runtimecontract.RuntimeOriginalDiagnostic {
	return value.CloneOriginal()
}

func runtimeExpectedDocumentHash(expected *runtimecontract.RuntimeExpectedDocumentContract) string {
	if expected == nil {
		return ""
	}
	return expected.Normalized().Hash
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
	case runtimecontract.RuntimeCandidateFailureClassProviderProtocol:
		category = runtimecontract.RuntimeFeedbackCategoryProviderProtocol
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

func (m *Biz) publishLLMUsageRecorded(ctx context.Context, audit *llmmodels.ProviderRequestAudit, usage *llmaudit.ProviderUsage, status, failureClass string) error {
	if audit == nil {
		return fmt.Errorf("%w: LLM usage audit is required", common.ErrInvalidArgument)
	}
	if m == nil || m.EventHub() == nil {
		return fmt.Errorf("%w: LLM usage audit publisher is not configured", common.ErrInvalidArgument)
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
		LogicalRequestID:      audit.LogicalRequestID,
		PhysicalCallID:        audit.PhysicalCallID,
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
		PhysicalResponseMode:  audit.PhysicalResponseMode,
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
	if err := data.Validate(); err != nil {
		return fmt.Errorf("%w: invalid LLM usage audit: %v", common.ErrInvalidArgument, err)
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
	return nil
}

func providerRequestAuditView(audit *llmmodels.ProviderRequestAudit) *runtimecontract.ProviderRequestAuditView {
	if audit == nil {
		return nil
	}
	return &runtimecontract.ProviderRequestAuditView{
		RequestID:             audit.RequestID,
		LogicalRequestID:      audit.LogicalRequestID,
		PhysicalCallID:        audit.PhysicalCallID,
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
		PhysicalResponseMode:  audit.PhysicalResponseMode,
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
		usage.CacheableInputTokens == 0 && usage.NonCacheableInputTokens == 0 && !usage.CacheReadTokensKnown && !usage.CacheWriteTokensKnown) {
		return llmaudit.CacheUsage{}
	}
	return llmaudit.CacheUsage{
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
		CacheReadTokensKnown: usage.CacheReadTokensKnown || usage.CacheReadTokens != 0, CacheWriteTokensKnown: usage.CacheWriteTokensKnown || usage.CacheWriteTokens != 0,
		CacheCreationTokens: usage.CacheCreationTokens, CacheableInputTokens: usage.CacheableInputTokens,
		NonCacheableInputTokens: usage.NonCacheableInputTokens, Estimated: false, Source: modulecommon.UsageSourceProviderNative,
	}
}

func (m *Biz) handleProviderCeilingPrerequisite(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if !runtimeCommandHasExactOrchestratorSource(event) {
		setPrerequisiteMagicError(result, fmt.Errorf("%w: provider ceiling acquisition requires the exact Orchestrator source", common.ErrInvalidArgument))
		return
	}
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
	// Every LLM topic is a synchronous query/execute contract. In particular,
	// completion, embedding, refresh, and candidate calls must never be invoked
	// by fire-and-forget Post delivery.
	if result == nil {
		return
	}
	if !llmCommandHasExactApplicationSource(ev) {
		setLLMMagicError(result, fmt.Errorf("%w: LLM command requires its exact Application source", common.ErrInvalidArgument))
		return
	}
	switch ev.ID() {
	case llmevents.TopicObserveProviderCatalogCache:
		command, ok := ev.Data().(llmevents.ObserveProviderCatalogCacheCommand)
		if !ok || command.Validate() != nil {
			setLLMMagicError(result, common.ErrInvalidArgument)
			return
		}
		value, err := m.observeProviderCatalogCache(ev.Context(), command)
		if err != nil {
			// This path intentionally returns no wrapped owner/config diagnostic.
			setLLMMagicError(result, common.ErrConflict)
			return
		}
		result.Set(value, nil)
		return
	case llmevents.TopicLLMCommandCompleteProviderDocument:
		m.handleProviderDocument(ev, result)
		return
	case llmevents.TopicLLMCommandGetActiveConfiguration:
		if _, ok := ev.Data().(llmevents.GetActiveConfigurationCommand); !ok {
			setLLMMagicError(result, common.ErrInvalidArgument)
			return
		}
		status, err := m.activeConfigurationStatus()
		if err != nil {
			setLLMMagicError(result, err)
			return
		}
		result.Set(status, nil)
		return
	case llmevents.TopicLLMCommandGetProviderModelCatalog:
		_, ok := ev.Data().(llmevents.GetProviderModelCatalogCommand)
		if !ok {
			setLLMMagicError(result, common.ErrInvalidArgument)
			return
		}
		catalog, err := m.getCatalog(ev.Context(), false)
		if err != nil {
			setLLMMagicError(result, err)
			return
		}
		result.Set(llmevents.GetProviderModelCatalogResult{Catalog: catalog}, nil)
		return
	case llmevents.TopicLLMCommandRefreshProviderModelCatalog:
		_, ok := ev.Data().(llmevents.RefreshProviderModelCatalogCommand)
		if !ok {
			setLLMMagicError(result, common.ErrInvalidArgument)
			return
		}
		catalog, err := m.getCatalog(ev.Context(), true)
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

	execution, err := m.executeCommand(ev)
	status := llmmodels.RuntimeCandidateStatusCompleted
	failureClass := ""
	if err != nil {
		status = llmmodels.RuntimeCandidateStatusFailed
		failureClass = string(runtimeCandidateProviderFailureClass(err, execution.Audit != nil && execution.Audit.ProviderSubmitted))
	}
	if auditErr := m.publishLLMUsageRecorded(ev.Context(), execution.Audit, execution.Usage, status, failureClass); auditErr != nil {
		setLLMMagicError(result, auditErr)
		return
	}
	setLLMMagicResult(result, execution.Result, err)
}

// llmCommandHasExactApplicationSource is the generic LLM command ingress
// authorization gate. Runtime commands have their own exact Orchestrator
// gate; these topics are owned by the Application setup/configuration paths.
// The setup-draft generator is a distinct appservice producer. Persisted
// specification requests must additionally carry the reserved configuration
// revision, which this owner checks again before physical submission.
func llmCommandHasExactApplicationSource(ev magicevent.Event) bool {
	if ev == nil {
		return false
	}
	if ev.ID() == llmevents.TopicLLMCommandCompleteProvider {
		if ev.Source() == llmSetupDraftAppserviceSource {
			return true
		}
		if ev.Source() != llmApplicationOwnerSource {
			return false
		}
		cmd, ok := ev.Data().(llmevents.CompleteProviderCommand)
		return ok && specificationCompletionKindAllowed(cmd.Request.Kind) && cmd.Request.WorkspaceID != "" && cmd.ExpectedConfigurationRevision != ""
	}
	return ev.Source() == llmApplicationOwnerSource
}

func specificationCompletionKindAllowed(kind string) bool {
	switch kind {
	case "workspace/specification/derive", "workspace/specification/proposal":
		return true
	case "workspace/specification/review/dimension_coverage", "workspace/specification/review/negative_path", "workspace/specification/review/regression", "workspace/specification/review/resource_environment", "workspace/specification/review/traceability", "workspace/specification/review/verifier_feasibility":
		return true
	default:
		return false
	}
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
	catalog, err := m.getCatalog(ctx, false)
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

func (m *Biz) executeCommand(ev magicevent.Event) (providerCommandExecution, error) {
	if m == nil {
		return providerCommandExecution{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case llmevents.TopicLLMCommandCompleteProvider:
		cmd, ok := ev.Data().(llmevents.CompleteProviderCommand)
		if !ok {
			return providerCommandExecution{}, common.ErrInvalidArgument
		}
		request, err := cmd.Request.ToModel()
		if err != nil {
			return providerCommandExecution{}, err
		}
		request.ContextBudget = cloneProviderContextBudget(request.ContextBudget)
		attachProviderCachePlan(&request)
		startedAt := time.Now().UTC()
		completion, metadata, err := m.completeProvider(ctx, cmd.Target, request, cmd.ExpectedConfigurationRevision)
		audit := providerCommandAudit(request, metadata, completion.RequestBytes, completion.ResponseBytes, startedAt, time.Now().UTC(), err)
		applyProviderUsageToAudit(audit, completion.Usage)
		execution := providerCommandExecution{Audit: audit, Usage: cloneProviderUsage(completion.Usage)}
		if err == nil {
			response := llmevents.CompleteProviderResult{Target: metadata.Target, Completion: llmCompletionEventData(completion)}
			if err = response.ValidateFor(cmd); err == nil {
				execution.Result = response
			}
		}
		return execution, err
	case llmevents.TopicLLMCommandGenerateEmbedding:
		cmd, ok := ev.Data().(llmevents.GenerateEmbeddingCommand)
		if !ok || strings.TrimSpace(cmd.Input) == "" {
			return providerCommandExecution{}, common.ErrInvalidArgument
		}
		startedAt := time.Now().UTC()
		embedding, metadata, err := m.generateEmbedding(ctx, cmd.Target, cmd.Input)
		audit := providerEmbeddingAudit(cmd, metadata, embedding.RequestBytes, embedding.ResponseBytes, startedAt, time.Now().UTC(), err)
		applyProviderUsageToAudit(audit, embedding.Usage)
		execution := providerCommandExecution{Audit: audit, Usage: cloneProviderUsage(embedding.Usage)}
		if err == nil {
			response := llmevents.GenerateEmbeddingResult{Target: metadata.Target, Embedding: embedding}
			if err = response.ValidateFor(cmd); err == nil {
				execution.Result = response
			}
		}
		return execution, err
	default:
		return providerCommandExecution{}, common.ErrInvalidArgument
	}
}

func cloneProviderUsage(usage *llmaudit.ProviderUsage) *llmaudit.ProviderUsage {
	if usage == nil {
		return nil
	}
	ret := *usage
	return &ret
}

func applyProviderUsageToAudit(audit *llmmodels.ProviderRequestAudit, usage *llmaudit.ProviderUsage) {
	if audit == nil || audit.ContextBudget == nil || usage == nil {
		return
	}
	audit.ContextBudget.ActualTokens = usage.TotalTokens
}

func providerCommandAudit(request llmmodels.ProviderRequest, metadata providerExecutionMetadata, requestBytes, responseBytes int, startedAt, completedAt time.Time, callErr error) *llmmodels.ProviderRequestAudit {
	status, failureClass := providerCommandAuditOutcome(callErr, metadata.Submitted)
	budget := cloneProviderContextBudget(request.ContextBudget)
	if budget == nil {
		budget = providerContextBudgetFromProfile(metadata.Profile)
	} else if usesSectionMessages(request) {
		resolved := providerContextBudgetFromProfile(metadata.Profile)
		if resolved != nil {
			resolved.ActualBytes = budget.ActualBytes
			resolved.CachePlan = budget.CachePlan
			resolved.PromptCaptured = budget.PromptCaptured
			resolved.IncludedSections = budget.IncludedSections
			resolved.NormalizedSections = budget.NormalizedSections
			budget = resolved
		}
	}
	if budget != nil {
		budget.ProviderSubmitted = metadata.Submitted
	}
	stablePrefixHash := ""
	if usesSectionMessages(request) && budget != nil && budget.CachePlan != nil {
		stablePrefixHash = budget.CachePlan.StablePrefixHash
	}
	return &llmmodels.ProviderRequestAudit{
		RequestID:             newProviderCommandRequestID(workorchconfig.LLMOperationResponses, startedAt),
		WorkspaceID:           strings.TrimSpace(request.WorkspaceID),
		RunID:                 strings.TrimSpace(request.RunID),
		StepID:                strings.TrimSpace(request.StepID),
		Target:                metadata.Target,
		Operation:             workorchconfig.LLMOperationResponses,
		StablePrefixHash:      stablePrefixHash,
		CapabilityRevision:    metadata.Profile.CapabilityRevision,
		ConfigurationRevision: metadata.Profile.ConfigurationRevision,
		Kind:                  firstNonEmpty(strings.TrimSpace(request.Kind), "provider_completion"),
		APISurface:            metadata.Profile.APISurface,
		Provider:              metadata.Target.ProviderID,
		PhysicalResponseMode:  canonicalProviderPhysicalResponseMode(metadata),
		ContextBudget:         budget,
		ProviderSubmitted:     metadata.Submitted,
		Status:                status,
		FailureClass:          failureClass,
		StartedAt:             startedAt.Format(time.RFC3339Nano),
		CompletedAt:           completedAt.Format(time.RFC3339Nano),
		DurationMs:            max(int(completedAt.Sub(startedAt).Milliseconds()), 0),
		RequestBytes:          max(requestBytes, 0),
		ResponseBytes:         max(responseBytes, providerResponseBytesFromError(callErr)),
	}
}

func providerEmbeddingAudit(command llmevents.GenerateEmbeddingCommand, metadata providerExecutionMetadata, requestBytes, responseBytes int, startedAt, completedAt time.Time, callErr error) *llmmodels.ProviderRequestAudit {
	status, failureClass := providerCommandAuditOutcome(callErr, metadata.Submitted)
	budget := providerContextBudgetFromProfile(metadata.Profile)
	if budget != nil {
		budget.ProviderSubmitted = metadata.Submitted
		budget.ActualBytes = len(command.Input)
	}
	return &llmmodels.ProviderRequestAudit{
		RequestID:             newProviderCommandRequestID(workorchconfig.LLMOperationEmbeddings, startedAt),
		WorkspaceID:           string(command.WorkspaceID),
		RunID:                 string(command.RunID),
		StepID:                strings.TrimSpace(command.StepID),
		Target:                metadata.Target,
		Operation:             workorchconfig.LLMOperationEmbeddings,
		CapabilityRevision:    metadata.Profile.CapabilityRevision,
		ConfigurationRevision: metadata.Profile.ConfigurationRevision,
		Kind:                  "provider_embedding",
		APISurface:            metadata.Profile.APISurface,
		Provider:              metadata.Target.ProviderID,
		PhysicalResponseMode:  llmaudit.PhysicalResponseModeNonStreaming,
		ContextBudget:         budget,
		ProviderSubmitted:     metadata.Submitted,
		Status:                status,
		FailureClass:          failureClass,
		StartedAt:             startedAt.Format(time.RFC3339Nano),
		CompletedAt:           completedAt.Format(time.RFC3339Nano),
		DurationMs:            max(int(completedAt.Sub(startedAt).Milliseconds()), 0),
		RequestBytes:          max(requestBytes, 0),
		ResponseBytes:         max(responseBytes, providerResponseBytesFromError(callErr)),
	}
}

func providerCommandAuditOutcome(callErr error, submitted bool) (string, string) {
	if callErr == nil {
		return llmmodels.RuntimeCandidateStatusCompleted, ""
	}
	return llmmodels.RuntimeCandidateStatusFailed, string(runtimeCandidateProviderFailureClass(callErr, submitted))
}

func providerContextBudgetFromProfile(profile llmmodels.ResolvedProviderModelProfile) *llmmodels.ProviderContextBudget {
	if profile.Target.ProviderID == "" || profile.Target.ModelID == "" {
		return nil
	}
	mode := "owner_active_configuration"
	maxRequestBytes := profile.MaxRequestBytes
	if profile.Operation == workorchconfig.LLMOperationEmbeddings {
		mode = embeddingTransportPolicyVersion
		maxRequestBytes = embeddingMaxRequestBytes
	}
	return &llmmodels.ProviderContextBudget{
		ProviderID: profile.Target.ProviderID, ModelID: profile.Target.ModelID,
		CapabilityRevision: profile.CapabilityRevision, ConfigurationRevision: profile.ConfigurationRevision,
		ContextWindowTokens: profile.ContextWindowTokens, ProviderMaxOutputTokens: profile.ProviderMaxOutputTokens,
		EffectiveMaxOutputTokens: profile.EffectiveMaxOutputTokens, MaxBytes: profile.MaxRequestBytes,
		MaxTokens: profile.MaxInputTokens, MaxRequestBytes: maxRequestBytes,
		ProfileMode: mode, CapabilitySource: profile.CapabilitySource,
	}
}

func newProviderCommandRequestID(operation string, startedAt time.Time) string {
	return fmt.Sprintf("llm-%s-%d-%d", strings.TrimSpace(operation), startedAt.UnixNano(), providerCommandSequence.Add(1))
}

func llmCompletionEventData(completion llmmodels.ProviderCompletionResult) llmevents.ProviderCompletionData {
	ret := llmevents.ProviderCompletionData{Content: string(completion.Payload)}
	if completion.Usage != nil {
		usage := *completion.Usage
		ret.Usage = &usage
	}
	return ret
}

func setLLMMagicResult(result magicevent.Result, data any, err error) {
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

func (m *Biz) completeProvider(ctx context.Context, target runtimecontract.ProviderModelTarget, request llmmodels.ProviderRequest, expectedConfigurationRevision string) (llmmodels.ProviderCompletionResult, providerExecutionMetadata, error) {
	metadata := providerExecutionMetadata{Target: target.Normalized(), PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming}
	if m == nil {
		return llmmodels.ProviderCompletionResult{}, metadata, common.ErrInvalidArgument
	}
	if expectedConfigurationRevision != "" && expectedConfigurationRevision != m.activeLLMConfigurationRevision {
		return llmmodels.ProviderCompletionResult{}, metadata, fmt.Errorf("%w: provider configuration changed after specification reserve", common.ErrConflict)
	}
	target = metadata.Target
	if target.ProviderID == "" && target.ModelID == "" {
		configured, err := m.activeConfiguration()
		if err != nil {
			return llmmodels.ProviderCompletionResult{}, metadata, err
		}
		target = configured.DefaultChat.ToRuntimeTarget()
		metadata.Target = target
	}
	if err := target.Validate(); err != nil {
		return llmmodels.ProviderCompletionResult{}, metadata, err
	}
	if model := strings.TrimSpace(request.Model); model != "" && model != target.ModelID {
		return llmmodels.ProviderCompletionResult{}, metadata, fmt.Errorf("%w: provider request model %q conflicts with target model %q", common.ErrInvalidArgument, model, target.ModelID)
	}
	profile, err := m.resolveTarget(ctx, target, workorchconfig.LLMOperationResponses)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, metadata, err
	}
	metadata.Profile = profile
	cfg, err := m.activeConfiguration()
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, metadata, err
	}
	runtime, err := ResolveProviderClient(cfg, target, profile)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, metadata, err
	}
	provider := NewProviderFromConfig(runtime)
	if provider == nil {
		return llmmodels.ProviderCompletionResult{}, metadata, fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
	}
	request.Model = target.ModelID
	request.Config = llmproviderconfig.RedactedProviderConfig(runtime)
	var completion llmmodels.ProviderCompletionResult
	if runtime.Capabilities.Streaming {
		streaming, ok := provider.(StreamingProvider)
		if !ok {
			return llmmodels.ProviderCompletionResult{}, metadata, fmt.Errorf("%w: provider does not implement streaming", common.ErrInvalidArgument)
		}
		metadata.PhysicalResponseMode = llmaudit.PhysicalResponseModeStreaming
		completion, err = streaming.StreamComplete(ctx, request, func(llmmodels.ProviderStreamEvent) error { return nil })
	} else {
		metadata.PhysicalResponseMode = llmaudit.PhysicalResponseModeNonStreaming
		completion, err = provider.Complete(ctx, request)
	}
	metadata.Submitted = completion.ProviderSubmitted
	if modeErr := validatePhysicalResponseMode(completion.PhysicalResponseMode, canonicalProviderPhysicalResponseMode(metadata)); modeErr != nil {
		return completion, metadata, modeErr
	}
	metadata.PhysicalResponseMode = completion.PhysicalResponseMode.Normalized()
	if err != nil {
		return completion, metadata, err
	}
	return completion, metadata, nil
}

func (m *Biz) completeProviderWithSealedCeiling(ctx context.Context, ceiling runtimecontract.ProviderCeiling, request llmmodels.ProviderRequest) (llmmodels.ProviderCompletionResult, providerExecutionMetadata, error) {
	metadata := providerExecutionMetadata{PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming}
	if m == nil {
		return llmmodels.ProviderCompletionResult{}, metadata, common.ErrInvalidArgument
	}
	ceiling = ceiling.Normalized()
	metadata.Target = ceiling.Selection.Target
	if err := ceiling.Validate(); err != nil {
		return llmmodels.ProviderCompletionResult{}, metadata, err
	}
	if model := strings.TrimSpace(request.Model); model != "" && model != ceiling.Selection.Target.ModelID {
		return llmmodels.ProviderCompletionResult{}, metadata, fmt.Errorf("%w: provider request model %q conflicts with sealed ceiling model %q", common.ErrInvalidArgument, model, ceiling.Selection.Target.ModelID)
	}
	cfg, err := m.activeConfiguration()
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, metadata, err
	}
	runtime, profile, err := ResolveProviderClientForSealedCeiling(cfg, ceiling)
	if err != nil {
		return llmmodels.ProviderCompletionResult{}, metadata, err
	}
	metadata.Profile = profile
	if request.MaxOutputTokens > 0 && request.MaxOutputTokens < runtime.InferenceProfile.MaxOutputTokens {
		runtime.InferenceProfile.MaxOutputTokens = request.MaxOutputTokens
	}
	provider := NewProviderFromConfig(runtime)
	if provider == nil {
		return llmmodels.ProviderCompletionResult{}, metadata, fmt.Errorf("%w: llm provider client is not configured", common.ErrInvalidArgument)
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
			return llmmodels.ProviderCompletionResult{}, metadata, fmt.Errorf("%w: provider does not implement streaming", common.ErrInvalidArgument)
		}
		metadata.PhysicalResponseMode = llmaudit.PhysicalResponseModeStreaming
		completion, err = streaming.StreamComplete(ctx, request, func(llmmodels.ProviderStreamEvent) error { return nil })
	} else {
		completion, err = provider.Complete(ctx, request)
	}
	metadata.Submitted = completion.ProviderSubmitted
	if modeErr := validatePhysicalResponseMode(completion.PhysicalResponseMode, canonicalProviderPhysicalResponseMode(metadata)); modeErr != nil {
		return completion, metadata, modeErr
	}
	if err != nil {
		return completion, metadata, err
	}
	if !metadata.Submitted {
		return completion, metadata, fmt.Errorf("%w: provider completion did not report physical request submission", common.ErrInvalidArgument)
	}
	request.ContextBudget.ProviderSubmitted = true
	if completion.Usage != nil {
		request.ContextBudget.ActualTokens = completion.Usage.TotalTokens
	}
	return completion, metadata, nil
}

func (m *Biz) generateEmbedding(ctx context.Context, target runtimecontract.ProviderModelTarget, input string) (llmmodels.ProviderEmbeddingResult, providerExecutionMetadata, error) {
	target = target.Normalized()
	metadata := providerExecutionMetadata{Target: target, PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming}
	if err := target.Validate(); err != nil || strings.TrimSpace(input) == "" {
		return llmmodels.ProviderEmbeddingResult{}, metadata, common.ErrInvalidArgument
	}
	if m == nil {
		return llmmodels.ProviderEmbeddingResult{}, metadata, common.ErrInvalidArgument
	}
	profile, err := m.resolveTarget(ctx, target, "embeddings")
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, metadata, err
	}
	metadata.Profile = profile
	cfg, err := m.activeConfiguration()
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, metadata, err
	}
	runtime, err := ResolveProviderClient(cfg, target, profile)
	if err != nil {
		return llmmodels.ProviderEmbeddingResult{}, metadata, err
	}
	result, err := GenerateEmbedding(ctx, target.ProviderID, runtime, input)
	metadata.Submitted = result.ProviderSubmitted
	return result, metadata, err
}

func (m *Biz) resolveTarget(ctx context.Context, target runtimecontract.ProviderModelTarget, operation string) (llmmodels.ResolvedProviderModelProfile, error) {
	cfg, err := m.activeConfiguration()
	if err != nil {
		return llmmodels.ResolvedProviderModelProfile{}, err
	}
	target = target.Normalized()
	var catalog *llmmodels.ProviderModelCatalog
	if target.ProviderID == runtimecontract.BuiltinAIProxyProviderID {
		current, catalogErr := m.getCatalog(ctx, false)
		if catalogErr != nil && current.State == llmmodels.ProviderCatalogStateUnavailable {
			return llmmodels.ResolvedProviderModelProfile{}, catalogErr
		}
		catalog = &current
	}
	return ResolveProviderModelTarget(cfg, catalog, target, operation)
}

func (m *Biz) getCatalog(ctx context.Context, forceRefresh bool) (llmmodels.ProviderModelCatalog, error) {
	if m == nil || m.catalogService == nil {
		return llmmodels.ProviderModelCatalog{}, common.ErrInvalidArgument
	}
	cfg, err := m.activeConfiguration()
	if err != nil {
		return llmmodels.ProviderModelCatalog{}, err
	}
	return m.catalogService.GetCatalog(ctx, cfg, forceRefresh)
}
