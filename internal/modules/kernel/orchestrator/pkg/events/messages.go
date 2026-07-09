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

type ListRunEvidenceCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type ListToolCallsByRunCommand = ListRunEvidenceCommand
type ListPatchesByRunCommand = ListRunEvidenceCommand
type ListValidationResultsByRunCommand = ListRunEvidenceCommand
type ListCoverageWorkItemsByRunCommand = ListRunEvidenceCommand
type ListArtifactContinuationsByRunCommand = ListRunEvidenceCommand

type OrchestratorData struct{}

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
	}
	for _, topic := range []string{
		TopicOrchestratorDispatchStepRequested,
		TopicOrchestratorStageChanged,
		TopicOrchestratorGraphDispatched,
		TopicOrchestratorRepairStarted,
		TopicOrchestratorReplanTriggered,
		TopicOrchestratorContractViolation,
		TopicOrchestratorContractViolationRecorded,
		TopicOrchestratorContractViolationRetryRequested,
		TopicOrchestratorContractViolationEscalated,
		TopicPatchPreviewed,
		TopicPatchApplied,
		TopicPatchFailed,
		TopicPatchReverted,
		TopicArtifactContinuationCreated,
		TopicArtifactContinuationChunkRecorded,
		TopicArtifactContinuationChunkRejected,
		TopicArtifactContinuationAssembled,
		TopicArtifactContinuationApplied,
		TopicArtifactContinuationFailed,
	} {
		ret = append(ret, eventcontract.DataTopic[OrchestratorData]("orchestrator", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	return ret
}
