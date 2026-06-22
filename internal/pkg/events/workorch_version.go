package events

import (
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
)

const (
	WorkorchEventVersion    = "workorch.event.v1"
	WorkorchSnapshotVersion = "workorch.v1"
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
		runevents.TopicRunStateChanged,
		runevents.TopicRunLLMStreamProgress,
		runevents.TopicRunLLMUsageRecorded,
		runevents.TopicStepDispatched,
		runevents.TopicStepStateChanged,
		decisionevents.TopicTicketCreated,
		decisionevents.TopicTicketResolved,
		decisionevents.TopicTicketUpdated,
		approvalevents.TopicApprovalCreated,
		approvalevents.TopicApprovalResolved,
		resultevents.TopicResultAccepted,
		resultevents.TopicResultRejected,
		resultevents.TopicResultSuperseded,
		resultevents.TopicArtifactCreated,
		orchestratorevents.TopicPatchPreviewed,
		orchestratorevents.TopicPatchApplied,
		orchestratorevents.TopicPatchFailed,
		orchestratorevents.TopicPatchReverted,
		orchestratorevents.TopicOrchestratorContractViolationRecorded,
		orchestratorevents.TopicOrchestratorContractViolationRetryRequested,
		orchestratorevents.TopicOrchestratorContractViolationEscalated,
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
		governanceevents.TopicGovernanceAutoEvaluated,
		governanceevents.TopicGovernanceAutoResolved,
		governanceevents.TopicGovernanceAutoEscalated,
		governanceevents.TopicGovernanceAutoBlocked,
	}
	workorchEventContracts    = buildWorkorchEventContracts(workorchEventTopics)
	workorchSnapshotContracts = map[string]WorkorchContractSpec{
		"/workspace/snapshot":       newWorkorchSnapshotContract("/workspace/snapshot"),
		"/run/snapshot":             newWorkorchSnapshotContract("/run/snapshot"),
		"/step/snapshot":            newWorkorchSnapshotContract("/step/snapshot"),
		"/decision/ticket/snapshot": newWorkorchSnapshotContract("/decision/ticket/snapshot"),
		"/approval/snapshot":        newWorkorchSnapshotContract("/approval/snapshot"),
	}
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

func WorkorchSnapshotContract(topic string) (WorkorchContractSpec, bool) {
	spec, ok := workorchSnapshotContracts[topic]
	return spec, ok
}

func ValidateWorkorchEventVersion(topic, version string) bool {
	spec, ok := WorkorchEventContract(topic)
	return ok && version == spec.CurrentVersion
}

func ValidateWorkorchSnapshotVersion(topic, version string) bool {
	spec, ok := WorkorchSnapshotContract(topic)
	return ok && version == spec.CurrentVersion
}

func newWorkorchEventContract(topic string) WorkorchContractSpec {
	return WorkorchContractSpec{
		Topic:          topic,
		CurrentVersion: WorkorchEventVersion,
	}
}

func newWorkorchSnapshotContract(topic string) WorkorchContractSpec {
	return WorkorchContractSpec{
		Topic:          topic,
		CurrentVersion: WorkorchSnapshotVersion,
	}
}
