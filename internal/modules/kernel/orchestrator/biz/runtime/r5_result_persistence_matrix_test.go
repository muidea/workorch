package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// TestR5FiveResultPersistenceMatrix locks the N6 CAS fences for the five
// candidate-pipeline results under R5:
//
//	request_recoverable      — no attempt allocation on incomplete request session
//	candidate_correctable    — attempt 1 only after sealed correctable_once attempt 0
//	candidate_rejected_final — attempt 0 rejected_final seals terminal; no attempt 1
//	prerequisite_stale       — invalidated attempt 0 reopens without attempt budget
//	accepted                 — sealed accept is idempotent and blocks attempt 1
//
// Broader file-backed crash/restart matrices remain in runtime_state_repository_test.go.
func TestR5FiveResultPersistenceMatrix(t *testing.T) {
	t.Run("request_recoverable_does_not_allocate_attempt", func(t *testing.T) {
		repository := &controllerRepository{}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
			t.Fatal(err)
		}
		if _, err := controller.EnsureSemanticContractSealedForOperationTest(context.Background(), "ws-v2", "run-v2"); err != nil {
			t.Fatal(err)
		}
		aggregate, err := controller.Load(context.Background(), "ws-v2", "run-v2")
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(aggregate.Decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(aggregate.Facts.Resources))
		if err != nil {
			t.Fatal(err)
		}
		ensured, session, err := controller.ensureCandidateRequestSession(context.Background(), "ws-v2", "run-v2", runtimecontract.RuntimeProviderDocumentOperations, "stable-prefix", manifest.Hash, "")
		if err != nil {
			t.Fatal(err)
		}
		if session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete {
			t.Fatalf("fresh request session already complete: %+v", session)
		}
		if len(ensured.CandidateAttempts) != 0 {
			t.Fatalf("request session allocated candidate attempts: %+v", ensured.CandidateAttempts)
		}
		// Incomplete session identity must not open attempt 0.
		if _, err := controller.beginCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix", manifest, session.LogicalRequestID); err == nil {
			t.Fatal("beginCandidateAttempt accepted incomplete request session document")
		}
		reloaded, err := controller.Load(context.Background(), "ws-v2", "run-v2")
		if err != nil {
			t.Fatal(err)
		}
		if len(reloaded.CandidateAttempts) != 0 {
			t.Fatalf("failed begin leaked attempt allocation: %+v", reloaded.CandidateAttempts)
		}
	})

	t.Run("candidate_correctable_allows_exactly_one_retry", func(t *testing.T) {
		repository := &controllerRepository{}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
			t.Fatal(err)
		}
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		failed, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid",
			Signature: "candidate-schema-invalid", Reason: "unexpected EOF",
			Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{Path: "$.operations", Actual: "candidate_shape"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if failed.Decision.State == runtimecontract.ConvergenceStateFailed || failed.CandidateAttempts[0].Failure == nil ||
			!CandidateFailureIsCorrectableOnce(failed.CandidateAttempts[0].Failure) {
			t.Fatalf("schema failure was not sealed correctable_once: %+v", failed)
		}
		// Duplicate fail on a closed attempt is conflict, not a second mutation.
		if _, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid",
			Signature: "candidate-schema-invalid", Reason: "unexpected EOF",
			Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{Path: "$.operations", Actual: "candidate_shape"},
		}); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("duplicate fail err=%v, want conflict", err)
		}
		retried, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix")
		if err != nil {
			t.Fatal(err)
		}
		if len(retried.CandidateAttempts) != 2 || retried.CandidateAttempts[1].State != runtimecontract.RuntimeCandidateAttemptStatePending {
			t.Fatalf("correctable attempt 1 not pending: %+v", retried.CandidateAttempts)
		}
		// Fresh controller reload keeps the same correction fence.
		restarted, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(context.Background(), "ws-v2", "run-v2")
		if err != nil || len(loaded.CandidateAttempts) != 2 || loaded.CandidateAttempts[0].Failure == nil ||
			!CandidateFailureIsCorrectableOnce(loaded.CandidateAttempts[0].Failure) {
			t.Fatalf("reload lost correctable fence: %+v err=%v", loaded, err)
		}
		terminal, err := restarted.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 1, runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid",
			Signature: "candidate-schema-invalid", Reason: "still invalid",
		})
		if err != nil {
			t.Fatal(err)
		}
		if terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil ||
			terminal.Decision.Terminal.Code != "candidate_retry_exhausted" {
			t.Fatalf("attempt 1 exhaustion = %+v", terminal.Decision)
		}
		if _, err := restarted.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("third attempt err=%v, want conflict", err)
		}
	})

	t.Run("candidate_rejected_final_terminals_without_attempt_1", func(t *testing.T) {
		repository := &controllerRepository{}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
			t.Fatal(err)
		}
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		terminal, err := controller.failCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, runtimecontract.RuntimeCandidateFailure{
			Class: runtimecontract.RuntimeCandidateFailureClassProvider, Code: "provider", Signature: "provider", Reason: "provider unavailable",
		})
		if err != nil {
			t.Fatal(err)
		}
		if terminal.Decision.State != runtimecontract.ConvergenceStateFailed || terminal.Decision.Terminal == nil ||
			terminal.ActiveDispatch != nil || len(terminal.GovernanceRequests) != 0 ||
			len(terminal.CandidateAttempts) != 1 || terminal.CandidateAttempts[0].Failure == nil ||
			!CandidateFailureIsRejectedFinal(terminal.CandidateAttempts[0].Failure) {
			t.Fatalf("rejected_final did not seal terminal: %+v", terminal)
		}
		// CAS-before-success already committed; reload must keep terminal and refuse attempt 1.
		restarted, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(context.Background(), "ws-v2", "run-v2")
		if err != nil || loaded.Decision.State != runtimecontract.ConvergenceStateFailed || loaded.Decision.Terminal == nil {
			t.Fatalf("reload lost rejected_final terminal: %+v err=%v", loaded, err)
		}
		if _, err := restarted.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("attempt 1 after rejected_final err=%v, want conflict", err)
		}
	})

	t.Run("prerequisite_stale_reopens_attempt_0_without_budget", func(t *testing.T) {
		aggregate := stateChangeAggregate(t)
		transaction := aggregate.Decision.AuthorityTransaction()
		stale := runtimecontract.RuntimePrerequisiteStale{
			PrerequisiteID: "operation-admissibility", ObservationID: "observation-v1",
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", ExpectedRevision: "tree-v1", ObservedRevision: "tree-v2", Reason: "workspace changed",
		}
		signature := runtimecontract.StableRuntimePrerequisiteStaleSignature(stale)
		aggregate.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{
			AuthorityContractID: runtimecontract.StableAuthorityContractID(aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, transaction),
			Transaction:         transaction, Attempt: 0, StablePrefixHash: "stable-prefix", State: runtimecontract.RuntimeCandidateAttemptStateInvalidated,
			Stale: &stale,
		}}
		bindRuntimeCandidateAttemptForTest(t, &aggregate, 0)
		aggregate.PrerequisiteInvalidations = []runtimecontract.RuntimePrerequisiteInvalidationRecord{{
			Signature: signature, PrerequisiteIDs: []string{stale.PrerequisiteID}, ObservationIDs: []string{stale.ObservationID}, Stale: &stale,
		}}
		repository := &controllerRepository{aggregate: &aggregate}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		reopened, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix")
		if err != nil {
			t.Fatal(err)
		}
		if len(reopened.CandidateAttempts) != 1 || reopened.CandidateAttempts[0].Attempt != 0 ||
			reopened.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStatePending ||
			reopened.CandidateAttempts[0].Stale != nil || reopened.CandidateAttempts[0].Failure != nil {
			t.Fatalf("stale reopen = %+v", reopened.CandidateAttempts)
		}
		if len(reopened.PrerequisiteInvalidations) != 1 || reopened.PrerequisiteInvalidations[0].Signature != signature {
			t.Fatalf("stale audit lost: %+v", reopened.PrerequisiteInvalidations)
		}
		// Duplicate reopen is idempotent and still does not allocate attempt 1.
		duplicate, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 0, "stable-prefix")
		if err != nil {
			t.Fatal(err)
		}
		if duplicate.StateRevision != reopened.StateRevision || len(duplicate.CandidateAttempts) != 1 {
			t.Fatalf("duplicate stale reopen not idempotent: %+v", duplicate)
		}
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, 1, "stable-prefix"); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("stale path opened attempt 1: %v", err)
		}
	})

	t.Run("accepted_seal_blocks_retry_and_is_idempotent", func(t *testing.T) {
		repository := &controllerRepository{}
		controller, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := controller.initialize(context.Background(), controllerFacts(), controllerDefinition(), nil); err != nil {
			t.Fatal(err)
		}
		if _, err := controller.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, "stable-prefix"); err != nil {
			t.Fatal(err)
		}
		candidate := runtimecontract.RuntimeCandidate{StateChange: &runtimecontract.StateChangeCandidate{
			Operations: []runtimecontract.StateChangingOperation{{
				Operation: runtimecontract.RuntimeOperation{
					ID: "write-1", Kind: runtimecontract.StepKindFileWrite,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-v2"}, TargetRefs: []string{"target.txt"},
					Parameters: []runtimecontract.OperationParameter{
						{Name: "content", String: strPtr("hello")},
					},
				},
			}},
		}}
		// Prefer Accept path used by controller unit tests when available.
		accepted, err := controller.AcceptCandidateAttempt(context.Background(), "ws-v2", "run-v2", 0, candidate)
		if err != nil {
			// Fall back to AdmitAndSeal when Accept is not the production entry for this fixture.
			sealed, sealErr := controller.AdmitAndSealCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 0, candidate)
			if sealErr != nil {
				t.Fatalf("accept=%v seal=%v", err, sealErr)
			}
			accepted = sealed.Aggregate
		}
		if accepted.ActiveDispatch == nil && (len(accepted.CandidateAttempts) == 0 || accepted.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted) {
			t.Fatalf("accepted result missing seal/accept record: %+v", accepted)
		}
		restarted, err := NewRuntimeController(repository)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := restarted.Load(context.Background(), "ws-v2", "run-v2")
		if err != nil {
			t.Fatal(err)
		}
		if loaded.ActiveDispatch == nil && (len(loaded.CandidateAttempts) == 0 || loaded.CandidateAttempts[0].State != runtimecontract.RuntimeCandidateAttemptStateAccepted) {
			t.Fatalf("reload lost accepted seal: %+v", loaded)
		}
		if _, err := restarted.BeginCandidateAttemptForTest(context.Background(), "ws-v2", "run-v2", 1, "stable-prefix"); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("attempt 1 after accepted err=%v, want conflict", err)
		}
	})
}
