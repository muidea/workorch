package workorchruntimecontract

import (
	"fmt"
	"sync"
)

// RuntimeOwnerStateSourceImage supplies complete raw inputs, never a validation
// credential. Original is present for an actual owner reload and absent for an
// after-image. The owner supplies the actual ordered aggregate invariant bodies.
// This value is lexical compiler input, not an EventHub or persistence contract.
type RuntimeOwnerStateSourceImage struct {
	Original        *RuntimeStateRecord
	Sources         RuntimeLineageAssertionSources
	Output          RequiredOutputContract
	Revision        uint64
	FeedbackVersion uint32
	BeforeSemantic  func() error
	AfterSemantic   func(*RuntimeOwnerInvariantSourceGate) error
}

// CompileRuntimeOwnerStateTransaction bounds pure subresult reuse to one
// synchronous owner-private event transaction. No session or successful product
// is exposed: the supplied function always executes all ordered image gates on
// complete inputs. It is revoked on return (including panic), rejects concurrent
// or recursive use, and poisons the compilation on the first validation error.
// The owner, not this pure compiler, performs every read, reducer and CAS.
func CompileRuntimeOwnerStateTransaction(run func(func(RuntimeOwnerStateSourceImage) error) error) error {
	return compileRuntimeOwnerStateTransaction(run, newRuntimeSemanticValidationSession(), nil)
}

func compileRuntimeOwnerStateTransaction(run func(func(RuntimeOwnerStateSourceImage) error) error, semantic *runtimeSemanticValidationSession, scope *runtimeOwnerHistoricalScope) error {
	if run == nil {
		return fmt.Errorf("runtime owner-state transaction requires an owner body")
	}
	historical := &runtimeHistoricalPreimageSession{semantic: semantic, sourceCompiler: &runtimeHistoricalSourceCompiler{}}
	var mu sync.Mutex
	active := true
	var firstError error
	defer func() {
		mu.Lock()
		active = false
		// Do not retain raw subtrees through a captured, revoked gate function.
		semantic, historical = nil, nil
		mu.Unlock()
	}()
	validate := func(image RuntimeOwnerStateSourceImage) error {
		if !mu.TryLock() {
			return fmt.Errorf("runtime owner-state transaction gate is already running")
		}
		defer mu.Unlock()
		if !active {
			return fmt.Errorf("runtime owner-state transaction gate is closed")
		}
		if firstError != nil {
			return firstError
		}
		if scope != nil && !scope.matches(image) {
			// Keep the same lifecycle, poison and error order on a cold image.
			cold := newRuntimeSemanticValidationSession()
			firstError = validateRuntimeOwnerStateSourceImage(image, cold, &runtimeHistoricalPreimageSession{semantic: cold, sourceCompiler: &runtimeHistoricalSourceCompiler{}})
		} else {
			firstError = validateRuntimeOwnerStateSourceImage(image, semantic, historical)
		}
		return firstError
	}
	err := run(validate)
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		// Keep the owner's original contextual error wrapping and first-error
		// order (in particular Commit's after-image invariant prefix).
		return err
	}
	if firstError != nil {
		return firstError
	}
	return err
}

func validateRuntimeOwnerStateSourceImage(image RuntimeOwnerStateSourceImage, semantic *runtimeSemanticValidationSession, historical *runtimeHistoricalPreimageSession) error {
	if image.Original != nil {
		if err := image.Original.validateInSourceSession(semantic, historical); err != nil {
			return fmt.Errorf("load Runtime state: %w", err)
		}
	}
	if err := ValidateRuntimeDefinitionLineageAssertions(image.Sources.Facts.DefinitionFocuses, image.Sources.Facts.Activation.ExecutionContract.ProfileContracts); err != nil {
		return err
	}
	if err := ValidateRuntimeResultFeedbackVersion(image.FeedbackVersion); err != nil {
		return err
	}
	var after func() error
	if image.AfterSemantic != nil {
		after = func() error { return withRuntimeOwnerInvariantSourceGate(semantic, image.AfterSemantic) }
	}
	return validateRuntimeConvergenceSourcesAndSemanticLedgerInSession(image.Sources, image.Output, image.Revision,
		image.BeforeSemantic, after, semantic, historical)
}
