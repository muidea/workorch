package server

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
)

func defaultValidate(ctx context.Context, backend string, dataRoot string) error {
	session, err := appservice.NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return err
	}
	return session.Close(ctx)
}

func defaultCreateWorkspace(ctx context.Context, backend string, dataRoot string, cmd appservice.CreateWorkspaceCommand) (appservice.Workspace, error) {
	return appservice.CreateWorkspaceWithTemplateWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultDeleteWorkspace(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd appservice.DeleteWorkspaceCommand) error {
	return appservice.DeleteWorkspaceWithBackend(ctx, backend, dataRoot, workspaceID, cmd)
}

func defaultRefreshWorkspaceTemplateBinding(ctx context.Context, backend string, dataRoot string, workspaceID string) (appservice.Workspace, error) {
	return appservice.RefreshWorkspaceTemplateBindingWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultDefineGoal(ctx context.Context, backend string, dataRoot string, workspaceID string, text string) (appservice.Goal, error) {
	return appservice.DefineGoalWithBackend(ctx, backend, dataRoot, workspaceID, text)
}

func defaultUpdateGoal(ctx context.Context, backend string, dataRoot string, goalID string, text string) (appservice.Goal, error) {
	return appservice.UpdateGoalWithBackend(ctx, backend, dataRoot, goalID, text)
}

func defaultReadGoal(ctx context.Context, backend string, dataRoot string, goalID string) (appservice.Goal, error) {
	return appservice.ReadGoalWithBackend(ctx, backend, dataRoot, goalID)
}

func defaultListGoals(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]appservice.Goal, error) {
	return appservice.ListGoalsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultDefineDoD(ctx context.Context, backend string, dataRoot string, workspaceID string, items []string) (appservice.DoD, error) {
	return appservice.DefineDoDWithBackend(ctx, backend, dataRoot, workspaceID, items)
}

func defaultUpdateDoD(ctx context.Context, backend string, dataRoot string, dodID string, items []string) (appservice.DoD, error) {
	return appservice.UpdateDoDWithBackend(ctx, backend, dataRoot, dodID, items)
}

func defaultReadDoD(ctx context.Context, backend string, dataRoot string, dodID string) (appservice.DoD, error) {
	return appservice.ReadDoDWithBackend(ctx, backend, dataRoot, dodID)
}

func defaultListDoDs(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]appservice.DoD, error) {
	return appservice.ListDoDsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultCheckGoalDoDQuality(ctx context.Context, backend string, dataRoot string, cmd appservice.GoalDoDQualityCheckCommand) (appservice.GoalDoDQualityCheck, error) {
	return appservice.CheckGoalDoDQualityWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultCreateRun(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string, goalID string, dodID string) (appservice.Run, error) {
	return appservice.CreateRunWithBackend(ctx, backend, dataRoot, workspaceID, runID, goalID, dodID)
}

func defaultReadWorkspace(ctx context.Context, backend string, dataRoot string, workspaceID string) (appservice.Workspace, error) {
	return appservice.ReadWorkspaceWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadWorkspaceSnapshot(ctx context.Context, backend string, dataRoot string, workspaceID string) (appservice.WorkspaceSnapshot, error) {
	return appservice.ReadWorkspaceSnapshotWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadWorkspaceSoulBindings(ctx context.Context, backend string, dataRoot string, workspaceID string) (appservice.WorkspaceSoulBindings, error) {
	return appservice.ReadWorkspaceSoulBindingsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultUpdateWorkspaceSoulBindings(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd appservice.WorkspaceSoulBindings) (appservice.WorkspaceSoulBindings, error) {
	return appservice.UpdateWorkspaceSoulBindingsWithBackend(ctx, backend, dataRoot, workspaceID, cmd)
}

func defaultReadRun(ctx context.Context, backend string, dataRoot string, runID string) (appservice.Run, error) {
	return appservice.ReadRunWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadRunSnapshot(ctx context.Context, backend string, dataRoot string, runID string) (appservice.RunSnapshot, error) {
	return appservice.ReadRunSnapshotWithBackend(ctx, backend, dataRoot, runID)
}

func defaultListRunSteps(ctx context.Context, backend string, dataRoot string, query appservice.StepQuery) (appservice.StepPage, error) {
	return appservice.ListRunStepsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunToolCalls(ctx context.Context, backend string, dataRoot string, query appservice.ToolCallQuery) (appservice.ToolCallPage, error) {
	return appservice.ListRunToolCallsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunRuntimeCheckpoints(ctx context.Context, backend string, dataRoot string, query appservice.RuntimeCheckpointQuery) (appservice.RuntimeCheckpointPage, error) {
	return appservice.ListRunRuntimeCheckpointsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunResults(ctx context.Context, backend string, dataRoot string, query appservice.ResultQuery) (appservice.ResultPage, error) {
	return appservice.ListRunResultsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunPatches(ctx context.Context, backend string, dataRoot string, query appservice.PatchQuery) (appservice.PatchPage, error) {
	return appservice.ListRunPatchesWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunValidationResults(ctx context.Context, backend string, dataRoot string, query appservice.ValidationResultQuery) (appservice.ValidationResultPage, error) {
	return appservice.ListRunValidationResultsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListRunArtifacts(ctx context.Context, backend string, dataRoot string, query appservice.ArtifactQuery) (appservice.ArtifactPage, error) {
	return appservice.ListRunArtifactsWithBackend(ctx, backend, dataRoot, query)
}

func defaultReadRunStatus(ctx context.Context, backend string, dataRoot string, runID string) (appservice.RunStatus, error) {
	return appservice.ReadRunStatusWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadRunObservability(ctx context.Context, backend string, dataRoot string, runID string) (appservice.RunObservability, error) {
	return appservice.ReadRunObservabilityWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadArtifactContent(ctx context.Context, backend string, dataRoot string, artifactURI string) (appservice.ArtifactContent, error) {
	return appservice.ReadArtifactContentWithBackend(ctx, backend, dataRoot, artifactURI)
}

func defaultReadRuntimeRawContent(ctx context.Context, backend string, dataRoot string, ref string) (appservice.RuntimeRawContent, error) {
	return appservice.ReadRuntimeRawContentWithBackend(ctx, backend, dataRoot, ref)
}

func defaultReadRunTaskProgress(ctx context.Context, backend string, dataRoot string, runID string) (appservice.TaskProgress, error) {
	return appservice.ReadRunTaskProgressWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadWorkspaceTaskProgress(ctx context.Context, backend string, dataRoot string, workspaceID string) (appservice.TaskProgress, error) {
	return appservice.ReadWorkspaceTaskProgressWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadRunContextBundle(ctx context.Context, backend string, dataRoot string, runID string) (appservice.ContextBundle, error) {
	return appservice.ReadRunContextBundleWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadWorkspaceContextBundle(ctx context.Context, backend string, dataRoot string, workspaceID string) (appservice.ContextBundle, error) {
	return appservice.ReadWorkspaceContextBundleWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultReadRunTaskDecompositions(ctx context.Context, backend string, dataRoot string, runID string) ([]appservice.TaskDecomposition, error) {
	return appservice.ReadRunTaskDecompositionsWithBackend(ctx, backend, dataRoot, runID)
}

func defaultReadWorkspaceTaskDecompositions(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]appservice.TaskDecomposition, error) {
	return appservice.ReadWorkspaceTaskDecompositionsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultListEventRecords(ctx context.Context, backend string, dataRoot string, query appservice.EventRecordQuery) ([]appservice.EventRecord, error) {
	return appservice.ListEventRecordsWithBackend(ctx, backend, dataRoot, query)
}

func defaultListWorkspaces(ctx context.Context, backend string, dataRoot string) ([]appservice.Workspace, error) {
	return appservice.ListWorkspacesWithBackend(ctx, backend, dataRoot)
}

func defaultListRuns(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]appservice.Run, error) {
	return appservice.ListRunsWithBackend(ctx, backend, dataRoot, workspaceID)
}

func defaultStartRun(ctx context.Context, backend string, dataRoot string, runID string) (appservice.Run, error) {
	return appservice.StartRunWithBackend(ctx, backend, dataRoot, runID)
}

func defaultResumeRun(ctx context.Context, backend string, dataRoot string, runID string, executionContextPatch ...map[string]any) (appservice.Run, error) {
	return appservice.ResumeRunWithBackend(ctx, backend, dataRoot, runID, executionContextPatch...)
}

func defaultResumeWorkspaceExecution(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
	return appservice.ActivateWorkspaceExecutionWithBackend(ctx, backend, dataRoot, workspaceID, cmd)
}

func defaultListDecisions(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]appservice.Decision, error) {
	return appservice.ListDecisionsWithBackend(ctx, backend, dataRoot, workspaceID, runID)
}

func defaultListDecisionPage(ctx context.Context, backend string, dataRoot string, query appservice.DecisionQuery) (appservice.DecisionPage, error) {
	return appservice.ListDecisionPageWithBackend(ctx, backend, dataRoot, query)
}

func defaultReadDecision(ctx context.Context, backend string, dataRoot string, ticketID string) (appservice.Decision, error) {
	return appservice.ReadDecisionWithBackend(ctx, backend, dataRoot, ticketID)
}

func defaultResolveDecision(ctx context.Context, backend string, dataRoot string, ticketID string, cmd appservice.ResolveDecisionCommand) (appservice.Decision, error) {
	return appservice.ResolveDecisionWithBackend(ctx, backend, dataRoot, ticketID, cmd)
}

func defaultBatchResolveDecisions(ctx context.Context, backend string, dataRoot string, cmd appservice.BatchResolveDecisionCommand) ([]appservice.Decision, error) {
	return appservice.BatchResolveDecisionsWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultListApprovals(ctx context.Context, backend string, dataRoot string, workspaceID string, runID string) ([]appservice.Approval, error) {
	return appservice.ListApprovalsWithBackend(ctx, backend, dataRoot, workspaceID, runID)
}

func defaultListApprovalPage(ctx context.Context, backend string, dataRoot string, query appservice.ApprovalQuery) (appservice.ApprovalPage, error) {
	return appservice.ListApprovalPageWithBackend(ctx, backend, dataRoot, query)
}

func defaultReadApproval(ctx context.Context, backend string, dataRoot string, approvalID string) (appservice.Approval, error) {
	return appservice.ReadApprovalWithBackend(ctx, backend, dataRoot, approvalID)
}

func defaultApproveApproval(ctx context.Context, backend string, dataRoot string, approvalID string, cmd appservice.ResolveApprovalCommand) (appservice.Approval, error) {
	if cmd.Action == "" {
		cmd.Action = "approve"
	}
	return appservice.ResolveApprovalWithBackend(ctx, backend, dataRoot, approvalID, cmd)
}

func defaultRejectApproval(ctx context.Context, backend string, dataRoot string, approvalID string, cmd appservice.ResolveApprovalCommand) (appservice.Approval, error) {
	if cmd.Action == "" {
		cmd.Action = "reject"
	}
	return appservice.ResolveApprovalWithBackend(ctx, backend, dataRoot, approvalID, cmd)
}

func defaultBatchResolveApprovals(ctx context.Context, backend string, dataRoot string, cmd appservice.BatchResolveApprovalCommand) ([]appservice.Approval, error) {
	return appservice.BatchResolveApprovalsWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultCreateSoul(ctx context.Context, backend string, dataRoot string, cmd appservice.CreateSoulCommand) (appservice.Soul, error) {
	return appservice.CreateSoulWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultUpdateSoul(ctx context.Context, backend string, dataRoot string, soulID string, cmd appservice.UpdateSoulCommand) (appservice.Soul, error) {
	return appservice.UpdateSoulWithBackend(ctx, backend, dataRoot, soulID, cmd)
}

func defaultDeleteSoul(ctx context.Context, backend string, dataRoot string, soulID string) error {
	return appservice.DeleteSoulWithBackend(ctx, backend, dataRoot, soulID)
}

func defaultReadSoul(ctx context.Context, backend string, dataRoot string, soulID string) (appservice.Soul, error) {
	return appservice.ReadSoulWithBackend(ctx, backend, dataRoot, soulID)
}

func defaultListSouls(ctx context.Context, backend string, dataRoot string) ([]appservice.Soul, error) {
	return appservice.ListSoulsWithBackend(ctx, backend, dataRoot)
}

func defaultCreateWorkspaceTemplate(ctx context.Context, backend string, dataRoot string, cmd appservice.WorkspaceTemplate) (appservice.WorkspaceTemplate, error) {
	return appservice.CreateWorkspaceTemplateWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultUpdateWorkspaceTemplate(ctx context.Context, backend string, dataRoot string, templateID string, cmd appservice.WorkspaceTemplate) (appservice.WorkspaceTemplate, error) {
	return appservice.UpdateWorkspaceTemplateWithBackend(ctx, backend, dataRoot, templateID, cmd)
}

func defaultDeleteWorkspaceTemplate(ctx context.Context, backend string, dataRoot string, templateID string) error {
	return appservice.DeleteWorkspaceTemplateWithBackend(ctx, backend, dataRoot, templateID)
}

func defaultListWorkspaceTemplates(ctx context.Context, backend string, dataRoot string) ([]appservice.WorkspaceTemplate, error) {
	return appservice.ListWorkspaceTemplatesWithBackend(ctx, backend, dataRoot)
}

func defaultCreateSkill(ctx context.Context, backend string, dataRoot string, cmd appservice.CreateSkillCommand) (appservice.Skill, error) {
	return appservice.CreateSkillWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultReadSkill(ctx context.Context, backend string, dataRoot string, skillID string, query appservice.SkillQuery) (appservice.Skill, error) {
	return appservice.ReadSkillWithBackend(ctx, backend, dataRoot, skillID, query)
}

func defaultListSkills(ctx context.Context, backend string, dataRoot string, query appservice.SkillQuery) ([]appservice.Skill, error) {
	return appservice.ListSkillsWithBackend(ctx, backend, dataRoot, query)
}

func defaultCreateMCPServer(ctx context.Context, backend string, dataRoot string, cmd appservice.CreateMCPServerCommand) (appservice.MCPServer, error) {
	return appservice.CreateMCPServerWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultUpdateMCPServer(ctx context.Context, backend string, dataRoot string, serverID string, cmd appservice.UpdateMCPServerCommand) (appservice.MCPServer, error) {
	return appservice.UpdateMCPServerWithBackend(ctx, backend, dataRoot, serverID, cmd)
}

func defaultReadMCPServer(ctx context.Context, backend string, dataRoot string, serverID string) (appservice.MCPServer, error) {
	return appservice.ReadMCPServerWithBackend(ctx, backend, dataRoot, serverID)
}

func defaultListMCPServers(ctx context.Context, backend string, dataRoot string) ([]appservice.MCPServer, error) {
	return appservice.ListMCPServersWithBackend(ctx, backend, dataRoot)
}

func defaultListMCPServerTools(ctx context.Context, backend string, dataRoot string, serverID string) ([]appservice.MCPTool, error) {
	return appservice.ListMCPServerToolsWithBackend(ctx, backend, dataRoot, serverID)
}

func defaultListMCPServerResources(ctx context.Context, backend string, dataRoot string, serverID string) ([]appservice.MCPResource, error) {
	return appservice.ListMCPServerResourcesWithBackend(ctx, backend, dataRoot, serverID)
}

func defaultListMCPServerPrompts(ctx context.Context, backend string, dataRoot string, serverID string) ([]appservice.MCPPrompt, error) {
	return appservice.ListMCPServerPromptsWithBackend(ctx, backend, dataRoot, serverID)
}

func defaultCreateMCPCapability(ctx context.Context, backend string, dataRoot string, cmd appservice.CreateMCPCapabilityCommand) (appservice.MCPCapability, error) {
	return appservice.CreateMCPCapabilityWithBackend(ctx, backend, dataRoot, cmd)
}

func defaultUpdateMCPCapability(ctx context.Context, backend string, dataRoot string, capabilityID string, cmd appservice.UpdateMCPCapabilityCommand) (appservice.MCPCapability, error) {
	return appservice.UpdateMCPCapabilityWithBackend(ctx, backend, dataRoot, capabilityID, cmd)
}

func defaultReadMCPCapability(ctx context.Context, backend string, dataRoot string, capabilityID string) (appservice.MCPCapability, error) {
	return appservice.ReadMCPCapabilityWithBackend(ctx, backend, dataRoot, capabilityID)
}

func defaultListMCPCapabilities(ctx context.Context, backend string, dataRoot string, query appservice.MCPCapabilityQuery) ([]appservice.MCPCapability, error) {
	return appservice.ListMCPCapabilitiesWithBackend(ctx, backend, dataRoot, query)
}

func defaultCreateMemoryItem(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd appservice.CreateMemoryItemCommand) (appservice.MemoryItem, error) {
	return appservice.CreateMemoryItemWithBackend(ctx, backend, dataRoot, workspaceID, cmd)
}

func defaultReadMemoryItem(ctx context.Context, backend string, dataRoot string, memoryID string) (appservice.MemoryItem, error) {
	return appservice.ReadMemoryItemWithBackend(ctx, backend, dataRoot, memoryID)
}

func defaultUpdateMemoryItem(ctx context.Context, backend string, dataRoot string, memoryID string, cmd appservice.UpdateMemoryItemCommand) (appservice.MemoryItem, error) {
	return appservice.UpdateMemoryItemWithBackend(ctx, backend, dataRoot, memoryID, cmd)
}

func defaultDeleteMemoryItem(ctx context.Context, backend string, dataRoot string, memoryID string) error {
	return appservice.DeleteMemoryItemWithBackend(ctx, backend, dataRoot, memoryID)
}

func defaultListMemoryByWorkspace(ctx context.Context, backend string, dataRoot string, workspaceID string, query string) ([]appservice.MemoryItem, error) {
	return appservice.ListMemoryByWorkspaceWithBackend(ctx, backend, dataRoot, workspaceID, query)
}

func defaultSearchMemory(ctx context.Context, backend string, dataRoot string, query appservice.MemoryQuery) ([]appservice.MemoryItem, error) {
	return appservice.SearchMemoryWithBackend(ctx, backend, dataRoot, query)
}

func defaultListMemoryRecallTraces(ctx context.Context, backend string, dataRoot string, query appservice.MemoryRecallTraceQuery) ([]appservice.MemoryRecallTrace, error) {
	return appservice.ListMemoryRecallTracesWithBackend(ctx, backend, dataRoot, query)
}

func defaultReadConfiguration() (appservice.ServiceConfiguration, error) {
	return appservice.ReadServiceConfiguration()
}
