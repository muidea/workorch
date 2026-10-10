package readmodelports

import (
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runSnapshotModels struct {
	Workspace             *workspacemodel.Workspace
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
	Decisions             RunDecisions
	Approvals             RunApprovals
	MCPCapabilities       []*mcpmodel.Capability
}
