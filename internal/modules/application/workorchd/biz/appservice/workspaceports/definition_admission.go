package workspaceports

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Definition admission is part of the application Runtime authority path,
// matching execution-intent and clarification ingress. It must not use the
// generic CRUD/read-port source identity.
const definitionAdmissionSourceModule = "application/workorchd"

// SubmitDefinitionAdmissionIntent submits immutable authoring intent and, at
// most, one immutable Decision-owner fact. It never sends a requested
// Workspace activation state or reseals caller-owned candidate bytes.
func SubmitDefinitionAdmissionIntent(ctx context.Context, hub magicevent.Hub, command workspaceevents.DefinitionAdmissionIntentCommand) (workspaceevents.DefinitionAdmissionIntentResult, error) {
	command = command.Normalized()
	if hub == nil {
		return workspaceevents.DefinitionAdmissionIntentResult{}, common.ErrInvalidArgument
	}
	if err := command.Validate(); err != nil {
		return workspaceevents.DefinitionAdmissionIntentResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionAdmissionIntent, definitionAdmissionSourceModule, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ownerevent.BindLane(event, workspaceevents.Contracts(), event.ID(), "workspace", string(command.Candidate.WorkspaceID), command.Candidate.WorkspaceID, "", "")
	value, err := ownerevent.WorkspaceOwner[workspaceevents.DefinitionAdmissionIntentResult](hub.Send(event), event.ID())
	if err != nil {
		return workspaceevents.DefinitionAdmissionIntentResult{}, err
	}
	value = value.Normalized()
	if err := value.ValidateFor(command.Candidate); err != nil {
		return workspaceevents.DefinitionAdmissionIntentResult{}, fmt.Errorf("%w: Workspace owner returned invalid definition admission outcome: %v", common.ErrConflict, err)
	}
	return value, nil
}
