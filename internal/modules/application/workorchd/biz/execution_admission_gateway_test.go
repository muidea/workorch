package biz

import (
	"context"
	"sync"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	executionusecase "github.com/muidea/workorch/internal/modules/application/workorchd/biz/execution"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type laneOrderingSubmitter struct {
	firstEntered  chan struct{}
	secondEntered chan struct{}
	releaseFirst  chan struct{}
}

func (s *laneOrderingSubmitter) SubmitWorkspaceRun(_ context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	switch command.RequestID {
	case "request-old":
		close(s.firstEntered)
		<-s.releaseFirst
	case "request-new":
		close(s.secondEntered)
	case "request-rejected":
		return workorchdevents.WorkspaceRunRequestedResult{
			WorkspaceID: command.WorkspaceID,
			RunID:       "run-rejected",
			RequestID:   command.RequestID,
			Kind:        runtimecontract.RuntimeExecutionIntentKind(command.Intent),
			Outcome:     runtimecontract.RuntimeExecutionIntentGovernanceRequired,
			Reason:      runtimecontract.RuntimeExecutionIntentReasonGovernancePending,
		}, nil
	}
	return workorchdevents.WorkspaceRunRequestedResult{
		WorkspaceID: command.WorkspaceID,
		RunID:       "run-1",
		RequestID:   command.RequestID,
		Kind:        runtimecontract.RuntimeExecutionIntentKind(command.Intent),
		Outcome:     runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		Reason:      runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
	}, nil
}

type laneOrderingProjection struct {
	mu       sync.Mutex
	requests []string
}

func (p *laneOrderingProjection) RecordWorkspaceExecutionOutcome(_ string, command workorchdevents.WorkspaceRunRequestedCommand, _ workorchdevents.WorkspaceRunRequestedResult) {
	p.mu.Lock()
	p.requests = append(p.requests, command.RequestID)
	p.mu.Unlock()
}

func (p *laneOrderingProjection) RecordWorkspaceExecutionFailure(_ string, command workorchdevents.WorkspaceRunRequestedCommand, _ error) {
	p.mu.Lock()
	p.requests = append(p.requests, command.RequestID)
	p.mu.Unlock()
}

func TestExecutionAdmissionGatewaySerializesOwnerAdmissionAndProjectionByWorkspace(t *testing.T) {
	hub := magicevent.NewHub(16)
	background := magictask.NewBackgroundRoutine(16)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	t.Cleanup(func() {
		hub.Terminate(ctx)
		background.Shutdown(ctx)
	})

	submitter := &laneOrderingSubmitter{
		firstEntered:  make(chan struct{}),
		secondEntered: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
	t.Cleanup(func() {
		select {
		case <-submitter.releaseFirst:
		default:
			close(submitter.releaseFirst)
		}
	})
	projection := &laneOrderingProjection{}
	b := &Biz{Base: basebiz.New(ModuleID, hub, background)}
	b.execution = executionusecase.New(submitter, projection)
	b.admissions = newExecutionAdmissionLifecycle()
	b.SubscribeFunc(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, b.handleWorkspaceRunRequested)
	t.Cleanup(func() { b.UnsubscribeFunc(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested) })
	gateway := newExecutionAdmissionGateway(b)

	errors := make(chan error, 2)
	go func() {
		_, err := gateway.AdmitWorkspaceExecution(ctx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
			WorkspaceID: "ws-lane-order", Intent: "resume", RequestID: "request-old", Source: "http",
		})
		errors <- err
	}()
	select {
	case <-submitter.firstEntered:
	case <-ctx.Done():
		t.Fatalf("first admission did not enter owner: %v", ctx.Err())
	}

	go func() {
		_, err := gateway.AdmitWorkspaceExecution(ctx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
			WorkspaceID: "ws-lane-order", Intent: "resume", RequestID: "request-new", Source: "http",
		})
		errors <- err
	}()
	select {
	case <-submitter.secondEntered:
		t.Fatal("newer same-workspace admission entered before the older lane transaction committed")
	case <-time.After(100 * time.Millisecond):
		// The second request is waiting on the shared Workorchd workspace lane.
	}
	close(submitter.releaseFirst)
	for range 2 {
		select {
		case err := <-errors:
			if err != nil {
				t.Fatalf("admission: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("admission did not complete: %v", ctx.Err())
		}
	}
	rejected, err := gateway.AdmitWorkspaceExecution(ctx, workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-lane-order", Intent: "resume", RequestID: "request-rejected", Source: "http",
	})
	if err != nil {
		t.Fatalf("rejected owner admission transport: %v", err)
	}
	if rejected.RunID != "run-rejected" || rejected.Outcome != runtimecontract.RuntimeExecutionIntentGovernanceRequired || rejected.Reason != runtimecontract.RuntimeExecutionIntentReasonGovernancePending {
		t.Fatalf("gateway owner result = %+v, want exact rejection", rejected)
	}

	projection.mu.Lock()
	defer projection.mu.Unlock()
	if len(projection.requests) != 3 || projection.requests[0] != "request-old" || projection.requests[1] != "request-new" || projection.requests[2] != "request-rejected" {
		t.Fatalf("projection commit order = %v, want [request-old request-new request-rejected]", projection.requests)
	}
}

func TestExecutionAdmissionGatewayUsesPureWorkspaceLane(t *testing.T) {
	if got, want := executionAdmissionLaneKey(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "ws-lane-scope"), "state/workorchd.execution_admission/workspace/ws-lane-scope"; got != want {
		t.Fatalf("execution admission lane = %q, want %q", got, want)
	}
	if left, right := executionAdmissionLaneKey(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "ws-left"), executionAdmissionLaneKey(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "ws-right"); left == right {
		t.Fatalf("workspace execution admission lanes collided: %q", left)
	}
}
