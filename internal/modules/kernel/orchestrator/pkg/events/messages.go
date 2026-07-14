package events

import (
	"fmt"
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

type ReleaseRuntimeV1ResourcesCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type ReleaseRuntimeV1ResourcesResult struct {
	ReleasedValidationResources int
	ClosedSteps                 int
	ClosedToolCalls             int
	ClosedArtifactContinuations int
	ReleasedTotal               int
}

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
}

type AcquireProviderCeilingCommand struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	PrerequisiteID      string
	ProviderID          string
	MinimumBytes        int
}

type AcquireProviderCeilingResult struct {
	PrerequisiteID string
	Ceiling        runtimecontract.ProviderCeiling
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
}

type RequestRuntimeCandidateResult struct {
	Transaction runtimecontract.AuthorityTransaction
	Candidate   runtimecontract.RuntimeCandidate
	Failure     *runtimecontract.RuntimeCandidateFailure
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

type ExecuteRuntimeOperationCommand struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   runtimecontract.AuthorityTransaction
	Operation                     runtimecontract.RuntimeOperation
	EvidenceKind                  string
	EvidencePolicies              []runtimecontract.RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
}

type ExecuteRuntimeOperationResult struct {
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
}

type FileEditOperationData struct {
	Path       string
	OldText    string
	NewText    string
	BaseHash   string
	Occurrence int64
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

type ExecuteFileEditOperationCommand struct {
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

type ExecutePatchApplyOperationCommand struct {
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

type ExecuteFileEditOperationResult struct {
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
}

type ExecutePatchApplyOperationResult struct {
	EventID      string
	OperationID  string
	EvidenceKind string
	Status       runtimecontract.RuntimeOperationExecutionStatus
	SourceRef    string
	Evidence     runtimecontract.RuntimeOperationEvidence
	Failure      *runtimecontract.FailureFact
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
	return AcquireRuntimeCapabilitiesResult{PrerequisiteID: command.PrerequisiteID, Capabilities: capabilities}, nil
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
		eventcontract.CommandTopic[ReleaseRuntimeV1ResourcesCommand, ReleaseRuntimeV1ResourcesResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandReleaseRuntimeV1Resources).MessageContract(),
		eventcontract.CommandTopic[ListToolCallsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListToolCallsByRun).MessageContract(),
		eventcontract.CommandTopic[ListPatchesByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListPatchesByRun).MessageContract(),
		eventcontract.CommandTopic[ListValidationResultsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListValidationResultsByRun).MessageContract(),
		eventcontract.CommandTopic[ListCoverageWorkItemsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListCoverageWorkItemsByRun).MessageContract(),
		eventcontract.CommandTopic[ListArtifactContinuationsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListArtifactContinuationsByRun).MessageContract(),
		eventcontract.CommandTopic[AcquireTargetSnapshotCommand, AcquireTargetSnapshotResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireTargetSnapshot).MessageContract(),
		eventcontract.CommandTopic[AcquireWorkspaceManifestCommand, AcquireWorkspaceManifestResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireWorkspaceManifest).MessageContract(),
		eventcontract.CommandTopic[AcquireProviderCeilingCommand, AcquireProviderCeilingResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireProviderCeiling).MessageContract(),
		eventcontract.CommandTopic[AcquireRuntimeCapabilitiesCommand, AcquireRuntimeCapabilitiesResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorAcquireRuntimeCapabilities).MessageContract(),
		eventcontract.CommandTopic[DriveRuntimeV2Command, DriveRuntimeV2Result]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorDriveRuntimeV2).MessageContract(),
		eventcontract.CommandTopic[RequestRuntimeCandidateCommand, RequestRuntimeCandidateResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorRequestRuntimeCandidate).MessageContract(),
		eventcontract.CommandTopic[ExecuteRuntimeOperationCommand, ExecuteRuntimeOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteRuntimeOperation).MessageContract(),
		eventcontract.CommandTopic[ExecuteFileEditOperationCommand, ExecuteFileEditOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecuteFileEditOperation).MessageContract(),
		eventcontract.CommandTopic[ExecutePatchApplyOperationCommand, ExecutePatchApplyOperationResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorExecutePatchApplyOperation).MessageContract(),
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
