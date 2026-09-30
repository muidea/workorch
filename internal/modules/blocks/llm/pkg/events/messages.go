package events

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CompleteProviderCommand struct {
	Target                        runtimecontract.ProviderModelTarget
	Request                       ProviderRequest
	ExpectedConfigurationRevision string
}

type TraceReference struct {
	Name string
	Ref  string
}

type ProviderRequest struct {
	WorkspaceID     string
	RunID           string
	StepID          string
	Kind            string
	Model           string
	ReasoningEffort string
	SystemLocale    string
	DialogLanguage  string
	Schema          string
	MaxOutputTokens int
	RawTextOutput   bool
	TraceRefs       []TraceReference
	Config          llmmodel.RedactedProviderConfig
	PromptContext   *llmmodel.PromptContext
	ContextBudget   *llmmodel.ProviderContextBudget
}

func ProviderRequestFromModel(input llmmodel.ProviderRequest) ProviderRequest {
	refs := make([]TraceReference, 0, len(input.TraceRefs))
	for name, ref := range input.TraceRefs {
		refs = append(refs, TraceReference{Name: name, Ref: ref})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	return ProviderRequest{
		WorkspaceID: input.WorkspaceID, RunID: input.RunID, StepID: input.StepID, Kind: input.Kind,
		Model: input.Model, ReasoningEffort: input.ReasoningEffort, SystemLocale: input.SystemLocale, DialogLanguage: input.DialogLanguage,
		Schema: input.Schema, MaxOutputTokens: input.MaxOutputTokens, RawTextOutput: input.RawTextOutput, TraceRefs: refs, Config: input.Config,
		PromptContext: input.PromptContext, ContextBudget: input.ContextBudget,
	}
}

func (r ProviderRequest) ToModel() (llmmodel.ProviderRequest, error) {
	refs := make(map[string]string, len(r.TraceRefs))
	for _, entry := range r.TraceRefs {
		name := strings.TrimSpace(entry.Name)
		if name == "" || strings.TrimSpace(entry.Ref) == "" {
			return llmmodel.ProviderRequest{}, common.ErrInvalidArgument
		}
		if _, ok := refs[name]; ok {
			return llmmodel.ProviderRequest{}, common.ErrInvalidArgument
		}
		refs[name] = entry.Ref
	}
	return llmmodel.ProviderRequest{
		WorkspaceID: r.WorkspaceID, RunID: r.RunID, StepID: r.StepID, Kind: r.Kind,
		Model: r.Model, ReasoningEffort: r.ReasoningEffort, SystemLocale: r.SystemLocale, DialogLanguage: r.DialogLanguage,
		Schema: r.Schema, MaxOutputTokens: r.MaxOutputTokens, RawTextOutput: r.RawTextOutput, TraceRefs: refs, Config: r.Config,
		PromptContext: r.PromptContext, ContextBudget: r.ContextBudget,
	}, nil
}

type GenerateEmbeddingCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      string
	Target      runtimecontract.ProviderModelTarget
	Input       string
}

// GetActiveConfigurationCommand queries the LLM owner's immutable process
// configuration snapshot. Persisted configuration belongs to the application
// configuration writer and is intentionally not read by this owner command.
type GetActiveConfigurationCommand struct{}

type GetActiveConfigurationResult struct {
	ProviderID            string
	ConfigurationRevision string
	DefaultChat           runtimecontract.ProviderModelTarget
	DefaultEmbedding      *runtimecontract.ProviderModelTarget
}

type GetProviderModelCatalogCommand struct{}

type GetProviderModelCatalogResult struct {
	Catalog llmmodel.ProviderModelCatalog
}

type RefreshProviderModelCatalogCommand struct{}

type RefreshProviderModelCatalogResult struct {
	Catalog llmmodel.ProviderModelCatalog
}

type ValidateProviderInferenceSelectionCommand struct {
	Selection runtimecontract.ProviderInferenceSelection
	Operation string
}

type ValidateProviderInferenceSelectionResult struct {
	Selection          runtimecontract.ProviderInferenceSelection
	CapabilityRevision string
}

type AcquireProviderCeilingCommand struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	PrerequisiteID      string
	Selection           runtimecontract.ProviderInferenceSelection
	MinimumBytes        int
}

type AcquireProviderCeilingResult struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	Ceiling             runtimecontract.ProviderCeiling
	Observation         runtimecontract.RuntimePrerequisiteObservation
}

type RequestRuntimeCandidateCommand struct {
	WorkspaceID                    common.WorkspaceID
	RunID                          common.RunID
	DocumentKind                   runtimecontract.RuntimeProviderDocumentKind
	RequestIdentity                runtimecontract.RuntimeCandidateRequestIdentity
	ExpectedDocument               *runtimecontract.RuntimeExpectedDocumentContract
	Decision                       runtimecontract.RuntimeDecision
	Transaction                    runtimecontract.AuthorityTransaction
	CandidateAttempt               int
	CandidateCorrectionObservation *runtimecontract.RuntimeCandidateCorrectionObservation
	CandidateCorrection            *llmmodel.RuntimeCandidateCorrectionInput
	StablePrefixHash               string
	CandidateManifest              runtimecontract.RuntimeCandidateContractManifest
	DefinitionContext              runtimecontract.RuntimeDefinitionContext
	CandidateContext               runtimecontract.RuntimeCandidateContext
	SemanticManifest               runtimecontract.RuntimeSemanticPlanManifest
	SemanticContext                runtimecontract.RuntimeSemanticPlanRequestContext
	SemanticCorrectionObservation  *runtimecontract.RuntimeSemanticCorrectionObservation
	SemanticCorrectionDirective    *runtimecontract.RuntimeSemanticCorrectionDirective
}

type PreflightRuntimeCandidateCommand struct {
	WorkspaceID                    common.WorkspaceID
	RunID                          common.RunID
	DocumentKind                   runtimecontract.RuntimeProviderDocumentKind
	Decision                       runtimecontract.RuntimeDecision
	Transaction                    runtimecontract.AuthorityTransaction
	CandidateAttempt               int
	CandidateCorrectionObservation *runtimecontract.RuntimeCandidateCorrectionObservation
	CandidateCorrection            *llmmodel.RuntimeCandidateCorrectionInput
	StablePrefixHash               string
	CandidateManifest              runtimecontract.RuntimeCandidateContractManifest
	DefinitionContext              runtimecontract.RuntimeDefinitionContext
	CandidateContext               runtimecontract.RuntimeCandidateContext
	SemanticManifest               runtimecontract.RuntimeSemanticPlanManifest
	SemanticContext                runtimecontract.RuntimeSemanticPlanRequestContext
	SemanticCorrectionObservation  *runtimecontract.RuntimeSemanticCorrectionObservation
	SemanticCorrectionDirective    *runtimecontract.RuntimeSemanticCorrectionDirective
}

type PreflightRuntimeCandidateResult struct {
	Transaction  runtimecontract.AuthorityTransaction
	DocumentKind runtimecontract.RuntimeProviderDocumentKind
	Feedback     runtimecontract.RuntimeFeedback
	Audit        *runtimecontract.ProviderRequestAuditView
}

// RuntimeProviderDocumentOutcome is the LLM owner's authoritative factual
// outcome for one expected provider document. Content is present exactly when
// framing reports a complete document and is excluded from JSON/audit
// projection so raw provider bodies cannot leak into durable event payloads.
type RuntimeProviderDocumentOutcome struct {
	Framing runtimecontract.RuntimeProviderDocumentFraming `json:"framing"`
	Content []byte                                         `json:"-"`
}

func (o RuntimeProviderDocumentOutcome) Normalized() RuntimeProviderDocumentOutcome {
	o.Framing = o.Framing.Normalized()
	o.Content = append([]byte(nil), o.Content...)
	return o
}

func (o RuntimeProviderDocumentOutcome) Validate() error {
	framing := o.Framing.Normalized()
	if framing != o.Framing {
		return fmt.Errorf("invalid LLM provider document outcome: framing is not canonical")
	}
	if err := runtimecontract.ValidateRuntimeProviderDocumentFramingContent(framing, o.Content); err != nil {
		return fmt.Errorf("invalid LLM provider document outcome: %w", err)
	}
	return nil
}

type RequestRuntimeCandidateResult struct {
	WorkspaceID          common.WorkspaceID `json:"workspaceId"`
	RunID                common.RunID       `json:"runId"`
	Transaction          runtimecontract.AuthorityTransaction
	DocumentKind         runtimecontract.RuntimeProviderDocumentKind
	RequestIdentity      runtimecontract.RuntimeCandidateRequestIdentity
	ExpectedDocumentHash string
	DocumentOutcome      *RuntimeProviderDocumentOutcome
	Feedback             *runtimecontract.RuntimeFeedback
	Stale                *runtimecontract.RuntimePrerequisiteStale
	Audit                *runtimecontract.ProviderRequestAuditView
	Usage                *llmaudit.ProviderUsage
}

// Validate enforces the exhaustive EventHub result union and its exact physical
// request audit. Every result variant carries one workspace/run and
// logical/physical request identity. Audit submission is an adapter-owned fact,
// never an inference from request bytes or the selected response mode.
func (r RequestRuntimeCandidateResult) Validate() error {
	if r.WorkspaceID == "" || r.RunID == "" || string(r.WorkspaceID) != strings.TrimSpace(string(r.WorkspaceID)) || string(r.RunID) != strings.TrimSpace(string(r.RunID)) {
		return fmt.Errorf("LLM runtime candidate result requires workspace and run scope")
	}
	if err := r.RequestIdentity.Validate(); err != nil {
		return fmt.Errorf("LLM runtime candidate result request identity: %w", err)
	}
	if r.RequestIdentity != r.RequestIdentity.Normalized() {
		return fmt.Errorf("LLM runtime candidate result request identity is not canonical")
	}
	if strings.TrimSpace(r.ExpectedDocumentHash) == "" || r.ExpectedDocumentHash != strings.TrimSpace(r.ExpectedDocumentHash) {
		return fmt.Errorf("LLM runtime candidate result requires expected document identity")
	}
	switch r.DocumentKind {
	case runtimecontract.RuntimeProviderDocumentOperations, runtimecontract.RuntimeProviderDocumentSemanticPlan:
	default:
		return fmt.Errorf("LLM runtime candidate result requires explicit document kind")
	}
	if err := validateRuntimeCandidateResultAudit(r); err != nil {
		return err
	}

	variants := 0
	if r.Stale != nil {
		variants++
		stale := r.Stale.Normalized()
		if err := stale.Validate(); err != nil {
			return fmt.Errorf("invalid LLM prerequisite stale: %w", err)
		}
	}
	if r.Feedback != nil {
		variants++
		if err := ValidateRuntimeProviderRequestFeedback(*r.Feedback); err != nil {
			return err
		}
	}
	if r.DocumentOutcome != nil {
		variants++
		if err := r.DocumentOutcome.Validate(); err != nil {
			return err
		}
		outcome := r.DocumentOutcome.Normalized()
		if expected := strings.TrimSpace(r.ExpectedDocumentHash); expected == "" || expected != outcome.Framing.ExpectedDocumentHash {
			return fmt.Errorf("LLM provider document expected hash mismatch")
		}
	}
	if variants != 1 {
		return fmt.Errorf("LLM runtime candidate result requires exactly one of stale, feedback, or document outcome")
	}
	if r.Stale != nil {
		if r.Audit.ProviderSubmitted || r.Usage != nil || strings.TrimSpace(r.Audit.Status) != "failed" ||
			strings.TrimSpace(r.Audit.FailureClass) != string(runtimecontract.RuntimeCandidateFailureClassProviderConfiguration) {
			return fmt.Errorf("LLM prerequisite stale requires an unsubmitted failed audit without usage")
		}
	}
	if r.Feedback != nil {
		if r.Feedback.WorkspaceID != r.WorkspaceID || r.Feedback.RunID != r.RunID || strings.TrimSpace(r.Audit.Status) != "failed" {
			return fmt.Errorf("LLM provider feedback scope or audit status mismatch")
		}
		if err := validateRuntimeCandidateFeedbackAudit(*r.Feedback, *r.Audit); err != nil {
			return err
		}
	}
	if r.DocumentOutcome != nil {
		if !r.Audit.ProviderSubmitted {
			return fmt.Errorf("LLM provider document outcome requires a submitted physical request")
		}
		if strings.TrimSpace(r.Audit.Status) != llmmodel.RuntimeCandidateStatusCompleted || strings.TrimSpace(r.Audit.FailureClass) != "" {
			return fmt.Errorf("LLM provider document outcome requires a completed audit without failure class")
		}
	}
	return nil
}

// validateRuntimeCandidateFeedbackAudit keeps the owner result exhaustive
// without turning factual failure categories into recovery policy. A
// provider-transient fact can only exist after client.Do was invoked; allowing
// it on an unsubmitted result would let a zero-transport outcome consume the
// caller's transport retry budget indefinitely. Configuration and owner-side
// protocol guards may occur before dispatch; authentication is only produced
// from a provider response. Every unsubmitted variant is still constrained by
// the common no-physical-facts rules in validateRuntimeCandidateResultAudit.
func validateRuntimeCandidateFeedbackAudit(feedback runtimecontract.RuntimeFeedback, audit runtimecontract.ProviderRequestAuditView) error {
	if feedback.OriginalDiagnostic == nil {
		return fmt.Errorf("LLM provider feedback requires original diagnostic")
	}
	wantClass := strings.TrimSpace(feedback.OriginalDiagnostic.OriginalClass)
	if strings.TrimSpace(audit.FailureClass) != wantClass {
		return fmt.Errorf("LLM provider feedback failure class does not match request audit")
	}
	switch feedback.StableCategory {
	case runtimecontract.RuntimeFeedbackCategoryProviderTransient:
		if !audit.ProviderSubmitted {
			return fmt.Errorf("LLM provider transient feedback requires a submitted physical request")
		}
	case runtimecontract.RuntimeFeedbackCategoryProviderAuthentication:
		if !audit.ProviderSubmitted {
			return fmt.Errorf("LLM provider authentication feedback requires a submitted physical request")
		}
	}
	return nil
}

func validateRuntimeCandidateResultAudit(result RequestRuntimeCandidateResult) error {
	audit := result.Audit
	if audit == nil {
		return fmt.Errorf("LLM runtime candidate result requires exact provider request audit")
	}
	identity := result.RequestIdentity
	if !canonicalNonEmptyAuditString(audit.RequestID) || !canonicalNonEmptyAuditString(audit.LogicalRequestID) || !canonicalNonEmptyAuditString(audit.PhysicalCallID) ||
		audit.RequestID != identity.PhysicalCallID || audit.LogicalRequestID != identity.LogicalRequestID || audit.PhysicalCallID != identity.PhysicalCallID {
		return fmt.Errorf("LLM provider request audit identity mismatch")
	}
	if !canonicalNonEmptyAuditString(audit.WorkspaceID) || !canonicalNonEmptyAuditString(audit.RunID) || audit.WorkspaceID != string(result.WorkspaceID) || audit.RunID != string(result.RunID) {
		return fmt.Errorf("LLM provider request audit scope mismatch")
	}
	if !canonicalNonEmptyAuditString(audit.Operation) || !canonicalNonEmptyAuditString(audit.StablePrefixHash) {
		return fmt.Errorf("LLM provider request audit requires operation and stable prefix identity")
	}
	wantKind := "runtime_candidate"
	if result.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan {
		wantKind = "runtime_semantic_plan"
	}
	if audit.Kind != wantKind {
		return fmt.Errorf("LLM provider request audit document kind mismatch")
	}
	if err := audit.Target.Validate(); err != nil {
		return fmt.Errorf("LLM provider request audit target: %w", err)
	}
	if audit.Target != audit.Target.Normalized() {
		return fmt.Errorf("LLM provider request audit target is not canonical")
	}
	if err := audit.PhysicalResponseMode.Validate(); err != nil {
		return fmt.Errorf("LLM provider request audit physical response mode: %w", err)
	}
	if audit.PhysicalResponseMode != audit.PhysicalResponseMode.Normalized() {
		return fmt.Errorf("LLM provider request audit physical response mode is not canonical")
	}
	if audit.DurationMs < 0 || audit.RequestBytes < 0 || audit.ResponseBytes < 0 {
		return fmt.Errorf("LLM provider request audit has negative measurement")
	}
	status := strings.TrimSpace(audit.Status)
	if status != audit.Status || strings.TrimSpace(audit.FailureClass) != audit.FailureClass {
		return fmt.Errorf("LLM provider request audit status or failure class is not canonical")
	}
	if status != "completed" && status != "failed" {
		return fmt.Errorf("LLM provider request audit has invalid status %q", status)
	}
	if status == "completed" && strings.TrimSpace(audit.FailureClass) != "" {
		return fmt.Errorf("completed LLM provider request audit carried failure class")
	}
	if status == "failed" && strings.TrimSpace(audit.FailureClass) == "" && result.Stale == nil {
		return fmt.Errorf("failed LLM provider request audit requires failure class")
	}
	if audit.ContextBudget.Empty() || audit.ContextBudget.ProviderSubmitted != audit.ProviderSubmitted {
		return fmt.Errorf("LLM provider request audit context submission mismatch")
	}
	if audit.ContextBudget.ProviderID != audit.Target.ProviderID || audit.ContextBudget.ModelID != audit.Target.ModelID {
		return fmt.Errorf("LLM provider request audit target/context mismatch")
	}
	if audit.ContextBudget.CapabilityRevision != audit.CapabilityRevision || audit.ContextBudget.ConfigurationRevision != audit.ConfigurationRevision {
		return fmt.Errorf("LLM provider request audit revision/context mismatch")
	}
	if hash := audit.ContextBudget.StablePrefixHash; hash != "" && hash != audit.StablePrefixHash {
		return fmt.Errorf("LLM provider request audit stable prefix/context mismatch")
	}
	contextMeasurements := []int{
		audit.ContextBudget.ContextWindowTokens, audit.ContextBudget.ProviderMaxOutputTokens,
		audit.ContextBudget.EffectiveMaxOutputTokens, audit.ContextBudget.MaxBytes,
		audit.ContextBudget.ActualBytes, audit.ContextBudget.MaxTokens,
		audit.ContextBudget.EstimatedInputTokens, audit.ContextBudget.EstimatedOutputTokens,
		audit.ContextBudget.ActualTokens, audit.ContextBudget.MaxRequestBytes,
	}
	for _, value := range contextMeasurements {
		if value < 0 {
			return fmt.Errorf("LLM provider request audit context has negative measurement")
		}
	}
	if !audit.ProviderSubmitted {
		if audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming || audit.ResponseBytes != 0 || result.Usage != nil || audit.ContextBudget.ActualTokens != 0 {
			return fmt.Errorf("unsubmitted LLM provider request audit carried physical response facts")
		}
		return nil
	}
	if audit.RequestBytes <= 0 {
		return fmt.Errorf("submitted LLM provider request audit requires serialized request bytes")
	}
	if err := validateRuntimeCandidateResultUsage(result.Usage, audit.ContextBudget); err != nil {
		return err
	}
	return nil
}

func canonicalNonEmptyAuditString(value string) bool {
	return value != "" && value == strings.TrimSpace(value)
}

func validateRuntimeCandidateResultUsage(usage *llmaudit.ProviderUsage, budget llmaudit.ContextBudget) error {
	if usage == nil {
		if budget.ActualTokens != 0 {
			return fmt.Errorf("LLM provider request audit has actual tokens without typed usage")
		}
		return nil
	}
	values := []int{
		usage.InputTokens, usage.OutputTokens, usage.TotalTokens,
		usage.CacheReadTokens, usage.CacheWriteTokens, usage.CacheCreationTokens,
		usage.CacheableInputTokens, usage.NonCacheableInputTokens,
	}
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("LLM provider usage has negative token count")
		}
	}
	if usage.TotalTokens <= 0 || usage.TotalTokens < usage.InputTokens || usage.TotalTokens < usage.OutputTokens || budget.ActualTokens != usage.TotalTokens {
		return fmt.Errorf("LLM provider usage does not match request audit")
	}
	return nil
}

// ValidateRuntimeProviderRequestFeedback validates the factual provider
// feedback vocabulary owned by the LLM block. It deliberately does not
// validate a caller's requested workspace/run/authority scope; consumers must
// compare that scope with their sealed command before accepting the fact.
func ValidateRuntimeProviderRequestFeedback(feedback runtimecontract.RuntimeFeedback) error {
	if err := feedback.Validate(); err != nil {
		return fmt.Errorf("invalid LLM provider request feedback: %w", err)
	}
	if feedback.Domain != runtimecontract.RuntimeFeedbackDomainProviderRequest {
		return fmt.Errorf("LLM provider request feedback has domain %q", feedback.Domain)
	}
	if feedback.DispatchID != "" {
		return fmt.Errorf("LLM provider request feedback must not carry a dispatch identity")
	}
	if feedback.OriginalDiagnostic == nil || feedback.OriginalDiagnostic.OwnerModuleID != runtimecontract.RuntimeModuleLLM {
		return fmt.Errorf("LLM provider request feedback is missing its owner diagnostic")
	}
	class := runtimecontract.RuntimeCandidateFailureClass(feedback.OriginalDiagnostic.OriginalClass)
	switch feedback.StableCategory {
	case runtimecontract.RuntimeFeedbackCategoryProviderAuthentication:
		if class != runtimecontract.RuntimeCandidateFailureClassProviderAuthentication {
			return fmt.Errorf("LLM provider authentication feedback class mismatch")
		}
	case runtimecontract.RuntimeFeedbackCategoryProviderConfiguration:
		if class != runtimecontract.RuntimeCandidateFailureClassProviderConfiguration {
			return fmt.Errorf("LLM provider configuration feedback class mismatch")
		}
	case runtimecontract.RuntimeFeedbackCategoryProviderTransient:
		if class != runtimecontract.RuntimeCandidateFailureClassProviderTransient {
			return fmt.Errorf("LLM provider transient feedback class mismatch")
		}
	case runtimecontract.RuntimeFeedbackCategoryProviderProtocol:
		switch class {
		case runtimecontract.RuntimeCandidateFailureClassProviderProtocol,
			runtimecontract.RuntimeCandidateFailureClassProvider,
			runtimecontract.RuntimeCandidateFailureClassSchema,
			runtimecontract.RuntimeCandidateFailureClassContract,
			runtimecontract.RuntimeCandidateFailureClassInternal:
		default:
			return fmt.Errorf("LLM provider protocol feedback class mismatch")
		}
	default:
		return fmt.Errorf("unknown LLM provider request feedback category %q", feedback.StableCategory)
	}
	return nil
}

type ProviderCompletionData struct {
	Content string
	Usage   *llmaudit.ProviderUsage
}

// CompleteProviderResult is the only successful result for
// TopicLLMCommandCompleteProvider. Completion is a value rather than a nullable
// union arm, so a successful EventHub response cannot be confused with an
// embedding response or an empty generic envelope.
type CompleteProviderResult struct {
	Target     runtimecontract.ProviderModelTarget
	Completion ProviderCompletionData
}

func (r CompleteProviderResult) ValidateFor(command CompleteProviderCommand) error {
	target := r.Target.Normalized()
	if err := target.Validate(); err != nil {
		return fmt.Errorf("%w: completion result target: %v", common.ErrInvalidArgument, err)
	}
	if r.Target != target {
		return fmt.Errorf("%w: completion result target is not canonical", common.ErrInvalidArgument)
	}
	requested := command.Target.Normalized()
	if requested.ProviderID != "" || requested.ModelID != "" {
		if err := requested.Validate(); err != nil || !target.Same(requested) {
			return fmt.Errorf("%w: completion result target does not match command", common.ErrInvalidArgument)
		}
	}
	if model := strings.TrimSpace(command.Request.Model); model != "" && target.ModelID != model {
		return fmt.Errorf("%w: completion result model does not match request", common.ErrInvalidArgument)
	}
	if strings.TrimSpace(r.Completion.Content) == "" || !utf8.ValidString(r.Completion.Content) {
		return fmt.Errorf("%w: completion result requires non-empty UTF-8 content", common.ErrInvalidArgument)
	}
	if err := validateGenericProviderUsage(r.Completion.Usage); err != nil {
		return err
	}
	return nil
}

// GenerateEmbeddingResult is the only successful result for
// TopicLLMCommandGenerateEmbedding. Embedding is a value rather than a nullable
// union arm, so the topic contract is exhaustive by construction.
type GenerateEmbeddingResult struct {
	Target    runtimecontract.ProviderModelTarget
	Embedding llmmodel.ProviderEmbeddingResult
}

func (r GenerateEmbeddingResult) ValidateFor(command GenerateEmbeddingCommand) error {
	target := r.Target.Normalized()
	requested := command.Target.Normalized()
	if err := target.Validate(); err != nil {
		return fmt.Errorf("%w: embedding result target: %v", common.ErrInvalidArgument, err)
	}
	if r.Target != target {
		return fmt.Errorf("%w: embedding result target is not canonical", common.ErrInvalidArgument)
	}
	if err := requested.Validate(); err != nil || !target.Same(requested) {
		return fmt.Errorf("%w: embedding result target does not match command", common.ErrInvalidArgument)
	}
	input := strings.TrimSpace(command.Input)
	if input == "" || r.Embedding.ProviderID != target.ProviderID || r.Embedding.Model != target.ModelID || r.Embedding.Input != input {
		return fmt.Errorf("%w: embedding result provider, model, or input binding mismatch", common.ErrInvalidArgument)
	}
	if len(r.Embedding.Vector) == 0 {
		return fmt.Errorf("%w: embedding result vector is empty", common.ErrInvalidArgument)
	}
	for _, value := range r.Embedding.Vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%w: embedding result vector contains a non-finite value", common.ErrInvalidArgument)
		}
	}
	if r.Embedding.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming ||
		!r.Embedding.ProviderSubmitted || r.Embedding.RequestBytes <= 0 || r.Embedding.ResponseBytes <= 0 {
		return fmt.Errorf("%w: embedding result is missing its physical response fact", common.ErrInvalidArgument)
	}
	if err := validateGenericProviderUsage(r.Embedding.Usage); err != nil {
		return err
	}
	return nil
}

func validateGenericProviderUsage(usage *llmaudit.ProviderUsage) error {
	if usage == nil {
		return nil
	}
	values := []int{
		usage.InputTokens, usage.OutputTokens, usage.TotalTokens, usage.CacheReadTokens, usage.CacheWriteTokens,
		usage.CacheCreationTokens, usage.CacheableInputTokens, usage.NonCacheableInputTokens,
	}
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("%w: provider usage contains a negative token count", common.ErrInvalidArgument)
		}
	}
	if usage.TotalTokens <= 0 || usage.TotalTokens < usage.InputTokens || usage.TotalTokens < usage.OutputTokens {
		return fmt.Errorf("%w: provider usage total is inconsistent", common.ErrInvalidArgument)
	}
	return nil
}

// LLMUsageRecordedData is the durable request audit notification for one provider call.
// eventstream persists it under TopicLLMUsageRecorded (/llm/usage/recorded).
type LLMUsageRecordedData struct {
	RequestID             string
	LogicalRequestID      string
	PhysicalCallID        string
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	StepID                string
	Target                runtimecontract.ProviderModelTarget
	Operation             string
	CapabilityRevision    string
	ConfigurationRevision string
	StablePrefixHash      string
	Kind                  string
	APISurface            string
	Provider              string
	PhysicalResponseMode  llmaudit.PhysicalResponseMode
	ProviderSubmitted     bool
	Status                string
	FailureClass          string
	StartedAt             string
	CompletedAt           string
	DurationMs            int
	RequestBytes          int
	ResponseBytes         int
	InputTokens           int
	OutputTokens          int
	TotalTokens           int
	Estimated             bool
	UsageSource           string
	ContextBudget         llmaudit.ContextBudget
	CacheUsage            llmaudit.CacheUsage
	OccurredAt            string
}

func (d LLMUsageRecordedData) Validate() error {
	if !canonicalNonEmptyAuditString(d.RequestID) || !canonicalNonEmptyAuditString(d.Status) {
		return fmt.Errorf("LLM usage audit requires request identity and status")
	}
	if d.Status != "completed" && d.Status != "failed" {
		return fmt.Errorf("LLM usage audit has invalid status %q", d.Status)
	}
	if d.Kind != strings.TrimSpace(d.Kind) {
		return fmt.Errorf("LLM usage audit kind is not canonical")
	}
	if string(d.WorkspaceID) != strings.TrimSpace(string(d.WorkspaceID)) || string(d.RunID) != strings.TrimSpace(string(d.RunID)) || (d.RunID != "" && d.WorkspaceID == "") {
		return fmt.Errorf("LLM usage audit run requires workspace scope")
	}
	if err := d.PhysicalResponseMode.Validate(); err != nil {
		return fmt.Errorf("LLM usage audit: %w", err)
	}
	if d.PhysicalResponseMode != d.PhysicalResponseMode.Normalized() {
		return fmt.Errorf("LLM usage audit physical response mode is not canonical")
	}
	if err := validateLLMUsageRequestIdentity(d); err != nil {
		return err
	}
	measurements := []int{d.DurationMs, d.RequestBytes, d.ResponseBytes, d.InputTokens, d.OutputTokens, d.TotalTokens}
	for _, value := range measurements {
		if value < 0 {
			return fmt.Errorf("LLM usage audit has negative measurement")
		}
	}
	cacheMeasurements := []int{
		d.CacheUsage.CacheReadTokens, d.CacheUsage.CacheWriteTokens, d.CacheUsage.CacheCreationTokens,
		d.CacheUsage.CacheableInputTokens, d.CacheUsage.NonCacheableInputTokens,
	}
	for _, value := range cacheMeasurements {
		if value < 0 {
			return fmt.Errorf("LLM usage audit has negative cache token measurement")
		}
	}
	if d.TotalTokens < d.InputTokens || d.TotalTokens < d.OutputTokens {
		return fmt.Errorf("LLM usage audit token totals are inconsistent")
	}
	if !d.ContextBudget.Empty() {
		if d.ContextBudget.ProviderSubmitted != d.ProviderSubmitted {
			return fmt.Errorf("LLM usage audit context submission mismatch")
		}
		if err := validateLLMContextBudgetMeasurements(d.ContextBudget); err != nil {
			return err
		}
		if d.ContextBudget.ActualTokens != d.TotalTokens {
			return fmt.Errorf("LLM usage audit token total does not match context audit")
		}
	}
	if !d.ProviderSubmitted {
		if d.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming || d.ResponseBytes != 0 || d.InputTokens != 0 || d.OutputTokens != 0 || d.TotalTokens != 0 ||
			d.ContextBudget.ActualTokens != 0 || !llmCacheUsageHasNoTokens(d.CacheUsage) {
			return fmt.Errorf("unsubmitted LLM usage audit carried physical response facts")
		}
		return nil
	}
	if d.RequestBytes <= 0 {
		return fmt.Errorf("submitted LLM usage audit requires serialized request bytes")
	}
	return nil
}

func validateLLMUsageRequestIdentity(data LLMUsageRecordedData) error {
	logicalID := strings.TrimSpace(data.LogicalRequestID)
	physicalID := strings.TrimSpace(data.PhysicalCallID)
	if logicalID != data.LogicalRequestID || physicalID != data.PhysicalCallID {
		return fmt.Errorf("LLM usage audit request identity is not canonical")
	}
	runtimeRequest := data.Kind == "runtime_candidate" || data.Kind == "runtime_semantic_plan"
	if runtimeRequest && (logicalID == "" || physicalID == "") {
		return fmt.Errorf("Runtime LLM usage audit requires logical and physical request identity")
	}
	if (logicalID == "") != (physicalID == "") {
		return fmt.Errorf("LLM usage audit requires complete logical/physical request identity")
	}
	if physicalID != "" && data.RequestID != physicalID {
		return fmt.Errorf("LLM usage audit correlation request ID must equal physical call ID")
	}
	return nil
}

func validateLLMContextBudgetMeasurements(budget llmaudit.ContextBudget) error {
	measurements := []int{
		budget.ContextWindowTokens, budget.ProviderMaxOutputTokens, budget.EffectiveMaxOutputTokens,
		budget.MaxBytes, budget.ActualBytes, budget.MaxTokens, budget.EstimatedInputTokens,
		budget.EstimatedOutputTokens, budget.ActualTokens, budget.MaxRequestBytes,
	}
	for _, value := range measurements {
		if value < 0 {
			return fmt.Errorf("LLM usage audit context has negative measurement")
		}
	}
	return nil
}

func llmCacheUsageHasNoTokens(usage llmaudit.CacheUsage) bool {
	return usage.CacheReadTokens == 0 && usage.CacheWriteTokens == 0 && usage.CacheCreationTokens == 0 &&
		usage.CacheableInputTokens == 0 && usage.NonCacheableInputTokens == 0
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("llm", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities),
		eventcontract.CommandTopic[CompleteProviderCommand, CompleteProviderResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicLLMCommandCompleteProvider),
		eventcontract.CommandTopic[GenerateEmbeddingCommand, GenerateEmbeddingResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicLLMCommandGenerateEmbedding),
		eventcontract.CommandTopic[GetActiveConfigurationCommand, GetActiveConfigurationResult]("llm", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicLLMCommandGetActiveConfiguration),
		eventcontract.CommandTopic[GetProviderModelCatalogCommand, GetProviderModelCatalogResult]("llm", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicLLMCommandGetProviderModelCatalog),
		eventcontract.CommandTopic[RefreshProviderModelCatalogCommand, RefreshProviderModelCatalogResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicLLMCommandRefreshProviderModelCatalog),
		eventcontract.CommandTopic[ValidateProviderInferenceSelectionCommand, ValidateProviderInferenceSelectionResult]("llm", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicLLMCommandValidateInferenceSelection),
		eventcontract.CommandTopic[AcquireProviderCeilingCommand, AcquireProviderCeilingResult]("llm", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireProviderCeiling),
		eventcontract.CommandTopic[PreflightRuntimeCandidateCommand, PreflightRuntimeCandidateResult]("llm", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicPreflightRuntimeCandidate),
		eventcontract.CommandTopic[RequestRuntimeCandidateCommand, RequestRuntimeCandidateResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicRequestRuntimeCandidate),
		eventcontract.DataTopic[LLMUsageRecordedData]("llm", definitionPackage, eventcontract.LanePolicyState, TopicLLMUsageRecorded),
	)
}
