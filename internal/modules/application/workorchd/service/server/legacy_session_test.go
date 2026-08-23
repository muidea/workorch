package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	executionusecase "github.com/muidea/workorch/internal/modules/application/workorchd/biz/execution"
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
	s.workspaceExecutionAdmission = sessionBackedTestExecutionAdmission{state: s, submitter: directWorkspaceExecutionSubmitter{state: s}}
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

func (a sessionBackedTestExecutionAdmission) AdmitWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if a.state == nil || testRuntimeSession(a.state) == nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrConflict
	}
	command = executionusecase.NormalizeWorkspaceExecutionCommandAt(topic, command, time.Now().UTC())
	if a.submitter == nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrConflict
	}
	outcome, err := a.submitter.SubmitWorkspaceRun(ctx, command)
	if err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	projectTestWorkspaceExecutionOutcome(a.state, command, outcome)
	return outcome, nil
}

func projectTestWorkspaceExecutionOutcome(state *serverState, command workorchdevents.WorkspaceRunRequestedCommand, outcome workorchdevents.WorkspaceRunRequestedResult) {
	if state == nil {
		return
	}
	// This test-only session fixture bypasses the production Workorchd Biz lane,
	// so feed its exact owner fact directly to the concrete Biz-owned manager.
	// The production server has no execution-admission projection write port.
	manager, ok := state.workspaceProjection.(*workspaceviewcore.Manager)
	if !ok {
		return
	}
	manager.ApplyWorkspaceExecutionAdmission(command, runtimecontract.RuntimeExecutionIntentAdmission(outcome))
}
