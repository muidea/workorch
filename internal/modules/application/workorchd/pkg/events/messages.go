package events

import (
	"fmt"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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

// WorkspaceRunRequestedResult is the unmodified admission outcome produced by
// the Workspace owner.  The application scheduler owns request transport only;
// it must never collapse an owner rejection into transport acceptance.
type WorkspaceRunRequestedResult runtimecontract.RuntimeExecutionIntentAdmission

func (r WorkspaceRunRequestedResult) Validate() error {
	if err := (runtimecontract.RuntimeExecutionIntentAdmission(r)).Validate(); err != nil {
		return fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	return nil
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
	ChangedSections         []projectioncontract.WorkspaceProjectionSection
	ProjectionState         string
	UpdatedAt               string
	RebuiltAt               string
	StaleReason             string
	Error                   string
}

// ReadWorkspaceRuntimeViewCommand is the typed EventHub read for the TUI
// runtime main view. Local backends require a concrete response.
type ReadWorkspaceRuntimeViewCommand struct {
	Query workorchquerycontract.WorkspaceRuntimeViewQuery
}

type ReadWorkspaceRuntimeViewResult struct {
	View CLIWorkspaceRuntimeViewInfo
}

func MessageContracts() []eventcontract.MessageContract {
	ret := eventcontract.MessageContracts(
		eventcontract.DataTopic[ServiceStoppedData]("workorchd.service", definitionPackage, eventcontract.LanePolicyTopic, TopicWorkorchDServiceStopped),
		eventcontract.CommandTopic[WorkspaceRunRequestedCommand, WorkspaceRunRequestedResult]("workorchd.scheduler", definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicWorkorchDCommandWorkspaceRunRequested),
		eventcontract.CommandTopic[ReadWorkspaceRuntimeViewCommand, ReadWorkspaceRuntimeViewResult]("workorchd.workspace_runtime_view", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicWorkorchDCommandReadWorkspaceRuntimeView),
		eventcontract.DataTopic[WorkspaceProjectionChangedData]("workorchd.workspace_projection", definitionPackage, eventcontract.LanePolicyState, TopicWorkspaceProjectionChanged),
	)
	return append(ret, CLIMessageContracts()...)
}
