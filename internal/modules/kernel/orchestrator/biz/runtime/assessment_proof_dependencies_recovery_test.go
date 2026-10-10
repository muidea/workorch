package runtime

import (
	"context"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAssessmentSelectedProofDependenciesSurviveCASFailureAndReplyLoss(t *testing.T) {
	for _, afterCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_commit", true: "after_commit_reply_loss"}[afterCommit], func(t *testing.T) {
			dependencies := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: "definition-v2", Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
				OwnerKind: "capability", OwnerInstanceID: "typed-owner/ws-v2", ResourceKind: "workspace_target", ResourceID: "observed-input", Revision: "original-bytes", DependencyHash: "original-selector",
			}}}).WithHash()
			_, prepared, _ := prepareOwnerFactAssessmentWithOwnerDependencies(t, true, nil, &dependencies)
			before, err := prepared.Load(context.Background(), "ws-v2", "run-v2")
			if err != nil {
				t.Fatal(err)
			}
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}, failBefore: !afterCommit, failAfter: afterCommit}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			event := RuntimeInternalEvent{Kind: RuntimeInternalEventAssessmentObserved, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"}, Authority: &RuntimeInternalAuthorityPayload{}}
			if _, err := controller.ApplyInternalEvent(context.Background(), event); err == nil {
				t.Fatal("assessment CAS fault was lost")
			}
			persisted, err := repository.Load(context.Background(), "ws-v2", "run-v2")
			if err != nil || persisted.Validate() != nil {
				t.Fatalf("assessment fault recovery preimage: %v", err)
			}
			if !afterCommit && !reflect.DeepEqual(persisted, before) {
				t.Fatal("failed assessment CAS polluted original receipt or dependencies")
			}
			reopened, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			result, replayErr := reopened.ApplyInternalEvent(context.Background(), event)
			if afterCommit {
				if replayErr == nil {
					t.Fatal("reply loss allowed duplicate assessment commit")
				}
			} else {
				if replayErr != nil {
					t.Fatal(replayErr)
				}
				persisted = result.Aggregate
			}
			dispatchIndex, _ := runtimeAssessmentAdjudicationIndexes(t, persisted)
			adjudication := persisted.CompletedDispatches[dispatchIndex].AssessmentAdjudication
			proofs := adjudication.Proofs
			expectedSource, err := runtimecontract.DeriveRuntimeAssessmentAdjudication(sealedDispatchToRuntimeRecord(*before.ActiveDispatch), before.Facts, sealedDispatchesToRuntimeRecords(before.CompletedDispatches), before.CoverageProofLineage, runtimeLineageAssertionSources(before))
			if err != nil || len(expectedSource.OwnerFactSources) != 1 || !reflect.DeepEqual(adjudication.OwnerFactSources, expectedSource.OwnerFactSources) {
				t.Fatalf("assessment recovery changed sealed owner postcondition source: %v", err)
			}
			if repository.saves != 1 || persisted.StateRevision != before.StateRevision+1 || len(persisted.Facts.Assessments) != 1 || len(proofs) != 1 || !reflect.DeepEqual(proofs[0].OwnerRevisionSet, &dependencies) {
				t.Fatal("assessment recovery repeated commit or replaced original selected dependencies")
			}
		})
	}
}
