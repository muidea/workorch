package runtime

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAssessmentApplicabilitySealUsesIndependentObservationPrefix(t *testing.T) {
	reads := (runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: "definition-v2", Entries: []runtimecontract.RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "typed-owner/ws-v2", ResourceKind: "workspace_target", ResourceID: "target", Revision: "historical", DependencyHash: "selector"}}}).WithHash()
	pair := &runtimecontract.RuntimeEvidenceApplicabilityDependencies{ProtocolRef: runtimecontract.RuntimeEvidenceApplicabilityDependenciesV1, OwnerRevisionSet: reads}
	_, repository, _ := prepareOwnerFactAssessmentWithDependencyPair(t, true, nil, &reads, pair)
	sealed := repository.aggregate.CloneRaw()
	if sealed.ActiveDispatch == nil || sealed.ActiveDispatch.AssessmentAuthorityInput == nil {
		t.Fatal("missing assessment seal")
	}
	previous := sealed.CloneRaw()
	previous.ActiveDispatch = nil // Independent valid pre-seal source fixture.
	previous.CandidateAttempts = previous.CandidateAttempts[:len(previous.CandidateAttempts)-1]
	if err := validateAssessmentApplicabilitySealTransition(previous, sealed); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"omit_prefix", "replace_prefix", "replace_generation", "prior_generation"} {
		t.Run(name, func(t *testing.T) {
			after := sealed.CloneRaw()
			before := previous.CloneRaw()
			switch name {
			case "omit_prefix":
				after.ActiveDispatch.AssessmentAuthorityInput.ApplicabilityObservations = nil
			case "replace_prefix":
				after.ActiveDispatch.AssessmentAuthorityInput.ApplicabilityObservations[0].Request.IssuedAtStateRevision++
			case "replace_generation":
				after.ActiveDispatch.AssessmentAuthorityInput.ApplicabilityReconciliationRequestHash = "forged"
			case "prior_generation":
				before.Reconciliation.RequestHash = "new-generation"
				after.ActiveDispatch.AssessmentAuthorityInput.ApplicabilityReconciliationRequestHash = "new-generation"
			}
			if err := validateAssessmentApplicabilitySealTransition(before, after); err == nil {
				t.Fatal("new seal accepted missing/stale/coordinated observation fence")
			}
		})
	}
	before := sealed.CloneRaw()
	after := sealed.CloneRaw()
	after.Reconciliation.RequestHash = "later-generation"
	if err := validateAssessmentApplicabilitySealTransition(before, after); err != nil || !reflect.DeepEqual(after.ActiveDispatch.AssessmentAuthorityInput, sealed.ActiveDispatch.AssessmentAuthorityInput) {
		t.Fatalf("historical existing dispatch rewritten by later Resume: %v", err)
	}
}
