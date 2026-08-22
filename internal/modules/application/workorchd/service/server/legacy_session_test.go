package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var testRuntimeSessions sync.Map

// The session-backed constructors below are package-local test fixtures only.
// Production server code is compiled from ApplicationPorts and has no
// runtimecore dependency or session propagation path.
func newSessionBackedTestHTTPServer(config Config, session *runtimecore.Session, backgroundRoutine task.BackgroundRoutine) *HTTPServer {
	return newSessionBackedTestHTTPServerWithError(normalizeConfig(config), session, backgroundRoutine, nil)
}

func newSessionBackedTestHTTPServerWithError(config Config, session *runtimecore.Session, backgroundRoutine task.BackgroundRoutine, sessionErr error) *HTTPServer {
	state := newSessionBackedTestServerState(config, session, backgroundRoutine, sessionErr)
	return newHTTPServerWithState(normalizeConfig(config), state)
}

func newSessionBackedTestServerState(config Config, session *runtimecore.Session, backgroundRoutine task.BackgroundRoutine, sessionErr error) *serverState {
	state := newServerStateBase(config, backgroundRoutine, newDefaultApplicationOperations())
	if sessionErr != nil {
		runtimeSessionErr := fmt.Errorf("workorch runtime session unavailable: %w", sessionErr)
		state.validate = func(context.Context, string, string) error { return runtimeSessionErr }
		return state
	}
	if session == nil {
		runtimeSessionErr := errors.New("workorch runtime session is not configured")
		state.validate = func(context.Context, string, string) error { return runtimeSessionErr }
		return state
	}
	state.eventUnsubscribes = append(state.eventUnsubscribes, func() {
		if bound := clearTestRuntimeSession(state); bound != nil {
			_ = bound.Close(context.Background())
		}
	})
	configureSessionBackedTestOperations(state, session)
	if err := installTestWorkspaceProjection(state); err != nil {
		projectionErr := fmt.Errorf("test workspace projection unavailable: %w", err)
		state.validate = func(context.Context, string, string) error { return projectionErr }
	}
	return state
}

func configureSessionBackedTestOperations(s *serverState, session *runtimecore.Session) {
	if s == nil || session == nil {
		return
	}
	bindTestRuntimeSession(s, session)
	s.configureConfigurationOperations(session)
	s.configureWorkspaceOperations(session)
	s.configureDefinitionOperations(session)
	s.configureRunOperations(session)
	s.configureGovernanceOperations(session)
	s.configureRegistryOperations(session)
	s.configureMemoryOperations(session)
	s.updateWorkspaceLLMSelectionPort = func(ctx context.Context, workspaceID string, selection *runtimecontract.ProviderInferenceSelection, expectedRevision uint64) (appmodelcore.Workspace, error) {
		return appservice.UpdateWorkspaceLLMSelectionWithSession(ctx, session, workspaceID, selection, expectedRevision)
	}
	s.workspaceExecutionAdmission = sessionBackedTestExecutionAdmission{state: s, submitter: directWorkspaceRunScheduler{state: s}}
}

func withBorrowedTestSession(ctx context.Context, session *runtimecore.Session, fn func(*runtimecore.Session) error) error {
	if session == nil || fn == nil {
		return common.ErrInvalidArgument
	}
	borrowed := session.Borrow()
	if borrowed == nil {
		return common.ErrInvalidArgument
	}
	err := fn(borrowed)
	return errors.Join(err, borrowed.Close(ctx))
}

func bindTestRuntimeSession(state *serverState, session *runtimecore.Session) {
	if state != nil && session != nil {
		testRuntimeSessions.Store(state, session)
	}
}

func clearTestRuntimeSession(state *serverState) *runtimecore.Session {
	if state == nil {
		return nil
	}
	value, ok := testRuntimeSessions.LoadAndDelete(state)
	if !ok {
		return nil
	}
	session, _ := value.(*runtimecore.Session)
	return session
}

func testRuntimeSession(state *serverState) *runtimecore.Session {
	if state == nil {
		return nil
	}
	value, _ := testRuntimeSessions.Load(state)
	session, _ := value.(*runtimecore.Session)
	return session
}

type testWorkspaceRunSubmitter interface {
	SubmitWorkspaceRun(context.Context, workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error)
}

type sessionBackedTestExecutionAdmission struct {
	state     *serverState
	submitter testWorkspaceRunSubmitter
}

func (a sessionBackedTestExecutionAdmission) AdmitWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedCommand, error) {
	if a.state == nil || testRuntimeSession(a.state) == nil {
		return workorchdevents.WorkspaceRunRequestedCommand{}, common.ErrConflict
	}
	command = prepareWorkspaceRunSchedulerCommand(topic, command)
	if a.submitter == nil {
		return workorchdevents.WorkspaceRunRequestedCommand{}, common.ErrConflict
	}
	outcome, err := a.submitter.SubmitWorkspaceRun(ctx, command)
	if err != nil {
		a.state.applyProjectedRunCommandRejected(workspaceviewcore.RunCommandRejectedUpdate{
			WorkspaceID:    string(command.WorkspaceID),
			Intent:         command.Intent,
			Reason:         command.Reason,
			RequestID:      command.RequestID,
			IdempotencyKey: testSchedulerCommandIdempotencyKey(command),
			Source:         command.Source,
			RejectedAt:     time.Now().UTC().Format(time.RFC3339Nano),
			Error:          err.Error(),
		})
		a.state.recordWorkspaceRunFailure(command, schedulerKindWorkspaceActivation, schedulerReasonWorkspaceActivationFailed, err)
		return command, err
	}
	projectTestWorkspaceExecutionOutcome(a.state, command, outcome)
	return command, nil
}

func projectTestWorkspaceExecutionOutcome(state *serverState, command workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) {
	if state == nil {
		return
	}
	workspaceID := strings.TrimSpace(string(outcome.WorkspaceID))
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(string(command.WorkspaceID))
	}
	disposition := string(outcome.Outcome)
	diagnostic := runtimecontract.RuntimeExecutionIntentReasonSummary(outcome.Reason)
	if outcome.Outcome == runtimecontract.RuntimeExecutionIntentReconciliationRequested || outcome.Outcome == runtimecontract.RuntimeExecutionIntentReplacementPending {
		state.applyProjectedRunCommandAccepted(workspaceviewcore.RunCommandAcceptedUpdate{
			WorkspaceID: workspaceID, RunID: string(outcome.RunID), Intent: command.Intent, Reason: command.Reason,
			Disposition: disposition, Diagnostic: diagnostic, RequestID: command.RequestID,
			IdempotencyKey: testSchedulerCommandIdempotencyKey(command), Source: command.Source,
			AcceptedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		return
	}
	state.applyProjectedRunCommandRejected(workspaceviewcore.RunCommandRejectedUpdate{
		WorkspaceID: workspaceID, RunID: string(outcome.RunID), Intent: command.Intent, Reason: command.Reason,
		Disposition: disposition, Diagnostic: diagnostic, RequestID: command.RequestID,
		IdempotencyKey: testSchedulerCommandIdempotencyKey(command), Source: command.Source,
		RejectedAt: time.Now().UTC().Format(time.RFC3339Nano), Error: diagnostic,
	})
}

func testSchedulerCommandIdempotencyKey(command workorchdevents.WorkspaceRunRequestedCommand) string {
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	intent := strings.TrimSpace(command.Intent)
	if workspaceID == "" || intent == "" {
		return ""
	}
	return intent + "/" + workspaceID
}
