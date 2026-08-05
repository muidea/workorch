package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type CLIWorkspaceViewModelInfo struct {
	WorkspaceID             common.WorkspaceID
	State                   string
	PromptMode              string
	Revision                int64
	StreamGeneration        string
	GlobalRevision          int64
	WorkspaceRevision       int64
	SourceEventRevision     int64
	ProjectedSourceRevision int64
	UpdatedAt               string
	RebuiltAt               string
	Degraded                bool
	StaleReason             string
	Error                   string
	FailedWorkspaceIds      []string
	DegradedSince           string
	Sections                []projectioncontract.WorkspaceViewModelSection
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
	Projection           projectioncontract.ProjectionStatus
	ViewModel            CLIWorkspaceViewModelInfo
}

type CLIWorkspaceSnapshotInfo struct {
	Workspace               CLIWorkspaceInfo
	ViewModel               CLIWorkspaceViewModelInfo
	Projection              projectioncontract.ProjectionStatus
	Run                     *workorchport.RunInfo
	Step                    *workorchport.StepViewInfo
	Result                  *workorchport.ResultInfo
	Outcome                 workorchport.WorkspaceOutcomeInfo
	TaskProgress            workorchport.TaskProgressInfo
	RunHeartbeat            workorchport.RunHeartbeatInfo
	ConvergenceStatus       *workorchport.ConvergenceStatusInfo
	RunCommand              *workorchport.RunCommandStatusInfo
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics
	ContextBundle           workorcheventpayload.Payload
	CoverageSummary         workorcheventpayload.Payload
	GapBaseline             *workorchport.GapBaselineInfo
	AuthorityDiagnostics    []workorchport.GapDiagnosticInfo
	Acceptance              workorchport.AcceptanceProjectionInfo
	TaskDecompositions      []workorchport.TaskDecompositionInfo
	TaskPlans               []workorchport.TaskPlanInfo
	ExecutionSections       []projectioncontract.ExecutionSection
	Goals                   []workorchport.GoalInfo
	DoDs                    []workorchport.DoDInfo
	Runs                    []workorchport.RunInfo
	Results                 []workorchport.ResultInfo
	Artifacts               []workorchport.ArtifactInfo
	ArtifactContinuations   []workorchport.ArtifactContinuationInfo
	Patches                 []workorchport.PatchInfo
	MemoryRecallTraces      []CLIMemoryRecallTraceInfo
	WebSources              []workorchport.WebSourceTraceInfo
	LLMContextBudgets       []workorchport.LLMContextBudgetTraceInfo
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary
	LLMUsageTraces          []workorchport.LLMUsageTraceInfo
	LLMUsageSummary         workorchport.LLMUsageSummaryInfo
	EventRecords            []workorchport.EventRecordInfo
	Decisions               []workorchport.DecisionInfo
	Approvals               []workorchport.ApprovalInfo
}

// CLIMemoryRecallTraceInfo is the map-free EventHub representation of the
// process-facing trace DTO. TypeQuota is a stable entry list at this boundary.
type CLIMemoryRecallTraceInfo struct {
	ID                     common.MemoryRecallTraceID
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	StepID                 common.StepID
	GraphNodeID            string
	Stage                  string
	Source                 string
	Policy                 string
	Status                 string
	ProviderID             string
	Operation              string
	QueryTextPreview       string
	QueryTextSha256        string
	MinScore               float64
	Limit                  int
	EmbeddingModel         string
	VectorDimension        int
	RefreshSource          string
	SelectionPolicyVersion string
	CandidateMemoryIDs     []common.MemoryID
	SelectedMemoryIDs      []common.MemoryID
	ExcludedMemoryIDs      []common.MemoryID
	TypeQuota              []workorchmemorycontract.TypeQuotaEntry
	SelectionScores        []workorchmemorycontract.RecallSelectionScore
	RetrievedMemoryIDs     []common.MemoryID
	RetrievedMemoryHits    []workorchmemorycontract.RecallHit
	Reason                 string
	OccurredAt             string
	EvidenceRef            string
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
