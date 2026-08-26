package repository_test

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

type resultBackend struct {
	binding   repository.OwnerRepositoryBinding
	results   repository.ResultRepository
	artifacts repository.ArtifactRepository
	deletions repository.ResultWorkspaceDeletionRepository
}

func (b resultBackend) ResultOwnerBinding() repository.OwnerRepositoryBinding { return b.binding }
func (b resultBackend) ResultOwnerResults() repository.ResultRepository       { return b.results }
func (b resultBackend) ResultOwnerArtifacts() repository.ArtifactRepository   { return b.artifacts }
func (b resultBackend) ResultOwnerWorkspaceDeletions() repository.ResultWorkspaceDeletionRepository {
	return b.deletions
}

type decisionBackend struct {
	binding     repository.OwnerRepositoryBinding
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
	deletions   repository.DecisionWorkspaceDeletionRepository
}

func (b decisionBackend) DecisionOwnerBinding() repository.OwnerRepositoryBinding { return b.binding }
func (b decisionBackend) DecisionOwnerTickets() repository.DecisionTicketRepository {
	return b.tickets
}
func (b decisionBackend) DecisionOwnerResolutions() repository.DecisionResolutionRepository {
	return b.resolutions
}
func (b decisionBackend) DecisionOwnerWorkspaceDeletions() repository.DecisionWorkspaceDeletionRepository {
	return b.deletions
}

type approvalBackend struct {
	binding   repository.OwnerRepositoryBinding
	approvals repository.ApprovalRepository
	deletions repository.ApprovalWorkspaceDeletionRepository
}

func (b approvalBackend) ApprovalOwnerBinding() repository.OwnerRepositoryBinding { return b.binding }
func (b approvalBackend) ApprovalOwnerApprovals() repository.ApprovalRepository   { return b.approvals }
func (b approvalBackend) ApprovalOwnerWorkspaceDeletions() repository.ApprovalWorkspaceDeletionRepository {
	return b.deletions
}

type memoryBackend struct {
	binding   repository.OwnerRepositoryBinding
	items     repository.MemoryItemRepository
	traces    repository.MemoryRecallTraceRepository
	deletions repository.MemoryWorkspaceDeletionRepository
}

func (b memoryBackend) MemoryOwnerBinding() repository.OwnerRepositoryBinding { return b.binding }
func (b memoryBackend) MemoryOwnerItems() repository.MemoryItemRepository     { return b.items }
func (b memoryBackend) MemoryOwnerRecallTraces() repository.MemoryRecallTraceRepository {
	return b.traces
}
func (b memoryBackend) MemoryOwnerWorkspaceDeletions() repository.MemoryWorkspaceDeletionRepository {
	return b.deletions
}

type eventStreamBackend struct {
	binding   repository.OwnerRepositoryBinding
	records   repository.EventRecordRepository
	deletions repository.EventStreamWorkspaceDeletionRepository
	purger    repository.EventStreamWorkspacePurger
}

func (b eventStreamBackend) EventStreamOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b eventStreamBackend) EventStreamOwnerEventRecords() repository.EventRecordRepository {
	return b.records
}
func (b eventStreamBackend) EventStreamOwnerWorkspaceDeletions() repository.EventStreamWorkspaceDeletionRepository {
	return b.deletions
}
func (b eventStreamBackend) EventStreamOwnerWorkspacePurger() repository.EventStreamWorkspacePurger {
	return b.purger
}

func TestResourceOwnerRepositoryBundlesRejectZeroAndUnboundFakeBackends(t *testing.T) {
	for name, validate := range map[string]func() error{
		"result zero":      func() error { return (repository.ResultOwnerRepositories{}).Validate() },
		"decision zero":    func() error { return (repository.DecisionOwnerRepositories{}).Validate() },
		"approval zero":    func() error { return (repository.ApprovalOwnerRepositories{}).Validate() },
		"memory zero":      func() error { return (repository.MemoryOwnerRepositories{}).Validate() },
		"eventstream zero": func() error { return (repository.EventStreamOwnerRepositories{}).Validate() },
	} {
		if err := validate(); !errors.Is(err, common.ErrInvalidArgument) {
			t.Errorf("%s validation = %v, want invalid argument", name, err)
		}
	}

	if _, err := repository.NewResultOwnerRepositories(resultBackend{
		binding: repository.NewResultOwnerRepositoryBinding(), results: memory.NewResultRepository(),
		artifacts: memory.NewArtifactRepository(), deletions: memory.NewResultWorkspaceDeletionRepository(),
	}); !errors.Is(err, common.ErrConflict) {
		t.Errorf("unbound fake Result backend = %v, want conflict", err)
	}
	if _, err := repository.NewDecisionOwnerRepositories(decisionBackend{
		binding: repository.NewDecisionOwnerRepositoryBinding(), tickets: memory.NewDecisionTicketRepository(),
		resolutions: memory.NewDecisionResolutionRepository(), deletions: memory.NewDecisionWorkspaceDeletionRepository(),
	}); !errors.Is(err, common.ErrConflict) {
		t.Errorf("unbound fake Decision backend = %v, want conflict", err)
	}
	if _, err := repository.NewApprovalOwnerRepositories(approvalBackend{
		binding: repository.NewApprovalOwnerRepositoryBinding(), approvals: memory.NewApprovalRepository(),
		deletions: memory.NewApprovalWorkspaceDeletionRepository(),
	}); !errors.Is(err, common.ErrConflict) {
		t.Errorf("unbound fake Approval backend = %v, want conflict", err)
	}
	if _, err := repository.NewMemoryOwnerRepositories(memoryBackend{
		binding: repository.NewMemoryOwnerRepositoryBinding(), items: memory.NewMemoryItemRepository(),
		traces: memory.NewMemoryRecallTraceRepository(), deletions: memory.NewMemoryWorkspaceDeletionRepository(),
	}); !errors.Is(err, common.ErrConflict) {
		t.Errorf("unbound fake Memory backend = %v, want conflict", err)
	}
	records := memory.NewEventRecordRepository()
	if _, err := repository.NewEventStreamOwnerRepositories(eventStreamBackend{
		binding: repository.NewEventStreamOwnerRepositoryBinding(), records: records,
		deletions: memory.NewEventStreamWorkspaceDeletionRepository(), purger: records,
	}); !errors.Is(err, common.ErrConflict) {
		t.Errorf("unbound fake Eventstream backend = %v, want conflict", err)
	}
}

func TestResourceOwnerRepositoryBundlesRejectMixedMemorySets(t *testing.T) {
	resultFirst, resultSecond := memory.NewResultOwnerRepositorySet(), memory.NewResultOwnerRepositorySet()
	if _, err := repository.NewResultOwnerRepositories(resultBackend{
		binding: resultFirst.ResultOwnerBinding(), results: resultFirst.Results,
		artifacts: resultSecond.Artifacts, deletions: resultFirst.WorkspaceDeletions,
	}); !errors.Is(err, common.ErrConflict) {
		t.Errorf("mixed Result set = %v, want conflict", err)
	}

	decisionFirst, decisionSecond := memory.NewDecisionOwnerRepositorySet(), memory.NewDecisionOwnerRepositorySet()
	if _, err := repository.NewDecisionOwnerRepositories(decisionBackend{
		binding: decisionFirst.DecisionOwnerBinding(), tickets: decisionFirst.Tickets,
		resolutions: decisionSecond.Resolutions, deletions: decisionFirst.WorkspaceDeletions,
	}); !errors.Is(err, common.ErrConflict) {
		t.Errorf("mixed Decision set = %v, want conflict", err)
	}

	approvalFirst, approvalSecond := memory.NewApprovalOwnerRepositorySet(), memory.NewApprovalOwnerRepositorySet()
	if _, err := repository.NewApprovalOwnerRepositories(approvalBackend{
		binding: approvalFirst.ApprovalOwnerBinding(), approvals: approvalSecond.Approvals,
		deletions: approvalFirst.WorkspaceDeletions,
	}); !errors.Is(err, common.ErrConflict) {
		t.Errorf("mixed Approval set = %v, want conflict", err)
	}

	memoryFirst, memorySecond := memory.NewMemoryOwnerRepositorySet(), memory.NewMemoryOwnerRepositorySet()
	if _, err := repository.NewMemoryOwnerRepositories(memoryBackend{
		binding: memoryFirst.MemoryOwnerBinding(), items: memoryFirst.Items,
		traces: memorySecond.RecallTraces, deletions: memoryFirst.WorkspaceDeletions,
	}); !errors.Is(err, common.ErrConflict) {
		t.Errorf("mixed Memory set = %v, want conflict", err)
	}

	eventFirst, eventSecond := memory.NewEventStreamOwnerRepositorySet(), memory.NewEventStreamOwnerRepositorySet()
	if _, err := repository.NewEventStreamOwnerRepositories(eventStreamBackend{
		binding: eventFirst.EventStreamOwnerBinding(), records: eventFirst.EventRecords,
		deletions: eventSecond.WorkspaceDeletions, purger: eventFirst.EventRecords,
	}); !errors.Is(err, common.ErrConflict) {
		t.Errorf("mixed Eventstream set = %v, want conflict", err)
	}
}

func TestResourceOwnerRepositoryBundlesValidateForMemoryAndReopenedFileBackends(t *testing.T) {
	memoryBundles := []interface{ Validate() error }{
		memory.NewResultOwnerRepositorySet().OwnerRepositories(),
		memory.NewDecisionOwnerRepositorySet().OwnerRepositories(),
		memory.NewApprovalOwnerRepositorySet().OwnerRepositories(),
		memory.NewMemoryOwnerRepositorySet().OwnerRepositories(),
		memory.NewEventStreamOwnerRepositorySet().OwnerRepositories(),
	}
	for idx, bundle := range memoryBundles {
		if err := bundle.Validate(); err != nil {
			t.Fatalf("memory bundle %d validation: %v", idx, err)
		}
	}

	root := t.TempDir()
	for attempt := 0; attempt < 2; attempt++ {
		file := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
		fileBundles := []interface{ Validate() error }{
			file.ResultOwner, file.DecisionOwner, file.ApprovalOwner, file.MemoryOwner, file.EventStreamOwner,
		}
		for idx, bundle := range fileBundles {
			if err := bundle.Validate(); err != nil {
				t.Fatalf("file attempt %d bundle %d validation: %v", attempt, idx, err)
			}
		}
	}
}
