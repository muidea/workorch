package appservice

import "github.com/muidea/workorch/internal/pkg/repository"

type appserviceTestBoundDecisionTickets struct {
	repository.DecisionTicketRepository
	binding repository.OwnerRepositoryBinding
}

func (r appserviceTestBoundDecisionTickets) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type appserviceTestBoundDecisionResolutions struct {
	repository.DecisionResolutionRepository
	binding repository.OwnerRepositoryBinding
}

func (r appserviceTestBoundDecisionResolutions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type appserviceTestBoundDecisionDeletions struct {
	repository.DecisionWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r appserviceTestBoundDecisionDeletions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type appserviceTestDecisionBackend struct {
	binding     repository.OwnerRepositoryBinding
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	deletions   repository.DecisionWorkspaceDeletionRepository
}

func (b appserviceTestDecisionBackend) DecisionOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b appserviceTestDecisionBackend) DecisionOwnerTickets() repository.DecisionTicketRepository {
	return b.tickets
}
func (b appserviceTestDecisionBackend) DecisionOwnerResolutions() repository.DecisionResolutionRepository {
	return b.resolutions
}
func (b appserviceTestDecisionBackend) DecisionOwnerWorkspaceDeletions() repository.DecisionWorkspaceDeletionRepository {
	return b.deletions
}
func appserviceTestDecisionOwnerRepositories(tickets repository.DecisionTicketRepository, resolutions repository.DecisionResolutionRepository, deletions repository.DecisionWorkspaceDeletionRepository) repository.DecisionOwnerRepositories {
	binding := repository.NewDecisionOwnerRepositoryBinding()
	owner, err := repository.NewDecisionOwnerRepositories(appserviceTestDecisionBackend{
		binding:     binding,
		tickets:     appserviceTestBoundDecisionTickets{DecisionTicketRepository: tickets, binding: binding},
		resolutions: appserviceTestBoundDecisionResolutions{DecisionResolutionRepository: resolutions, binding: binding},
		deletions:   appserviceTestBoundDecisionDeletions{DecisionWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}

type appserviceTestBoundApprovals struct {
	repository.ApprovalRepository
	binding repository.OwnerRepositoryBinding
}

func (r appserviceTestBoundApprovals) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type appserviceTestBoundApprovalDeletions struct {
	repository.ApprovalWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r appserviceTestBoundApprovalDeletions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type appserviceTestApprovalBackend struct {
	binding   repository.OwnerRepositoryBinding
	approvals repository.ApprovalRepository
	deletions repository.ApprovalWorkspaceDeletionRepository
}

func (b appserviceTestApprovalBackend) ApprovalOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b appserviceTestApprovalBackend) ApprovalOwnerApprovals() repository.ApprovalRepository {
	return b.approvals
}
func (b appserviceTestApprovalBackend) ApprovalOwnerWorkspaceDeletions() repository.ApprovalWorkspaceDeletionRepository {
	return b.deletions
}
func appserviceTestApprovalOwnerRepositories(approvals repository.ApprovalRepository, deletions repository.ApprovalWorkspaceDeletionRepository) repository.ApprovalOwnerRepositories {
	binding := repository.NewApprovalOwnerRepositoryBinding()
	owner, err := repository.NewApprovalOwnerRepositories(appserviceTestApprovalBackend{
		binding:   binding,
		approvals: appserviceTestBoundApprovals{ApprovalRepository: approvals, binding: binding},
		deletions: appserviceTestBoundApprovalDeletions{ApprovalWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}
