package readmodelports

import (
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
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
	CoverageWorkItems     []*orchestratormodel.CoverageWorkItem
	EventRecords          []*eventstreammodel.EventRecord
	LLMUsageEventRecords  []*eventstreammodel.EventRecord
	MemoryRecallTraces    []*memorymodel.MemoryRecallTrace
	Decisions             RunDecisions
	Approvals             RunApprovals
	MCPCapabilities       []*mcpmodel.Capability
}
