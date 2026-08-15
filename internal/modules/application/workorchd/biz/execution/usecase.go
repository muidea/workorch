// Package execution coordinates application-level workspace execution
// admission. It owns request transport and projection side effects only; the
// Workspace/Run owners remain the sole authorities for lifecycle transitions.
package execution

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/muidea/magicCommon/task"
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
	RecordWorkspaceExecutionQueued(workorchdevents.WorkspaceRunRequestedCommand)
	RecordWorkspaceExecutionOutcome(string, workorchdevents.WorkspaceRunRequestedCommand, workorchdevents.WorkspaceRunRequestedResult)
	RecordWorkspaceExecutionRejected(string, workorchdevents.WorkspaceRunRequestedCommand, error)
	RecordWorkspaceExecutionEnqueueFailure(string, workorchdevents.WorkspaceRunRequestedCommand, error)
}

// UseCase is the single asynchronous execution-admission coordinator used by
// the HTTP application port. It deliberately has no retry or successor
// policy: a failed owner admission is projected as a fact and the caller gets
// a bounded error.
type UseCase struct {
	submitter  Submitter
	projection Projection
	background task.BackgroundRoutine
	clock      func() time.Time

	processCtx context.Context
	cancel     context.CancelFunc

	mu      sync.RWMutex
	closed  bool
	pending sync.WaitGroup
	recent  map[string]requestRecord
	order   []string
}

const recentRequestLimit = 1024

type requestRecord struct {
	command workorchdevents.WorkspaceRunRequestedCommand
	active  bool
}

// New constructs an execution coordinator with the process clock.
func New(submitter Submitter, projection Projection, background task.BackgroundRoutine) *UseCase {
	return NewWithClock(submitter, projection, background, func() time.Time { return time.Now().UTC() })
}

// NewWithClock is equivalent to New but permits deterministic request identity
// tests. The clock is application metadata only and is never sent as owner
// authority.
func NewWithClock(submitter Submitter, projection Projection, background task.BackgroundRoutine, clock func() time.Time) *UseCase {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &UseCase{
		submitter:  submitter,
		projection: projection,
		background: background,
		clock:      clock,
		processCtx: ctx,
		cancel:     cancel,
		recent:     make(map[string]requestRecord),
	}
}

// EnqueueWorkspaceExecution accepts one resume/restart intent and schedules
// exactly one owner submission. The request context is intentionally not
// captured by the asynchronous callback; only a fresh process context with an
// explicit workspace scope is used.
func (u *UseCase) EnqueueWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedCommand, error) {
	if u == nil || ctx == nil || u.submitter == nil || u.projection == nil || u.background == nil {
		return workorchdevents.WorkspaceRunRequestedCommand{}, common.ErrInvalidArgument
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return workorchdevents.WorkspaceRunRequestedCommand{}, common.ErrInvalidArgument
	}

	command, err := u.normalize(topic, command)
	if err != nil {
		return workorchdevents.WorkspaceRunRequestedCommand{}, err
	}

	processCtx, duplicate, requestKey, err := u.reserveAsync(command)
	if err != nil {
		return workorchdevents.WorkspaceRunRequestedCommand{}, err
	}
	if duplicate {
		return command, nil
	}

	// Queued is a transport/projection fact, not owner admission.
	u.projection.RecordWorkspaceExecutionQueued(command)
	if err := u.background.AsyncFunction(func() {
		defer u.finishAsync(requestKey)
		if u.isClosed() {
			return
		}
		workspaceID := common.WorkspaceID(strings.TrimSpace(string(command.WorkspaceID)))
		ownerCtx := common.ContextWithRuntimeWorkspaceID(processCtx, workspaceID)
		if ownerCtx == nil {
			u.projection.RecordWorkspaceExecutionRejected(topic, command, common.ErrInvalidArgument)
			return
		}
		outcome, submitErr := u.submitter.SubmitWorkspaceRun(ownerCtx, command)
		if submitErr != nil {
			u.projection.RecordWorkspaceExecutionRejected(topic, command, submitErr)
			return
		}
		if err := validateOwnerOutcome(command, outcome); err != nil {
			u.projection.RecordWorkspaceExecutionRejected(topic, command, err)
			return
		}
		u.projection.RecordWorkspaceExecutionOutcome(topic, command, outcome)
	}); err != nil {
		u.dropAsync(requestKey)
		rejection := fmt.Errorf("%w: workspace execution activation could not be queued: %v", common.ErrConflict, err)
		u.projection.RecordWorkspaceExecutionEnqueueFailure(topic, command, rejection)
		return workorchdevents.WorkspaceRunRequestedCommand{}, rejection
	}
	return command, nil
}

// SubmitWorkspaceExecution is the synchronous counterpart used by the typed
// EventHub scheduler command. It shares normalization, scope and owner-result
// validation with the HTTP queue, but preserves the command contract's
// synchronous owner admission result.
func (u *UseCase) SubmitWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if u == nil || ctx == nil || u.submitter == nil || u.projection == nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	command, err := u.normalize(topic, command)
	if err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	if _, err := u.begin(); err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	defer u.pending.Done()
	ownerCtx, err := common.BindRuntimeWorkspaceContext(ctx, command.WorkspaceID)
	if err != nil {
		u.projection.RecordWorkspaceExecutionRejected(topic, command, err)
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	outcome, err := u.submitter.SubmitWorkspaceRun(ownerCtx, command)
	if err != nil {
		u.projection.RecordWorkspaceExecutionRejected(topic, command, err)
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	if err := validateOwnerOutcome(command, outcome); err != nil {
		u.projection.RecordWorkspaceExecutionRejected(topic, command, err)
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	u.projection.RecordWorkspaceExecutionOutcome(topic, command, outcome)
	return outcome, nil
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

func (u *UseCase) begin() (context.Context, error) {
	if u == nil {
		return nil, common.ErrInvalidArgument
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return nil, fmt.Errorf("%w: execution admission is closed", common.ErrConflict)
	}
	u.pending.Add(1)
	return u.processCtx, nil
}

func (u *UseCase) reserveAsync(command workorchdevents.WorkspaceRunRequestedCommand) (context.Context, bool, string, error) {
	if u == nil {
		return nil, false, "", common.ErrInvalidArgument
	}
	key := strings.TrimSpace(string(command.WorkspaceID)) + "\x00" + strings.TrimSpace(command.RequestID)
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return nil, false, "", fmt.Errorf("%w: execution admission is closed", common.ErrConflict)
	}
	if existing, ok := u.recent[key]; ok {
		if existing.command != command {
			return nil, false, "", fmt.Errorf("%w: request identity reused with different workspace execution command", common.ErrConflict)
		}
		return u.processCtx, true, key, nil
	}
	if len(u.recent) >= recentRequestLimit && !u.evictCompletedRequestLocked() {
		return nil, false, "", fmt.Errorf("%w: execution admission identity window is full", common.ErrConflict)
	}
	u.recent[key] = requestRecord{command: command, active: true}
	u.order = append(u.order, key)
	u.pending.Add(1)
	return u.processCtx, false, key, nil
}

func (u *UseCase) finishAsync(key string) {
	if u == nil {
		return
	}
	u.mu.Lock()
	if record, ok := u.recent[key]; ok {
		record.active = false
		u.recent[key] = record
	}
	u.mu.Unlock()
	u.pending.Done()
}

func (u *UseCase) dropAsync(key string) {
	if u == nil {
		return
	}
	u.mu.Lock()
	delete(u.recent, key)
	for idx, candidate := range u.order {
		if candidate == key {
			u.order = append(u.order[:idx], u.order[idx+1:]...)
			break
		}
	}
	u.mu.Unlock()
	u.pending.Done()
}

func (u *UseCase) evictCompletedRequestLocked() bool {
	remaining := len(u.order)
	for idx := 0; idx < remaining && len(u.order) > 0; idx++ {
		key := u.order[0]
		u.order = u.order[1:]
		record, ok := u.recent[key]
		if !ok {
			continue
		}
		if record.active {
			u.order = append(u.order, key)
			continue
		}
		delete(u.recent, key)
		return true
	}
	return false
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

func (u *UseCase) isClosed() bool {
	if u == nil {
		return true
	}
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.closed
}

// Close prevents new admissions, cancels process-scoped callbacks, and waits
// for already accepted callbacks. It does not shut down the shared framework
// BackgroundRoutine; its owner remains the application bootstrap.
func (u *UseCase) Close(ctx context.Context) error {
	if u == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	u.mu.Lock()
	if !u.closed {
		u.closed = true
		if u.cancel != nil {
			u.cancel()
		}
	}
	u.mu.Unlock()

	done := make(chan struct{})
	go func() {
		u.pending.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
