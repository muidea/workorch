// Package biz owns the WorkOrch server application's lifecycle and EventHub
// participation. HTTP remains an inbound protocol adapter in service/server.
package biz

import (
	"context"
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

const ModuleID = modulecommon.ModuleID

// Biz is the application owner. It owns the framework-provided EventHub,
// runtime session, CLI command subscriptions and background lifecycle; the
// module root only delegates framework callbacks to it.
type Biz struct {
	basebiz.Base
	service       *server.Service
	session       *appservice.RuntimeSession
	cliDispatcher *appservice.CLICommandDispatcher
}

func New() *Biz { return &Biz{} }

func (b *Biz) Setup(ctx context.Context, commandHub magicevent.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	if b == nil || commandHub == nil || backgroundRoutine == nil {
		return cd.NewError(cd.IllegalParam, "workorch Biz infrastructure is not configured")
	}
	config, err := server.ConfigFromEnv()
	if err != nil {
		return cd.NewError(cd.IllegalParam, err.Error())
	}
	b.Base = basebiz.New(ModuleID, commandHub, backgroundRoutine)
	b.service = server.NewService(config)
	session, sessionErr := appservice.NewRuntimeSessionFromCommandHub(b.EventHub(), b.BackgroundRoutine())
	if sessionErr != nil {
		b.service = nil
		return cd.NewError(cd.IllegalParam, sessionErr.Error())
	}
	b.session = session
	if err := b.service.Startup(ctx, modulecommon.ServiceName, b.session, b.BackgroundRoutine()); err != nil {
		_ = b.session.Close(ctx)
		b.session = nil
		b.service = nil
		return err
	}
	b.service.SetWorkspaceRunScheduler(b)
	b.service.SetServiceStoppedNotifier(b.publishServiceStopped)
	b.service.SetWorkspaceProjectionEventPorts(workspaceviewcore.EventPorts{
		ReadHighWatermarks: b.readWorkspaceProjectionHighWatermarks,
		PublishChanged:     b.publishWorkspaceProjectionChanged,
	})
	b.SubscribeFunc(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, b.handleWorkspaceRunRequested)
	b.SubscribeMatchedFunc(ModuleID+"/workspace-projection", workorchdevents.NotificationDestination, workorchdevents.TopicWorkspaceProjectionChanged, b.handleWorkspaceProjectionChanged)
	b.SubscribeMatchedFunc(ModuleID+"/run-metrics", runevents.NotificationDestination, runevents.TopicRunStateChanged, b.handleRunStateChanged)
	b.SubscribeMatchedFunc(ModuleID+"/eventstream", eventstreamevents.NotificationDestination, eventstreamevents.TopicEventStreamRecordAppended, b.handleEventRecordAppended)
	return nil
}

func (b *Biz) publishServiceStopped(data workorchdevents.ServiceStoppedData) {
	if b == nil || b.EventHub() == nil {
		return
	}
	b.PostEvent(magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDServiceStopped, ModuleID, workorchdevents.NotificationDestination, magicevent.NewHeader(), context.Background(), data))
}

func (b *Biz) Start(ctx context.Context) *cd.Error {
	if b == nil || b.service == nil || b.session == nil {
		return cd.NewError(cd.IllegalParam, "workorch module is not configured")
	}
	if err := b.service.Run(ctx); err != nil {
		return cd.NewError(err.Code, fmt.Sprintf("workorch service run failed: %s", err.Message))
	}
	b.cliDispatcher = appservice.NewCLICommandDispatcher(b.session, b.EventHub(), b.service)
	if err := b.cliDispatcher.Run(); err != nil {
		b.cliDispatcher = nil
		b.service.Shutdown(ctx)
		return cd.NewError(cd.IllegalParam, fmt.Sprintf("workorch cli dispatcher run failed: %v", err))
	}
	return nil
}

func (b *Biz) Stop(ctx context.Context) {
	if b == nil {
		return
	}
	if b.cliDispatcher != nil {
		b.cliDispatcher.Close()
		b.cliDispatcher = nil
	}
	b.UnsubscribeFunc(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)
	b.UnsubscribeMatchedFunc(ModuleID+"/workspace-projection", workorchdevents.TopicWorkspaceProjectionChanged)
	b.UnsubscribeMatchedFunc(ModuleID+"/run-metrics", runevents.TopicRunStateChanged)
	b.UnsubscribeMatchedFunc(ModuleID+"/eventstream", eventstreamevents.TopicEventStreamRecordAppended)
	if b.service != nil {
		b.service.Shutdown(ctx)
	}
	if b.session != nil {
		_ = b.session.Close(ctx)
		b.session = nil
	}
	b.service = nil
}

func (b *Biz) handleWorkspaceProjectionChanged(event magicevent.Event, result magicevent.Result) {
	data, ok := event.Data().(workorchdevents.WorkspaceProjectionChangedData)
	if ok && b != nil && b.service != nil {
		b.service.RecordWorkspaceProjectionChanged(event.Context(), data)
	}
	if result != nil {
		result.Set(nil, nil)
	}
}

// handleRunStateChanged consumes the Run owner's concrete state notification
// for process-local observability. Eventstream remains an audit/SSE source and
// must not become the metric fact source.
func (b *Biz) handleRunStateChanged(event magicevent.Event, result magicevent.Result) {
	data, ok := event.Data().(runevents.RunStateChangedData)
	if ok && b != nil && b.service != nil {
		b.service.RecordRunStateChanged(event.Context(), data)
	}
	if result != nil {
		result.Set(nil, nil)
	}
}

func (b *Biz) handleEventRecordAppended(event magicevent.Event, result magicevent.Result) {
	data, ok := event.Data().(eventstreamevents.EventRecordAppendedData)
	if ok && b != nil && b.service != nil {
		b.service.RecordEventRecordAppended(event.Context(), data.Record)
	}
	if result != nil {
		result.Set(nil, nil)
	}
}

func (b *Biz) readWorkspaceProjectionHighWatermarks(ctx context.Context, workspaceIDs []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error) {
	if b == nil || b.EventHub() == nil || len(workspaceIDs) == 0 {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(eventstreamevents.TopicEventStreamCommandHighWatermarks, ModuleID, eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, eventstreamevents.HighWatermarksCommand{WorkspaceIDs: workspaceIDs})
	response := b.SendEvent(event)
	if response == nil {
		return nil, common.ErrConflict
	}
	if err := response.Error(); err != nil {
		if err.Cause != nil {
			return nil, err.Cause
		}
		return nil, err
	}
	data, err := response.Get()
	if err != nil {
		return nil, err
	}
	result, ok := data.(eventstreamevents.EventStreamCommandResult)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return result.HighWatermarks, nil
}

func (b *Biz) publishWorkspaceProjectionChanged(ctx context.Context, data workorchdevents.WorkspaceProjectionChangedData) {
	if b == nil || b.EventHub() == nil {
		return
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkspaceProjectionChanged, ModuleID, workorchdevents.NotificationDestination, magicevent.NewHeader(), ctx, data)
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{SubjectType: "workspace_projection", SubjectID: string(data.WorkspaceID), WorkspaceID: string(data.WorkspaceID)}))
	b.PostEvent(event)
}

// SubmitWorkspaceRun is the server-facing narrow port for an HTTP scheduling
// request. The server owns request adaptation and projection; EventHub
// authority remains in this application Biz.
func (b *Biz) SubmitWorkspaceRun(ctx context.Context, command workorchdevents.WorkspaceRunRequestedCommand) error {
	if b == nil || b.service == nil {
		return common.ErrInvalidArgument
	}
	return b.service.HandleWorkspaceRunRequest(ctx, command)
}

func (b *Biz) handleWorkspaceRunRequested(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(workorchdevents.WorkspaceRunRequestedCommand)
	if !ok {
		if result != nil {
			result.Set(nil, cd.NewError(cd.IllegalParam, common.ErrInvalidArgument.Error()))
		}
		return
	}
	if err := b.SubmitWorkspaceRun(event.Context(), command); err != nil {
		if result != nil {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		}
		return
	}
	if result != nil {
		result.Set(workorchdevents.WorkspaceRunRequestedResult{Accepted: true}, nil)
	}
}

// DispatchRuntimeRun is the typed cross-owner command bridge used by the
// server's scheduler use case. It does not reconstruct Runtime V2 authority.
func (b *Biz) DispatchRuntimeRun(ctx context.Context, workspaceID, runID, action string) error {
	if b == nil || b.EventHub() == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	workspaceScope := common.WorkspaceID(workspaceID)
	runScope := common.RunID(runID)
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceScope)
	switch action {
	case "start":
		event := magicevent.NewEventWithContext(
			runevents.TopicRunCommandActivate,
			ModuleID,
			runevents.CommandDestination,
			magicevent.NewHeader(),
			ctx,
			runevents.ActivateRunCommand{WorkspaceID: workspaceScope, RunID: runScope},
		)
		return b.sendRuntimeRunCommand(event, workspaceScope, runScope)
	case "resume":
		event := magicevent.NewEventWithContext(
			runevents.TopicRunCommandContinue,
			ModuleID,
			runevents.CommandDestination,
			magicevent.NewHeader(),
			ctx,
			runevents.ContinueRunCommand{WorkspaceID: workspaceScope, RunID: runScope},
		)
		return b.sendRuntimeRunCommand(event, workspaceScope, runScope)
	default:
		return common.ErrInvalidArgument
	}
}

func (b *Biz) sendRuntimeRunCommand(event magicevent.Event, workspaceID common.WorkspaceID, runID common.RunID) error {
	if b == nil || event == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(event.ID(), workorchevents.EventHubLaneScope{SubjectType: "run", SubjectID: string(runID), WorkspaceID: string(workspaceID), RunID: string(runID)}))
	response := b.SendEvent(event)
	if response == nil {
		return common.ErrConflict
	}
	if err := response.Error(); err != nil {
		if err.Cause != nil {
			return err.Cause
		}
		return err
	}
	data, err := response.Get()
	if err != nil {
		return err
	}
	if _, ok := data.(runevents.RunLifecycleCommandResult); !ok {
		return common.ErrInvalidArgument
	}
	return nil
}
