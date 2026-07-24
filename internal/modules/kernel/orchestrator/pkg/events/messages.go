package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
const CommandDestination = modulecommon.ModuleID

type StartRunCommand = runcontracts.StartRunCommand
type BlockDecisionCommand = runcontracts.BlockOnDecisionCommand
type BlockApprovalCommand = runcontracts.BlockOnApprovalCommand
type ResumeRunCommand = runcontracts.ResumeRunCommand
type CancelRunCommand = runcontracts.CancelRunCommand
type SupersedeRunCommand = runcontracts.SupersedeRunCommand
type FailRunCommand = runcontracts.FailRunCommand
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
	// Observation is the persisted freshness metadata for Admissibility. It is
	// supplied by Orchestrator so an owner can return an exact typed stale
	// detail without reconstructing authority from local process state.
	Observation *runtimecontract.RuntimePrerequisiteObservation
	// CurrentTargetSnapshot is the persisted owner snapshot for this operation's
	// concrete target, when available. It distinguishes a missing repair input
	// from a candidate that ignored already supplied current content.
	CurrentTargetSnapshot *runtimecontract.TargetSnapshot
}

type ProbeRuntimeOperationFeasibilityResult struct {
	OperationID             string
	AdmissibilitySnapshotID string
	CapabilityStateBinding  string
	Feasible                bool
	ReplayProof             *runtimecontract.RuntimeOperationReplayProof
	Failure                 *runtimecontract.FailureFact
	Stale                   *runtimecontract.RuntimePrerequisiteStale
	PrerequisiteRequest     *runtimecontract.RuntimeOperationPrerequisiteRequest
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

type RequestRuntimeCandidateCommand struct {
	WorkspaceID       common.WorkspaceID
	RunID             common.RunID
	Decision          runtimecontract.RuntimeDecision
	Transaction       runtimecontract.AuthorityTransaction
	CandidateAttempt  int
	PreviousFailure   *runtimecontract.RuntimeCandidateFailure
	StablePrefixHash  string
	DefinitionContext runtimecontract.RuntimeDefinitionContext
	CandidateContext  runtimecontract.RuntimeCandidateContext
	// OutputContinuation is absent for the initial structured-candidate
	// request.  When present it requests the next raw suffix for a persisted,
	// bounded candidate-output continuation.
	OutputContinuation *runtimecontract.RuntimeCandidateOutputContinuationRequest
}

type RequestRuntimeCandidateResult struct {
	Transaction runtimecontract.AuthorityTransaction
	Candidate   runtimecontract.RuntimeCandidate
	Failure     *runtimecontract.RuntimeCandidateFailure
	// Stale is an owner-confirmed pre-candidate freshness outcome. It is
	// mutually exclusive with Candidate and Failure; Orchestrator invalidates
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
	// OutputFragment is mutually exclusive with Candidate, Failure and Stale.
	// The Orchestrator persists it in its own artifact repository before making
	// another suffix request or assembling the candidate document.
	OutputFragment *runtimecontract.RuntimeCandidateOutputFragment
}

type RuntimeV2DriveDisposition string

const (
	RuntimeV2DriveDispositionGovernedWait RuntimeV2DriveDisposition = "governed_wait"
	RuntimeV2DriveDispositionTerminal     RuntimeV2DriveDisposition = "terminal"
)

type DriveRuntimeV2Command struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type DriveRuntimeV2Result struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	Disposition         RuntimeV2DriveDisposition
	StateRevision       uint64
	EvidenceRevision    string
	AuthoritySnapshotID string
	Action              runtimecontract.RuntimeAction
	BlockingTicketID    common.TicketID
	Terminal            *runtimecontract.TerminalOutcome
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
	Action              runtimecontract.RuntimeAction
	Terminal            *runtimecontract.TerminalOutcome
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
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	OperationID                   string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	TargetRole                    string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
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
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
}

type ExecuteFileWriteOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
}

type ExecuteFileListOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
	Data         FileListOperationResultData
}

type FileListOperationResultData struct {
	Status         string
	Stage          string
	Path           string
	Entries        []FileListEntry
	Truncated      bool
	MaxEntries     int64
	FailureCode    string
	FailureMessage string
}

type ExecuteFileReadOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
	Data         FileReadOperationResultData
}

type FileReadOperationResultData struct {
	Status           string
	Stage            string
	Path             string
	TargetRef        string
	Exists           bool
	Content          string
	ContentHash      string
	Size             int64
	ModifiedAt       string
	LineCount        int64
	Truncated        bool
	Binary           bool
	Redacted         bool
	ContentComplete  bool
	SafeForPatch     bool
	SafeForFileWrite bool
	FailureCode      string
	FailureMessage   string
}

type ExecutePatchApplyOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
}

type ExecutePatchPreviewOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
	Data         PatchPreviewOperationResultData
}

type PatchPreviewOperationResultData struct {
	Status           string
	Stage            string
	Operation        string
	PatchType        string
	TargetRef        string
	AffectedFiles    []string
	BeforeHash       string
	AfterHash        string
	ContentHash      string
	DiffHash         string
	AddedLines       int64
	DeletedLines     int64
	ApprovalRequired bool
	RiskLevel        string
	RiskReasons      []string
	WillWrite        bool
	Summary          string
	FailureCode      string
	FailureMessage   string
}

type ExecuteShellOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
	Data         ShellOperationResultData
}

type ShellOperationResultData struct {
	Status                      string
	Stage                       string
	Command                     string
	Args                        []string
	ActualCommand               string
	WorkingDirectory            string
	Stdout                      string
	Stderr                      string
	ExitCode                    int64
	DurationMillis              int64
	TimeoutMillis               int64
	OutputTruncated             bool
	StdoutTruncated             bool
	StderrTruncated             bool
	StdoutBytes                 int64
	StderrBytes                 int64
	FailureCode                 string
	FailureMessage              string
	Summary                     string
	Reason                      string
	FailureReason               string
	UnavailableCommand          string
	CapabilityMissing           bool
	ZeroWork                    bool
	EvidenceQuality             string
	EvidenceReason              string
	EvidenceTags                []string
	TargetRefs                  []string
	CommandNormalization        string
	MaxOutputBytes              int64
	ValidationEvidence          runtimecontract.RuntimeValidationEvidence
	ManagedValidation           runtimecontract.RuntimeManagedValidation
	ValidationResources         []runtimecontract.RuntimeValidationResource
	RuntimeDiagnostics          runtimecontract.RuntimeDiagnostics
	ValidationFailureDiagnostic *runtimecontract.ValidationFailureDiagnostic
}

type GitFileChangeData struct {
	Path      string
	OldPath   string
	Status    string
	Staged    bool
	Unstaged  bool
	Untracked bool
}

type GitAheadBehindData struct {
	Ahead  int64
	Behind int64
}

type GitCommitData struct {
	Hash       string
	ShortHash  string
	Author     string
	AuthorTime string
	Subject    string
}

type GitBranchData struct {
	Name     string
	Commit   string
	Current  bool
	Upstream string
}

type ExecuteGitOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
	Data         GitOperationResultData
}

type GitOperationResultData struct {
	Status             string
	Stage              string
	Operation          string
	RepoRoot           string
	WorkingDirectory   string
	Branch             string
	HeadCommit         string
	HeadCommitBefore   string
	HeadCommitAfter    string
	DetachedHead       bool
	UnbornHead         bool
	Stdout             string
	Stderr             string
	ExitCode           int64
	DurationMillis     int64
	TimeoutMillis      int64
	Truncated          bool
	ErrorCode          string
	ErrorMessage       string
	Summary            string
	ZeroWork           bool
	Initialized        bool
	ExistingRepository bool
	Pathspecs          []string
	Ref                string
	Limit              int64
	LimitClamped       bool
	Message            string
	CommitHash         string
	CommitSubject      string
	ChangedFiles       []GitFileChangeData
	StagedFiles        []GitFileChangeData
	CommittedFiles     []GitFileChangeData
	Commits            []GitCommitData
	Branches           []GitBranchData
	AheadBehind        GitAheadBehindData
}

type ExecuteMCPOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
	Data         MCPOperationResultData
}

type MCPOperationResultData struct {
	Status         string
	Stage          string
	Operation      string
	ServerID       string
	Tool           string
	URI            string
	Prompt         string
	Summary        string
	Transport      string
	Endpoint       string
	DurationMillis int64
	StatusCode     int64
	Stderr         string
	FailureCode    string
	FailureMessage string
	TargetRefs     []string
	Diagnostics    MCPFailureDiagnosticsData
}

type MCPFailureDiagnosticsData struct {
	Operation      string
	ServerID       string
	Tool           string
	URI            string
	Prompt         string
	TargetRef      string
	FailureTargets []string
	TargetRole     string
	OwnerRevision  string
}

type ExecuteSSHOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
	Data         SSHOperationResultData
}

type SSHOperationResultData struct {
	Status          string
	Host            string
	Port            int64
	Username        string
	Command         string
	Stdout          string
	Stderr          string
	ExitCode        int64
	DurationMillis  int64
	TimeoutMillis   int64
	AuthMethod      string
	OutputTruncated bool
	StdoutTruncated bool
	StderrTruncated bool
	StdoutBytes     int64
	StderrBytes     int64
	FailureCode     string
	FailureMessage  string
}

type ExecuteWebOperationResult struct {
	Receipt      runtimecontract.RuntimeOperationReceipt
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
	Data         WebOperationResultData
}

type WebOperationResultData struct {
	Status         string
	Stage          string
	ProviderID     string
	Query          string
	URL            string
	FinalURL       string
	Title          string
	Summary        string
	ContentType    string
	ContentHash    string
	RetrievedAt    string
	PublishedAt    string
	Source         string
	Size           int64
	ResultCount    int64
	Truncated      bool
	ApprovalID     common.ApprovalID
	ApprovalNeeded bool
	DurationMillis int64
	FailureCode    string
	FailureMessage string
	TargetRefs     []string
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
			return AcquireRuntimeCapabilitiesResult{}, fmt.Errorf("runtime module %s does not provide capability %s", moduleID, kind)
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
		eventcontract.CommandTopic[StartRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandStartRun).MessageContract(),
		eventcontract.CommandTopic[BlockDecisionCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandBlockDecision).MessageContract(),
		eventcontract.CommandTopic[BlockApprovalCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandBlockApproval).MessageContract(),
		eventcontract.CommandTopic[ResumeRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandResumeRun).MessageContract(),
		eventcontract.CommandTopic[CancelRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandCancelRun).MessageContract(),
		eventcontract.CommandTopic[SupersedeRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandSupersedeRun).MessageContract(),
		eventcontract.CommandTopic[FailRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandFailRun).MessageContract(),
		eventcontract.CommandTopic[PurgeRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandPurgeRun).MessageContract(),
		eventcontract.CommandTopic[ListToolCallsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListToolCallsByRun).MessageContract(),
		eventcontract.CommandTopic[ListPatchesByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListPatchesByRun).MessageContract(),
		eventcontract.CommandTopic[ListValidationResultsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListValidationResultsByRun).MessageContract(),
		eventcontract.CommandTopic[ListCoverageWorkItemsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListCoverageWorkItemsByRun).MessageContract(),
		eventcontract.CommandTopic[ListArtifactContinuationsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListArtifactContinuationsByRun).MessageContract(),
		eventcontract.CommandTopic[AcquireTargetSnapshotCommand, AcquireTargetSnapshotResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireTargetSnapshot).MessageContract(),
		eventcontract.CommandTopic[AcquireRuntimeTargetSelectionCommand, AcquireRuntimeTargetSelectionResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireRuntimeTargetSelection).MessageContract(),
		eventcontract.CommandTopic[AcquireWorkspaceManifestCommand, AcquireWorkspaceManifestResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireWorkspaceManifest).MessageContract(),
		eventcontract.CommandTopic[AcquireProviderCeilingCommand, AcquireProviderCeilingResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireProviderCeiling).MessageContract(),
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireRuntimeCapabilities).MessageContract(),
		eventcontract.CommandTopic[AcquireRuntimeOperationAdmissibilityCommand, AcquireRuntimeOperationAdmissibilityResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireRuntimeOperationAdmissibility).MessageContract(),
		eventcontract.CommandTopic[ProbeRuntimeOperationFeasibilityCommand, ProbeRuntimeOperationFeasibilityResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorProbeRuntimeOperationFeasibility).MessageContract(),
		eventcontract.CommandTopic[AcquireGitCapabilityStateCommand, AcquireGitCapabilityStateResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireGitCapabilityState).MessageContract(),
		eventcontract.CommandTopic[DriveRuntimeV2Command, DriveRuntimeV2Result]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorDriveRuntimeV2).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeOperationReceiptCommand, QueryRuntimeOperationReceiptResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorQueryRuntimeOperationReceipt).MessageContract(),
		eventcontract.CommandTopic[CommitRuntimeOperationReceiptCommand, CommitRuntimeOperationReceiptResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommitRuntimeOperationReceipt).MessageContract(),
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
	return ret
}
