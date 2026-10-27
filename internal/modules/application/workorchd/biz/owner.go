// Package biz owns the WorkOrch server application's lifecycle and EventHub
// participation. HTTP remains an inbound protocol adapter in service/server.
package biz

import (
	"context"
	"fmt"
	"log/slog"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	executionusecase "github.com/muidea/workorch/internal/modules/application/workorchd/biz/execution"
	projectioncomponent "github.com/muidea/workorch/internal/modules/application/workorchd/biz/projection"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	eventstreamcommon "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/common"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspacecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
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
	service                   *server.Service
	runtime                   *runtimeComponent
	workspaceGate             *workspaceCommandGateway
	cliDispatcher             *appservice.CLICommandDispatcher
	execution                 *executionusecase.UseCase
	admissions                *executionAdmissionLifecycle
	projection                *projectioncomponent.Component
	authoringTasks            *authoringTaskLifecycle
	subscriptions             []func() error
	shutdownErr               error
	preparationSkillHubRunner contextregistryports.SkillHubPreparationRunner
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
	b.authoringTasks = runtime.authoringTasks
	applicationPorts := runtime.applicationPorts()
	b.configureAuthoringBackgroundPorts(&applicationPorts)
	if err := b.service.StartupWithApplicationPorts(ctx, modulecommon.ServiceName, applicationPorts, b.BackgroundRoutine()); err != nil {
		// 保留部分启动取得的 session/service，由检查式停机统一排空和清理。
		return err
	}
	b.projection = projectioncomponent.New(applicationProjectionLoader{config: config, ports: applicationPorts.Workspace}, b.BackgroundRoutine(), workspaceviewcore.EventPorts{
		ReadHighWatermarks: b.readWorkspaceProjectionHighWatermarks,
		PublishChanged:     b.publishWorkspaceProjectionChanged,
	})
	b.service.SetWorkspaceProjection(b.projection)
	b.service.SetWorkspaceProjectionWriter(b.projection)
	b.execution = executionusecase.New(b, executionProjection{projection: b.projection, transport: b.service})
	b.admissions = newExecutionAdmissionLifecycle()
	b.service.SetWorkspaceExecutionAdmission(newExecutionAdmissionGateway(b))
	b.service.SetServiceStoppedNotifier(b.publishServiceStopped)
	if err := b.subscribeCommand(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, b.handleWorkspaceRunRequested); err != nil {
		return cd.WrapError(cd.Unexpected, err, "workorch command subscription failed")
	}
	for _, subscription := range []struct {
		id, destination, topic string
		handler                magicevent.ObserverFunc
	}{
		{ModuleID + "/workspace-projection", workorchdevents.NotificationDestination, workorchdevents.TopicWorkspaceProjectionChanged, b.handleWorkspaceProjectionChanged},
		{ModuleID + "/run-metrics", runevents.NotificationDestination, runevents.TopicRunStateChanged, b.handleRunStateChanged},
		{ModuleID + "/runtime-drive-metrics", orchestratorevents.NotificationDestination, orchestratorevents.TopicOrchestratorRuntimeDriveObserved, b.handleRuntimeDriveObserved},
		{ModuleID + "/eventstream", eventstreamevents.NotificationDestination, eventstreamevents.TopicEventStreamRecordAppended, b.handleEventRecordAppended},
		{ModuleID + "/workspace-deletion-transport", workspaceevents.NotificationDestination, workspaceevents.TopicWorkspaceDeleted, b.handleWorkspaceDeleted},
	} {
		if err := b.subscribeNotification(subscription.id, subscription.destination, subscription.topic, subscription.handler); err != nil {
			return cd.WrapError(cd.Unexpected, err, "workorch notification subscription failed")
		}
	}
	return nil
}

func (b *Biz) subscribeCommand(topic string, handler magicevent.ObserverFunc) error {
	if err := b.SubscribeFuncChecked(topic, handler); err != nil {
		return err
	}
	b.subscriptions = append(b.subscriptions, func() error { return b.UnsubscribeFuncChecked(topic) })
	return nil
}

func (b *Biz) subscribeNotification(id, destination, topic string, handler magicevent.ObserverFunc) error {
	if err := b.SubscribeMatchedFuncChecked(id, destination, topic, handler); err != nil {
		return err
	}
	b.subscriptions = append(b.subscriptions, func() error { return b.UnsubscribeMatchedFuncChecked(id, topic) })
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
		return cd.WrapError(err.Code, err, "workorch service run failed")
	}
	b.cliDispatcher = b.runtime.newCLICommandDispatcher(b.EventHub(), b.service)
	if err := b.cliDispatcher.Run(); err != nil {
		// 保留部分注册的 dispatcher，不能在 Run 失败分支提前释放依赖。
		return cd.WrapError(cd.IllegalParam, err, "workorch cli dispatcher run failed")
	}
	return nil
}

func (b *Biz) BeginShutdown(ctx context.Context) {
	if b == nil {
		return
	}
	if b.admissions != nil {
		b.admissions.stop()
	}
	if b.authoringTasks != nil {
		b.authoringTasks.beginStop(ctx)
	}
	if b.cliDispatcher != nil {
		b.cliDispatcher.BeginShutdown()
	}
	if b.projection != nil {
		b.projection.BeginShutdown()
	}
	if b.service != nil {
		// listener 关闭失败仍保留 service，Quiesce 将检查并重试该动作。
		b.shutdownErr = b.service.BeginShutdown(ctx)
	}
}

func (b *Biz) Quiesce(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if ctx == nil {
		return common.ErrInvalidArgument
	}
	if b.service != nil {
		if err := b.service.Quiesce(ctx); err != nil {
			return err
		}
		b.shutdownErr = nil
	}
	if b.shutdownErr != nil {
		return b.shutdownErr
	}
	if b.authoringTasks != nil {
		if err := b.authoringTasks.waitContext(ctx); err != nil {
			return err
		}
	}
	if err := b.admissions.waitContext(ctx); err != nil {
		return err
	}
	if b.cliDispatcher != nil {
		if err := b.cliDispatcher.Quiesce(ctx); err != nil {
			return err
		}
	}
	if b.projection != nil {
		return b.projection.Quiesce(ctx)
	}
	return nil
}

func (b *Biz) Stop(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if ctx == nil {
		return common.ErrInvalidArgument
	}
	b.BeginShutdown(ctx)
	if err := b.Quiesce(ctx); err != nil {
		return err
	}
	admissions := b.admissions
	if admissions != nil {
		// Reject every new HTTP/CLI/TUI admission before stopping an ingress.
		// Already-entered handlers retain all dependencies until the shared
		// barrier is drained below.
		admissions.stop()
	}
	if b.authoringTasks != nil {
		b.authoringTasks.beginStop(ctx)
	}
	if b.cliDispatcher != nil {
		if err := b.cliDispatcher.CloseContext(ctx); err != nil {
			return err
		}
		b.cliDispatcher = nil
	}
	if b.service != nil {
		// Stop accepting HTTP requests and wait for in-flight admissions while
		// the Workorchd command subscription, execution use case and projection
		// are still live. This preserves the lane transaction through shutdown.
		if err := b.service.DrainHTTPChecked(ctx); err != nil {
			return err
		}
	}
	if b.authoringTasks != nil {
		if err := b.authoringTasks.waitContext(ctx); err != nil {
			slog.Warn("authoring shutdown did not drain accepted calls; dependencies retained", "failureClass", "authoring_drain_timeout")
			return err
		}
	}
	if admissions != nil {
		if err := admissions.waitContext(ctx); err != nil {
			return err
		}
	}
	if b.service != nil {
		if err := b.service.CloseTransportStateChecked(ctx); err != nil {
			return err
		}
		b.service.SetWorkspaceExecutionAdmission(nil)
		b.service.SetWorkspaceProjection(nil)
		b.service.SetWorkspaceProjectionWriter(nil)
	}
	for len(b.subscriptions) > 0 {
		last := len(b.subscriptions) - 1
		if err := b.subscriptions[last](); err != nil {
			return err
		}
		b.subscriptions = b.subscriptions[:last]
	}
	b.execution = nil
	if b.projection != nil {
		if err := b.projection.CloseContext(ctx); err != nil {
			return err
		}
		b.projection = nil
	}
	if b.runtime != nil {
		if err := b.runtime.close(ctx); err != nil {
			return err
		}
		b.runtime = nil
	}
	b.workspaceGate = nil
	b.service = nil
	return nil
}

func (b *Biz) handleWorkspaceProjectionChanged(event magicevent.Event, _ magicevent.Result) {
	data, ok := workspaceProjectionChangedFromOwner(event)
	if ok && b != nil && b.service != nil {
		b.service.RecordWorkspaceProjectionChanged(event.Context(), data)
	}
}

func workspaceProjectionChangedFromOwner(event magicevent.Event) (workorchdevents.WorkspaceProjectionChangedData, bool) {
	if event == nil || event.Source() != ModuleID {
		return workorchdevents.WorkspaceProjectionChangedData{}, false
	}
	data, ok := event.Data().(workorchdevents.WorkspaceProjectionChangedData)
	return data, ok
}

// handleRunStateChanged consumes the Run owner's concrete state notification
// for process-local observability. Eventstream remains an audit/SSE source and
// must not become the metric fact source.
func (b *Biz) handleRunStateChanged(event magicevent.Event, _ magicevent.Result) {
	data, ok := event.Data().(runevents.RunStateChangedData)
	if ok && event.Source() == "blocks/run" && data.Validate() == nil && b != nil && b.service != nil {
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
	if ok && event.Source() == eventstreamcommon.ModuleID && data.Record.Validate() == nil && b != nil && b.service != nil {
		b.service.RecordEventRecordAppended(event.Context(), data.Record)
		if b.projection != nil {
			_ = b.projection.HandleEventRecord(event.Context(), data.Record)
		}
	}
}

func (b *Biz) handleWorkspaceDeleted(event magicevent.Event, _ magicevent.Result) {
	data, ok := event.Data().(workspaceevents.WorkspaceDeletedData)
	if ok && event.Source() == workspacecommon.ModuleID && data.Validate() == nil && b != nil && b.service != nil {
		b.service.RecordWorkspaceDeleted(event.Context(), data)
	}
}

func (b *Biz) readWorkspaceProjectionHighWatermarks(ctx context.Context, workspaceIDs []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error) {
	if b == nil || b.EventHub() == nil || len(workspaceIDs) == 0 {
		return nil, common.ErrInvalidArgument
	}
	command, err := (eventstreamevents.HighWatermarksCommand{WorkspaceIDs: workspaceIDs}).Normalized()
	if err != nil {
		return nil, err
	}
	event := magicevent.NewEventWithContext(eventstreamevents.TopicEventStreamCommandHighWatermarks, ModuleID, eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, command)
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
	result, ok := data.(eventstreamevents.HighWatermarksResult)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	if err := result.ValidateFor(command); err != nil {
		return nil, err
	}
	return result.Clone().HighWatermarks, nil
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
	if b == nil {
		if result != nil {
			err := fmt.Errorf("%w: workorchd execution admission is unavailable", common.ErrConflict)
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		}
		return
	}
	admissions := b.admissions
	if admissions == nil || !admissions.begin() {
		if result != nil {
			err := fmt.Errorf("%w: workorchd execution admission is stopping", common.ErrConflict)
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		}
		return
	}
	defer admissions.end()
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
