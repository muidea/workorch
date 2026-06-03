package result

import (
	"context"

	resultbiz "github.com/muidea/workorch/internal/modules/kernel/result/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	resultservice "github.com/muidea/workorch/internal/modules/kernel/result/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = resultservice.Service

type Dependencies struct {
	Results   repository.ResultRepository
	Artifacts repository.ArtifactRepository
	EventHub  events.Hub
}

type Module struct {
	bizPtr     *resultbiz.Service
	servicePtr *resultservice.Module
	eventHub   events.Hub
	unsub      []func()
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
	if len(m.unsub) > 0 {
		return nil
	}
	m.unsub = append(m.unsub,
		m.eventHub.Subscribe(events.TopicResultCommandCreate, m.handleResultCommand),
		m.eventHub.Subscribe(events.TopicResultCommandGet, m.handleResultCommand),
		m.eventHub.Subscribe(events.TopicResultCommandListByWorkspace, m.handleResultCommand),
		m.eventHub.Subscribe(events.TopicResultCommandListByRun, m.handleResultCommand),
		m.eventHub.Subscribe(events.TopicResultCommandUpdate, m.handleResultCommand),
		m.eventHub.Subscribe(events.TopicResultCommandDelete, m.handleResultCommand),
		m.eventHub.Subscribe(events.TopicArtifactCommandCreate, m.handleArtifactCommand),
		m.eventHub.Subscribe(events.TopicArtifactCommandGet, m.handleArtifactCommand),
		m.eventHub.Subscribe(events.TopicArtifactCommandListByWorkspace, m.handleArtifactCommand),
		m.eventHub.Subscribe(events.TopicArtifactCommandListByRun, m.handleArtifactCommand),
		m.eventHub.Subscribe(events.TopicArtifactCommandListByResult, m.handleArtifactCommand),
		m.eventHub.Subscribe(events.TopicArtifactCommandUpdate, m.handleArtifactCommand),
		m.eventHub.Subscribe(events.TopicArtifactCommandDelete, m.handleArtifactCommand),
	)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	for idx := len(m.unsub) - 1; idx >= 0; idx-- {
		if m.unsub[idx] != nil {
			m.unsub[idx]()
		}
	}
	m.unsub = nil
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

func (m *Module) handleResultCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	switch envelope.Topic {
	case events.TopicResultCommandCreate:
		item, err := resultFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.servicePtr.CreateResult(ctx, item); err != nil {
			return err
		}
		return resultCommandResponse(envelope, nil)
	case events.TopicResultCommandGet:
		item, err := m.servicePtr.GetResult(ctx, common.ResultID(stringPayload(envelope.Payload, events.PayloadKeyResultID)))
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyResult: item})
	case events.TopicResultCommandListByWorkspace:
		items, err := m.servicePtr.ListResultsByWorkspace(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case events.TopicResultCommandListByRun:
		items, err := m.servicePtr.ListResultsByRun(ctx, common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID)))
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case events.TopicResultCommandUpdate:
		item, err := resultFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.servicePtr.UpdateResult(ctx, item); err != nil {
			return err
		}
		return resultCommandResponse(envelope, nil)
	case events.TopicResultCommandDelete:
		if err := m.servicePtr.DeleteResult(ctx, common.ResultID(stringPayload(envelope.Payload, events.PayloadKeyResultID))); err != nil {
			return err
		}
		return resultCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
}

func (m *Module) handleArtifactCommand(ctx context.Context, envelope events.Envelope) error {
	if m == nil || m.servicePtr == nil {
		return common.ErrInvalidArgument
	}
	switch envelope.Topic {
	case events.TopicArtifactCommandCreate:
		item, err := artifactFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.servicePtr.CreateArtifact(ctx, item); err != nil {
			return err
		}
		return resultCommandResponse(envelope, nil)
	case events.TopicArtifactCommandGet:
		item, err := m.servicePtr.GetArtifact(ctx, common.ArtifactID(stringPayload(envelope.Payload, events.PayloadKeyArtifactID)))
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyArtifact: item})
	case events.TopicArtifactCommandListByWorkspace:
		items, err := m.servicePtr.ListArtifactsByWorkspace(ctx, common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID)))
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case events.TopicArtifactCommandListByRun:
		items, err := m.servicePtr.ListArtifactsByRun(ctx, common.RunID(stringPayload(envelope.Payload, events.PayloadKeyRunID)))
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case events.TopicArtifactCommandListByResult:
		items, err := m.servicePtr.ListArtifactsByResult(ctx, common.ResultID(stringPayload(envelope.Payload, events.PayloadKeyResultID)))
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case events.TopicArtifactCommandUpdate:
		item, err := artifactFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.servicePtr.UpdateArtifact(ctx, item); err != nil {
			return err
		}
		return resultCommandResponse(envelope, nil)
	case events.TopicArtifactCommandDelete:
		if err := m.servicePtr.DeleteArtifact(ctx, common.ArtifactID(stringPayload(envelope.Payload, events.PayloadKeyArtifactID))); err != nil {
			return err
		}
		return resultCommandResponse(envelope, nil)
	default:
		return common.ErrInvalidArgument
	}
}

func resultFromPayload(payload map[string]any) (*resultmodel.Result, error) {
	item, ok := payload[events.PayloadKeyResult].(*resultmodel.Result)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func artifactFromPayload(payload map[string]any) (*resultmodel.Artifact, error) {
	item, ok := payload[events.PayloadKeyArtifact].(*resultmodel.Artifact)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func resultCommandResponse(request events.Envelope, payload map[string]any) error {
	return events.Response(events.Envelope{
		Topic:        request.Topic + "/response",
		SourceModule: ModuleID,
		SubjectType:  request.SubjectType,
		SubjectID:    request.SubjectID,
		WorkspaceID:  request.WorkspaceID,
		RunID:        request.RunID,
		StepID:       request.StepID,
		Payload:      payload,
	})
}
