package runtime

import (
	"context"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 使用显式 typed receipt 替身验证 assessment 消费；不是实际 Shell copy 的 producer oracle。
func TestRuntimeAssessmentSelectedProofPreservesOriginalOwnerDependencies(t *testing.T) {
	for _, ready := range []bool{true, false} {
		t.Run(map[bool]string{true: "satisfied", false: "repairable"}[ready], func(t *testing.T) {
			dependencies := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: "definition-v2", Entries: []runtimecontract.RuntimeOwnerRevisionRef{{
				OwnerKind: "capability", OwnerInstanceID: "typed-owner/ws-v2", ResourceKind: "workspace_target", ResourceID: "observed-input",
				Revision: "original-bytes", DependencyHash: "original-selector",
			}}}).WithHash()
			controller, repository, bootstrap := prepareOwnerFactAssessmentWithOwnerDependencies(t, ready, nil, &dependencies)
			before, err := repository.Load(context.Background(), "ws-v2", "run-v2")
			if err != nil || before.Validate() != nil {
				t.Fatalf("assessment preimage: %v", err)
			}
			// 调用方持有的 producer 集合不能改写 receipt/fact 或后续选中 proof。
			dependencies.Entries[0].Revision = "caller-change"
			originalDependencies := before.OperationReceipts[0].Evidence.OwnerRevisionSet
			completed, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
				Kind: RuntimeInternalEventAssessmentObserved, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"}, Authority: &RuntimeInternalAuthorityPayload{},
			})
			if err != nil {
				t.Fatal(err)
			}
			aggregate := completed.Aggregate
			dispatchIndex, factIndex := runtimeAssessmentAdjudicationIndexes(t, aggregate)
			adjudication := aggregate.CompletedDispatches[dispatchIndex].AssessmentAdjudication
			if len(adjudication.Proofs) != 1 || !reflect.DeepEqual(adjudication.Proofs[0].OwnerRevisionSet, originalDependencies) {
				t.Fatal("assessment did not preserve the exact selected receipt dependency body")
			}
			if adjudication.Proofs[0].OwnerRevisionSet.Entries[0].Revision != "original-bytes" {
				t.Fatal("assessment used caller/newer dependency instead of persisted receipt")
			}
			// 纯 classifier 的 cutoff 反例；此人工副本不提交 aggregate，不能冒充 owner 来源。
			classifierFacts := before.Facts.Normalized()
			future := classifierFacts.Evidence.Accepted[0]
			future.ID = "future-evidence"
			future.Revision = "runtime-evidence-99999999999999999999"
			future.OwnerRevisionSet = runtimecontract.CloneRuntimeOwnerRevisionSet(future.OwnerRevisionSet)
			future.OwnerRevisionSet.Entries[0].Revision = "future-bytes"
			*future.OwnerRevisionSet = future.OwnerRevisionSet.WithHash()
			classifierFacts.Evidence.Accepted = append(classifierFacts.Evidence.Accepted, future)
			sources := runtimeLineageAssertionSources(before)
			sources.Facts = classifierFacts
			cutoffProof, err := runtimecontract.DeriveRuntimeAssessmentAdjudication(sealedDispatchToRuntimeRecord(*before.ActiveDispatch), classifierFacts, sealedDispatchesToRuntimeRecords(before.CompletedDispatches), before.CoverageProofLineage, sources)
			if err != nil || !reflect.DeepEqual(cutoffProof, *adjudication) {
				t.Fatalf("future evidence changed historical selected dependencies: %v", err)
			}
			for _, invalid := range []string{"raw", "hash", "definition"} {
				facts := before.Facts.Normalized()
				set := facts.Evidence.Accepted[0].OwnerRevisionSet
				switch invalid {
				case "raw":
					set.Entries[0].ResourceID = " observed-input"
				case "hash":
					set.SetHash = "forged"
				case "definition":
					set.DefinitionRevision = "other"
					*set = set.WithHash()
				}
				sources := runtimeLineageAssertionSources(before)
				sources.Facts = facts
				if _, err := runtimecontract.DeriveRuntimeAssessmentAdjudication(sealedDispatchToRuntimeRecord(*before.ActiveDispatch), facts, sealedDispatchesToRuntimeRecords(before.CompletedDispatches), before.CoverageProofLineage, sources); err == nil {
					t.Fatalf("pure classifier accepted invalid selected dependencies: %s", invalid)
				}
			}
			if aggregate.CompletedDispatches[dispatchIndex].DispatchAuthorityHash != before.ActiveDispatch.DispatchAuthorityHash {
				t.Fatal("post-seal proof dependencies changed dispatch authority")
			}
			for _, clone := range []RuntimeAggregate{aggregate.CloneRaw(), runtimeTestEvidenceAggregateRoundTrip(t, aggregate)} {
				if err := clone.Validate(); err != nil {
					t.Fatal(err)
				}
				clone.CompletedDispatches[dispatchIndex].AssessmentAdjudication.Proofs[0].OwnerRevisionSet.Entries[0].Revision = "clone-change"
				if adjudication.Proofs[0].OwnerRevisionSet.Entries[0].Revision != "original-bytes" {
					t.Fatal("aggregate/reload shares selected proof dependencies")
				}
			}
			for _, scenario := range []string{"missing", "revision", "selector", "definition", "raw", "hash"} {
				t.Run(scenario, func(t *testing.T) {
					changed := aggregate.CloneRaw()
					dispatch := &changed.CompletedDispatches[dispatchIndex]
					set := dispatch.AssessmentAdjudication.Proofs[0].OwnerRevisionSet
					switch scenario {
					case "missing":
						dispatch.AssessmentAdjudication.Proofs[0].OwnerRevisionSet = nil
					case "revision":
						set.Entries[0].Revision = "other"
						*set = set.WithHash()
					case "selector":
						set.Entries[0].DependencyHash = "other"
						*set = set.WithHash()
					case "definition":
						set.DefinitionRevision = "other"
						*set = set.WithHash()
					case "raw":
						set.Entries[0].ResourceID = " observed-input"
					case "hash":
						set.SetHash = "forged"
					}
					// 两侧 adjudication hash 一起改写仍必须反查原 evidence/receipt。
					dispatch.AssessmentAdjudicationHash = runtimecontract.StableRuntimeAssessmentAdjudicationHash(*dispatch.AssessmentAdjudication)
					changed.Facts.Assessments[factIndex].AdjudicationHash = dispatch.AssessmentAdjudicationHash
					if err := changed.Validate(); err == nil {
						t.Fatal("coherent assessment hashes laundered original proof dependencies")
					}
				})
			}
			saves := repository.saves
			reopened, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			reloaded, err := reopened.initialize(context.Background(), bootstrap, aggregate.Definition, nil)
			if err != nil || repository.saves != saves || !reflect.DeepEqual(reloaded.CompletedDispatches[dispatchIndex].AssessmentAdjudication, adjudication) {
				t.Fatalf("restart changed selected proof dependencies: %v", err)
			}
			if _, err := reopened.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{
				Kind: RuntimeInternalEventAssessmentObserved, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"}, Authority: &RuntimeInternalAuthorityPayload{},
			}); err == nil || repository.saves != saves {
				t.Fatal("duplicate assessment consumed another save")
			}
		})
	}
}
