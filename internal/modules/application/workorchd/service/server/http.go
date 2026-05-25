package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server/consoleassets"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const (
	configServerAddr              = "server.addr"
	configServerReadHeaderTimeout = "server.readHeaderTimeout"
	configServerReadTimeout       = "server.readTimeout"
	configServerWriteTimeout      = "server.writeTimeout"
	configServerIdleTimeout       = "server.idleTimeout"
	configServerShutdownTimeout   = "server.shutdownTimeout"
	configServerMaxConcurrentRuns = "server.maxConcurrentRuns"

	defaultServerAddr                 = ":8080"
	defaultReadHeaderTimeout          = 5 * time.Second
	defaultReadTimeout                = 15 * time.Second
	defaultWriteTimeout               = 180 * time.Second
	defaultIdleTimeout                = 60 * time.Second
	defaultShutdownTimeout            = 5 * time.Second
	defaultMaxConcurrentRuns    int64 = 1
	runSnapshotCacheTTL               = 2 * time.Second
	defaultEventRecordPageLimit       = 100
	maxEventRecordPageLimit           = 500
)

type Config struct {
	Addr              string
	Backend           string
	DataRoot          string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxConcurrentRuns int64
}

type HTTPServer struct {
	config Config
	http   *http.Server
	state  *serverState
}

type statusResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

type configurationResponse struct {
	Service       string                          `json:"service"`
	Configuration appservice.ServiceConfiguration `json:"configuration"`
}

type workspaceResponse struct {
	Service   string               `json:"service"`
	Backend   string               `json:"backend,omitempty"`
	DataRoot  string               `json:"dataRoot,omitempty"`
	Workspace appservice.Workspace `json:"workspace"`
}

type workspacesResponse struct {
	Service    string                 `json:"service"`
	Backend    string                 `json:"backend,omitempty"`
	DataRoot   string                 `json:"dataRoot,omitempty"`
	Workspaces []appservice.Workspace `json:"workspaces"`
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
	Service  string                       `json:"service"`
	Backend  string                       `json:"backend,omitempty"`
	DataRoot string                       `json:"dataRoot,omitempty"`
	Snapshot appservice.WorkspaceSnapshot `json:"snapshot"`
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

type activateWorkspaceExecutionRequest struct {
	GoalID                string            `json:"goalId,omitempty"`
	DoDID                 string            `json:"doDId,omitempty"`
	LLMProviderID         string            `json:"llmProviderId,omitempty"`
	SoulID                string            `json:"soulId,omitempty"`
	SoulBindings          map[string]string `json:"soulBindings,omitempty"`
	SkillIDs              []string          `json:"skillIds,omitempty"`
	MCPServerIDs          []string          `json:"mcpServerIds,omitempty"`
	CapabilityIDs         []string          `json:"capabilityIds,omitempty"`
	MemoryIDs             []string          `json:"memoryIds,omitempty"`
	DialogLanguage        string            `json:"dialogLanguage,omitempty"`
	Priority              int               `json:"priority,omitempty"`
	ExecutionContextPatch map[string]any    `json:"executionContextPatch,omitempty"`
}

type createSoulRequest = appservice.CreateSoulCommand
type createWorkspaceTemplateRequest = appservice.WorkspaceTemplate
type createSkillRequest = appservice.CreateSkillCommand
type createMCPServerRequest = appservice.CreateMCPServerCommand
type updateMCPServerRequest = appservice.UpdateMCPServerCommand
type createMCPCapabilityRequest = appservice.CreateMCPCapabilityCommand
type createMemoryItemRequest = appservice.CreateMemoryItemCommand

type resolveDecisionRequest struct {
	ResolutionType    string         `json:"resolutionType"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}

type batchResolveDecisionsRequest struct {
	TicketIDs         []string       `json:"ticketIds"`
	ResolutionType    string         `json:"resolutionType"`
	ResolutionPayload map[string]any `json:"resolutionPayload,omitempty"`
}

type batchResolveApprovalsRequest struct {
	ApprovalIDs []string `json:"approvalIds"`
	Action      string   `json:"action"`
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
	Error string `json:"error"`
}

type createWorkspaceFunc func(context.Context, string, string, appservice.CreateWorkspaceCommand) (appservice.Workspace, error)
type deleteWorkspaceFunc func(context.Context, string, string, string, appservice.DeleteWorkspaceCommand) error
type readWorkspaceFunc func(context.Context, string, string, string) (appservice.Workspace, error)
type readWorkspaceSnapshotFunc func(context.Context, string, string, string) (appservice.WorkspaceSnapshot, error)
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
type readWorkspaceTaskProgressFunc func(context.Context, string, string, string) (appservice.TaskProgress, error)
type readRunContextBundleFunc func(context.Context, string, string, string) (appservice.ContextBundle, error)
type readWorkspaceContextBundleFunc func(context.Context, string, string, string) (appservice.ContextBundle, error)
type readRunTaskDecompositionsFunc func(context.Context, string, string, string) ([]appservice.TaskDecomposition, error)
type readWorkspaceTaskDecompositionsFunc func(context.Context, string, string, string) ([]appservice.TaskDecomposition, error)
type listMemoryRecallTracesFunc func(context.Context, string, string, appservice.MemoryRecallTraceQuery) ([]appservice.MemoryRecallTrace, error)
type listEventRecordsFunc func(context.Context, string, string, appservice.EventRecordQuery) ([]appservice.EventRecord, error)
type listRunInstancesFunc func(context.Context, string, string, string) ([]appservice.Run, error)
type startRunInstanceFunc func(context.Context, string, string, string) (appservice.Run, error)
type resumeRunInstanceFunc func(context.Context, string, string, string, ...map[string]any) (appservice.Run, error)
type activateWorkspaceExecutionFunc func(context.Context, string, string, string, appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error)
type listDecisionsFunc func(context.Context, string, string, string, string) ([]appservice.Decision, error)
type listDecisionPageFunc func(context.Context, string, string, appservice.DecisionQuery) (appservice.DecisionPage, error)
type readDecisionFunc func(context.Context, string, string, string) (appservice.Decision, error)
type resolveDecisionFunc func(context.Context, string, string, string, appservice.ResolveDecisionCommand) (appservice.Decision, error)
type batchResolveDecisionsFunc func(context.Context, string, string, appservice.BatchResolveDecisionCommand) ([]appservice.Decision, error)
type listApprovalsFunc func(context.Context, string, string, string, string) ([]appservice.Approval, error)
type listApprovalPageFunc func(context.Context, string, string, appservice.ApprovalQuery) (appservice.ApprovalPage, error)
type readApprovalFunc func(context.Context, string, string, string) (appservice.Approval, error)
type approveApprovalFunc func(context.Context, string, string, string) (appservice.Approval, error)
type rejectApprovalFunc func(context.Context, string, string, string) (appservice.Approval, error)
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
type updateLLMConfigurationFunc func(appservice.UpdateLLMConfigurationCommand) (appservice.ServiceConfiguration, error)
type updateWebSearchConfigurationFunc func(appservice.UpdateWebSearchConfigurationCommand) (appservice.ServiceConfiguration, error)

type serverState struct {
	config                          Config
	eventHub                        events.Hub
	metrics                         *serverMetrics
	validate                        validateFunc
	createWorkspace                 createWorkspaceFunc
	deleteWorkspace                 deleteWorkspaceFunc
	readWorkspace                   readWorkspaceFunc
	readWorkspaceSnapshot           readWorkspaceSnapshotFunc
	readWorkspaceSoulBindings       readWorkspaceSoulBindingsFunc
	updateWorkspaceSoulBindings     updateWorkspaceSoulBindingsFunc
	listWorkspaces                  listWorkspacesFunc
	defineGoal                      defineGoalFunc
	updateGoal                      updateGoalFunc
	readGoal                        readGoalFunc
	listGoals                       listGoalsFunc
	defineDoD                       defineDoDFunc
	updateDoD                       updateDoDFunc
	readDoD                         readDoDFunc
	listDoDs                        listDoDsFunc
	checkGoalDoDQuality             checkGoalDoDQualityFunc
	generateWorkspaceSetupDraft     generateWorkspaceSetupDraftFunc
	createRun                       createRunInstanceFunc
	readRun                         readRunInstanceFunc
	readRunSnapshot                 readRunSnapshotFunc
	listRunSteps                    listRunStepsFunc
	listRunToolCalls                listRunToolCallsFunc
	listRunRuntimeCheckpoints       listRunRuntimeCheckpointsFunc
	listRunResults                  listRunResultsFunc
	listRunPatches                  listRunPatchesFunc
	listRunValidationResults        listRunValidationResultsFunc
	listRunArtifacts                listRunArtifactsFunc
	readRunStatus                   readRunStatusFunc
	readRunObservability            readRunObservabilityFunc
	readArtifactContent             readArtifactContentFunc
	readRuntimeRawContent           readRuntimeRawContentFunc
	readRunTaskProgress             readRunTaskProgressFunc
	readWorkspaceTaskProgress       readWorkspaceTaskProgressFunc
	readRunContextBundle            readRunContextBundleFunc
	readWorkspaceContextBundle      readWorkspaceContextBundleFunc
	readRunTaskDecompositions       readRunTaskDecompositionsFunc
	readWorkspaceTaskDecompositions readWorkspaceTaskDecompositionsFunc
	listMemoryRecallTraces          listMemoryRecallTracesFunc
	listEventRecords                listEventRecordsFunc
	listRuns                        listRunInstancesFunc
	startRun                        startRunInstanceFunc
	resumeRun                       resumeRunInstanceFunc
	activateWorkspace               activateWorkspaceExecutionFunc
	listDecisions                   listDecisionsFunc
	listDecisionPage                listDecisionPageFunc
	readDecision                    readDecisionFunc
	resolveDecision                 resolveDecisionFunc
	batchResolveDecisions           batchResolveDecisionsFunc
	listApprovals                   listApprovalsFunc
	listApprovalPage                listApprovalPageFunc
	readApproval                    readApprovalFunc
	approveApproval                 approveApprovalFunc
	rejectApproval                  rejectApprovalFunc
	batchResolveApprovals           batchResolveApprovalsFunc
	createSoul                      createSoulFunc
	updateSoul                      updateSoulFunc
	deleteSoul                      deleteSoulFunc
	readSoul                        readSoulFunc
	listSouls                       listSoulsFunc
	createWorkspaceTemplate         createWorkspaceTemplateFunc
	updateWorkspaceTemplate         updateWorkspaceTemplateFunc
	deleteWorkspaceTemplate         deleteWorkspaceTemplateFunc
	listWorkspaceTemplates          listWorkspaceTemplatesFunc
	createSkill                     createSkillFunc
	readSkill                       readSkillFunc
	listSkills                      listSkillsFunc
	createMCPServer                 createMCPServerFunc
	updateMCPServer                 updateMCPServerFunc
	readMCPServer                   readMCPServerFunc
	listMCPServers                  listMCPServersFunc
	listMCPServerTools              listMCPServerToolsFunc
	listMCPServerResources          listMCPServerResourcesFunc
	listMCPServerPrompts            listMCPServerPromptsFunc
	createMCPCapability             createMCPCapabilityFunc
	updateMCPCapability             updateMCPCapabilityFunc
	readMCPCapability               readMCPCapabilityFunc
	listMCPCapabilities             listMCPCapabilitiesFunc
	createMemoryItem                createMemoryItemFunc
	readMemoryItem                  readMemoryItemFunc
	updateMemoryItem                updateMemoryItemFunc
	deleteMemoryItem                deleteMemoryItemFunc
	listMemoryByWorkspace           listMemoryByWorkspaceFunc
	searchMemory                    searchMemoryFunc
	readConfiguration               readConfigurationFunc
	updateServerConfiguration       updateServerConfigurationFunc
	updateLLMConfiguration          updateLLMConfigurationFunc
	updateWebSearchConfiguration    updateWebSearchConfigurationFunc
	runQueueMu                      sync.Mutex
	runQueueWaiters                 []*runQueueWaiter
	runQueueSequence                uint64
	activeRunSlots                  int64
	backgroundWG                    sync.WaitGroup
	backgroundCtx                   context.Context
	backgroundCancel                context.CancelFunc
	runStatusNotifyMu               sync.Mutex
	runStatusNotify                 chan struct{}
	eventUnsubscribes               []func()
	replanContinuationMu            sync.Mutex
	replanContinuationStarted       map[string]struct{}
	governanceResumeMu              sync.Mutex
	governanceResumeStarted         map[string]struct{}
	runSnapshotCacheMu              sync.Mutex
	runSnapshotCache                map[string]cachedRunSnapshot
	runSnapshotInflight             map[string]*runSnapshotInflight
	sharedSession                   *appservice.RuntimeSession
	workspaceSetupDraftMu           sync.Mutex
	workspaceSetupDraftSessions     map[string]*workspaceSetupDraftSession
}

type cachedRunSnapshot struct {
	snapshot appservice.RunSnapshot
	cachedAt time.Time
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
	healthzRequests                    atomic.Uint64
	readyzRequests                     atomic.Uint64
	metricsRequests                    atomic.Uint64
	workspaceCreateRequests            atomic.Uint64
	workspaceDeleteRequests            atomic.Uint64
	workspaceListRequests              atomic.Uint64
	workspaceGetRequests               atomic.Uint64
	workspaceSnapshotRequests          atomic.Uint64
	goalDefineRequests                 atomic.Uint64
	goalListRequests                   atomic.Uint64
	goalGetRequests                    atomic.Uint64
	dodDefineRequests                  atomic.Uint64
	dodListRequests                    atomic.Uint64
	dodGetRequests                     atomic.Uint64
	goalDoDQualityCheckRequests        atomic.Uint64
	runCreateRequests                  atomic.Uint64
	runListRequests                    atomic.Uint64
	runGetRequests                     atomic.Uint64
	runSnapshotRequests                atomic.Uint64
	runStatusRequests                  atomic.Uint64
	runObservabilityRequests           atomic.Uint64
	artifactContentRequests            atomic.Uint64
	runTaskProgressRequests            atomic.Uint64
	workspaceTaskProgressRequests      atomic.Uint64
	runContextBundleRequests           atomic.Uint64
	workspaceContextBundleRequests     atomic.Uint64
	runTaskDecompositionRequests       atomic.Uint64
	workspaceTaskDecompositionRequests atomic.Uint64
	eventListRequests                  atomic.Uint64
	runStartRequests                   atomic.Uint64
	runResumeRequests                  atomic.Uint64
	workspaceActivationRequests        atomic.Uint64
	decisionListRequests               atomic.Uint64
	decisionGetRequests                atomic.Uint64
	decisionResolveRequests            atomic.Uint64
	approvalListRequests               atomic.Uint64
	approvalGetRequests                atomic.Uint64
	approvalApproveRequests            atomic.Uint64
	approvalRejectRequests             atomic.Uint64
	soulCreateRequests                 atomic.Uint64
	soulUpdateRequests                 atomic.Uint64
	soulDeleteRequests                 atomic.Uint64
	soulListRequests                   atomic.Uint64
	soulGetRequests                    atomic.Uint64
	skillCreateRequests                atomic.Uint64
	skillListRequests                  atomic.Uint64
	skillGetRequests                   atomic.Uint64
	mcpServerCreateRequests            atomic.Uint64
	mcpServerListRequests              atomic.Uint64
	mcpServerGetRequests               atomic.Uint64
	mcpToolListRequests                atomic.Uint64
	mcpResourceListRequests            atomic.Uint64
	mcpPromptListRequests              atomic.Uint64
	mcpCapabilityCreateRequests        atomic.Uint64
	mcpCapabilityUpdateRequests        atomic.Uint64
	mcpCapabilityListRequests          atomic.Uint64
	mcpCapabilityGetRequests           atomic.Uint64
	memoryCreateRequests               atomic.Uint64
	memoryListRequests                 atomic.Uint64
	configurationReadRequests          atomic.Uint64
	configurationUpdateRequests        atomic.Uint64
	runSucceeded                       atomic.Uint64
	runFailed                          atomic.Uint64
	runRejected                        atomic.Uint64
	runCanceled                        atomic.Uint64
	runInFlight                        atomic.Int64
	runQueueWaiting                    atomic.Int64
	runQueuedTotal                     atomic.Uint64
}

func (m *serverMetrics) observeRunOutcome(run appservice.Run) {
	switch run.State {
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
	maxConcurrentRuns := cfg.Server.MaxConcurrentRuns
	if maxConcurrentRuns == 0 {
		maxConcurrentRuns = defaultMaxConcurrentRuns
	}
	if maxConcurrentRuns < 0 {
		return Config{}, fmt.Errorf("invalid integer for %s: must be positive", configServerMaxConcurrentRuns)
	}
	return Config{
		Addr:              firstNonEmpty(cfg.Server.Addr, defaultServerAddr),
		Backend:           cfg.Repository.Backend,
		DataRoot:          cfg.Repository.DataDir,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
		MaxConcurrentRuns: maxConcurrentRuns,
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
	return Config{
		Addr:              addr,
		Backend:           values["repository.backend"],
		DataRoot:          values["repository.dataDir"],
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
		MaxConcurrentRuns: maxConcurrentRuns,
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
	return config
}

func NewHTTPServer(config Config) *HTTPServer {
	return NewHTTPServerWithEventHub(config, nil)
}

func NewHTTPServerWithEventHub(config Config, eventHub events.Hub) *HTTPServer {
	config = normalizeConfig(config)
	state := newServerState(config, eventHub)
	handler := newMuxWithState(state)
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
	s.state.scheduleStartupRecovery()
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
	ctx = appservice.ContextWithEventHub(ctx, s.state.eventHub)
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
	return newMuxWithState(newServerState(normalizeConfig(config), nil))
}

func newMuxWithState(state *serverState) http.Handler {
	router := engine.NewRouteRegistry()
	registerSystemRoutes(router, state)
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

func registerSystemRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/healthz", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		state.metrics.healthzRequests.Add(1)
		writeJSON(w, http.StatusOK, statusResponse{
			Service: "workorch",
			Status:  "ok",
		})
	})
	router.AddHandler("/readyz", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.readyzRequests.Add(1)
		if err := state.validate(r.Context(), state.config.Backend, state.config.DataRoot); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, statusResponse{
			Service: "workorch",
			Status:  "ready",
		})
	})
	router.AddHandler("/metrics", http.MethodGet, func(_ context.Context, w http.ResponseWriter, _ *http.Request) {
		state.metrics.metricsRequests.Add(1)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprint(w, state.metrics.render(state.config))
	})
	router.AddHandler("/api/v1/workorch/configuration", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.configurationReadRequests.Add(1)
		item, err := state.readConfiguration()
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
	})
	router.AddHandler("/api/v1/workorch/configuration/server", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.configurationUpdateRequests.Add(1)
		var req appservice.UpdateServerConfigurationCommand
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateServerConfiguration(req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
	})
	router.AddHandler("/api/v1/workorch/configuration/llm", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.configurationUpdateRequests.Add(1)
		var req appservice.UpdateLLMConfigurationCommand
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateLLMConfiguration(req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
	})
	router.AddHandler("/api/v1/workorch/configuration/web-search", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.configurationUpdateRequests.Add(1)
		var req appservice.UpdateWebSearchConfigurationCommand
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateWebSearchConfiguration(req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, configurationResponse{
			Service:       "workorch",
			Configuration: item,
		})
	})
}

func registerReadRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspaces", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.workspaceListRequests.Add(1)
		items, err := state.listWorkspaces(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspacesResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Workspaces: items,
		})
	})
	router.AddHandler("/api/v1/workorch/runs", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.runListRequests.Add(1)
		items, err := state.listRuns(r.Context(), state.config.Backend, state.config.DataRoot, r.URL.Query().Get("workspaceId"))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Runs:     items,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.workspaceGetRequests.Add(1)
		item, err := state.readWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Workspace: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/snapshot", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "snapshot" {
			http.NotFound(w, r)
			return
		}
		state.metrics.workspaceSnapshotRequests.Add(1)
		item, err := state.readWorkspaceSnapshotCached(r.Context(), workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceSnapshotResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Snapshot: appservice.ProjectWorkspaceSnapshotDetail(item),
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/task-progress", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "task-progress" {
			http.NotFound(w, r)
			return
		}
		state.metrics.workspaceTaskProgressRequests.Add(1)
		item, err := state.readWorkspaceTaskProgress(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, taskProgressResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			TaskProgress: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/context-bundle", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "context-bundle" {
			http.NotFound(w, r)
			return
		}
		state.metrics.workspaceContextBundleRequests.Add(1)
		item, err := state.readWorkspaceContextBundle(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, contextBundleResponse{
			Service:       "workorch",
			Backend:       state.config.Backend,
			DataRoot:      state.config.DataRoot,
			ContextBundle: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/task-decompositions", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "task-decompositions" {
			http.NotFound(w, r)
			return
		}
		state.metrics.workspaceTaskDecompositionRequests.Add(1)
		items, err := state.readWorkspaceTaskDecompositions(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, taskDecompositionsResponse{
			Service:            "workorch",
			Backend:            state.config.Backend,
			DataRoot:           state.config.DataRoot,
			TaskDecompositions: items,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory-recalls", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "memory-recalls" {
			http.NotFound(w, r)
			return
		}
		query := memoryRecallTraceQueryFromValues(r.URL.Query())
		query.WorkspaceID = workspaceID
		items, err := state.listMemoryRecallTraces(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryRecallTracesResponse{
			Service:            "workorch",
			Backend:            state.config.Backend,
			DataRoot:           state.config.DataRoot,
			MemoryRecallTraces: items,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/soul-bindings", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "soul-bindings" {
			http.NotFound(w, r)
			return
		}
		item, err := state.readWorkspaceSoulBindings(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceSoulBindingsResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			SoulBindings: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/goals", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "goals" {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalListRequests.Add(1)
		items, err := state.listGoals(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, goalsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goals:    items,
		})
	})
	router.AddHandler("/api/v1/workorch/goals/:goalId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		goalID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/goals/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalGetRequests.Add(1)
		item, err := state.readGoal(r.Context(), state.config.Backend, state.config.DataRoot, goalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, goalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goal:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/dods", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "dods" {
			http.NotFound(w, r)
			return
		}
		state.metrics.dodListRequests.Add(1)
		items, err := state.listDoDs(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dodsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoDs:     items,
		})
	})
	router.AddHandler("/api/v1/workorch/dods/:dodId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		dodID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/dods/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.dodGetRequests.Add(1)
		item, err := state.readDoD(r.Context(), state.config.Backend, state.config.DataRoot, dodID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dodResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoD:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runGetRequests.Add(1)
		item, err := state.readRun(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/snapshot", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "snapshot" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runSnapshotRequests.Add(1)
		item, err := state.readRunSnapshotCached(r.Context(), runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runSnapshotResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Snapshot: appservice.ProjectRunSnapshotDetail(item),
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/steps", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "steps" {
			http.NotFound(w, r)
			return
		}
		query := stepQueryFromRequest(r)
		query.RunID = runID
		page, err := state.listRunSteps(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runStepsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Steps:    page.Steps,
			Page:     page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/tool-calls", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "tool-calls" {
			http.NotFound(w, r)
			return
		}
		query := toolCallQueryFromRequest(r)
		query.RunID = runID
		page, err := state.listRunToolCalls(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runToolCallsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			ToolCalls: page.ToolCalls,
			Page:      page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/runtime-checkpoints", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "runtime-checkpoints" {
			http.NotFound(w, r)
			return
		}
		query := runtimeCheckpointQueryFromRequest(r)
		query.RunID = runID
		page, err := state.listRunRuntimeCheckpoints(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runRuntimeCheckpointsResponse{
			Service:            "workorch",
			Backend:            state.config.Backend,
			DataRoot:           state.config.DataRoot,
			RuntimeCheckpoints: page.RuntimeCheckpoints,
			Page:               page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/results", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "results" {
			http.NotFound(w, r)
			return
		}
		query := resultQueryFromRequest(r)
		query.RunID = runID
		page, err := state.listRunResults(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runResultsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Results:  page.Results,
			Page:     page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/patches", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "patches" {
			http.NotFound(w, r)
			return
		}
		query := patchQueryFromRequest(r)
		query.RunID = runID
		page, err := state.listRunPatches(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runPatchesResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Patches:  page.Patches,
			Page:     page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/validation-results", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "validation-results" {
			http.NotFound(w, r)
			return
		}
		query := validationResultQueryFromRequest(r)
		query.RunID = runID
		page, err := state.listRunValidationResults(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runValidationResultsResponse{
			Service:           "workorch",
			Backend:           state.config.Backend,
			DataRoot:          state.config.DataRoot,
			ValidationResults: page.ValidationResults,
			Page:              page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/artifacts", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "artifacts" {
			http.NotFound(w, r)
			return
		}
		query := artifactQueryFromRequest(r)
		query.RunID = runID
		page, err := state.listRunArtifacts(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runArtifactsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Artifacts: page.Artifacts,
			Page:      page,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/status", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "status" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runStatusRequests.Add(1)
		started := time.Now()
		item, waitMatched, err := state.readRunStatusWithWait(r.Context(), runID, r.URL.Query())
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runStatusResponse{
			Service:       "workorch",
			Backend:       state.config.Backend,
			DataRoot:      state.config.DataRoot,
			Status:        item,
			WaitMatched:   waitMatched,
			WaitElapsedMs: int64(time.Since(started) / time.Millisecond),
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/observability", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "observability" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runObservabilityRequests.Add(1)
		item, err := state.readRunObservabilityCached(r.Context(), runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runObservabilityResponse{
			Service:       "workorch",
			Backend:       state.config.Backend,
			DataRoot:      state.config.DataRoot,
			Observability: item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/memory-recalls", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "memory-recalls" {
			http.NotFound(w, r)
			return
		}
		query := memoryRecallTraceQueryFromValues(r.URL.Query())
		query.RunID = runID
		items, err := state.listMemoryRecallTraces(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryRecallTracesResponse{
			Service:            "workorch",
			Backend:            state.config.Backend,
			DataRoot:           state.config.DataRoot,
			MemoryRecallTraces: items,
		})
	})
	router.AddHandler("/api/v1/workorch/artifacts/content", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.artifactContentRequests.Add(1)
		item, err := state.readArtifactContentCached(r.Context(), r.URL.Query().Get("uri"))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeArtifactContent(w, item)
	})
	router.AddHandler("/api/v1/workorch/runtime/payloads", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		item, err := state.readRuntimeRawContent(r.Context(), state.config.Backend, state.config.DataRoot, r.URL.Query().Get("ref"))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeRuntimeRawContent(w, item)
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/task-progress", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "task-progress" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runTaskProgressRequests.Add(1)
		item, err := state.readRunTaskProgress(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, taskProgressResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			TaskProgress: item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/context-bundle", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "context-bundle" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runContextBundleRequests.Add(1)
		item, err := state.readRunContextBundle(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, contextBundleResponse{
			Service:       "workorch",
			Backend:       state.config.Backend,
			DataRoot:      state.config.DataRoot,
			ContextBundle: item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/task-decompositions", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "task-decompositions" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runTaskDecompositionRequests.Add(1)
		items, err := state.readRunTaskDecompositions(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, taskDecompositionsResponse{
			Service:            "workorch",
			Backend:            state.config.Backend,
			DataRoot:           state.config.DataRoot,
			TaskDecompositions: items,
		})
	})
	router.AddHandler("/api/v1/workorch/events", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.eventListRequests.Add(1)
		items, err := state.listEventRecords(r.Context(), state.config.Backend, state.config.DataRoot, eventRecordQueryFromRequest(r))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, eventRecordsResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			EventRecords: items,
		})
	})
	router.AddHandler("/api/v1/workorch/events/stream", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		if state.eventHub == nil {
			writeBusinessError(w, common.ErrNotFound)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "streaming unsupported"})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
		runID := strings.TrimSpace(r.URL.Query().Get("runId"))
		eventCh := make(chan events.Envelope, 128)
		unsubscribe := state.eventHub.Subscribe("*", func(_ context.Context, envelope events.Envelope) error {
			if workspaceID != "" && envelope.WorkspaceID != workspaceID {
				return nil
			}
			if runID != "" && envelope.RunID != runID {
				return nil
			}
			enqueueEventStreamEnvelope(eventCh, envelope)
			return nil
		})
		defer unsubscribe()
		_, _ = fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case envelope := <-eventCh:
				data, err := json.Marshal(envelope)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintf(w, "event: workorch\n")
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	})
	router.AddHandler("/api/v1/workorch/decisions", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.decisionListRequests.Add(1)
		query := decisionQueryFromRequest(r)
		page, err := state.listDecisionPage(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decisionsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Decisions: page.Decisions,
			Page:      page,
		})
	})
	router.AddHandler("/api/v1/workorch/decisions/:ticketId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		ticketID, action, ok := decisionActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.decisionGetRequests.Add(1)
		item, err := state.readDecision(r.Context(), state.config.Backend, state.config.DataRoot, ticketID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, decisionResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Decision: item,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.approvalListRequests.Add(1)
		query := approvalQueryFromRequest(r)
		page, err := state.listApprovalPage(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, approvalsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Approvals: page.Approvals,
			Page:      page,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/:approvalId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		approvalID, action, ok := approvalActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.approvalGetRequests.Add(1)
		item, err := state.readApproval(r.Context(), state.config.Backend, state.config.DataRoot, approvalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, approvalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Approval: item,
		})
	})
	router.AddHandler("/api/v1/workorch/souls", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.soulListRequests.Add(1)
		items, err := state.listSouls(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, soulsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Souls:    items,
		})
	})
	router.AddHandler("/api/v1/workorch/souls/:soulId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		soulID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/souls/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.soulGetRequests.Add(1)
		item, err := state.readSoul(r.Context(), state.config.Backend, state.config.DataRoot, soulID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, soulResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Soul:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspace-templates", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		items, err := state.listWorkspaceTemplates(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceTemplatesResponse{
			Service:            "workorch",
			Backend:            state.config.Backend,
			DataRoot:           state.config.DataRoot,
			WorkspaceTemplates: items,
		})
	})
	router.AddHandler("/api/v1/workorch/skills", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.skillListRequests.Add(1)
		query := appservice.SkillQuery{WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspaceId"))}
		items, err := state.listSkills(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, skillsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Skills:   items,
		})
	})
	router.AddHandler("/api/v1/workorch/skills/:skillId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		skillID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/skills/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.skillGetRequests.Add(1)
		query := appservice.SkillQuery{WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspaceId"))}
		item, err := state.readSkill(r.Context(), state.config.Backend, state.config.DataRoot, skillID, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, skillResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Skill:    item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.mcpServerListRequests.Add(1)
		items, err := state.listMCPServers(r.Context(), state.config.Backend, state.config.DataRoot)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpServersResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Servers:  items,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/capabilities", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.mcpCapabilityListRequests.Add(1)
		items, err := state.listMCPCapabilities(r.Context(), state.config.Backend, state.config.DataRoot, mcpCapabilityQueryFromRequest(r))
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpCapabilitiesResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			Capabilities: items,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/capabilities/:capabilityId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		capabilityID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/mcp/capabilities/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpCapabilityGetRequests.Add(1)
		item, err := state.readMCPCapability(r.Context(), state.config.Backend, state.config.DataRoot, capabilityID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpCapabilityResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Capability: item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if action != "" {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpServerGetRequests.Add(1)
		item, err := state.readMCPServer(r.Context(), state.config.Backend, state.config.DataRoot, serverID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpServerResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Server:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId/tools", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok || action != "tools" {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpToolListRequests.Add(1)
		items, err := state.listMCPServerTools(r.Context(), state.config.Backend, state.config.DataRoot, serverID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpToolsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Tools:    items,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId/resources", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok || action != "resources" {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpResourceListRequests.Add(1)
		items, err := state.listMCPServerResources(r.Context(), state.config.Backend, state.config.DataRoot, serverID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpResourcesResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Resources: items,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId/prompts", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok || action != "prompts" {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpPromptListRequests.Add(1)
		items, err := state.listMCPServerPrompts(r.Context(), state.config.Backend, state.config.DataRoot, serverID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpPromptsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Prompts:  items,
		})
	})
	router.AddHandler("/api/v1/workorch/memory", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		query := memoryQueryFromValues(r.URL.Query())
		workspaceID := query.WorkspaceID
		if workspaceID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "workspaceId is required"})
			return
		}
		state.metrics.memoryListRequests.Add(1)
		items, err := state.searchMemory(r.Context(), state.config.Backend, state.config.DataRoot, query)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryItemsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Memory:   items,
		})
	})
	router.AddHandler("/api/v1/workorch/memory/:memoryId", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		memoryID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/memory/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		item, err := state.readMemoryItem(r.Context(), state.config.Backend, state.config.DataRoot, memoryID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryItemResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Memory:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/memory/:memoryId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		memoryID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/memory/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		var req createMemoryItemRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateMemoryItem(r.Context(), state.config.Backend, state.config.DataRoot, memoryID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryItemResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Memory:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/memory/:memoryId", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		memoryID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/memory/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		if err := state.deleteMemoryItem(r.Context(), state.config.Backend, state.config.DataRoot, memoryID); err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "workorch",
			"deleted": true,
		})
	})
}

func registerBusinessWriteRoutes(router engine.RouteRegistry, state *serverState) {
	registerWorkspaceSetupDraftRoutes(router, state)
	router.AddHandler("/api/v1/workorch/workspaces", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.workspaceCreateRequests.Add(1)
		var req createWorkspaceRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		if req.WorkspaceID != "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "workspaceId is generated by workorch"})
			return
		}
		item, err := state.createWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, appservice.CreateWorkspaceCommand{
			Title:                req.Title,
			Description:          req.Description,
			DeliverableDirectory: req.DeliverableDirectory,
			TemplateID:           req.TemplateID,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, workspaceResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Workspace: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "" {
			http.NotFound(w, r)
			return
		}
		cmd := deleteWorkspaceRequest{}
		if r.Body != nil && r.ContentLength != 0 {
			if err := decodeJSON(r, &cmd); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
				return
			}
		}
		if value := strings.TrimSpace(r.URL.Query().Get("purge")); value != "" {
			purge, err := strconv.ParseBool(value)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid purge query"})
				return
			}
			cmd.Purge = purge
		}
		state.metrics.workspaceDeleteRequests.Add(1)
		if err := state.deleteWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, cmd); err != nil {
			writeBusinessError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "runs" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runCreateRequests.Add(1)
		var req createRunRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		if req.GoalID == "" || req.DoDID == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "goalId and doDId must reference defined workspace goal and DoD"})
			return
		}
		item, err := state.createRun(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req.RunID, req.GoalID, req.DoDID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/execution-activations", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "execution-activations" {
			http.NotFound(w, r)
			return
		}
		var req activateWorkspaceExecutionRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.workspaceActivationRequests.Add(1)
		priority := runPriorityFromRequest(r, req.Priority)
		release, err := state.acquireRunSlotWithPriority(r.Context(), priority)
		if err != nil {
			state.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()
		item, err := state.activateWorkspace(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, appservice.ActivateWorkspaceExecutionCommand{
			GoalID:                req.GoalID,
			DoDID:                 req.DoDID,
			LLMProviderID:         req.LLMProviderID,
			SoulID:                req.SoulID,
			SoulBindings:          req.SoulBindings,
			SkillIDs:              req.SkillIDs,
			MCPServerIDs:          req.MCPServerIDs,
			CapabilityIDs:         req.CapabilityIDs,
			MemoryIDs:             req.MemoryIDs,
			DialogLanguage:        req.DialogLanguage,
			Priority:              priority,
			ExecutionContextPatch: req.ExecutionContextPatch,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.observeRunOutcome(item.Run)
		if item.Run.State == string(common.RunStateSuperseded) {
			state.scheduleReplanContinuationIfNeeded(r.Context(), item.Run.WorkspaceID, item.Run.ID, priority)
		}
		state.notifyRunStatusChangedForRun(item.Run)
		writeJSON(w, http.StatusOK, workspaceExecutionActivationResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Activation: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/soul-bindings", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "soul-bindings" {
			http.NotFound(w, r)
			return
		}
		var req appservice.WorkspaceSoulBindings
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateWorkspaceSoulBindings(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceSoulBindingsResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			SoulBindings: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/goals", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "goals" {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalDefineRequests.Add(1)
		var req defineGoalRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.defineGoal(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req.Text)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, goalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goal:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/goals/:goalId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		goalID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/goals/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalDefineRequests.Add(1)
		var req defineGoalRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateGoal(r.Context(), state.config.Backend, state.config.DataRoot, goalID, req.Text)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, goalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Goal:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/dods", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "dods" {
			http.NotFound(w, r)
			return
		}
		state.metrics.dodDefineRequests.Add(1)
		var req defineDoDRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.defineDoD(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req.Items)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, dodResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoD:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/dods/:dodId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		dodID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/dods/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.dodDefineRequests.Add(1)
		var req defineDoDRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateDoD(r.Context(), state.config.Backend, state.config.DataRoot, dodID, req.Items)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, dodResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			DoD:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/goal-dod-quality-check", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "goal-dod-quality-check" {
			http.NotFound(w, r)
			return
		}
		state.metrics.goalDoDQualityCheckRequests.Add(1)
		var req goalDoDQualityCheckRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.checkGoalDoDQuality(r.Context(), state.config.Backend, state.config.DataRoot, appservice.GoalDoDQualityCheckCommand{
			WorkspaceID: workspaceID,
			GoalID:      req.GoalID,
			DoDID:       req.DoDID,
			GoalText:    req.GoalText,
			DoDItems:    req.DoDItems,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, goalDoDQualityCheckResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			QualityCheck: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/memory", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "memory" {
			http.NotFound(w, r)
			return
		}
		state.metrics.memoryCreateRequests.Add(1)
		var req createMemoryItemRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createMemoryItem(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, memoryItemResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Memory:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/souls", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.soulCreateRequests.Add(1)
		var req createSoulRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createSoul(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, soulResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Soul:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/souls/:soulId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		soulID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/souls/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.soulUpdateRequests.Add(1)
		var req appservice.UpdateSoulCommand
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateSoul(r.Context(), state.config.Backend, state.config.DataRoot, soulID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, soulResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Soul:     item,
		})
	})
	router.AddHandler("/api/v1/workorch/souls/:soulId", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		soulID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/souls/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.soulDeleteRequests.Add(1)
		if err := state.deleteSoul(r.Context(), state.config.Backend, state.config.DataRoot, soulID); err != nil {
			writeBusinessError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router.AddHandler("/api/v1/workorch/workspace-templates", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		var req createWorkspaceTemplateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createWorkspaceTemplate(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, workspaceTemplateResponse{
			Service:           "workorch",
			Backend:           state.config.Backend,
			DataRoot:          state.config.DataRoot,
			WorkspaceTemplate: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspace-templates/:templateId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		templateID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/workspace-templates/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		var req appservice.WorkspaceTemplate
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateWorkspaceTemplate(r.Context(), state.config.Backend, state.config.DataRoot, templateID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceTemplateResponse{
			Service:           "workorch",
			Backend:           state.config.Backend,
			DataRoot:          state.config.DataRoot,
			WorkspaceTemplate: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspace-templates/:templateId", http.MethodDelete, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		templateID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/workspace-templates/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		if err := state.deleteWorkspaceTemplate(r.Context(), state.config.Backend, state.config.DataRoot, templateID); err != nil {
			writeBusinessError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	router.AddHandler("/api/v1/workorch/skills", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.skillCreateRequests.Add(1)
		var req createSkillRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createSkill(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, skillResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Skill:    item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.mcpServerCreateRequests.Add(1)
		var req createMCPServerRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createMCPServer(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, mcpServerResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Server:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/servers/:serverId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		serverID, action, ok := mcpServerActionFromPath(r.URL.Path)
		if !ok || action != "" {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: common.ErrNotFound.Error()})
			return
		}
		var req updateMCPServerRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateMCPServer(r.Context(), state.config.Backend, state.config.DataRoot, serverID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpServerResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Server:   item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/capabilities", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		state.metrics.mcpCapabilityCreateRequests.Add(1)
		var req createMCPCapabilityRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.createMCPCapability(r.Context(), state.config.Backend, state.config.DataRoot, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, mcpCapabilityResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Capability: item,
		})
	})
	router.AddHandler("/api/v1/workorch/mcp/capabilities/:capabilityId", http.MethodPut, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		capabilityID, ok := resourceIDFromPath(r.URL.Path, "/api/v1/workorch/mcp/capabilities/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		state.metrics.mcpCapabilityUpdateRequests.Add(1)
		var req createMCPCapabilityRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		item, err := state.updateMCPCapability(r.Context(), state.config.Backend, state.config.DataRoot, capabilityID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpCapabilityResponse{
			Service:    "workorch",
			Backend:    state.config.Backend,
			DataRoot:   state.config.DataRoot,
			Capability: item,
		})
	})
	router.AddHandler("/api/v1/workorch/workspaces/:workspaceId/runs", http.MethodGet, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		workspaceID, action, ok := workspaceActionFromPath(r.URL.Path)
		if !ok || action != "runs" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runListRequests.Add(1)
		items, err := state.listRuns(r.Context(), state.config.Backend, state.config.DataRoot, workspaceID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runsResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Runs:     items,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/start", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "start" {
			http.NotFound(w, r)
			return
		}
		state.metrics.runStartRequests.Add(1)
		release, err := state.acquireRunSlotWithPriority(r.Context(), runPriorityFromRequest(r, 0))
		if err != nil {
			state.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()
		item, err := state.startRun(r.Context(), state.config.Backend, state.config.DataRoot, runID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.observeRunOutcome(item)
		if item.State == string(common.RunStateSuperseded) {
			state.scheduleReplanContinuationIfNeeded(r.Context(), item.WorkspaceID, item.ID, runPriorityFromRequest(r, 0))
		}
		state.notifyRunStatusChangedForRun(item)
		writeJSON(w, http.StatusOK, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/runs/:runId/resume", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		runID, action, ok := runActionFromPath(r.URL.Path)
		if !ok || action != "resume" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			ExecutionContextPatch map[string]any `json:"executionContextPatch,omitempty"`
		}
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.runResumeRequests.Add(1)
		release, err := state.acquireRunSlotWithPriority(r.Context(), runPriorityFromRequest(r, 0))
		if err != nil {
			state.metrics.runCanceled.Add(1)
			writeRunQueueError(w, err)
			return
		}
		defer release()
		item, err := state.resumeRun(r.Context(), state.config.Backend, state.config.DataRoot, runID, req.ExecutionContextPatch)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.observeRunOutcome(item)
		if item.State == string(common.RunStateSuperseded) {
			state.scheduleReplanContinuationIfNeeded(r.Context(), item.WorkspaceID, item.ID, runPriorityFromRequest(r, 0))
		}
		state.notifyRunStatusChangedForRun(item)
		writeJSON(w, http.StatusOK, runInstanceResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Run:      item,
		})
	})
	router.AddHandler("/api/v1/workorch/decisions/:ticketId/resolve", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		ticketID, action, ok := decisionActionFromPath(r.URL.Path)
		if !ok || action != "resolve" {
			http.NotFound(w, r)
			return
		}
		var req resolveDecisionRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.decisionResolveRequests.Add(1)
		item, err := state.resolveDecision(r.Context(), state.config.Backend, state.config.DataRoot, ticketID, appservice.ResolveDecisionCommand{
			ResolutionType:    req.ResolutionType,
			ResolutionPayload: req.ResolutionPayload,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.scheduleGovernanceRunResumes([]string{item.RunID}, runPriorityFromRequest(r, 0))
		writeJSON(w, http.StatusOK, decisionResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Decision: item,
		})
	})
	router.AddHandler("/api/v1/workorch/decisions/resolve-batch", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		var req batchResolveDecisionsRequest
		if err := decodeJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		state.metrics.decisionResolveRequests.Add(uint64(len(req.TicketIDs)))
		items, err := state.batchResolveDecisions(r.Context(), state.config.Backend, state.config.DataRoot, appservice.BatchResolveDecisionCommand{
			TicketIDs:         req.TicketIDs,
			ResolutionType:    req.ResolutionType,
			ResolutionPayload: req.ResolutionPayload,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.scheduleGovernanceRunResumes(runIDsFromDecisions(items), runPriorityFromRequest(r, 0))
		writeJSON(w, http.StatusOK, decisionsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Decisions: items,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/:approvalId/approve", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		approvalID, action, ok := approvalActionFromPath(r.URL.Path)
		if !ok || action != "approve" {
			http.NotFound(w, r)
			return
		}
		state.metrics.approvalApproveRequests.Add(1)
		item, err := state.approveApproval(r.Context(), state.config.Backend, state.config.DataRoot, approvalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.scheduleGovernanceRunResumes([]string{item.RunID}, runPriorityFromRequest(r, 0))
		writeJSON(w, http.StatusOK, approvalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Approval: item,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/:approvalId/reject", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		approvalID, action, ok := approvalActionFromPath(r.URL.Path)
		if !ok || action != "reject" {
			http.NotFound(w, r)
			return
		}
		state.metrics.approvalRejectRequests.Add(1)
		item, err := state.rejectApproval(r.Context(), state.config.Backend, state.config.DataRoot, approvalID)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.scheduleGovernanceRunResumes([]string{item.RunID}, runPriorityFromRequest(r, 0))
		writeJSON(w, http.StatusOK, approvalResponse{
			Service:  "workorch",
			Backend:  state.config.Backend,
			DataRoot: state.config.DataRoot,
			Approval: item,
		})
	})
	router.AddHandler("/api/v1/workorch/approvals/resolve-batch", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(appservice.ContextWithEventHub(r.Context(), state.eventHub))
		var req batchResolveApprovalsRequest
		if err := decodeJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		if req.Action == "approve" {
			state.metrics.approvalApproveRequests.Add(uint64(len(req.ApprovalIDs)))
		} else if req.Action == "reject" {
			state.metrics.approvalRejectRequests.Add(uint64(len(req.ApprovalIDs)))
		}
		items, err := state.batchResolveApprovals(r.Context(), state.config.Backend, state.config.DataRoot, appservice.BatchResolveApprovalCommand{
			ApprovalIDs: req.ApprovalIDs,
			Action:      req.Action,
		})
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		state.scheduleGovernanceRunResumes(runIDsFromApprovals(items), runPriorityFromRequest(r, 0))
		writeJSON(w, http.StatusOK, approvalsResponse{
			Service:   "workorch",
			Backend:   state.config.Backend,
			DataRoot:  state.config.DataRoot,
			Approvals: items,
		})
	})
}

func registerConsoleRoutes(router engine.RouteRegistry) {
	handler := func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		serveConsoleAsset(w, r)
	}
	router.AddHandler("/favicon.ico", http.MethodGet, func(_ context.Context, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	router.AddHandler("/console", http.MethodGet, handler)
	router.AddHandler("/console/", http.MethodGet, handler)
	router.AddHandler("/console/**", http.MethodGet, handler)
}

func serveConsoleAsset(w http.ResponseWriter, r *http.Request) {
	staticFS, err := fs.Sub(consoleassets.Assets, "dist")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	assetPath := strings.TrimPrefix(r.URL.Path, "/console")
	switch {
	case assetPath == "" || assetPath == "/":
		serveConsoleFile(w, r, staticFS, "index.html")
	case strings.HasPrefix(assetPath, "/assets/"):
		serveConsoleFile(w, r, staticFS, strings.TrimPrefix(assetPath, "/"))
	default:
		serveConsoleFile(w, r, staticFS, "index.html")
	}
}

func serveConsoleFile(w http.ResponseWriter, r *http.Request, staticFS fs.FS, name string) {
	content, err := fs.ReadFile(staticFS, path.Clean(name))
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
}

func newServerState(config Config, eventHub events.Hub) *serverState {
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	state := &serverState{
		config:                          config,
		eventHub:                        eventHub,
		metrics:                         &serverMetrics{},
		validate:                        defaultValidate,
		createWorkspace:                 defaultCreateWorkspace,
		deleteWorkspace:                 defaultDeleteWorkspace,
		readWorkspace:                   defaultReadWorkspace,
		readWorkspaceSnapshot:           defaultReadWorkspaceSnapshot,
		readWorkspaceSoulBindings:       defaultReadWorkspaceSoulBindings,
		updateWorkspaceSoulBindings:     defaultUpdateWorkspaceSoulBindings,
		listWorkspaces:                  defaultListWorkspaces,
		defineGoal:                      defaultDefineGoal,
		updateGoal:                      defaultUpdateGoal,
		readGoal:                        defaultReadGoal,
		listGoals:                       defaultListGoals,
		defineDoD:                       defaultDefineDoD,
		updateDoD:                       defaultUpdateDoD,
		readDoD:                         defaultReadDoD,
		listDoDs:                        defaultListDoDs,
		checkGoalDoDQuality:             defaultCheckGoalDoDQuality,
		generateWorkspaceSetupDraft:     defaultGenerateWorkspaceSetupDraft,
		createRun:                       defaultCreateRun,
		readRun:                         defaultReadRun,
		readRunSnapshot:                 defaultReadRunSnapshot,
		listRunSteps:                    defaultListRunSteps,
		listRunToolCalls:                defaultListRunToolCalls,
		listRunRuntimeCheckpoints:       defaultListRunRuntimeCheckpoints,
		listRunResults:                  defaultListRunResults,
		listRunPatches:                  defaultListRunPatches,
		listRunValidationResults:        defaultListRunValidationResults,
		listRunArtifacts:                defaultListRunArtifacts,
		readRunStatus:                   defaultReadRunStatus,
		readRunObservability:            defaultReadRunObservability,
		readArtifactContent:             defaultReadArtifactContent,
		readRuntimeRawContent:           defaultReadRuntimeRawContent,
		readRunTaskProgress:             defaultReadRunTaskProgress,
		readWorkspaceTaskProgress:       defaultReadWorkspaceTaskProgress,
		readRunContextBundle:            defaultReadRunContextBundle,
		readWorkspaceContextBundle:      defaultReadWorkspaceContextBundle,
		readRunTaskDecompositions:       defaultReadRunTaskDecompositions,
		readWorkspaceTaskDecompositions: defaultReadWorkspaceTaskDecompositions,
		listMemoryRecallTraces:          defaultListMemoryRecallTraces,
		listEventRecords:                defaultListEventRecords,
		listRuns:                        defaultListRuns,
		startRun:                        defaultStartRun,
		resumeRun:                       defaultResumeRun,
		activateWorkspace:               defaultActivateWorkspaceExecution,
		listDecisions:                   defaultListDecisions,
		listDecisionPage:                defaultListDecisionPage,
		readDecision:                    defaultReadDecision,
		resolveDecision:                 defaultResolveDecision,
		batchResolveDecisions:           defaultBatchResolveDecisions,
		listApprovals:                   defaultListApprovals,
		listApprovalPage:                defaultListApprovalPage,
		readApproval:                    defaultReadApproval,
		approveApproval:                 defaultApproveApproval,
		rejectApproval:                  defaultRejectApproval,
		batchResolveApprovals:           defaultBatchResolveApprovals,
		createSoul:                      defaultCreateSoul,
		updateSoul:                      defaultUpdateSoul,
		deleteSoul:                      defaultDeleteSoul,
		readSoul:                        defaultReadSoul,
		listSouls:                       defaultListSouls,
		createWorkspaceTemplate:         defaultCreateWorkspaceTemplate,
		updateWorkspaceTemplate:         defaultUpdateWorkspaceTemplate,
		deleteWorkspaceTemplate:         defaultDeleteWorkspaceTemplate,
		listWorkspaceTemplates:          defaultListWorkspaceTemplates,
		createSkill:                     defaultCreateSkill,
		readSkill:                       defaultReadSkill,
		listSkills:                      defaultListSkills,
		createMCPServer:                 defaultCreateMCPServer,
		updateMCPServer:                 defaultUpdateMCPServer,
		readMCPServer:                   defaultReadMCPServer,
		listMCPServers:                  defaultListMCPServers,
		listMCPServerTools:              defaultListMCPServerTools,
		listMCPServerResources:          defaultListMCPServerResources,
		listMCPServerPrompts:            defaultListMCPServerPrompts,
		createMCPCapability:             defaultCreateMCPCapability,
		updateMCPCapability:             defaultUpdateMCPCapability,
		readMCPCapability:               defaultReadMCPCapability,
		listMCPCapabilities:             defaultListMCPCapabilities,
		createMemoryItem:                defaultCreateMemoryItem,
		readMemoryItem:                  defaultReadMemoryItem,
		updateMemoryItem:                defaultUpdateMemoryItem,
		deleteMemoryItem:                defaultDeleteMemoryItem,
		listMemoryByWorkspace:           defaultListMemoryByWorkspace,
		searchMemory:                    defaultSearchMemory,
		readConfiguration:               defaultReadConfiguration,
		updateServerConfiguration:       appservice.UpdateServerConfiguration,
		updateLLMConfiguration:          appservice.UpdateLLMConfiguration,
		updateWebSearchConfiguration:    appservice.UpdateWebSearchConfiguration,
		backgroundCtx:                   backgroundCtx,
		backgroundCancel:                backgroundCancel,
		workspaceSetupDraftSessions:     map[string]*workspaceSetupDraftSession{},
		replanContinuationStarted:       map[string]struct{}{},
		governanceResumeStarted:         map[string]struct{}{},
	}
	state.configureApplicationRuntimeSession()
	state.configureSharedMemorySession()
	state.registerAutoGovernanceResumeWatcher()
	state.registerReplanContinuationWatcher()
	return state
}

func (s *serverState) configureApplicationRuntimeSession() {
	if s == nil || s.sharedSession != nil {
		return
	}
	session, err := appservice.NewRuntimeSessionFromInitiator()
	if err != nil {
		return
	}
	s.configureSessionBackedOperations(session)
}

func (s *serverState) configureSharedMemorySession() {
	if s == nil || s.sharedSession != nil || !isMemoryBackend(s.config.Backend) {
		return
	}
	session, err := appservice.NewRuntimeSessionWithBackendAndEventHub(s.config.Backend, s.config.DataRoot, s.eventHub)
	if err != nil {
		return
	}
	s.configureSessionBackedOperations(session)
}

func (s *serverState) configureSessionBackedOperations(session *appservice.RuntimeSession) {
	if s == nil || session == nil {
		return
	}
	if s.eventHub == nil {
		if exports, err := session.Exports(); err == nil && exports.Infrastructure.EventHub != nil {
			s.eventHub = exports.Infrastructure.EventHub
		}
	}
	s.sharedSession = session
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
	s.readWorkspaceTaskProgress = func(ctx context.Context, _, _, workspaceID string) (appservice.TaskProgress, error) {
		var ret appservice.TaskProgress
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceTaskProgressWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readWorkspaceContextBundle = func(ctx context.Context, _, _, workspaceID string) (appservice.ContextBundle, error) {
		var ret appservice.ContextBundle
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceContextBundleWithSession(ctx, session, workspaceID)
			return err
		})
		return ret, err
	}
	s.readWorkspaceTaskDecompositions = func(ctx context.Context, _, _, workspaceID string) ([]appservice.TaskDecomposition, error) {
		var ret []appservice.TaskDecomposition
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceTaskDecompositionsWithSession(ctx, session, workspaceID)
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
	s.resumeRun = func(ctx context.Context, _, _, runID string, executionContextPatch ...map[string]any) (appservice.Run, error) {
		var ret appservice.Run
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ResumeRunWithSession(ctx, session, runID, executionContextPatch...)
			return err
		})
		return ret, err
	}
	s.activateWorkspace = func(ctx context.Context, _, _, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
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
	s.approveApproval = func(ctx context.Context, _, _, approvalID string) (appservice.Approval, error) {
		var ret appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ApproveApprovalWithSession(ctx, session, approvalID)
			return err
		})
		return ret, err
	}
	s.rejectApproval = func(ctx context.Context, _, _, approvalID string) (appservice.Approval, error) {
		var ret appservice.Approval
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.RejectApprovalWithSession(ctx, session, approvalID)
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

func (s *serverState) registerReplanContinuationWatcher() {
	if s == nil || s.eventHub == nil {
		return
	}
	unsubscribe := s.eventHub.Subscribe(events.TopicRunStateChanged, func(context.Context, events.Envelope) error {
		return nil
	})
	s.eventUnsubscribes = append(s.eventUnsubscribes, unsubscribe)
}

func (s *serverState) registerAutoGovernanceResumeWatcher() {
	if s == nil || s.eventHub == nil {
		return
	}
	unsubscribe := s.eventHub.Subscribe(events.TopicGovernanceAutoResolved, func(_ context.Context, envelope events.Envelope) error {
		if strings.TrimSpace(envelope.RunID) == "" {
			return nil
		}
		s.scheduleGovernanceRunResumes([]string{envelope.RunID}, 0)
		return nil
	})
	s.eventUnsubscribes = append(s.eventUnsubscribes, unsubscribe)
}

func isMemoryBackend(backend string) bool {
	return backend == "memory"
}

func enqueueEventStreamEnvelope(ch chan events.Envelope, envelope events.Envelope) {
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
	case events.TopicWorkspaceCreated,
		events.TopicWorkspaceDeleted,
		events.TopicWorkspaceStateChanged:
		return true
	default:
		return false
	}
}

func (s *serverState) withSharedSession(ctx context.Context, fn func(*appservice.RuntimeSession) error) error {
	if s == nil || s.sharedSession == nil {
		return common.ErrInvalidArgument
	}
	session := s.sharedSession.Borrow()
	err := fn(session)
	return errors.Join(err, session.Close(ctx))
}

func (s *serverState) close(ctx context.Context) error {
	if s == nil {
		return nil
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
	if err := s.waitBackground(ctx); err != nil {
		return err
	}
	if s.sharedSession == nil {
		return nil
	}
	err := s.sharedSession.Close(ctx)
	s.sharedSession = nil
	return err
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

func (s *serverState) backgroundContext() context.Context {
	if s != nil && s.backgroundCtx != nil {
		return s.backgroundCtx
	}
	return context.Background()
}

func (s *serverState) scheduleStartupRecovery() {
	if s == nil {
		return
	}
	ctx := s.backgroundContext()
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
		if err := s.recoverStuckRunningRuns(appservice.ContextWithEventHub(ctx, s.eventHub)); err != nil {
			slog.Warn("workorch startup run recovery failed", "err", err)
		}
	}()
}

func (s *serverState) recoverStuckRunningRuns(ctx context.Context) error {
	if s == nil {
		return nil
	}
	var recovered []appservice.Run
	var err error
	if s.sharedSession != nil {
		err = s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var recoverErr error
			recovered, recoverErr = appservice.RecoverStuckRunningRunsWithSession(ctx, session)
			return recoverErr
		})
	} else {
		recovered, err = appservice.RecoverStuckRunningRunsWithBackend(ctx, s.config.Backend, s.config.DataRoot)
	}
	if err != nil {
		return err
	}
	if len(recovered) > 0 {
		runIDs := make([]string, 0, len(recovered))
		for _, item := range recovered {
			runIDs = append(runIDs, item.ID)
		}
		slog.Warn("workorch recovered stuck running runs", "runIDs", runIDs)
	}
	s.recoverReplanContinuations(ctx)
	return nil
}

func (s *serverState) recoverReplanContinuations(ctx context.Context) {
	if s == nil || s.listWorkspaces == nil || s.listRuns == nil {
		return
	}
	workspaces, err := s.listWorkspaces(ctx, s.config.Backend, s.config.DataRoot)
	if err != nil {
		slog.Warn("workorch replan continuation workspace scan failed", "err", err)
		return
	}
	for _, workspace := range workspaces {
		if strings.TrimSpace(workspace.ID) == "" ||
			strings.TrimSpace(workspace.ActiveRunID) != "" ||
			workspace.State != string(common.WorkspaceStateFailed) {
			continue
		}
		runs, err := s.listRuns(ctx, s.config.Backend, s.config.DataRoot, workspace.ID)
		if err != nil {
			slog.Warn("workorch replan continuation run scan failed", "workspaceID", workspace.ID, "err", err)
			continue
		}
		for _, run := range runs {
			if run.WorkspaceID != workspace.ID || run.State != string(common.RunStateSuperseded) {
				continue
			}
			s.scheduleReplanContinuationIfNeeded(ctx, workspace.ID, run.ID, run.Priority)
		}
	}
}

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

func defaultActivateWorkspaceExecution(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
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

func defaultApproveApproval(ctx context.Context, backend string, dataRoot string, approvalID string) (appservice.Approval, error) {
	return appservice.ApproveApprovalWithBackend(ctx, backend, dataRoot, approvalID)
}

func defaultRejectApproval(ctx context.Context, backend string, dataRoot string, approvalID string) (appservice.Approval, error) {
	return appservice.RejectApprovalWithBackend(ctx, backend, dataRoot, approvalID)
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

func (s *serverState) acquireRunSlot(ctx context.Context) (func(), error) {
	return s.acquireRunSlotWithPriority(ctx, 0)
}

func (s *serverState) readRunStatusWithWait(ctx context.Context, runID string, values url.Values) (appservice.RunStatus, bool, error) {
	waitState := strings.TrimSpace(values.Get("waitState"))
	timeout := durationFromMillisQuery(values, "waitTimeoutMillis", 0, 30*time.Second)
	interval := durationFromMillisQuery(values, "waitIntervalMillis", 50*time.Millisecond, time.Second)
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	if cached, ok := s.cachedRunSnapshot(runID); ok {
		status := appservice.RunStatusFromSnapshot(cached)
		if waitState == "" || status.State == waitState {
			return status, waitState == "" || status.State == waitState, nil
		}
	}
	if waitState == "" || timeout <= 0 {
		item, err := s.readRunStatus(ctx, s.config.Backend, s.config.DataRoot, runID)
		matched := waitState == "" || item.State == waitState
		if err == nil && matched {
			s.warmRunSnapshotCache(ctx, runID, item.State)
		}
		return item, matched, err
	}
	deadline := time.Now().Add(timeout)
	for {
		notify := s.runStatusWaitNotify()
		item, err := s.readRunStatus(ctx, s.config.Backend, s.config.DataRoot, runID)
		if err != nil {
			return appservice.RunStatus{}, false, err
		}
		if item.State == waitState {
			s.warmRunSnapshotCache(ctx, runID, item.State)
			return item, true, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return item, false, nil
		}
		sleepFor := interval
		if remaining < sleepFor {
			sleepFor = remaining
		}
		timer := time.NewTimer(sleepFor)
		select {
		case <-ctx.Done():
			timer.Stop()
			return appservice.RunStatus{}, false, ctx.Err()
		case <-notify:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (s *serverState) warmRunSnapshotCache(ctx context.Context, runID string, state string) {
	if s == nil || strings.TrimSpace(runID) == "" || !terminalRunState(state) {
		return
	}
	if _, ok := s.cachedRunSnapshot(runID); ok {
		return
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	if _, ok := s.ensureRunSnapshotInflight(runID); ok {
		return
	}
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		defer s.finishRunSnapshotInflight(runID)
		warmCtx, cancel := context.WithTimeout(s.backgroundContext(), runSnapshotCacheTTL)
		defer cancel()
		item, err := s.readRunSnapshot(warmCtx, s.config.Backend, s.config.DataRoot, runID)
		s.completeRunSnapshotInflight(runID, item, err)
	}()
}

func (s *serverState) readRunSnapshotCached(ctx context.Context, runID string) (appservice.RunSnapshot, error) {
	if cached, ok := s.cachedRunSnapshot(runID); ok {
		return cached, nil
	}
	if item, err, ok := s.waitRunSnapshotInflight(ctx, runID); ok {
		return item, err
	}
	item, err := s.readRunSnapshot(ctx, s.config.Backend, s.config.DataRoot, runID)
	if err != nil {
		return appservice.RunSnapshot{}, err
	}
	s.cacheRunSnapshot(item)
	return item, nil
}

func (s *serverState) readWorkspaceSnapshotCached(ctx context.Context, workspaceID string) (appservice.WorkspaceSnapshot, error) {
	if cached, ok := s.cachedRunSnapshotByWorkspace(workspaceID); ok && s.sharedSession != nil {
		var ret appservice.WorkspaceSnapshot
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadWorkspaceSnapshotWithRunSnapshotWithSession(ctx, session, workspaceID, cached)
			return err
		})
		return ret, err
	}
	return s.readWorkspaceSnapshot(ctx, s.config.Backend, s.config.DataRoot, workspaceID)
}

func (s *serverState) readArtifactContentCached(ctx context.Context, artifactURI string) (appservice.ArtifactContent, error) {
	if cached, ok := s.cachedRunSnapshotByArtifactURI(artifactURI); ok && s.sharedSession != nil {
		var ret appservice.ArtifactContent
		err := s.withSharedSession(ctx, func(session *appservice.RuntimeSession) error {
			var err error
			ret, err = appservice.ReadArtifactContentWithRunSnapshotWithSession(ctx, session, artifactURI, cached)
			return err
		})
		return ret, err
	}
	return s.readArtifactContent(ctx, s.config.Backend, s.config.DataRoot, artifactURI)
}

func (s *serverState) readRunObservabilityCached(ctx context.Context, runID string) (appservice.RunObservability, error) {
	if cached, ok := s.cachedRunSnapshot(runID); ok {
		return appservice.RunObservabilityFromSnapshot(cached), nil
	}
	if item, err, ok := s.waitRunSnapshotInflight(ctx, runID); ok {
		if err != nil {
			return appservice.RunObservability{}, err
		}
		return appservice.RunObservabilityFromSnapshot(item), nil
	}
	item, err := s.readRunObservability(ctx, s.config.Backend, s.config.DataRoot, runID)
	if err != nil {
		return appservice.RunObservability{}, err
	}
	return item, nil
}

func (s *serverState) cachedRunSnapshot(runID string) (appservice.RunSnapshot, bool) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return appservice.RunSnapshot{}, false
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	entry, ok := s.runSnapshotCache[runID]
	if !ok || time.Since(entry.cachedAt) > runSnapshotCacheTTL {
		if ok {
			delete(s.runSnapshotCache, runID)
		}
		return appservice.RunSnapshot{}, false
	}
	return entry.snapshot, true
}

func (s *serverState) cachedRunSnapshotByWorkspace(workspaceID string) (appservice.RunSnapshot, bool) {
	if s == nil || strings.TrimSpace(workspaceID) == "" {
		return appservice.RunSnapshot{}, false
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	now := time.Now()
	for runID, entry := range s.runSnapshotCache {
		if now.Sub(entry.cachedAt) > runSnapshotCacheTTL {
			delete(s.runSnapshotCache, runID)
			continue
		}
		if entry.snapshot.Run.WorkspaceID == workspaceID {
			return entry.snapshot, true
		}
	}
	return appservice.RunSnapshot{}, false
}

func (s *serverState) ensureRunSnapshotInflight(runID string) (*runSnapshotInflight, bool) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return nil, true
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	if s.runSnapshotInflight == nil {
		s.runSnapshotInflight = map[string]*runSnapshotInflight{}
	}
	if current, ok := s.runSnapshotInflight[runID]; ok {
		return current, true
	}
	current := &runSnapshotInflight{done: make(chan struct{})}
	s.runSnapshotInflight[runID] = current
	return current, false
}

func (s *serverState) waitRunSnapshotInflight(ctx context.Context, runID string) (appservice.RunSnapshot, error, bool) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return appservice.RunSnapshot{}, nil, false
	}
	s.runSnapshotCacheMu.Lock()
	current, ok := s.runSnapshotInflight[runID]
	s.runSnapshotCacheMu.Unlock()
	if !ok {
		return appservice.RunSnapshot{}, nil, false
	}
	select {
	case <-ctx.Done():
		return appservice.RunSnapshot{}, ctx.Err(), true
	case <-current.done:
		return current.snapshot, current.err, true
	}
}

func (s *serverState) completeRunSnapshotInflight(runID string, snapshot appservice.RunSnapshot, err error) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	current, ok := s.runSnapshotInflight[runID]
	if !ok {
		return
	}
	current.snapshot = snapshot
	current.err = err
	if err == nil && snapshot.Run.ID != "" && terminalRunState(snapshot.Run.State) {
		if s.runSnapshotCache == nil {
			s.runSnapshotCache = map[string]cachedRunSnapshot{}
		}
		s.runSnapshotCache[snapshot.Run.ID] = cachedRunSnapshot{snapshot: snapshot, cachedAt: time.Now()}
	}
	close(current.done)
}

func (s *serverState) finishRunSnapshotInflight(runID string) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	delete(s.runSnapshotInflight, runID)
}

func (s *serverState) cachedRunSnapshotByArtifactURI(artifactURI string) (appservice.RunSnapshot, bool) {
	if s == nil || strings.TrimSpace(artifactURI) == "" {
		return appservice.RunSnapshot{}, false
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	now := time.Now()
	for runID, entry := range s.runSnapshotCache {
		if now.Sub(entry.cachedAt) > runSnapshotCacheTTL {
			delete(s.runSnapshotCache, runID)
			continue
		}
		for _, artifact := range entry.snapshot.Artifacts {
			if strings.TrimSpace(artifact.URI) == artifactURI {
				return entry.snapshot, true
			}
		}
	}
	return appservice.RunSnapshot{}, false
}

func (s *serverState) cacheRunSnapshot(snapshot appservice.RunSnapshot) {
	if s == nil || snapshot.Run.ID == "" || !terminalRunState(snapshot.Run.State) {
		return
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	if s.runSnapshotCache == nil {
		s.runSnapshotCache = map[string]cachedRunSnapshot{}
	}
	s.runSnapshotCache[snapshot.Run.ID] = cachedRunSnapshot{snapshot: snapshot, cachedAt: time.Now()}
}

func (s *serverState) clearRunSnapshotCache() {
	if s == nil {
		return
	}
	s.runSnapshotCacheMu.Lock()
	defer s.runSnapshotCacheMu.Unlock()
	clear(s.runSnapshotCache)
}

func (s *serverState) runStatusWaitNotify() <-chan struct{} {
	if s == nil {
		return closedNotifyChannel()
	}
	s.runStatusNotifyMu.Lock()
	defer s.runStatusNotifyMu.Unlock()
	if s.runStatusNotify == nil {
		s.runStatusNotify = make(chan struct{})
	}
	return s.runStatusNotify
}

func (s *serverState) notifyRunStatusChanged() {
	if s == nil {
		return
	}
	s.clearRunSnapshotCache()
	s.notifyRunStatusWaiters()
}

func (s *serverState) notifyRunStatusChangedForRun(run appservice.Run) {
	if s == nil {
		return
	}
	s.clearRunSnapshotCache()
	s.warmRunSnapshotCache(context.Background(), run.ID, run.State)
	s.notifyRunStatusWaiters()
}

func (s *serverState) notifyRunStatusWaiters() {
	if s == nil {
		return
	}
	s.runStatusNotifyMu.Lock()
	defer s.runStatusNotifyMu.Unlock()
	if s.runStatusNotify == nil {
		s.runStatusNotify = make(chan struct{})
		return
	}
	close(s.runStatusNotify)
	s.runStatusNotify = make(chan struct{})
}

func closedNotifyChannel() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func durationFromMillisQuery(values url.Values, key string, fallback time.Duration, maxValue time.Duration) time.Duration {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return fallback
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || millis < 0 {
		return fallback
	}
	ret := time.Duration(millis) * time.Millisecond
	if maxValue > 0 && ret > maxValue {
		return maxValue
	}
	return ret
}

func (s *serverState) scheduleGovernanceRunResumes(runIDs []string, priority int) {
	runIDs = uniqueNonEmptyStrings(runIDs)
	for _, runID := range runIDs {
		runID := runID
		if !s.markGovernanceResumeStarted(runID) {
			slog.Debug("workorch governance resume already scheduled", "runID", runID)
			continue
		}
		s.backgroundWG.Add(1)
		go func() {
			defer s.backgroundWG.Done()
			defer s.clearGovernanceResumeStarted(runID)
			var resumedRun appservice.Run
			defer func() {
				if resumedRun.ID != "" {
					s.notifyRunStatusChangedForRun(resumedRun)
					return
				}
				s.notifyRunStatusChanged()
			}()
			ctx := appservice.ContextWithEventHub(s.backgroundContext(), s.eventHub)
			s.metrics.runResumeRequests.Add(1)
			release, err := s.acquireRunSlotWithPriority(ctx, priority)
			if err != nil {
				s.metrics.runCanceled.Add(1)
				slog.Warn("workorch governance resume queue failed", "runID", runID, "err", err)
				return
			}
			defer release()
			item, err := s.resumeRun(ctx, s.config.Backend, s.config.DataRoot, runID)
			if err != nil {
				slog.Warn("workorch governance resume failed", "runID", runID, "err", err)
				return
			}
			s.metrics.observeRunOutcome(item)
			if item.State == string(common.RunStateSuperseded) {
				s.scheduleReplanContinuationIfNeeded(ctx, item.WorkspaceID, item.ID, priority)
			}
			resumedRun = item
		}()
	}
}

func (s *serverState) markGovernanceResumeStarted(runID string) bool {
	if s == nil {
		return false
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	s.governanceResumeMu.Lock()
	defer s.governanceResumeMu.Unlock()
	if s.governanceResumeStarted == nil {
		s.governanceResumeStarted = map[string]struct{}{}
	}
	if _, ok := s.governanceResumeStarted[runID]; ok {
		return false
	}
	s.governanceResumeStarted[runID] = struct{}{}
	return true
}

func (s *serverState) clearGovernanceResumeStarted(runID string) {
	if s == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	s.governanceResumeMu.Lock()
	defer s.governanceResumeMu.Unlock()
	delete(s.governanceResumeStarted, runID)
}

func (s *serverState) scheduleReplanContinuationIfNeeded(ctx context.Context, workspaceID string, runID string, priority int) {
	if s == nil || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(runID) == "" {
		return
	}
	if !s.runHasResolvedReplanDecision(ctx, workspaceID, runID) {
		return
	}
	if s.workspaceHasReplanSuccessor(ctx, workspaceID, runID) {
		return
	}
	key := workspaceID + "/" + runID
	s.replanContinuationMu.Lock()
	if s.replanContinuationStarted == nil {
		s.replanContinuationStarted = map[string]struct{}{}
	}
	if _, ok := s.replanContinuationStarted[key]; ok {
		s.replanContinuationMu.Unlock()
		return
	}
	s.replanContinuationStarted[key] = struct{}{}
	s.replanContinuationMu.Unlock()

	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		runCtx := appservice.ContextWithEventHub(s.backgroundContext(), s.eventHub)
		release, err := s.acquireRunSlotWithPriority(runCtx, priority)
		if err != nil {
			s.clearReplanContinuationStarted(key)
			s.metrics.runCanceled.Add(1)
			slog.Warn("workorch replan continuation queue failed", "workspaceID", workspaceID, "runID", runID, "err", err)
			return
		}
		defer release()
		activation, err := s.activateWorkspace(runCtx, s.config.Backend, s.config.DataRoot, workspaceID, appservice.ActivateWorkspaceExecutionCommand{Priority: priority})
		if err != nil {
			s.clearReplanContinuationStarted(key)
			slog.Warn("workorch replan continuation activation failed", "workspaceID", workspaceID, "runID", runID, "err", err)
			s.notifyRunStatusChanged()
			return
		}
		s.metrics.observeRunOutcome(activation.Run)
		s.notifyRunStatusChangedForRun(activation.Run)
	}()
}

func (s *serverState) clearReplanContinuationStarted(key string) {
	if s == nil || strings.TrimSpace(key) == "" {
		return
	}
	s.replanContinuationMu.Lock()
	defer s.replanContinuationMu.Unlock()
	delete(s.replanContinuationStarted, key)
}

func (s *serverState) workspaceHasReplanSuccessor(ctx context.Context, workspaceID string, runID string) bool {
	if s == nil || s.listRuns == nil || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(runID) == "" {
		return false
	}
	items, err := s.listRuns(ctx, s.config.Backend, s.config.DataRoot, workspaceID)
	if err != nil {
		slog.Warn("workorch replan continuation successor scan failed", "workspaceID", workspaceID, "runID", runID, "err", err)
		return false
	}
	for _, item := range items {
		if item.WorkspaceID != workspaceID || item.ID == runID {
			continue
		}
		if runIsReplanSuccessorOf(item, runID) {
			return true
		}
	}
	return false
}

func runIsReplanSuccessorOf(item appservice.Run, predecessorRunID string) bool {
	predecessorRunID = strings.TrimSpace(predecessorRunID)
	if predecessorRunID == "" {
		return false
	}
	if replan, _ := item.ExecutionContext["replanContinuation"].(map[string]any); len(replan) > 0 {
		if predecessor, _ := replan["predecessorRunId"].(string); strings.TrimSpace(predecessor) == predecessorRunID {
			return true
		}
	}
	return strings.HasPrefix(item.ID, predecessorRunID+"-replan-")
}

func (s *serverState) runHasResolvedReplanDecision(ctx context.Context, workspaceID string, runID string) bool {
	if s == nil || s.listDecisions == nil {
		return false
	}
	items, err := s.listDecisions(ctx, s.config.Backend, s.config.DataRoot, workspaceID, runID)
	if err != nil {
		slog.Warn("workorch replan continuation decision lookup failed", "workspaceID", workspaceID, "runID", runID, "err", err)
		return false
	}
	for _, item := range items {
		if item.RunID == runID &&
			item.State == string(common.DecisionTicketStateResolved) &&
			item.ResolutionType == decisioncommon.ResolutionTypeReplan {
			return true
		}
	}
	return false
}

func runIDsFromDecisions(items []appservice.Decision) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.RunID)
	}
	return ret
}

func runIDsFromApprovals(items []appservice.Approval) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.RunID)
	}
	return ret
}

func uniqueNonEmptyStrings(items []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func (s *serverState) acquireRunSlotWithPriority(ctx context.Context, priority int) (func(), error) {
	s.runQueueMu.Lock()
	if s.activeRunSlots < s.config.MaxConcurrentRuns && len(s.runQueueWaiters) == 0 {
		s.activeRunSlots++
		s.metrics.runInFlight.Add(1)
		s.runQueueMu.Unlock()
		return s.releaseRunSlot, nil
	}

	s.runQueueSequence++
	waiter := &runQueueWaiter{
		ready:    make(chan struct{}),
		priority: priority,
		sequence: s.runQueueSequence,
	}
	s.runQueueWaiters = append(s.runQueueWaiters, waiter)
	s.metrics.runQueueWaiting.Add(1)
	s.metrics.runQueuedTotal.Add(1)
	s.runQueueMu.Unlock()

	select {
	case <-waiter.ready:
		if ctx.Err() != nil {
			s.releaseRunSlot()
			return nil, ctx.Err()
		}
		return s.releaseRunSlot, nil
	case <-ctx.Done():
		if s.cancelQueuedRunWaiter(waiter.ready) {
			return nil, ctx.Err()
		}
		s.releaseRunSlot()
		return nil, ctx.Err()
	}
}

func (s *serverState) releaseRunSlot() {
	s.runQueueMu.Lock()
	if len(s.runQueueWaiters) > 0 {
		idx := s.nextRunQueueWaiterIndex()
		waiter := s.runQueueWaiters[idx]
		s.runQueueWaiters = append(s.runQueueWaiters[:idx], s.runQueueWaiters[idx+1:]...)
		s.metrics.runQueueWaiting.Add(-1)
		s.runQueueMu.Unlock()
		close(waiter.ready)
		return
	}
	if s.activeRunSlots > 0 {
		s.activeRunSlots--
		s.metrics.runInFlight.Add(-1)
	}
	s.runQueueMu.Unlock()
}

func (s *serverState) cancelQueuedRunWaiter(target chan struct{}) bool {
	s.runQueueMu.Lock()
	defer s.runQueueMu.Unlock()

	for idx, waiter := range s.runQueueWaiters {
		if waiter.ready != target {
			continue
		}
		s.runQueueWaiters = append(s.runQueueWaiters[:idx], s.runQueueWaiters[idx+1:]...)
		s.metrics.runQueueWaiting.Add(-1)
		return true
	}
	return false
}

func (s *serverState) nextRunQueueWaiterIndex() int {
	best := 0
	for idx := 1; idx < len(s.runQueueWaiters); idx++ {
		if runQueueWaiterBefore(s.runQueueWaiters[idx], s.runQueueWaiters[best]) {
			best = idx
		}
	}
	return best
}

func runQueueWaiterBefore(left *runQueueWaiter, right *runQueueWaiter) bool {
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	if left.priority != right.priority {
		return left.priority > right.priority
	}
	return left.sequence < right.sequence
}

func runPriorityFromRequest(r *http.Request, bodyPriority int) int {
	if bodyPriority != 0 {
		return bodyPriority
	}
	if r == nil {
		return 0
	}
	for _, raw := range []string{
		r.URL.Query().Get("priority"),
		r.Header.Get("X-Workorch-Run-Priority"),
	} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		priority, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil {
			return priority
		}
	}
	return 0
}

func eventRecordQueryFromRequest(r *http.Request) appservice.EventRecordQuery {
	values := r.URL.Query()
	limit := defaultEventRecordPageLimit
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit <= 0 {
		limit = defaultEventRecordPageLimit
	}
	if limit > maxEventRecordPageLimit {
		limit = maxEventRecordPageLimit
	}
	return appservice.EventRecordQuery{
		WorkspaceID:    values.Get("workspaceId"),
		RunID:          values.Get("runId"),
		Topic:          values.Get("topic"),
		SubjectType:    values.Get("subjectType"),
		SubjectID:      values.Get("subjectId"),
		CursorAfterID:  values.Get("cursorAfterId"),
		OccurredAfter:  values.Get("occurredAfter"),
		OccurredBefore: values.Get("occurredBefore"),
		Limit:          limit,
	}
}

func stepQueryFromRequest(r *http.Request) appservice.StepQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.StepQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func toolCallQueryFromRequest(r *http.Request) appservice.ToolCallQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.ToolCallQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func runtimeCheckpointQueryFromRequest(r *http.Request) appservice.RuntimeCheckpointQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.RuntimeCheckpointQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func resultQueryFromRequest(r *http.Request) appservice.ResultQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.ResultQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func patchQueryFromRequest(r *http.Request) appservice.PatchQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.PatchQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func validationResultQueryFromRequest(r *http.Request) appservice.ValidationResultQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.ValidationResultQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func artifactQueryFromRequest(r *http.Request) appservice.ArtifactQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.ArtifactQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func decisionQueryFromRequest(r *http.Request) appservice.DecisionQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.DecisionQuery{
		WorkspaceID:   values.Get("workspaceId"),
		RunID:         values.Get("runId"),
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func approvalQueryFromRequest(r *http.Request) appservice.ApprovalQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.ApprovalQuery{
		WorkspaceID:   values.Get("workspaceId"),
		RunID:         values.Get("runId"),
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func mcpCapabilityQueryFromRequest(r *http.Request) appservice.MCPCapabilityQuery {
	values := r.URL.Query()
	var enabled *bool
	if raw := strings.TrimSpace(values.Get("enabled")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			enabled = &parsed
		}
	}
	return appservice.MCPCapabilityQuery{
		ServerID:  values.Get("serverId"),
		Kind:      values.Get("kind"),
		Category:  values.Get("category"),
		Stage:     values.Get("stage"),
		RiskLevel: values.Get("riskLevel"),
		Enabled:   enabled,
		Query:     values.Get("q"),
	}
}

func (m *serverMetrics) render(config Config) string {
	return fmt.Sprintf(
		"# TYPE workorch_http_requests_total counter\n"+
			"workorch_http_requests_total{endpoint=\"healthz\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"readyz\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"metrics\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_create\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_delete\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_snapshot\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"goal_define\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"goal_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"goal_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"dod_define\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"dod_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"dod_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_create\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_snapshot\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_status\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_observability\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"artifact_content\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_task_progress\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_task_progress\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_context_bundle\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_context_bundle\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_task_decomposition\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_task_decomposition\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_start\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_resume\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_execution_activation\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_resolve\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_approve\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_reject\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"configuration_read\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"configuration_update\"} %d\n"+
			"# TYPE workorch_run_requests_in_flight gauge\n"+
			"workorch_run_requests_in_flight %d\n"+
			"# TYPE workorch_run_queue_waiting gauge\n"+
			"workorch_run_queue_waiting %d\n"+
			"# TYPE workorch_run_queue_total counter\n"+
			"workorch_run_queue_total %d\n"+
			"# TYPE workorch_run_outcomes_total counter\n"+
			"workorch_run_outcomes_total{status=\"succeeded\"} %d\n"+
			"workorch_run_outcomes_total{status=\"failed\"} %d\n"+
			"workorch_run_outcomes_total{status=\"rejected\"} %d\n"+
			"workorch_run_outcomes_total{status=\"canceled\"} %d\n"+
			"# TYPE workorch_server_max_concurrent_runs gauge\n"+
			"workorch_server_max_concurrent_runs %d\n",
		m.healthzRequests.Load(),
		m.readyzRequests.Load(),
		m.metricsRequests.Load(),
		m.workspaceCreateRequests.Load(),
		m.workspaceDeleteRequests.Load(),
		m.workspaceListRequests.Load(),
		m.workspaceGetRequests.Load(),
		m.workspaceSnapshotRequests.Load(),
		m.goalDefineRequests.Load(),
		m.goalListRequests.Load(),
		m.goalGetRequests.Load(),
		m.dodDefineRequests.Load(),
		m.dodListRequests.Load(),
		m.dodGetRequests.Load(),
		m.runCreateRequests.Load(),
		m.runListRequests.Load(),
		m.runGetRequests.Load(),
		m.runSnapshotRequests.Load(),
		m.runStatusRequests.Load(),
		m.runObservabilityRequests.Load(),
		m.artifactContentRequests.Load(),
		m.runTaskProgressRequests.Load(),
		m.workspaceTaskProgressRequests.Load(),
		m.runContextBundleRequests.Load(),
		m.workspaceContextBundleRequests.Load(),
		m.runTaskDecompositionRequests.Load(),
		m.workspaceTaskDecompositionRequests.Load(),
		m.runStartRequests.Load(),
		m.runResumeRequests.Load(),
		m.workspaceActivationRequests.Load(),
		m.decisionListRequests.Load(),
		m.decisionGetRequests.Load(),
		m.decisionResolveRequests.Load(),
		m.approvalListRequests.Load(),
		m.approvalGetRequests.Load(),
		m.approvalApproveRequests.Load(),
		m.approvalRejectRequests.Load(),
		m.configurationReadRequests.Load(),
		m.configurationUpdateRequests.Load(),
		m.runInFlight.Load(),
		m.runQueueWaiting.Load(),
		m.runQueuedTotal.Load(),
		m.runSucceeded.Load(),
		m.runFailed.Load(),
		m.runRejected.Load(),
		m.runCanceled.Load(),
		config.MaxConcurrentRuns,
	)
}

func resourceIDFromPath(path string, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	trimmed := strings.TrimPrefix(path, prefix)
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return "", false
	}
	return trimmed, true
}

func memoryQueryFromValues(values url.Values) appservice.MemoryQuery {
	limit, _ := strconv.Atoi(strings.TrimSpace(values.Get("limit")))
	return appservice.MemoryQuery{
		WorkspaceID: strings.TrimSpace(values.Get("workspaceId")),
		Query:       strings.TrimSpace(values.Get("q")),
		Tags:        compactQueryValues(values["tag"]),
		Types:       compactQueryValues(values["type"]),
		Limit:       limit,
	}
}

func memoryRecallTraceQueryFromValues(values url.Values) appservice.MemoryRecallTraceQuery {
	limit, _ := strconv.Atoi(strings.TrimSpace(values.Get("limit")))
	return appservice.MemoryRecallTraceQuery{
		WorkspaceID: strings.TrimSpace(values.Get("workspaceId")),
		RunID:       strings.TrimSpace(values.Get("runId")),
		StepID:      strings.TrimSpace(values.Get("stepId")),
		Stage:       strings.TrimSpace(values.Get("stage")),
		Source:      strings.TrimSpace(values.Get("source")),
		Status:      strings.TrimSpace(values.Get("status")),
		Limit:       limit,
	}
}

func compactQueryValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		ret = append(ret, value)
	}
	return ret
}

func workspaceActionFromPath(path string) (workspaceID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/workspaces/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/workspaces/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func runActionFromPath(path string) (runID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/runs/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/runs/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func decisionActionFromPath(path string) (ticketID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/decisions/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/decisions/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func approvalActionFromPath(path string) (approvalID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/approvals/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/approvals/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func mcpServerActionFromPath(path string) (serverID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/mcp/servers/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/mcp/servers/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func durationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return 0, err
	}
	serverConfig, err := ConfigFromWorkorch(cfg)
	if err != nil {
		return 0, err
	}
	switch name {
	case configServerReadHeaderTimeout:
		return serverConfig.ReadHeaderTimeout, nil
	case configServerReadTimeout:
		return serverConfig.ReadTimeout, nil
	case configServerWriteTimeout:
		return serverConfig.WriteTimeout, nil
	case configServerIdleTimeout:
		return serverConfig.IdleTimeout, nil
	case configServerShutdownTimeout:
		return serverConfig.ShutdownTimeout, nil
	default:
		return fallback, nil
	}
}

func durationFromValues(values map[string]string, name string, fallback time.Duration) (time.Duration, error) {
	return durationFromValue(name, values[name], fallback)
}

func durationFromValue(name string, value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", name, err)
	}
	return duration, nil
}

func int64FromEnv(name string, fallback int64) (int64, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return 0, err
	}
	if name == configServerMaxConcurrentRuns {
		serverConfig, err := ConfigFromWorkorch(cfg)
		if err != nil {
			return 0, err
		}
		return serverConfig.MaxConcurrentRuns, nil
	}
	return fallback, nil
}

func int64FromValues(values map[string]string, name string, fallback int64) (int64, error) {
	value := values[name]
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", name, err)
	}
	if number <= 0 {
		return 0, fmt.Errorf("invalid integer for %s: must be positive", name)
	}
	return number, nil
}

func firstNonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
}

func writeBusinessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, common.ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, common.ErrBlocked):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, common.ErrConflict):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, common.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
	}
}

func writeArtifactContent(w http.ResponseWriter, item appservice.ArtifactContent) {
	contentType := strings.TrimSpace(item.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	fileName := strings.TrimSpace(item.FileName)
	if fileName == "" {
		fileName = "artifact"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
	w.Header().Set("X-Workorch-Artifact-URI", item.URI)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(item.Content)
}

func writeRuntimeRawContent(w http.ResponseWriter, item appservice.RuntimeRawContent) {
	contentType := strings.TrimSpace(item.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	fileName := strings.TrimSpace(item.FileName)
	if fileName == "" {
		fileName = "payload"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
	w.Header().Set("X-Workorch-Raw-Ref", item.Ref)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(item.Content)
}

func writeRunQueueError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusRequestTimeout, errorResponse{
		Error: fmt.Sprintf("run execution request canceled while waiting for available execution slot: %v", err),
	})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func decodeOptionalJSON(r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}
