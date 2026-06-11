package result

import (
	"context"

	resultbiz "github.com/muidea/workorch/internal/modules/kernel/result/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"
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
		m.eventHub.Subscribe(resultevents.TopicResultCommandCreate, m.handleResultCommand),
		m.eventHub.Subscribe(resultevents.TopicResultCommandGet, m.handleResultCommand),
		m.eventHub.Subscribe(resultevents.TopicResultCommandListByWorkspace, m.handleResultCommand),
		m.eventHub.Subscribe(resultevents.TopicResultCommandListByRun, m.handleResultCommand),
		m.eventHub.Subscribe(resultevents.TopicResultCommandUpdate, m.handleResultCommand),
		m.eventHub.Subscribe(resultevents.TopicResultCommandDelete, m.handleResultCommand),
		m.eventHub.Subscribe(resultevents.TopicArtifactCommandCreate, m.handleArtifactCommand),
		m.eventHub.Subscribe(resultevents.TopicArtifactCommandGet, m.handleArtifactCommand),
		m.eventHub.Subscribe(resultevents.TopicArtifactCommandListByWorkspace, m.handleArtifactCommand),
		m.eventHub.Subscribe(resultevents.TopicArtifactCommandListByRun, m.handleArtifactCommand),
		m.eventHub.Subscribe(resultevents.TopicArtifactCommandListByResult, m.handleArtifactCommand),
		m.eventHub.Subscribe(resultevents.TopicArtifactCommandUpdate, m.handleArtifactCommand),
		m.eventHub.Subscribe(resultevents.TopicArtifactCommandDelete, m.handleArtifactCommand),
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
	if workspaceID := resultScopedWorkspaceID(envelope); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch envelope.Topic {
	case resultevents.TopicResultCommandCreate:
		cmd, err := createResultCommandFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.servicePtr.CreateResult(ctx, cmd.Result); err != nil {
			return err
		}
		return resultCommandResponse(envelope, nil)
	case resultevents.TopicResultCommandGet:
		cmd := getResultCommandFromPayload(envelope.Payload)
		item, err := m.servicePtr.GetResult(ctx, cmd.ResultID)
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyResult: item})
	case resultevents.TopicResultCommandListByWorkspace:
		cmd := listResultsByWorkspaceCommandFromPayload(envelope.Payload)
		items, err := m.servicePtr.ListResultsByWorkspace(ctx, cmd.WorkspaceID)
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case resultevents.TopicResultCommandListByRun:
		cmd := listResultsByRunCommandFromPayload(envelope.Payload)
		items, err := m.servicePtr.ListResultsByRun(ctx, cmd.RunID)
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case resultevents.TopicResultCommandUpdate:
		cmd, err := updateResultCommandFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.servicePtr.UpdateResult(ctx, cmd.Result); err != nil {
			return err
		}
		return resultCommandResponse(envelope, nil)
	case resultevents.TopicResultCommandDelete:
		cmd := deleteResultCommandFromPayload(envelope.Payload)
		if err := m.servicePtr.DeleteResult(ctx, cmd.ResultID); err != nil {
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
	if workspaceID := resultScopedWorkspaceID(envelope); workspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	}
	switch envelope.Topic {
	case resultevents.TopicArtifactCommandCreate:
		cmd, err := createArtifactCommandFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.servicePtr.CreateArtifact(ctx, cmd.Artifact); err != nil {
			return err
		}
		return resultCommandResponse(envelope, nil)
	case resultevents.TopicArtifactCommandGet:
		cmd := getArtifactCommandFromPayload(envelope.Payload)
		item, err := m.servicePtr.GetArtifact(ctx, cmd.ArtifactID)
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyArtifact: item})
	case resultevents.TopicArtifactCommandListByWorkspace:
		cmd := listArtifactsByWorkspaceCommandFromPayload(envelope.Payload)
		items, err := m.servicePtr.ListArtifactsByWorkspace(ctx, cmd.WorkspaceID)
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case resultevents.TopicArtifactCommandListByRun:
		cmd := listArtifactsByRunCommandFromPayload(envelope.Payload)
		items, err := m.servicePtr.ListArtifactsByRun(ctx, cmd.RunID)
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case resultevents.TopicArtifactCommandListByResult:
		cmd := listArtifactsByResultCommandFromPayload(envelope.Payload)
		items, err := m.servicePtr.ListArtifactsByResult(ctx, cmd.ResultID)
		if err != nil {
			return err
		}
		return resultCommandResponse(envelope, map[string]any{events.PayloadKeyItems: items})
	case resultevents.TopicArtifactCommandUpdate:
		cmd, err := updateArtifactCommandFromPayload(envelope.Payload)
		if err != nil {
			return err
		}
		if err := m.servicePtr.UpdateArtifact(ctx, cmd.Artifact); err != nil {
			return err
		}
		return resultCommandResponse(envelope, nil)
	case resultevents.TopicArtifactCommandDelete:
		cmd := deleteArtifactCommandFromPayload(envelope.Payload)
		if err := m.servicePtr.DeleteArtifact(ctx, cmd.ArtifactID); err != nil {
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

func createResultCommandFromPayload(payload map[string]any) (resultcontracts.CreateResultCommand, error) {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.CreateResultCommand); ok {
		if cmd.Result == nil {
			return resultcontracts.CreateResultCommand{}, common.ErrInvalidArgument
		}
		return cmd, nil
	}
	item, err := resultFromPayload(payload)
	if err != nil {
		return resultcontracts.CreateResultCommand{}, err
	}
	return resultcontracts.CreateResultCommand{Result: item}, nil
}

func getResultCommandFromPayload(payload map[string]any) resultcontracts.GetResultCommand {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.GetResultCommand); ok {
		return cmd
	}
	return resultcontracts.GetResultCommand{ResultID: common.ResultID(stringPayload(payload, events.PayloadKeyResultID))}
}

func listResultsByWorkspaceCommandFromPayload(payload map[string]any) resultcontracts.ListResultsByWorkspaceCommand {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.ListResultsByWorkspaceCommand); ok {
		return cmd
	}
	return resultcontracts.ListResultsByWorkspaceCommand{WorkspaceID: common.WorkspaceID(stringPayload(payload, events.PayloadKeyWorkspaceID))}
}

func listResultsByRunCommandFromPayload(payload map[string]any) resultcontracts.ListResultsByRunCommand {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.ListResultsByRunCommand); ok {
		return cmd
	}
	return resultcontracts.ListResultsByRunCommand{RunID: common.RunID(stringPayload(payload, events.PayloadKeyRunID))}
}

func updateResultCommandFromPayload(payload map[string]any) (resultcontracts.UpdateResultCommand, error) {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.UpdateResultCommand); ok {
		if cmd.Result == nil {
			return resultcontracts.UpdateResultCommand{}, common.ErrInvalidArgument
		}
		return cmd, nil
	}
	item, err := resultFromPayload(payload)
	if err != nil {
		return resultcontracts.UpdateResultCommand{}, err
	}
	return resultcontracts.UpdateResultCommand{Result: item}, nil
}

func deleteResultCommandFromPayload(payload map[string]any) resultcontracts.DeleteResultCommand {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.DeleteResultCommand); ok {
		return cmd
	}
	return resultcontracts.DeleteResultCommand{ResultID: common.ResultID(stringPayload(payload, events.PayloadKeyResultID))}
}

func artifactFromPayload(payload map[string]any) (*resultmodel.Artifact, error) {
	item, ok := payload[events.PayloadKeyArtifact].(*resultmodel.Artifact)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func createArtifactCommandFromPayload(payload map[string]any) (resultcontracts.CreateArtifactCommand, error) {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.CreateArtifactCommand); ok {
		if cmd.Artifact == nil {
			return resultcontracts.CreateArtifactCommand{}, common.ErrInvalidArgument
		}
		return cmd, nil
	}
	item, err := artifactFromPayload(payload)
	if err != nil {
		return resultcontracts.CreateArtifactCommand{}, err
	}
	return resultcontracts.CreateArtifactCommand{Artifact: item}, nil
}

func getArtifactCommandFromPayload(payload map[string]any) resultcontracts.GetArtifactCommand {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.GetArtifactCommand); ok {
		return cmd
	}
	return resultcontracts.GetArtifactCommand{ArtifactID: common.ArtifactID(stringPayload(payload, events.PayloadKeyArtifactID))}
}

func listArtifactsByWorkspaceCommandFromPayload(payload map[string]any) resultcontracts.ListArtifactsByWorkspaceCommand {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.ListArtifactsByWorkspaceCommand); ok {
		return cmd
	}
	return resultcontracts.ListArtifactsByWorkspaceCommand{WorkspaceID: common.WorkspaceID(stringPayload(payload, events.PayloadKeyWorkspaceID))}
}

func listArtifactsByRunCommandFromPayload(payload map[string]any) resultcontracts.ListArtifactsByRunCommand {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.ListArtifactsByRunCommand); ok {
		return cmd
	}
	return resultcontracts.ListArtifactsByRunCommand{RunID: common.RunID(stringPayload(payload, events.PayloadKeyRunID))}
}

func listArtifactsByResultCommandFromPayload(payload map[string]any) resultcontracts.ListArtifactsByResultCommand {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.ListArtifactsByResultCommand); ok {
		return cmd
	}
	return resultcontracts.ListArtifactsByResultCommand{ResultID: common.ResultID(stringPayload(payload, events.PayloadKeyResultID))}
}

func updateArtifactCommandFromPayload(payload map[string]any) (resultcontracts.UpdateArtifactCommand, error) {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.UpdateArtifactCommand); ok {
		if cmd.Artifact == nil {
			return resultcontracts.UpdateArtifactCommand{}, common.ErrInvalidArgument
		}
		return cmd, nil
	}
	item, err := artifactFromPayload(payload)
	if err != nil {
		return resultcontracts.UpdateArtifactCommand{}, err
	}
	return resultcontracts.UpdateArtifactCommand{Artifact: item}, nil
}

func deleteArtifactCommandFromPayload(payload map[string]any) resultcontracts.DeleteArtifactCommand {
	if cmd, ok := payload[events.PayloadKeyCommand].(resultcontracts.DeleteArtifactCommand); ok {
		return cmd
	}
	return resultcontracts.DeleteArtifactCommand{ArtifactID: common.ArtifactID(stringPayload(payload, events.PayloadKeyArtifactID))}
}

func stringPayload(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func resultScopedWorkspaceID(envelope events.Envelope) common.WorkspaceID {
	workspaceID := common.WorkspaceID(stringPayload(envelope.Payload, events.PayloadKeyWorkspaceID))
	if workspaceID == "" {
		workspaceID = common.WorkspaceID(envelope.WorkspaceID)
	}
	return workspaceID
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
