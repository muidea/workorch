package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Called with the real confirmed-repair continuation and pending validation
// proposal from the Shell/File integration chain. A closure rejection is an
// owner fact, never a provider-selected state. Its SM-3 failure and SM-2
// continuation retirement must survive both sides of the same CAS boundary.
func testObservedFreshValidationRejectionCAS(t *testing.T, pending RuntimeAggregate) {
	t.Helper()
	for name, mode := range map[string]failureAuthorityCASMode{
		"normal":        failureAuthorityCASNormal,
		"before write":  failureAuthorityCASFailBeforeWrite,
		"lost response": failureAuthorityCASLoseResponse,
	} {
		t.Run("closure rejection CAS "+name, func(t *testing.T) {
			repository := &failureAuthorityCASRepository{aggregate: cloneRuntimeAggregateForMCPProvenanceTest(t, pending), mode: mode}
			before := cloneRuntimeAggregateForMCPProvenanceTest(t, repository.aggregate)
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			event := RuntimeInternalEvent{
				Kind:  RuntimeInternalEventCandidateRejected,
				Scope: RuntimeInternalEventScope{WorkspaceID: pending.Facts.WorkspaceID, RunID: pending.Facts.RunID},
				Transaction: &RuntimeInternalTransactionPayload{Attempt: 0, CandidateFailure: &runtimecontract.RuntimeCandidateFailure{
					Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: candidateAdmissionCodeRepairClosureMismatch,
					Signature: "candidate-repair-closure-mismatch", Reason: "candidate conflicts with the sealed repair closure",
				}},
			}
			_, err = controller.ApplyInternalEvent(context.Background(), event)
			if mode == failureAuthorityCASNormal {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, errFailureAuthorityCASCut) {
				t.Fatalf("CAS fault was not observed: %v", err)
			}
			if mode == failureAuthorityCASFailBeforeWrite {
				if !reflect.DeepEqual(before, repository.aggregate) {
					t.Fatal("pre-write failure tore the SM-2/SM-3 handoff")
				}
				repository.mode = failureAuthorityCASNormal
				if _, err := controller.ApplyInternalEvent(context.Background(), event); err != nil {
					t.Fatal(err)
				}
			}
			repository.mode = failureAuthorityCASNormal
			restarted, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			closed, err := restarted.Load(context.Background(), pending.Facts.WorkspaceID, pending.Facts.RunID)
			if err != nil || closed.StateRevision != pending.StateRevision+1 || closed.Decision.State != runtimecontract.ConvergenceStateFailed ||
				closed.Decision.Terminal == nil || closed.Decision.Terminal.Code != "runtime_contract_defect" ||
				closed.Facts.RepairDirective != nil || closed.ActiveDispatch != nil {
				t.Fatalf("closure rejection did not atomically retire validation and terminalize: revision=%d expected=%d state=%s terminal=%+v directive=%t dispatch=%t err=%v", closed.StateRevision, pending.StateRevision+1, closed.Decision.State, closed.Decision.Terminal, closed.Facts.RepairDirective != nil, closed.ActiveDispatch != nil, err)
			}
			// SM-2 keeps its canonical contract-defect terminal vocabulary;
			// the SM-3 record must retain the original classified cause.
			attempt := closed.CandidateAttempts[len(closed.CandidateAttempts)-1]
			if attempt.Failure == nil || attempt.Failure.Code != candidateAdmissionCodeRepairClosureMismatch || attempt.Correction == nil ||
				attempt.Correction.TerminalCode != candidateAdmissionCodeRepairClosureMismatch || attempt.Correction.Witness != nil {
				t.Fatal("closure rejection lost its SM-3 cause or created correction authority")
			}
			if !reflect.DeepEqual(closed.ExecutionBudget, before.ExecutionBudget) || closed.EvidenceSequence != before.EvidenceSequence ||
				!reflect.DeepEqual(closed.OperationReceipts, before.OperationReceipts) || !reflect.DeepEqual(closed.Facts.Coverage, before.Facts.Coverage) {
				t.Fatal("closure rejection created budget usage, effects or CWI credit")
			}
			writes := repository.saveAttempts
			// Terminal has new authority and no pending attempt. The old
			// attempt-scoped private event is stale, not a new failure fact;
			// recovery consumes the already-persisted terminal instead.
			_, err = restarted.ApplyInternalEvent(context.Background(), event)
			if !errors.Is(err, common.ErrConflict) || repository.aggregate.StateRevision != closed.StateRevision || repository.saveAttempts != writes {
				t.Fatalf("stale rejection replay was not fenced without a write: %v", err)
			}
			if _, err := restarted.BeginCandidateAttemptForTest(context.Background(), pending.Facts.WorkspaceID, pending.Facts.RunID, 1, "forbidden-correction"); err == nil || repository.saveAttempts != writes {
				t.Fatal("rejected-final closure reopened correction/provider work")
			}
		})
	}
}
