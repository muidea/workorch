package backend

import (
	port "github.com/muidea/workorch/internal/pkg/workorchport"
)

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
type LLMContextBudgetTraceInfo = port.LLMContextBudgetTraceInfo
type LLMContextBudgetSummaryInfo = port.LLMContextBudgetSummaryInfo
type LLMUsageTraceInfo = port.LLMUsageTraceInfo
type LLMUsageSummaryInfo = port.LLMUsageSummaryInfo
type LLMUsageProviderModelBreakdownInfo = port.LLMUsageProviderModelBreakdownInfo
type WebSourceInfo = port.WebSourceInfo
type FetchedWebSourceInfo = port.FetchedWebSourceInfo
type WorkspaceSnapshotInfo = port.WorkspaceSnapshotInfo
type WorkspaceProjectionSection = port.WorkspaceProjectionSection
type WorkspaceRefreshRevision = port.WorkspaceRefreshRevision
type WorkspaceProjectionChangedInfo = port.WorkspaceProjectionChangedInfo
type WorkspaceActivityKind = port.WorkspaceActivityKind
type WorkspaceActivityItemInfo = port.WorkspaceActivityItemInfo
type WorkspaceActivityFeedInfo = port.WorkspaceActivityFeedInfo
type WorkspaceRuntimeViewInfo = port.WorkspaceRuntimeViewInfo
type ReadWorkspaceRuntimeViewRequest = port.ReadWorkspaceRuntimeViewRequest
type SubscriptionState = port.SubscriptionState
type SubscriptionStatusInfo = port.SubscriptionStatusInfo
type AcceptanceSliceInfo = port.AcceptanceSliceInfo
type CoverageFocusInfo = port.CoverageFocusInfo
type ConvergenceBudgetInfo = port.ConvergenceBudgetInfo
type AcceptanceSummaryInfo = port.AcceptanceSummaryInfo
type AcceptanceProjectionInfo = port.AcceptanceProjectionInfo
type RuntimeRepairDirectiveInfo = port.RuntimeRepairDirectiveInfo
type RuntimeRepairClosureInfo = port.RuntimeRepairClosureInfo
type RuntimeAuditInfo = port.RuntimeAuditInfo
type RuntimeAuditRepairInfo = port.RuntimeAuditRepairInfo
type RuntimeAuditCoverageBindingInfo = port.RuntimeAuditCoverageBindingInfo
type RuntimeAuditCoverageProofInfo = port.RuntimeAuditCoverageProofInfo
type RuntimeAuditDispatchInfo = port.RuntimeAuditDispatchInfo
type RuntimeAuditSealedOperationInfo = port.RuntimeAuditSealedOperationInfo
type RuntimeAuditEvidenceInfo = port.RuntimeAuditEvidenceInfo
type RuntimeAuditOperationReceiptInfo = port.RuntimeAuditOperationReceiptInfo
type RuntimeAuditAssessmentInfo = port.RuntimeAuditAssessmentInfo
type RuntimeAuditAssessmentItemInfo = port.RuntimeAuditAssessmentItemInfo
type RuntimeAuditAssessmentGapInfo = port.RuntimeAuditAssessmentGapInfo
type RuntimeAuditFailureInfo = port.RuntimeAuditFailureInfo
type GapBaselineInfo = port.GapBaselineInfo
type GapTodoItemInfo = port.GapTodoItemInfo
type GapNextActionInfo = port.GapNextActionInfo
type GapDiagnosticInfo = port.GapDiagnosticInfo
type EventRecordInfo = port.EventRecordInfo
type RecoveryPlanInfo = port.RecoveryPlanInfo
type RepairLoopInfo = port.RepairLoopInfo
type TaskProgressInfo = port.TaskProgressInfo
type ProgressMilestoneInfo = port.ProgressMilestoneInfo
type RuntimeProgressInfo = port.RuntimeProgressInfo
type RuntimePrerequisiteRefreshInvalidationInfo = port.RuntimePrerequisiteRefreshInvalidationInfo
type RuntimePrerequisiteResourceChangeInfo = port.RuntimePrerequisiteResourceChangeInfo
type AssessmentOutcomeInfo = port.AssessmentOutcomeInfo
type TaskDecompositionNodeInfo = port.TaskDecompositionNodeInfo
type TaskDecompositionInfo = port.TaskDecompositionInfo
type TaskPlanInfo = port.TaskPlanInfo
type WorkspaceBackend = port.WorkspaceBackend
type GoalInfo = port.GoalInfo
type GoalBackend = port.GoalBackend
type DoDInfo = port.DoDInfo
type DoDBackend = port.DoDBackend
type RunInfo = port.RunInfo
type RuntimeActivationStatusInfo = port.RuntimeActivationStatusInfo
type ActivateRunRequest = port.ActivateRunRequest
type RunCommandIntent = port.RunCommandIntent
type WorkspaceRunCommand = port.WorkspaceRunCommand

const (
	RunCommandIntentResume  = port.RunCommandIntentResume
	RunCommandIntentRestart = port.RunCommandIntentRestart
)

func NormalizeRunCommandIntent(intent RunCommandIntent) (RunCommandIntent, error) {
	return port.NormalizeRunCommandIntent(intent)
}

func ValidateWorkspaceRunCommand(req WorkspaceRunCommand) (RunCommandIntent, error) {
	return port.ValidateWorkspaceRunCommand(req)
}

func CanonicalRuntimeBlockKind(kind string) string {
	return port.CanonicalRuntimeBlockKind(kind)
}

type RunStatusInfo = port.RunStatusInfo
type ConvergenceStatusInfo = port.ConvergenceStatusInfo
type RunHeartbeatInfo = port.RunHeartbeatInfo
type RunCommandStatusInfo = port.RunCommandStatusInfo
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
type ServiceConfig = port.ServiceConfig
type ChatModelTargetOption = port.ChatModelTargetOption
type ConfigBackend = port.ConfigBackend
type EventType = port.EventType
type Event = port.Event
type EventHandler = port.EventHandler
type EventsBackend = port.EventsBackend
type WorkspaceProjectionEventsBackend = port.WorkspaceProjectionEventsBackend
type WorkspaceProjectionChangedHandler = port.WorkspaceProjectionChangedHandler
type SubscriptionStatusHandler = port.SubscriptionStatusHandler

const (
	WorkspaceProjectionSectionOverview        = port.WorkspaceProjectionSectionOverview
	WorkspaceProjectionSectionRuntimeProgress = port.WorkspaceProjectionSectionRuntimeProgress
	WorkspaceProjectionSectionActivity        = port.WorkspaceProjectionSectionActivity
	WorkspaceProjectionSectionGovernance      = port.WorkspaceProjectionSectionGovernance
	WorkspaceProjectionSectionEvidence        = port.WorkspaceProjectionSectionEvidence
	WorkspaceProjectionSectionResult          = port.WorkspaceProjectionSectionResult
	WorkspaceProjectionSectionArtifacts       = port.WorkspaceProjectionSectionArtifacts
	WorkspaceProjectionSectionMemory          = port.WorkspaceProjectionSectionMemory
	WorkspaceProjectionSectionLLMUsage        = port.WorkspaceProjectionSectionLLMUsage

	SubscriptionConnecting   = port.SubscriptionConnecting
	SubscriptionLive         = port.SubscriptionLive
	SubscriptionReconnecting = port.SubscriptionReconnecting
	SubscriptionDegraded     = port.SubscriptionDegraded

	EventWorkspaceCreated           = port.EventWorkspaceCreated
	EventWorkspaceListChanged       = port.EventWorkspaceListChanged
	EventWorkspaceProjectionChanged = port.EventWorkspaceProjectionChanged
	EventWorkspaceStateChanged      = port.EventWorkspaceStateChanged
	EventWorkspaceDeleted           = port.EventWorkspaceDeleted
	EventGoalDefined                = port.EventGoalDefined
	EventDoDDefined                 = port.EventDoDDefined
	EventRunStateChanged            = port.EventRunStateChanged
	EventDecisionCreated            = port.EventDecisionCreated
	EventDecisionResolved           = port.EventDecisionResolved
	EventApprovalResolved           = port.EventApprovalResolved
	EventResultAccepted             = port.EventResultAccepted
	EventMemoryCreated              = port.EventMemoryCreated
	EventMemoryUpdated              = port.EventMemoryUpdated
	EventMemoryDeleted              = port.EventMemoryDeleted
	EventMemoryRecallRecorded       = port.EventMemoryRecallRecorded
	EventGovernanceEvaluated        = port.EventGovernanceEvaluated
	EventGovernanceResolved         = port.EventGovernanceResolved
	EventGovernanceEscalated        = port.EventGovernanceEscalated
	EventGovernanceBlocked          = port.EventGovernanceBlocked
)
