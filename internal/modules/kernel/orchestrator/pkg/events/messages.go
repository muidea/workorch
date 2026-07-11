package events

import (
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
type RecoverRuntimeGapCommand = runcontracts.RecoverRuntimeGapCommand
type ValidateRunCommand = runcontracts.ValidateRunCommand
type CompleteRunCommand = runcontracts.CompleteRunCommand
type CancelRunCommand = runcontracts.CancelRunCommand
type SupersedeRunCommand = runcontracts.SupersedeRunCommand
type FailRunCommand = runcontracts.FailRunCommand
type PurgeRunCommand = orchestratorcontracts.PurgeRunCommand
type UpdateRuntimePolicyCommand = orchestratorcontracts.UpdateRuntimePolicyCommand
type DispatchStepCommand = orchestratorcontracts.DispatchStepCommand

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

type ContractViolationData struct {
	SubjectID    string
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	SourceStepID common.StepID
	RetryStepID  common.StepID
	TicketID     common.TicketID
	Topic        string
	Summary      string
	Violation    StructuredPayload
	Context      StructuredPayload
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
		eventcontract.CommandTopic[RecoverRuntimeGapCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandRecoverRuntimeGap).MessageContract(),
		eventcontract.CommandTopic[ValidateRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandValidateRun).MessageContract(),
		eventcontract.CommandTopic[CompleteRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandCompleteRun).MessageContract(),
		eventcontract.CommandTopic[CancelRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandCancelRun).MessageContract(),
		eventcontract.CommandTopic[SupersedeRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandSupersedeRun).MessageContract(),
		eventcontract.CommandTopic[FailRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandFailRun).MessageContract(),
		eventcontract.CommandTopic[PurgeRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandPurgeRun).MessageContract(),
		eventcontract.CommandTopic[UpdateRuntimePolicyCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorCommandUpdateRuntimePolicy).MessageContract(),
		eventcontract.CommandTopic[ListToolCallsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListToolCallsByRun).MessageContract(),
		eventcontract.CommandTopic[ListPatchesByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListPatchesByRun).MessageContract(),
		eventcontract.CommandTopic[ListValidationResultsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListValidationResultsByRun).MessageContract(),
		eventcontract.CommandTopic[ListCoverageWorkItemsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListCoverageWorkItemsByRun).MessageContract(),
		eventcontract.CommandTopic[ListArtifactContinuationsByRunCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicOrchestratorCommandListArtifactContinuationsByRun).MessageContract(),
		eventcontract.CommandTopic[DispatchStepCommand, OrchestratorCommandResult]("orchestrator", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicOrchestratorDispatchStepRequested).MessageContract(),
	}
	for _, topic := range []string{
		TopicOrchestratorContractViolation,
		TopicOrchestratorContractViolationRecorded,
		TopicOrchestratorContractViolationRetryRequested,
		TopicOrchestratorContractViolationEscalated,
	} {
		ret = append(ret, eventcontract.DataTopic[ContractViolationData]("orchestrator", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
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
