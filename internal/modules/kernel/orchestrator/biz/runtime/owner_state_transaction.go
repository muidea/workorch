package runtime

import (
	"context"
	"fmt"
	"reflect"
	"runtime/trace"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeOwnerStateTransactionVersion = "runtime-owner-state-transition-transaction/v1"

// ownerStateOriginalRepository is Orchestrator-local plumbing, never a Run
// owner port. The production adapter performs the real typed Run query and
// detaches conversion; only a complete owner-local compiler admits the inputs.
type ownerStateOriginalRepository interface {
	LoadOwnerStateOriginal(context.Context, common.WorkspaceID, common.RunID) (runtimecontract.RuntimeStateRecord, RuntimeAggregate, error)
}

// runtime-owner-query-reload-compiler/v1 shares only the historical compiler's
// exact-raw pure subresults. Every query actually reloads the owner and runs the
// full ordered DTO/aggregate gates. Invariant products are never retained, and
// no reducer, Commit, Save or successful validation credential is involved.
func loadOwnerStateQuery(ctx context.Context, c *RuntimeController, original ownerStateOriginalRepository, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	region := startRuntimeTraceRegion(ctx, "workorch.runtime.query_reload")
	defer region.End()
	var aggregate RuntimeAggregate
	err := c.historicalCompiler.Compile(workspaceID, runID, func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
		record, loaded, err := original.LoadOwnerStateOriginal(ctx, workspaceID, runID)
		if err != nil {
			return err
		}
		if err := validate(ownerStateSourceImage(&record, loaded)); err != nil {
			return err
		}
		aggregate = loaded
		return nil
	})
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

// ownerStateTransaction is allocated inside one ApplyInternalEvent invocation.
// It is not installed on the shared controller, stored in context, serialized,
// or returned. Its gate executes complete validation, not a success assertion.
type ownerStateTransaction struct {
	scope     RuntimeInternalEventScope
	validate  func(*runtimecontract.RuntimeStateRecord, RuntimeAggregate) error
	loaded    bool
	committed bool
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
	if t.loaded || workspaceID != t.scope.WorkspaceID || runID != t.scope.RunID {
		// Additional loads never inherit a previous successful Load assertion.
		return r.RuntimeStateRepository.Load(ctx, workspaceID, runID)
	}
	record, aggregate, err := r.original.LoadOwnerStateOriginal(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := t.validate(&record, aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	t.loaded = true
	return aggregate, nil
}

func ownerStateSourceImage(record *runtimecontract.RuntimeStateRecord, aggregate RuntimeAggregate) runtimecontract.RuntimeOwnerStateSourceImage {
	return ownerStateSourceImageInCompiler(record, aggregate, nil)
}

// Each entry contains independent full raw inputs, not a digest or caller
// assertion. Superset comparison deliberately includes every aggregate field;
// no hand-maintained consumed-field projection can omit an invariant source.
// Only the two ordered, pure owner continuations can use these private results.
type ownerStateInvariantPreimage struct {
	raw           RuntimeAggregate
	before, after bool
}

type ownerStateInvariantCompiler struct{ images []ownerStateInvariantPreimage }

func (c *ownerStateInvariantCompiler) validate(a RuntimeAggregate, after bool, body func() error) error {
	if c == nil {
		return body()
	}
	index := -1
	for i := range c.images {
		if reflect.DeepEqual(c.images[i].raw, a) {
			index = i
			if (after && c.images[i].after) || (!after && c.images[i].before) {
				return nil
			}
			break
		}
	}
	if err := body(); err != nil {
		return err
	}
	if index < 0 {
		c.images = append(c.images, ownerStateInvariantPreimage{raw: a.CloneRaw()})
		index = len(c.images) - 1
	}
	if after {
		c.images[index].after = true
	} else {
		c.images[index].before = true
	}
	return nil
}

func ownerStateSourceImageInCompiler(record *runtimecontract.RuntimeStateRecord, aggregate RuntimeAggregate, compiler *ownerStateInvariantCompiler) runtimecontract.RuntimeOwnerStateSourceImage {
	var active *runtimecontract.RuntimeDispatchRecord
	if aggregate.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*aggregate.ActiveDispatch)
		active = &value
	}
	completed := sealedDispatchesToRuntimeRecords(aggregate.CompletedDispatches)
	return runtimecontract.RuntimeOwnerStateSourceImage{
		Original: record, Sources: runtimeLineageAssertionSourcesWithDispatches(aggregate, active, completed),
		Output: aggregate.Decision.RequiredOutput, Revision: aggregate.StateRevision, FeedbackVersion: aggregate.ResultFeedbackVersion,
		BeforeSemantic: func() error {
			return compiler.validate(aggregate, false, func() error { return validateRuntimeAggregateSM2BeforeSemantic(aggregate, active, completed) })
		},
		AfterSemantic: func(gate *runtimecontract.RuntimeOwnerInvariantSourceGate) error {
			return compiler.validate(aggregate, true, func() error { return aggregate.validateAfterSemanticSourcesInGate(active, completed, gate) })
		},
	}
}

func (c *RuntimeController) applyOwnerStateTransaction(ctx context.Context, event RuntimeInternalEvent, original ownerStateOriginalRepository) (result RuntimeInternalEventResult, err error) {
	region := startRuntimeTraceRegion(ctx, "workorch.runtime.event_transaction")
	defer region.End()
	err = c.historicalCompiler.Compile(event.Scope.WorkspaceID, event.Scope.RunID, func(validate func(runtimecontract.RuntimeOwnerStateSourceImage) error) error {
		compiler := &ownerStateInvariantCompiler{}
		transaction := &ownerStateTransaction{scope: event.Scope, validate: func(record *runtimecontract.RuntimeStateRecord, a RuntimeAggregate) error {
			return validate(ownerStateSourceImageInCompiler(record, a, compiler))
		}}
		defer func() { transaction.validate = nil }()
		repository := ownerStateTransactionRepository{RuntimeStateRepository: c.repository, original: original, transaction: transaction}
		local, createErr := NewRuntimeController(repository)
		if createErr != nil {
			return createErr
		}
		local.transitions.ownerTransaction = transaction
		// The coordinator keeps the original repository for the actual Commit
		// reread and sole CAS. The local wrapper only compiles the initial Load.
		local.transitions.repository = c.repository
		// The wrapper deliberately does not implement the raw-load interface:
		// this re-enters the exhaustive, typed gate exactly once, not a registry
		// of arbitrary reducers. Every concrete handler remains unchanged.
		result, err = local.ApplyInternalEvent(ctx, event)
		return err
	})
	return result, err
}

func (t *ownerStateTransaction) validateAfterImage(aggregate RuntimeAggregate) error {
	if t.validate == nil || t.committed {
		return fmt.Errorf("%w: owner-state transaction is closed or consumed", common.ErrInvalidArgument)
	}
	t.committed = true
	return t.validate(nil, aggregate)
}

// Trace is diagnostic only; a nil business context must not become a new panic
// when optional tracing is enabled. The original context is never replaced.
func startRuntimeTraceRegion(ctx context.Context, name string) *trace.Region {
	if ctx == nil {
		ctx = context.Background()
	}
	return trace.StartRegion(ctx, name)
}
