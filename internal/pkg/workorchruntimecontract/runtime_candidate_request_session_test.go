package workorchruntimecontract

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeCandidateRequestIdentityIsTotalAndDeterministic(t *testing.T) {
	for _, test := range []struct {
		name     string
		logical  string
		sequence int
		physical string
		wantErr  bool
	}{
		{name: "complete", logical: "candidate-request-1", sequence: 1, physical: "candidate-call-1"},
		{name: "empty", wantErr: true},
		{name: "missing logical", sequence: 1, physical: "candidate-call-1", wantErr: true},
		{name: "zero sequence", logical: "candidate-request-1", physical: "candidate-call-1", wantErr: true},
		{name: "missing physical", logical: "candidate-request-1", sequence: 1, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := (RuntimeCandidateRequestIdentity{LogicalRequestID: test.logical, Sequence: test.sequence, PhysicalCallID: test.physical}).Validate()
			if test.wantErr {
				if !errors.Is(err, common.ErrInvalidArgument) {
					t.Fatalf("RuntimeCandidateRequestIdentity.Validate() error = %v, want invalid argument", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	first := StableRuntimeCandidatePhysicalCallID("candidate-request-1", 1)
	if first == "" || first != StableRuntimeCandidatePhysicalCallID("candidate-request-1", 1) || first == StableRuntimeCandidatePhysicalCallID("candidate-request-1", 2) {
		t.Fatalf("physical call ID is not stable per logical request/sequence: %q", first)
	}
	base := &RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "schema-v1", Reason: "volatile reason"}
	if StableRuntimeCandidateRequestCorrectionHash(nil) != "" || StableRuntimeCandidateRequestCorrectionHash(base) == "" {
		t.Fatal("correction hash did not distinguish initial and correction request")
	}
	changedReason := *base
	changedReason.Reason = "another volatile reason"
	if StableRuntimeCandidateRequestCorrectionHash(base) != StableRuntimeCandidateRequestCorrectionHash(&changedReason) {
		t.Fatal("correction hash must not depend on unstable reason text")
	}
	normalized := (RuntimeCandidateRequestIdentity{LogicalRequestID: "  candidate-request-1 ", Sequence: 1, PhysicalCallID: " candidate-call-1  "}).Normalized()
	want := RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-1", Sequence: 1, PhysicalCallID: "candidate-call-1"}
	if normalized != want || !SameRuntimeCandidateRequestIdentity(normalized, want) {
		t.Fatalf("normalized request identity = %+v, want %+v", normalized, want)
	}
	changedSequence := want
	changedSequence.Sequence++
	if SameRuntimeCandidateRequestIdentity(want, changedSequence) {
		t.Fatal("request identities with different physical sequences were equal")
	}
}

func TestRuntimeCandidateOutputContinuationPreservesRawTail(t *testing.T) {
	request := RuntimeCandidateOutputContinuationRequest{ID: "candidate-request-content-1", Sequence: 2, PreviousChunkTail: "}\n  "}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	if normalized := request.Normalized(); normalized.PreviousChunkTail != request.PreviousChunkTail {
		t.Fatalf("continuation tail was rewritten: %q", normalized.PreviousChunkTail)
	}
}
