package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
	"time"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
const CommandDestination = modulecommon.ModuleID

// ServiceStoppedData is a bounded lifecycle notification. Process-local wait
// handles never cross the EventHub boundary.
type ServiceStoppedData struct {
	ServiceName  string
	State        string
	ErrorCode    string
	ErrorMessage string
	StoppedAt    time.Time
}

type WorkspaceRunRequestedCommand struct {
	WorkspaceID    common.WorkspaceID
	Intent         string
	Reason         string
	RequestID      string
	IdempotencyKey string
	Source         string
	Priority       int
}

type WorkspaceRunRequestedResult struct {
	Accepted bool
}

type WorkspaceProjectionChangedData struct {
	WorkspaceID             common.WorkspaceID
	RunID                   common.RunID
	StreamGeneration        string
	GlobalRevision          int64
	WorkspaceRevision       int64
	SourceEventRevision     int64
	ProjectedSourceRevision int64
	RuntimeStateRevision    uint64
	EvidenceSequence        uint64
	ActivityRevision        int64
	WorkspaceTotal          int
	ChangeKind              string
	ChangedSections         []workorchport.WorkspaceProjectionSection
	ProjectionState         string
	UpdatedAt               string
	RebuiltAt               string
	StaleReason             string
	Error                   string
}

// ReadWorkspaceRuntimeViewCommand is the typed EventHub read for the TUI
// runtime main view. Local backends require a concrete response.
type ReadWorkspaceRuntimeViewCommand struct {
	WorkspaceID   common.WorkspaceID
	ActivityLimit int
	BeforeCursor  string
}

type ReadWorkspaceRuntimeViewResult struct {
	View workorchport.WorkspaceRuntimeViewInfo
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
		eventcontract.DataTopic[ServiceStoppedData]("workorchd.service", definitionPackage, eventcontract.LanePolicyTopic, TopicWorkorchDServiceStopped),
		eventcontract.CommandTopic[WorkspaceRunRequestedCommand, WorkspaceRunRequestedResult]("workorchd.scheduler", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkorchDCommandWorkspaceRunRequested),
		eventcontract.CommandTopic[ReadWorkspaceRuntimeViewCommand, ReadWorkspaceRuntimeViewResult]("workorchd.workspace_runtime_view", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkorchDCommandReadWorkspaceRuntimeView),
		eventcontract.DataTopic[WorkspaceProjectionChangedData]("workorchd.workspace_projection", definitionPackage, eventcontract.LanePolicyState, TopicWorkspaceProjectionChanged),
		eventcontract.DataTopic[CLIProgress]("cli", definitionPackage, eventcontract.LanePolicyTopic, TopicCLIProgress),
	)
	return append(ret, CLIMessageContracts()...)
}
