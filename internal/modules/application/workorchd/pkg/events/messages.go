package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
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
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}

type CLICommand struct {
	CommandID     string
	CorrelationID string
	Actor         string
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	SubjectType   string
	SubjectID     string
	Command       string
}

type CLIResult struct {
	CorrelationID string
	CommandID     string
	OK            bool
	Error         string
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
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
		eventcontract.NewTopic[eventcontract.NoCommand, CLIResult, eventcontract.NoResult]("cli", definitionPackage, eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIResult),
		eventcontract.NewTopic[eventcontract.NoCommand, CLIProgress, eventcontract.NoResult]("cli", definitionPackage, eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIProgress),
	)
}
