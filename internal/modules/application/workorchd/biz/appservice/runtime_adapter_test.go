package appservice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

type managedRuntimeRecordFixture struct {
	ID           string
	Version      string
	Topic        string
	OccurredAt   string
	SourceModule string
	SubjectType  string
	SubjectID    string
	WorkspaceID  string
	RunID        string
	StepID       string
	Revision     int64
	Payload      map[string]any
}

func newManagedRuntimeTestCommandHub(t *testing.T) magicevent.Hub {
	t.Helper()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	return hub
}

func publishManagedRuntimeRecord(ctx context.Context, hub magicevent.Hub, record managedRuntimeRecordFixture) error {
	data, err := eventstreamevents.EventRecordDataFromModel(&eventstreammodel.EventRecord{
		ID:           common.EventRecordID(record.ID),
		Version:      record.Version,
		Topic:        record.Topic,
		OccurredAt:   record.OccurredAt,
		SourceModule: record.SourceModule,
		SubjectType:  record.SubjectType,
		SubjectID:    record.SubjectID,
		WorkspaceID:  common.WorkspaceID(record.WorkspaceID),
		RunID:        common.RunID(record.RunID),
		StepID:       common.StepID(record.StepID),
		Revision:     record.Revision,
		Payload:      record.Payload,
	})
	if err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(eventstreamevents.TopicEventStreamRecordAppended, "kernel/eventstream", eventstreamevents.NotificationDestination, magicevent.NewHeader(), ctx, eventstreamevents.EventRecordAppendedData{Record: data})
	result := hub.Send(event)
	if result == nil {
		return fmt.Errorf("record appended event returned no result")
	}
	if resultErr := result.Error(); resultErr != nil {
		if strings.Contains(resultErr.Message, "missing observer") {
			return nil
		}
		return resultErr
	}
	return nil
}

func TestManagedRuntimePublishedEventRecordsUsesTypedCollector(t *testing.T) {
	rawHub := newManagedRuntimeTestCommandHub(t)
	runtime := runtimecore.NewManagedRuntime(runtimecore.NewAdapter(runtimecore.Dependencies{}), rawHub)
	defer runtime.Close()

	if err := publishManagedRuntimeRecord(context.Background(), rawHub, managedRuntimeRecordFixture{
		Version:     events.WorkorchEventVersion,
		Topic:       runevents.TopicRunStateChanged,
		SubjectType: "run",
		SubjectID:   "run-1",
		RunID:       "run-1",
		Payload:     map[string]any{events.PayloadKeyState: "running"},
	}); err != nil {
		t.Fatalf("publish run event: %v", err)
	}

	published := runtime.PublishedEventRecords()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic != runevents.TopicRunStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].Topic, runevents.TopicRunStateChanged)
	}
	if payload := published[0].Payload.Map(); payload[events.PayloadKeyState] != "running" {
		t.Fatalf("payload state = %v, want running", payload[events.PayloadKeyState])
	}
}

func TestManagedRuntimeCloseStopsFurtherCollection(t *testing.T) {
	rawHub := newManagedRuntimeTestCommandHub(t)
	runtime := runtimecore.NewManagedRuntime(runtimecore.NewAdapter(runtimecore.Dependencies{}), rawHub)

	runtime.Close()

	if err := publishManagedRuntimeRecord(context.Background(), rawHub, managedRuntimeRecordFixture{
		Version:     events.WorkorchEventVersion,
		Topic:       approvalevents.TopicApprovalCreated,
		SubjectType: "approval",
		SubjectID:   "approval-1",
		Payload:     map[string]any{events.PayloadKeyState: "pending"},
	}); err != nil {
		t.Fatalf("publish approval event: %v", err)
	}

	if published := runtime.PublishedEventRecords(); len(published) != 0 {
		t.Fatalf("len(published) = %d, want 0", len(published))
	}
}

func TestNewManagedRuntimeReturnsNilWhenRuntimeMissing(t *testing.T) {
	rawHub := newManagedRuntimeTestCommandHub(t)
	if runtime := runtimecore.NewManagedRuntime(nil, rawHub); runtime != nil {
		t.Fatalf("runtime = %#v, want nil", runtime)
	}
}

func TestRuntimeAdapterReturnsInvalidArgumentWhenDependenciesMissing(t *testing.T) {
	ctx := context.Background()
	runtime := runtimecore.NewAdapter(runtimecore.Dependencies{})

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "get workspace", run: func() error { _, err := runtime.GetWorkspaceByID(ctx, "ws"); return err }},
		{name: "ensure workspace", run: func() error { return runtime.EnsureWorkspace(ctx, "ws", "title") }},
		{name: "get run", run: func() error { _, err := runtime.GetRun(ctx, "run"); return err }},
		{name: "ensure run", run: func() error { return runtime.EnsureRun(ctx, "run", "ws") }},
		{name: "ensure run started", run: func() error { return runtime.EnsureRunStarted(ctx, "run") }},
		{name: "ensure run blocked on decision", run: func() error { return runtime.EnsureRunBlockedOnDecision(ctx, "run", "ticket") }},
		{name: "ensure run blocked on approval", run: func() error { return runtime.EnsureRunBlockedOnApproval(ctx, "run", "approval") }},
		{name: "ensure run resumed", run: func() error { return runtime.EnsureRunResumed(ctx, "run") }},
		{name: "get step", run: func() error { _, err := runtime.GetStep(ctx, "run/step-1"); return err }},
		{name: "ensure ticket", run: func() error { return runtime.EnsureTicket(ctx, "ticket", "ws", "run", "run/manual-gate", "decision") }},
		{name: "get ticket", run: func() error { _, err := runtime.GetTicket(ctx, "ticket"); return err }},
		{name: "get resolution", run: func() error { _, err := runtime.GetResolutionByTicketID(ctx, "ticket"); return err }},
		{name: "ensure resolved ticket", run: func() error {
			return runtime.EnsureResolvedTicket(ctx, "resolution", "ticket", "provide_missing_info", nil)
		}},
		{name: "ensure approval", run: func() error {
			return runtime.EnsureApproval(ctx, "approval", "ws", "run", "run/manual-approval", "approval")
		}},
		{name: "get approval", run: func() error { _, err := runtime.GetApproval(ctx, "approval"); return err }},
		{name: "ensure approval approved", run: func() error { return runtime.EnsureApprovalApproved(ctx, "approval") }},
		{name: "ensure approval rejected", run: func() error { return runtime.EnsureApprovalRejected(ctx, "approval") }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("err = %v, want invalid argument", err)
			}
		})
	}
}

func TestRuntimeAdapterNilReceiverReturnsInvalidArgument(t *testing.T) {
	var runtime *runtimecore.Adapter

	if _, err := runtime.GetRun(context.Background(), "run"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("get run err = %v, want invalid argument", err)
	}
	if err := runtime.EnsureRun(context.Background(), "run", "ws"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ensure run err = %v, want invalid argument", err)
	}
}
