package events

import (
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
)

const (
	WorkorchEventVersion = "workorch.event.v1"
)

type WorkorchContractSpec struct {
	Topic          string
	CurrentVersion string
}

var (
	workorchEventTopics = []string{
		workspaceevents.TopicWorkspaceCreated,
		workspaceevents.TopicWorkspaceStateChanged,
		workspaceevents.TopicWorkspaceDeleted,
		workspaceevents.TopicGoalDefined,
		workspaceevents.TopicDoDDefined,
		runevents.TopicRunCreated,
		runevents.TopicRunStarted,
		runevents.TopicRunResumed,
		runevents.TopicRunBlockedOnDecision,
		runevents.TopicRunBlockedOnApproval,
		runevents.TopicRunCompleted,
		runevents.TopicRunFailed,
		runevents.TopicRunCancelled,
		runevents.TopicRunSuperseded,
		runevents.TopicRunStateChanged,
		decisionevents.TopicTicketCreated,
		decisionevents.TopicTicketResolved,
		decisionevents.TopicTicketSuperseded,
		decisionevents.TopicTicketUpdated,
		decisionevents.TopicDecisionTicketCreated,
		decisionevents.TopicDecisionTicketResolved,
		decisionevents.TopicDecisionTicketSuperseded,
		approvalevents.TopicApprovalCreated,
		approvalevents.TopicApprovalApproved,
		approvalevents.TopicApprovalRejected,
		approvalevents.TopicApprovalResolved,
		approvalevents.TopicApprovalSuperseded,
		resultevents.TopicResultCreated,
		resultevents.TopicResultAccepted,
		resultevents.TopicResultRejected,
		resultevents.TopicResultSuperseded,
		resultevents.TopicArtifactCreated,
		orchestratorevents.TopicPatchPreviewed,
		orchestratorevents.TopicPatchApplied,
		orchestratorevents.TopicPatchFailed,
		orchestratorevents.TopicPatchReverted,
		orchestratorevents.TopicArtifactContinuationCreated,
		orchestratorevents.TopicArtifactContinuationChunkRecorded,
		orchestratorevents.TopicArtifactContinuationChunkRejected,
		orchestratorevents.TopicArtifactContinuationAssembled,
		orchestratorevents.TopicArtifactContinuationApplied,
		orchestratorevents.TopicArtifactContinuationFailed,
		memoryevents.TopicMemoryCreated,
		memoryevents.TopicMemoryUpdated,
		memoryevents.TopicMemoryDeleted,
		memoryevents.TopicMemoryRecallRecorded,
		llmevents.TopicLLMUsageRecorded,
		governanceevents.TopicGovernanceAutoEvaluated,
		governanceevents.TopicGovernanceAutoResolved,
		governanceevents.TopicGovernanceAutoEscalated,
		governanceevents.TopicGovernanceAutoBlocked,
	}
	workorchEventContracts = buildWorkorchEventContracts(workorchEventTopics)
)

func buildWorkorchEventContracts(topics []string) map[string]WorkorchContractSpec {
	ret := make(map[string]WorkorchContractSpec, len(topics))
	for _, topic := range topics {
		ret[topic] = newWorkorchEventContract(topic)
	}
	return ret
}

func WorkorchEventContract(topic string) (WorkorchContractSpec, bool) {
	spec, ok := workorchEventContracts[topic]
	return spec, ok
}

func ValidateWorkorchEventVersion(topic, version string) bool {
	spec, ok := WorkorchEventContract(topic)
	return ok && version == spec.CurrentVersion
}

func newWorkorchEventContract(topic string) WorkorchContractSpec {
	return WorkorchContractSpec{
		Topic:          topic,
		CurrentVersion: WorkorchEventVersion,
	}
}
