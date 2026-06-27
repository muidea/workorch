package backend

import port "github.com/muidea/workorch/internal/pkg/workorchport"

type Backend = port.Backend
type CreateWorkspaceRequest = port.CreateWorkspaceRequest
type WorkspaceInfo = port.WorkspaceInfo
type ProjectionInfo = port.ProjectionInfo
type WorkspaceViewModelInfo = port.WorkspaceViewModelInfo
type WorkspaceViewModelSectionInfo = port.WorkspaceViewModelSectionInfo
type WorkspaceOutcomeInfo = port.WorkspaceOutcomeInfo
type ResultSummaryInfo = port.ResultSummaryInfo
type ResultInfo = port.ResultInfo
type ArtifactInfo = port.ArtifactInfo
type ArtifactContinuationInfo = port.ArtifactContinuationInfo
type PatchInfo = port.PatchInfo
type MemoryRecallHitInfo = port.MemoryRecallHitInfo
type MemoryRecallTraceInfo = port.MemoryRecallTraceInfo
type WebSourceTraceInfo = port.WebSourceTraceInfo
type LLMUsageTraceInfo = port.LLMUsageTraceInfo
type LLMUsageSummaryInfo = port.LLMUsageSummaryInfo
type LLMUsageProviderModelBreakdownInfo = port.LLMUsageProviderModelBreakdownInfo
type WebSourceInfo = port.WebSourceInfo
type FetchedWebSourceInfo = port.FetchedWebSourceInfo
type WorkspaceSnapshotInfo = port.WorkspaceSnapshotInfo
type GapBaselineInfo = port.GapBaselineInfo
type GapTodoItemInfo = port.GapTodoItemInfo
type GapNextActionInfo = port.GapNextActionInfo
type EventRecordInfo = port.EventRecordInfo
type RecoveryPlanInfo = port.RecoveryPlanInfo
type RepairLoopInfo = port.RepairLoopInfo
type TaskProgressInfo = port.TaskProgressInfo
type TaskDecompositionNodeInfo = port.TaskDecompositionNodeInfo
type TaskDecompositionInfo = port.TaskDecompositionInfo
type TaskPlanInfo = port.TaskPlanInfo
type WorkspaceBackend = port.WorkspaceBackend
type GoalInfo = port.GoalInfo
type GoalBackend = port.GoalBackend
type DoDInfo = port.DoDInfo
type DoDBackend = port.DoDBackend
type RunInfo = port.RunInfo
type ActivateRunRequest = port.ActivateRunRequest
type RunCommandIntent = port.RunCommandIntent
type WorkspaceRunCommand = port.WorkspaceRunCommand

const (
	RunCommandIntentResume  = port.RunCommandIntentResume
	RunCommandIntentRestart = port.RunCommandIntentRestart
	RunCommandIntentReplan  = port.RunCommandIntentReplan
	RunCommandIntentRecover = port.RunCommandIntentRecover
)

func NormalizeRunCommandIntent(intent RunCommandIntent) (RunCommandIntent, error) {
	return port.NormalizeRunCommandIntent(intent)
}

type RunStatusInfo = port.RunStatusInfo
type RuntimeExecutionInfo = port.RuntimeExecutionInfo
type ConvergenceStatusInfo = port.ConvergenceStatusInfo
type ContinuationDiagnosticInfo = port.ContinuationDiagnosticInfo
type ContinuationReadbackDiagnosticInfo = port.ContinuationReadbackDiagnosticInfo
type ContinuationDiagnosticCountInfo = port.ContinuationDiagnosticCountInfo
type WatchdogDiagnosticsInfo = port.WatchdogDiagnosticsInfo
type SchedulerDiagnosticsInfo = port.SchedulerDiagnosticsInfo
type RunBackend = port.RunBackend
type DecisionInfo = port.DecisionInfo
type ResolveDecisionRequest = port.ResolveDecisionRequest
type DecisionBackend = port.DecisionBackend
type ApprovalInfo = port.ApprovalInfo
type AutoGovernanceSummaryInfo = port.AutoGovernanceSummaryInfo
type ApprovalBackend = port.ApprovalBackend
type SoulInfo = port.SoulInfo
type SoulBackend = port.SoulBackend
type SkillInfo = port.SkillInfo
type SkillBackend = port.SkillBackend
type MCPServerInfo = port.MCPServerInfo
type MCPCapabilityInfo = port.MCPCapabilityInfo
type MCPBackend = port.MCPBackend
type MemoryItemInfo = port.MemoryItemInfo
type MemorySearchRequest = port.MemorySearchRequest
type MemoryBackend = port.MemoryBackend
type TemplateInfo = port.TemplateInfo
type TemplateBackend = port.TemplateBackend
type LLMProvider = port.LLMProvider
type LLMOutputConfig = port.LLMOutputConfig
type ServiceConfig = port.ServiceConfig
type ConfigBackend = port.ConfigBackend
type EventType = port.EventType
type Event = port.Event
type EventHandler = port.EventHandler
type EventsBackend = port.EventsBackend

const (
	EventWorkspaceCreated            = port.EventWorkspaceCreated
	EventWorkspaceListChanged        = port.EventWorkspaceListChanged
	EventWorkspaceProjectionChanged  = port.EventWorkspaceProjectionChanged
	EventWorkspaceStateChanged       = port.EventWorkspaceStateChanged
	EventWorkspaceDeleted            = port.EventWorkspaceDeleted
	EventGoalDefined                 = port.EventGoalDefined
	EventDoDDefined                  = port.EventDoDDefined
	EventRunStateChanged             = port.EventRunStateChanged
	EventStepDispatched              = port.EventStepDispatched
	EventStepCompleted               = port.EventStepCompleted
	EventDecisionCreated             = port.EventDecisionCreated
	EventDecisionResolved            = port.EventDecisionResolved
	EventApprovalCreated             = port.EventApprovalCreated
	EventApprovalResolved            = port.EventApprovalResolved
	EventResultAccepted              = port.EventResultAccepted
	EventMemoryCreated               = port.EventMemoryCreated
	EventMemoryUpdated               = port.EventMemoryUpdated
	EventMemoryDeleted               = port.EventMemoryDeleted
	EventMemoryRecallRecorded        = port.EventMemoryRecallRecorded
	EventLLMStreamProgress           = port.EventLLMStreamProgress
	EventLLMUsageRecorded            = port.EventLLMUsageRecorded
	EventPatchPreviewed              = port.EventPatchPreviewed
	EventPatchApplied                = port.EventPatchApplied
	EventPatchFailed                 = port.EventPatchFailed
	EventPatchReverted               = port.EventPatchReverted
	EventArtifactContinuationChanged = port.EventArtifactContinuationChanged
	EventGovernanceEvaluated         = port.EventGovernanceEvaluated
	EventGovernanceResolved          = port.EventGovernanceResolved
	EventGovernanceEscalated         = port.EventGovernanceEscalated
	EventGovernanceBlocked           = port.EventGovernanceBlocked
)
