package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// DeriveTerminalOutcome is the single pure SM-4 terminal identity helper.
// The boolean distinguishes a valid non-terminal Runtime from a sealed
// terminal; malformed or competing SM-1/SM-2 terminal inputs fail closed.
func DeriveTerminalOutcome(decision runtimecontract.RuntimeDecision, termination *runtimecontract.RuntimeTerminationRecord) (runtimecontract.TerminalOutcome, bool, error) {
	return runtimecontract.DeriveRuntimeTerminalOutcome(decision, termination)
}
