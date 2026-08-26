package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestValidateRuntimeGovernanceRequestDispatchLineageBindsCompleteDependency(t *testing.T) {
	context := workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "artifact.bin"))
	dependency := ExternalDependency{
		Kind: "approval", SubjectRef: "artifact.bin", Reason: "owner approval required",
		RequestedAction: "publish", RiskLevel: "high", AuthorizationContext: context,
	}.Normalized()
	dispatch := RuntimeDispatchRecord{
		ID: "governance-dispatch-1", Transaction: AuthorityTransaction{AuthoritySnapshotID: "snapshot-1"},
		Candidate: RuntimeCandidate{Governance: &GovernanceCandidate{ExternalDependency: dependency}},
	}
	request := RuntimeGovernanceRequestRecord{
		ApprovalID: "approval-1", AuthoritySnapshotID: "snapshot-1", DispatchID: dispatch.ID,
		DependencyKind: dependency.Kind, SubjectRef: dependency.SubjectRef, RequestedAction: dependency.RequestedAction,
		RiskLevel: dependency.RiskLevel, AuthorizationHash: StableRuntimeAuthorizationContextHash(dependency.AuthorizationContext),
	}
	if err := ValidateRuntimeGovernanceRequestDispatchLineage([]RuntimeGovernanceRequestRecord{request}, &dispatch, nil); err != nil {
		t.Fatalf("valid governance request lineage: %v", err)
	}

	for name, mutate := range map[string]func(*RuntimeGovernanceRequestRecord){
		"kind":               func(value *RuntimeGovernanceRequestRecord) { value.DependencyKind = "decision" },
		"subject":            func(value *RuntimeGovernanceRequestRecord) { value.SubjectRef = "other" },
		"action":             func(value *RuntimeGovernanceRequestRecord) { value.RequestedAction = "delete" },
		"risk":               func(value *RuntimeGovernanceRequestRecord) { value.RiskLevel = "low" },
		"authorization hash": func(value *RuntimeGovernanceRequestRecord) { value.AuthorizationHash = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := request
			mutate(&changed)
			if err := ValidateRuntimeGovernanceRequestDispatchLineage([]RuntimeGovernanceRequestRecord{changed}, &dispatch, nil); err == nil {
				t.Fatal("drifted governance request was accepted")
			}
		})
	}

	nonCanonical := request
	nonCanonical.SubjectRef = " " + nonCanonical.SubjectRef
	if err := ValidateRuntimeGovernanceRequestRecords([]RuntimeGovernanceRequestRecord{nonCanonical}); err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("non-canonical request error = %v", err)
	}
}
