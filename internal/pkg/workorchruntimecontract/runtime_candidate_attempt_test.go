package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeCandidateAttemptScopeValidationRequiresExplicitContractAndRejectsDuplicate(t *testing.T) {
	facts := testRuntimeFacts()
	decision := validDecisionForState(ConvergenceStateNeedStateChange)
	current := decision.AuthorityTransaction()
	resealed := current
	contractID := StableAuthorityContractID(facts.WorkspaceID, facts.RunID, current)
	failure := &RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassContract, Reason: "invalid candidate"}
	values := []RuntimeCandidateAttemptRecord{
		{Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateFailed, Failure: failure},
		{AuthorityContractID: contractID, Transaction: current, Attempt: 1, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-1", State: RuntimeCandidateAttemptStatePending},
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err == nil {
		t.Fatal("candidate attempt without authority contract id was accepted")
	}
	values[0].AuthorityContractID = contractID
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err != nil {
		t.Fatalf("explicit same-contract retry rejected: %v", err)
	}
	duplicate := current
	duplicate.AuthoritySnapshotID = "duplicate-snapshot"
	duplicate.Attempt = 1
	values = []RuntimeCandidateAttemptRecord{
		{AuthorityContractID: contractID, Transaction: current, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateFailed, Failure: failure},
		{AuthorityContractID: contractID, Transaction: duplicate, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-duplicate", State: RuntimeCandidateAttemptStateFailed, Failure: failure},
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err == nil {
		t.Fatal("new duplicate attempt 0 was accepted across snapshots")
	}
}

func TestRuntimeCandidateAttemptBindingContradictionValidatesAndClones(t *testing.T) {
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	failure := FailureFact{
		Class: FailureClassCandidateContractFailure, Code: "operation_target_exists", Signature: "file/target-exists",
		Message: "file/write is create-only", TargetRefs: []string{"target.txt"},
	}
	feedback, err := RuntimeFeedbackFromFailureFact(RuntimeFeedbackScope{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: transaction.AuthoritySnapshotID,
	}, RuntimeFeedbackDomainFeasibility, RuntimeFeedbackCategory(RuntimeOwnerFeasibilityCategoryPolicyDisallowed), RuntimeModuleFile, failure)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = "write-target"
	feedback.OriginalDiagnostic.ObservedRevision = "workspace-2"
	policy := RuntimeOwnerPolicyIdentity{OwnerModuleID: RuntimeModuleFile, PolicyRevision: "file-owner-feasibility", CatalogSnapshotID: "catalog-1"}
	observation := RuntimeOwnerFeasibilityObservation{
		Outcome: RuntimeOwnerFeasibilityInfeasible, Feedback: &feedback, OwnerPolicy: policy,
		ConstraintImpact: RuntimeOwnerFeasibilityConstraintImpactSealedTargetAuthorizationContradicted,
	}
	contradiction := RuntimeBindingContradictionIdentity{
		WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthorityInputID: transaction.AuthorityInputID,
		BindingID: "binding-1", BindingSetHash: "binding-set-1", ProposalHash: "proposal-1", ObservationHash: "observation-1",
		OwnerPolicy: policy, ObservedRevision: "workspace-2",
	}
	record := RuntimeCandidateAttemptRecord{
		AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction), Transaction: transaction,
		Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0",
		State: RuntimeCandidateAttemptStateInvalidated, OwnerFeasibility: &observation, BindingContradiction: &contradiction,
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{record}); err != nil {
		t.Fatal(err)
	}
	state := RuntimeStateRecord{CandidateAttempts: []RuntimeCandidateAttemptRecord{record}}
	clone := state.Clone()
	record.BindingContradiction.BindingID = "mutated"
	record.OwnerFeasibility.Feedback.OriginalDiagnostic.OriginalMessage = "mutated"
	if clone.CandidateAttempts[0].BindingContradiction == nil || clone.CandidateAttempts[0].BindingContradiction.BindingID != "binding-1" ||
		clone.CandidateAttempts[0].OwnerFeasibility == nil || clone.CandidateAttempts[0].OwnerFeasibility.Feedback.OriginalDiagnostic.OriginalMessage != "file/write is create-only" {
		t.Fatalf("binding contradiction clone shared mutable identity: %+v", clone.CandidateAttempts[0])
	}
	tampered := clone.CandidateAttempts[0]
	tampered.BindingContradiction.AuthorityInputID = "other-authority-input"
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{tampered}); err == nil {
		t.Fatal("candidate attempt accepted a contradiction from another authority input")
	}
}

func TestRuntimeCandidateAttemptScopeValidationRejectsRunawayAttempts(t *testing.T) {
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	failure := &RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassContract, Reason: "invalid candidate"}
	values := make([]RuntimeCandidateAttemptRecord, 0, 3)
	contractID := StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction)
	for attempt := 0; attempt < 3; attempt++ {
		resealed := transaction
		resealed.AuthoritySnapshotID = "resealed-snapshot-" + string(rune('a'+attempt))
		resealed.Attempt = attempt
		values = append(values, RuntimeCandidateAttemptRecord{
			AuthorityContractID: contractID, Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-runaway",
			State: RuntimeCandidateAttemptStateFailed, Failure: failure,
		})
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err == nil {
		t.Fatal("runaway candidate attempts were accepted")
	}
}

func TestRuntimeCandidateFeasibilityRecordValidationRejectsAmbiguousOrAcceptedLedger(t *testing.T) {
	failure := &FailureFact{Class: FailureClassCandidateContractFailure, Code: "target_invalid", Signature: "file/target_invalid", Message: "target is invalid"}
	feedback, err := RuntimeFeedbackFromFailureFact(RuntimeFeedbackScope{WorkspaceID: "ws-1", RunID: "run-1"}, RuntimeFeedbackDomainFeasibility, RuntimeFeedbackCategory(RuntimeOwnerFeasibilityCategoryTargetNotRegular), RuntimeModuleFile, *failure)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := RuntimeOwnerFeasibilityObservationFromOutcome(RuntimeOwnerFeasibilityInfeasible, &feedback, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	values := []RuntimeCandidateFeasibilityRecord{
		{OperationID: "operation-1", AdmissibilitySnapshotID: "inventory-1", CandidateManifestHash: "manifest-1", Observation: &observation},
		{OperationID: "operation-1", AdmissibilitySnapshotID: "inventory-1", CandidateManifestHash: "manifest-1", Observation: &RuntimeOwnerFeasibilityObservation{Outcome: RuntimeOwnerFeasibilityFeasible}},
	}
	if err := ValidateRuntimeCandidateFeasibilityRecords(values); err == nil {
		t.Fatal("duplicate candidate feasibility operation was accepted")
	}
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	values = values[:1]
	attempt := RuntimeCandidateAttemptRecord{
		AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction), Transaction: transaction,
		Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateAccepted, Feasibilities: values,
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{attempt}); err == nil {
		t.Fatal("accepted candidate attempt retained an unsealed feasibility ledger")
	}
}

func TestRuntimeStateRecordRejectsTerminalDecisionWithPendingCandidateAttempt(t *testing.T) {
	facts := testRuntimeFacts()
	binding := CoverageEvidenceBinding{
		ID: "binding/cwi-1", CoverageWorkItemID: "cwi-1", FocusID: facts.Focus.ID, DefinitionRevision: facts.DefinitionRevision,
		Proofs:     []CoverageEvidenceProof{{EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{RuntimeBlockKindPatchApply}}},
		TargetRefs: []string{"target.txt"}, DeclarationRef: "test/runtime-state",
	}.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	facts.Focus.RequiredEvidenceKinds = []string{EvidenceKindStateChange}
	facts.Focus.TargetRefs = []string{"target.txt"}
	facts.Focus.AcceptedStepKinds = []string{RuntimeBlockKindPatchApply}
	facts.Focus.CoverageEvidenceBindings = []CoverageEvidenceBinding{binding}
	currentBinding := RuntimeCWIBindingRefForCoverageBinding(binding, "manifest-1")
	facts.CWILedgerSchemaVersion = RuntimeCWILedgerSchemaCurrent
	facts.Coverage = []RuntimeCoverageItemFact{{
		ID:                  common.CoverageWorkItemID("cwi-1"),
		State:               RuntimeCoverageItemStateOpen,
		CWIConvergenceState: RuntimeCWIConvergenceStateBound,
		CurrentBindingRef:   &currentBinding,
		BindingLineage:      []RuntimeCWIBindingRef{currentBinding},
		FocusID:             facts.Focus.ID,
	}}
	facts.DefinitionSeed = RuntimeDefinitionSeed{
		SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: facts.DefinitionRevision, PolicyHash: "policy-1",
		CoverageItems: []RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "deliver"}},
		Slices:        []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}},
	}
	facts.DefinitionSeed.Hash = StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	facts.DefinitionFocuses = []CoverageFocus{{
		ID: facts.Focus.ID, SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{EvidenceKindStateChange},
	}}
	reconciliation, err := CompileRuntimeCWIReconciliationContract(facts.DefinitionRevision, []CoverageFocus{facts.Focus})
	if err != nil {
		t.Fatal(err)
	}
	facts.ReconciliationContract = reconciliation
	decision := validDecisionForState(ConvergenceStateFailed)
	transaction := decision.AuthorityTransaction()
	semanticPlanning, err := NewRuntimeSemanticPlanningLedger(facts.DefinitionSeed)
	if err != nil {
		t.Fatal(err)
	}
	facts = facts.Normalized()
	executionBudget, err := NewRuntimeExecutionBudgetLedger(facts.ExecutionBudgetPolicy)
	if err != nil {
		t.Fatal(err)
	}
	record := RuntimeStateRecord{
		StateRevision: 1,
		Definition: RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
			Revision: facts.DefinitionRevision, GoalID: "goal-1", GoalDefinition: "goal",
			DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/write"},
		},
		Facts: facts, SemanticPlanning: semanticPlanning, ExecutionBudget: executionBudget, Decision: decision,
		CandidateAttempts: []RuntimeCandidateAttemptRecord{{
			AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction),
			Transaction:         transaction,
			Attempt:             0,
			StablePrefixHash:    "stable-prefix",
			ManifestHash:        "manifest-1",
			LogicalRequestID:    "request-0",
			State:               RuntimeCandidateAttemptStatePending,
		}},
	}
	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "terminal runtime state cannot retain a pending candidate attempt") {
		t.Fatalf("terminal runtime state with pending candidate attempt err=%v", err)
	}
}
