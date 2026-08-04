package server

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contentcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/qualitycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func defaultValidate(ctx context.Context, backend string, dataRoot string) error {
	session, err := appservice.NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return err
	}
	return session.Close(ctx)
}

func defaultCreateWorkspace(ctx context.Context, backend string, dataRoot string, cmd appmodelcore.CreateWorkspaceCommand) (appmodelcore.Workspace, error) {
	return appservice.CreateWorkspaceWithTemplateWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultDeleteWorkspace(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd appmodelcore.DeleteWorkspaceCommand) error {
	return appservice.DeleteWorkspaceWithBackend(ctx, backend, dataRoot, workspaceID, cmd)
}

func defaultDefineGoal(ctx context.Context, backend string, dataRoot string, workspaceID string, text string) (workorchmanagementcontract.Goal, error) {
	return appservice.DefineGoalWithBackend(ctx, backend, dataRoot, workspaceID, text)
}

func defaultUpdateGoal(ctx context.Context, backend string, dataRoot string, goalID string, text string) (workorchmanagementcontract.Goal, error) {
	return appservice.UpdateGoalWithBackend(ctx, backend, dataRoot, goalID, text)
}

func defaultReadGoal(ctx context.Context, backend string, dataRoot string, goalID string) (workorchmanagementcontract.Goal, error) {
	return appservice.ReadGoalWithBackend(ctx, backend, dataRoot, goalID)
}

func defaultListGoals(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]workorchmanagementcontract.Goal, error) {
	return appservice.ListGoalsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultDefineDoD(ctx context.Context, backend string, dataRoot string, workspaceID string, items []string) (workorchmanagementcontract.DoD, error) {
	return appservice.DefineDoDWithBackend(ctx, backend, dataRoot, workspaceID, items)
}

func defaultUpdateDoD(ctx context.Context, backend string, dataRoot string, dodID string, items []string) (workorchmanagementcontract.DoD, error) {
	return appservice.UpdateDoDWithBackend(ctx, backend, dataRoot, dodID, items)
}

func defaultReadDoD(ctx context.Context, backend string, dataRoot string, dodID string) (workorchmanagementcontract.DoD, error) {
	return appservice.ReadDoDWithBackend(ctx, backend, dataRoot, dodID)
}

func defaultListDoDs(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]workorchmanagementcontract.DoD, error) {
	return appservice.ListDoDsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultCheckGoalDoDQuality(ctx context.Context, backend string, dataRoot string, cmd qualitycore.Command) (qualitycore.Check, error) {
	return appservice.CheckGoalDoDQualityWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultReadWorkspace(ctx context.Context, backend string, dataRoot string, workspaceID string) (appmodelcore.Workspace, error) {
	return appservice.ReadWorkspaceWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadWorkspaceSnapshot(ctx context.Context, backend string, dataRoot string, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	return appservice.ReadWorkspaceSnapshotWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadWorkspaceDetailSnapshot(ctx context.Context, backend string, dataRoot string, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	return appservice.ReadWorkspaceDetailSnapshotWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadWorkspaceProjectionSnapshot(ctx context.Context, backend string, dataRoot string, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	return appservice.ReadWorkspaceProjectionSnapshotWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadWorkspaceSoulBindings(ctx context.Context, backend string, dataRoot string, workspaceID string) (appmodelcore.WorkspaceSoulBindings, error) {
	return appservice.ReadWorkspaceSoulBindingsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultUpdateWorkspaceSoulBindings(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd appmodelcore.WorkspaceSoulBindings) (appmodelcore.WorkspaceSoulBindings, error) {
	return appservice.UpdateWorkspaceSoulBindingsWithBackend(ctx, backend, dataRoot, workspaceID, cmd)
}

func defaultReadRun(ctx context.Context, backend string, dataRoot string, runID string) (appmodelcore.Run, error) {
	return appservice.ReadRunWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadRunSnapshot(ctx context.Context, backend string, dataRoot string, runID string) (readmodelcore.RunSnapshot, error) {
	return appservice.ReadRunSnapshotWithBackend(ctx, backend, dataRoot, runID)
}

func defaultListRunSteps(ctx context.Context, backend string, dataRoot string, query readmodelcore.StepQuery) (readmodelcore.StepPage, error) {
	return appservice.ListRunStepsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunToolCalls(ctx context.Context, backend string, dataRoot string, query readmodelcore.ToolCallQuery) (readmodelcore.ToolCallPage, error) {
	return appservice.ListRunToolCallsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunRuntimeCheckpoints(ctx context.Context, backend string, dataRoot string, query readmodelcore.RuntimeCheckpointQuery) (readmodelcore.RuntimeCheckpointPage, error) {
	return appservice.ListRunRuntimeCheckpointsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunResults(ctx context.Context, backend string, dataRoot string, query readmodelcore.ResultQuery) (readmodelcore.ResultPage, error) {
	return appservice.ListRunResultsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunPatches(ctx context.Context, backend string, dataRoot string, query readmodelcore.PatchQuery) (readmodelcore.PatchPage, error) {
	return appservice.ListRunPatchesWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunValidationResults(ctx context.Context, backend string, dataRoot string, query readmodelcore.ValidationResultQuery) (readmodelcore.ValidationResultPage, error) {
	return appservice.ListRunValidationResultsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunArtifacts(ctx context.Context, backend string, dataRoot string, query readmodelcore.ArtifactQuery) (readmodelcore.ArtifactPage, error) {
	return appservice.ListRunArtifactsWithBackend(ctx, backend, dataRoot, query)
}

func defaultReadRunStatus(ctx context.Context, backend string, dataRoot string, runID string) (projectioncontract.RunStatus, error) {
	return appservice.ReadRunStatusWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadRunObservability(ctx context.Context, backend string, dataRoot string, runID string) (readmodelcore.RunObservability, error) {
	return appservice.ReadRunObservabilityWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadArtifactContent(ctx context.Context, backend string, dataRoot string, artifactURI string) (contentcore.ArtifactContent, error) {
	return appservice.ReadArtifactContentWithBackend(ctx, backend, dataRoot, artifactURI)
}

func defaultReadRuntimeRawContent(ctx context.Context, backend string, dataRoot string, ref string) (contentcore.RuntimeRawContent, error) {
	return appservice.ReadRuntimeRawContentWithBackend(ctx, backend, dataRoot, ref)
}

func defaultReadRunTaskProgress(ctx context.Context, backend string, dataRoot string, runID string) (projectioncontract.TaskProgress, error) {
	return appservice.ReadRunTaskProgressWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadWorkspaceTaskProgress(ctx context.Context, backend string, dataRoot string, workspaceID string) (projectioncontract.TaskProgress, error) {
	return appservice.ReadWorkspaceTaskProgressWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadRunContextBundle(ctx context.Context, backend string, dataRoot string, runID string) (readmodelcore.ContextBundle, error) {
	return appservice.ReadRunContextBundleWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadWorkspaceContextBundle(ctx context.Context, backend string, dataRoot string, workspaceID string) (readmodelcore.ContextBundle, error) {
	return appservice.ReadWorkspaceContextBundleWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadRunTaskDecompositions(ctx context.Context, backend string, dataRoot string, runID string) ([]projectioncontract.TaskDecomposition, error) {
	return appservice.ReadRunTaskDecompositionsWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadWorkspaceTaskDecompositions(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]projectioncontract.TaskDecomposition, error) {
	return appservice.ReadWorkspaceTaskDecompositionsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultListEventRecords(ctx context.Context, backend string, dataRoot string, query readmodelcore.EventRecordQuery) ([]projectioncontract.EventRecord, error) {
	return appservice.ListEventRecordsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListWorkspaces(ctx context.Context, backend string, dataRoot string) ([]appmodelcore.Workspace, error) {
	return appservice.ListWorkspacesWithBackend(ctx, backend, dataRoot)
}

func defaultListRuns(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]appmodelcore.Run, error) {
	return appservice.ListRunsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultListDecisions(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]projectioncontract.Decision, error) {
	return appservice.ListDecisionsWithBackend(ctx, backend, dataRoot, workspaceID, runID)
}

func defaultListDecisionPage(ctx context.Context, backend string, dataRoot string, query appmodelcore.DecisionQuery) (appmodelcore.DecisionPage, error) {
	return appservice.ListDecisionPageWithBackend(ctx, backend, dataRoot, query)
}

func defaultReadDecision(ctx context.Context, backend string, dataRoot string, ticketID string) (projectioncontract.Decision, error) {
	return appservice.ReadDecisionWithBackend(ctx, backend, dataRoot, ticketID)
}

func defaultResolveDecision(ctx context.Context, backend string, dataRoot string, ticketID string, cmd appmodelcore.ResolveDecisionCommand) (projectioncontract.Decision, error) {
	return appservice.ResolveDecisionWithBackend(ctx, backend, dataRoot, ticketID, cmd)
}

func defaultBatchResolveDecisions(ctx context.Context, backend string, dataRoot string, cmd appmodelcore.BatchResolveDecisionCommand) ([]projectioncontract.Decision, error) {
	return appservice.BatchResolveDecisionsWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultListApprovals(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]projectioncontract.Approval, error) {
	return appservice.ListApprovalsWithBackend(ctx, backend, dataRoot, workspaceID, runID)
}

func defaultListApprovalPage(ctx context.Context, backend string, dataRoot string, query appmodelcore.ApprovalQuery) (appmodelcore.ApprovalPage, error) {
	return appservice.ListApprovalPageWithBackend(ctx, backend, dataRoot, query)
}

func defaultReadApproval(ctx context.Context, backend string, dataRoot string, approvalID string) (projectioncontract.Approval, error) {
	return appservice.ReadApprovalWithBackend(ctx, backend, dataRoot, approvalID)
}

func defaultApproveApproval(ctx context.Context, backend string, dataRoot string, approvalID string, cmd appmodelcore.ResolveApprovalCommand) (projectioncontract.Approval, error) {
	if cmd.Action == "" {
		cmd.Action = "approve"
	}
	return appservice.ResolveApprovalWithBackend(ctx, backend, dataRoot, approvalID, cmd)
}

func defaultRejectApproval(ctx context.Context, backend string, dataRoot string, approvalID string, cmd appmodelcore.ResolveApprovalCommand) (projectioncontract.Approval, error) {
	if cmd.Action == "" {
		cmd.Action = "reject"
	}
	return appservice.ResolveApprovalWithBackend(ctx, backend, dataRoot, approvalID, cmd)
}

func defaultBatchResolveApprovals(ctx context.Context, backend string, dataRoot string, cmd appmodelcore.BatchResolveApprovalCommand) ([]projectioncontract.Approval, error) {
	return appservice.BatchResolveApprovalsWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultCreateSoul(ctx context.Context, backend string, dataRoot string, cmd workorchmanagementcontract.Soul) (workorchmanagementcontract.Soul, error) {
	return appservice.CreateSoulWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultUpdateSoul(ctx context.Context, backend string, dataRoot string, soulID string, cmd workorchmanagementcontract.Soul) (workorchmanagementcontract.Soul, error) {
	return appservice.UpdateSoulWithBackend(ctx, backend, dataRoot, soulID, cmd)
}

func defaultDeleteSoul(ctx context.Context, backend string, dataRoot string, soulID string) error {
	return appservice.DeleteSoulWithBackend(ctx, backend, dataRoot, soulID)
}

func defaultReadSoul(ctx context.Context, backend string, dataRoot string, soulID string) (workorchmanagementcontract.Soul, error) {
	return appservice.ReadSoulWithBackend(ctx, backend, dataRoot, soulID)
}

func defaultListSouls(ctx context.Context, backend string, dataRoot string) ([]workorchmanagementcontract.Soul, error) {
	return appservice.ListSoulsWithBackend(ctx, backend, dataRoot)
}

func defaultCreateWorkspaceTemplate(ctx context.Context, backend string, dataRoot string, cmd workorchmanagementcontract.WorkspaceTemplate) (workorchmanagementcontract.WorkspaceTemplate, error) {
	return appservice.CreateWorkspaceTemplateWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultUpdateWorkspaceTemplate(ctx context.Context, backend string, dataRoot string, templateID string, cmd workorchmanagementcontract.WorkspaceTemplate) (workorchmanagementcontract.WorkspaceTemplate, error) {
	return appservice.UpdateWorkspaceTemplateWithBackend(ctx, backend, dataRoot, templateID, cmd)
}

func defaultDeleteWorkspaceTemplate(ctx context.Context, backend string, dataRoot string, templateID string) error {
	return appservice.DeleteWorkspaceTemplateWithBackend(ctx, backend, dataRoot, templateID)
}

func defaultListWorkspaceTemplates(ctx context.Context, backend string, dataRoot string) ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	return appservice.ListWorkspaceTemplatesWithBackend(ctx, backend, dataRoot)
}

func defaultCreateSkill(ctx context.Context, backend string, dataRoot string, cmd workorchmanagementcontract.Skill) (workorchmanagementcontract.Skill, error) {
	return appservice.CreateSkillWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultReadSkill(ctx context.Context, backend string, dataRoot string, skillID string, query contextregistrycore.SkillQuery) (workorchmanagementcontract.Skill, error) {
	return appservice.ReadSkillWithBackend(ctx, backend, dataRoot, skillID, query)
}

func defaultListSkills(ctx context.Context, backend string, dataRoot string, query contextregistrycore.SkillQuery) ([]workorchmanagementcontract.Skill, error) {
	return appservice.ListSkillsWithBackend(ctx, backend, dataRoot, query)
}

func defaultCreateMCPServer(ctx context.Context, backend string, dataRoot string, cmd workorchmanagementcontract.MCPServer) (workorchmanagementcontract.MCPServer, error) {
	return appservice.CreateMCPServerWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultUpdateMCPServer(ctx context.Context, backend string, dataRoot string, serverID string, cmd workorchmanagementcontract.MCPServer) (workorchmanagementcontract.MCPServer, error) {
	return appservice.UpdateMCPServerWithBackend(ctx, backend, dataRoot, serverID, cmd)
}

func defaultReadMCPServer(ctx context.Context, backend string, dataRoot string, serverID string) (workorchmanagementcontract.MCPServer, error) {
	return appservice.ReadMCPServerWithBackend(ctx, backend, dataRoot, serverID)
}

func defaultListMCPServers(ctx context.Context, backend string, dataRoot string) ([]workorchmanagementcontract.MCPServer, error) {
	return appservice.ListMCPServersWithBackend(ctx, backend, dataRoot)
}

func defaultListMCPServerTools(ctx context.Context, backend string, dataRoot string, serverID string) ([]workorchmanagementcontract.MCPTool, error) {
	return appservice.ListMCPServerToolsWithBackend(ctx, backend, dataRoot, serverID)
}

func defaultListMCPServerResources(ctx context.Context, backend string, dataRoot string, serverID string) ([]workorchmanagementcontract.MCPResource, error) {
	return appservice.ListMCPServerResourcesWithBackend(ctx, backend, dataRoot, serverID)
}

func defaultListMCPServerPrompts(ctx context.Context, backend string, dataRoot string, serverID string) ([]workorchmanagementcontract.MCPPrompt, error) {
	return appservice.ListMCPServerPromptsWithBackend(ctx, backend, dataRoot, serverID)
}

func defaultCreateMCPCapability(ctx context.Context, backend string, dataRoot string, cmd workorchmanagementcontract.MCPCapability) (workorchmanagementcontract.MCPCapability, error) {
	return appservice.CreateMCPCapabilityWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultUpdateMCPCapability(ctx context.Context, backend string, dataRoot string, capabilityID string, cmd workorchmanagementcontract.MCPCapability) (workorchmanagementcontract.MCPCapability, error) {
	return appservice.UpdateMCPCapabilityWithBackend(ctx, backend, dataRoot, capabilityID, cmd)
}

func defaultReadMCPCapability(ctx context.Context, backend string, dataRoot string, capabilityID string) (workorchmanagementcontract.MCPCapability, error) {
	return appservice.ReadMCPCapabilityWithBackend(ctx, backend, dataRoot, capabilityID)
}

func defaultListMCPCapabilities(ctx context.Context, backend string, dataRoot string, query contextregistrycore.MCPCapabilityQuery) ([]workorchmanagementcontract.MCPCapability, error) {
	return appservice.ListMCPCapabilitiesWithBackend(ctx, backend, dataRoot, query)
}

func defaultCreateMemoryItem(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error) {
	return appservice.CreateMemoryItemWithBackend(ctx, backend, dataRoot, workspaceID, cmd)
}

func defaultReadMemoryItem(ctx context.Context, backend string, dataRoot string, memoryID string) (workorchmanagementcontract.MemoryItem, error) {
	return appservice.ReadMemoryItemWithBackend(ctx, backend, dataRoot, memoryID)
}

func defaultUpdateMemoryItem(ctx context.Context, backend string, dataRoot string, memoryID string, cmd workorchmanagementcontract.MemoryMutation) (workorchmanagementcontract.MemoryItem, error) {
	return appservice.UpdateMemoryItemWithBackend(ctx, backend, dataRoot, memoryID, cmd)
}

func defaultDeleteMemoryItem(ctx context.Context, backend string, dataRoot string, memoryID string) error {
	return appservice.DeleteMemoryItemWithBackend(ctx, backend, dataRoot, memoryID)
}

func defaultListMemoryByWorkspace(ctx context.Context, backend string, dataRoot string, workspaceID string, query string) ([]workorchmanagementcontract.MemoryItem, error) {
	return appservice.ListMemoryByWorkspaceWithBackend(ctx, backend, dataRoot, workspaceID, query)
}

func defaultSearchMemory(ctx context.Context, backend string, dataRoot string, query workorchmanagementcontract.MemorySearchQuery) ([]workorchmanagementcontract.MemoryItem, error) {
	return appservice.SearchMemoryWithBackend(ctx, backend, dataRoot, query)
}

func defaultListMemoryRecallTraces(ctx context.Context, backend string, dataRoot string, query readmodelcore.MemoryRecallTraceQuery) ([]projectioncontract.MemoryRecallTrace, error) {
	return appservice.ListMemoryRecallTracesWithBackend(ctx, backend, dataRoot, query)
}

func defaultReadConfiguration() (workorchconfigurationcontract.ServiceConfiguration, error) {
	return appservice.ReadServiceConfiguration()
}
