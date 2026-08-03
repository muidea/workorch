package events

import (
	"fmt"

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
	StateRevision       uint64
	EvidenceRevision    string
	DefinitionRevision  string
	AuthoritySnapshotID string
	DispatchID          string
	State               runtimecontract.ConvergenceState
	BlockingTicketID    common.TicketID
	Terminal            *runtimecontract.RuntimeTerminalDecision
}

type RuntimeOperationExecutionEnvelope = runtimecontract.RuntimeOperationExecutionEnvelope

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
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	ResolutionID        common.ResolutionID
	Disposition         RuntimeGovernanceResolutionDisposition
	StateRevision       uint64
	EvidenceRevision    string
	AuthoritySnapshotID string
	State               runtimecontract.ConvergenceState
	Terminal            *runtimecontract.RuntimeTerminalDecision
}

// Validate checks the immutable cross-owner commit fact. It does not advance
// Runtime state; SM-2 remains exclusively owned by Orchestrator Biz.
func (r CommitRuntimeGovernanceResolutionResult) Validate() error {
	if r.WorkspaceID == "" || r.RunID == "" || r.ResolutionID == "" {
		return fmt.Errorf("%w: runtime governance result requires workspace, run, and resolution identity", common.ErrInvalidArgument)
	}
	switch r.Disposition {
	case RuntimeGovernanceResolutionDispositionCommitted,
		RuntimeGovernanceResolutionDispositionStale,
		RuntimeGovernanceResolutionDispositionDuplicate:
	default:
		return fmt.Errorf("%w: invalid runtime governance resolution disposition %q", common.ErrInvalidArgument, r.Disposition)
	}
	if r.StateRevision == 0 || r.AuthoritySnapshotID == "" || r.State == "" {
		return fmt.Errorf("%w: runtime governance result requires committed state identity", common.ErrInvalidArgument)
	}
	switch r.State {
	case runtimecontract.ConvergenceStateComplete, runtimecontract.ConvergenceStateFailed:
		if r.Terminal == nil {
			return fmt.Errorf("%w: terminal runtime governance result requires terminal detail", common.ErrInvalidArgument)
		}
	case runtimecontract.ConvergenceStateNeedPrerequisite,
		runtimecontract.ConvergenceStateNeedStateChange,
		runtimecontract.ConvergenceStateNeedValidation,
		runtimecontract.ConvergenceStateNeedAssessment,
		runtimecontract.ConvergenceStateNeedGovernance:
		if r.Terminal != nil {
			return fmt.Errorf("%w: non-terminal runtime governance result cannot carry terminal detail", common.ErrInvalidArgument)
		}
	default:
		return fmt.Errorf("%w: invalid runtime governance convergence state %q", common.ErrInvalidArgument, r.State)
	}
	return nil
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
		eventcontract.CommandTopic[DriveRuntimeCommand, DriveRuntimeResult]("orchestrator", definitionPackage, eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute, TopicOrchestratorDriveRuntime).MessageContract(),
		eventcontract.CommandTopic[QueryRuntimeOperationReceiptCommand, QueryRuntimeOperationReceiptResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorQueryRuntimeOperationReceipt).MessageContract(),
		eventcontract.CommandTopic[CommitRuntimeOperationReceiptCommand, CommitRuntimeOperationReceiptResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommitRuntimeOperationReceipt).MessageContract(),
		eventcontract.CommandTopic[RequestRuntimeTerminationCommand, RequestRuntimeTerminationResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorRequestRuntimeTermination).MessageContract(),
		eventcontract.CommandTopic[CommitRuntimeGovernanceResolutionCommand, CommitRuntimeGovernanceResolutionResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommitRuntimeGovernanceResolution).MessageContract(),
		eventcontract.CommandTopic[CommitRuntimeGovernanceTicketResolutionCommand, CommitRuntimeGovernanceResolutionResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommitRuntimeGovernanceTicket).MessageContract(),
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
