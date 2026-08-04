package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type CLIWorkspaceCreateRequest struct {
	Title                string
	Description          string
	DeliverableDirectory string
	TemplateID           string
}

type CLIWorkspaceViewModelSection struct {
	Name                    string
	Revision                int64
	WorkspaceRevision       int64
	SourceEventRevision     int64
	ProjectedSourceRevision int64
	State                   string
}

type CLIWorkspaceViewModelInfo struct {
	WorkspaceID             common.WorkspaceID
	State                   string
	PromptMode              string
	Revision                int64
	GlobalRevision          int64
	WorkspaceRevision       int64
	SourceEventRevision     int64
	ProjectedSourceRevision int64
	UpdatedAt               string
	RebuiltAt               string
	Degraded                bool
	StaleReason             string
	Error                   string
	Sections                []CLIWorkspaceViewModelSection
}

type CLIWorkspaceInfo struct {
	ID                   common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	BlockingTicketID     common.TicketID
	BlockingApprovalID   common.ApprovalID
	TemplateID           string
	ProfileID            string
	DefaultLLMTarget     *runtimecontract.ProviderModelTarget
	DefaultSoulID        common.SoulID
	SoulBindings         []soulbinding.Binding
	ContextHints         workorcheventpayload.Payload
	EvidencePolicies     []workorcheventpayload.Payload
	Projection           workorchport.ProjectionInfo
	ViewModel            CLIWorkspaceViewModelInfo
}

type CLIWorkspaceSnapshotInfo struct {
	Workspace               CLIWorkspaceInfo
	ViewModel               CLIWorkspaceViewModelInfo
	Projection              workorchport.ProjectionInfo
	Outcome                 workorchport.WorkspaceOutcomeInfo
	TaskProgress            workorchport.TaskProgressInfo
	RunHeartbeat            workorchport.RunHeartbeatInfo
	ConvergenceStatus       *workorchport.ConvergenceStatusInfo
	RunCommand              *workorchport.RunCommandStatusInfo
	SchedulerDiagnostics    *workorchport.SchedulerDiagnosticsInfo
	ContextBundle           workorcheventpayload.Payload
	CoverageSummary         workorcheventpayload.Payload
	GapBaseline             *workorchport.GapBaselineInfo
	Acceptance              workorchport.AcceptanceProjectionInfo
	TaskDecompositions      []workorchport.TaskDecompositionInfo
	TaskPlans               []workorchport.TaskPlanInfo
	Goals                   []workorchport.GoalInfo
	DoDs                    []workorchport.DoDInfo
	Runs                    []workorchport.RunInfo
	Results                 []workorchport.ResultInfo
	Artifacts               []workorchport.ArtifactInfo
	ArtifactContinuations   []workorchport.ArtifactContinuationInfo
	Patches                 []workorchport.PatchInfo
	MemoryRecallTraces      []workorchport.MemoryRecallTraceInfo
	WebSources              []workorchport.WebSourceTraceInfo
	LLMContextBudgets       []workorchport.LLMContextBudgetTraceInfo
	LLMContextBudgetSummary workorchport.LLMContextBudgetSummaryInfo
	LLMUsageTraces          []workorchport.LLMUsageTraceInfo
	LLMUsageSummary         workorchport.LLMUsageSummaryInfo
	EventRecords            []workorchport.EventRecordInfo
	Decisions               []workorchport.DecisionInfo
	Approvals               []workorchport.ApprovalInfo
}

type CLITemplateInfo struct {
	ID               string
	Name             string
	Description      string
	GoalText         string
	DoDItems         []string
	DefaultSoulID    common.SoulID
	SoulBindings     []soulbinding.Binding
	ContextHints     workorcheventpayload.Payload
	EvidencePolicies []workorcheventpayload.Payload
}
