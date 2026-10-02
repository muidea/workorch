package server

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionquality"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/setupdraft"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// HTTP request/response DTOs stay private to the transport package. They are
// intentionally separated from server lifecycle, mutable state and function
// wiring so protocol changes cannot silently alter application ownership.

type statusResponse struct {
	Service    string                               `json:"service"`
	Status     string                               `json:"status"`
	Projection *projectioncontract.ProjectionStatus `json:"projection,omitempty"`
}

type configurationResponse struct {
	Service       string                                             `json:"service"`
	Configuration workorchconfigurationcontract.ServiceConfiguration `json:"configuration"`
}

type llmModelCatalogResponse struct {
	Service string                                            `json:"service"`
	Catalog workorchmanagementcontract.ProviderCatalogSummary `json:"catalog"`
}

type workspaceResponse struct {
	Service   string                       `json:"service"`
	Backend   string                       `json:"backend,omitempty"`
	DataRoot  string                       `json:"dataRoot,omitempty"`
	Workspace projectioncontract.Workspace `json:"workspace"`
}

type workspacesResponse struct {
	Service    string                              `json:"service"`
	Backend    string                              `json:"backend,omitempty"`
	DataRoot   string                              `json:"dataRoot,omitempty"`
	Projection projectioncontract.ProjectionStatus `json:"projection"`
	Workspaces []projectioncontract.Workspace      `json:"workspaces"`
	Error      string                              `json:"error,omitempty"`
}

type goalResponse struct {
	Service  string                          `json:"service"`
	Backend  string                          `json:"backend,omitempty"`
	DataRoot string                          `json:"dataRoot,omitempty"`
	Goal     workorchmanagementcontract.Goal `json:"goal"`
}

type goalsResponse struct {
	Service  string                            `json:"service"`
	Backend  string                            `json:"backend,omitempty"`
	DataRoot string                            `json:"dataRoot,omitempty"`
	Goals    []workorchmanagementcontract.Goal `json:"goals"`
}

type dodResponse struct {
	Service  string                         `json:"service"`
	Backend  string                         `json:"backend,omitempty"`
	DataRoot string                         `json:"dataRoot,omitempty"`
	DoD      workorchmanagementcontract.DoD `json:"dod"`
}

type dodsResponse struct {
	Service  string                           `json:"service"`
	Backend  string                           `json:"backend,omitempty"`
	DataRoot string                           `json:"dataRoot,omitempty"`
	DoDs     []workorchmanagementcontract.DoD `json:"dods"`
}

type goalDoDQualityCheckResponse struct {
	Service      string                  `json:"service"`
	Backend      string                  `json:"backend,omitempty"`
	DataRoot     string                  `json:"dataRoot,omitempty"`
	QualityCheck definitionquality.Check `json:"qualityCheck"`
}

type workspaceSetupDraftSessionResponse struct {
	Service      string                     `json:"service"`
	Backend      string                     `json:"backend,omitempty"`
	DataRoot     string                     `json:"dataRoot,omitempty"`
	DraftSession workspaceSetupDraftSession `json:"draftSession"`
}

type runInstanceResponse struct {
	Service  string                 `json:"service"`
	Backend  string                 `json:"backend,omitempty"`
	DataRoot string                 `json:"dataRoot,omitempty"`
	Run      projectioncontract.Run `json:"run"`
}

type runSnapshotResponse struct {
	Service  string                         `json:"service"`
	Backend  string                         `json:"backend,omitempty"`
	DataRoot string                         `json:"dataRoot,omitempty"`
	Snapshot projectioncontract.RunSnapshot `json:"snapshot"`
}

type runStepsResponse struct {
	Service  string                        `json:"service"`
	Backend  string                        `json:"backend,omitempty"`
	DataRoot string                        `json:"dataRoot,omitempty"`
	Steps    []projectioncontract.StepView `json:"steps,omitempty"`
	Page     readmodelcore.StepPage        `json:"page"`
}

type runToolCallsResponse struct {
	Service   string                        `json:"service"`
	Backend   string                        `json:"backend,omitempty"`
	DataRoot  string                        `json:"dataRoot,omitempty"`
	ToolCalls []projectioncontract.ToolCall `json:"toolCalls,omitempty"`
	Page      readmodelcore.ToolCallPage    `json:"page"`
}

type runRuntimeCheckpointsResponse struct {
	Service            string                                 `json:"service"`
	Backend            string                                 `json:"backend,omitempty"`
	DataRoot           string                                 `json:"dataRoot,omitempty"`
	RuntimeCheckpoints []projectioncontract.RuntimeCheckpoint `json:"runtimeCheckpoints,omitempty"`
	Page               readmodelcore.RuntimeCheckpointPage    `json:"page"`
}

type runResultsResponse struct {
	Service  string                      `json:"service"`
	Backend  string                      `json:"backend,omitempty"`
	DataRoot string                      `json:"dataRoot,omitempty"`
	Results  []projectioncontract.Result `json:"results,omitempty"`
	Page     readmodelcore.ResultPage    `json:"page"`
}

type runPatchesResponse struct {
	Service  string                     `json:"service"`
	Backend  string                     `json:"backend,omitempty"`
	DataRoot string                     `json:"dataRoot,omitempty"`
	Patches  []projectioncontract.Patch `json:"patches,omitempty"`
	Page     readmodelcore.PatchPage    `json:"page"`
}

type runValidationResultsResponse struct {
	Service           string                                `json:"service"`
	Backend           string                                `json:"backend,omitempty"`
	DataRoot          string                                `json:"dataRoot,omitempty"`
	ValidationResults []projectioncontract.ValidationResult `json:"validationResults,omitempty"`
	Page              readmodelcore.ValidationResultPage    `json:"page"`
}

type runArtifactsResponse struct {
	Service   string                        `json:"service"`
	Backend   string                        `json:"backend,omitempty"`
	DataRoot  string                        `json:"dataRoot,omitempty"`
	Artifacts []projectioncontract.Artifact `json:"artifacts,omitempty"`
	Page      readmodelcore.ArtifactPage    `json:"page"`
}

type runStatusResponse struct {
	Service       string                       `json:"service"`
	Backend       string                       `json:"backend,omitempty"`
	DataRoot      string                       `json:"dataRoot,omitempty"`
	Status        projectioncontract.RunStatus `json:"status"`
	WaitMatched   bool                         `json:"waitMatched,omitempty"`
	WaitElapsedMs int64                        `json:"waitElapsedMs,omitempty"`
}

type runObservabilityResponse struct {
	Service       string                              `json:"service"`
	Backend       string                              `json:"backend,omitempty"`
	DataRoot      string                              `json:"dataRoot,omitempty"`
	Observability projectioncontract.RunObservability `json:"observability"`
}

type taskProgressResponse struct {
	Service      string                          `json:"service"`
	Backend      string                          `json:"backend,omitempty"`
	DataRoot     string                          `json:"dataRoot,omitempty"`
	TaskProgress projectioncontract.TaskProgress `json:"taskProgress"`
}

type contextBundleResponse struct {
	Service       string                           `json:"service"`
	Backend       string                           `json:"backend,omitempty"`
	DataRoot      string                           `json:"dataRoot,omitempty"`
	ContextBundle projectioncontract.ContextBundle `json:"contextBundle"`
}

type taskDecompositionsResponse struct {
	Service            string                                 `json:"service"`
	Backend            string                                 `json:"backend,omitempty"`
	DataRoot           string                                 `json:"dataRoot,omitempty"`
	TaskDecompositions []projectioncontract.TaskDecomposition `json:"taskDecompositions"`
}

type eventRecordsResponse struct {
	Service      string                           `json:"service"`
	Backend      string                           `json:"backend,omitempty"`
	DataRoot     string                           `json:"dataRoot,omitempty"`
	EventRecords []projectioncontract.EventRecord `json:"eventRecords"`
}

type runsResponse struct {
	Service  string                   `json:"service"`
	Backend  string                   `json:"backend,omitempty"`
	DataRoot string                   `json:"dataRoot,omitempty"`
	Runs     []projectioncontract.Run `json:"runs"`
}

type workspaceSnapshotResponse struct {
	Service    string                               `json:"service"`
	Backend    string                               `json:"backend,omitempty"`
	DataRoot   string                               `json:"dataRoot,omitempty"`
	Projection projectioncontract.ProjectionStatus  `json:"projection"`
	Snapshot   projectioncontract.WorkspaceSnapshot `json:"snapshot"`
	Error      string                               `json:"error,omitempty"`
}

type workspaceRuntimeViewResponse struct {
	Service     string                                  `json:"service"`
	Backend     string                                  `json:"backend,omitempty"`
	DataRoot    string                                  `json:"dataRoot,omitempty"`
	Projection  projectioncontract.ProjectionStatus     `json:"projection"`
	RuntimeView projectioncontract.WorkspaceRuntimeView `json:"runtimeView"`
	Error       string                                  `json:"error,omitempty"`
}

type workspaceExecutionActivationResponse struct {
	Service    string                                          `json:"service"`
	Backend    string                                          `json:"backend,omitempty"`
	DataRoot   string                                          `json:"dataRoot,omitempty"`
	Activation projectioncontract.WorkspaceExecutionActivation `json:"activation"`
}

type workspaceSoulBindingsResponse struct {
	Service      string                                           `json:"service"`
	Backend      string                                           `json:"backend,omitempty"`
	DataRoot     string                                           `json:"dataRoot,omitempty"`
	SoulBindings workorchmanagementcontract.WorkspaceSoulBindings `json:"soulBindings"`
}

type decisionResponse struct {
	Service  string                      `json:"service"`
	Backend  string                      `json:"backend,omitempty"`
	DataRoot string                      `json:"dataRoot,omitempty"`
	Decision projectioncontract.Decision `json:"decision"`
}

type decisionsResponse struct {
	Service   string                        `json:"service"`
	Backend   string                        `json:"backend,omitempty"`
	DataRoot  string                        `json:"dataRoot,omitempty"`
	Decisions []projectioncontract.Decision `json:"decisions"`
	Page      appmodelcore.DecisionPage     `json:"page"`
}

type approvalResponse struct {
	Service  string                      `json:"service"`
	Backend  string                      `json:"backend,omitempty"`
	DataRoot string                      `json:"dataRoot,omitempty"`
	Approval projectioncontract.Approval `json:"approval"`
}

type approvalsResponse struct {
	Service   string                        `json:"service"`
	Backend   string                        `json:"backend,omitempty"`
	DataRoot  string                        `json:"dataRoot,omitempty"`
	Approvals []projectioncontract.Approval `json:"approvals"`
	Page      appmodelcore.ApprovalPage     `json:"page"`
}

type soulResponse struct {
	Service  string                          `json:"service"`
	Backend  string                          `json:"backend,omitempty"`
	DataRoot string                          `json:"dataRoot,omitempty"`
	Soul     workorchmanagementcontract.Soul `json:"soul"`
}

type soulsResponse struct {
	Service  string                            `json:"service"`
	Backend  string                            `json:"backend,omitempty"`
	DataRoot string                            `json:"dataRoot,omitempty"`
	Souls    []workorchmanagementcontract.Soul `json:"souls"`
}

type workspaceTemplateResponse struct {
	Service           string                                       `json:"service"`
	Backend           string                                       `json:"backend,omitempty"`
	DataRoot          string                                       `json:"dataRoot,omitempty"`
	WorkspaceTemplate workorchmanagementcontract.WorkspaceTemplate `json:"workspaceTemplate"`
}

type workspaceTemplatesResponse struct {
	Service            string                                         `json:"service"`
	Backend            string                                         `json:"backend,omitempty"`
	DataRoot           string                                         `json:"dataRoot,omitempty"`
	WorkspaceTemplates []workorchmanagementcontract.WorkspaceTemplate `json:"workspaceTemplates"`
}

type skillResponse struct {
	Service  string                           `json:"service"`
	Backend  string                           `json:"backend,omitempty"`
	DataRoot string                           `json:"dataRoot,omitempty"`
	Skill    workorchmanagementcontract.Skill `json:"skill"`
}

type skillsResponse struct {
	Service  string                             `json:"service"`
	Backend  string                             `json:"backend,omitempty"`
	DataRoot string                             `json:"dataRoot,omitempty"`
	Skills   []workorchmanagementcontract.Skill `json:"skills"`
}

type mcpServerResponse struct {
	Service  string                               `json:"service"`
	Backend  string                               `json:"backend,omitempty"`
	DataRoot string                               `json:"dataRoot,omitempty"`
	Server   workorchmanagementcontract.MCPServer `json:"server"`
}

type mcpServersResponse struct {
	Service  string                                 `json:"service"`
	Backend  string                                 `json:"backend,omitempty"`
	DataRoot string                                 `json:"dataRoot,omitempty"`
	Servers  []workorchmanagementcontract.MCPServer `json:"servers"`
}

type mcpToolsResponse struct {
	Service  string                               `json:"service"`
	Backend  string                               `json:"backend,omitempty"`
	DataRoot string                               `json:"dataRoot,omitempty"`
	Tools    []workorchmanagementcontract.MCPTool `json:"tools"`
}

type mcpResourcesResponse struct {
	Service   string                                   `json:"service"`
	Backend   string                                   `json:"backend,omitempty"`
	DataRoot  string                                   `json:"dataRoot,omitempty"`
	Resources []workorchmanagementcontract.MCPResource `json:"resources"`
}

type mcpPromptsResponse struct {
	Service  string                                 `json:"service"`
	Backend  string                                 `json:"backend,omitempty"`
	DataRoot string                                 `json:"dataRoot,omitempty"`
	Prompts  []workorchmanagementcontract.MCPPrompt `json:"prompts"`
}

type mcpCapabilityResponse struct {
	Service    string                                   `json:"service"`
	Backend    string                                   `json:"backend,omitempty"`
	DataRoot   string                                   `json:"dataRoot,omitempty"`
	Capability workorchmanagementcontract.MCPCapability `json:"capability"`
}

type mcpCapabilitiesResponse struct {
	Service      string                                     `json:"service"`
	Backend      string                                     `json:"backend,omitempty"`
	DataRoot     string                                     `json:"dataRoot,omitempty"`
	Capabilities []workorchmanagementcontract.MCPCapability `json:"capabilities"`
}

type memoryItemResponse struct {
	Service  string                                `json:"service"`
	Backend  string                                `json:"backend,omitempty"`
	DataRoot string                                `json:"dataRoot,omitempty"`
	Memory   workorchmanagementcontract.MemoryItem `json:"memory"`
}

type memoryItemsResponse struct {
	Service  string                                  `json:"service"`
	Backend  string                                  `json:"backend,omitempty"`
	DataRoot string                                  `json:"dataRoot,omitempty"`
	Memory   []workorchmanagementcontract.MemoryItem `json:"memory"`
}

type memoryRecallTracesResponse struct {
	Service            string                                 `json:"service"`
	Backend            string                                 `json:"backend,omitempty"`
	DataRoot           string                                 `json:"dataRoot,omitempty"`
	MemoryRecallTraces []projectioncontract.MemoryRecallTrace `json:"memoryRecallTraces"`
}

type resolveApprovalRequest struct {
	Payload workorcheventpayload.Payload `json:"payload,omitempty"`
}

type createWorkspaceSetupDraftSessionRequest struct {
	TemplateID           string `json:"templateId,omitempty"`
	Title                string `json:"title,omitempty"`
	Description          string `json:"description,omitempty"`
	DeliverableDirectory string `json:"deliverableDirectory,omitempty"`
	DialogLanguage       string `json:"dialogLanguage,omitempty"`
}

type appendWorkspaceSetupDraftMessageRequest struct {
	TemplateID           string           `json:"templateId,omitempty"`
	Title                string           `json:"title,omitempty"`
	Description          string           `json:"description,omitempty"`
	DeliverableDirectory string           `json:"deliverableDirectory,omitempty"`
	Content              string           `json:"content"`
	Draft                setupdraft.Draft `json:"draft,omitempty"`
	DialogLanguage       string           `json:"dialogLanguage,omitempty"`
}

type errorResponse struct {
	Error                   string   `json:"error"`
	Code                    string   `json:"code,omitempty"`
	TicketID                string   `json:"ticketId,omitempty"`
	RequestedResolutionType string   `json:"requestedResolutionType,omitempty"`
	AllowedActions          []string `json:"allowedActions,omitempty"`
	Remediation             string   `json:"remediation,omitempty"`
}
