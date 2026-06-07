package readmodelports

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func NewOwnerReaders(hub events.Hub) (OwnerReaders, error) {
	if hub == nil {
		return OwnerReaders{}, common.ErrInvalidArgument
	}
	return OwnerReaders{
		Workspaces:            WorkspaceReader{Hub: hub},
		Goals:                 GoalReader{Hub: hub},
		DoDs:                  DoDReader{Hub: hub},
		Runs:                  RunReader{Hub: hub},
		Steps:                 StepReader{Hub: hub},
		Results:               ResultReader{Hub: hub},
		Artifacts:             ArtifactReader{Hub: hub},
		ArtifactContinuations: ArtifactContinuationReader{Hub: hub},
		EventRecords:          EventRecordReader{Hub: hub},
		MemoryRecalls:         MemoryRecallTraceReader{Hub: hub},
		ToolCalls:             ToolCallReader{Hub: hub},
		MCPCapabilities:       MCPCapabilityReader{Hub: hub},
		Patches:               PatchReader{Hub: hub},
		Validations:           ValidationResultReader{Hub: hub},
		Checkpoints:           RuntimeCheckpointReader{Hub: hub},
		AutoGovernanceTraces:  AutoGovernanceTraceReader{Hub: hub},
		Tickets:               DecisionTicketReader{Hub: hub},
		Resolutions:           DecisionResolutionReader{Hub: hub},
		Approvals:             ApprovalReader{Hub: hub},
	}, nil
}
