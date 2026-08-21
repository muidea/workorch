package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeController struct {
	*runtimeMutationKernel
	authority         *authorityComponent
	planning          *planningTransactionComponent
	candidateDispatch *candidateDispatchComponent
	driveDelivery     *driveDeliveryComponent
}

// CandidateAdmissionCommitResult couples the persisted aggregate with the
// expected, typed admission outcome. Callers must branch on Admission rather
// than parse an error message.
type CandidateAdmissionCommitResult struct {
	Aggregate RuntimeAggregate
	Admission CandidateAdmissionResult
}

func NewRuntimeController(repository RuntimeStateRepository) (*RuntimeController, error) {
	if repository == nil {
		return nil, common.ErrInvalidArgument
	}
	transitions, err := NewRuntimeTransitionCoordinator(repository)
	if err != nil {
		return nil, err
	}
	kernel, err := newRuntimeMutationKernel(repository, transitions)
	if err != nil {
		return nil, err
	}
	return &RuntimeController{
		runtimeMutationKernel: kernel,
		authority:             newAuthorityComponent(kernel),
		planning:              newPlanningTransactionComponent(kernel),
		candidateDispatch:     newCandidateDispatchComponent(kernel),
		driveDelivery:         newDriveDeliveryComponent(kernel),
	}, nil
}

// initialize 建立一个 Run 唯一的 Runtime aggregate。
// 已初始化的 Run 只允许幂等读取，不会因为 Start/Resume/restart 再次执行 reducer。
func (c *RuntimeController) initialize(ctx context.Context, facts runtimecontract.RuntimeFacts, definition runtimecontract.RuntimeDefinitionContext, requirements []runtimecontract.RuntimeRequirement) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	facts = facts.Normalized()
	if facts.CWILedgerSchemaVersion == "" {
		var ledgerErr error
		facts, ledgerErr = runtimecontract.InitializeRuntimeCWILedgerForActivation(facts)
		if ledgerErr != nil {
			return RuntimeAggregate{}, ledgerErr
		}
	}
	definition = definition.Normalized()
	if err := facts.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := definition.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if definition.Revision != facts.DefinitionRevision {
		return RuntimeAggregate{}, fmt.Errorf("runtime definition context revision does not match facts")
	}
	existing, err := c.repository.Load(ctx, facts.WorkspaceID, facts.RunID)
	if err == nil {
		if !isTerminalConvergenceState(existing.Decision.State) {
			if err := runtimecontract.ValidateRuntimeOperationContractRevision(existing.Facts.Activation.ExecutionContract.OperationContractRevision); err != nil {
				return RuntimeAggregate{}, err
			}
			// The controller's direct callers do not own a persisted bootstrap.
			// Align their restart identity with the executable catalog only after
			// checking the persisted aggregate; this never writes a default into
			// existing authority.
			facts.Activation.ExecutionContract.OperationContractRevision = runtimecontract.RuntimeOperationContractRevision()
			facts = facts.Normalized()
		}
		if !sameRuntimeDefinition(existing.Facts, facts) || !runtimecontract.SameRuntimeDefinitionContext(existing.Definition, definition) {
			return RuntimeAggregate{}, fmt.Errorf("%w: runtime state already exists with a different definition", common.ErrConflict)
		}
		return existing, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return RuntimeAggregate{}, err
	}
	// Bootstrap construction is the only point that seals the operation catalog
	// compiled into this binary. Existing persisted aggregates are never
	// defaulted or rewritten.
	facts.Activation.ExecutionContract.OperationContractRevision = runtimecontract.RuntimeOperationContractRevision()
	facts = facts.Normalized()
	facts = PreflightRuntimeFacts(facts, requirements)
	semanticPlanning, err := runtimecontract.NewRuntimeSemanticPlanningLedger(facts.DefinitionSeed)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	executionBudget, err := runtimecontract.NewRuntimeExecutionBudgetLedger(facts.ExecutionBudgetPolicy)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate := RuntimeAggregate{
		StateRevision:    1,
		Definition:       definition,
		Facts:            facts,
		SemanticPlanning: semanticPlanning,
		ExecutionBudget:  executionBudget,
		Decision:         decision,
	}
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, 0); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func isTerminalConvergenceState(state runtimecontract.ConvergenceState) bool {
	return state == runtimecontract.ConvergenceStateComplete || state == runtimecontract.ConvergenceStateFailed
}

// rejectRuntimeProgressAfterTermination fences every SM-2/SM-3 transition
// that could turn an already in-flight external response into new authority or
// a side effect after SM-1 sealed cancel/supersede. The response remains in
// provider/owner audit, but it cannot mutate the terminated aggregate.
func rejectRuntimeProgressAfterTermination(aggregate RuntimeAggregate) error {
	if aggregate.Termination == nil {
		return nil
	}
	return fmt.Errorf("%w: runtime administrative termination %s is already sealed", common.ErrConflict, aggregate.Termination.Kind)
}

func (c *RuntimeController) Load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return c.repository.Load(ctx, workspaceID, runID)
}

// reconcileAuthorityBeforeCandidate CAS-persists the deterministic SM-2
// cleanup required when a loaded aggregate still contains a non-current
// focus-scoped prerequisite/resource. It is intentionally narrower than
// candidate admission: no provider request, candidate attempt, repair budget,
// or owner operation is involved.

// commitAssessment is invoked only by RuntimeInternalEventAssessmentObserved.
// It is deliberately private so external adapters cannot bypass the
// Orchestrator's single internal-event mutation boundary.
