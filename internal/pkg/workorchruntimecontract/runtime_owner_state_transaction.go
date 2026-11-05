package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"sync"
)

// RuntimeOwnerStateSourceImage supplies complete raw inputs, never a validation
// credential. Original is present for an actual owner reload and absent for an
// after-image. The owner supplies the actual ordered aggregate invariant bodies.
// This value is lexical compiler input, not an EventHub or persistence contract.
type RuntimeOwnerStateSourceImage struct {
	Original *RuntimeStateRecord
	// Converted is an exact representation of Original, not another business
	// input to admit. AggregateJoins owns only aggregate-specific joins.
	Converted      *RuntimeStateRecord
	AggregateJoins func(*RuntimeOwnerInvariantSourceGate) error
}

// CompileRuntimeOwnerStateTransaction bounds pure subresult reuse to one
// synchronous owner-private event transaction. No session or successful product
// is exposed: the supplied function always executes all ordered image gates on
// complete inputs. It is revoked on return (including panic), rejects concurrent
// or recursive use, and poisons the compilation on the first validation error.
// The owner, not this pure compiler, performs every read, reducer and CAS.
func CompileRuntimeOwnerStateTransaction(run func(func(RuntimeOwnerStateSourceImage) error) error) error {
	return compileRuntimeOwnerStateTransaction(run, newRuntimeSemanticValidationSession())
}

func compileRuntimeOwnerStateTransaction(run func(func(RuntimeOwnerStateSourceImage) error) error, semantic *runtimeSemanticValidationSession) error {
	if run == nil {
		return fmt.Errorf("runtime owner-state transaction requires an owner body")
	}
	// Complete candidate consumers are lexical products only, not entries in
	// the retained semantic whitelist or ordinary public cold validators.
	semantic.candidateSources = &runtimeCandidateSourceCompiler{}
	historical := &runtimeHistoricalPreimageSession{semantic: semantic, sourceCompiler: &runtimeHistoricalSourceCompiler{}}
	var mu sync.Mutex
	active := true
	var firstError error
	defer func() {
		mu.Lock()
		active = false
		// Do not retain raw subtrees through a captured, revoked gate function.
		semantic.candidateSources = nil
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
		firstError = validateRuntimeOwnerStateSourceImage(image, semantic, historical)
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

// CompileRuntimeOwnerStateTransition is the one post-image check for a CAS.
// It borrows the exact formal preimage synchronously, retains no result, and
// never re-admits unchanged typed inputs. It is not a read/recovery entry.
func CompileRuntimeOwnerStateTransition(previous RuntimeStateRecord, image RuntimeOwnerStateSourceImage) error {
	if image.Converted == nil || image.Original != nil {
		return fmt.Errorf("runtime transition requires complete postimage")
	}
	if err := ValidateRuntimeCommittedHistoryTransition(previous, *image.Converted); err != nil {
		return err
	}
	session := newRuntimeSemanticValidationSession()
	session.committedPreimage = &previous
	return validateRuntimeOwnerStateSourceImage(image, session, &runtimeHistoricalPreimageSession{semantic: session})
}

func validateRuntimeOwnerStateSourceImage(image RuntimeOwnerStateSourceImage, semantic *runtimeSemanticValidationSession, historical *runtimeHistoricalPreimageSession) error {
	if image.Converted == nil || image.AggregateJoins == nil {
		return fmt.Errorf("runtime owner image requires complete record and aggregate joins")
	}
	canonical := image.Converted
	if image.Original != nil {
		canonical = image.Original
	}
	if err := runtimeProcessingRegion("record_admission", func() error { return canonical.validateInSourceSession(semantic, historical) }); err != nil {
		if image.Original != nil {
			return fmt.Errorf("load Runtime state: %w", err)
		}
		return err
	}
	if image.Original != nil && !reflect.DeepEqual(*image.Original, *image.Converted) {
		return fmt.Errorf("runtime owner record conversion changed original data")
	}
	return runtimeProcessingRegion("aggregate_joins", func() error { return withRuntimeOwnerInvariantSourceGate(semantic, image.AggregateJoins) })
}
