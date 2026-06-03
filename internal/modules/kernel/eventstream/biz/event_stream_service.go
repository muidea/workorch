package eventstream

import (
	"context"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	repo          repository.EventRecordRepository
	eventHub      events.Hub
	unsubscribers []func()
}

func NewService(repo repository.EventRecordRepository, eventHub events.Hub) *Service {
	return &Service{repo: repo, eventHub: eventHub}
}

func (s *Service) Start() error {
	if s == nil || s.repo == nil || s.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if len(s.unsubscribers) > 0 {
		return nil
	}
	s.unsubscribers = events.SubscribeWorkorch(s.eventHub, func(ctx context.Context, event events.WorkorchEvent) error {
		record := eventRecordFromEnvelope(event.Envelope())
		if err := s.repo.Append(ctx, record); err != nil && err != common.ErrConflict {
			return err
		}
		return nil
	})
	return nil
}

func (s *Service) Stop() {
	if s == nil {
		return
	}
	unsubscribers := s.unsubscribers
	s.unsubscribers = nil
	for _, unsubscribe := range unsubscribers {
		if unsubscribe != nil {
			unsubscribe()
		}
	}
}

func (s *Service) ListEvents(ctx context.Context, query repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	if s == nil || s.repo == nil {
		return nil, common.ErrInvalidArgument
	}
	return s.repo.List(ctx, query)
}

func (s *Service) PurgeWorkspaceEvents(ctx context.Context, workspaceID common.WorkspaceID) (int, error) {
	if s == nil || s.repo == nil || workspaceID == "" {
		return 0, common.ErrInvalidArgument
	}
	items, err := s.repo.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID})
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := s.repo.Delete(ctx, item.ID); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func eventRecordFromEnvelope(envelope events.Envelope) *eventstreammodel.EventRecord {
	return &eventstreammodel.EventRecord{
		ID:             common.EventRecordID(envelope.ID),
		Version:        envelope.Version,
		Topic:          envelope.Topic,
		OccurredAt:     envelope.OccurredAt,
		SourceModule:   envelope.SourceModule,
		SubjectType:    envelope.SubjectType,
		SubjectID:      envelope.SubjectID,
		WorkspaceID:    common.WorkspaceID(envelope.WorkspaceID),
		RunID:          common.RunID(envelope.RunID),
		StepID:         common.StepID(envelope.StepID),
		Revision:       envelope.Revision,
		Payload:        clonePayload(envelope.Payload),
		PayloadSummary: eventPayloadSummary(envelope.Payload),
	}
}

func eventPayloadSummary(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	return map[string]any{"fieldCount": len(payload)}
}

func clonePayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
