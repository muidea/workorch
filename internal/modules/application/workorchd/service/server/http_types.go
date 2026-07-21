package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/magicCommon/task"
	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type statusResponse struct {
	Service     string                           `json:"service"`
	Status      string                           `json:"status"`
	Projection  *workspaceProjectionStatus       `json:"projection,omitempty"`
	Diagnostics *appservice.SchedulerDiagnostics `json:"diagnostics,omitempty"`
}

type configurationResponse struct {
	Service       string                          `json:"service"`
	Configuration appservice.ServiceConfiguration `json:"configuration"`
}

type llmModelCatalogResponse struct {
	Service string                            `json:"service"`
	Catalog appservice.ProviderCatalogSummary `json:"catalog"`
}

type workspaceResponse struct {
	Service   string               `json:"service"`
	Backend   string               `json:"backend,omitempty"`
	DataRoot  string               `json:"dataRoot,omitempty"`
	Workspace appservice.Workspace `json:"workspace"`
}

type workspacesResponse struct {
	Service    string                    `json:"service"`
	Backend    string                    `json:"backend,omitempty"`
	DataRoot   string                    `json:"dataRoot,omitempty"`
	Projection workspaceProjectionStatus `json:"projection"`
	Workspaces []appservice.Workspace    `json:"workspaces"`
	Error      string                    `json:"error,omitempty"`
}

type goalResponse struct {
	Service  string          `json:"service"`
	Backend  string          `json:"backend,omitempty"`
	DataRoot string          `json:"dataRoot,omitempty"`
	Goal     appservice.Goal `json:"goal"`
}

type goalsResponse struct {
	Service  string            `json:"service"`
	Backend  string            `json:"backend,omitempty"`
	DataRoot string            `json:"dataRoot,omitempty"`
	Goals    []appservice.Goal `json:"goals"`
}

type dodResponse struct {
	Service  string         `json:"service"`
	Backend  string         `json:"backend,omitempty"`
	DataRoot string         `json:"dataRoot,omitempty"`
	DoD      appservice.DoD `json:"dod"`
}

type dodsResponse struct {
	Service  string           `json:"service"`
	Backend  string           `json:"backend,omitempty"`
	DataRoot string           `json:"dataRoot,omitempty"`
	DoDs     []appservice.DoD `json:"dods"`
}

type goalDoDQualityCheckResponse struct {
	Service      string                         `json:"service"`
	Backend      string                         `json:"backend,omitempty"`
	DataRoot     string                         `json:"dataRoot,omitempty"`
	QualityCheck appservice.GoalDoDQualityCheck `json:"qualityCheck"`
}

type workspaceSetupDraftSessionResponse struct {
	Service      string                     `json:"service"`
	Backend      string                     `json:"backend,omitempty"`
	DataRoot     string                     `json:"dataRoot,omitempty"`
	DraftSession workspaceSetupDraftSession `json:"draftSession"`
}

type runInstanceResponse struct {
	Service  string         `json:"service"`
	Backend  string         `json:"backend,omitempty"`
	DataRoot string         `json:"dataRoot,omitempty"`
	Run      appservice.Run `json:"run"`
}

type runSnapshotResponse struct {
	Service  string                 `json:"service"`
	Backend  string                 `json:"backend,omitempty"`
	DataRoot string                 `json:"dataRoot,omitempty"`
	Snapshot appservice.RunSnapshot `json:"snapshot"`
}

type runStepsResponse struct {
	Service  string               `json:"service"`
	Backend  string               `json:"backend,omitempty"`
	DataRoot string               `json:"dataRoot,omitempty"`
	Steps    []presenter.StepView `json:"steps,omitempty"`
	Page     appservice.StepPage  `json:"page"`
}

type runToolCallsResponse struct {
	Service   string                  `json:"service"`
	Backend   string                  `json:"backend,omitempty"`
	DataRoot  string                  `json:"dataRoot,omitempty"`
	ToolCalls []appservice.ToolCall   `json:"toolCalls,omitempty"`
	Page      appservice.ToolCallPage `json:"page"`
}

type runRuntimeCheckpointsResponse struct {
	Service            string                           `json:"service"`
	Backend            string                           `json:"backend,omitempty"`
	DataRoot           string                           `json:"dataRoot,omitempty"`
	RuntimeCheckpoints []appservice.RuntimeCheckpoint   `json:"runtimeCheckpoints,omitempty"`
	Page               appservice.RuntimeCheckpointPage `json:"page"`
}

type runResultsResponse struct {
	Service  string                `json:"service"`
	Backend  string                `json:"backend,omitempty"`
	DataRoot string                `json:"dataRoot,omitempty"`
	Results  []appservice.Result   `json:"results,omitempty"`
	Page     appservice.ResultPage `json:"page"`
}

type runPatchesResponse struct {
	Service  string               `json:"service"`
	Backend  string               `json:"backend,omitempty"`
	DataRoot string               `json:"dataRoot,omitempty"`
	Patches  []appservice.Patch   `json:"patches,omitempty"`
	Page     appservice.PatchPage `json:"page"`
}

type runValidationResultsResponse struct {
	Service           string                          `json:"service"`
	Backend           string                          `json:"backend,omitempty"`
	DataRoot          string                          `json:"dataRoot,omitempty"`
	ValidationResults []appservice.ValidationResult   `json:"validationResults,omitempty"`
	Page              appservice.ValidationResultPage `json:"page"`
}

type runArtifactsResponse struct {
	Service   string                  `json:"service"`
	Backend   string                  `json:"backend,omitempty"`
	DataRoot  string                  `json:"dataRoot,omitempty"`
	Artifacts []appservice.Artifact   `json:"artifacts,omitempty"`
	Page      appservice.ArtifactPage `json:"page"`
}

type runStatusResponse struct {
	Service       string               `json:"service"`
	Backend       string               `json:"backend,omitempty"`
	DataRoot      string               `json:"dataRoot,omitempty"`
	Status        appservice.RunStatus `json:"status"`
	WaitMatched   bool                 `json:"waitMatched,omitempty"`
	WaitElapsedMs int64                `json:"waitElapsedMs,omitempty"`
}

type runObservabilityResponse struct {
	Service       string                      `json:"service"`
	Backend       string                      `json:"backend,omitempty"`
	DataRoot      string                      `json:"dataRoot,omitempty"`
	Observability appservice.RunObservability `json:"observability"`
}

type taskProgressResponse struct {
	Service      string                  `json:"service"`
	Backend      string                  `json:"backend,omitempty"`
	DataRoot     string                  `json:"dataRoot,omitempty"`
	TaskProgress appservice.TaskProgress `json:"taskProgress"`
}

type contextBundleResponse struct {
	Service       string                   `json:"service"`
	Backend       string                   `json:"backend,omitempty"`
	DataRoot      string                   `json:"dataRoot,omitempty"`
	ContextBundle appservice.ContextBundle `json:"contextBundle"`
}

type taskDecompositionsResponse struct {
	Service            string                         `json:"service"`
	Backend            string                         `json:"backend,omitempty"`
	DataRoot           string                         `json:"dataRoot,omitempty"`
	TaskDecompositions []appservice.TaskDecomposition `json:"taskDecompositions"`
}

type eventRecordsResponse struct {
	Service      string                   `json:"service"`
	Backend      string                   `json:"backend,omitempty"`
	DataRoot     string                   `json:"dataRoot,omitempty"`
	EventRecords []appservice.EventRecord `json:"eventRecords"`
}

type runsResponse struct {
	Service  string           `json:"service"`
	Backend  string           `json:"backend,omitempty"`
	DataRoot string           `json:"dataRoot,omitempty"`
	Runs     []appservice.Run `json:"runs"`
}

type workspaceSnapshotResponse struct {
	Service    string                       `json:"service"`
	Backend    string                       `json:"backend,omitempty"`
	DataRoot   string                       `json:"dataRoot,omitempty"`
	Projection workspaceProjectionStatus    `json:"projection"`
	Snapshot   appservice.WorkspaceSnapshot `json:"snapshot"`
	Error      string                       `json:"error,omitempty"`
}

type workspaceRuntimeViewResponse struct {
	Service     string                          `json:"service"`
	Backend     string                          `json:"backend,omitempty"`
	DataRoot    string                          `json:"dataRoot,omitempty"`
	Projection  workspaceProjectionStatus       `json:"projection"`
	RuntimeView appservice.WorkspaceRuntimeView `json:"runtimeView"`
	Error       string                          `json:"error,omitempty"`
}

type workspaceExecutionActivationResponse struct {
	Service    string                                  `json:"service"`
	Backend    string                                  `json:"backend,omitempty"`
	DataRoot   string                                  `json:"dataRoot,omitempty"`
	Activation appservice.WorkspaceExecutionActivation `json:"activation"`
}

type workspaceSoulBindingsResponse struct {
	Service      string                           `json:"service"`
	Backend      string                           `json:"backend,omitempty"`
	DataRoot     string                           `json:"dataRoot,omitempty"`
	SoulBindings appservice.WorkspaceSoulBindings `json:"soulBindings"`
}

type decisionResponse struct {
	Service  string              `json:"service"`
	Backend  string              `json:"backend,omitempty"`
	DataRoot string              `json:"dataRoot,omitempty"`
	Decision appservice.Decision `json:"decision"`
}

type decisionsResponse struct {
	Service   string                  `json:"service"`
	Backend   string                  `json:"backend,omitempty"`
	DataRoot  string                  `json:"dataRoot,omitempty"`
	Decisions []appservice.Decision   `json:"decisions"`
	Page      appservice.DecisionPage `json:"page"`
}

type approvalResponse struct {
	Service  string              `json:"service"`
	Backend  string              `json:"backend,omitempty"`
	DataRoot string              `json:"dataRoot,omitempty"`
	Approval appservice.Approval `json:"approval"`
}

type approvalsResponse struct {
	Service   string                  `json:"service"`
	Backend   string                  `json:"backend,omitempty"`
	DataRoot  string                  `json:"dataRoot,omitempty"`
	Approvals []appservice.Approval   `json:"approvals"`
	Page      appservice.ApprovalPage `json:"page"`
}

type soulResponse struct {
	Service  string          `json:"service"`
	Backend  string          `json:"backend,omitempty"`
	DataRoot string          `json:"dataRoot,omitempty"`
	Soul     appservice.Soul `json:"soul"`
}

type soulsResponse struct {
	Service  string            `json:"service"`
	Backend  string            `json:"backend,omitempty"`
	DataRoot string            `json:"dataRoot,omitempty"`
	Souls    []appservice.Soul `json:"souls"`
}

type workspaceTemplateResponse struct {
	Service           string                       `json:"service"`
	Backend           string                       `json:"backend,omitempty"`
	DataRoot          string                       `json:"dataRoot,omitempty"`
	WorkspaceTemplate appservice.WorkspaceTemplate `json:"workspaceTemplate"`
}

type workspaceTemplatesResponse struct {
	Service            string                         `json:"service"`
	Backend            string                         `json:"backend,omitempty"`
	DataRoot           string                         `json:"dataRoot,omitempty"`
	WorkspaceTemplates []appservice.WorkspaceTemplate `json:"workspaceTemplates"`
}

type skillResponse struct {
	Service  string           `json:"service"`
	Backend  string           `json:"backend,omitempty"`
	DataRoot string           `json:"dataRoot,omitempty"`
	Skill    appservice.Skill `json:"skill"`
}

type skillsResponse struct {
	Service  string             `json:"service"`
	Backend  string             `json:"backend,omitempty"`
	DataRoot string             `json:"dataRoot,omitempty"`
	Skills   []appservice.Skill `json:"skills"`
}

type mcpServerResponse struct {
	Service  string               `json:"service"`
	Backend  string               `json:"backend,omitempty"`
	DataRoot string               `json:"dataRoot,omitempty"`
	Server   appservice.MCPServer `json:"server"`
}

type mcpServersResponse struct {
	Service  string                 `json:"service"`
	Backend  string                 `json:"backend,omitempty"`
	DataRoot string                 `json:"dataRoot,omitempty"`
	Servers  []appservice.MCPServer `json:"servers"`
}

type mcpToolsResponse struct {
	Service  string               `json:"service"`
	Backend  string               `json:"backend,omitempty"`
	DataRoot string               `json:"dataRoot,omitempty"`
	Tools    []appservice.MCPTool `json:"tools"`
}

type mcpResourcesResponse struct {
	Service   string                   `json:"service"`
	Backend   string                   `json:"backend,omitempty"`
	DataRoot  string                   `json:"dataRoot,omitempty"`
	Resources []appservice.MCPResource `json:"resources"`
}

type mcpPromptsResponse struct {
	Service  string                 `json:"service"`
	Backend  string                 `json:"backend,omitempty"`
	DataRoot string                 `json:"dataRoot,omitempty"`
	Prompts  []appservice.MCPPrompt `json:"prompts"`
}

type mcpCapabilityResponse struct {
	Service    string                   `json:"service"`
	Backend    string                   `json:"backend,omitempty"`
	DataRoot   string                   `json:"dataRoot,omitempty"`
	Capability appservice.MCPCapability `json:"capability"`
}

type mcpCapabilitiesResponse struct {
	Service      string                     `json:"service"`
	Backend      string                     `json:"backend,omitempty"`
	DataRoot     string                     `json:"dataRoot,omitempty"`
	Capabilities []appservice.MCPCapability `json:"capabilities"`
}

type memoryItemResponse struct {
	Service  string                `json:"service"`
	Backend  string                `json:"backend,omitempty"`
	DataRoot string                `json:"dataRoot,omitempty"`
	Memory   appservice.MemoryItem `json:"memory"`
}

type memoryItemsResponse struct {
	Service  string                  `json:"service"`
	Backend  string                  `json:"backend,omitempty"`
	DataRoot string                  `json:"dataRoot,omitempty"`
	Memory   []appservice.MemoryItem `json:"memory"`
}

type memoryRecallTracesResponse struct {
	Service            string                         `json:"service"`
	Backend            string                         `json:"backend,omitempty"`
	DataRoot           string                         `json:"dataRoot,omitempty"`
	MemoryRecallTraces []appservice.MemoryRecallTrace `json:"memoryRecallTraces"`
}

type createWorkspaceRequest struct {
	WorkspaceID          string `json:"workspaceId"`
	Title                string `json:"title"`
	Description          string `json:"description"`
	DeliverableDirectory string `json:"deliverableDirectory"`
	TemplateID           string `json:"templateId,omitempty"`
}

type deleteWorkspaceRequest = appservice.DeleteWorkspaceCommand

type createRunRequest struct {
	RunID  string `json:"runId"`
	GoalID string `json:"goalId,omitempty"`
	DoDID  string `json:"doDId,omitempty"`
}

type workspaceExecutionCommandRequest struct {
	RunID              string                               `json:"runId,omitempty"`
	GoalID             string                               `json:"goalId,omitempty"`
	DoDID              string                               `json:"doDId,omitempty"`
	LLMTarget          *runtimecontract.ProviderModelTarget `json:"llmTarget,omitempty"`
	SoulID             string                               `json:"soulId,omitempty"`
	SoulBindings       map[string]string                    `json:"soulBindings,omitempty"`
	SkillIDs           []string                             `json:"skillIds,omitempty"`
	MCPServerIDs       []string                             `json:"mcpServerIds,omitempty"`
	CapabilityIDs      []string                             `json:"capabilityIds,omitempty"`
	MemoryIDs          []string                             `json:"memoryIds,omitempty"`
	DialogLanguage     string                               `json:"dialogLanguage,omitempty"`
	Priority           int                                  `json:"priority,omitempty"`
	RequestID          string                               `json:"requestId,omitempty"`
	Reason             string                               `json:"reason,omitempty"`
	Source             string                               `json:"source,omitempty"`
	Intent             string                               `json:"intent,omitempty"`
	DefinitionRevision string                               `json:"definitionRevision,omitempty"`
	DecisionID         string                               `json:"decisionId,omitempty"`
	Restart            bool                                 `json:"restart,omitempty"`
}

type createSoulRequest = appservice.CreateSoulCommand
type createWorkspaceTemplateRequest = appservice.WorkspaceTemplate
type createSkillRequest = appservice.CreateSkillCommand
type createMCPServerRequest = appservice.CreateMCPServerCommand
type updateMCPServerRequest = appservice.UpdateMCPServerCommand
type createMCPCapabilityRequest = appservice.CreateMCPCapabilityCommand
type createMemoryItemRequest = appservice.CreateMemoryItemCommand

type resolveDecisionRequest struct {
	ResolutionType    string                       `json:"resolutionType"`
	ResolutionPayload workorcheventpayload.Payload `json:"resolutionPayload,omitempty"`
}

type batchResolveDecisionsRequest struct {
	TicketIDs         []string                     `json:"ticketIds"`
	ResolutionType    string                       `json:"resolutionType"`
	ResolutionPayload workorcheventpayload.Payload `json:"resolutionPayload,omitempty"`
}

type resolveApprovalRequest struct {
	Payload workorcheventpayload.Payload `json:"payload,omitempty"`
}

type batchResolveApprovalsRequest struct {
	ApprovalIDs []string                     `json:"approvalIds"`
	Action      string                       `json:"action"`
	Payload     workorcheventpayload.Payload `json:"payload,omitempty"`
}

type defineGoalRequest struct {
	Text string `json:"text"`
}

type defineDoDRequest struct {
	Items []string `json:"items"`
}

type goalDoDQualityCheckRequest struct {
	GoalID   string   `json:"goalId,omitempty"`
	DoDID    string   `json:"doDId,omitempty"`
	GoalText string   `json:"goalText,omitempty"`
	DoDItems []string `json:"doDItems,omitempty"`
}

type createWorkspaceSetupDraftSessionRequest struct {
	TemplateID           string `json:"templateId,omitempty"`
	Title                string `json:"title,omitempty"`
	Description          string `json:"description,omitempty"`
	DeliverableDirectory string `json:"deliverableDirectory,omitempty"`
	DialogLanguage       string `json:"dialogLanguage,omitempty"`
}

type appendWorkspaceSetupDraftMessageRequest struct {
	TemplateID           string                         `json:"templateId,omitempty"`
	Title                string                         `json:"title,omitempty"`
	Description          string                         `json:"description,omitempty"`
	DeliverableDirectory string                         `json:"deliverableDirectory,omitempty"`
	Content              string                         `json:"content"`
	Draft                appservice.WorkspaceSetupDraft `json:"draft,omitempty"`
	DialogLanguage       string                         `json:"dialogLanguage,omitempty"`
}

type errorResponse struct {
	Error                   string   `json:"error"`
	Code                    string   `json:"code,omitempty"`
	TicketID                string   `json:"ticketId,omitempty"`
	RequestedResolutionType string   `json:"requestedResolutionType,omitempty"`
	AllowedActions          []string `json:"allowedActions,omitempty"`
	Remediation             string   `json:"remediation,omitempty"`
}

type createWorkspaceFunc func(context.Context, string, string, appservice.CreateWorkspaceCommand) (appservice.Workspace, error)
type deleteWorkspaceFunc func(context.Context, string, string, string, appservice.DeleteWorkspaceCommand) error
type readWorkspaceFunc func(context.Context, string, string, string) (appservice.Workspace, error)
type readWorkspaceSnapshotFunc func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error)
type readProjectionSnapshotFunc func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error)
type readWorkspaceSoulBindingsFunc func(context.Context, string, string, string) (appservice.WorkspaceSoulBindings, error)
type updateWorkspaceSoulBindingsFunc func(context.Context, string, string, string, appservice.WorkspaceSoulBindings) (appservice.WorkspaceSoulBindings, error)
type listWorkspacesFunc func(context.Context, string, string) ([]appservice.Workspace, error)
type defineGoalFunc func(context.Context, string, string, string, string) (appservice.Goal, error)
type updateGoalFunc func(context.Context, string, string, string, string) (appservice.Goal, error)
type readGoalFunc func(context.Context, string, string, string) (appservice.Goal, error)
type listGoalsFunc func(context.Context, string, string, string) ([]appservice.Goal, error)
type defineDoDFunc func(context.Context, string, string, string, []string) (appservice.DoD, error)
type updateDoDFunc func(context.Context, string, string, string, []string) (appservice.DoD, error)
type readDoDFunc func(context.Context, string, string, string) (appservice.DoD, error)
type listDoDsFunc func(context.Context, string, string, string) ([]appservice.DoD, error)
type checkGoalDoDQualityFunc func(context.Context, string, string, appservice.GoalDoDQualityCheckCommand) (appservice.GoalDoDQualityCheck, error)
type generateWorkspaceSetupDraftFunc func(context.Context, string, string, appservice.WorkspaceSetupDraftGenerationCommand) (appservice.WorkspaceSetupDraft, error)
type createRunInstanceFunc func(context.Context, string, string, string, string, string, string) (appservice.Run, error)
type readRunInstanceFunc func(context.Context, string, string, string) (appservice.Run, error)
type readRunSnapshotFunc func(context.Context, string, string, string) (appservice.RunSnapshot, error)
type listRunStepsFunc func(context.Context, string, string, appservice.StepQuery) (appservice.StepPage, error)
type listRunToolCallsFunc func(context.Context, string, string, appservice.ToolCallQuery) (appservice.ToolCallPage, error)
type listRunRuntimeCheckpointsFunc func(context.Context, string, string, appservice.RuntimeCheckpointQuery) (appservice.RuntimeCheckpointPage, error)
type listRunResultsFunc func(context.Context, string, string, appservice.ResultQuery) (appservice.ResultPage, error)
type listRunPatchesFunc func(context.Context, string, string, appservice.PatchQuery) (appservice.PatchPage, error)
type listRunValidationResultsFunc func(context.Context, string, string, appservice.ValidationResultQuery) (appservice.ValidationResultPage, error)
type listRunArtifactsFunc func(context.Context, string, string, appservice.ArtifactQuery) (appservice.ArtifactPage, error)
type readRunStatusFunc func(context.Context, string, string, string) (appservice.RunStatus, error)
type readRunObservabilityFunc func(context.Context, string, string, string) (appservice.RunObservability, error)
type readArtifactContentFunc func(context.Context, string, string, string) (appservice.ArtifactContent, error)
type readRuntimeRawContentFunc func(context.Context, string, string, string) (appservice.RuntimeRawContent, error)
type readRunTaskProgressFunc func(context.Context, string, string, string) (appservice.TaskProgress, error)
type readRunContextBundleFunc func(context.Context, string, string, string) (appservice.ContextBundle, error)
type readRunTaskDecompositionsFunc func(context.Context, string, string, string) ([]appservice.TaskDecomposition, error)
type listMemoryRecallTracesFunc func(context.Context, string, string, appservice.MemoryRecallTraceQuery) ([]appservice.MemoryRecallTrace, error)
type listEventRecordsFunc func(context.Context, string, string, appservice.EventRecordQuery) ([]appservice.EventRecord, error)
type listRunInstancesFunc func(context.Context, string, string, string) ([]appservice.Run, error)
type startRunInstanceFunc func(context.Context, string, string, string) (appservice.Run, error)
type resumeRunInstanceFunc func(context.Context, string, string, string) (appservice.Run, error)
type resumeWorkspaceExecutionFunc func(context.Context, string, string, string, appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error)
type listDecisionsFunc func(context.Context, string, string, string, string) ([]appservice.Decision, error)
type listDecisionPageFunc func(context.Context, string, string, appservice.DecisionQuery) (appservice.DecisionPage, error)
type readDecisionFunc func(context.Context, string, string, string) (appservice.Decision, error)
type resolveDecisionFunc func(context.Context, string, string, string, appservice.ResolveDecisionCommand) (appservice.Decision, error)
type batchResolveDecisionsFunc func(context.Context, string, string, appservice.BatchResolveDecisionCommand) ([]appservice.Decision, error)
type listApprovalsFunc func(context.Context, string, string, string, string) ([]appservice.Approval, error)
type listApprovalPageFunc func(context.Context, string, string, appservice.ApprovalQuery) (appservice.ApprovalPage, error)
type readApprovalFunc func(context.Context, string, string, string) (appservice.Approval, error)
type approveApprovalFunc func(context.Context, string, string, string, appservice.ResolveApprovalCommand) (appservice.Approval, error)
type rejectApprovalFunc func(context.Context, string, string, string, appservice.ResolveApprovalCommand) (appservice.Approval, error)
type batchResolveApprovalsFunc func(context.Context, string, string, appservice.BatchResolveApprovalCommand) ([]appservice.Approval, error)
type createSoulFunc func(context.Context, string, string, appservice.CreateSoulCommand) (appservice.Soul, error)
type updateSoulFunc func(context.Context, string, string, string, appservice.UpdateSoulCommand) (appservice.Soul, error)
type deleteSoulFunc func(context.Context, string, string, string) error
type readSoulFunc func(context.Context, string, string, string) (appservice.Soul, error)
type listSoulsFunc func(context.Context, string, string) ([]appservice.Soul, error)
type createWorkspaceTemplateFunc func(context.Context, string, string, appservice.WorkspaceTemplate) (appservice.WorkspaceTemplate, error)
type updateWorkspaceTemplateFunc func(context.Context, string, string, string, appservice.WorkspaceTemplate) (appservice.WorkspaceTemplate, error)
type deleteWorkspaceTemplateFunc func(context.Context, string, string, string) error
type listWorkspaceTemplatesFunc func(context.Context, string, string) ([]appservice.WorkspaceTemplate, error)
type createSkillFunc func(context.Context, string, string, appservice.CreateSkillCommand) (appservice.Skill, error)
type readSkillFunc func(context.Context, string, string, string, appservice.SkillQuery) (appservice.Skill, error)
type listSkillsFunc func(context.Context, string, string, appservice.SkillQuery) ([]appservice.Skill, error)
type createMCPServerFunc func(context.Context, string, string, appservice.CreateMCPServerCommand) (appservice.MCPServer, error)
type updateMCPServerFunc func(context.Context, string, string, string, appservice.UpdateMCPServerCommand) (appservice.MCPServer, error)
type readMCPServerFunc func(context.Context, string, string, string) (appservice.MCPServer, error)
type listMCPServersFunc func(context.Context, string, string) ([]appservice.MCPServer, error)
type listMCPServerToolsFunc func(context.Context, string, string, string) ([]appservice.MCPTool, error)
type listMCPServerResourcesFunc func(context.Context, string, string, string) ([]appservice.MCPResource, error)
type listMCPServerPromptsFunc func(context.Context, string, string, string) ([]appservice.MCPPrompt, error)
type createMCPCapabilityFunc func(context.Context, string, string, appservice.CreateMCPCapabilityCommand) (appservice.MCPCapability, error)
type updateMCPCapabilityFunc func(context.Context, string, string, string, appservice.UpdateMCPCapabilityCommand) (appservice.MCPCapability, error)
type readMCPCapabilityFunc func(context.Context, string, string, string) (appservice.MCPCapability, error)
type listMCPCapabilitiesFunc func(context.Context, string, string, appservice.MCPCapabilityQuery) ([]appservice.MCPCapability, error)
type createMemoryItemFunc func(context.Context, string, string, string, appservice.CreateMemoryItemCommand) (appservice.MemoryItem, error)
type readMemoryItemFunc func(context.Context, string, string, string) (appservice.MemoryItem, error)
type updateMemoryItemFunc func(context.Context, string, string, string, appservice.UpdateMemoryItemCommand) (appservice.MemoryItem, error)
type deleteMemoryItemFunc func(context.Context, string, string, string) error
type listMemoryByWorkspaceFunc func(context.Context, string, string, string, string) ([]appservice.MemoryItem, error)
type searchMemoryFunc func(context.Context, string, string, appservice.MemoryQuery) ([]appservice.MemoryItem, error)
type validateFunc func(context.Context, string, string) error
type readConfigurationFunc func() (appservice.ServiceConfiguration, error)
type updateServerConfigurationFunc func(appservice.UpdateServerConfigurationCommand) (appservice.ServiceConfiguration, error)
type updateRuntimeConfigurationFunc func(appservice.UpdateRuntimeConfigurationCommand) (appservice.ServiceConfiguration, error)
type updateLLMConfigurationFunc func(appservice.UpdateLLMConfigurationCommand) (appservice.ServiceConfiguration, error)
type updateWebSearchConfigurationFunc func(appservice.UpdateWebSearchConfigurationCommand) (appservice.ServiceConfiguration, error)

type getLLMModelCatalogFunc func(context.Context, appservice.GetLLMModelCatalogCommand) (appservice.ProviderCatalogSummary, error)
type refreshLLMModelCatalogFunc func(context.Context, appservice.RefreshLLMModelCatalogCommand) (appservice.ProviderCatalogSummary, error)
type readConfigurationWithCatalogFunc func(context.Context) (appservice.ServiceConfiguration, error)

type serverState struct {
	config                       Config
	workspaceRunScheduler        WorkspaceRunScheduler
	eventReplay                  *sseReplayBuffer
	streamSubscribersMu          sync.Mutex
	streamSubscribers            map[uint64]chan streamEvent
	streamSubscriberSequence     uint64
	metrics                      *serverMetrics
	validate                     validateFunc
	createWorkspace              createWorkspaceFunc
	deleteWorkspace              deleteWorkspaceFunc
	readWorkspace                readWorkspaceFunc
	readWorkspaceSnapshot        readWorkspaceSnapshotFunc
	readWorkspaceDetailSnapshot  readWorkspaceSnapshotFunc
	readProjectionSnapshot       readProjectionSnapshotFunc
	readWorkspaceSoulBindings    readWorkspaceSoulBindingsFunc
	updateWorkspaceSoulBindings  updateWorkspaceSoulBindingsFunc
	listWorkspaces               listWorkspacesFunc
	defineGoal                   defineGoalFunc
	updateGoal                   updateGoalFunc
	readGoal                     readGoalFunc
	listGoals                    listGoalsFunc
	defineDoD                    defineDoDFunc
	updateDoD                    updateDoDFunc
	readDoD                      readDoDFunc
	listDoDs                     listDoDsFunc
	checkGoalDoDQuality          checkGoalDoDQualityFunc
	generateWorkspaceSetupDraft  generateWorkspaceSetupDraftFunc
	createRun                    createRunInstanceFunc
	readRun                      readRunInstanceFunc
	readRunSnapshot              readRunSnapshotFunc
	listRunSteps                 listRunStepsFunc
	listRunToolCalls             listRunToolCallsFunc
	listRunRuntimeCheckpoints    listRunRuntimeCheckpointsFunc
	listRunResults               listRunResultsFunc
	listRunPatches               listRunPatchesFunc
	listRunValidationResults     listRunValidationResultsFunc
	listRunArtifacts             listRunArtifactsFunc
	readRunStatus                readRunStatusFunc
	readRunObservability         readRunObservabilityFunc
	readArtifactContent          readArtifactContentFunc
	readRuntimeRawContent        readRuntimeRawContentFunc
	readRunTaskProgress          readRunTaskProgressFunc
	readRunContextBundle         readRunContextBundleFunc
	readRunTaskDecompositions    readRunTaskDecompositionsFunc
	listMemoryRecallTraces       listMemoryRecallTracesFunc
	listEventRecords             listEventRecordsFunc
	listRuns                     listRunInstancesFunc
	startRun                     startRunInstanceFunc
	resumeRun                    resumeRunInstanceFunc
	resumeWorkspaceExecution     resumeWorkspaceExecutionFunc
	listDecisions                listDecisionsFunc
	listDecisionPage             listDecisionPageFunc
	readDecision                 readDecisionFunc
	resolveDecision              resolveDecisionFunc
	batchResolveDecisions        batchResolveDecisionsFunc
	listApprovals                listApprovalsFunc
	listApprovalPage             listApprovalPageFunc
	readApproval                 readApprovalFunc
	approveApproval              approveApprovalFunc
	rejectApproval               rejectApprovalFunc
	batchResolveApprovals        batchResolveApprovalsFunc
	createSoul                   createSoulFunc
	updateSoul                   updateSoulFunc
	deleteSoul                   deleteSoulFunc
	readSoul                     readSoulFunc
	listSouls                    listSoulsFunc
	createWorkspaceTemplate      createWorkspaceTemplateFunc
	updateWorkspaceTemplate      updateWorkspaceTemplateFunc
	deleteWorkspaceTemplate      deleteWorkspaceTemplateFunc
	listWorkspaceTemplates       listWorkspaceTemplatesFunc
	createSkill                  createSkillFunc
	readSkill                    readSkillFunc
	listSkills                   listSkillsFunc
	createMCPServer              createMCPServerFunc
	updateMCPServer              updateMCPServerFunc
	readMCPServer                readMCPServerFunc
	listMCPServers               listMCPServersFunc
	listMCPServerTools           listMCPServerToolsFunc
	listMCPServerResources       listMCPServerResourcesFunc
	listMCPServerPrompts         listMCPServerPromptsFunc
	createMCPCapability          createMCPCapabilityFunc
	updateMCPCapability          updateMCPCapabilityFunc
	readMCPCapability            readMCPCapabilityFunc
	listMCPCapabilities          listMCPCapabilitiesFunc
	createMemoryItem             createMemoryItemFunc
	readMemoryItem               readMemoryItemFunc
	updateMemoryItem             updateMemoryItemFunc
	deleteMemoryItem             deleteMemoryItemFunc
	listMemoryByWorkspace        listMemoryByWorkspaceFunc
	searchMemory                 searchMemoryFunc
	readConfiguration            readConfigurationFunc
	updateServerConfiguration    updateServerConfigurationFunc
	updateRuntimeConfiguration   updateRuntimeConfigurationFunc
	updateLLMConfiguration       updateLLMConfigurationFunc
	getLLMModelCatalog           getLLMModelCatalogFunc
	refreshLLMModelCatalog       refreshLLMModelCatalogFunc
	readConfigurationWithCatalog readConfigurationWithCatalogFunc
	updateWebSearchConfiguration updateWebSearchConfigurationFunc
	runQueueMu                   sync.Mutex
	runQueueWaiters              []*runQueueWaiter
	runQueueSequence             uint64
	activeRunSlots               int64
	backgroundRoutine            task.BackgroundRoutine
	backgroundWG                 sync.WaitGroup
	backgroundCtx                context.Context
	backgroundCancel             context.CancelFunc
	runStatusNotifyMu            sync.Mutex
	runStatusNotify              chan struct{}
	eventUnsubscribes            []func()
	schedulerDiagnostics         *schedulerDiagnosticsStore
	workspaceProjection          *workspaceProjectionManager
	runSnapshotCacheMu           sync.Mutex
	runSnapshotEntries           map[string]*runSnapshotCacheEntry
	sharedSessionMu              sync.RWMutex
	sharedSession                *appservice.RuntimeSession
	runtimeSessionErr            error
	workspaceSetupDraftMu        sync.Mutex
	workspaceSetupDraftSessions  map[string]*workspaceSetupDraftSession
}

type cachedRunSnapshot struct {
	snapshot appservice.RunSnapshot
	cachedAt time.Time
}

type runSnapshotCacheEntry struct {
	cached   cachedRunSnapshot
	hasCache bool
	inflight *runSnapshotInflight
}

type runSnapshotInflight struct {
	done     chan struct{}
	snapshot appservice.RunSnapshot
	err      error
}

type runQueueWaiter struct {
	ready    chan struct{}
	priority int
	sequence uint64
}

type serverMetrics struct {
	healthzRequests              atomic.Uint64
	readyzRequests               atomic.Uint64
	metricsRequests              atomic.Uint64
	workspaceCreateRequests      atomic.Uint64
	workspaceDeleteRequests      atomic.Uint64
	workspaceListRequests        atomic.Uint64
	workspaceGetRequests         atomic.Uint64
	workspaceSnapshotRequests    atomic.Uint64
	goalDefineRequests           atomic.Uint64
	goalListRequests             atomic.Uint64
	goalGetRequests              atomic.Uint64
	dodDefineRequests            atomic.Uint64
	dodListRequests              atomic.Uint64
	dodGetRequests               atomic.Uint64
	goalDoDQualityCheckRequests  atomic.Uint64
	runCreateRequests            atomic.Uint64
	runListRequests              atomic.Uint64
	runGetRequests               atomic.Uint64
	runSnapshotRequests          atomic.Uint64
	runStatusRequests            atomic.Uint64
	runObservabilityRequests     atomic.Uint64
	artifactContentRequests      atomic.Uint64
	runTaskProgressRequests      atomic.Uint64
	runContextBundleRequests     atomic.Uint64
	runTaskDecompositionRequests atomic.Uint64
	eventListRequests            atomic.Uint64
	runStartRequests             atomic.Uint64
	runResumeRequests            atomic.Uint64
	workspaceActivationRequests  atomic.Uint64
	decisionListRequests         atomic.Uint64
	decisionGetRequests          atomic.Uint64
	decisionResolveRequests      atomic.Uint64
	approvalListRequests         atomic.Uint64
	approvalGetRequests          atomic.Uint64
	approvalApproveRequests      atomic.Uint64
	approvalRejectRequests       atomic.Uint64
	soulCreateRequests           atomic.Uint64
	soulUpdateRequests           atomic.Uint64
	soulDeleteRequests           atomic.Uint64
	soulListRequests             atomic.Uint64
	soulGetRequests              atomic.Uint64
	skillCreateRequests          atomic.Uint64
	skillListRequests            atomic.Uint64
	skillGetRequests             atomic.Uint64
	mcpServerCreateRequests      atomic.Uint64
	mcpServerListRequests        atomic.Uint64
	mcpServerGetRequests         atomic.Uint64
	mcpToolListRequests          atomic.Uint64
	mcpResourceListRequests      atomic.Uint64
	mcpPromptListRequests        atomic.Uint64
	mcpCapabilityCreateRequests  atomic.Uint64
	mcpCapabilityUpdateRequests  atomic.Uint64
	mcpCapabilityListRequests    atomic.Uint64
	mcpCapabilityGetRequests     atomic.Uint64
	memoryCreateRequests         atomic.Uint64
	memoryListRequests           atomic.Uint64
	configurationReadRequests    atomic.Uint64
	configurationUpdateRequests  atomic.Uint64
	runSucceeded                 atomic.Uint64
	runFailed                    atomic.Uint64
	runRejected                  atomic.Uint64
	runCanceled                  atomic.Uint64
	runInFlight                  atomic.Int64
	runQueueWaiting              atomic.Int64
	runQueuedTotal               atomic.Uint64
	convergenceRepairAttempts    atomic.Uint64
	runtimeMetricsMu             sync.Mutex
	observedTerminalStates       map[string]struct{}
	observedRepairAttempts       map[string]int
	schedulerDiagnostics         *schedulerDiagnosticsStore
}

// observeRunState is an SM-4 observer only. The Run owner/eventstream own the
// source state; this process-local method deduplicates terminal snapshots and
// derives monotonic repair deltas without resuming or writing any owner state.
func (m *serverMetrics) observeRunState(workspaceID, runID, state string, runtimeRevision uint64, repairAttempts int) {
	if m == nil || strings.TrimSpace(runID) == "" {
		return
	}
	state = strings.TrimSpace(state)
	m.runtimeMetricsMu.Lock()
	defer m.runtimeMetricsMu.Unlock()
	if m.observedTerminalStates == nil {
		m.observedTerminalStates = make(map[string]struct{})
	}
	if m.observedRepairAttempts == nil {
		m.observedRepairAttempts = make(map[string]int)
	}
	runKey := strings.TrimSpace(workspaceID) + "/" + strings.TrimSpace(runID)
	if repairAttempts > m.observedRepairAttempts[runKey] {
		m.convergenceRepairAttempts.Add(uint64(repairAttempts - m.observedRepairAttempts[runKey]))
		m.observedRepairAttempts[runKey] = repairAttempts
	}
	if !terminalRunState(state) {
		return
	}
	terminalKey := fmt.Sprintf("%s/%s/%d", runKey, state, runtimeRevision)
	if _, observed := m.observedTerminalStates[terminalKey]; observed {
		return
	}
	m.observedTerminalStates[terminalKey] = struct{}{}
	switch state {
	case string(common.RunStateCompleted):
		m.runSucceeded.Add(1)
	case string(common.RunStateFailed):
		m.runFailed.Add(1)
	case string(common.RunStateCancelled):
		m.runCanceled.Add(1)
	case string(common.RunStateSuperseded):
		m.runRejected.Add(1)
	}
}

func terminalRunState(state string) bool {
	switch state {
	case string(common.RunStateCompleted), string(common.RunStateFailed), string(common.RunStateCancelled), string(common.RunStateSuperseded):
		return true
	default:
		return false
	}
}

func ConfigFromEnv() (Config, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return Config{}, err
	}
	return ConfigFromWorkorch(cfg)
}

func ConfigFromWorkorch(cfg workorchconfig.Config) (Config, error) {
	readHeaderTimeout, err := durationFromValue(configServerReadHeaderTimeout, cfg.Server.ReadHeaderTimeout, defaultReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := durationFromValue(configServerReadTimeout, cfg.Server.ReadTimeout, defaultReadTimeout)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := durationFromValue(configServerWriteTimeout, cfg.Server.WriteTimeout, defaultWriteTimeout)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := durationFromValue(configServerIdleTimeout, cfg.Server.IdleTimeout, defaultIdleTimeout)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationFromValue(configServerShutdownTimeout, cfg.Server.ShutdownTimeout, defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	staleDispatchedStepMinAge, err := durationFromValue(configServerStaleDispatchedStepMinAge, cfg.Server.StaleDispatchedStepMinAge, defaultStaleDispatchedStepMinAge)
	if err != nil {
		return Config{}, err
	}
	maxConcurrentRuns := cfg.Server.MaxConcurrentRuns
	if maxConcurrentRuns == 0 {
		maxConcurrentRuns = defaultMaxConcurrentRuns
	}
	if maxConcurrentRuns < 0 {
		return Config{}, fmt.Errorf("invalid integer for %s: must be positive", configServerMaxConcurrentRuns)
	}
	return Config{
		Addr:                      firstNonEmpty(cfg.Server.Addr, defaultServerAddr),
		Backend:                   cfg.Repository.Backend,
		DataRoot:                  cfg.Repository.DataDir,
		ReadHeaderTimeout:         readHeaderTimeout,
		ReadTimeout:               readTimeout,
		WriteTimeout:              writeTimeout,
		IdleTimeout:               idleTimeout,
		ShutdownTimeout:           shutdownTimeout,
		MaxConcurrentRuns:         maxConcurrentRuns,
		StaleDispatchedStepMinAge: staleDispatchedStepMinAge,
	}, nil
}

func ConfigFromValues(values map[string]string) (Config, error) {
	addr := values[configServerAddr]
	if addr == "" {
		addr = defaultServerAddr
	}
	readHeaderTimeout, err := durationFromValues(values, configServerReadHeaderTimeout, defaultReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := durationFromValues(values, configServerReadTimeout, defaultReadTimeout)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := durationFromValues(values, configServerWriteTimeout, defaultWriteTimeout)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := durationFromValues(values, configServerIdleTimeout, defaultIdleTimeout)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationFromValues(values, configServerShutdownTimeout, defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}
	maxConcurrentRuns, err := int64FromValues(values, configServerMaxConcurrentRuns, defaultMaxConcurrentRuns)
	if err != nil {
		return Config{}, err
	}
	staleDispatchedStepMinAge, err := durationFromValues(values, configServerStaleDispatchedStepMinAge, defaultStaleDispatchedStepMinAge)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Addr:                      addr,
		Backend:                   values["repository.backend"],
		DataRoot:                  values["repository.dataDir"],
		ReadHeaderTimeout:         readHeaderTimeout,
		ReadTimeout:               readTimeout,
		WriteTimeout:              writeTimeout,
		IdleTimeout:               idleTimeout,
		ShutdownTimeout:           shutdownTimeout,
		MaxConcurrentRuns:         maxConcurrentRuns,
		StaleDispatchedStepMinAge: staleDispatchedStepMinAge,
	}, nil
}

func normalizeConfig(config Config) Config {
	if config.Addr == "" {
		config.Addr = defaultServerAddr
	}
	if config.ReadHeaderTimeout <= 0 {
		config.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if config.ReadTimeout <= 0 {
		config.ReadTimeout = defaultReadTimeout
	}
	if config.WriteTimeout <= 0 {
		config.WriteTimeout = defaultWriteTimeout
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = defaultIdleTimeout
	}
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = defaultShutdownTimeout
	}
	if config.MaxConcurrentRuns <= 0 {
		config.MaxConcurrentRuns = defaultMaxConcurrentRuns
	}
	if config.StaleDispatchedStepMinAge <= 0 {
		config.StaleDispatchedStepMinAge = defaultStaleDispatchedStepMinAge
	}
	return config
}

func NewHTTPServer(config Config) *HTTPServer {
	config = normalizeConfig(config)
	session, err := appservice.NewRuntimeSessionWithBackend(config.Backend, config.DataRoot)
	if err != nil {
		return newHTTPServerWithRuntimeSessionAndBackground(config, nil, nil, err)
	}
	return NewHTTPServerWithRuntimeSessionAndBackground(config, session, nil)
}

// NewHTTPServerWithRuntimeSessionAndBackground constructs the protocol
// adapter from a session-owned application boundary. The server never retains
// or dispatches through EventHub directly.
func NewHTTPServerWithRuntimeSessionAndBackground(config Config, session *appservice.RuntimeSession, backgroundRoutine task.BackgroundRoutine) *HTTPServer {
	return newHTTPServerWithRuntimeSessionAndBackground(normalizeConfig(config), session, backgroundRoutine, nil)
}

func newHTTPServerWithRuntimeSessionAndBackground(config Config, session *appservice.RuntimeSession, backgroundRoutine task.BackgroundRoutine, sessionErr error) *HTTPServer {
	config = normalizeConfig(config)
	state := newServerStateWithRuntimeSessionAndBackground(config, session, backgroundRoutine, sessionErr)
	handler := newMuxWithStateNoStart(state)
	return &HTTPServer{
		config: config,
		state:  state,
		http: &http.Server{
			Addr:              config.Addr,
			Handler:           handler,
			ReadHeaderTimeout: config.ReadHeaderTimeout,
			ReadTimeout:       config.ReadTimeout,
			WriteTimeout:      config.WriteTimeout,
			IdleTimeout:       config.IdleTimeout,
		},
	}
}

func (s *HTTPServer) Start() error {
	if s == nil || s.http == nil {
		return errors.New("http server is not configured")
	}
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *HTTPServer) Validate(ctx context.Context) error {
	if s == nil {
		return errors.New("http server is not configured")
	}
	return s.state.validate(ctx, s.config.Backend, s.config.DataRoot)
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	err := s.http.Shutdown(ctx)
	if err != nil && ctx != nil && errors.Is(err, context.DeadlineExceeded) {
		_ = s.http.Close()
	}
	return errors.Join(err, s.state.close(ctx))
}

func newMux(config Config) http.Handler {
	return NewHTTPServer(config).http.Handler
}

func newMuxWithState(state *serverState) http.Handler {
	_ = state.startWorkspaceProjection(state.backgroundContext())
	return newMuxWithStateNoStart(state)
}

func newMuxWithStateNoStart(state *serverState) http.Handler {
	router := engine.NewRouteRegistry()
	registerSystemRoutes(router, state)
	registerConfigurationRoutes(router, state)
	registerReadRoutes(router, state)
	registerBusinessWriteRoutes(router, state)
	registerConsoleRoutes(router)
	engineServer := engine.NewHTTPServer()
	engineServer.Bind(router)
	handler, ok := engineServer.(http.Handler)
	if !ok {
		return http.NotFoundHandler()
	}
	return handler
}

func newServerStateWithRuntimeSessionAndBackground(config Config, session *appservice.RuntimeSession, backgroundRoutine task.BackgroundRoutine, sessionErr error) *serverState {
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	diagnostics := newSchedulerDiagnosticsStore()
	state := &serverState{
		config:                       config,
		eventReplay:                  newSSEReplayBuffer(defaultSSEReplayLimit),
		streamSubscribers:            make(map[uint64]chan streamEvent),
		metrics:                      &serverMetrics{schedulerDiagnostics: diagnostics, observedTerminalStates: make(map[string]struct{}), observedRepairAttempts: make(map[string]int)},
		validate:                     defaultValidate,
		createWorkspace:              defaultCreateWorkspace,
		deleteWorkspace:              defaultDeleteWorkspace,
		readWorkspace:                defaultReadWorkspace,
		readWorkspaceSnapshot:        defaultReadWorkspaceSnapshot,
		readWorkspaceDetailSnapshot:  defaultReadWorkspaceDetailSnapshot,
		readProjectionSnapshot:       defaultReadWorkspaceProjectionSnapshot,
		readWorkspaceSoulBindings:    defaultReadWorkspaceSoulBindings,
		updateWorkspaceSoulBindings:  defaultUpdateWorkspaceSoulBindings,
		listWorkspaces:               defaultListWorkspaces,
		defineGoal:                   defaultDefineGoal,
		updateGoal:                   defaultUpdateGoal,
		readGoal:                     defaultReadGoal,
		listGoals:                    defaultListGoals,
		defineDoD:                    defaultDefineDoD,
		updateDoD:                    defaultUpdateDoD,
		readDoD:                      defaultReadDoD,
		listDoDs:                     defaultListDoDs,
		checkGoalDoDQuality:          defaultCheckGoalDoDQuality,
		generateWorkspaceSetupDraft:  defaultGenerateWorkspaceSetupDraft,
		createRun:                    defaultCreateRun,
		readRun:                      defaultReadRun,
		readRunSnapshot:              defaultReadRunSnapshot,
		listRunSteps:                 defaultListRunSteps,
		listRunToolCalls:             defaultListRunToolCalls,
		listRunRuntimeCheckpoints:    defaultListRunRuntimeCheckpoints,
		listRunResults:               defaultListRunResults,
		listRunPatches:               defaultListRunPatches,
		listRunValidationResults:     defaultListRunValidationResults,
		listRunArtifacts:             defaultListRunArtifacts,
		readRunStatus:                defaultReadRunStatus,
		readRunObservability:         defaultReadRunObservability,
		readArtifactContent:          defaultReadArtifactContent,
		readRuntimeRawContent:        defaultReadRuntimeRawContent,
		readRunTaskProgress:          defaultReadRunTaskProgress,
		readRunContextBundle:         defaultReadRunContextBundle,
		readRunTaskDecompositions:    defaultReadRunTaskDecompositions,
		listMemoryRecallTraces:       defaultListMemoryRecallTraces,
		listEventRecords:             defaultListEventRecords,
		listRuns:                     defaultListRuns,
		startRun:                     defaultStartRun,
		resumeRun:                    defaultResumeRun,
		resumeWorkspaceExecution:     defaultResumeWorkspaceExecution,
		listDecisions:                defaultListDecisions,
		listDecisionPage:             defaultListDecisionPage,
		readDecision:                 defaultReadDecision,
		resolveDecision:              defaultResolveDecision,
		batchResolveDecisions:        defaultBatchResolveDecisions,
		listApprovals:                defaultListApprovals,
		listApprovalPage:             defaultListApprovalPage,
		readApproval:                 defaultReadApproval,
		approveApproval:              defaultApproveApproval,
		rejectApproval:               defaultRejectApproval,
		batchResolveApprovals:        defaultBatchResolveApprovals,
		createSoul:                   defaultCreateSoul,
		updateSoul:                   defaultUpdateSoul,
		deleteSoul:                   defaultDeleteSoul,
		readSoul:                     defaultReadSoul,
		listSouls:                    defaultListSouls,
		createWorkspaceTemplate:      defaultCreateWorkspaceTemplate,
		updateWorkspaceTemplate:      defaultUpdateWorkspaceTemplate,
		deleteWorkspaceTemplate:      defaultDeleteWorkspaceTemplate,
		listWorkspaceTemplates:       defaultListWorkspaceTemplates,
		createSkill:                  defaultCreateSkill,
		readSkill:                    defaultReadSkill,
		listSkills:                   defaultListSkills,
		createMCPServer:              defaultCreateMCPServer,
		updateMCPServer:              defaultUpdateMCPServer,
		readMCPServer:                defaultReadMCPServer,
		listMCPServers:               defaultListMCPServers,
		listMCPServerTools:           defaultListMCPServerTools,
		listMCPServerResources:       defaultListMCPServerResources,
		listMCPServerPrompts:         defaultListMCPServerPrompts,
		createMCPCapability:          defaultCreateMCPCapability,
		updateMCPCapability:          defaultUpdateMCPCapability,
		readMCPCapability:            defaultReadMCPCapability,
		listMCPCapabilities:          defaultListMCPCapabilities,
		createMemoryItem:             defaultCreateMemoryItem,
		readMemoryItem:               defaultReadMemoryItem,
		updateMemoryItem:             defaultUpdateMemoryItem,
		deleteMemoryItem:             defaultDeleteMemoryItem,
		listMemoryByWorkspace:        defaultListMemoryByWorkspace,
		searchMemory:                 defaultSearchMemory,
		readConfiguration:            defaultReadConfiguration,
		updateServerConfiguration:    appservice.UpdateServerConfiguration,
		updateRuntimeConfiguration:   appservice.UpdateRuntimeConfiguration,
		updateLLMConfiguration:       appservice.UpdateLLMConfiguration,
		updateWebSearchConfiguration: appservice.UpdateWebSearchConfiguration,
		backgroundRoutine:            backgroundRoutine,
		backgroundCtx:                backgroundCtx,
		backgroundCancel:             backgroundCancel,
		schedulerDiagnostics:         diagnostics,
		workspaceSetupDraftSessions:  map[string]*workspaceSetupDraftSession{},
	}
	state.workspaceProjection = newWorkspaceProjectionManager(state)
	if sessionErr != nil {
		state.runtimeSessionErr = fmt.Errorf("workorch runtime session unavailable: %w", sessionErr)
		state.validate = func(context.Context, string, string) error { return state.runtimeSessionErr }
		return state
	}
	if session == nil {
		state.runtimeSessionErr = errors.New("workorch runtime session is not configured")
		state.validate = func(context.Context, string, string) error { return state.runtimeSessionErr }
		return state
	}
	state.configureSessionBackedOperations(session)
	return state
}

func (s *serverState) configureSessionBackedOperations(session *appservice.RuntimeSession) {
	if s == nil || session == nil {
		return
	}
	s.runtimeSessionErr = nil
	s.setSharedSession(session)
	s.getLLMModelCatalog = func(ctx context.Context, cmd appservice.GetLLMModelCatalogCommand) (appservice.ProviderCatalogSummary, error) {
		var ret appservice.ProviderCatalogSummary
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.GetLLMModelCatalogWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.refreshLLMModelCatalog = func(ctx context.Context, cmd appservice.RefreshLLMModelCatalogCommand) (appservice.ProviderCatalogSummary, error) {
		var ret appservice.ProviderCatalogSummary
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.RefreshLLMModelCatalogWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.readConfigurationWithCatalog = func(ctx context.Context) (appservice.ServiceConfiguration, error) {
		var ret appservice.ServiceConfiguration
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadServiceConfigurationWithCatalogWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.validate = func(ctx context.Context, _, _ string) error {
		return s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			_, err := session.Exports()
			return err
		})
	}
	s.createWorkspace = func(ctx context.Context, _, _ string, cmd appservice.CreateWorkspaceCommand) (appservice.Workspace, error) {
		var ret appservice.Workspace
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateWorkspaceWithTemplateWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.deleteWorkspace = func(ctx context.Context, _, _, workspaceID string, cmd appservice.DeleteWorkspaceCommand) error {
		return s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			return appservice.DeleteWorkspaceWithSession(ctx, session, workspaceID, cmd)
		})
	}
	s.readWorkspace = func(ctx context.Context, _, _, workspaceID string) (appservice.Workspace, error) {
		var ret appservice.Workspace
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readWorkspaceSnapshot = func(ctx context.Context, _, _, workspaceID string) (appservice.WorkspaceSnapshot, error) {
		var ret appservice.WorkspaceSnapshot
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readWorkspaceDetailSnapshot = func(ctx context.Context, _, _, workspaceID string) (appservice.WorkspaceSnapshot, error) {
		var ret appservice.WorkspaceSnapshot
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceDetailSnapshotWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readProjectionSnapshot = func(ctx context.Context, _, _, workspaceID string) (appservice.WorkspaceSnapshot, error) {
		var ret appservice.WorkspaceSnapshot
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceProjectionSnapshotWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readWorkspaceSoulBindings = func(ctx context.Context, _, _, workspaceID string) (appservice.WorkspaceSoulBindings, error) {
		var ret appservice.WorkspaceSoulBindings
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceSoulBindingsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.updateWorkspaceSoulBindings = func(ctx context.Context, _, _, workspaceID string, cmd appservice.WorkspaceSoulBindings) (appservice.WorkspaceSoulBindings, error) {
		var ret appservice.WorkspaceSoulBindings
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.UpdateWorkspaceSoulBindingsWithSession(ctx, session, workspaceID, cmd)
			return err
		})
		return ret, err
	}
	s.listWorkspaces = func(ctx context.Context, _, _ string) ([]appservice.Workspace, error) {
		var ret []appservice.Workspace
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListWorkspacesWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.defineGoal = func(ctx context.Context, _, _, workspaceID, text string) (appservice.Goal, error) {
		var ret appservice.Goal
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.DefineGoalWithSession(ctx, session, workspaceID, text)
			return err
		})
		return ret, err
	}
	s.updateGoal = func(ctx context.Context, _, _, goalID, text string) (appservice.Goal, error) {
		var ret appservice.Goal
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.UpdateGoalWithSession(ctx, session, goalID, text)
			return err
		})
		return ret, err
	}
	s.readGoal = func(ctx context.Context, _, _, goalID string) (appservice.Goal, error) {
		var ret appservice.Goal
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadGoalWithSession(ctx, session, goalID)
			return err
		})
		return ret, err
	}
	s.listGoals = func(ctx context.Context, _, _, workspaceID string) ([]appservice.Goal, error) {
		var ret []appservice.Goal
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListGoalsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.defineDoD = func(ctx context.Context, _, _, workspaceID string, items []string) (appservice.DoD, error) {
		var ret appservice.DoD
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.DefineDoDWithSession(ctx, session, workspaceID, items)
			return err
		})
		return ret, err
	}
	s.updateDoD = func(ctx context.Context, _, _, dodID string, items []string) (appservice.DoD, error) {
		var ret appservice.DoD
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.UpdateDoDWithSession(ctx, session, dodID, items)
			return err
		})
		return ret, err
	}
	s.readDoD = func(ctx context.Context, _, _, dodID string) (appservice.DoD, error) {
		var ret appservice.DoD
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadDoDWithSession(ctx, session, dodID)
			return err
		})
		return ret, err
	}
	s.listDoDs = func(ctx context.Context, _, _, workspaceID string) ([]appservice.DoD, error) {
		var ret []appservice.DoD
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListDoDsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.checkGoalDoDQuality = func(ctx context.Context, _, _ string, cmd appservice.GoalDoDQualityCheckCommand) (appservice.GoalDoDQualityCheck, error) {
		var ret appservice.GoalDoDQualityCheck
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CheckGoalDoDQualityWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.generateWorkspaceSetupDraft = func(ctx context.Context, _, _ string, cmd appservice.WorkspaceSetupDraftGenerationCommand) (appservice.WorkspaceSetupDraft, error) {
		var ret appservice.WorkspaceSetupDraft
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.GenerateWorkspaceSetupDraftWithSession(ctx, session, cmd, nil)
			return err
		})
		return ret, err
	}
	s.createRun = func(ctx context.Context, _, _, workspaceID, runID, goalID, dodID string) (appservice.Run, error) {
		var ret appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateRunWithSession(ctx, session, workspaceID, runID, goalID, dodID)
			return err
		})
		return ret, err
	}
	s.readRun = func(ctx context.Context, _, _, runID string) (appservice.Run, error) {
		var ret appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readRunSnapshot = func(ctx context.Context, _, _, runID string) (appservice.RunSnapshot, error) {
		var ret appservice.RunSnapshot
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunSnapshotWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.listRunSteps = func(ctx context.Context, _, _ string, query appservice.StepQuery) (appservice.StepPage, error) {
		var ret appservice.StepPage
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListRunStepsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunToolCalls = func(ctx context.Context, _, _ string, query appservice.ToolCallQuery) (appservice.ToolCallPage, error) {
		var ret appservice.ToolCallPage
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListRunToolCallsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunRuntimeCheckpoints = func(ctx context.Context, _, _ string, query appservice.RuntimeCheckpointQuery) (appservice.RuntimeCheckpointPage, error) {
		var ret appservice.RuntimeCheckpointPage
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListRunRuntimeCheckpointsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunResults = func(ctx context.Context, _, _ string, query appservice.ResultQuery) (appservice.ResultPage, error) {
		var ret appservice.ResultPage
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListRunResultsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunPatches = func(ctx context.Context, _, _ string, query appservice.PatchQuery) (appservice.PatchPage, error) {
		var ret appservice.PatchPage
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListRunPatchesWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunValidationResults = func(ctx context.Context, _, _ string, query appservice.ValidationResultQuery) (appservice.ValidationResultPage, error) {
		var ret appservice.ValidationResultPage
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListRunValidationResultsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRunArtifacts = func(ctx context.Context, _, _ string, query appservice.ArtifactQuery) (appservice.ArtifactPage, error) {
		var ret appservice.ArtifactPage
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListRunArtifactsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.readRunStatus = func(ctx context.Context, _, _, runID string) (appservice.RunStatus, error) {
		var ret appservice.RunStatus
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunStatusWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readRunObservability = func(ctx context.Context, _, _, runID string) (appservice.RunObservability, error) {
		var ret appservice.RunObservability
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunObservabilityWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readArtifactContent = func(ctx context.Context, _, _, artifactURI string) (appservice.ArtifactContent, error) {
		var ret appservice.ArtifactContent
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadArtifactContentWithSession(ctx, session, artifactURI)
			return err
		})
		return ret, err
	}
	s.readRuntimeRawContent = func(ctx context.Context, _, _, ref string) (appservice.RuntimeRawContent, error) {
		var ret appservice.RuntimeRawContent
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRuntimeRawContentWithSession(ctx, session, ref)
			return err
		})
		return ret, err
	}
	s.readRunTaskProgress = func(ctx context.Context, _, _, runID string) (appservice.TaskProgress, error) {
		var ret appservice.TaskProgress
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunTaskProgressWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readRunContextBundle = func(ctx context.Context, _, _, runID string) (appservice.ContextBundle, error) {
		var ret appservice.ContextBundle
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunContextBundleWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.readRunTaskDecompositions = func(ctx context.Context, _, _, runID string) ([]appservice.TaskDecomposition, error) {
		var ret []appservice.TaskDecomposition
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadRunTaskDecompositionsWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.listEventRecords = func(ctx context.Context, _, _ string, query appservice.EventRecordQuery) ([]appservice.EventRecord, error) {
		var ret []appservice.EventRecord
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListEventRecordsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listRuns = func(ctx context.Context, _, _, workspaceID string) ([]appservice.Run, error) {
		var ret []appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListRunsWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.startRun = func(ctx context.Context, _, _, runID string) (appservice.Run, error) {
		var ret appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.StartRunWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.resumeRun = func(ctx context.Context, _, _, runID string) (appservice.Run, error) {
		var ret appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ResumeRunWithSession(ctx, session, runID)
			return err
		})
		return ret, err
	}
	s.resumeWorkspaceExecution = func(ctx context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		var ret appservice.WorkspaceExecutionActivation
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ActivateWorkspaceExecutionWithSession(ctx, session, workspaceID, cmd)
			return err
		})
		return ret, err
	}
	s.listDecisions = func(ctx context.Context, _, _, workspaceID, runID string) ([]appservice.Decision, error) {
		var ret []appservice.Decision
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListDecisionsWithSession(ctx, session, workspaceID, runID)
			return err
		})
		return ret, err
	}
	s.listDecisionPage = func(ctx context.Context, _, _ string, query appservice.DecisionQuery) (appservice.DecisionPage, error) {
		var ret appservice.DecisionPage
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListDecisionPageWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.readDecision = func(ctx context.Context, _, _, ticketID string) (appservice.Decision, error) {
		var ret appservice.Decision
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadDecisionWithSession(ctx, session, ticketID)
			return err
		})
		return ret, err
	}
	s.resolveDecision = func(ctx context.Context, _, _, ticketID string, cmd appservice.ResolveDecisionCommand) (appservice.Decision, error) {
		var ret appservice.Decision
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ResolveDecisionWithSession(ctx, session, ticketID, cmd)
			return err
		})
		return ret, err
	}
	s.batchResolveDecisions = func(ctx context.Context, _, _ string, cmd appservice.BatchResolveDecisionCommand) ([]appservice.Decision, error) {
		var ret []appservice.Decision
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.BatchResolveDecisionsWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.listApprovals = func(ctx context.Context, _, _, workspaceID, runID string) ([]appservice.Approval, error) {
		var ret []appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListApprovalsWithSession(ctx, session, workspaceID, runID)
			return err
		})
		return ret, err
	}
	s.listApprovalPage = func(ctx context.Context, _, _ string, query appservice.ApprovalQuery) (appservice.ApprovalPage, error) {
		var ret appservice.ApprovalPage
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListApprovalPageWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.readApproval = func(ctx context.Context, _, _, approvalID string) (appservice.Approval, error) {
		var ret appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadApprovalWithSession(ctx, session, approvalID)
			return err
		})
		return ret, err
	}
	s.approveApproval = func(ctx context.Context, _, _, approvalID string, cmd appservice.ResolveApprovalCommand) (appservice.Approval, error) {
		var ret appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			if cmd.Action == "" {
				cmd.Action = "approve"
			}
			ret, err = appservice.ResolveApprovalWithSession(ctx, session, approvalID, cmd)
			return err
		})
		return ret, err
	}
	s.rejectApproval = func(ctx context.Context, _, _, approvalID string, cmd appservice.ResolveApprovalCommand) (appservice.Approval, error) {
		var ret appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			if cmd.Action == "" {
				cmd.Action = "reject"
			}
			ret, err = appservice.ResolveApprovalWithSession(ctx, session, approvalID, cmd)
			return err
		})
		return ret, err
	}
	s.batchResolveApprovals = func(ctx context.Context, _, _ string, cmd appservice.BatchResolveApprovalCommand) ([]appservice.Approval, error) {
		var ret []appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.BatchResolveApprovalsWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.createSoul = func(ctx context.Context, _, _ string, cmd appservice.CreateSoulCommand) (appservice.Soul, error) {
		var ret appservice.Soul
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateSoulWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.updateSoul = func(ctx context.Context, _, _ string, soulID string, cmd appservice.UpdateSoulCommand) (appservice.Soul, error) {
		var ret appservice.Soul
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.UpdateSoulWithSession(ctx, session, soulID, cmd)
			return err
		})
		return ret, err
	}
	s.deleteSoul = func(ctx context.Context, _, _, soulID string) error {
		return s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			return appservice.DeleteSoulWithSession(ctx, session, soulID)
		})
	}
	s.deleteWorkspaceTemplate = func(ctx context.Context, _, _, templateID string) error {
		return s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			return appservice.DeleteWorkspaceTemplateWithSession(ctx, session, templateID)
		})
	}
	s.readSoul = func(ctx context.Context, _, _, soulID string) (appservice.Soul, error) {
		var ret appservice.Soul
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadSoulWithSession(ctx, session, soulID)
			return err
		})
		return ret, err
	}
	s.listSouls = func(ctx context.Context, _, _ string) ([]appservice.Soul, error) {
		var ret []appservice.Soul
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListSoulsWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.createWorkspaceTemplate = func(ctx context.Context, _, _ string, cmd appservice.WorkspaceTemplate) (appservice.WorkspaceTemplate, error) {
		var ret appservice.WorkspaceTemplate
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateWorkspaceTemplateWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.updateWorkspaceTemplate = func(ctx context.Context, _, _ string, templateID string, cmd appservice.WorkspaceTemplate) (appservice.WorkspaceTemplate, error) {
		var ret appservice.WorkspaceTemplate
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.UpdateWorkspaceTemplateWithSession(ctx, session, templateID, cmd)
			return err
		})
		return ret, err
	}
	s.listWorkspaceTemplates = func(ctx context.Context, _, _ string) ([]appservice.WorkspaceTemplate, error) {
		var ret []appservice.WorkspaceTemplate
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListWorkspaceTemplatesWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.createSkill = func(ctx context.Context, _, _ string, cmd appservice.CreateSkillCommand) (appservice.Skill, error) {
		var ret appservice.Skill
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateSkillWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.readSkill = func(ctx context.Context, _, _, skillID string, query appservice.SkillQuery) (appservice.Skill, error) {
		var ret appservice.Skill
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadSkillWithSession(ctx, session, skillID, query)
			return err
		})
		return ret, err
	}
	s.listSkills = func(ctx context.Context, _, _ string, query appservice.SkillQuery) ([]appservice.Skill, error) {
		var ret []appservice.Skill
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListSkillsWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.createMCPServer = func(ctx context.Context, _, _ string, cmd appservice.CreateMCPServerCommand) (appservice.MCPServer, error) {
		var ret appservice.MCPServer
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateMCPServerWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.updateMCPServer = func(ctx context.Context, _, _ string, serverID string, cmd appservice.UpdateMCPServerCommand) (appservice.MCPServer, error) {
		var ret appservice.MCPServer
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.UpdateMCPServerWithSession(ctx, session, serverID, cmd)
			return err
		})
		return ret, err
	}
	s.readMCPServer = func(ctx context.Context, _, _, serverID string) (appservice.MCPServer, error) {
		var ret appservice.MCPServer
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadMCPServerWithSession(ctx, session, serverID)
			return err
		})
		return ret, err
	}
	s.listMCPServers = func(ctx context.Context, _, _ string) ([]appservice.MCPServer, error) {
		var ret []appservice.MCPServer
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListMCPServersWithSession(ctx, session)
			return err
		})
		return ret, err
	}
	s.listMCPServerTools = func(ctx context.Context, _, _, serverID string) ([]appservice.MCPTool, error) {
		var ret []appservice.MCPTool
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListMCPServerToolsWithSession(ctx, session, serverID)
			return err
		})
		return ret, err
	}
	s.listMCPServerResources = func(ctx context.Context, _, _, serverID string) ([]appservice.MCPResource, error) {
		var ret []appservice.MCPResource
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListMCPServerResourcesWithSession(ctx, session, serverID)
			return err
		})
		return ret, err
	}
	s.listMCPServerPrompts = func(ctx context.Context, _, _, serverID string) ([]appservice.MCPPrompt, error) {
		var ret []appservice.MCPPrompt
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListMCPServerPromptsWithSession(ctx, session, serverID)
			return err
		})
		return ret, err
	}
	s.createMCPCapability = func(ctx context.Context, _, _ string, cmd appservice.CreateMCPCapabilityCommand) (appservice.MCPCapability, error) {
		var ret appservice.MCPCapability
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateMCPCapabilityWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	s.updateMCPCapability = func(ctx context.Context, _, _, capabilityID string, cmd appservice.UpdateMCPCapabilityCommand) (appservice.MCPCapability, error) {
		var ret appservice.MCPCapability
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.UpdateMCPCapabilityWithSession(ctx, session, capabilityID, cmd)
			return err
		})
		return ret, err
	}
	s.readMCPCapability = func(ctx context.Context, _, _, capabilityID string) (appservice.MCPCapability, error) {
		var ret appservice.MCPCapability
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadMCPCapabilityWithSession(ctx, session, capabilityID)
			return err
		})
		return ret, err
	}
	s.listMCPCapabilities = func(ctx context.Context, _, _ string, query appservice.MCPCapabilityQuery) ([]appservice.MCPCapability, error) {
		var ret []appservice.MCPCapability
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListMCPCapabilitiesWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.createMemoryItem = func(ctx context.Context, _, _, workspaceID string, cmd appservice.CreateMemoryItemCommand) (appservice.MemoryItem, error) {
		var ret appservice.MemoryItem
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.CreateMemoryItemWithSession(ctx, session, workspaceID, cmd)
			return err
		})
		return ret, err
	}
	s.readMemoryItem = func(ctx context.Context, _, _, memoryID string) (appservice.MemoryItem, error) {
		var ret appservice.MemoryItem
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadMemoryItemWithSession(ctx, session, memoryID)
			return err
		})
		return ret, err
	}
	s.updateMemoryItem = func(ctx context.Context, _, _, memoryID string, cmd appservice.UpdateMemoryItemCommand) (appservice.MemoryItem, error) {
		var ret appservice.MemoryItem
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.UpdateMemoryItemWithSession(ctx, session, memoryID, cmd)
			return err
		})
		return ret, err
	}
	s.deleteMemoryItem = func(ctx context.Context, _, _, memoryID string) error {
		return s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			return appservice.DeleteMemoryItemWithSession(ctx, session, memoryID)
		})
	}
	s.listMemoryByWorkspace = func(ctx context.Context, _, _, workspaceID string, query string) ([]appservice.MemoryItem, error) {
		var ret []appservice.MemoryItem
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListMemoryByWorkspaceWithSession(ctx, session, workspaceID, query)
			return err
		})
		return ret, err
	}
	s.searchMemory = func(ctx context.Context, _, _ string, query appservice.MemoryQuery) ([]appservice.MemoryItem, error) {
		var ret []appservice.MemoryItem
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.SearchMemoryWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	s.listMemoryRecallTraces = func(ctx context.Context, _, _ string, query appservice.MemoryRecallTraceQuery) ([]appservice.MemoryRecallTrace, error) {
		var ret []appservice.MemoryRecallTrace
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ListMemoryRecallTracesWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
}

func (s *serverState) recordStreamEvent(envelope streamEvent) {
	if s == nil {
		return
	}
	if s.eventReplay != nil {
		s.eventReplay.Append(envelope)
	}
	s.streamSubscribersMu.Lock()
	defer s.streamSubscribersMu.Unlock()
	for _, ch := range s.streamSubscribers {
		enqueueEventStreamEnvelope(ch, envelope)
	}
}

func (s *serverState) subscribeStreamEvents() (<-chan streamEvent, func()) {
	if s == nil {
		return nil, func() {}
	}
	ch := make(chan streamEvent, 128)
	s.streamSubscribersMu.Lock()
	s.streamSubscriberSequence++
	id := s.streamSubscriberSequence
	s.streamSubscribers[id] = ch
	s.streamSubscribersMu.Unlock()
	return ch, func() {
		s.streamSubscribersMu.Lock()
		delete(s.streamSubscribers, id)
		s.streamSubscribersMu.Unlock()
	}
}

func isMemoryBackend(backend string) bool {
	return backend == "memory"
}

func enqueueEventStreamEnvelope(ch chan streamEvent, envelope streamEvent) {
	if ch == nil {
		return
	}
	select {
	case ch <- envelope:
		return
	default:
	}
	if !isEventStreamCriticalTopic(envelope.Topic) {
		return
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- envelope:
	default:
	}
}

func isEventStreamCriticalTopic(topic string) bool {
	switch topic {
	case workorchdevents.TopicWorkspaceProjectionChanged,
		workspaceevents.TopicWorkspaceCreated,
		workspaceevents.TopicWorkspaceDeleted,
		workspaceevents.TopicWorkspaceStateChanged,
		runevents.TopicRunStateChanged:
		return true
	default:
		return false
	}
}

func writeSSEEnvelope(w http.ResponseWriter, flusher http.Flusher, envelope streamEvent) bool {
	// Projection notifications must serialize as the concrete top-level DTO so
	// remote decodeProjectionSSE can consume live and replay frames identically.
	// streamEvent.Revision is a scalar and would conflict with nested revision.
	if dto, ok := workspaceProjectionChangedSSEFromStreamEvent(envelope); ok {
		return writeProjectionSSE(w, flusher, dto)
	}
	data, err := json.Marshal(enrichSSEEnvelope(envelope))
	if err != nil {
		return true
	}
	if strings.TrimSpace(envelope.ID) != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", envelope.ID); err != nil {
			return false
		}
	}
	if _, err := fmt.Fprint(w, "event: workorch\n"); err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

func (s *serverState) withSharedSession(ctx context.Context, fn func(*appservice.RuntimeSession) error) error {
	if s == nil {
		return common.ErrInvalidArgument
	}
	s.sharedSessionMu.RLock()
	defer s.sharedSessionMu.RUnlock()
	if s.sharedSession == nil {
		return common.ErrInvalidArgument
	}
	session := s.sharedSession.Borrow()
	err := fn(session)
	return errors.Join(err, session.Close(ctx))
}

func (s *serverState) hasSharedSession() bool {
	if s == nil {
		return false
	}
	s.sharedSessionMu.RLock()
	defer s.sharedSessionMu.RUnlock()
	return s.sharedSession != nil
}

func (s *serverState) setSharedSession(session *appservice.RuntimeSession) {
	if s == nil {
		return
	}
	s.sharedSessionMu.Lock()
	defer s.sharedSessionMu.Unlock()
	s.sharedSession = session
}

func (s *serverState) close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error
	if s.workspaceProjection != nil {
		s.workspaceProjection.Close()
	}
	if s.backgroundCancel != nil {
		s.backgroundCancel()
	}
	for _, unsubscribe := range s.eventUnsubscribes {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
	s.eventUnsubscribes = nil
	// Each teardown stage gets its own deadline so an earlier timeout cannot leave
	// later Close/Terminate/Drain calls running against an already-cancelled ctx
	// (which can hang under full-suite load).
	waitCtx, waitCancel := context.WithTimeout(context.Background(), closeStageTimeout(ctx))
	defer waitCancel()
	if err := s.waitBackground(waitCtx); err != nil {
		errs = append(errs, err)
	}
	s.sharedSessionMu.Lock()
	session := s.sharedSession
	s.sharedSession = nil
	s.sharedSessionMu.Unlock()
	if session != nil {
		sessionCtx, sessionCancel := context.WithTimeout(context.Background(), closeStageTimeout(ctx))
		errs = append(errs, session.Close(sessionCtx))
		sessionCancel()
	}
	sharedCtx, sharedCancel := context.WithTimeout(context.Background(), closeStageTimeout(ctx))
	defer sharedCancel()
	errs = append(errs, appservice.CloseSharedBackendSession(sharedCtx, s.config.Backend, s.config.DataRoot))
	return errors.Join(errs...)
}

func closeStageTimeout(ctx context.Context) time.Duration {
	const defaultTimeout = 5 * time.Second
	if ctx == nil {
		return defaultTimeout
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return defaultTimeout
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return defaultTimeout
	}
	if remaining > defaultTimeout {
		return defaultTimeout
	}
	// Keep a usable floor so parallel suite cleanup still terminates.
	if remaining < 500*time.Millisecond {
		return 500 * time.Millisecond
	}
	return remaining
}

func (s *serverState) waitBackground(ctx context.Context) error {
	if s == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		s.backgroundWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *serverState) scheduleBackground(name string, fn func(context.Context)) bool {
	if s == nil || fn == nil {
		return false
	}
	ctx := s.backgroundContext()
	s.backgroundWG.Add(1)
	run := func() {
		defer s.backgroundWG.Done()
		fn(ctx)
	}
	if s.backgroundRoutine != nil {
		if err := s.backgroundRoutine.AsyncFunction(run); err != nil {
			s.backgroundWG.Done()
			slog.Warn("workorch background task scheduling failed", "task", name, "err", err)
			return false
		}
		return true
	}
	s.backgroundWG.Done()
	slog.Warn("workorch background task scheduling skipped because background routine is not configured", "task", name)
	return false
}

func (s *serverState) backgroundContext() context.Context {
	if s != nil && s.backgroundCtx != nil {
		return s.backgroundCtx
	}
	return context.Background()
}
