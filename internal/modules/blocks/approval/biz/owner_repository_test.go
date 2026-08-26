package approval

import "github.com/muidea/workorch/internal/pkg/repository"

type approvalBizTestBoundApprovals struct {
	repository.ApprovalRepository
	binding repository.OwnerRepositoryBinding
}

func (r approvalBizTestBoundApprovals) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type approvalBizTestBoundDeletions struct {
	repository.ApprovalWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r approvalBizTestBoundDeletions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type approvalBizTestBackend struct {
	binding   repository.OwnerRepositoryBinding
	approvals repository.ApprovalRepository
	deletions repository.ApprovalWorkspaceDeletionRepository
}

func (b approvalBizTestBackend) ApprovalOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b approvalBizTestBackend) ApprovalOwnerApprovals() repository.ApprovalRepository {
	return b.approvals
}
func (b approvalBizTestBackend) ApprovalOwnerWorkspaceDeletions() repository.ApprovalWorkspaceDeletionRepository {
	return b.deletions
}

func approvalBizTestOwnerRepositories(approvals repository.ApprovalRepository, deletions repository.ApprovalWorkspaceDeletionRepository) repository.ApprovalOwnerRepositories {
	binding := repository.NewApprovalOwnerRepositoryBinding()
	owner, err := repository.NewApprovalOwnerRepositories(approvalBizTestBackend{
		binding:   binding,
		approvals: approvalBizTestBoundApprovals{ApprovalRepository: approvals, binding: binding},
		deletions: approvalBizTestBoundDeletions{ApprovalWorkspaceDeletionRepository: deletions, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}
