package readmodelports

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runSnapshotModels struct {
	Workspace             *workspacemodel.Workspace
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
	Decisions             RunDecisions
	Approvals             RunApprovals
	MCPCapabilities       []*mcpmodel.Capability
}
