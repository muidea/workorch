package result

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/result/pkg/common"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	resultservice "github.com/muidea/workorch/internal/modules/blocks/result/service"
	"github.com/muidea/workorch/internal/pkg/common"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = resultservice.Service

type Dependencies struct {
	Results   repository.ResultRepository
	Artifacts repository.ArtifactRepository
	EventHub  magicevent.Hub
}

type Module struct {
	bizPtr     *resultbiz.Service
	servicePtr *resultservice.Module
	eventHub   magicevent.Hub
	observer   magicevent.SimpleObserver
	topics     []string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	if deps.Results == nil || deps.Artifacts == nil || deps.EventHub == nil {
		return common.ErrInvalidArgument
	}
	m.eventHub = deps.EventHub
	m.bizPtr = resultbiz.NewService(deps.Results, deps.Artifacts)
	m.servicePtr = resultservice.New(resultservice.Dependencies{
		CreateResult:             m.bizPtr.CreateResult,
		GetResult:                m.bizPtr.GetResult,
		ListResultsByWorkspace:   m.bizPtr.ListResultsByWorkspace,
		ListResultsByRun:         m.bizPtr.ListResultsByRun,
		UpdateResult:             m.bizPtr.UpdateResult,
		DeleteResult:             m.bizPtr.DeleteResult,
		CreateArtifact:           m.bizPtr.CreateArtifact,
		GetArtifact:              m.bizPtr.GetArtifact,
		ListArtifactsByWorkspace: m.bizPtr.ListArtifactsByWorkspace,
		ListArtifactsByRun:       m.bizPtr.ListArtifactsByRun,
		ListArtifactsByResult:    m.bizPtr.ListArtifactsByResult,
		UpdateArtifact:           m.bizPtr.UpdateArtifact,
		DeleteArtifact:           m.bizPtr.DeleteArtifact,
	})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.servicePtr == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.observer != nil {
		return nil
	}
	m.observer = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
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
		m.observer.Subscribe(topic, m.handleCommand)
	}
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.observer != nil {
		for idx := len(m.topics) - 1; idx >= 0; idx-- {
			m.observer.Unsubscribe(m.topics[idx])
		}
	}
	m.topics = nil
	m.observer = nil
	m.servicePtr = nil
	m.bizPtr = nil
	m.eventHub = nil
	return nil
}

func (m *Module) Service() Service {
	if m == nil || m.servicePtr == nil {
		return nil
	}
	return m.servicePtr
}

func (m *Module) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev.Context(), ev.ID(), ev.Data())
	setMagicResult(result, response, err)
}

func (m *Module) executeCommand(ctx context.Context, topic string, data any) (resultevents.ResultCommandResult, error) {
	if m == nil || m.servicePtr == nil {
		return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
	}
	switch topic {
	case resultevents.TopicResultCommandCreate:
		cmd, ok := data.(resultevents.CreateResultCommand)
		if !ok || cmd.Result == nil {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.Result.WorkspaceID)
		if err := m.servicePtr.CreateResult(ctx, cmd.Result); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		m.publishResultChanged(ctx, cmd.Result)
		return resultevents.ResultCommandResult{}, nil
	case resultevents.TopicResultCommandGet:
		cmd, ok := data.(resultevents.GetResultCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.servicePtr.GetResult(ctx, cmd.ResultID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Result: item}, nil
	case resultevents.TopicResultCommandListByWorkspace:
		cmd, ok := data.(resultevents.ListResultsByWorkspaceCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.WorkspaceID)
		items, err := m.servicePtr.ListResultsByWorkspace(ctx, cmd.WorkspaceID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Results: items}, nil
	case resultevents.TopicResultCommandListByRun:
		cmd, ok := data.(resultevents.ListResultsByRunCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.servicePtr.ListResultsByRun(ctx, cmd.RunID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Results: items}, nil
	case resultevents.TopicResultCommandUpdate:
		cmd, ok := data.(resultevents.UpdateResultCommand)
		if !ok || cmd.Result == nil {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.Result.WorkspaceID)
		if err := m.servicePtr.UpdateResult(ctx, cmd.Result); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		m.publishResultChanged(ctx, cmd.Result)
		return resultevents.ResultCommandResult{}, nil
	case resultevents.TopicResultCommandDelete:
		cmd, ok := data.(resultevents.DeleteResultCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.DeleteResult(ctx, cmd.ResultID); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{}, nil
	case resultevents.TopicArtifactCommandCreate:
		cmd, ok := data.(resultevents.CreateArtifactCommand)
		if !ok || cmd.Artifact == nil {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.Artifact.WorkspaceID)
		if err := m.servicePtr.CreateArtifact(ctx, cmd.Artifact); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		m.publishArtifactCreated(ctx, cmd.Artifact)
		return resultevents.ResultCommandResult{}, nil
	case resultevents.TopicArtifactCommandGet:
		cmd, ok := data.(resultevents.GetArtifactCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.servicePtr.GetArtifact(ctx, cmd.ArtifactID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Artifact: item}, nil
	case resultevents.TopicArtifactCommandListByWorkspace:
		cmd, ok := data.(resultevents.ListArtifactsByWorkspaceCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.WorkspaceID)
		items, err := m.servicePtr.ListArtifactsByWorkspace(ctx, cmd.WorkspaceID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Artifacts: items}, nil
	case resultevents.TopicArtifactCommandListByRun:
		cmd, ok := data.(resultevents.ListArtifactsByRunCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.servicePtr.ListArtifactsByRun(ctx, cmd.RunID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Artifacts: items}, nil
	case resultevents.TopicArtifactCommandListByResult:
		cmd, ok := data.(resultevents.ListArtifactsByResultCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.servicePtr.ListArtifactsByResult(ctx, cmd.ResultID)
		if err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{Artifacts: items}, nil
	case resultevents.TopicArtifactCommandUpdate:
		cmd, ok := data.(resultevents.UpdateArtifactCommand)
		if !ok || cmd.Artifact == nil {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		ctx = resultCommandContext(ctx, cmd.Artifact.WorkspaceID)
		if err := m.servicePtr.UpdateArtifact(ctx, cmd.Artifact); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{}, nil
	case resultevents.TopicArtifactCommandDelete:
		cmd, ok := data.(resultevents.DeleteArtifactCommand)
		if !ok {
			return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.servicePtr.DeleteArtifact(ctx, cmd.ArtifactID); err != nil {
			return resultevents.ResultCommandResult{}, err
		}
		return resultevents.ResultCommandResult{}, nil
	default:
		return resultevents.ResultCommandResult{}, common.ErrInvalidArgument
	}
}

func (m *Module) publishResultChanged(ctx context.Context, item *resultmodel.Result) {
	if m == nil || m.eventHub == nil || item == nil {
		return
	}
	topic := resultNotificationTopic(item.State)
	event := magicevent.NewEventWithContext(topic, ModuleID, resultevents.NotificationDestination, magicevent.NewHeader(), ctx, resultevents.ResultChangedDataFromModel(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(topic, workorchevents.EventHubLaneScope{
		SubjectType: "result", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID),
	}))
	m.eventHub.Post(event)
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

func (m *Module) publishArtifactCreated(ctx context.Context, item *resultmodel.Artifact) {
	if m == nil || m.eventHub == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(resultevents.TopicArtifactCreated, ModuleID, resultevents.NotificationDestination, magicevent.NewHeader(), ctx, resultevents.ArtifactCreatedDataFromModel(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(resultevents.TopicArtifactCreated, workorchevents.EventHubLaneScope{
		SubjectType: "artifact", SubjectID: string(item.ID), WorkspaceID: string(item.WorkspaceID), RunID: string(item.RunID), StepID: string(item.StepID),
	}))
	m.eventHub.Post(event)
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
