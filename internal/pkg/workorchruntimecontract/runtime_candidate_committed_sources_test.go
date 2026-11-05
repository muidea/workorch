package workorchruntimecontract

import (
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCommittedCandidateSourcesAdmitOnlyChangedExactProofGroups(t *testing.T) {
	after := candidateSourceCompilerFixture(t, 5)
	before := after.Clone()
	before.CandidateAttempts = before.CandidateAttempts[:4]
	before.CandidateRequestSessions = before.CandidateRequestSessions[:4]
	before.CompletedDispatches = before.CompletedDispatches[:4]
	original := after.Clone()
	pending, inputs := runtimeCandidateSourcesRequiringAdmission(&before, after.ActiveDispatch, after.CompletedDispatches, after.CandidateAttempts, after.CandidateRequestSessions)
	if len(pending) != 1 || len(inputs) != 1 || !reflect.DeepEqual(pending[0], after.CandidateAttempts[4]) || !reflect.DeepEqual(inputs[0], after.CandidateRequestSessions[4]) {
		t.Fatal("source selection did not isolate the new exact proof")
	}
	s := newRuntimeSemanticValidationSession()
	s.committedPreimage = &before
	if err := validateRuntimeAcceptedCandidateManifestLineageInSession(after.ActiveDispatch, after.CompletedDispatches, after.CandidateAttempts, after.CandidateRequestSessions, s); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, original) {
		t.Fatal("proof selection mutated original input")
	}
	for name, mutate := range map[string]func(*RuntimeStateRecord){
		"duplicate request": func(r *RuntimeStateRecord) {
			r.CandidateRequestSessions = append(r.CandidateRequestSessions, r.CandidateRequestSessions[0])
		},
		"duplicate dispatch": func(r *RuntimeStateRecord) {
			r.CompletedDispatches = append(r.CompletedDispatches, r.CompletedDispatches[0])
		},
		"changed manifest": func(r *RuntimeStateRecord) {
			r.CandidateRequestSessions[0].CandidateManifest.Hash = "replacement self hash"
		},
		"missing dispatch":   func(r *RuntimeStateRecord) { r.CompletedDispatches = r.CompletedDispatches[1:] },
		"new missing source": func(r *RuntimeStateRecord) { r.CandidateRequestSessions = r.CandidateRequestSessions[:4] },
	} {
		t.Run(name, func(t *testing.T) {
			r := after.Clone()
			mutate(&r)
			if err := validateRuntimeAcceptedCandidateManifestLineageInSession(r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions, s); err == nil {
				t.Fatal("invalid current group borrowed a formal result")
			}
		})
	}
}

func TestCommittedCandidateAuthorityCannotBeReplacedOrRemoved(t *testing.T) {
	before := candidateSourceCompilerFixture(t, 2)
	for name, mutate := range map[string]func(*RuntimeStateRecord){
		"attempt body":   func(r *RuntimeStateRecord) { r.CandidateAttempts[0].CandidateHash = "coordinated replacement" },
		"request body":   func(r *RuntimeStateRecord) { r.CandidateRequestSessions[0].LastPhysicalCallID = "replacement original" },
		"remove attempt": func(r *RuntimeStateRecord) { r.CandidateAttempts = r.CandidateAttempts[1:] },
		"remove request": func(r *RuntimeStateRecord) { r.CandidateRequestSessions = r.CandidateRequestSessions[1:] },
	} {
		t.Run(name, func(t *testing.T) {
			after := before.Clone()
			mutate(&after)
			if err := ValidateRuntimeCommittedHistoryTransition(before, after); !errors.Is(err, common.ErrConflict) {
				t.Fatal("accepted original changed", err)
			}
		})
	}
}

func TestCommittedDispatchConsumptionKeepsCurrentPartitionAndRawGates(t *testing.T) {
	before := candidateSourceCompilerFixture(t, 2)
	for i := range before.CompletedDispatches {
		before.CompletedDispatches[i].State = RuntimeDispatchStateInvalidated
	}
	check := func(r RuntimeStateRecord) error {
		return validateRuntimeDispatchHistoryTransition(r.Facts.WorkspaceID, r.Facts.RunID, r.ActiveDispatch, r.CompletedDispatches, &before)
	}
	if err := check(before); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeStateRecord){
		"duplicate": func(r *RuntimeStateRecord) {
			r.CompletedDispatches = append(r.CompletedDispatches, r.CompletedDispatches[0])
		},
		"scope": func(r *RuntimeStateRecord) { r.Facts.RunID = "other" },
		"changed raw": func(r *RuntimeStateRecord) {
			r.CompletedDispatches[0].CandidateHash = "same declared authority different body"
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := before.Clone()
			mutate(&r)
			if check(r) == nil {
				t.Fatal("current invalidity borrowed intrinsic result")
			}
		})
	}
}
