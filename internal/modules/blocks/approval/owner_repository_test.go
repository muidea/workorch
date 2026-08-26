package approval

import "github.com/muidea/workorch/internal/pkg/repository"

type approvalTestBoundApprovals struct {
	repository.ApprovalRepository
	binding repository.OwnerRepositoryBinding
}

func (r approvalTestBoundApprovals) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type approvalTestBoundDeletions struct {
	repository.ApprovalWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r approvalTestBoundDeletions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type approvalTestBackend struct {
	binding   repository.OwnerRepositoryBinding
	approvals repository.ApprovalRepository
	deletions repository.ApprovalWorkspaceDeletionRepository
}

func (b approvalTestBackend) ApprovalOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b approvalTestBackend) ApprovalOwnerApprovals() repository.ApprovalRepository {
	return b.approvals
}
func (b approvalTestBackend) ApprovalOwnerWorkspaceDeletions() repository.ApprovalWorkspaceDeletionRepository {
	return b.deletions
}

func approvalTestOwnerRepositories(approvals repository.ApprovalRepository, deletions repository.ApprovalWorkspaceDeletionRepository) repository.ApprovalOwnerRepositories {
	binding := repository.NewApprovalOwnerRepositoryBinding()
	owner, err := repository.NewApprovalOwnerRepositories(approvalTestBackend{
		binding:   binding,
		approvals: approvalTestBoundApprovals{ApprovalRepository: approvals, binding: binding},
		deletions: approvalTestBoundDeletions{ApprovalWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}
