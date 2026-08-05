package events

import (
	"fmt"
	"strings"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
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
type WorkspaceRunRequestedResult struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	RequestID   string
	Kind        string
	Outcome     runtimecontract.RuntimeExecutionIntentOutcome
	Reason      runtimecontract.RuntimeExecutionIntentReason
}

func (r WorkspaceRunRequestedResult) Validate() error {
	if r.WorkspaceID == "" || strings.TrimSpace(r.RequestID) == "" || (r.Kind != "resume" && r.Kind != "restart") {
		return fmt.Errorf("%w: workspace run result requires workspace, request and intent kind", common.ErrInvalidArgument)
	}
	if err := runtimecontract.ValidateRuntimeExecutionIntentAdmission(r.Outcome, r.Reason); err != nil {
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
