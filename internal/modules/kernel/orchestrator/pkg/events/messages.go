package events

import (
	"fmt"
	"sort"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/common"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
const CommandDestination = modulecommon.ModuleID

type PurgeRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type DriveRuntimeCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	// RecoveryLeaseGeneration is an SM-4 private scheduler credential. A
	// normal Start/Resume leaves it zero and therefore cannot claim or execute
	// a persisted recoverable-wait probe.
	RecoveryLeaseGeneration uint64
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
	BlockingApprovalID  common.ApprovalID
	RecoverableWait     *runtimecontract.RuntimeRecoverableWaitView
	Terminal            *runtimecontract.RuntimeTerminalDecision
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

type CommitRuntimeGovernanceApprovalResolutionCommand struct {
	ResolutionID common.ResolutionID
	ApprovalID   common.ApprovalID
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

type ListToolCallsByRunCommand ListRunEvidenceCommand
type ListPatchesByRunCommand ListRunEvidenceCommand
type ListValidationResultsByRunCommand ListRunEvidenceCommand
type ListCoverageWorkItemsByRunCommand ListRunEvidenceCommand
type ListArtifactContinuationsByRunCommand ListRunEvidenceCommand

type ArtifactContinuationChunkHash struct {
	Sequence int
	Hash     string
}

type ArtifactContinuationRetryCounter struct {
	Key   string
	Count int
}

// ArtifactContinuation is the owner-local read Result. Runtime's map-backed
// bookkeeping remains private to Orchestrator persistence; EventHub exposes
// deterministic entries and callers explicitly rebuild their read model.
type ArtifactContinuation struct {
	ID                   common.ArtifactContinuationID
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	RootStepID           common.StepID
	CurrentStepID        common.StepID
	ArtifactID           string
	TargetRef            string
	Operation            string
	OutputMode           string
	AuthorityTransaction runtimecontract.AuthorityTransaction
	CandidateAttempt     int
	StablePrefixHash     string
	MaxChunks            int
	TailBytes            int
	CoverageTargets      []string
	State                common.ArtifactContinuationState
	ExpectedSequence     int
	ReceivedSequences    []int
	ReceivedChunkHashes  []ArtifactContinuationChunkHash
	FinalReceived        bool
	BytesWritten         int64
	RollingHash          string
	LastChunkHash        string
	PreviousChunkTail    string
	RetryCounters        []ArtifactContinuationRetryCounter
	EvidenceRefs         []string
	CreatedAt            string
	UpdatedAt            string
	Metadata             workorcheventpayload.Payload
}

func ArtifactContinuationFromModel(item *runtimecontract.ArtifactContinuation) *ArtifactContinuation {
	if item == nil {
		return nil
	}
	return &ArtifactContinuation{
		ID: item.ID, WorkspaceID: item.WorkspaceID, RunID: item.RunID, RootStepID: item.RootStepID,
		CurrentStepID: item.CurrentStepID, ArtifactID: item.ArtifactID, TargetRef: item.TargetRef,
		Operation: item.Operation, OutputMode: item.OutputMode, AuthorityTransaction: item.AuthorityTransaction,
		CandidateAttempt: item.CandidateAttempt, StablePrefixHash: item.StablePrefixHash, MaxChunks: item.MaxChunks,
		TailBytes: item.TailBytes, CoverageTargets: append([]string(nil), item.CoverageTargets...), State: item.State,
		ExpectedSequence: item.ExpectedSequence, ReceivedSequences: append([]int(nil), item.ReceivedSequences...),
		ReceivedChunkHashes: artifactContinuationChunkHashes(item.ReceivedChunkHashes), FinalReceived: item.FinalReceived,
		BytesWritten: item.BytesWritten, RollingHash: item.RollingHash, LastChunkHash: item.LastChunkHash,
		PreviousChunkTail: item.PreviousChunkTail, RetryCounters: artifactContinuationRetryCounters(item.RetryCounters),
		EvidenceRefs: append([]string(nil), item.EvidenceRefs...), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		Metadata: item.Metadata.Clone(),
	}
}

func ArtifactContinuationsFromModels(items []*runtimecontract.ArtifactContinuation) []*ArtifactContinuation {
	ret := make([]*ArtifactContinuation, 0, len(items))
	for _, item := range items {
		ret = append(ret, ArtifactContinuationFromModel(item))
	}
	return ret
}

func (a *ArtifactContinuation) Model() (*runtimecontract.ArtifactContinuation, error) {
	if a == nil {
		return nil, common.ErrInvalidArgument
	}
	hashes, err := artifactContinuationChunkHashMap(a.ReceivedChunkHashes)
	if err != nil {
		return nil, err
	}
	retries, err := artifactContinuationRetryCounterMap(a.RetryCounters)
	if err != nil {
		return nil, err
	}
	return &runtimecontract.ArtifactContinuation{
		ID: a.ID, WorkspaceID: a.WorkspaceID, RunID: a.RunID, RootStepID: a.RootStepID,
		CurrentStepID: a.CurrentStepID, ArtifactID: a.ArtifactID, TargetRef: a.TargetRef,
		Operation: a.Operation, OutputMode: a.OutputMode, AuthorityTransaction: a.AuthorityTransaction,
		CandidateAttempt: a.CandidateAttempt, StablePrefixHash: a.StablePrefixHash, MaxChunks: a.MaxChunks,
		TailBytes: a.TailBytes, CoverageTargets: append([]string(nil), a.CoverageTargets...), State: a.State,
		ExpectedSequence: a.ExpectedSequence, ReceivedSequences: append([]int(nil), a.ReceivedSequences...),
		ReceivedChunkHashes: hashes, FinalReceived: a.FinalReceived, BytesWritten: a.BytesWritten,
		RollingHash: a.RollingHash, LastChunkHash: a.LastChunkHash, PreviousChunkTail: a.PreviousChunkTail,
		RetryCounters: retries, EvidenceRefs: append([]string(nil), a.EvidenceRefs...), CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt, Metadata: a.Metadata.Clone(),
	}, nil
}

func artifactContinuationChunkHashes(values map[int]string) []ArtifactContinuationChunkHash {
	ret := make([]ArtifactContinuationChunkHash, 0, len(values))
	for sequence, hash := range values {
		ret = append(ret, ArtifactContinuationChunkHash{Sequence: sequence, Hash: hash})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Sequence < ret[j].Sequence })
	return ret
}

func artifactContinuationChunkHashMap(values []ArtifactContinuationChunkHash) (map[int]string, error) {
	ret := make(map[int]string, len(values))
	for _, item := range values {
		if _, exists := ret[item.Sequence]; exists {
			return nil, common.ErrInvalidArgument
		}
		ret[item.Sequence] = item.Hash
	}
	return ret, nil
}

func artifactContinuationRetryCounters(values map[string]int) []ArtifactContinuationRetryCounter {
	ret := make([]ArtifactContinuationRetryCounter, 0, len(values))
	for key, count := range values {
		ret = append(ret, ArtifactContinuationRetryCounter{Key: key, Count: count})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Key < ret[j].Key })
	return ret
}

func artifactContinuationRetryCounterMap(values []ArtifactContinuationRetryCounter) (map[string]int, error) {
	ret := make(map[string]int, len(values))
	for _, item := range values {
		if strings.TrimSpace(item.Key) == "" {
			return nil, common.ErrInvalidArgument
		}
		if _, exists := ret[item.Key]; exists {
			return nil, common.ErrInvalidArgument
		}
		ret[item.Key] = item.Count
	}
	return ret, nil
}

type OrchestratorCommandResult struct {
	ToolCalls             []*runtimeevidencemodel.ToolCall
	Patches               []*runtimeevidencemodel.Patch
	ValidationResults     []*runtimeevidencemodel.ValidationResult
	CoverageWorkItems     []*runtimecontract.CoverageWorkItem
	ArtifactContinuations []*ArtifactContinuation
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
		eventcontract.CommandTopic[CommitRuntimeGovernanceApprovalResolutionCommand, CommitRuntimeGovernanceResolutionResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommitRuntimeGovernanceApproval).MessageContract(),
	}
	ret = append(ret,
		eventcontract.DataTopic[RuntimeDriveObservedData]("orchestrator", definitionPackage, eventcontract.LanePolicyState, TopicOrchestratorRuntimeDriveObserved).MessageContract(),
	)
	return ret
}
