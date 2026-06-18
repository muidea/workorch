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
	workorchEventContracts = map[string]WorkorchContractSpec{
		workspaceevents.TopicWorkspaceCreated:                     newWorkorchEventContract(workspaceevents.TopicWorkspaceCreated),
		workspaceevents.TopicWorkspaceStateChanged:                newWorkorchEventContract(workspaceevents.TopicWorkspaceStateChanged),
		workspaceevents.TopicWorkspaceDeleted:                     newWorkorchEventContract(workspaceevents.TopicWorkspaceDeleted),
		workspaceevents.TopicGoalDefined:                          newWorkorchEventContract(workspaceevents.TopicGoalDefined),
		workspaceevents.TopicDoDDefined:                           newWorkorchEventContract(workspaceevents.TopicDoDDefined),
		runevents.TopicRunCreated:                                 newWorkorchEventContract(runevents.TopicRunCreated),
		runevents.TopicRunStateChanged:                            newWorkorchEventContract(runevents.TopicRunStateChanged),
		runevents.TopicRunLLMStreamProgress:                       newWorkorchEventContract(runevents.TopicRunLLMStreamProgress),
		runevents.TopicRunLLMUsageRecorded:                        newWorkorchEventContract(runevents.TopicRunLLMUsageRecorded),
		runevents.TopicStepDispatched:                             newWorkorchEventContract(runevents.TopicStepDispatched),
		runevents.TopicStepStateChanged:                           newWorkorchEventContract(runevents.TopicStepStateChanged),
		decisionevents.TopicTicketCreated:                         newWorkorchEventContract(decisionevents.TopicTicketCreated),
		decisionevents.TopicTicketResolved:                        newWorkorchEventContract(decisionevents.TopicTicketResolved),
		decisionevents.TopicTicketUpdated:                         newWorkorchEventContract(decisionevents.TopicTicketUpdated),
		approvalevents.TopicApprovalCreated:                       newWorkorchEventContract(approvalevents.TopicApprovalCreated),
		approvalevents.TopicApprovalResolved:                      newWorkorchEventContract(approvalevents.TopicApprovalResolved),
		resultevents.TopicResultAccepted:                          newWorkorchEventContract(resultevents.TopicResultAccepted),
		resultevents.TopicResultRejected:                          newWorkorchEventContract(resultevents.TopicResultRejected),
		resultevents.TopicResultSuperseded:                        newWorkorchEventContract(resultevents.TopicResultSuperseded),
		resultevents.TopicArtifactCreated:                         newWorkorchEventContract(resultevents.TopicArtifactCreated),
		orchestratorevents.TopicPatchPreviewed:                    newWorkorchEventContract(orchestratorevents.TopicPatchPreviewed),
		orchestratorevents.TopicPatchApplied:                      newWorkorchEventContract(orchestratorevents.TopicPatchApplied),
		orchestratorevents.TopicPatchFailed:                       newWorkorchEventContract(orchestratorevents.TopicPatchFailed),
		orchestratorevents.TopicPatchReverted:                     newWorkorchEventContract(orchestratorevents.TopicPatchReverted),
		orchestratorevents.TopicArtifactContinuationCreated:       newWorkorchEventContract(orchestratorevents.TopicArtifactContinuationCreated),
		orchestratorevents.TopicArtifactContinuationChunkRecorded: newWorkorchEventContract(orchestratorevents.TopicArtifactContinuationChunkRecorded),
		orchestratorevents.TopicArtifactContinuationChunkRejected: newWorkorchEventContract(orchestratorevents.TopicArtifactContinuationChunkRejected),
		orchestratorevents.TopicArtifactContinuationAssembled:     newWorkorchEventContract(orchestratorevents.TopicArtifactContinuationAssembled),
		orchestratorevents.TopicArtifactContinuationApplied:       newWorkorchEventContract(orchestratorevents.TopicArtifactContinuationApplied),
		orchestratorevents.TopicArtifactContinuationFailed:        newWorkorchEventContract(orchestratorevents.TopicArtifactContinuationFailed),
		memoryevents.TopicMemoryCreated:                           newWorkorchEventContract(memoryevents.TopicMemoryCreated),
		memoryevents.TopicMemoryUpdated:                           newWorkorchEventContract(memoryevents.TopicMemoryUpdated),
		memoryevents.TopicMemoryDeleted:                           newWorkorchEventContract(memoryevents.TopicMemoryDeleted),
		memoryevents.TopicMemoryRecallRecorded:                    newWorkorchEventContract(memoryevents.TopicMemoryRecallRecorded),
		governanceevents.TopicGovernanceAutoEvaluated:             newWorkorchEventContract(governanceevents.TopicGovernanceAutoEvaluated),
		governanceevents.TopicGovernanceAutoResolved:              newWorkorchEventContract(governanceevents.TopicGovernanceAutoResolved),
		governanceevents.TopicGovernanceAutoEscalated:             newWorkorchEventContract(governanceevents.TopicGovernanceAutoEscalated),
		governanceevents.TopicGovernanceAutoBlocked:               newWorkorchEventContract(governanceevents.TopicGovernanceAutoBlocked),
	}
	workorchSnapshotContracts = map[string]WorkorchContractSpec{
		"/workspace/snapshot":       newWorkorchSnapshotContract("/workspace/snapshot"),
		"/run/snapshot":             newWorkorchSnapshotContract("/run/snapshot"),
		"/step/snapshot":            newWorkorchSnapshotContract("/step/snapshot"),
		"/decision/ticket/snapshot": newWorkorchSnapshotContract("/decision/ticket/snapshot"),
		"/approval/snapshot":        newWorkorchSnapshotContract("/approval/snapshot"),
	}
)

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
