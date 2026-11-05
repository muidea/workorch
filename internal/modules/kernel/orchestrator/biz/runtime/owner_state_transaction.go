package runtime

import (
	"context"
	"fmt"
	"reflect"
	"runtime/trace"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ownerStateOriginalRepository is Orchestrator-local plumbing, never a Run
// owner port. The production adapter performs the real typed Run query and
// detaches conversion; only a complete owner-local compiler admits the inputs.
type ownerStateOriginalRepository interface {
	LoadOwnerStateOriginal(context.Context, common.WorkspaceID, common.RunID) (runtimecontract.RuntimeStateRecord, RuntimeAggregate, error)
}

// ownerStateTransaction is allocated inside one ApplyInternalEvent invocation.
// It is not installed on the shared controller, stored in context, serialized,
// or returned. Its detached original is private to one event and real reread.
type ownerStateTransaction struct {
	scope     RuntimeInternalEventScope
	validate  func(*runtimecontract.RuntimeStateRecord, RuntimeAggregate) error
	loaded    bool
	committed bool
	original  *runtimecontract.RuntimeStateRecord
}

type ownerStateTransactionRepository struct {
	RuntimeStateRepository
	original    ownerStateOriginalRepository
	transaction *ownerStateTransaction
}

func (r ownerStateTransactionRepository) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	region := startRuntimeTraceRegion(ctx, "workorch.runtime.event_load")
	defer region.End()
	t := r.transaction
	var record *runtimecontract.RuntimeStateRecord
	var aggregate RuntimeAggregate
	var err error
	if r.original != nil {
		var raw runtimecontract.RuntimeStateRecord
		raw, aggregate, err = r.original.LoadOwnerStateOriginal(ctx, workspaceID, runID)
		record = &raw
	} else {
		aggregate, err = r.RuntimeStateRepository.Load(ctx, workspaceID, runID)
	}
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if record != nil {
		if err := t.admit(record, aggregate); err != nil {
			return RuntimeAggregate{}, err
		}

	}
	if !t.loaded && workspaceID == t.scope.WorkspaceID && runID == t.scope.RunID {
		value := RuntimeStateRecordFromAggregate(aggregate)
		t.original = &value
		t.loaded = true
	}
	return aggregate, nil
}

func ownerStateSourceImage(record *runtimecontract.RuntimeStateRecord, aggregate RuntimeAggregate) runtimecontract.RuntimeOwnerStateSourceImage {
	return ownerStateSourceImageInCompiler(record, aggregate)
}

func ownerStateSourceImageInCompiler(record *runtimecontract.RuntimeStateRecord, aggregate RuntimeAggregate) runtimecontract.RuntimeOwnerStateSourceImage {
	converted := ownerStateRecord(aggregate)
	return runtimecontract.RuntimeOwnerStateSourceImage{
		Original: record, Converted: &converted,
		AggregateJoins: func(gate *runtimecontract.RuntimeOwnerInvariantSourceGate) error {
			return aggregate.validateOwnerAggregateJoins(gate)
		},
	}
}

func (c *RuntimeController) applyOwnerStateTransaction(ctx context.Context, event RuntimeInternalEvent, original ownerStateOriginalRepository) (result RuntimeInternalEventResult, err error) {
	region := startRuntimeTraceRegion(ctx, "workorch.runtime.event_transaction")
	defer region.End()
	transaction := &ownerStateTransaction{scope: event.Scope, validate: func(record *runtimecontract.RuntimeStateRecord, a RuntimeAggregate) error {
		return runtimecontract.CompileRuntimeOwnerStateTransaction(func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
			return validate(ownerStateSourceImage(record, a))
		})
	}}
	defer func() { transaction.original, transaction.validate = nil, nil }()
	repository := ownerStateTransactionRepository{RuntimeStateRepository: c.repository, original: original, transaction: transaction}
	local, err := NewRuntimeController(repository)
	if err != nil {
		return result, err
	}
	local.transitions.ownerTransaction = transaction
	local.transitions.repository = c.repository
	return local.ApplyInternalEvent(ctx, event)
}

// admit reuses only this event's detached admitted original after a real
// reread and full raw equality. Changed input always re-enters owner admission;
// no caller flag, digest or retained PASS can authorize a different image.
func (t *ownerStateTransaction) admit(record *runtimecontract.RuntimeStateRecord, aggregate RuntimeAggregate) error {
	if t.original != nil {
		converted := ownerStateRecord(aggregate)
		if reflect.DeepEqual(*t.original, converted) && (record == nil || reflect.DeepEqual(*record, converted)) {
			return nil
		}
	}
	return t.validate(record, aggregate)
}

func (t *ownerStateTransaction) validateAfterImage(aggregate RuntimeAggregate) error {
	if t.original == nil || t.committed {
		return fmt.Errorf("%w: owner transaction has no formal preimage or is closed", common.ErrInvalidArgument)
	}
	t.committed = true
	previous := RuntimeAggregate(*t.original)
	image := ownerStateSourceImage(nil, aggregate)
	image.AggregateJoins = func(gate *runtimecontract.RuntimeOwnerInvariantSourceGate) error {
		return aggregate.validateOwnerAggregateTransition(&previous, gate)
	}
	return runtimecontract.CompileRuntimeOwnerStateTransition(*t.original, image)
}

// Trace is diagnostic only; a nil business context must not become a new panic
// when optional tracing is enabled. The original context is never replaced.
func startRuntimeTraceRegion(ctx context.Context, name string) *trace.Region {
	if ctx == nil {
		ctx = context.Background()
	}
	return trace.StartRegion(ctx, name)
}
