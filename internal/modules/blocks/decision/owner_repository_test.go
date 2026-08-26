package decision

import "github.com/muidea/workorch/internal/pkg/repository"

type decisionTestBoundTickets struct {
	repository.DecisionTicketRepository
	binding repository.OwnerRepositoryBinding
}

func (r decisionTestBoundTickets) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type decisionTestBoundResolutions struct {
	repository.DecisionResolutionRepository
	binding repository.OwnerRepositoryBinding
}

func (r decisionTestBoundResolutions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type decisionTestBoundDeletions struct {
	repository.DecisionWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r decisionTestBoundDeletions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type decisionTestBackend struct {
	binding     repository.OwnerRepositoryBinding
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	deletions   repository.DecisionWorkspaceDeletionRepository
}

func (b decisionTestBackend) DecisionOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b decisionTestBackend) DecisionOwnerTickets() repository.DecisionTicketRepository {
	return b.tickets
}
func (b decisionTestBackend) DecisionOwnerResolutions() repository.DecisionResolutionRepository {
	return b.resolutions
}
func (b decisionTestBackend) DecisionOwnerWorkspaceDeletions() repository.DecisionWorkspaceDeletionRepository {
	return b.deletions
}

func decisionTestOwnerRepositories(tickets repository.DecisionTicketRepository, resolutions repository.DecisionResolutionRepository, deletions repository.DecisionWorkspaceDeletionRepository) repository.DecisionOwnerRepositories {
	binding := repository.NewDecisionOwnerRepositoryBinding()
	owner, err := repository.NewDecisionOwnerRepositories(decisionTestBackend{
		binding:     binding,
		tickets:     decisionTestBoundTickets{DecisionTicketRepository: tickets, binding: binding},
		resolutions: decisionTestBoundResolutions{DecisionResolutionRepository: resolutions, binding: binding},
		deletions:   decisionTestBoundDeletions{DecisionWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}
