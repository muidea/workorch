package runtime

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// transitionPreimageRepository is an owner-local optional complete-load path.
// The adapter must read Run again and validate its original DTO before Compile.
// No request/product is an EventHub payload or a recovery credential.
type transitionPreimageRepository interface {
	LoadTransitionPreimage(context.Context, RuntimeTransitionPreimageRequest) (RuntimeTransitionPreimage, error)
}

const runtimeTransitionPreimageVersion = "runtime-transition-preimage/v1"

const runtimeOwnerStateJointPreimageVersion = "runtime-owner-state-joint-preimage/v1"

type runtimeTransitionPreimageCall struct {
	workspaceID common.WorkspaceID
	runID       common.RunID
	revision    uint64
	// 0 = new, 1 = compiling, 2 = compiled, 3 = consumed, 4 = closed.
	state atomic.Uint32
	// Present only inside the current owner-private event compiler. It always
	// executes complete image gates and is revoked when that lexical call ends.
	compiler atomic.Pointer[runtimeOwnerStateImageCompiler]
}

type runtimeOwnerStateImageCompiler struct {
	validate func(*runtimecontract.RuntimeStateRecord, RuntimeAggregate) error
}

// RuntimeTransitionPreimageRequest can only be issued by Commit. Both compilers
// always validate the complete independent original aggregate.
// Copies share the same one-shot lifetime, including failure and cancellation.
type RuntimeTransitionPreimageRequest struct {
	call *runtimeTransitionPreimageCall
}

// RuntimeTransitionPreimage is a private-field, detached original value for
// one Commit invocation, not a caller assertion that validation succeeded.
// Zero, serialized, other-call and consumed products cannot be used.
type RuntimeTransitionPreimage struct {
	version   string
	call      *runtimeTransitionPreimageCall
	aggregate RuntimeAggregate
}

func (r RuntimeTransitionPreimageRequest) Scope() (common.WorkspaceID, common.RunID, uint64) {
	if r.call == nil {
		return "", "", 0
	}
	return r.call.workspaceID, r.call.runID, r.call.revision
}

func (r RuntimeTransitionPreimageRequest) Compile(aggregate RuntimeAggregate) (RuntimeTransitionPreimage, error) {
	if r.call == nil || !r.call.state.CompareAndSwap(0, 1) {
		return RuntimeTransitionPreimage{}, common.ErrInvalidArgument
	}
	// Validate a separate graph: neither validation nor the adapter can mutate
	// the original value subsequently consumed by the transition gates.
	original := aggregate.CloneRaw()
	checked := original.CloneRaw()
	validate := checked.Validate
	compiler := r.call.compiler.Load()
	if compiler != nil {
		validate = func() error { return compiler.validate(nil, checked) }
	}
	if err := validate(); err != nil {
		r.call.state.CompareAndSwap(1, 4)
		// Preserve the complete-load aggregate gate's original error.
		return RuntimeTransitionPreimage{}, err
	}
	if !r.call.state.CompareAndSwap(1, 2) {
		return RuntimeTransitionPreimage{}, common.ErrInvalidArgument
	}
	version := runtimeTransitionPreimageVersion
	if compiler != nil {
		version = runtimeOwnerStateTransactionVersion
	}
	return RuntimeTransitionPreimage{version: version, call: r.call, aggregate: original}, nil
}

// CompileOwnerRecord performs the complete raw DTO and converted aggregate
// gates in one lexical call. No shared context leaves the pure source gate;
// only this Commit's existing one-shot detached product can leave the compiler.
// The aggregate is not trusted to be an unchanged projection of the DTO.
func (r RuntimeTransitionPreimageRequest) CompileOwnerRecord(record runtimecontract.RuntimeStateRecord, aggregate RuntimeAggregate) (RuntimeTransitionPreimage, error) {
	if r.call == nil || !r.call.state.CompareAndSwap(0, 1) {
		return RuntimeTransitionPreimage{}, common.ErrInvalidArgument
	}
	original := aggregate.CloneRaw()
	checked := original.CloneRaw()
	var active *runtimecontract.RuntimeDispatchRecord
	if checked.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*checked.ActiveDispatch)
		active = &value
	}
	completed := sealedDispatchesToRuntimeRecords(checked.CompletedDispatches)
	var err error
	compiler := r.call.compiler.Load()
	if compiler != nil {
		err = compiler.validate(&record, checked)
	} else {
		err = runtimecontract.ValidateRuntimeStateReloadSourcePair(record,
			runtimeLineageAssertionSourcesWithDispatches(checked, active, completed),
			checked.Decision.RequiredOutput, checked.StateRevision, checked.ResultFeedbackVersion,
			func() error { return validateRuntimeAggregateSM2BeforeSemantic(checked, active, completed) },
			func() error { return checked.validateAfterSemanticSources(active, completed) })
	}
	if err != nil {
		r.call.state.CompareAndSwap(1, 4)
		return RuntimeTransitionPreimage{}, err
	}
	if !r.call.state.CompareAndSwap(1, 2) {
		return RuntimeTransitionPreimage{}, common.ErrInvalidArgument
	}
	version := runtimeOwnerStateJointPreimageVersion
	if compiler != nil {
		version = runtimeOwnerStateTransactionVersion
	}
	return RuntimeTransitionPreimage{version: version, call: r.call, aggregate: original}, nil
}

func (r RuntimeTransitionPreimageRequest) consume(value RuntimeTransitionPreimage) (RuntimeAggregate, error) {
	if r.call == nil || (value.version != runtimeTransitionPreimageVersion && value.version != runtimeOwnerStateJointPreimageVersion && value.version != runtimeOwnerStateTransactionVersion) || value.call != r.call || !r.call.state.CompareAndSwap(2, 3) {
		return RuntimeAggregate{}, fmt.Errorf("%w: invalid single-commit runtime preimage", common.ErrInvalidArgument)
	}
	return value.aggregate, nil
}
