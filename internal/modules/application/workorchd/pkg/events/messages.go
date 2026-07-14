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
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	Intent             string
	Action             string
	Reason             string
	RequestID          string
	IdempotencyKey     string
	Source             string
	DefinitionRevision string
	DecisionID         string
	Priority           int
	GoalID             common.GoalID
	DoDID              common.DoDID
	LLMProviderID      string
	SoulID             common.SoulID
	SoulBindings       map[string]string
	SkillIDs           []common.SkillID
	MCPServerIDs       []common.MCPServerID
	CapabilityIDs      []common.MCPCapabilityID
	MemoryIDs          []common.MemoryID
	DialogLanguage     string
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
	ret := eventcontract.MessageContracts(
		eventcontract.FacadeTopic[DoneCommand, DoneCommandResult]("workorchd", definitionPackage, TopicWorkorchDCommandDone),
		eventcontract.CommandTopic[WorkspaceRunRequestedCommand, WorkspaceRunRequestedResult]("workorchd.scheduler", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkorchDCommandWorkspaceRunRequested),
		eventcontract.DataTopic[WorkspaceProjectionChangedData]("workorchd.workspace_projection", definitionPackage, eventcontract.LanePolicyState, TopicWorkspaceProjectionChanged),
		eventcontract.DataTopic[CLIProgress]("cli", definitionPackage, eventcontract.LanePolicyTopic, TopicCLIProgress),
	)
	return append(ret, CLIMessageContracts()...)
}
