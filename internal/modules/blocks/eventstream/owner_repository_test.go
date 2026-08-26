package eventstream

import "github.com/muidea/workorch/internal/pkg/repository"

type eventStreamTestBoundRecords struct {
	repository.EventRecordRepository
	binding repository.OwnerRepositoryBinding
}

func (r eventStreamTestBoundRecords) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type eventStreamTestBoundDeletions struct {
	repository.EventStreamWorkspaceDeletionRepository
	binding repository.OwnerRepositoryBinding
}

func (r eventStreamTestBoundDeletions) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type eventStreamTestBoundPurger struct {
	repository.EventStreamWorkspacePurger
	binding repository.OwnerRepositoryBinding
}

func (r eventStreamTestBoundPurger) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type eventStreamTestBackend struct {
	binding   repository.OwnerRepositoryBinding
	records   repository.EventRecordRepository
	deletions repository.EventStreamWorkspaceDeletionRepository
	purger    repository.EventStreamWorkspacePurger
}

func (b eventStreamTestBackend) EventStreamOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b eventStreamTestBackend) EventStreamOwnerEventRecords() repository.EventRecordRepository {
	return b.records
}
func (b eventStreamTestBackend) EventStreamOwnerWorkspaceDeletions() repository.EventStreamWorkspaceDeletionRepository {
	return b.deletions
}
func (b eventStreamTestBackend) EventStreamOwnerWorkspacePurger() repository.EventStreamWorkspacePurger {
	return b.purger
}

func eventStreamTestOwnerRepositories(records repository.EventRecordRepository, deletions repository.EventStreamWorkspaceDeletionRepository, purger repository.EventStreamWorkspacePurger) repository.EventStreamOwnerRepositories {
	binding := repository.NewEventStreamOwnerRepositoryBinding()
	owner, err := repository.NewEventStreamOwnerRepositories(eventStreamTestBackend{
		binding:   binding,
		records:   eventStreamTestBoundRecords{EventRecordRepository: records, binding: binding},
		deletions: eventStreamTestBoundDeletions{EventStreamWorkspaceDeletionRepository: deletions, binding: binding},
		purger:    eventStreamTestBoundPurger{EventStreamWorkspacePurger: purger, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}
