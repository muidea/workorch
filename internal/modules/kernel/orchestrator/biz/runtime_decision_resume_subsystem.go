package orchestrator

import (
	"context"

	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type decisionResumeSubsystem struct {
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
}

func newDecisionResumeSubsystem(
	tickets repository.DecisionTicketRepository,
	resolutions repository.DecisionResolutionRepository,
) *decisionResumeSubsystem {
	return &decisionResumeSubsystem{
		tickets:     tickets,
		resolutions: resolutions,
	}
}

func (s *decisionResumeSubsystem) resolvedDecisionForStep(
	ctx context.Context,
	runID common.RunID,
	stepID common.StepID,
) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	if s == nil || s.tickets == nil || s.resolutions == nil || runID == "" || stepID == "" {
		return nil, nil, common.ErrConflict
	}
	tickets, err := s.tickets.ListByRunID(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	for _, ticket := range tickets {
		if ticket == nil ||
			ticket.StepID != stepID ||
			ticket.State != common.DecisionTicketStateResolved {
			continue
		}
		resolution, err := s.resolutions.GetByTicketID(ctx, ticket.ID)
		if err != nil {
			return nil, nil, err
		}
		return ticket, resolution, nil
	}
	return nil, nil, common.ErrConflict
}

func (s *decisionResumeSubsystem) resolvedReplanDecisionForFailedRunStep(
	ctx context.Context,
	runID common.RunID,
	steps []*runModel.Step,
) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	if s == nil || runID == "" || s.tickets == nil || s.resolutions == nil {
		return nil, nil, common.ErrConflict
	}
	failedSteps := map[common.StepID]bool{}
	for _, step := range steps {
		if step == nil || step.ID == "" || step.State != common.StepStateFailed {
			continue
		}
		failedSteps[step.ID] = true
	}
	if len(failedSteps) == 0 {
		return nil, nil, common.ErrConflict
	}
	tickets, err := s.tickets.ListByRunID(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	var selectedTicket *decisionmodel.Ticket
	var selectedResolution *decisionmodel.Resolution
	for _, ticket := range tickets {
		if ticket == nil ||
			ticket.RunID != runID ||
			ticket.State != common.DecisionTicketStateResolved ||
			!failedSteps[ticket.StepID] {
			continue
		}
		resolution, err := s.resolutions.GetByTicketID(ctx, ticket.ID)
		if err != nil {
			return nil, nil, err
		}
		if effectiveDecisionResolutionType(ticket, resolution) != decisioncommon.ResolutionTypeReplan {
			continue
		}
		if selectedTicket == nil || resolvedDecisionTicketIsNewer(ticket, selectedTicket) {
			selectedTicket = ticket
			selectedResolution = resolution
		}
	}
	if selectedTicket == nil {
		return nil, nil, common.ErrConflict
	}
	return selectedTicket, selectedResolution, nil
}

func resolvedDecisionTicketIsNewer(candidate *decisionmodel.Ticket, current *decisionmodel.Ticket) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if candidate.UpdatedAt != "" || current.UpdatedAt != "" {
		if candidate.UpdatedAt != current.UpdatedAt {
			return candidate.UpdatedAt > current.UpdatedAt
		}
	}
	if candidate.CreatedAt != "" || current.CreatedAt != "" {
		if candidate.CreatedAt != current.CreatedAt {
			return candidate.CreatedAt > current.CreatedAt
		}
	}
	return candidate.ID > current.ID
}
