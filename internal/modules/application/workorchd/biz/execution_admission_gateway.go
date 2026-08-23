package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	executionusecase "github.com/muidea/workorch/internal/modules/application/workorchd/biz/execution"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// executionAdmissionGateway is the HTTP-facing ingress into the Workorchd
// execution-admission command lane. It only normalizes application request
// metadata and sends a concrete typed command; the subscribed execution use
// case performs the Workspace-owner admission and projection transaction
// before the lane is released.
//
// Keeping HTTP on the same workspace-scoped Workorchd lane as CLI/TUI avoids
// reverse projection commits from concurrent requests without introducing an
// application queue, retry policy, dedupe authority or lifecycle state.
type executionAdmissionGateway struct {
	owner *Biz
	clock func() time.Time
}

func newExecutionAdmissionGateway(owner *Biz) *executionAdmissionGateway {
	return &executionAdmissionGateway{owner: owner, clock: func() time.Time { return time.Now().UTC() }}
}

func (g *executionAdmissionGateway) AdmitWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if g == nil || g.owner == nil || g.owner.EventHub() == nil || ctx == nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	topic = strings.TrimSpace(topic)
	if topic != workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	now := time.Now().UTC()
	if g.clock != nil {
		now = g.clock()
	}
	command = executionusecase.NormalizeWorkspaceExecutionCommandAt(topic, command, now)
	command.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(command.WorkspaceID)))
	command.Intent = strings.TrimSpace(command.Intent)
	if command.WorkspaceID == "" {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}

	event := magicevent.NewEventWithContext(topic, ModuleID, workorchdevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	event.BindLaneKey(executionAdmissionLaneKey(topic, command.WorkspaceID))
	response := g.owner.SendEvent(event)
	if response == nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, fmt.Errorf("%w: workspace execution admission returned no result", common.ErrConflict)
	}
	if err := response.Error(); err != nil {
		if err.Cause != nil {
			return workorchdevents.WorkspaceRunRequestedResult{}, err.Cause
		}
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	result, ok := data.(workorchdevents.WorkspaceRunRequestedResult)
	if !ok || result.WorkspaceID != command.WorkspaceID || strings.TrimSpace(result.RequestID) != strings.TrimSpace(command.RequestID) || string(result.Kind) != command.Intent {
		return workorchdevents.WorkspaceRunRequestedResult{}, fmt.Errorf("%w: workspace execution admission returned an invalid owner outcome", common.ErrConflict)
	}
	if err := result.Validate(); err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, fmt.Errorf("%w: workspace execution admission returned an invalid owner outcome: %v", common.ErrConflict, err)
	}
	return result, nil
}

func executionAdmissionLaneKey(topic string, workspaceID common.WorkspaceID) string {
	return workorchevents.EventHubLaneKeyFor(workorchdevents.Contracts(), topic, eventcontract.LaneScope{
		WorkspaceID: string(workspaceID),
	})
}
