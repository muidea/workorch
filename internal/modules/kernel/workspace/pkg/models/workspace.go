package workspace

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type AuthoringSummary struct {
	State         string
	FailurePhase  string
	FailureCode   string
	ReviewCount   int
	InputRevision uint64
}

func (s *AuthoringSummary) Clone() *AuthoringSummary {
	if s == nil {
		return nil
	}
	cloned := *s
	return &cloned
}

type Workspace struct {
	// Revision is the owner-local persistence revision. Projection reducers use
	// it as their CAS fence instead of relying on in-process EventHub ordering.
	Revision             uint64
	ID                   common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	// RunProjectionCursor is the Workspace owner's durable observation fence.
	// It stores only Run-owner fact identity, never decoded Runtime authority or
	// a caller-selected target state.
	RunProjectionCursor WorkspaceRunProjectionCursor
	TemplateID          string
	// TemplateBindingHash identifies the live template materialized into the
	// authoring fields below. It is not a Run seal or an admission-history field.
	TemplateBindingHash string
	ProfileID           string
	DefaultLLMSelection *runtimecontract.ProviderInferenceSelection
	DefaultSoulID       common.SoulID
	SoulBindings        map[string]common.SoulID
	ContextHints        workorcheventpayload.Payload
	EvidencePolicies    []workorcheventpayload.Payload
	// DefinitionAdmissionCandidates is immutable owner history. The current
	// candidate is the highest revision in one CandidateID lineage.
	DefinitionAdmissionCandidates []DefinitionAdmissionCandidate
	// DefinitionSpecificationLedger is Workspace-private pre-seal request history.
	// It neither promotes a Definition nor authorizes Runtime execution.
	DefinitionAuthoringCreation     *DefinitionAuthoringCreation
	DefinitionAuthoring             *DefinitionAuthoringRecord
	DefinitionChangeCandidate       *DefinitionChangeCandidate
	DefinitionAuthoringConfirmation *DefinitionAuthoringConfirmation
	DefinitionAuthoringHistory      []DefinitionAuthoringConfirmedRevision
	DefinitionSpecificationLedger   *DefinitionSpecificationRequestLedger
	StartupInput                    *WorkspaceStartupInputContract
	ActivationAdmission             *WorkspaceActivationAdmission
	// StartupPreparation is separate from admission and Run lifecycle. Reads
	// only project existing attempts; no positive M3 exists in its v1 shape.
	StartupPreparation *WorkspacePreparationRecord
	// Owner-private historical material. Public serialization never exports raw
	// paths/diagnostics; file repository has an explicit restricted wire field.
	PreparationDirectoryEvidence       []WorkspacePreparationDirectoryEvidence       `json:"-"`
	PreparationSealEvidence            []WorkspacePreparationSealEvidence            `json:"-"`
	PreparationResourceEvidence        []WorkspacePreparationResourceEvidence        `json:"-"`
	PreparationManagedConfigEvidence   []WorkspacePreparationManagedConfigEvidence   `json:"-"`
	PreparationSoulEvidence            []WorkspacePreparationSoulEvidence            `json:"-"`
	PreparationSkillRegistryEvidence   []WorkspacePreparationSkillRegistryEvidence   `json:"-"`
	PreparationProviderCacheEvidence   []WorkspacePreparationProviderCacheEvidence   `json:"-"`
	PreparationRunHistoryEvidence      []WorkspacePreparationRunHistoryEvidence      `json:"-"`
	PreparationMemoryInventoryEvidence []WorkspacePreparationMemoryInventoryEvidence `json:"-"`
	PreparationMCPRegistryEvidence     []WorkspacePreparationMCPRegistryEvidence     `json:"-"`
	PreparationResourceInitializations []PreparationResourceInitialization           `json:"-"`
	PreparationSkillHubEffects         []WorkspacePreparationSkillHubEffects         `json:"-"`
	FirstExecutionPreparationEvidence  *WorkspaceFirstExecutionPreparationEvidence   `json:"-"`
	// PreparationFence is owner-private coordination, not execution authority.
	// A nil fence is legacy; ordinary updates must never invent a creation epoch.
	PreparationFence *WorkspacePreparationFence
	// TerminalDelivery is the Workspace owner's persisted release fence. It is
	// written only by the lifecycle reducer and never by generic metadata update.
	TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity
	// AuthoringSummary 是瞬态读取观察，不参与持久化。
	AuthoringSummary *AuthoringSummary `json:"-"`
	// Deletion is the Workspace owner's private retirement/purge aggregate. It
	// is deliberately omitted from public Workspace EventHub DTOs.
	Deletion *WorkspaceDeletionRecord
}

// WorkspaceRunProjectionCursor prevents a delayed/duplicate Run fact from
// rolling the Workspace lifecycle projection backward after CAS retries or a
// fresh-process reconciliation.
type WorkspaceRunProjectionCursor struct {
	WorkspaceRunGeneration uint64
	RunID                  common.RunID
	RunRevision            uint64
	RuntimeStateRevision   uint64
	RuntimeStateRef        string
	CausationID            string
}
