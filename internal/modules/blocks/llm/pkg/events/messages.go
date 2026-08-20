package events

import (
	"sort"
	"strings"

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
	Target  runtimecontract.ProviderModelTarget
	Request ProviderRequest
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
	WorkspaceID                 common.WorkspaceID
	RunID                       common.RunID
	DocumentKind                runtimecontract.RuntimeProviderDocumentKind
	RequestIdentity             runtimecontract.RuntimeCandidateRequestIdentity
	ExpectedDocument            *runtimecontract.RuntimeExpectedDocumentContract
	Decision                    runtimecontract.RuntimeDecision
	Transaction                 runtimecontract.AuthorityTransaction
	CandidateAttempt            int
	PreviousFailure             *runtimecontract.RuntimeCandidateFailure
	StablePrefixHash            string
	CandidateManifest           runtimecontract.RuntimeCandidateContractManifest
	DefinitionContext           runtimecontract.RuntimeDefinitionContext
	CandidateContext            runtimecontract.RuntimeCandidateContext
	SemanticManifest            runtimecontract.RuntimeSemanticPlanManifest
	SemanticContext             runtimecontract.RuntimeSemanticPlanRequestContext
	PreviousSemanticRejection   *runtimecontract.RuntimeOriginalDiagnostic
	SemanticCorrectionDirective *runtimecontract.RuntimeSemanticCorrectionDirective
}

type PreflightRuntimeCandidateCommand struct {
	WorkspaceID                 common.WorkspaceID
	RunID                       common.RunID
	DocumentKind                runtimecontract.RuntimeProviderDocumentKind
	Decision                    runtimecontract.RuntimeDecision
	Transaction                 runtimecontract.AuthorityTransaction
	CandidateAttempt            int
	PreviousFailure             *runtimecontract.RuntimeCandidateFailure
	StablePrefixHash            string
	CandidateManifest           runtimecontract.RuntimeCandidateContractManifest
	DefinitionContext           runtimecontract.RuntimeDefinitionContext
	CandidateContext            runtimecontract.RuntimeCandidateContext
	SemanticManifest            runtimecontract.RuntimeSemanticPlanManifest
	SemanticContext             runtimecontract.RuntimeSemanticPlanRequestContext
	PreviousSemanticRejection   *runtimecontract.RuntimeOriginalDiagnostic
	SemanticCorrectionDirective *runtimecontract.RuntimeSemanticCorrectionDirective
}

type PreflightRuntimeCandidateResult struct {
	Transaction  runtimecontract.AuthorityTransaction
	DocumentKind runtimecontract.RuntimeProviderDocumentKind
	Feedback     runtimecontract.RuntimeFeedback
	Audit        *runtimecontract.ProviderRequestAuditView
}

type RequestRuntimeCandidateResult struct {
	Transaction          runtimecontract.AuthorityTransaction
	DocumentKind         runtimecontract.RuntimeProviderDocumentKind
	RequestIdentity      runtimecontract.RuntimeCandidateRequestIdentity
	ExpectedDocumentHash string
	DocumentContent      []byte
	DocumentOutcome      *runtimecontract.RuntimeProviderDocumentOutcome
	Feedback             *runtimecontract.RuntimeFeedback
	Stale                *runtimecontract.RuntimePrerequisiteStale
	Audit                *runtimecontract.ProviderRequestAuditView
	Usage                *llmaudit.ProviderUsage
}

type ProviderCompletionData struct {
	Content string
	Usage   *llmaudit.ProviderUsage
}

type LLMCommandResult struct {
	Target     runtimecontract.ProviderModelTarget
	Completion *ProviderCompletionData
	Embedding  *llmmodel.ProviderEmbeddingResult
}

// LLMUsageRecordedData is the durable request audit notification for one provider call.
// eventstream persists it under TopicLLMUsageRecorded (/run/llm_usage/recorded).
type LLMUsageRecordedData struct {
	RequestID             string
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

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("llm", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicAcquireRuntimeCapabilities),
		eventcontract.CommandTopic[CompleteProviderCommand, LLMCommandResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicLLMCommandCompleteProvider),
		eventcontract.CommandTopic[GenerateEmbeddingCommand, LLMCommandResult]("llm", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicLLMCommandGenerateEmbedding),
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
