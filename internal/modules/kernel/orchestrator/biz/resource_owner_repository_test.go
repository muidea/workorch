package orchestrator

import "github.com/muidea/workorch/internal/pkg/repository"

type testOwnerBoundResultRepository struct {
	repository.ResultRepository
	binding repository.OwnerRepositoryBinding
}

func (r testOwnerBoundResultRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type testOwnerBoundArtifactRepository struct {
	repository.ArtifactRepository
	binding repository.OwnerRepositoryBinding
}

func (r testOwnerBoundArtifactRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type testOwnerBoundResultDeletionRepository struct {
	repository.ResultWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r testOwnerBoundResultDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type testResultOwnerBackend struct {
	binding   repository.OwnerRepositoryBinding
	results   repository.ResultRepository
	artifacts repository.ArtifactRepository
	deletions repository.ResultWorkspaceDeletionRepository
}

func (b testResultOwnerBackend) ResultOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b testResultOwnerBackend) ResultOwnerResults() repository.ResultRepository { return b.results }
func (b testResultOwnerBackend) ResultOwnerArtifacts() repository.ArtifactRepository {
	return b.artifacts
}
func (b testResultOwnerBackend) ResultOwnerWorkspaceDeletions() repository.ResultWorkspaceDeletionRepository {
	return b.deletions
}
func testResultOwnerRepositories(results repository.ResultRepository, artifacts repository.ArtifactRepository, deletions repository.ResultWorkspaceDeletionRepository) repository.ResultOwnerRepositories {
	binding := repository.NewResultOwnerRepositoryBinding()
	owner, err := repository.NewResultOwnerRepositories(testResultOwnerBackend{
		binding:   binding,
		results:   testOwnerBoundResultRepository{ResultRepository: results, binding: binding},
		artifacts: testOwnerBoundArtifactRepository{ArtifactRepository: artifacts, binding: binding},
		deletions: testOwnerBoundResultDeletionRepository{ResultWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}

type testOwnerBoundDecisionTicketRepository struct {
	repository.DecisionTicketRepository
	binding repository.OwnerRepositoryBinding
}

func (r testOwnerBoundDecisionTicketRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type testOwnerBoundDecisionResolutionRepository struct {
	repository.DecisionResolutionRepository
	binding repository.OwnerRepositoryBinding
}

func (r testOwnerBoundDecisionResolutionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type testOwnerBoundDecisionDeletionRepository struct {
	repository.DecisionWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r testOwnerBoundDecisionDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type testDecisionOwnerBackend struct {
	binding     repository.OwnerRepositoryBinding
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	deletions   repository.DecisionWorkspaceDeletionRepository
}

func (b testDecisionOwnerBackend) DecisionOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b testDecisionOwnerBackend) DecisionOwnerTickets() repository.DecisionTicketRepository {
	return b.tickets
}
func (b testDecisionOwnerBackend) DecisionOwnerResolutions() repository.DecisionResolutionRepository {
	return b.resolutions
}
func (b testDecisionOwnerBackend) DecisionOwnerWorkspaceDeletions() repository.DecisionWorkspaceDeletionRepository {
	return b.deletions
}
func testDecisionOwnerRepositories(tickets repository.DecisionTicketRepository, resolutions repository.DecisionResolutionRepository, deletions repository.DecisionWorkspaceDeletionRepository) repository.DecisionOwnerRepositories {
	binding := repository.NewDecisionOwnerRepositoryBinding()
	owner, err := repository.NewDecisionOwnerRepositories(testDecisionOwnerBackend{
		binding:     binding,
		tickets:     testOwnerBoundDecisionTicketRepository{DecisionTicketRepository: tickets, binding: binding},
		resolutions: testOwnerBoundDecisionResolutionRepository{DecisionResolutionRepository: resolutions, binding: binding},
		deletions:   testOwnerBoundDecisionDeletionRepository{DecisionWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}

type testOwnerBoundApprovalRepository struct {
	repository.ApprovalRepository
	binding repository.OwnerRepositoryBinding
}

func (r testOwnerBoundApprovalRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type testOwnerBoundApprovalDeletionRepository struct {
	repository.ApprovalWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r testOwnerBoundApprovalDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type testApprovalOwnerBackend struct {
	binding   repository.OwnerRepositoryBinding
	approvals repository.ApprovalRepository
	deletions repository.ApprovalWorkspaceDeletionRepository
}

func (b testApprovalOwnerBackend) ApprovalOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b testApprovalOwnerBackend) ApprovalOwnerApprovals() repository.ApprovalRepository {
	return b.approvals
}
func (b testApprovalOwnerBackend) ApprovalOwnerWorkspaceDeletions() repository.ApprovalWorkspaceDeletionRepository {
	return b.deletions
}
func testApprovalOwnerRepositories(approvals repository.ApprovalRepository, deletions repository.ApprovalWorkspaceDeletionRepository) repository.ApprovalOwnerRepositories {
	binding := repository.NewApprovalOwnerRepositoryBinding()
	owner, err := repository.NewApprovalOwnerRepositories(testApprovalOwnerBackend{
		binding:   binding,
		approvals: testOwnerBoundApprovalRepository{ApprovalRepository: approvals, binding: binding},
		deletions: testOwnerBoundApprovalDeletionRepository{ApprovalWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}
