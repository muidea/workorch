package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAggregateEvidenceCommitLedgerRejectsAcceptedFactDriftAfterFreshLoad(t *testing.T) {
	baseline := validationAggregate(t)
	if err := baseline.Validate(); err != nil {
		t.Fatalf("valid accepted receipt aggregate rejected: %v", err)
	}
	if len(baseline.Facts.Evidence.Accepted) != 1 || len(baseline.Facts.Evidence.Accepted[0].CoverageWorkItemIDs) == 0 {
		t.Fatalf("accepted receipt fixture lacks exact CWI credit: %+v", baseline.Facts.Evidence.Accepted)
	}
	factID := baseline.Facts.Evidence.Accepted[0].ID

	for _, test := range []struct {
		name    string
		mutate  func(*RuntimeAggregate)
		wantErr string
	}{
		{
			name: "deleted fact",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted = nil
			},
			wantErr: "must exist exactly once",
		},
		{
			name: "moved to rejected ledger",
			mutate: func(value *RuntimeAggregate) {
				fact := value.Facts.Evidence.Accepted[0]
				value.Facts.Evidence.Accepted = nil
				value.Facts.Evidence.Rejected = []runtimecontract.EvidenceFact{fact}
			},
			wantErr: "accepted/rejected ledger",
		},
		{
			name: "focus drift with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted[0].FocusID = "focus-forged"
				runtimeTestRebindEvidenceFactHash(t, value, factID)
			},
			wantErr: "dispatch/focus/authority/operation",
		},
		{
			name: "receipt payload drift with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted[0].Summary = "coordinated forged summary"
				runtimeTestRebindEvidenceFactHash(t, value, factID)
			},
			wantErr: "payload does not match",
		},
		{
			name: "binding drift with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted[0].BindingRefs = []string{"binding/forged"}
				runtimeTestRebindEvidenceFactHash(t, value, factID)
			},
			wantErr: "binding refs do not match",
		},
		{
			name: "cleared CWI credit with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted[0].CoverageWorkItemIDs = nil
				runtimeTestRebindEvidenceFactHash(t, value, factID)
			},
			wantErr: "coverage does not match",
		},
		{
			name: "reason drift with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Evidence.Accepted[0].EvidenceReason = "coordinated forged reason"
				runtimeTestRebindEvidenceFactHash(t, value, factID)
			},
			wantErr: "reason does not match",
		},
		{
			name: "duplicate processed fence",
			mutate: func(value *RuntimeAggregate) {
				value.ProcessedEventIDs = append(value.ProcessedEventIDs, string(factID))
			},
			wantErr: "one processed-event fence",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := runtimeTestEvidenceAggregateRoundTrip(t, baseline)
			test.mutate(&value)
			value = runtimeTestEvidenceAggregateRoundTrip(t, value)
			if err := validateRuntimeAggregateEvidenceCommitLedger(value); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fresh-load accepted fact error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestRuntimeAggregateEvidenceCommitLedgerRejectsOrphanOperationFacts(t *testing.T) {
	baseline := validationAggregate(t)
	original := baseline.Facts.Evidence.Accepted[0]
	for _, test := range []struct {
		name     string
		rejected bool
		mutate   func(*runtimecontract.EvidenceFact)
		wantErr  string
	}{
		{name: "accepted orphan", wantErr: "no exact evidence-committed execution"},
		{name: "rejected orphan", rejected: true, wantErr: "no exact evidence-committed execution"},
		{name: "accepted missing focus", mutate: func(fact *runtimecontract.EvidenceFact) { fact.FocusID = "" }, wantErr: "requires focus, dispatch and operation lineage"},
		{name: "accepted missing dispatch", mutate: func(fact *runtimecontract.EvidenceFact) { fact.DispatchID = "" }, wantErr: "requires focus, dispatch and operation lineage"},
		{name: "accepted missing operation", mutate: func(fact *runtimecontract.EvidenceFact) { fact.OperationID = "" }, wantErr: "requires focus, dispatch and operation lineage"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := runtimeTestEvidenceAggregateRoundTrip(t, baseline)
			orphan := original
			orphan.ID = common.EvidenceRecordID("runtime-receipt|orphan-" + strings.ReplaceAll(test.name, " ", "-"))
			orphan.Revision = "runtime-evidence-99999999999999999999"
			if test.mutate != nil {
				test.mutate(&orphan)
			}
			if test.rejected {
				value.Facts.Evidence.Rejected = append(value.Facts.Evidence.Rejected, orphan)
			} else {
				value.Facts.Evidence.Accepted = append(value.Facts.Evidence.Accepted, orphan)
			}
			value.ProcessedEventIDs = append(value.ProcessedEventIDs, string(orphan.ID))
			value = runtimeTestEvidenceAggregateRoundTrip(t, value)
			if err := validateRuntimeAggregateEvidenceCommitLedger(value); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fresh-load orphan fact error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestRuntimeAggregateEvidenceCommitLedgerRejectsAcceptedValidationFactDrift(t *testing.T) {
	baseline := runtimeTestAcceptedValidationEvidenceAggregate(t)
	if err := baseline.Validate(); err != nil {
		t.Fatalf("valid accepted validation aggregate rejected: %v", err)
	}
	if len(baseline.Facts.Evidence.Accepted) < 2 {
		t.Fatalf("accepted validation fixture has no validation fact: %+v", baseline.Facts.Evidence.Accepted)
	}
	var fact runtimecontract.EvidenceFact
	for _, candidate := range baseline.Facts.Evidence.Accepted {
		if candidate.Kind == runtimecontract.EvidenceKindValidation {
			fact = candidate
			break
		}
	}
	if fact.ID == "" {
		t.Fatalf("accepted validation fixture has no validation fact: %+v", baseline.Facts.Evidence.Accepted)
	}

	for _, test := range []struct {
		name    string
		mutate  func(*RuntimeAggregate)
		wantErr string
	}{
		{
			name: "deleted validation fact",
			mutate: func(value *RuntimeAggregate) {
				kept := value.Facts.Evidence.Accepted[:0]
				for _, candidate := range value.Facts.Evidence.Accepted {
					if candidate.ID != fact.ID {
						kept = append(kept, candidate)
					}
				}
				value.Facts.Evidence.Accepted = kept
			},
			wantErr: "must exist exactly once",
		},
		{
			name: "validation observation drift with coordinated mutable hash",
			mutate: func(value *RuntimeAggregate) {
				for index := range value.Facts.Evidence.Accepted {
					if value.Facts.Evidence.Accepted[index].ID == fact.ID {
						value.Facts.Evidence.Accepted[index].Validation.RecipeID = "recipe/forged"
					}
				}
				runtimeTestRebindEvidenceFactHash(t, value, fact.ID)
			},
			wantErr: "payload does not match",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := runtimeTestEvidenceAggregateRoundTrip(t, baseline)
			test.mutate(&value)
			value = runtimeTestEvidenceAggregateRoundTrip(t, value)
			if err := validateRuntimeAggregateEvidenceCommitLedger(value); err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("fresh-load accepted validation fact error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func runtimeTestAcceptedValidationEvidenceAggregate(t *testing.T) RuntimeAggregate {
	t.Helper()
	repository := &durableRuntimeStateRepository{aggregate: validationAggregate(t)}
	coordinator, err := NewDispatchCoordinator(repository)
	if err != nil {
		t.Fatal(err)
	}
	sealed := sealCandidateForTest(t, repository, validationCandidate())
	evidence := ExecutionEvidence{
		EventID: "accepted-validation-evidence-invariant", WorkspaceID: sealed.Facts.WorkspaceID, RunID: sealed.Facts.RunID,
		DispatchID: sealed.ActiveDispatch.ID, AuthoritySnapshotID: sealed.ActiveDispatch.Transaction.AuthoritySnapshotID,
		OperationID: runtimeTestActiveOperationID(t, sealed, 0), Kind: runtimecontract.EvidenceKindValidation,
		Status: ExecutionEvidenceStatusAccepted,
	}
	attachRuntimeTestOwnerReceipt(t, &repository.aggregate, &evidence)
	committed, err := coordinator.commitEvidence(context.Background(), evidence)
	if err != nil {
		t.Fatalf("commit accepted validation fixture: %v", err)
	}
	if committed.Disposition != CommitDispositionCommitted {
		t.Fatalf("accepted validation fixture disposition = %q, aggregate=%+v", committed.Disposition, committed.Aggregate)
	}
	validationPersisted := false
	for _, fact := range committed.Aggregate.Facts.Evidence.Accepted {
		if fact.Kind == runtimecontract.EvidenceKindValidation {
			validationPersisted = true
			break
		}
	}
	if len(committed.Aggregate.Facts.Evidence.Accepted) < 2 || !validationPersisted {
		t.Fatalf("accepted validation fixture did not persist validation fact: %+v", committed.Aggregate.Facts.Evidence.Accepted)
	}
	return committed.Aggregate
}

func runtimeTestRebindEvidenceFactHash(t *testing.T, aggregate *RuntimeAggregate, factID common.EvidenceRecordID) {
	t.Helper()
	var fact *runtimecontract.EvidenceFact
	for index := range aggregate.Facts.Evidence.Accepted {
		if aggregate.Facts.Evidence.Accepted[index].ID == factID {
			fact = &aggregate.Facts.Evidence.Accepted[index]
		}
	}
	for index := range aggregate.Facts.Evidence.Rejected {
		if aggregate.Facts.Evidence.Rejected[index].ID == factID {
			fact = &aggregate.Facts.Evidence.Rejected[index]
		}
	}
	if fact == nil {
		t.Fatalf("fact %q is missing", factID)
	}
	factHash := runtimecontract.StableRuntimeEvidenceFactHash(*fact)
	for dispatchIndex := range aggregate.CompletedDispatches {
		for executionIndex := range aggregate.CompletedDispatches[dispatchIndex].OperationExecutions {
			execution := &aggregate.CompletedDispatches[dispatchIndex].OperationExecutions[executionIndex]
			if execution.EvidenceFactID == factID {
				execution.EvidenceFactHash = factHash
			}
		}
	}
	if aggregate.ActiveDispatch != nil {
		for executionIndex := range aggregate.ActiveDispatch.OperationExecutions {
			execution := &aggregate.ActiveDispatch.OperationExecutions[executionIndex]
			if execution.EvidenceFactID == factID {
				execution.EvidenceFactHash = factHash
			}
		}
	}
}

func runtimeTestEvidenceAggregateRoundTrip(t *testing.T, aggregate RuntimeAggregate) RuntimeAggregate {
	t.Helper()
	payload, err := json.Marshal(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	var loaded RuntimeAggregate
	if err := json.Unmarshal(payload, &loaded); err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestRuntimeAggregateRejectsOwnerFactAcquisitionTerminalCredit(t *testing.T) {
	for _, retireManifest := range []bool{false, true} {
		name := "active_manifest"
		if retireManifest {
			name = "retired_manifest"
		}
		t.Run(name, func(t *testing.T) {
			_, repository, _, _ := prepareTerminalValidationOwnerFactAcquisitionAssessment(t, retireManifest)
			base := *repository.aggregate
			if err := base.Validate(); err != nil {
				t.Fatalf("valid acquisition aggregate rejected: %v", err)
			}

			corrupt := base
			corrupt.Facts = base.Facts.Normalized()
			corrupt.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
			corrupt.Facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
			if err := corrupt.Validate(); err == nil || !strings.Contains(err.Error(), "acquisition CWI") {
				t.Fatalf("acquisition terminal credit validation err=%v", err)
			}
		})
	}
}

func TestRuntimeAggregateRejectsRetiredFailureTargetPrerequisite(t *testing.T) {
	aggregate := validationAggregate(t)
	aggregate.Facts.Prerequisites.Pending = append(aggregate.Facts.Prerequisites.Pending, runtimecontract.RuntimePrerequisite{
		ID: "failure_target:retired", Kind: runtimecontract.PrerequisiteKind("failure_target"),
	})
	if err := aggregate.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("retired failure_target prerequisite validation err=%v", err)
	}
}

func TestRuntimeAggregateSemanticPurposeInvariantFailsClosedWithoutExactLineage(t *testing.T) {
	_, repository, _, _ := prepareTerminalValidationOwnerFactAcquisitionAssessment(t)
	base := *repository.aggregate

	withoutAssessment := base
	withoutAssessment.Facts = base.Facts.Normalized()
	withoutAssessment.Facts.Focus.RequireAssessment = false
	if err := validateRuntimeSemanticBindingPurposeInvariant(withoutAssessment); err == nil || !strings.Contains(err.Error(), "requires assessment") {
		t.Fatalf("acquisition without assessment validation err=%v", err)
	}

	missing := base
	missing.Facts = base.Facts.Normalized()
	ref := missing.Facts.Coverage[0].CurrentBindingRef.Clone()
	ref.SemanticManifestHash = "missing-semantic-manifest"
	missing.Facts.Coverage[0].CurrentBindingRef = &ref
	missing.Facts.Coverage[0].BindingLineage[len(missing.Facts.Coverage[0].BindingLineage)-1] = ref
	if err := validateRuntimeSemanticBindingPurposeInvariant(missing); err == nil || !strings.Contains(err.Error(), "no exact semantic purpose lineage") {
		t.Fatalf("missing semantic purpose validation err=%v", err)
	}
}

func TestRuntimeAggregateSemanticPurposeInvariantAllowsDefinitionTerminalCredit(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts = aggregate.Facts.Normalized()
	aggregate.Facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateSatisfied
	aggregate.Facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateSettled
	if err := validateRuntimeSemanticBindingPurposeInvariant(aggregate); err != nil {
		t.Fatalf("Definition terminal proof was rejected: %v", err)
	}
}

func TestRuntimeAggregateValidationPhasesRejectOwnedInvalidState(t *testing.T) {
	base := validationAggregate(t)
	if err := base.Validate(); err != nil {
		t.Fatalf("valid aggregate rejected: %v", err)
	}

	tests := []struct {
		name     string
		mutate   func(*RuntimeAggregate)
		validate func(RuntimeAggregate) error
	}{
		{
			name: "sm2 authority",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.WorkspaceID = ""
			},
			validate: validateRuntimeAggregateSM2Authority,
		},
		{
			name: "sm3 active dispatch",
			mutate: func(value *RuntimeAggregate) {
				value.ActiveDispatch = &SealedDispatch{}
			},
			validate: validateRuntimeAggregateSM3ActiveDispatch,
		},
		{
			name: "activation fence",
			mutate: func(value *RuntimeAggregate) {
				value.Termination = &runtimecontract.RuntimeTerminationRecord{}
			},
			validate: validateRuntimeAggregateActivationFence,
		},
		{
			name: "sm3 candidate transaction",
			mutate: func(value *RuntimeAggregate) {
				value.CandidateAttempts = []runtimecontract.RuntimeCandidateAttemptRecord{{}}
			},
			validate: validateRuntimeAggregateSM3CandidateTransaction,
		},
		{
			name: "governance ledger",
			mutate: func(value *RuntimeAggregate) {
				value.GovernanceRequests = []runtimecontract.RuntimeGovernanceRequestRecord{{}}
			},
			validate: validateRuntimeAggregateGovernanceLedgers,
		},
		{
			name: "sm4 delivery acknowledgement",
			mutate: func(value *RuntimeAggregate) {
				value.DeliveryAcknowledgements = []runtimecontract.RuntimeDeliveryAcknowledgement{{}}
			},
			validate: validateRuntimeAggregateSM4DeliveryAcknowledgements,
		},
		{
			name: "sm4 non-terminal delivery budget",
			mutate: func(value *RuntimeAggregate) {
				value.Facts.Recovery.TerminalDeliveryAttempts = 1
			},
			validate: validateRuntimeAggregateSM4DeliveryAcknowledgements,
		},
		{
			name: "sm2 repair ledger",
			mutate: func(value *RuntimeAggregate) {
				value.PrerequisiteInvalidations = []runtimecontract.RuntimePrerequisiteInvalidationRecord{{}}
			},
			validate: validateRuntimeAggregateSM2RepairLedgers,
		},
		{
			name: "sm4 drive lease",
			mutate: func(value *RuntimeAggregate) {
				value.DriveLease = &runtimecontract.RuntimeDriveLease{}
			},
			validate: validateRuntimeAggregateSM4DriveLease,
		},
		{
			name: "cross-machine governance fence",
			mutate: func(value *RuntimeAggregate) {
				value.GovernanceRequests = []runtimecontract.RuntimeGovernanceRequestRecord{{}}
			},
			validate: validateRuntimeAggregateCrossMachineFences,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.mutate(&value)
			if err := test.validate(value); err == nil {
				t.Fatal("invalid state was accepted by its validation phase")
			}
		})
	}
}
