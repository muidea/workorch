package decision

import "github.com/muidea/workorch/internal/pkg/repository"

type decisionBizTestBoundTickets struct {
	repository.DecisionTicketRepository
	binding repository.OwnerRepositoryBinding
}

func (r decisionBizTestBoundTickets) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type decisionBizTestBoundResolutions struct {
	repository.DecisionResolutionRepository
	binding repository.OwnerRepositoryBinding
}

func (r decisionBizTestBoundResolutions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type decisionBizTestBoundDeletions struct {
	repository.DecisionWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r decisionBizTestBoundDeletions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type decisionBizTestBackend struct {
	binding     repository.OwnerRepositoryBinding
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	deletions   repository.DecisionWorkspaceDeletionRepository
}

func (b decisionBizTestBackend) DecisionOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b decisionBizTestBackend) DecisionOwnerTickets() repository.DecisionTicketRepository {
	return b.tickets
}
func (b decisionBizTestBackend) DecisionOwnerResolutions() repository.DecisionResolutionRepository {
	return b.resolutions
}
func (b decisionBizTestBackend) DecisionOwnerWorkspaceDeletions() repository.DecisionWorkspaceDeletionRepository {
	return b.deletions
}

func decisionBizTestOwnerRepositories(tickets repository.DecisionTicketRepository, resolutions repository.DecisionResolutionRepository, deletions repository.DecisionWorkspaceDeletionRepository) repository.DecisionOwnerRepositories {
	binding := repository.NewDecisionOwnerRepositoryBinding()
	owner, err := repository.NewDecisionOwnerRepositories(decisionBizTestBackend{
		binding:     binding,
		tickets:     decisionBizTestBoundTickets{DecisionTicketRepository: tickets, binding: binding},
		resolutions: decisionBizTestBoundResolutions{DecisionResolutionRepository: resolutions, binding: binding},
		deletions:   decisionBizTestBoundDeletions{DecisionWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}
