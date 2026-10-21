package server

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/content"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionquality"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// These named callback contracts keep the transport adapter's application
// boundary explicit. They are not application state or owner transitions;
// implementations are supplied by Workorchd Biz.

type CreateWorkspaceFunc func(context.Context, string, string, workorchmanagementcontract.WorkspaceCreateRequest) (appmodelcore.Workspace, error)
type DeleteWorkspaceFunc func(context.Context, string, string, string, appmodelcore.DeleteWorkspaceCommand) error
type ReadWorkspaceFunc func(context.Context, string, string, string) (appmodelcore.Workspace, error)
type ReadWorkspaceSnapshotFunc func(context.Context, string, string, string) (readmodelcore.WorkspaceSnapshotState, error)
type ReadProjectionSnapshotFunc func(context.Context, string, string, string) (readmodelcore.WorkspaceSnapshotState, error)
type ReadWorkspaceSoulBindingsFunc func(context.Context, string, string, string) (workorchmanagementcontract.WorkspaceSoulBindings, error)
type UpdateWorkspaceSoulBindingsFunc func(context.Context, string, string, string, workorchmanagementcontract.WorkspaceSoulBindings) (workorchmanagementcontract.WorkspaceSoulBindings, error)
type UpdateWorkspaceLLMSelectionFunc func(context.Context, string, *runtimecontract.ProviderInferenceSelection, uint64) (appmodelcore.Workspace, error)
type ListWorkspacesFunc func(context.Context, string, string) ([]appmodelcore.Workspace, error)
type ReadStartupPreparationFunc func(context.Context, string) (workorchmanagementcontract.StartupPreparationView, error)
type DefineGoalFunc func(context.Context, string, string, string, string) (workorchmanagementcontract.Goal, error)
type UpdateGoalFunc func(context.Context, string, string, string, string, string) (workorchmanagementcontract.Goal, error)
type ReadGoalFunc func(context.Context, string, string, string, string) (workorchmanagementcontract.Goal, error)
type ListGoalsFunc func(context.Context, string, string, string) ([]workorchmanagementcontract.Goal, error)
type DefineDoDFunc func(context.Context, string, string, string, []string) (workorchmanagementcontract.DoD, error)
type UpdateDoDFunc func(context.Context, string, string, string, string, []string) (workorchmanagementcontract.DoD, error)
type ReadDoDFunc func(context.Context, string, string, string, string) (workorchmanagementcontract.DoD, error)
type ListDoDsFunc func(context.Context, string, string, string) ([]workorchmanagementcontract.DoD, error)
type CheckGoalDoDQualityFunc func(context.Context, string, string, definitionquality.Command) (definitionquality.Check, error)
type ReadRunInstanceFunc func(context.Context, string, string, string) (appmodelcore.Run, error)
type ReadRunSnapshotFunc func(context.Context, string, string, string) (readmodelcore.RunSnapshotState, error)
type ListRunStepsFunc func(context.Context, string, string, workorchquerycontract.RunResourcePageQuery) (readmodelcore.StepPage, error)
type ListRunToolCallsFunc func(context.Context, string, string, workorchquerycontract.RunResourcePageQuery) (readmodelcore.ToolCallPage, error)
type ListRunRuntimeCheckpointsFunc func(context.Context, string, string, workorchquerycontract.RunResourcePageQuery) (readmodelcore.RuntimeCheckpointPage, error)
type ListRunResultsFunc func(context.Context, string, string, workorchquerycontract.RunResourcePageQuery) (readmodelcore.ResultPage, error)
type ListRunPatchesFunc func(context.Context, string, string, workorchquerycontract.RunResourcePageQuery) (readmodelcore.PatchPage, error)
type ListRunValidationResultsFunc func(context.Context, string, string, workorchquerycontract.RunResourcePageQuery) (readmodelcore.ValidationResultPage, error)
type ListRunArtifactsFunc func(context.Context, string, string, workorchquerycontract.RunResourcePageQuery) (readmodelcore.ArtifactPage, error)
type ReadRunStatusFunc func(context.Context, string, string, string) (projectioncontract.RunStatus, error)
type ReadRunObservabilityFunc func(context.Context, string, string, string) (readmodelcore.RunObservabilityState, error)
type ReadArtifactContentFunc func(context.Context, string, string, string) (content.ArtifactContent, error)
type ReadRuntimeRawContentFunc func(context.Context, string, string, string) (content.RuntimeRawContent, error)
type ReadRunTaskProgressFunc func(context.Context, string, string, string) (projectioncontract.TaskProgress, error)
type ReadRunContextBundleFunc func(context.Context, string, string, string) (projectioncontract.ContextBundle, error)
type ReadRunTaskDecompositionsFunc func(context.Context, string, string, string) ([]projectioncontract.TaskDecomposition, error)
type ListMemoryRecallTracesFunc func(context.Context, string, string, workorchquerycontract.MemoryRecallTraceQuery) ([]projectioncontract.MemoryRecallTrace, error)
type ListEventRecordsFunc func(context.Context, string, string, workorchquerycontract.EventRecordQuery) ([]projectioncontract.EventRecord, error)
type ListRunInstancesFunc func(context.Context, string, string, string) ([]appmodelcore.Run, error)
type ListDecisionsFunc func(context.Context, string, string, string, string) ([]projectioncontract.Decision, error)
type ListDecisionPageFunc func(context.Context, string, string, workorchquerycontract.RunResourcePageQuery) (appmodelcore.DecisionPage, error)
type ReadDecisionFunc func(context.Context, string, string, string) (projectioncontract.Decision, error)
type ResolveDecisionFunc func(context.Context, string, string, string, appmodelcore.ResolveDecisionCommand) (projectioncontract.Decision, error)
type BatchResolveDecisionsFunc func(context.Context, string, string, appmodelcore.BatchResolveDecisionCommand) ([]projectioncontract.Decision, error)
type ListApprovalsFunc func(context.Context, string, string, string, string) ([]projectioncontract.Approval, error)
type ListApprovalPageFunc func(context.Context, string, string, workorchquerycontract.RunResourcePageQuery) (appmodelcore.ApprovalPage, error)
type ReadApprovalFunc func(context.Context, string, string, string) (projectioncontract.Approval, error)
type ApproveApprovalFunc func(context.Context, string, string, string, appmodelcore.ResolveApprovalCommand) (projectioncontract.Approval, error)
type RejectApprovalFunc func(context.Context, string, string, string, appmodelcore.ResolveApprovalCommand) (projectioncontract.Approval, error)
type BatchResolveApprovalsFunc func(context.Context, string, string, appmodelcore.BatchResolveApprovalCommand) ([]projectioncontract.Approval, error)
type CreateSoulFunc func(context.Context, string, string, workorchmanagementcontract.SoulMutation) (workorchmanagementcontract.Soul, error)
type ListBuiltinDefaultsFunc func(context.Context, string, string) ([]workorchmanagementcontract.BuiltinDefault, error)
type UpgradeBuiltinDefaultFunc func(context.Context, string, string, workorchmanagementcontract.BuiltinDefaultUpgrade) (workorchmanagementcontract.BuiltinDefaultUpgradeResult, error)
type UpdateSoulFunc func(context.Context, string, string, string, workorchmanagementcontract.SoulMutation) (workorchmanagementcontract.Soul, error)
type DeleteSoulFunc func(context.Context, string, string, string) error
type ReadSoulFunc func(context.Context, string, string, string) (workorchmanagementcontract.Soul, error)
type ListSoulsFunc func(context.Context, string, string) ([]workorchmanagementcontract.Soul, error)
type CreateWorkspaceTemplateFunc func(context.Context, string, string, workorchmanagementcontract.WorkspaceTemplate) (workorchmanagementcontract.WorkspaceTemplate, error)
type UpdateWorkspaceTemplateFunc func(context.Context, string, string, string, workorchmanagementcontract.WorkspaceTemplate) (workorchmanagementcontract.WorkspaceTemplate, error)
type DeleteWorkspaceTemplateFunc func(context.Context, string, string, string) error
type ListWorkspaceTemplatesFunc func(context.Context, string, string) ([]workorchmanagementcontract.WorkspaceTemplate, error)
type CreateSkillFunc func(context.Context, string, string, workorchmanagementcontract.SkillMutation) (workorchmanagementcontract.Skill, error)
type ReadSkillFunc func(context.Context, string, string, string, contextregistrycore.SkillQuery) (workorchmanagementcontract.Skill, error)
type ListSkillsFunc func(context.Context, string, string, contextregistrycore.SkillQuery) ([]workorchmanagementcontract.Skill, error)
type CreateMCPServerFunc func(context.Context, string, string, workorchmanagementcontract.MCPServerMutation) (workorchmanagementcontract.MCPServer, error)
type UpdateMCPServerFunc func(context.Context, string, string, string, workorchmanagementcontract.MCPServerMutation) (workorchmanagementcontract.MCPServer, error)
type ReadMCPServerFunc func(context.Context, string, string, string) (workorchmanagementcontract.MCPServer, error)
type ListMCPServersFunc func(context.Context, string, string) ([]workorchmanagementcontract.MCPServer, error)
type ListMCPServerToolsFunc func(context.Context, string, string, string) ([]workorchmanagementcontract.MCPTool, error)
type ListMCPServerResourcesFunc func(context.Context, string, string, string) ([]workorchmanagementcontract.MCPResource, error)
type ListMCPServerPromptsFunc func(context.Context, string, string, string) ([]workorchmanagementcontract.MCPPrompt, error)
type CreateMCPCapabilityFunc func(context.Context, string, string, workorchmanagementcontract.MCPCapabilityMutation) (workorchmanagementcontract.MCPCapability, error)
type UpdateMCPCapabilityFunc func(context.Context, string, string, string, workorchmanagementcontract.MCPCapabilityMutation) (workorchmanagementcontract.MCPCapability, error)
type ReadMCPCapabilityFunc func(context.Context, string, string, string) (workorchmanagementcontract.MCPCapability, error)
type ListMCPCapabilitiesFunc func(context.Context, string, string, contextregistrycore.MCPCapabilityQuery) ([]workorchmanagementcontract.MCPCapability, error)
type CreateMemoryItemFunc func(context.Context, string, string, string, workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error)
type ReadMemoryItemFunc func(context.Context, string, string, string) (workorchmanagementcontract.MemoryItem, error)
type UpdateMemoryItemFunc func(context.Context, string, string, string, workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error)
type DeleteMemoryItemFunc func(context.Context, string, string, string) error
type ListMemoryByWorkspaceFunc func(context.Context, string, string, string, string) ([]workorchmanagementcontract.MemoryItem, error)
type SearchMemoryFunc func(context.Context, string, string, workorchquerycontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error)
type ValidateFunc func(context.Context, string, string) error
type ReadConfigurationFunc func() (workorchconfigurationcontract.ServiceConfiguration, error)
type UpdateServerConfigurationFunc func(configcore.UpdateServerConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error)
type UpdateRuntimeConfigurationFunc func(configcore.UpdateRuntimeConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error)
type UpdateLLMConfigurationFunc func(configcore.UpdateLLMConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error)
type UpdateWebSearchConfigurationFunc func(configcore.UpdateWebSearchConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error)

type GetLLMModelCatalogFunc func(context.Context) (workorchmanagementcontract.ProviderCatalogSummary, error)
type RefreshLLMModelCatalogFunc func(context.Context) (workorchmanagementcontract.ProviderCatalogSummary, error)
type ReadConfigurationWithCatalogFunc func(context.Context) (workorchconfigurationcontract.ServiceConfiguration, error)
