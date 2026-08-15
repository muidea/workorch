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
	executionusecase "github.com/muidea/workorch/internal/modules/application/workorchd/biz/execution"
	projectioncomponent "github.com/muidea/workorch/internal/modules/application/workorchd/biz/projection"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

const ModuleID = modulecommon.ModuleID

// Biz is the application owner. It owns the framework-provided EventHub,
// runtime session, CLI command subscriptions and background lifecycle; the
// module root only delegates framework callbacks to it.
type Biz struct {
	basebiz.Base
	service       *server.Service
	runtime       *runtimeComponent
	workspaceGate *workspaceCommandGateway
	cliDispatcher *appservice.CLICommandDispatcher
	execution     *executionusecase.UseCase
	projection    *projectioncomponent.Component
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
	runtime, runtimeErr := newRuntimeComponentFromCommandHub(b.EventHub(), b.BackgroundRoutine())
	if runtimeErr != nil {
		b.service = nil
		return cd.NewError(cd.IllegalParam, runtimeErr.Error())
	}
	b.runtime = runtime
	b.workspaceGate = &workspaceCommandGateway{hub: b.EventHub(), runtime: runtime}
	applicationPorts := runtime.applicationPorts()
	if err := b.service.StartupWithApplicationPorts(ctx, modulecommon.ServiceName, applicationPorts, b.BackgroundRoutine()); err != nil {
		_ = b.runtime.close(ctx)
		b.runtime = nil
		b.workspaceGate = nil
		b.service = nil
		return err
	}
	b.projection = projectioncomponent.New(applicationProjectionLoader{config: config, ports: applicationPorts.Workspace}, b.BackgroundRoutine(), workspaceviewcore.EventPorts{
		ReadHighWatermarks: b.readWorkspaceProjectionHighWatermarks,
		PublishChanged:     b.publishWorkspaceProjectionChanged,
	})
	b.service.SetWorkspaceProjection(b.projection)
	b.service.SetWorkspaceProjectionWriter(b.projection)
	b.execution = executionusecase.New(b, executionProjection{projection: b.projection, transport: b.service}, b.BackgroundRoutine())
	b.service.SetWorkspaceExecutionAdmission(b.execution)
	b.service.SetWorkspaceRunScheduler(b)
	b.service.SetServiceStoppedNotifier(b.publishServiceStopped)
	b.SubscribeFunc(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, b.handleWorkspaceRunRequested)
	b.SubscribeMatchedFunc(ModuleID+"/workspace-projection", workorchdevents.NotificationDestination, workorchdevents.TopicWorkspaceProjectionChanged, b.handleWorkspaceProjectionChanged)
	b.SubscribeMatchedFunc(ModuleID+"/run-metrics", runevents.NotificationDestination, runevents.TopicRunStateChanged, b.handleRunStateChanged)
	b.SubscribeMatchedFunc(ModuleID+"/runtime-drive-metrics", orchestratorevents.NotificationDestination, orchestratorevents.TopicOrchestratorRuntimeDriveObserved, b.handleRuntimeDriveObserved)
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
	if b == nil || b.service == nil || !b.runtime.ready() || b.projection == nil {
		return cd.NewError(cd.IllegalParam, "workorch module is not configured")
	}
	if err := b.projection.Start(ctx); err != nil {
		return cd.NewError(cd.Unexpected, fmt.Sprintf("workspace projection start failed: %v", err))
	}
	if err := b.service.Run(ctx); err != nil {
		b.projection.Close()
		return cd.NewError(err.Code, fmt.Sprintf("workorch service run failed: %s", err.Message))
	}
	b.cliDispatcher = b.runtime.newCLICommandDispatcher(b.EventHub(), b.service)
	if err := b.cliDispatcher.Run(); err != nil {
		b.cliDispatcher = nil
		b.service.Shutdown(ctx)
		b.projection.Close()
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
	b.UnsubscribeMatchedFunc(ModuleID+"/runtime-drive-metrics", orchestratorevents.TopicOrchestratorRuntimeDriveObserved)
	b.UnsubscribeMatchedFunc(ModuleID+"/eventstream", eventstreamevents.TopicEventStreamRecordAppended)
	if b.execution != nil {
		_ = b.execution.Close(ctx)
		b.execution = nil
	}
	if b.service != nil {
		b.service.Shutdown(ctx)
		// Clear injected ports only after the HTTP server has stopped. During
		// graceful shutdown an in-flight handler can still finish against the
		// live projection, while the closed execution use case already fails
		// closed for new admissions.
		b.service.SetWorkspaceExecutionAdmission(nil)
		b.service.SetWorkspaceProjection(nil)
		b.service.SetWorkspaceProjectionWriter(nil)
	}
	if b.projection != nil {
		_ = b.projection.CloseContext(ctx)
	}
	b.projection = nil
	if b.runtime != nil {
		_ = b.runtime.close(ctx)
		b.runtime = nil
	}
	b.workspaceGate = nil
	b.service = nil
}

func (b *Biz) handleWorkspaceProjectionChanged(event magicevent.Event, _ magicevent.Result) {
	data, ok := event.Data().(workorchdevents.WorkspaceProjectionChangedData)
	if ok && b != nil && b.service != nil {
		b.service.RecordWorkspaceProjectionChanged(event.Context(), data)
	}
}

// handleRunStateChanged consumes the Run owner's concrete state notification
// for process-local observability. Eventstream remains an audit/SSE source and
// must not become the metric fact source.
func (b *Biz) handleRunStateChanged(event magicevent.Event, _ magicevent.Result) {
	data, ok := event.Data().(runevents.RunStateChangedData)
	if ok && b != nil && b.service != nil {
		b.service.RecordRunStateChanged(event.Context(), data)
	}
}

// handleRuntimeDriveObserved forwards immutable Orchestrator telemetry to
// the process-local metric store. It cannot schedule work or change an owner
// state; Runtime authority remains in kernel/orchestrator.
func (b *Biz) handleRuntimeDriveObserved(event magicevent.Event, _ magicevent.Result) {
	data, ok := event.Data().(orchestratorevents.RuntimeDriveObservedData)
	if ok && event.Source() == "kernel/orchestrator" && data.Validate() == nil && b != nil && b.service != nil {
		b.service.RecordRuntimeDriveObserved(event.Context(), data)
		if b.projection != nil {
			b.projection.HandleRuntimeDriveObserved(data)
		}
	}
}

func (b *Biz) handleEventRecordAppended(event magicevent.Event, _ magicevent.Result) {
	data, ok := event.Data().(eventstreamevents.EventRecordAppendedData)
	if ok && b != nil && b.service != nil {
		b.service.RecordEventRecordAppended(event.Context(), data.Record)
		if b.projection != nil {
			_ = b.projection.HandleEventRecord(event.Context(), data.Record)
		}
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
	data, resultErr := response.Get()
	if resultErr != nil {
		return nil, resultErr
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
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(workorchdevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "workspace_projection", SubjectID: string(data.WorkspaceID), WorkspaceID: string(data.WorkspaceID)}))
	b.PostEvent(event)
}

// SubmitWorkspaceRun is the application ingress adapter. It may compile a
// restart bootstrap from owner snapshots, but it never chooses or performs a
// Workspace/Run lifecycle transition. Both resume and restart are forwarded as
// one typed Workspace-owner intent.
func (b *Biz) SubmitWorkspaceRun(ctx context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if b == nil || b.workspaceGate == nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, errRuntimeComponentUnavailable
	}
	return b.workspaceGate.submit(ctx, command)
}

func (b *Biz) handleWorkspaceRunRequested(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(workorchdevents.WorkspaceRunRequestedCommand)
	if !ok {
		if result != nil {
			result.Set(nil, cd.NewError(cd.IllegalParam, common.ErrInvalidArgument.Error()))
		}
		return
	}
	var (
		response workorchdevents.WorkspaceRunRequestedResult
		err      error
	)
	if b == nil || b.execution == nil {
		err = fmt.Errorf("%w: workspace execution use case is not configured", common.ErrConflict)
	} else {
		response, err = b.execution.SubmitWorkspaceExecution(event.Context(), workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, command)
	}
	if err != nil {
		if result != nil {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		}
		return
	}
	if result != nil {
		result.Set(response, nil)
	}
}
