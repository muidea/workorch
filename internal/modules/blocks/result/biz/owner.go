package result

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	Results           repository.ResultRepository
	Artifacts         repository.ArtifactRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	service *Service
	topics  []string
	started bool
}

func New(h magicevent.Hub, b task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Results == nil || deps.Artifacts == nil || deps.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	if h == nil {
		return nil, common.ErrInvalidArgument
	}
	return &Biz{Base: basebiz.New(modulecommon.ModuleID, h, b), service: NewService(deps.Results, deps.Artifacts)}, nil
}

func (m *Biz) Start() error {
	if m == nil || m.service == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	m.topics = []string{
		resultevents.TopicResultCommandCreate,
		resultevents.TopicResultCommandGet,
		resultevents.TopicResultCommandListByWorkspace,
		resultevents.TopicResultCommandListByRun,
		resultevents.TopicResultCommandUpdate,
		resultevents.TopicResultCommandDelete,
		resultevents.TopicArtifactCommandCreate,
		resultevents.TopicArtifactCommandGet,
		resultevents.TopicArtifactCommandListByWorkspace,
		resultevents.TopicArtifactCommandListByRun,
		resultevents.TopicArtifactCommandListByResult,
		resultevents.TopicArtifactCommandUpdate,
		resultevents.TopicArtifactCommandDelete,
	}
	for _, topic := range m.topics {
		m.SubscribeFunc(topic, m.handleCommand)
	}
	m.started = true
	return nil
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.topics = nil
	m.service = nil
	m.started = false
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setMagicResult(result, response, err)
}

func (m *Biz) executeCommand(ev magicevent.Event) (resultevents.ResultCommandResult, error) {
	if m == nil || m.service == nil {
		return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
	}
	ctx := ev.Context()
	switch ev.ID() {
	case resultevents.TopicResultCommandCreate:
		cmd, ok := ev.Data().(resultevents.CreateResultCommand)
		if !ok || cmd.Result == nil {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.Result.WorkspaceID)
		if err := m.service.CreateResult(ctx, cmd.Result); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		m.publishResultChanged(ctx, cmd.Result)
		return resultevents.ResultCommandResult{}, nil
	case resultevents.TopicResultCommandGet:
		cmd, ok := ev.Data().(resultevents.GetResultCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.service.GetResult(ctx, cmd.ResultID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Result: item}, nil
	case resultevents.TopicResultCommandListByWorkspace:
		cmd, ok := ev.Data().(resultevents.ListResultsByWorkspaceCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.WorkspaceID)
		items, err := m.service.ListResultsByWorkspace(ctx, cmd.WorkspaceID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Results: items}, nil
	case resultevents.TopicResultCommandListByRun:
		cmd, ok := ev.Data().(resultevents.ListResultsByRunCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.service.ListResultsByRun(ctx, cmd.RunID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Results: items}, nil
	case resultevents.TopicResultCommandUpdate:
		cmd, ok := ev.Data().(resultevents.UpdateResultCommand)
		if !ok || cmd.Result == nil {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.Result.WorkspaceID)
		if err := m.service.UpdateResult(ctx, cmd.Result); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		m.publishResultChanged(ctx, cmd.Result)
		return resultevents.ResultCommandResult{}, nil
	case resultevents.TopicResultCommandDelete:
		cmd, ok := ev.Data().(resultevents.DeleteResultCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.DeleteResult(ctx, cmd.ResultID); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{}, nil
	case resultevents.TopicArtifactCommandCreate:
		cmd, ok := ev.Data().(resultevents.CreateArtifactCommand)
		if !ok || cmd.Artifact == nil {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.Artifact.WorkspaceID)
		if err := m.service.CreateArtifact(ctx, cmd.Artifact); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		m.publishArtifactCreated(ctx, cmd.Artifact)
		return resultevents.ResultCommandResult{}, nil
	case resultevents.TopicArtifactCommandGet:
		cmd, ok := ev.Data().(resultevents.GetArtifactCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.service.GetArtifact(ctx, cmd.ArtifactID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Artifact: item}, nil
	case resultevents.TopicArtifactCommandListByWorkspace:
		cmd, ok := ev.Data().(resultevents.ListArtifactsByWorkspaceCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.WorkspaceID)
		items, err := m.service.ListArtifactsByWorkspace(ctx, cmd.WorkspaceID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Artifacts: items}, nil
	case resultevents.TopicArtifactCommandListByRun:
		cmd, ok := ev.Data().(resultevents.ListArtifactsByRunCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.service.ListArtifactsByRun(ctx, cmd.RunID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Artifacts: items}, nil
	case resultevents.TopicArtifactCommandListByResult:
		cmd, ok := ev.Data().(resultevents.ListArtifactsByResultCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.service.ListArtifactsByResult(ctx, cmd.ResultID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Artifacts: items}, nil
	case resultevents.TopicArtifactCommandUpdate:
		cmd, ok := ev.Data().(resultevents.UpdateArtifactCommand)
		if !ok || cmd.Artifact == nil {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.Artifact.WorkspaceID)
		if err := m.service.UpdateArtifact(ctx, cmd.Artifact); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{}, nil
	case resultevents.TopicArtifactCommandDelete:
		cmd, ok := ev.Data().(resultevents.DeleteArtifactCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.service.DeleteArtifact(ctx, cmd.ArtifactID); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{}, nil
	default:
		return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
	}
}

func (m *Biz) publishResultChanged(ctx context.Context, item *resultmodel.Result) {
	if m == nil || m.EventHub() == nil || item == nil {
		return
	}
	topic := resultNotificationTopic(item.State)
	event := magicevent.NewEventWithContext(topic, ModuleID, resultevents.NotificationDestination, magicevent.NewHeader(), ctx, resultevents.ResultChangedDataFromModel(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(topic, workorchevents.EventHubLaneScope{
		SubjectType: "result", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID),
	}))
	m.PostEvent(event)
}

func resultNotificationTopic(state common.ResultState) string {
	switch state {
	case common.ResultStateDraft:
		return resultevents.TopicResultCreated
	case common.ResultStateRejected:
		return resultevents.TopicResultRejected
	case common.ResultStateSuperseded:
		return resultevents.TopicResultSuperseded
	default:
		return resultevents.TopicResultAccepted
	}
}

func (m *Biz) publishArtifactCreated(ctx context.Context, item *resultmodel.Artifact) {
	if m == nil || m.EventHub() == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCreated, ModuleID, resultevents.NotificationDestination, magicevent.NewHeader(), ctx, resultevents.ArtifactCreatedDataFromModel(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(resultevents.TopicArtifactCreated, workorchevents.EventHubLaneScope{
		SubjectType: "artifact", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID), StepID: string(item.StepID),
	}))
	m.PostEvent(event)
}

func resultCommandContext(ctx context.Context, workspaceID common.WorkspaceID) context.Context {
	if workspaceID == "" {
		return ctx
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
}

func setMagicResult(result magicevent.Result, data resultevents.ResultCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}
