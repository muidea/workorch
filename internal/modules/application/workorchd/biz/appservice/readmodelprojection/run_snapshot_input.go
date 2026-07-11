package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
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
	CoverageWorkItems     []*orchestratormodel.CoverageWorkItem
	EventRecords          []*eventstreammodel.EventRecord
	LLMUsageEventRecords  []*eventstreammodel.EventRecord
	MemoryRecallTraces    []*memorymodel.MemoryRecallTrace
	Decisions             []readmodelcore.Decision
	SelectedTicket        *decisionmodel.Ticket
	SelectedResolution    *decisionmodel.Resolution
	Approvals             []readmodelcore.Approval
	SelectedApproval      *approvalmodel.Approval
	MCPCapabilities       []*mcpmodel.Capability
	PublishedEvents       []eventstreamevents.EventRecordData
}
