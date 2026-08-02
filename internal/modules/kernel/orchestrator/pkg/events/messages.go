package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
const CommandDestination = modulecommon.ModuleID

type PurgeRunCommand = orchestratorcontracts.PurgeRunCommand

type AcquireTargetSnapshotCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	WorkspaceDeliverableDirectory string
	TargetRef                     string
	MaxBytes                      int
}

type AcquireTargetSnapshotResult struct {
	PrerequisiteID string
	Snapshot       runtimecontract.TargetSnapshot
	Observation    *runtimecontract.RuntimePrerequisiteObservation
}

// AcquireRuntimeTargetSelectionCommand asks the File owner for a bounded
// target-selection observation. TargetRole is opaque to the owner; the owner
// may only apply its declared neutral selection rule to the workspace
// inventory and must return selected, create, scope, absent, or ambiguous facts.
type AcquireRuntimeTargetSelectionCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	WorkspaceDeliverableDirectory string
	TargetRole                    string
	TargetSelectorRef             string
	TargetSelectorOwnerRef        string
	TargetSelectorSubjectRef      string
	TargetSelectorCandidateRefs   []string
	ProspectiveTargetRef          string
	MaxItems                      int
}

type AcquireRuntimeTargetSelectionResult struct {
	PrerequisiteID string
	Selection      runtimecontract.RuntimeTargetSelection
	Observation    *runtimecontract.RuntimePrerequisiteObservation
}

type AcquireWorkspaceManifestCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	WorkspaceDeliverableDirectory string
}

type AcquireWorkspaceManifestResult struct {
	PrerequisiteID string
	Manifest       runtimecontract.WorkspaceManifest
	Observation    *runtimecontract.RuntimePrerequisiteObservation
}

// ConfirmRuntimeRepairSurfaceCommand asks the File owner to verify a bounded
// Shell validation-failure observation against the current workspace.  It is
// read-only and contains no Runtime action or aggregate mutation.
type ConfirmRuntimeRepairSurfaceCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	WorkspaceDeliverableDirectory string
	Observation                   runtimecontract.RuntimeValidationFailureObservation
}

type ConfirmRuntimeRepairSurfaceResult struct {
	Confirmation runtimecontract.RuntimeRepairSurfaceConfirmation
}

type AcquireProviderCeilingCommand struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	PrerequisiteID      string
	Target              runtimecontract.ProviderModelTarget
	MinimumBytes        int
}

type AcquireProviderCeilingResult struct {
	PrerequisiteID string
	Ceiling        runtimecontract.ProviderCeiling
	Observation    *runtimecontract.RuntimePrerequisiteObservation
}

type AcquireRuntimeCapabilitiesCommand struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	PrerequisiteID      string
	ModuleID            string
	CapabilityClass     runtimecontract.RuntimeCapabilityClass
	RequiredKinds       []string
}

type AcquireRuntimeCapabilitiesResult struct {
	PrerequisiteID string
	Capabilities   runtimecontract.RuntimeCapabilities
	Observation    *runtimecontract.RuntimePrerequisiteObservation
}

// RuntimeCapabilityUnsupportedError is a typed prerequisite failure returned
// by the addressed owner when a sealed owner/class/kind triple has no
// responder. It is not a candidate rejection and must never be converted into
// a generic governance ticket by a caller.
type RuntimeCapabilityUnsupportedError struct {
	ModuleID        string
	CapabilityClass runtimecontract.RuntimeCapabilityClass
	RequiredKinds   []string
	Reason          string
}

func (e *RuntimeCapabilityUnsupportedError) Error() string {
	if e == nil {
		return "runtime capability unavailable"
	}
	moduleID := strings.TrimSpace(e.ModuleID)
	reason := strings.TrimSpace(e.Reason)
	kinds := append([]string(nil), e.RequiredKinds...)
	for idx := range kinds {
		kinds[idx] = strings.TrimSpace(kinds[idx])
	}
	sort.Strings(kinds)
	if reason == "" {
		reason = "unsupported"
	}
	return fmt.Sprintf("runtime capability %s for owner %s is %s: %s", e.CapabilityClass, moduleID, reason, strings.Join(kinds, ","))
}

// AcquireRuntimeOperationAdmissibilityCommand requests a bounded, read-only
// parameter inventory from one canonical operation owner before candidate
// generation. The owner must not allocate resources, mutate a deliverable, or
// write a runtime receipt while handling this command.
type AcquireRuntimeOperationAdmissibilityCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	OwnerModuleID                 string
	WorkspaceDeliverableDirectory string
	RequiredKinds                 []string
	RequiredTargetRefs            []string
	// ValidationRecipes are activation-sealed declarations submitted only to
	// the Shell owner for its read-only managed-validation preflight. The
	// owner returns the executable subset in its admissibility snapshot; they
	// are not candidate-supplied command parameters.
	ValidationRecipes []runtimecontract.RuntimeValidationRecipe
	// CapabilityStateBinding requests an inventory derived from this exact
	// owner capability acquisition. Empty means the owner has no such binding.
	CapabilityStateBinding string
	MaxItems               int
}

type AcquireRuntimeOperationAdmissibilityResult struct {
	PrerequisiteID string
	Admissibility  runtimecontract.RuntimeOperationAdmissibility
	Observation    *runtimecontract.RuntimePrerequisiteObservation
}

// ProbeRuntimeOperationFeasibilityCommand is an SM-3 admission check for one
// schema-valid operation. It is intentionally separate from execute commands:
// Feasible=false cannot create a dispatch, receipt, evidence, or side effect.
//
// N5: the owner receives the sealed operation authorization identity from the
// candidate manifest plus the original owner snapshot identity. Owners only
// report feasible | stale | infeasible; they do not choose correction, action,
// or terminal.
type ProbeRuntimeOperationFeasibilityCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	Transaction                   runtimecontract.AuthorityTransaction
	WorkspaceDeliverableDirectory string
	Operation                     runtimecontract.RuntimeOperation
	Admissibility                 runtimecontract.RuntimeOperationAdmissibility
	// CapabilityStateBinding must equal Admissibility.AcquisitionBinding when
	// the inventory was derived from a capability-state acquisition.
	CapabilityStateBinding string
	// CandidateManifestHash is the sealed candidate contract hash for this
	// authority transaction. Owners validate identity only; they never rebuild
	// the manifest.
	CandidateManifestHash string
	// OperationAuthorization is the sealed pure authorization for this
	// operation kind from the candidate manifest. Owners verify kind/owner/
	// target-policy identity against it and must not re-derive policy from
	// focus/profile text.
	OperationAuthorization runtimecontract.RuntimeCandidateOperationManifest
	// Observation is the persisted freshness metadata for Admissibility. It is
	// supplied by Orchestrator so an owner can return an exact typed stale
	// detail without reconstructing authority from local process state.
	Observation *runtimecontract.RuntimePrerequisiteObservation
	// CurrentTargetSnapshot is the persisted owner snapshot for this operation's
	// concrete target, when available. It distinguishes a missing repair input
	// from a candidate that ignored already supplied current content.
	CurrentTargetSnapshot *runtimecontract.TargetSnapshot
}

// ProbeRuntimeOperationFeasibilityResult is the unified outer observation for
// every capability owner. Exactly one of Feasible / Stale / Failure (infeasible)
// is active after normalize. PrerequisiteRequest is an optional SM-2 reacquire
// hint that still implies non-feasible.
type ProbeRuntimeOperationFeasibilityResult struct {
	OperationID             string
	AdmissibilitySnapshotID string
	CapabilityStateBinding  string
	CandidateManifestHash   string
	Feasible                bool
	Category                runtimecontract.RuntimeOwnerFeasibilityCategory
	Feedback                *runtimecontract.RuntimeFeedback
	ReplayProof             *runtimecontract.RuntimeOperationReplayProof
	Failure                 *runtimecontract.FailureFact
	Stale                   *runtimecontract.RuntimePrerequisiteStale
	PrerequisiteRequest     *runtimecontract.RuntimeOperationPrerequisiteRequest
}

// ProbeRuntimeSemanticProofFeasibilityCommand asks exactly one canonical
// capability owner to judge one provider-authored semantic proof proposal
// against an already acquired owner catalog. It is read-only and cannot create
// a candidate dispatch, receipt, coverage credit, retry, or terminal outcome.
type ProbeRuntimeSemanticProofFeasibilityCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	FeasibilityBatchID            string
	WorkspaceDeliverableDirectory string
	Manifest                      runtimecontract.RuntimeSemanticPlanManifest
	Proposal                      runtimecontract.RuntimeSemanticProofProposal
	OwnerCatalog                  runtimecontract.RuntimeOperationAdmissibility
	CatalogObservation            *runtimecontract.RuntimePrerequisiteObservation
}

// ProbeRuntimeSemanticProofFeasibilityResult contains only the owner-authored
// factual observation. Orchestrator validates all echoed identities before an
// SM-2 private event may consume it.
type ProbeRuntimeSemanticProofFeasibilityResult struct {
	FeasibilityBatchID string
	Observation        runtimecontract.RuntimeSemanticProofFeasibility
}

type AcquireGitCapabilityStateCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	AuthoritySnapshotID           string
	PrerequisiteID                string
	WorkspaceDeliverableDirectory string
	CapabilityStateKind           string
	SubjectRef                    string
	AcquisitionKey                string
}

type AcquireGitCapabilityStateResult struct {
	PrerequisiteID string
	Snapshot       runtimecontract.RuntimeCapabilityStateSnapshot
	Observation    *runtimecontract.RuntimePrerequisiteObservation
}

// QueryRuntimeCWIReconciliationCommand asks one capability owner for a
// binding-grounded, read-only observation of the current workspace. The
// command carries only the activation-sealed proof surface; it contains no
// target state and cannot mutate Runtime coverage or lifecycle.
type QueryRuntimeCWIReconciliationCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	RequestID                     string
	Generation                    uint64
	DefinitionRevision            string
	BindingSetHash                string
	OwnerModuleID                 string
	Binding                       runtimecontract.RuntimeCWIReconciliationBindingRef
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	WorkspaceBaselineRevision     string
	WorkspaceBaselineRef          string
	ValidationRecipes             []runtimecontract.RuntimeValidationRecipe
}

// QueryRuntimeCWIReconciliationResult is an owner-authored factual result.
// OwnerReceiptRef is an opaque query/inspection receipt identity; it is not a
// coverage credit or a lifecycle command.
type QueryRuntimeCWIReconciliationResult struct {
	Observation     runtimecontract.RuntimeCWIReconciliationObservation
	OwnerReceiptRef string
}

func (c QueryRuntimeCWIReconciliationCommand) ValidateForOwner(ownerModuleID string) error {
	if c.WorkspaceID == "" || c.RunID == "" || strings.TrimSpace(c.RequestID) == "" || c.Generation == 0 ||
		strings.TrimSpace(c.DefinitionRevision) == "" || strings.TrimSpace(c.BindingSetHash) == "" ||
		strings.TrimSpace(c.OwnerModuleID) == "" || strings.TrimSpace(ownerModuleID) != c.OwnerModuleID ||
		strings.TrimSpace(c.WorkspaceDeliverableDirectory) == "" || strings.TrimSpace(c.WorkspaceBaselineRevision) == "" ||
		strings.TrimSpace(c.WorkspaceBaselineRef) == "" {
		return common.ErrInvalidArgument
	}
	if err := c.Binding.Validate(); err != nil {
		return err
	}
	if !runtimeCWIStringIncluded(c.Binding.OwnerModuleIDs, c.OwnerModuleID) {
		return fmt.Errorf("reconciliation binding %q does not authorize owner %q", c.Binding.BindingID, c.OwnerModuleID)
	}
	return nil
}

func runtimeCWIStringIncluded(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

type RequestRuntimeCandidateCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	// DocumentKind is mandatory and selects one of the two mutually exclusive
	// provider documents carried by this single request pipeline.
	DocumentKind runtimecontract.RuntimeProviderDocumentKind
	// LogicalRequestID and RequestSequence identify a persisted Orchestrator
	// request session. PhysicalCallID identifies one provider submission within
	// that session. All three are mandatory for every physical provider request.
	LogicalRequestID  string
	RequestSequence   int
	PhysicalCallID    string
	Decision          runtimecontract.RuntimeDecision
	Transaction       runtimecontract.AuthorityTransaction
	CandidateAttempt  int
	PreviousFailure   *runtimecontract.RuntimeCandidateFailure
	StablePrefixHash  string
	CandidateManifest runtimecontract.RuntimeCandidateContractManifest
	DefinitionContext runtimecontract.RuntimeDefinitionContext
	CandidateContext  runtimecontract.RuntimeCandidateContext
	SemanticManifest  runtimecontract.RuntimeSemanticPlanManifest
	SemanticContext   runtimecontract.RuntimeSemanticPlanRequestContext
	// PreviousSemanticRejection is the provider-safe attempt-0 diagnostic used
	// only by semantic_plan attempt 1. The original diagnostic remains in the
	// Orchestrator aggregate and never becomes retry authority in this command.
	PreviousSemanticRejection *runtimecontract.RuntimeOriginalDiagnostic
	// OutputContinuation is absent for the initial structured-candidate
	// request.  When present it requests the next raw suffix for a persisted,
	// bounded candidate-output continuation.
	OutputContinuation *runtimecontract.RuntimeCandidateOutputContinuationRequest
}

// PreflightRuntimeCandidateCommand asks the LLM owner to render and inspect a
// sealed candidate request without calling a provider. It exists so context
// integrity failures are classified before SM-3 allocates a correction attempt.
type PreflightRuntimeCandidateCommand struct {
	WorkspaceID               common.WorkspaceID
	RunID                     common.RunID
	DocumentKind              runtimecontract.RuntimeProviderDocumentKind
	Decision                  runtimecontract.RuntimeDecision
	Transaction               runtimecontract.AuthorityTransaction
	CandidateAttempt          int
	PreviousFailure           *runtimecontract.RuntimeCandidateFailure
	StablePrefixHash          string
	CandidateManifest         runtimecontract.RuntimeCandidateContractManifest
	DefinitionContext         runtimecontract.RuntimeDefinitionContext
	CandidateContext          runtimecontract.RuntimeCandidateContext
	SemanticManifest          runtimecontract.RuntimeSemanticPlanManifest
	SemanticContext           runtimecontract.RuntimeSemanticPlanRequestContext
	PreviousSemanticRejection *runtimecontract.RuntimeOriginalDiagnostic
}

// PreflightRuntimeCandidateResult contains only the secret-free rendering
// outcome. It cannot carry a candidate, provider usage or a mutable authority.
type PreflightRuntimeCandidateResult struct {
	Transaction  runtimecontract.AuthorityTransaction
	DocumentKind runtimecontract.RuntimeProviderDocumentKind
	Ready        bool
	Reason       string
	Audit        *runtimecontract.ProviderRequestAuditView
}

type RequestRuntimeCandidateResult struct {
	Transaction      runtimecontract.AuthorityTransaction
	DocumentKind     runtimecontract.RuntimeProviderDocumentKind
	LogicalRequestID string
	RequestSequence  int
	PhysicalCallID   string
	// DocumentContent is an in-process handoff for a complete provider document.
	// It is never an audit/projection payload and must be persisted only by the
	// Orchestrator-owned request content store.
	DocumentContent []byte
	Failure         *runtimecontract.RuntimeCandidateFailure
	// Stale is an owner-confirmed pre-candidate freshness outcome. It is
	// mutually exclusive with DocumentContent and Failure; Orchestrator invalidates
	// and reacquires without spending candidate or provider-recovery budget.
	Stale *runtimecontract.RuntimePrerequisiteStale
	// RequestID correlates this result with the persisted LLM usage audit record.
	RequestID string
	// Status is completed | failed when a provider request audit is available.
	Status string
	// Audit is the sealed provider request audit (target/revision/budget/submitted).
	// Present for both successful and failed provider submissions when generation ran.
	Audit *runtimecontract.ProviderRequestAuditView
	// Usage is provider-reported token usage when available.
	Usage *runtimecontract.ProviderUsageView
	// OutputFragment is mutually exclusive with DocumentContent, Failure and Stale.
	// The Orchestrator persists it in its own artifact repository before making
	// another suffix request or assembling the candidate document.
	OutputFragment *runtimecontract.RuntimeCandidateOutputFragment
}

type RuntimeDriveDisposition string

const (
	RuntimeDriveDispositionGovernedWait RuntimeDriveDisposition = "governed_wait"
	RuntimeDriveDispositionTerminal     RuntimeDriveDisposition = "terminal"
)

type DriveRuntimeCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	// ReconciliationRequestID/Generation are set only for an accepted Run
	// continuation.  They are persisted by the Orchestrator before ordinary
	// candidate driving, so a resume always re-observes the current workspace.
	// Zero values retain the activation-sealed first-generation behavior.
	ReconciliationRequestID  string
	ReconciliationGeneration uint64
}

type DriveRuntimeResult struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	Disposition         RuntimeDriveDisposition
	StateRevision       uint64
	EvidenceRevision    string
	DefinitionRevision  string
	AuthoritySnapshotID string
	DispatchID          string
	State               runtimecontract.ConvergenceState
	BlockingTicketID    common.TicketID
	Terminal            *runtimecontract.RuntimeTerminalDecision
}

type RuntimeOperationExecutionEnvelope struct {
	ExecutionKey        string
	Fence               uint64
	ExpectedEffectScope runtimecontract.RuntimeEffectScope
	ReplayProof         *runtimecontract.RuntimeOperationReplayProof
	GovernanceGrant     *runtimecontract.RuntimeOperationGovernanceGrant
}

type QueryRuntimeOperationReceiptCommand struct {
	WorkspaceID     common.WorkspaceID
	RunID           common.RunID
	DispatchID      string
	OperationID     string
	ExecutionKey    string
	Fence           uint64
	GovernanceGrant *runtimecontract.RuntimeOperationGovernanceGrant
}

type QueryRuntimeOperationReceiptResult struct {
	Found   bool
	Receipt *runtimecontract.RuntimeOperationReceipt
}

type CommitRuntimeOperationReceiptCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	DispatchID  string
	Fence       uint64
	Receipt     runtimecontract.RuntimeOperationReceipt
}

type CommitRuntimeOperationReceiptResult struct {
	Receipt       runtimecontract.RuntimeOperationReceipt
	StateRevision uint64
}

type RuntimeGovernanceResolutionDisposition string

const (
	RuntimeGovernanceResolutionDispositionCommitted RuntimeGovernanceResolutionDisposition = "committed"
	RuntimeGovernanceResolutionDispositionStale     RuntimeGovernanceResolutionDisposition = "stale_diagnostic"
	RuntimeGovernanceResolutionDispositionDuplicate RuntimeGovernanceResolutionDisposition = "duplicate"
)

type CommitRuntimeGovernanceResolutionCommand struct {
	Resolution runtimecontract.RuntimeGovernanceResolution
}

type CommitRuntimeGovernanceTicketResolutionCommand struct {
	ResolutionID common.ResolutionID
	TicketID     common.TicketID
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	Outcome      runtimecontract.RuntimeGovernanceResolutionOutcome
	Summary      string
	EvidenceRefs []string
}

type CommitRuntimeGovernanceResolutionResult struct {
	ResolutionID        common.ResolutionID
	Disposition         RuntimeGovernanceResolutionDisposition
	StateRevision       uint64
	EvidenceRevision    string
	AuthoritySnapshotID string
	State               runtimecontract.ConvergenceState
	Terminal            *runtimecontract.RuntimeTerminalDecision
}

type OpenRuntimeGovernanceRequestCommand struct {
	TicketID               common.TicketID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	DispatchID             string
	Transaction            runtimecontract.AuthorityTransaction
	Dependency             runtimecontract.ExternalDependency
	ContinueResolutionType string
	FailResolutionType     string
}

type OpenRuntimeGovernanceRequestResult struct {
	TicketID            common.TicketID
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	DispatchID          string
	AuthoritySnapshotID string
}

type FileListOperationData struct {
	Path       string
	Recursive  bool
	MaxEntries int64
}

type FileReadOperationData struct {
	Path      string
	Optional  bool
	MaxBytes  int64
	StartLine int64
	EndLine   int64
}

type FileListEntry struct {
	Path        string
	Kind        string
	Size        int64
	ModifiedAt  string
	ContentHash string
	Binary      bool
	Ignored     bool
}

type FileEditOperationData struct {
	Path       string
	OldText    string
	NewText    string
	BaseHash   string
	Occurrence int64
}

type FileWriteOperationData struct {
	TargetRef      string
	Content        string
	ContentPresent bool
}

type PatchApplyOperationData struct {
	TargetRef       string
	PatchType       string
	Content         string
	ContentPresent  bool
	Diff            string
	OldText         string
	NewText         string
	NewTextPresent  bool
	BaseHash        string
	Occurrence      int64
	CoverageTargets []string
	Summary         string
}

type ShellOperationData struct {
	// ValidationRecipeID is present only for validation/managed_session. It is
	// selection identity, not a free-form command: Shell resolves command/args
	// from the activation-sealed recipe after its owner preflight.
	ValidationRecipeID              string
	Command                         string
	Args                            []string
	TimeoutMillis                   int64
	StateChanging                   bool
	TargetRef                       string
	TargetRefs                      []string
	VerificationScope               string
	BoundedReason                   string
	ValidationScratchDirectory      string
	ValidationScratchAutoCleanup    bool
	ValidationScratchAutoCleanupSet bool
	ValidationEvidence              runtimecontract.RuntimeValidationEvidence
	ManagedValidation               runtimecontract.RuntimeManagedValidation
}

type GitOperationData struct {
	Operation     string
	TimeoutMillis int64
	Pathspecs     []string
	Ref           string
	BaseRef       string
	Limit         int64
	Message       string
	ExpectedHead  string
}

type MCPArgument struct {
	Name       string
	String     *string
	Integer    *int64
	Boolean    *bool
	StringList []string
}

type MCPOperationData struct {
	ServerID  string
	Tool      string
	URI       string
	Prompt    string
	Input     []MCPArgument
	Arguments []MCPArgument
	Summary   string
	Stream    bool
}

type SSHOperationData struct {
	Host           string
	Port           int64
	Username       string
	Password       string
	PrivateKey     string
	PrivateKeyPath string
	Command        string
	TimeoutMillis  int64
}

type WebOperationData struct {
	ProviderID     string
	Query          string
	Locale         string
	Recency        string
	Domains        []string
	ExcludeDomains []string
	MaxResults     int64
	SafeSearch     bool
	ApprovalID     common.ApprovalID
	URL            string
	MaxBytes       int64
	ExtractMode    string
}

type ExecuteFileListOperationCommand struct {
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	OperationID                   string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Data                          FileListOperationData
}

type ExecuteFileReadOperationCommand struct {
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	OperationID                   string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Data                          FileReadOperationData
}

type ExecuteFileEditOperationCommand struct {
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	OperationID                   string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Data                          FileEditOperationData
}

type ExecuteFileWriteOperationCommand struct {
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	OperationID                   string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Data                          FileWriteOperationData
}

type ExecutePatchApplyOperationCommand struct {
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	OperationID                   string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Data                          PatchApplyOperationData
}

type ExecutePatchPreviewOperationCommand struct {
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	OperationID                   string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Data                          PatchApplyOperationData
}

type ExecuteShellOperationCommand struct {
	Execution           RuntimeOperationExecutionEnvelope
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	DispatchID          string
	Transaction         runtimecontract.AuthorityTransaction
	OperationID         string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	TargetRefs          []string
	EvidenceKind        string
	TargetRole          string
	EvidencePolicies    []runtimecontract.RuntimeEvidencePolicy
	// ValidationRecipes is the exact Shell-owner-confirmed recipe set from the
	// sealed admissibility snapshot. Managed validation execution must resolve
	// recipeId exclusively from this value, never from activation policy text.
	ValidationRecipes             []runtimecontract.RuntimeValidationRecipe
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Data                          ShellOperationData
}

type ExecuteGitOperationCommand struct {
	Execution                          RuntimeOperationExecutionEnvelope
	WorkspaceID                        common.WorkspaceID
	RunID                              common.RunID
	DispatchID                         string
	Transaction                        runtimecontract.AuthorityTransaction
	OperationID                        string
	OperationKind                      string
	RequiredOperationRef               string
	CapabilityStates                   []runtimecontract.RuntimeCapabilityStateSnapshot
	CompletedRequiredOperationReceipts []runtimecontract.RuntimeRequiredOperationReceiptBinding
	CoverageWorkItemIDs                []common.CoverageWorkItemID
	TargetRefs                         []string
	EvidenceKind                       string
	EvidencePolicies                   []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory      string
	WorkspaceInternalDirectory         string
	Data                               GitOperationData
}

type ExecuteMCPOperationCommand struct {
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	OperationID                   string
	OperationKind                 string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	TargetRole                    string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Data                          MCPOperationData
}

type ExecuteSSHOperationCommand struct {
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	OperationID                   string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Data                          SSHOperationData
}

type ExecuteWebOperationCommand struct {
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	OperationID                   string
	OperationKind                 string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	Data                          WebOperationData
}

type ExecuteFileEditOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

type ExecuteFileWriteOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

type ExecuteFileListOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

type ExecuteFileReadOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

type ExecutePatchApplyOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

type ExecutePatchPreviewOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

type ExecuteShellOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

type ExecuteGitOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

type ExecuteMCPOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

type ExecuteSSHOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

type ExecuteWebOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	SourceRef    string
}

func ResolveRuntimeCapabilities(command AcquireRuntimeCapabilitiesCommand, moduleID string, supportedKinds ...string) (AcquireRuntimeCapabilitiesResult, error) {
	moduleID = strings.TrimSpace(moduleID)
	if command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || strings.TrimSpace(command.ModuleID) != moduleID || len(command.RequiredKinds) == 0 {
		return AcquireRuntimeCapabilitiesResult{}, fmt.Errorf("invalid runtime capability prerequisite command")
	}
	supported := make(map[string]struct{}, len(supportedKinds))
	for _, kind := range supportedKinds {
		kind = strings.TrimSpace(kind)
		if kind != "" {
			supported[kind] = struct{}{}
		}
	}
	resolved := make([]string, 0, len(command.RequiredKinds))
	seen := map[string]struct{}{}
	for _, kind := range command.RequiredKinds {
		kind = strings.TrimSpace(kind)
		if _, ok := supported[kind]; !ok {
			return AcquireRuntimeCapabilitiesResult{}, &RuntimeCapabilityUnsupportedError{
				ModuleID: moduleID, CapabilityClass: command.CapabilityClass, RequiredKinds: []string{kind}, Reason: "unsupported_kind",
			}
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		resolved = append(resolved, kind)
	}
	capabilities := runtimecontract.RuntimeCapabilities{}
	switch command.CapabilityClass {
	case runtimecontract.RuntimeCapabilityClassStateChange:
		capabilities.StateChangeKinds = resolved
	case runtimecontract.RuntimeCapabilityClassValidation:
		capabilities.ValidationKinds = resolved
	case runtimecontract.RuntimeCapabilityClassGovernance:
		capabilities.GovernanceKinds = resolved
	default:
		return AcquireRuntimeCapabilitiesResult{}, fmt.Errorf("invalid runtime capability class %q", command.CapabilityClass)
	}
	observation := runtimeCapabilityObservation(command, moduleID, resolved)
	return AcquireRuntimeCapabilitiesResult{PrerequisiteID: command.PrerequisiteID, Capabilities: capabilities, Observation: &observation}, nil
}

// runtimeCapabilityObservation is emitted by the capability owner along with
// its typed inventory. It records only an owner-neutral catalog revision;
// Orchestrator persists and invalidates it but does not probe or invent owner
// capabilities itself.
func runtimeCapabilityObservation(command AcquireRuntimeCapabilitiesCommand, moduleID string, kinds []string) runtimecontract.RuntimePrerequisiteObservation {
	kinds = append([]string(nil), kinds...)
	sort.Strings(kinds)
	material := strings.Join([]string{moduleID, string(command.CapabilityClass), strings.Join(kinds, ",")}, "|")
	digest := sha256.Sum256([]byte(material))
	revision := "capability-catalog-" + hex.EncodeToString(digest[:])[:20]
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: command.PrerequisiteID,
		Kind:           runtimecontract.PrerequisiteKindCapability,
		OwnerModuleID:  moduleID,
		ContentHash:    revision,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: moduleID,
			SubjectRef: "runtime_capabilities", Revision: revision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

type ListRunEvidenceCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type ListToolCallsByRunCommand = ListRunEvidenceCommand
type ListPatchesByRunCommand = ListRunEvidenceCommand
type ListValidationResultsByRunCommand = ListRunEvidenceCommand
type ListCoverageWorkItemsByRunCommand = ListRunEvidenceCommand
type ListArtifactContinuationsByRunCommand = ListRunEvidenceCommand

type PatchChangedData struct {
	PatchID      common.PatchID
	SubjectID    string
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	StepID       common.StepID
	ToolCallID   common.ToolCallID
	PatchType    string
	TargetRef    string
	Summary      string
	State        common.PatchState
	AddedLines   int
	DeletedLines int
	ContentHash  string
	DiffHash     string
	ApprovalID   common.ApprovalID
	Risk         string
	Error        string
}

type ArtifactContinuationChangedData struct {
	ContinuationID    common.ArtifactContinuationID
	WorkspaceID       common.WorkspaceID
	RunID             common.RunID
	StepID            common.StepID
	ArtifactID        string
	TargetRef         string
	State             common.ArtifactContinuationState
	ExpectedSequence  int
	FinalReceived     bool
	BytesWritten      int64
	RollingHash       string
	LastChunkHash     string
	ReceivedSequences []int
	CoverageTargets   []string
	EvidenceRefs      []string
	FailureReason     string
}

type OrchestratorCommandResult struct {
	ToolCalls             []*runmodel.ToolCall
	Patches               []*runmodel.Patch
	ValidationResults     []*runmodel.ValidationResult
	CoverageWorkItems     []*runtimecontract.CoverageWorkItem
	ArtifactContinuations []*runtimecontract.ArtifactContinuation
	DeletedCount          int
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[PurgeRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandPurgeRun).MessageContract(),
		eventcontract.CommandTopic[ListToolCallsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListToolCallsByRun).MessageContract(),
		eventcontract.CommandTopic[ListPatchesByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListPatchesByRun).MessageContract(),
		eventcontract.CommandTopic[ListValidationResultsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListValidationResultsByRun).MessageContract(),
		eventcontract.CommandTopic[ListCoverageWorkItemsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListCoverageWorkItemsByRun).MessageContract(),
		eventcontract.CommandTopic[ListArtifactContinuationsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListArtifactContinuationsByRun).MessageContract(),
		eventcontract.CommandTopic[AcquireTargetSnapshotCommand, AcquireTargetSnapshotResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireTargetSnapshot).MessageContract(),
		eventcontract.CommandTopic[AcquireRuntimeTargetSelectionCommand, AcquireRuntimeTargetSelectionResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireRuntimeTargetSelection).MessageContract(),
		eventcontract.CommandTopic[AcquireWorkspaceManifestCommand, AcquireWorkspaceManifestResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireWorkspaceManifest).MessageContract(),
		eventcontract.CommandTopic[ConfirmRuntimeRepairSurfaceCommand, ConfirmRuntimeRepairSurfaceResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorConfirmRuntimeRepairSurface).MessageContract(),
		eventcontract.CommandTopic[AcquireProviderCeilingCommand, AcquireProviderCeilingResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireProviderCeiling).MessageContract(),
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireRuntimeCapabilities).MessageContract(),
		eventcontract.CommandTopic[AcquireRuntimeOperationAdmissibilityCommand, AcquireRuntimeOperationAdmissibilityResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireRuntimeOperationAdmissibility).MessageContract(),
		eventcontract.CommandTopic[ProbeRuntimeOperationFeasibilityCommand, ProbeRuntimeOperationFeasibilityResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorProbeRuntimeOperationFeasibility).MessageContract(),
		eventcontract.CommandTopic[ProbeRuntimeSemanticProofFeasibilityCommand, ProbeRuntimeSemanticProofFeasibilityResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorProbeRuntimeSemanticProofFeasibility).MessageContract(),
		eventcontract.CommandTopic[AcquireGitCapabilityStateCommand, AcquireGitCapabilityStateResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireGitCapabilityState).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeCWIReconciliationCommand, QueryRuntimeCWIReconciliationResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorQueryRuntimeCWIReconciliation).MessageContract(),
		eventcontract.CommandTopic[DriveRuntimeCommand, DriveRuntimeResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorDriveRuntime).MessageContract(),
		eventcontract.CommandTopic[PreflightRuntimeCandidateCommand, PreflightRuntimeCandidateResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorPreflightRuntimeCandidate).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeOperationReceiptCommand, QueryRuntimeOperationReceiptResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorQueryRuntimeOperationReceipt).MessageContract(),
		eventcontract.CommandTopic[CommitRuntimeOperationReceiptCommand, CommitRuntimeOperationReceiptResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommitRuntimeOperationReceipt).MessageContract(),
		eventcontract.CommandTopic[RequestRuntimeTerminationCommand, RequestRuntimeTerminationResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorRequestRuntimeTermination).MessageContract(),
		eventcontract.CommandTopic[RequestRuntimeCandidateCommand, RequestRuntimeCandidateResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorRequestRuntimeCandidate).MessageContract(),
		eventcontract.CommandTopic[ExecuteFileListOperationCommand, ExecuteFileListOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteFileListOperation).MessageContract(),
		eventcontract.CommandTopic[ExecuteFileReadOperationCommand, ExecuteFileReadOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteFileReadOperation).MessageContract(),
		eventcontract.CommandTopic[ExecuteFileEditOperationCommand, ExecuteFileEditOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteFileEditOperation).MessageContract(),
		eventcontract.CommandTopic[ExecuteFileWriteOperationCommand, ExecuteFileWriteOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteFileWriteOperation).MessageContract(),
		eventcontract.CommandTopic[ExecutePatchApplyOperationCommand, ExecutePatchApplyOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecutePatchApplyOperation).MessageContract(),
		eventcontract.CommandTopic[ExecutePatchPreviewOperationCommand, ExecutePatchPreviewOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecutePatchPreviewOperation).MessageContract(),
		eventcontract.CommandTopic[ExecuteShellOperationCommand, ExecuteShellOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteShellOperation).MessageContract(),
		eventcontract.CommandTopic[ExecuteGitOperationCommand, ExecuteGitOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteGitOperation).MessageContract(),
		eventcontract.CommandTopic[ExecuteMCPOperationCommand, ExecuteMCPOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteMCPOperation).MessageContract(),
		eventcontract.CommandTopic[ExecuteSSHOperationCommand, ExecuteSSHOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteSSHOperation).MessageContract(),
		eventcontract.CommandTopic[ExecuteWebOperationCommand, ExecuteWebOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteWebOperation).MessageContract(),
		eventcontract.CommandTopic[CommitRuntimeGovernanceResolutionCommand, CommitRuntimeGovernanceResolutionResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommitRuntimeGovernanceResolution).MessageContract(),
		eventcontract.CommandTopic[CommitRuntimeGovernanceTicketResolutionCommand, CommitRuntimeGovernanceResolutionResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommitRuntimeGovernanceTicket).MessageContract(),
		eventcontract.CommandTopic[OpenRuntimeGovernanceRequestCommand, OpenRuntimeGovernanceRequestResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorOpenRuntimeGovernanceRequest).MessageContract(),
	}
	for _, topic := range []string{TopicPatchPreviewed, TopicPatchApplied, TopicPatchFailed, TopicPatchReverted} {
		ret = append(ret, eventcontract.DataTopic[PatchChangedData]("orchestrator", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	for _, topic := range []string{
		TopicArtifactContinuationCreated,
		TopicArtifactContinuationChunkRecorded,
		TopicArtifactContinuationChunkRejected,
		TopicArtifactContinuationAssembled,
		TopicArtifactContinuationApplied,
		TopicArtifactContinuationFailed,
	} {
		ret = append(ret, eventcontract.DataTopic[ArtifactContinuationChangedData]("orchestrator", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	ret = append(ret,
		eventcontract.DataTopic[RetryRuntimeDriveCommand]("orchestrator", definitionPackage, eventcontract.LanePolicyState, TopicOrchestratorRetryRuntimeDrive).MessageContract(),
		eventcontract.DataTopic[RuntimeGovernanceWaitRecordedData]("orchestrator", definitionPackage, eventcontract.LanePolicyState, TopicOrchestratorRuntimeGovernanceWaitRecorded).MessageContract(),
		eventcontract.DataTopic[RuntimeContinuationEligibleData]("orchestrator", definitionPackage, eventcontract.LanePolicyState, TopicOrchestratorRuntimeContinuationEligible).MessageContract(),
		eventcontract.DataTopic[RuntimeTerminalDeliveryRequestData]("orchestrator", definitionPackage, eventcontract.LanePolicyState, TopicOrchestratorRuntimeTerminalDeliveryRequested).MessageContract(),
		eventcontract.DataTopic[RuntimeDriveObservedData]("orchestrator", definitionPackage, eventcontract.LanePolicyState, TopicOrchestratorRuntimeDriveObserved).MessageContract(),
	)
	return ret
}
