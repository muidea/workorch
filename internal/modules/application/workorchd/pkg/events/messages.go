package events

import (
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
const CommandDestination = modulecommon.ModuleID

type DoneCommand struct{}
type DoneCommandResult struct {
	Done <-chan error
}

type WorkspaceRunRequestedCommand struct {
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	Intent                string
	Action                string
	Reason                string
	RequestID             string
	IdempotencyKey        string
	Source                string
	DefinitionRevision    string
	DecisionID            string
	Priority              int
	GoalID                common.GoalID
	DoDID                 common.DoDID
	LLMProviderID         string
	SoulID                common.SoulID
	SoulBindings          map[string]string
	SkillIDs              []common.SkillID
	MCPServerIDs          []common.MCPServerID
	CapabilityIDs         []common.MCPCapabilityID
	MemoryIDs             []common.MemoryID
	DialogLanguage        string
	ExecutionContextPatch map[string]any
}

type WorkspaceRunRequestedResult struct {
	Accepted bool
}

type WorkspaceProjectionChangedData struct {
	WorkspaceID             common.WorkspaceID
	RunID                   common.RunID
	GlobalRevision          int64
	WorkspaceRevision       int64
	SourceEventRevision     int64
	ProjectedSourceRevision int64
	WorkspaceTotal          int
	ChangeKind              string
	ProjectionState         string
	UpdatedAt               string
	RebuiltAt               string
	StaleReason             string
	Error                   string
}

type CLICommand struct {
	CommandID     string
	CorrelationID string
	Actor         string
	IssuedAt      time.Time
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	SubjectType   string
	SubjectID     string
	Command       string

	CreateWorkspace *workorchport.CreateWorkspaceRequest
	Purge           bool
	Text            string
	Items           []string
	DecisionIDs     []common.TicketID
	ResolutionType  string
	GovernanceData  CLIData
	ApprovalIDs     []common.ApprovalID
	ApprovalAction  string
	Soul            *workorchport.SoulInfo
	Skill           *workorchport.SkillInfo
	MCPServer       *workorchport.MCPServerInfo
	Memory          *workorchport.MemoryItemInfo
	MemorySearch    *workorchport.MemorySearchRequest
	Template        *workorchport.TemplateInfo
}

type CLIData struct {
	Fields []CLIDataField
}

type CLIDataField struct {
	Name  string
	Value CLIDataValue
}

type CLIDataValue struct {
	Kind   string
	String string
	Number float64
	Bool   bool
	Object CLIData
	Array  []CLIDataValue
}

type CLIResult struct {
	CorrelationID string
	CommandID     string
	OK            bool
	Error         string
	OccurredAt    time.Time
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID

	Workspace         *workorchport.WorkspaceInfo
	Workspaces        []workorchport.WorkspaceInfo
	WorkspaceSnapshot *workorchport.WorkspaceSnapshotInfo
	Goal              *workorchport.GoalInfo
	Goals             []workorchport.GoalInfo
	DoD               *workorchport.DoDInfo
	DoDs              []workorchport.DoDInfo
	Run               *workorchport.RunInfo
	Runs              []workorchport.RunInfo
	RunStatus         *workorchport.RunStatusInfo
	Decision          *workorchport.DecisionInfo
	Decisions         []workorchport.DecisionInfo
	Approval          *workorchport.ApprovalInfo
	Approvals         []workorchport.ApprovalInfo
	Soul              *workorchport.SoulInfo
	Souls             []workorchport.SoulInfo
	Skill             *workorchport.SkillInfo
	Skills            []workorchport.SkillInfo
	MCPServer         *workorchport.MCPServerInfo
	MCPServers        []workorchport.MCPServerInfo
	MCPCapabilities   []workorchport.MCPCapabilityInfo
	Memory            *workorchport.MemoryItemInfo
	Memories          []workorchport.MemoryItemInfo
	Template          *workorchport.TemplateInfo
	Templates         []workorchport.TemplateInfo
	Configuration     *workorchport.ServiceConfig
}

type CLIProgress struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	SubjectType string
	SubjectID   string
	State       string
	Summary     string
}

func MessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.FacadeTopic[DoneCommand, DoneCommandResult]("workorchd", definitionPackage, TopicWorkorchDCommandDone),
		eventcontract.CommandTopic[WorkspaceRunRequestedCommand, WorkspaceRunRequestedResult]("workorchd.scheduler", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkorchDCommandWorkspaceRunRequested),
		eventcontract.DataTopic[WorkspaceProjectionChangedData]("workorchd.workspace_projection", definitionPackage, eventcontract.LanePolicyState, TopicWorkspaceProjectionChanged),
		eventcontract.FacadeTopic[CLICommand, CLIResult]("cli", definitionPackage, TopicCLICommand),
		eventcontract.DataTopic[CLIProgress]("cli", definitionPackage, eventcontract.LanePolicyTopic, TopicCLIProgress),
	)
}
