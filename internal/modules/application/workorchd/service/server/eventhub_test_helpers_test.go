package server

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func newIsolatedTestHTTPServerWithBackground(config Config, backgroundRoutine task.BackgroundRoutine) *HTTPServer {
	server := newTestHTTPServerWithCommandHubAndBackground(config, nil, backgroundRoutine)
	attachTestEventNotifications(server.state)
	return server
}

func newIsolatedTestServerStateWithBackground(config Config, backgroundRoutine task.BackgroundRoutine) *serverState {
	state := newTestServerStateWithCommandHubAndBackground(config, nil, backgroundRoutine)
	attachTestEventNotifications(state)
	return state
}

func attachTestEventNotifications(state *serverState) {
	hub := testStateCommandHub(state)
	if hub == nil {
		return
	}
	projection := magicevent.NewSimpleObserver(workorchdevents.NotificationDestination, hub)
	projection.Subscribe(workorchdevents.TopicWorkspaceProjectionChanged, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(workorchdevents.WorkspaceProjectionChangedData); ok {
			state.RecordWorkspaceProjectionChanged(event.Context(), data)
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	records := magicevent.NewSimpleObserver(eventstreamevents.NotificationDestination, hub)
	records.Subscribe(eventstreamevents.TopicEventStreamRecordAppended, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(eventstreamevents.EventRecordAppendedData); ok {
			state.RecordEventRecordAppended(event.Context(), data.Record)
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	state.eventUnsubscribes = append(state.eventUnsubscribes,
		func() { projection.Unsubscribe(workorchdevents.TopicWorkspaceProjectionChanged) },
		func() { records.Unsubscribe(eventstreamevents.TopicEventStreamRecordAppended) },
	)
	state.workspaceProjection.SetEventPorts(workspaceviewcore.EventPorts{
		ReadHighWatermarks: func(_ context.Context, workspaceIDs []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error) {
			items := make([]eventstreamevents.WorkspaceHighWatermark, 0, len(workspaceIDs))
			for _, workspaceID := range workspaceIDs {
				items = append(items, eventstreamevents.WorkspaceHighWatermark{WorkspaceID: workspaceID})
			}
			return items, nil
		},
		PublishChanged: func(ctx context.Context, data workorchdevents.WorkspaceProjectionChangedData) {
			state.RecordWorkspaceProjectionChanged(ctx, data)
		},
	})
}

// These helpers keep raw EventHub access in test fixtures only. Production
// server construction is session-backed and never retains the Hub.
func newTestServerStateWithCommandHubAndBackground(config Config, hub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) *serverState {
	if hub == nil {
		hub = magicevent.NewHub(1024)
	}
	session, err := appservice.NewRuntimeSessionFromCommandHub(hub, backgroundRoutine)
	return newServerStateWithRuntimeSessionAndBackground(config, session, backgroundRoutine, err)
}

func newTestHTTPServerWithCommandHubAndBackground(config Config, hub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) *HTTPServer {
	if hub == nil {
		hub = magicevent.NewHub(1024)
	}
	session, err := appservice.NewRuntimeSessionFromCommandHub(hub, backgroundRoutine)
	return newHTTPServerWithRuntimeSessionAndBackground(config, session, backgroundRoutine, err)
}

func testStateCommandHub(state *serverState) magicevent.Hub {
	if state == nil {
		return nil
	}
	state.sharedSessionMu.RLock()
	session := state.sharedSession
	state.sharedSessionMu.RUnlock()
	if session == nil {
		return nil
	}
	exports, err := session.Exports()
	if err != nil {
		return nil
	}
	return exports.Infrastructure.CommandHub
}
