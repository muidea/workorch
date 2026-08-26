package eventstream

import "github.com/muidea/workorch/internal/pkg/repository"

type eventStreamBizTestBoundRecords struct {
	repository.EventRecordRepository
	binding repository.OwnerRepositoryBinding
}

func (r eventStreamBizTestBoundRecords) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type eventStreamBizTestBoundDeletions struct {
	repository.EventStreamWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r eventStreamBizTestBoundDeletions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type eventStreamBizTestBoundPurger struct {
	repository.EventStreamWorkspacePurger
	binding repository.OwnerRepositoryBinding
}

func (r eventStreamBizTestBoundPurger) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type eventStreamBizTestBackend struct {
	binding   repository.OwnerRepositoryBinding
	records   repository.EventRecordRepository
	deletions repository.EventStreamWorkspaceDeletionRepository
	purger    repository.EventStreamWorkspacePurger
}

func (b eventStreamBizTestBackend) EventStreamOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b eventStreamBizTestBackend) EventStreamOwnerEventRecords() repository.EventRecordRepository {
	return b.records
}
func (b eventStreamBizTestBackend) EventStreamOwnerWorkspaceDeletions() repository.EventStreamWorkspaceDeletionRepository {
	return b.deletions
}
func (b eventStreamBizTestBackend) EventStreamOwnerWorkspacePurger() repository.EventStreamWorkspacePurger {
	return b.purger
}

func eventStreamBizTestOwnerRepositories(records repository.EventRecordRepository, deletions repository.EventStreamWorkspaceDeletionRepository, purger repository.EventStreamWorkspacePurger) repository.EventStreamOwnerRepositories {
	binding := repository.NewEventStreamOwnerRepositoryBinding()
	owner, err := repository.NewEventStreamOwnerRepositories(eventStreamBizTestBackend{
		binding:   binding,
		records:   eventStreamBizTestBoundRecords{EventRecordRepository: records, binding: binding},
		deletions: eventStreamBizTestBoundDeletions{EventStreamWorkspaceDeletionRepository: deletions, binding: binding},
		purger:    eventStreamBizTestBoundPurger{EventStreamWorkspacePurger: purger, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}
