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
	record := RuntimeStateRecord{
		SchemaVersion: RuntimeStateSchemaVersionCurrent,
		StateRevision: 1,
		Definition: RuntimeDefinitionContext{
			Revision: facts.DefinitionRevision, GoalID: "goal-1", GoalDefinition: "goal",
			DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/write"},
		},
		Facts: facts, SemanticPlanning: semanticPlanning, Decision: decision,
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
