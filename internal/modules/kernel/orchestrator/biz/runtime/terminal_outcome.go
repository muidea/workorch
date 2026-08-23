package runtime

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// DeriveTerminalOutcome is the single pure SM-4 terminal identity helper.
// The boolean distinguishes a valid non-terminal Runtime from a sealed
// terminal; malformed or competing SM-1/SM-2 terminal inputs fail closed.
func DeriveTerminalOutcome(decision runtimecontract.RuntimeDecision, termination *runtimecontract.RuntimeTerminationRecord) (runtimecontract.TerminalOutcome, bool, error) {
	if termination != nil {
		if err := termination.Validate(); err != nil {
			return runtimecontract.TerminalOutcome{}, false, err
		}
		if decision.Terminal != nil || isTerminalConvergenceState(decision.State) {
			return runtimecontract.TerminalOutcome{}, false, fmt.Errorf("%w: administrative and convergence terminals cannot coexist", common.ErrConflict)
		}
		kind := runtimecontract.TerminalOutcomeKindCancelled
		if termination.Kind == runtimecontract.RuntimeTerminationSuperseded {
			kind = runtimecontract.TerminalOutcomeKindSuperseded
		}
		outcome := runtimecontract.TerminalOutcome{
			Kind: kind, Code: "administrative_" + string(termination.Kind), Summary: termination.Reason,
		}.Normalized()
		if err := outcome.Validate(); err != nil {
			return runtimecontract.TerminalOutcome{}, false, err
		}
		return outcome, true, nil
	}

	if !isTerminalConvergenceState(decision.State) {
		if decision.Terminal != nil {
			return runtimecontract.TerminalOutcome{}, false, fmt.Errorf("%w: non-terminal convergence state carries terminal detail", common.ErrConflict)
		}
		return runtimecontract.TerminalOutcome{}, false, nil
	}
	if decision.Terminal == nil {
		return runtimecontract.TerminalOutcome{}, false, fmt.Errorf("%w: terminal convergence state has no terminal detail", common.ErrConflict)
	}
	kind := runtimecontract.TerminalOutcomeKindFailed
	if decision.State == runtimecontract.ConvergenceStateComplete {
		kind = runtimecontract.TerminalOutcomeKindCompleted
	}
	outcome := runtimecontract.TerminalOutcome{Kind: kind, Code: decision.Terminal.Code, Summary: decision.Terminal.Summary}.Normalized()
	if err := outcome.Validate(); err != nil {
		return runtimecontract.TerminalOutcome{}, false, err
	}
	return outcome, true, nil
}
