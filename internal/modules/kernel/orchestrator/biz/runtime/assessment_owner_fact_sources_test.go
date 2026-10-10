package runtime

import (
	"context"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// catalog 是显式 typed owner fixture；只验证消费/CAS，不冒充真实 capability producer。
func TestRuntimeAssessmentOwnerFactSourcesSurviveReloadAndRejectCoherentTampering(t *testing.T) {
	for _, ready := range []bool{true, false} {
		t.Run(map[bool]string{true: "satisfied", false: "repairable"}[ready], func(t *testing.T) {
			controller, repository, bootstrap := prepareOwnerFactAssessmentWithDiagnostic(t, ready, nil)
			before, err := repository.Load(context.Background(), "ws-v2", "run-v2")
			if err != nil || before.ActiveDispatch == nil || before.ActiveDispatch.AssessmentAuthorityInput == nil {
				t.Fatalf("assessment source preimage: %v", err)
			}
			completed, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventAssessmentObserved, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"}, Authority: &RuntimeInternalAuthorityPayload{}})
			if err != nil {
				t.Fatal(err)
			}
			aggregate := completed.Aggregate
			dispatchIndex, factIndex := runtimeAssessmentAdjudicationIndexes(t, aggregate)
			adjudication := aggregate.CompletedDispatches[dispatchIndex].AssessmentAdjudication
			if len(adjudication.OwnerFactSources) != 1 || adjudication.OwnerFactSources[0].OwnerRevisionSet == nil {
				t.Fatal("assessment lost consumed owner postcondition catalog")
			}
			source := adjudication.OwnerFactSources[0]
			found := 0
			for _, catalog := range before.ActiveDispatch.AssessmentAuthorityInput.OperationAdmissibilities {
				if catalog.SnapshotID == source.CatalogSnapshotID && catalog.OwnerModuleID == source.Requirement.OwnerModuleID {
					found++
				}
			}
			if found != 1 || source.OwnerRevisionSet.DefinitionRevision != before.Facts.DefinitionRevision || source.OwnerRevisionSet.Validate() != nil {
				t.Fatal("source not rooted in unique pre-seal owner catalog")
			}
			if aggregate.CompletedDispatches[dispatchIndex].DispatchAuthorityHash != before.ActiveDispatch.DispatchAuthorityHash {
				t.Fatal("postcondition source changed immutable dispatch authority")
			}
			laterFacts := before.Facts.Normalized()
			laterFacts.Resources.OperationAdmissibilities = nil
			lineageSources := runtimeLineageAssertionSources(before)
			lineageSources.Facts = laterFacts
			historical, err := runtimecontract.DeriveRuntimeAssessmentAdjudication(sealedDispatchToRuntimeRecord(*before.ActiveDispatch), laterFacts, sealedDispatchesToRuntimeRecords(before.CompletedDispatches), before.CoverageProofLineage, lineageSources)
			if err != nil || !reflect.DeepEqual(historical.OwnerFactSources, adjudication.OwnerFactSources) {
				t.Fatalf("later mutable catalog replaced historical sealed source: %v", err)
			}
			for _, clone := range []RuntimeAggregate{aggregate.CloneRaw(), runtimeTestEvidenceAggregateRoundTrip(t, aggregate)} {
				if err := clone.Validate(); err != nil {
					t.Fatal(err)
				}
				clone.CompletedDispatches[dispatchIndex].AssessmentAdjudication.OwnerFactSources[0].OwnerRevisionSet.Entries[0].Revision = "caller-change"
				if source.OwnerRevisionSet.Entries[0].Revision == "caller-change" {
					t.Fatal("raw/reload source dependency shared")
				}
			}
			for _, field := range []string{"missing", "catalog", "requirement", "revision", "selector", "definition", "raw", "hash"} {
				t.Run(field, func(t *testing.T) {
					bad := aggregate.CloneRaw()
					dispatch := &bad.CompletedDispatches[dispatchIndex]
					value := &dispatch.AssessmentAdjudication.OwnerFactSources[0]
					switch field {
					case "missing":
						dispatch.AssessmentAdjudication.OwnerFactSources = nil
					case "catalog":
						value.CatalogSnapshotID = "newer-catalog"
					case "requirement":
						value.Requirement.ValidationProofRequirementHash = "validation-proof-requirement/forged"
					case "revision":
						value.OwnerRevisionSet.Entries[0].Revision = "newer-fact"
						*value.OwnerRevisionSet = value.OwnerRevisionSet.WithHash()
					case "selector":
						value.OwnerRevisionSet.Entries[0].DependencyHash = "other-selector"
						*value.OwnerRevisionSet = value.OwnerRevisionSet.WithHash()
					case "definition":
						value.OwnerRevisionSet.DefinitionRevision = "other"
						*value.OwnerRevisionSet = value.OwnerRevisionSet.WithHash()
					case "raw":
						value.OwnerRevisionSet.Entries[0].ResourceID = " " + value.OwnerRevisionSet.Entries[0].ResourceID
					case "hash":
						value.OwnerRevisionSet.SetHash = "forged"
					}
					dispatch.AssessmentAdjudicationHash = runtimecontract.StableRuntimeAssessmentAdjudicationHash(*dispatch.AssessmentAdjudication)
					bad.Facts.Assessments[factIndex].AdjudicationHash = dispatch.AssessmentAdjudicationHash
					if err := bad.Validate(); err == nil {
						t.Fatal("coherent double-entry hashes laundered sealed catalog source")
					}
				})
			}
			saves := repository.saves
			restarted, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := restarted.initialize(context.Background(), bootstrap, aggregate.Definition, nil)
			if err != nil || repository.saves != saves || !reflect.DeepEqual(loaded.CompletedDispatches[dispatchIndex].AssessmentAdjudication.OwnerFactSources, adjudication.OwnerFactSources) {
				t.Fatalf("restart changed original catalog source: %v", err)
			}
			if _, err := restarted.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventAssessmentObserved, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"}, Authority: &RuntimeInternalAuthorityPayload{}}); err == nil || repository.saves != saves {
				t.Fatal("duplicate source observation consumed another CAS")
			}
		})
	}
}
