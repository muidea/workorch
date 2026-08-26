package repository

import "github.com/muidea/workorch/internal/pkg/common"

type ownerRepositoryFamily uint8

const (
	ownerRepositoryFamilyResult ownerRepositoryFamily = iota + 1
	ownerRepositoryFamilyDecision
	ownerRepositoryFamilyApproval
	ownerRepositoryFamilyMemory
	ownerRepositoryFamilyEventStream
)

// OwnerRepositoryBinding is an opaque, process-local identity minted by a
// backend factory for one resource-owner repository set. It carries no
// repository capability and is used only to reject independently assembled or
// mixed-backend surfaces before an owner starts.
type OwnerRepositoryBinding struct {
	token  *ownerRepositoryBindingToken
	family ownerRepositoryFamily
}

// Keep the token non-zero-sized. Distinct zero-sized allocations are allowed
// to share an address and would defeat the same-set guard.
type ownerRepositoryBindingToken struct{ marker byte }

func newOwnerRepositoryBinding(family ownerRepositoryFamily) OwnerRepositoryBinding {
	return OwnerRepositoryBinding{token: &ownerRepositoryBindingToken{marker: 1}, family: family}
}

func NewResultOwnerRepositoryBinding() OwnerRepositoryBinding {
	return newOwnerRepositoryBinding(ownerRepositoryFamilyResult)
}

func NewDecisionOwnerRepositoryBinding() OwnerRepositoryBinding {
	return newOwnerRepositoryBinding(ownerRepositoryFamilyDecision)
}

func NewApprovalOwnerRepositoryBinding() OwnerRepositoryBinding {
	return newOwnerRepositoryBinding(ownerRepositoryFamilyApproval)
}

func NewMemoryOwnerRepositoryBinding() OwnerRepositoryBinding {
	return newOwnerRepositoryBinding(ownerRepositoryFamilyMemory)
}

func NewEventStreamOwnerRepositoryBinding() OwnerRepositoryBinding {
	return newOwnerRepositoryBinding(ownerRepositoryFamilyEventStream)
}

func (b OwnerRepositoryBinding) validFor(family ownerRepositoryFamily) bool {
	return b.token != nil && b.family == family
}

func (b OwnerRepositoryBinding) Same(other OwnerRepositoryBinding) bool {
	return b.token != nil && b.family == other.family && b.token == other.token
}

// OwnerRepositoryBound exposes only the backend construction identity. Every
// surface admitted to an owner bundle must return the same binding.
type OwnerRepositoryBound interface {
	OwnerRepositoryBinding() OwnerRepositoryBinding
}

func validateOwnerRepositorySurfaces(family ownerRepositoryFamily, binding OwnerRepositoryBinding, surfaces ...any) error {
	if !binding.validFor(family) {
		return common.ErrInvalidArgument
	}
	for _, surface := range surfaces {
		bound, ok := surface.(OwnerRepositoryBound)
		if !ok || !binding.Same(bound.OwnerRepositoryBinding()) {
			return common.ErrConflict
		}
	}
	return nil
}

// ResultOwnerBackend is implemented only by the memory and file backend
// aggregates. The constructor below never accepts independently supplied
// Result/Artifact/deletion repositories.
type ResultOwnerBackend interface {
	ResultOwnerBinding() OwnerRepositoryBinding
	ResultOwnerResults() ResultRepository
	ResultOwnerArtifacts() ArtifactRepository
	ResultOwnerWorkspaceDeletions() ResultWorkspaceDeletionRepository
}

type ResultOwnerRepositories struct {
	results            ResultRepository
	artifacts          ArtifactRepository
	workspaceDeletions ResultWorkspaceDeletionRepository
	binding            OwnerRepositoryBinding
}

func NewResultOwnerRepositories(backend ResultOwnerBackend) (ResultOwnerRepositories, error) {
	if backend == nil {
		return ResultOwnerRepositories{}, common.ErrInvalidArgument
	}
	results, artifacts, deletions := backend.ResultOwnerResults(), backend.ResultOwnerArtifacts(), backend.ResultOwnerWorkspaceDeletions()
	if results == nil || artifacts == nil || deletions == nil {
		return ResultOwnerRepositories{}, common.ErrInvalidArgument
	}
	binding := backend.ResultOwnerBinding()
	if err := validateOwnerRepositorySurfaces(ownerRepositoryFamilyResult, binding, results, artifacts, deletions); err != nil {
		return ResultOwnerRepositories{}, err
	}
	return ResultOwnerRepositories{results: results, artifacts: artifacts, workspaceDeletions: deletions, binding: binding}, nil
}

func (r ResultOwnerRepositories) Validate() error {
	if r.results == nil || r.artifacts == nil || r.workspaceDeletions == nil {
		return common.ErrInvalidArgument
	}
	return validateOwnerRepositorySurfaces(ownerRepositoryFamilyResult, r.binding, r.results, r.artifacts, r.workspaceDeletions)
}

func (r ResultOwnerRepositories) Present() bool {
	return r.results != nil || r.artifacts != nil || r.workspaceDeletions != nil || r.binding.token != nil
}

func (r ResultOwnerRepositories) Results() ResultRepository     { return r.results }
func (r ResultOwnerRepositories) Artifacts() ArtifactRepository { return r.artifacts }
func (r ResultOwnerRepositories) WorkspaceDeletions() ResultWorkspaceDeletionRepository {
	return r.workspaceDeletions
}

type DecisionOwnerBackend interface {
	DecisionOwnerBinding() OwnerRepositoryBinding
	DecisionOwnerTickets() DecisionTicketRepository
	DecisionOwnerResolutions() DecisionResolutionRepository
	DecisionOwnerWorkspaceDeletions() DecisionWorkspaceDeletionRepository
}

type DecisionOwnerRepositories struct {
	tickets            DecisionTicketRepository
	resolutions        DecisionResolutionRepository
	workspaceDeletions DecisionWorkspaceDeletionRepository
	binding            OwnerRepositoryBinding
}

func NewDecisionOwnerRepositories(backend DecisionOwnerBackend) (DecisionOwnerRepositories, error) {
	if backend == nil {
		return DecisionOwnerRepositories{}, common.ErrInvalidArgument
	}
	tickets, resolutions, deletions := backend.DecisionOwnerTickets(), backend.DecisionOwnerResolutions(), backend.DecisionOwnerWorkspaceDeletions()
	if tickets == nil || resolutions == nil || deletions == nil {
		return DecisionOwnerRepositories{}, common.ErrInvalidArgument
	}
	binding := backend.DecisionOwnerBinding()
	if err := validateOwnerRepositorySurfaces(ownerRepositoryFamilyDecision, binding, tickets, resolutions, deletions); err != nil {
		return DecisionOwnerRepositories{}, err
	}
	return DecisionOwnerRepositories{tickets: tickets, resolutions: resolutions, workspaceDeletions: deletions, binding: binding}, nil
}

func (r DecisionOwnerRepositories) Validate() error {
	if r.tickets == nil || r.resolutions == nil || r.workspaceDeletions == nil {
		return common.ErrInvalidArgument
	}
	return validateOwnerRepositorySurfaces(ownerRepositoryFamilyDecision, r.binding, r.tickets, r.resolutions, r.workspaceDeletions)
}

func (r DecisionOwnerRepositories) Present() bool {
	return r.tickets != nil || r.resolutions != nil || r.workspaceDeletions != nil || r.binding.token != nil
}

func (r DecisionOwnerRepositories) Tickets() DecisionTicketRepository         { return r.tickets }
func (r DecisionOwnerRepositories) Resolutions() DecisionResolutionRepository { return r.resolutions }
func (r DecisionOwnerRepositories) WorkspaceDeletions() DecisionWorkspaceDeletionRepository {
	return r.workspaceDeletions
}

type ApprovalOwnerBackend interface {
	ApprovalOwnerBinding() OwnerRepositoryBinding
	ApprovalOwnerApprovals() ApprovalRepository
	ApprovalOwnerWorkspaceDeletions() ApprovalWorkspaceDeletionRepository
}

type ApprovalOwnerRepositories struct {
	approvals          ApprovalRepository
	workspaceDeletions ApprovalWorkspaceDeletionRepository
	binding            OwnerRepositoryBinding
}

func NewApprovalOwnerRepositories(backend ApprovalOwnerBackend) (ApprovalOwnerRepositories, error) {
	if backend == nil {
		return ApprovalOwnerRepositories{}, common.ErrInvalidArgument
	}
	approvals, deletions := backend.ApprovalOwnerApprovals(), backend.ApprovalOwnerWorkspaceDeletions()
	if approvals == nil || deletions == nil {
		return ApprovalOwnerRepositories{}, common.ErrInvalidArgument
	}
	binding := backend.ApprovalOwnerBinding()
	if err := validateOwnerRepositorySurfaces(ownerRepositoryFamilyApproval, binding, approvals, deletions); err != nil {
		return ApprovalOwnerRepositories{}, err
	}
	return ApprovalOwnerRepositories{approvals: approvals, workspaceDeletions: deletions, binding: binding}, nil
}

func (r ApprovalOwnerRepositories) Validate() error {
	if r.approvals == nil || r.workspaceDeletions == nil {
		return common.ErrInvalidArgument
	}
	return validateOwnerRepositorySurfaces(ownerRepositoryFamilyApproval, r.binding, r.approvals, r.workspaceDeletions)
}

func (r ApprovalOwnerRepositories) Present() bool {
	return r.approvals != nil || r.workspaceDeletions != nil || r.binding.token != nil
}

func (r ApprovalOwnerRepositories) Approvals() ApprovalRepository { return r.approvals }
func (r ApprovalOwnerRepositories) WorkspaceDeletions() ApprovalWorkspaceDeletionRepository {
	return r.workspaceDeletions
}

type MemoryOwnerBackend interface {
	MemoryOwnerBinding() OwnerRepositoryBinding
	MemoryOwnerItems() MemoryItemRepository
	MemoryOwnerRecallTraces() MemoryRecallTraceRepository
	MemoryOwnerWorkspaceDeletions() MemoryWorkspaceDeletionRepository
}

type MemoryOwnerRepositories struct {
	items              MemoryItemRepository
	recallTraces       MemoryRecallTraceRepository
	workspaceDeletions MemoryWorkspaceDeletionRepository
	binding            OwnerRepositoryBinding
}

func NewMemoryOwnerRepositories(backend MemoryOwnerBackend) (MemoryOwnerRepositories, error) {
	if backend == nil {
		return MemoryOwnerRepositories{}, common.ErrInvalidArgument
	}
	items, traces, deletions := backend.MemoryOwnerItems(), backend.MemoryOwnerRecallTraces(), backend.MemoryOwnerWorkspaceDeletions()
	if items == nil || traces == nil || deletions == nil {
		return MemoryOwnerRepositories{}, common.ErrInvalidArgument
	}
	binding := backend.MemoryOwnerBinding()
	if err := validateOwnerRepositorySurfaces(ownerRepositoryFamilyMemory, binding, items, traces, deletions); err != nil {
		return MemoryOwnerRepositories{}, err
	}
	return MemoryOwnerRepositories{items: items, recallTraces: traces, workspaceDeletions: deletions, binding: binding}, nil
}

func (r MemoryOwnerRepositories) Validate() error {
	if r.items == nil || r.recallTraces == nil || r.workspaceDeletions == nil {
		return common.ErrInvalidArgument
	}
	return validateOwnerRepositorySurfaces(ownerRepositoryFamilyMemory, r.binding, r.items, r.recallTraces, r.workspaceDeletions)
}

func (r MemoryOwnerRepositories) Present() bool {
	return r.items != nil || r.recallTraces != nil || r.workspaceDeletions != nil || r.binding.token != nil
}

func (r MemoryOwnerRepositories) Items() MemoryItemRepository               { return r.items }
func (r MemoryOwnerRepositories) RecallTraces() MemoryRecallTraceRepository { return r.recallTraces }
func (r MemoryOwnerRepositories) WorkspaceDeletions() MemoryWorkspaceDeletionRepository {
	return r.workspaceDeletions
}

type EventStreamOwnerBackend interface {
	EventStreamOwnerBinding() OwnerRepositoryBinding
	EventStreamOwnerEventRecords() EventRecordRepository
	EventStreamOwnerWorkspaceDeletions() EventStreamWorkspaceDeletionRepository
	EventStreamOwnerWorkspacePurger() EventStreamWorkspacePurger
}

type EventStreamOwnerRepositories struct {
	eventRecords       EventRecordRepository
	workspaceDeletions EventStreamWorkspaceDeletionRepository
	workspacePurger    EventStreamWorkspacePurger
	binding            OwnerRepositoryBinding
}

func NewEventStreamOwnerRepositories(backend EventStreamOwnerBackend) (EventStreamOwnerRepositories, error) {
	if backend == nil {
		return EventStreamOwnerRepositories{}, common.ErrInvalidArgument
	}
	records, deletions, purger := backend.EventStreamOwnerEventRecords(), backend.EventStreamOwnerWorkspaceDeletions(), backend.EventStreamOwnerWorkspacePurger()
	if records == nil || deletions == nil || purger == nil {
		return EventStreamOwnerRepositories{}, common.ErrInvalidArgument
	}
	binding := backend.EventStreamOwnerBinding()
	if err := validateOwnerRepositorySurfaces(ownerRepositoryFamilyEventStream, binding, records, deletions, purger); err != nil {
		return EventStreamOwnerRepositories{}, err
	}
	return EventStreamOwnerRepositories{eventRecords: records, workspaceDeletions: deletions, workspacePurger: purger, binding: binding}, nil
}

func (r EventStreamOwnerRepositories) Validate() error {
	if r.eventRecords == nil || r.workspaceDeletions == nil || r.workspacePurger == nil {
		return common.ErrInvalidArgument
	}
	return validateOwnerRepositorySurfaces(ownerRepositoryFamilyEventStream, r.binding, r.eventRecords, r.workspaceDeletions, r.workspacePurger)
}

func (r EventStreamOwnerRepositories) Present() bool {
	return r.eventRecords != nil || r.workspaceDeletions != nil || r.workspacePurger != nil || r.binding.token != nil
}

func (r EventStreamOwnerRepositories) EventRecords() EventRecordRepository { return r.eventRecords }
func (r EventStreamOwnerRepositories) WorkspaceDeletions() EventStreamWorkspaceDeletionRepository {
	return r.workspaceDeletions
}
func (r EventStreamOwnerRepositories) WorkspacePurger() EventStreamWorkspacePurger {
	return r.workspacePurger
}
