package workorchruntimecontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeDispatchTargetAuthorizationFreshLoadRejectsTamper(t *testing.T) {
	baseline := runtimeDispatchIntrinsicFixture(t, 0)
	if err := runtimeDispatchTargetAuthorizationFreshLoad(baseline); err != nil {
		t.Fatalf("valid sealed target authorization did not survive fresh load: %v", err)
	}

	for _, test := range []struct {
		name    string
		mutate  func(*RuntimeDispatchRecord)
		wantErr string
	}{
		{
			name: "missing manifest identity",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.CandidateManifestHash = ""
			},
			wantErr: "requires id, scope, authority and candidate hash",
		},
		{
			name: "manifest identity drift without authority rehash",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.CandidateManifestHash = "manifest-drifted"
			},
			wantErr: "dispatch authority hash mismatch",
		},
		{
			name: "missing per-operation snapshot",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationTargetAuthorizations = nil
				dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(*dispatch)
			},
			wantErr: "exactly one target authorization",
		},
		{
			name: "snapshot excluded sealed target with coordinated authority rehash",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationTargetAuthorizations[0].Target.ExactTargetRefs = []string{"other.txt"}
				dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(*dispatch)
			},
			wantErr: "target_outside_sealed_set",
		},
		{
			name: "snapshot expansion without authority rehash",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationTargetAuthorizations[0].Target.ExactTargetRefs = []string{"other.txt", "target.txt"}
			},
			wantErr: "dispatch authority hash mismatch",
		},
		{
			name: "operation identity drift",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationTargetAuthorizations[0].OperationHash = "operation-hash-drift"
				dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(*dispatch)
			},
			wantErr: "target authorization hash mismatch",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dispatch := baseline.Clone()
			test.mutate(&dispatch)
			err := runtimeDispatchTargetAuthorizationFreshLoad(dispatch)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fresh-load error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestRuntimeDispatchAdmissionLineageBindsCandidateManifestHash(t *testing.T) {
	dispatch := runtimeDispatchIntrinsicFixture(t, 0)
	attempt := RuntimeCandidateAttemptRecord{
		Transaction: dispatch.Transaction, State: RuntimeCandidateAttemptStateAccepted,
		ManifestHash: dispatch.CandidateManifestHash, DispatchID: dispatch.ID,
		CandidateHash: dispatch.CandidateHash, DispatchAuthorityHash: dispatch.DispatchAuthorityHash,
	}
	if err := ValidateRuntimeDispatchAdmissionLineage(&dispatch, nil, []RuntimeCandidateAttemptRecord{attempt}, nil); err != nil {
		t.Fatalf("matching manifest lineage rejected: %v", err)
	}

	drifted := dispatch.Clone()
	drifted.CandidateManifestHash = "manifest-drifted"
	drifted.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(drifted)
	attempt.DispatchAuthorityHash = drifted.DispatchAuthorityHash
	if err := ValidateRuntimeDispatchAdmissionLineage(&drifted, nil, []RuntimeCandidateAttemptRecord{attempt}, nil); err == nil || !strings.Contains(err.Error(), "exactly one accepted-attempt") {
		t.Fatalf("manifest-drifted admission lineage error = %v", err)
	}
}

func TestRuntimeDispatchTargetAuthorizationRequiresOperationBijectionOnlyForOperationCandidates(t *testing.T) {
	first := RuntimeOperation{ID: "op-1", Kind: StepKindFileWrite, TargetRefs: []string{"one.txt"}}
	second := RuntimeOperation{ID: "op-2", Kind: StepKindFileWrite, TargetRefs: []string{"two.txt"}}
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: first}, {Operation: second}}}}
	authorizations := []RuntimeDispatchOperationTargetAuthorization{
		{OperationID: first.ID, OperationHash: StableRuntimeOperationHash(first), RequireTargets: true, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"one.txt", "two.txt"}}},
		{OperationID: second.ID, OperationHash: StableRuntimeOperationHash(second), RequireTargets: true, Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"one.txt", "two.txt"}}},
	}
	if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, authorizations); err != nil {
		t.Fatalf("one target snapshot per operation rejected: %v", err)
	}
	duplicate := CloneRuntimeDispatchOperationTargetAuthorizations(authorizations)
	duplicate[1] = duplicate[0]
	if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, duplicate); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate target snapshot error = %v", err)
	}

	for name, candidate := range map[string]RuntimeCandidate{
		"assessment": {Assessment: &AssessmentCandidate{}},
		"governance": {Governance: &GovernanceCandidate{}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, nil); err != nil {
				t.Fatalf("non-operation candidate with empty target snapshots: %v", err)
			}
			if err := ValidateRuntimeDispatchOperationTargetAuthorizations(candidate, authorizations[:1]); err == nil || !strings.Contains(err.Error(), "cannot carry") {
				t.Fatalf("non-operation candidate target snapshot error = %v", err)
			}
		})
	}

	hybrid := RuntimeCandidate{
		StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: first}}},
		Assessment:  &AssessmentCandidate{},
	}
	if err := ValidateRuntimeDispatchOperationTargetAuthorizations(hybrid, authorizations[:1]); err == nil || !strings.Contains(err.Error(), "exactly one candidate kind") {
		t.Fatalf("hybrid candidate target authorization error = %v", err)
	}
}

func runtimeDispatchTargetAuthorizationFreshLoad(dispatch RuntimeDispatchRecord) error {
	record := RuntimeStateRecord{ActiveDispatch: &dispatch}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	var loaded RuntimeStateRecord
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	return ValidateRuntimeDispatchHistoryShape(dispatch.WorkspaceID, dispatch.RunID, loaded.ActiveDispatch, nil)
}
