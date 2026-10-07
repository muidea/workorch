package readmodelprojection

import (
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RunSnapshotModelInput struct {
	Workspace             *workspacemodel.Workspace
	Run                   *runmodel.Run
	Steps                 []*runmodel.Step
	ToolCalls             []*runtimeevidencemodel.ToolCall
	Patches               []*runtimeevidencemodel.Patch
	Validations           []*runtimeevidencemodel.ValidationResult
	Checkpoints           []*runmodel.RuntimeCheckpoint
	Results               []*resultmodel.Result
	Artifacts             []*resultmodel.Artifact
	ArtifactContinuations []*orchestratormodel.ArtifactContinuation
	CoverageWorkItems     []*orchestratormodel.CoverageWorkItem
	EventRecords          []*eventstreammodel.EventRecord
	LLMUsageEventRecords  []*eventstreammodel.EventRecord
	LLMUsageLoaded        bool
	LLMUsageLimited       bool
	MemoryRecallTraces    []*memorymodel.MemoryRecallTrace
	Decisions             []projectioncontract.Decision
	SelectedTicket        *decisionmodel.Ticket
	SelectedResolution    *decisionmodel.Resolution
	Approvals             []projectioncontract.Approval
	SelectedApproval      *approvalmodel.Approval
	MCPCapabilities       []*mcpmodel.Capability
	PublishedEvents       []eventstreamevents.EventRecordData
}
