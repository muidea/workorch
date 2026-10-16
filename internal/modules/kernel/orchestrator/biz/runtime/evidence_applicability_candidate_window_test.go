package runtime

import (
	"context"
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Provider/session creation is an explicit fixture; this matrix proves the
// private coordinated write set, not real-provider or public authoring behavior.
func applicabilityCandidateWindowFixture(t *testing.T, window string) (RuntimeAggregate, runtimecontract.RuntimeEvidenceApplicabilityObservation) {
	t.Helper()
	before, matching := applicabilityTransitionFixture(t)
	repository := &controllerRepository{aggregate: &before}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := controller.ApplyInternalEvent(ctx, applicabilityInternalEvent(matching)); err != nil {
		t.Fatal(err)
	}
	if window == "pending" || window == "proposal" {
		if _, err := controller.BeginCandidateAttemptForTest(ctx, "ws-v2", "run-v2", 0, "applicability-window-prefix"); err != nil {
			t.Fatal(err)
		}
		if window == "proposal" {
			candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: repository.aggregate.Facts.Focus.ID, CoverageWorkItemIDs: repository.aggregate.Facts.Focus.CoverageWorkItemIDs, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "fixture claim", EvidenceRefs: []string{string(repository.aggregate.Facts.Evidence.Accepted[0].ID)}}}
			if _, err := controller.recordCandidateProposal(ctx, "ws-v2", "run-v2", 0, candidate); err != nil {
				t.Fatal(err)
			}
		}
	} else {
		manifest, err := BuildRuntimeCandidateContractManifestForAggregate(*repository.aggregate)
		if err != nil {
			t.Fatal(err)
		}
		_, session, err := controller.ensureCandidateRequestSession(ctx, "ws-v2", "run-v2", runtimecontract.RuntimeProviderDocumentOperations, "applicability-window-prefix", manifest.Hash, "")
		if err != nil {
			t.Fatal(err)
		}
		if window != "prepared" {
			physical := runtimecontract.StableRuntimeCandidatePhysicalCallID(session.LogicalRequestID, session.NextSequence)
			if _, err := controller.markCandidateRequestSubmitted(ctx, "ws-v2", "run-v2", session.LogicalRequestID, session.NextSequence, physical); err != nil {
				t.Fatal(err)
			}
			if window == "completed_document" {
				if _, err := controller.completeCandidateRequestDocument(ctx, "ws-v2", "run-v2", session.LogicalRequestID, physical); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	before = repository.aggregate.CloneRaw()
	request, err := CompileRuntimeEvidenceApplicabilityRequest(before, before.Facts.Evidence.Accepted[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	stale := matching.CloneRaw()
	stale.Request = request
	stale.RecordedAtStateRevision = request.IssuedAtStateRevision + 1
	stale.CurrentDependencies.Entries[0].Revision = "actual-later-resource"
	*stale.CurrentDependencies = stale.CurrentDependencies.WithHash()
	return before, stale
}

func TestEvidenceApplicabilityRetiresUnsealedCandidateWindowsWithSingleCAS(t *testing.T) {
	for _, window := range []string{"prepared", "completed_document", "pending", "proposal"} {
		for _, fault := range []string{"none", "before_cas", "after_cas"} {
			t.Run(window+"/"+fault, func(t *testing.T) {
				before, observation := applicabilityCandidateWindowFixture(t, window)
				repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}, failBefore: fault == "before_cas", failAfter: fault == "after_cas"}
				controller, err := NewRuntimeController(repository)
				if err != nil {
					t.Fatal(err)
				}
				result, err := controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation))
				if fault != "none" {
					if err == nil {
						t.Fatal("CAS fault was lost")
					}
					reloaded := runtimeTestEvidenceAggregateRoundTrip(t, *repository.aggregate)
					repository.aggregate = &reloaded
					controller, err = NewRuntimeController(repository)
					if err != nil {
						t.Fatal(err)
					}
					result, err = controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation))
				}
				if err != nil {
					t.Fatal(err)
				}
				after := result.Aggregate
				if after.StateRevision != before.StateRevision+1 || repository.saves != 1 || !reflect.DeepEqual(after.OperationReceipts, before.OperationReceipts) || !reflect.DeepEqual(after.ExecutionBudget, before.ExecutionBudget) || !reflect.DeepEqual(after.Facts.Recovery, before.Facts.Recovery) {
					t.Fatal("retirement repeated CAS or changed original effect/budget/recovery")
				}
				if window == "pending" || window == "proposal" {
					attempt := after.CandidateAttempts[len(after.CandidateAttempts)-1]
					if attempt.State != runtimecontract.RuntimeCandidateAttemptStateInvalidated || attempt.Proposal != nil || attempt.Failure != nil || attempt.Correction != nil || attempt.ApplicabilityInvalidationRequest == nil || !reflect.DeepEqual(*attempt.ApplicabilityInvalidationRequest, observation.Request) {
						t.Fatal("old candidate retained authority or got a fabricated retry/failure")
					}
					if !reflect.DeepEqual(after.CandidateRequestSessions, before.CandidateRequestSessions) {
						t.Fatal("bound complete document audit session was rewritten")
					}
					changed := after.CloneRaw()
					changed.CandidateAttempts[len(changed.CandidateAttempts)-1].ApplicabilityInvalidationRequest.SourceReceiptHash = "forged"
					*changed.CandidateAttempts[len(changed.CandidateAttempts)-1].ApplicabilityInvalidationRequest = changed.CandidateAttempts[len(changed.CandidateAttempts)-1].ApplicabilityInvalidationRequest.WithID()
					if changed.Validate() == nil || reflect.DeepEqual(changed.CandidateAttempts, after.CandidateAttempts) {
						t.Fatal("invalidation source forgery or pointer alias survived reload gate")
					}
				} else {
					session := after.CandidateRequestSessions[len(after.CandidateRequestSessions)-1]
					if session.State != runtimecontract.RuntimeCandidateRequestSessionTerminal || session.LastOutcome.Signature != observation.Request.RequestID || session.ProviderSubmitted {
						t.Fatal("unadmitted request retained authority")
					}
				}
				if err := runtimeTestEvidenceAggregateRoundTrip(t, after).Validate(); err != nil {
					t.Fatal(err)
				}
				if _, err := controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation)); err != nil || repository.saves != 1 {
					t.Fatalf("duplicate retirement changed original: %v", err)
				}
			})
		}
	}
}

func TestEvidenceApplicabilityCannotRetireUnsettledProviderAllocation(t *testing.T) {
	before, observation := applicabilityCandidateWindowFixture(t, "submitted")
	repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, required, err := NextRuntimeEvidenceApplicabilityRequest(before); err != nil || required {
		t.Fatalf("fresh query displaced original submission settlement: %v", err)
	}
	if _, err := controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation)); err == nil || repository.saves != 0 || !reflect.DeepEqual(*repository.aggregate, before) {
		t.Fatal("owner observation stranded original provider allocation")
	}
}

func TestEvidenceApplicabilityUnsettledProviderGuardIncludesHistoricalAuthority(t *testing.T) {
	before, _ := applicabilityCandidateWindowFixture(t, "submitted")
	// Predicate-only fixture: an unrelated authority identity never makes an
	// unresolved submission disappear. This does not authorize rewriting a
	// persisted session or manufacture a production late-settlement path.
	before.CandidateRequestSessions[len(before.CandidateRequestSessions)-1].Transaction.AuthoritySnapshotID = "historical-owner-authority"
	if !runtimeApplicabilityHasUnsettledProvider(before) {
		t.Fatal("current-read retirement ignored historical provider responsibility")
	}
	before.CandidateRequestSessions[len(before.CandidateRequestSessions)-1].State = runtimecontract.RuntimeCandidateRequestSessionDocumentComplete
	if runtimeApplicabilityHasUnsettledProvider(before) {
		t.Fatal("settled complete document was treated as an unfinished submission")
	}
}

func TestEvidenceApplicabilityRetirementRejectsLateProposalAndSealWithoutCAS(t *testing.T) {
	for _, window := range []string{"pending", "proposal"} {
		t.Run(window, func(t *testing.T) {
			before, observation := applicabilityCandidateWindowFixture(t, window)
			manifest, err := BuildRuntimeCandidateContractManifestForAggregate(before)
			if err != nil {
				t.Fatal(err)
			}
			attempt := before.CandidateAttempts[len(before.CandidateAttempts)-1]
			candidate := runtimecontract.RuntimeCandidate{Assessment: &runtimecontract.AssessmentCandidate{FocusID: before.Facts.Focus.ID, CoverageWorkItemIDs: before.Facts.Focus.CoverageWorkItemIDs, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "late fixture claim", EvidenceRefs: []string{string(before.Facts.Evidence.Accepted[0].ID)}}}
			if attempt.Proposal != nil {
				candidate = *attempt.Proposal
			}
			repository := &implementationWorkDetachedRepository{controllerRepository: controllerRepository{aggregate: &before}}
			controller, err := NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			result, err := controller.ApplyInternalEvent(context.Background(), applicabilityInternalEvent(observation))
			if err != nil {
				t.Fatal(err)
			}
			reloaded := runtimeTestEvidenceAggregateRoundTrip(t, result.Aggregate)
			repository.aggregate = &reloaded
			controller, err = NewRuntimeController(repository)
			if err != nil {
				t.Fatal(err)
			}
			writes := repository.saves
			if _, err := controller.admitCandidateProposal(context.Background(), "ws-v2", "run-v2", attempt.Attempt, manifest, attempt.LogicalRequestID, candidate); err == nil {
				t.Fatal("late proposal revived applicability-retired attempt")
			}
			if _, err := controller.admitAndSealCandidateAttempt(context.Background(), "ws-v2", "run-v2", attempt.Attempt, manifest, attempt.LogicalRequestID, candidate); err == nil {
				t.Fatal("late seal revived applicability-retired attempt")
			}
			if repository.saves != writes || !reflect.DeepEqual(*repository.aggregate, reloaded) {
				t.Fatal("late admission changed history, effect or budget")
			}
		})
	}
}
