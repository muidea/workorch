package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RunSnapshotModelInput struct {
	Workspace             *workspacemodel.Workspace
	Run                   *runmodel.Run
	Steps                 []*runmodel.Step
	ToolCalls             []*runmodel.ToolCall
	Patches               []*runmodel.Patch
	Validations           []*runmodel.ValidationResult
	Checkpoints           []*runmodel.RuntimeCheckpoint
	Results               []*resultmodel.Result
	Artifacts             []*resultmodel.Artifact
	ArtifactContinuations []*orchestratormodel.ArtifactContinuation
	EventRecords          []*eventstreammodel.EventRecord
	LLMUsageEventRecords  []*eventstreammodel.EventRecord
	MemoryRecallTraces    []*memorymodel.MemoryRecallTrace
	Decisions             []readmodelcore.Decision
	SelectedTicket        *decisionmodel.Ticket
	SelectedResolution    *decisionmodel.Resolution
	Approvals             []readmodelcore.Approval
	SelectedApproval      *approvalmodel.Approval
	MCPCapabilities       []*mcpmodel.Capability
	PublishedEvents       []events.WorkorchEvent
}
