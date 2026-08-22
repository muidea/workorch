// Package execution coordinates application-level workspace execution
// admission. It owns request normalization and projection side effects only;
// the Workspace/Run owners remain the sole authorities for lifecycle
// transitions.
package execution

import (
	"context"
	"fmt"
	"strings"
	"time"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Submitter is the narrow typed ingress to the Workspace owner. The use case
// never receives a repository, EventHub, or owner service facade.
type Submitter interface {
	SubmitWorkspaceRun(context.Context, workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error)
}

// Projection is a derived application observation port. Implementations must
// not issue owner commands or mutate Runtime authority.
type Projection interface {
	RecordWorkspaceExecutionOutcome(string, workorchdevents.WorkspaceRunRequestedCommand, workorchdevents.WorkspaceRunRequestedResult)
	RecordWorkspaceExecutionRejected(string, workorchdevents.WorkspaceRunRequestedCommand, error)
}

// UseCase is the single synchronous execution-admission coordinator used by
// every application ingress. It deliberately has no queue, retry, dedupe or
// successor policy: acceptance is returned only after the Workspace owner has
// synchronously admitted the typed intent.
type UseCase struct {
	submitter  Submitter
	projection Projection
	clock      func() time.Time
}

// New constructs an execution coordinator with the process clock.
func New(submitter Submitter, projection Projection) *UseCase {
	return NewWithClock(submitter, projection, func() time.Time { return time.Now().UTC() })
}

// NewWithClock is equivalent to New but permits deterministic request identity
// tests. The clock is application metadata only and is never sent as owner
// authority.
func NewWithClock(submitter Submitter, projection Projection, clock func() time.Time) *UseCase {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &UseCase{
		submitter:  submitter,
		projection: projection,
		clock:      clock,
	}
}

// AdmitWorkspaceExecution returns the normalized command only after the
// Workspace owner has admitted it. HTTP may still encode the successful
// response as 202, but 202 never represents an application-local queue entry.
func (u *UseCase) AdmitWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedCommand, error) {
	command, _, err := u.submitWorkspaceExecution(ctx, topic, command)
	return command, err
}

// SubmitWorkspaceExecution is the typed EventHub counterpart. HTTP and
// EventHub ingress share this same synchronous owner admission path.
func (u *UseCase) SubmitWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	_, outcome, err := u.submitWorkspaceExecution(ctx, topic, command)
	return outcome, err
}

func (u *UseCase) submitWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedCommand, workorchdevents.WorkspaceRunRequestedResult, error) {
	if u == nil || ctx == nil || u.submitter == nil || u.projection == nil {
		return workorchdevents.WorkspaceRunRequestedCommand{}, workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return workorchdevents.WorkspaceRunRequestedCommand{}, workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	command, err := u.normalize(topic, command)
	if err != nil {
		return workorchdevents.WorkspaceRunRequestedCommand{}, workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	ownerCtx, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		u.projection.RecordWorkspaceExecutionRejected(topic, command, err)
		return command, workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	outcome, err := u.submitter.SubmitWorkspaceRun(ownerCtx, command)
	if err != nil {
		u.projection.RecordWorkspaceExecutionRejected(topic, command, err)
		return command, workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	if err := validateOwnerOutcome(command, outcome); err != nil {
		u.projection.RecordWorkspaceExecutionRejected(topic, command, err)
		return command, workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	u.projection.RecordWorkspaceExecutionOutcome(topic, command, outcome)
	return command, outcome, nil
}

func (u *UseCase) normalize(topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedCommand, error) {
	command = server.NormalizeWorkspaceExecutionCommandAt(topic, command, u.clock())
	command.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(command.WorkspaceID)))
	command.Intent = strings.TrimSpace(command.Intent)
	if command.WorkspaceID == "" {
		return workorchdevents.WorkspaceRunRequestedCommand{}, common.ErrInvalidArgument
	}
	if command.Intent != "resume" && command.Intent != "restart" {
		return workorchdevents.WorkspaceRunRequestedCommand{}, fmt.Errorf("%w: workspace execution command supports resume or restart intent", common.ErrInvalidArgument)
	}
	if strings.TrimSpace(command.RequestID) == "" {
		return workorchdevents.WorkspaceRunRequestedCommand{}, common.ErrInvalidArgument
	}
	return command, nil
}

func validateOwnerOutcome(command workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) error {
	if err := outcome.Validate(); err != nil {
		return fmt.Errorf("%w: invalid workspace owner admission: %v", common.ErrConflict, err)
	}
	if outcome.WorkspaceID != command.WorkspaceID || strings.TrimSpace(outcome.RequestID) != strings.TrimSpace(command.RequestID) || string(outcome.Kind) != command.Intent {
		return fmt.Errorf("%w: workspace owner admission identity mismatch", common.ErrConflict)
	}
	return nil
}
