package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
	Projection           projectioncontract.ProjectionStatus
	ViewModel            CLIWorkspaceViewModelInfo
}

type CLIWorkspaceSnapshotInfo struct {
	Workspace               CLIWorkspaceInfo
	ViewModel               CLIWorkspaceViewModelInfo
	Projection              projectioncontract.ProjectionStatus
	Outcome                 workorchport.WorkspaceOutcomeInfo
	TaskProgress            workorchport.TaskProgressInfo
	RunHeartbeat            workorchport.RunHeartbeatInfo
	ConvergenceStatus       *workorchport.ConvergenceStatusInfo
	RunCommand              *workorchport.RunCommandStatusInfo
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics
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

type CLIStringIntEntry struct {
	Key   string
	Value int
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
	TypeQuota              []CLIStringIntEntry
	SelectionScores        []projectioncontract.MemoryRecallSelectionScore
	RetrievedMemoryIDs     []common.MemoryID
	RetrievedMemoryHits    []workorchport.MemoryRecallHitInfo
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
