package runtime

import (
	"context"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAssessmentApplicabilityProjectionReverseProvesOriginalReceipt(t *testing.T) {
	reads := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: "definition-v2", Entries: []runtimecontract.RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "typed-owner/ws-v2", ResourceKind: "workspace_target", ResourceID: "target", Revision: "historical-input", DependencyHash: "read-selector"}}}).WithHash()
	current := *runtimecontract.CloneRuntimeOwnerRevisionSet(&reads)
	current.Entries[0].ResourceKind = "workspace_target_content"
	current.Entries[0].Revision = "actual-postcondition"
	current.Entries[0].DependencyHash = "current-selector"
	current = current.WithHash()
	applicability := &runtimecontract.RuntimeEvidenceApplicabilityDependencies{ProtocolRef: runtimecontract.RuntimeEvidenceApplicabilityDependenciesProtocol, OwnerRevisionSet: current}
	controller, repository, _ := prepareOwnerFactAssessmentWithDependencyPair(t, true, nil, &reads, applicability)
	before := repository.aggregate.CloneRaw()
	result, err := controller.ApplyInternalEvent(context.Background(), RuntimeInternalEvent{Kind: RuntimeInternalEventAssessmentObserved, Scope: RuntimeInternalEventScope{WorkspaceID: "ws-v2", RunID: "run-v2"}, Authority: &RuntimeInternalAuthorityPayload{}})
	if err != nil {
		t.Fatal(err)
	}
	aggregate := result.Aggregate
	dispatchIndex, factIndex := runtimeAssessmentAdjudicationIndexes(t, aggregate)
	proof := aggregate.CompletedDispatches[dispatchIndex].AssessmentAdjudication.Proofs[0]
	if !reflect.DeepEqual(proof.OwnerRevisionSet, &reads) || !reflect.DeepEqual(proof.ApplicabilityDependencies, applicability) {
		t.Fatal("assessment did not preserve both original receipt observations")
	}
	for _, copy := range []RuntimeAggregate{aggregate.CloneRaw(), runtimeTestEvidenceAggregateRoundTrip(t, aggregate)} {
		if err := copy.Validate(); err != nil {
			t.Fatal(err)
		}
		copy.CompletedDispatches[dispatchIndex].AssessmentAdjudication.Proofs[0].ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision = "caller-change"
		if proof.ApplicabilityDependencies.OwnerRevisionSet.Entries[0].Revision != "actual-postcondition" {
			t.Fatal("raw clone/reload shared applicability")
		}
	}
	for _, name := range []string{"removed", "revision", "selector", "definition", "protocol", "raw"} {
		t.Run(name, func(t *testing.T) {
			changed := aggregate.CloneRaw()
			dispatch := &changed.CompletedDispatches[dispatchIndex]
			value := dispatch.AssessmentAdjudication.Proofs[0].ApplicabilityDependencies
			switch name {
			case "removed":
				dispatch.AssessmentAdjudication.Proofs[0].ApplicabilityDependencies = nil
			case "revision":
				value.OwnerRevisionSet.Entries[0].Revision = "forged"
			case "selector":
				value.OwnerRevisionSet.Entries[0].DependencyHash = "forged"
			case "definition":
				value.OwnerRevisionSet.DefinitionRevision = "other"
			case "protocol":
				value.ProtocolRef += " "
			case "raw":
				value.OwnerRevisionSet.Entries[0].ResourceID += " "
			}
			if value != nil && name != "raw" {
				value.OwnerRevisionSet = value.OwnerRevisionSet.WithHash()
			}
			dispatch.AssessmentAdjudicationHash = runtimecontract.StableRuntimeAssessmentAdjudicationHash(*dispatch.AssessmentAdjudication)
			changed.Facts.Assessments[factIndex].AdjudicationHash = dispatch.AssessmentAdjudicationHash
			if err := changed.Validate(); err == nil {
				t.Fatal("coordinated assessment rehash bypassed original receipt")
			}
		})
	}
	if !reflect.DeepEqual(before.OperationReceipts, aggregate.OperationReceipts) || !reflect.DeepEqual(before.Facts.Evidence, aggregate.Facts.Evidence) {
		t.Fatal("assessment rewrote original receipt or evidence")
	}
}
