package workorchruntimecontract

import (
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestEvaluateRuntimeCompleteAdmissionMatrix(t *testing.T) {
	base := runtimeCompleteAdmissionFixture(t)
	tests := []struct {
		name        string
		mutate      func(*RuntimeCompleteAdmissionInput)
		disposition RuntimeCompleteAdmissionDisposition
		code        string
	}{
		{name: "admitted", disposition: RuntimeCompleteAdmissionAdmitted, code: "admitted"},
		{name: "snapshot drift", mutate: func(input *RuntimeCompleteAdmissionInput) {
			input.Facts.EvidenceRevision = "evidence-drift"
		}, disposition: RuntimeCompleteAdmissionStaleConflict, code: "snapshot_identity_mismatch"},
		{name: "blocked prerequisite", mutate: func(input *RuntimeCompleteAdmissionInput) {
			settled := input.Facts.Coverage[0]
			settled.ID = "cwi-prerequisite"
			settled.FocusID = "focus-prerequisite"
			settled.State = RuntimeCoverageItemStateOpen
			settled.CWIConvergenceState = RuntimeCWIConvergenceStateUnbound
			settled.CurrentBindingRef = nil
			settled.BindingLineage = nil
			input.Facts.Coverage = append(input.Facts.Coverage, settled)
			input.Facts.RemainingFocuses = append(input.Facts.RemainingFocuses, CoverageFocus{
				ID: settled.FocusID, SliceID: "slice-prerequisite", CoverageWorkItemIDs: []common.CoverageWorkItemID{settled.ID},
			})
			input.Facts.DefinitionSeed.CoverageItems = append(input.Facts.DefinitionSeed.CoverageItems, RuntimeCoverageItem{
				ID: settled.ID, SliceID: "slice-prerequisite", SourceScope: "dodItem", SourceID: "dod-prerequisite", TargetText: "prepare", Order: 2,
			})
			input.Facts.DefinitionSeed.Slices = append(input.Facts.DefinitionSeed.Slices, RuntimeAcceptanceSlice{ID: "slice-prerequisite", CoverageWorkItemIDs: []common.CoverageWorkItemID{settled.ID}})
			input.Facts.DefinitionSeed.Hash = StableRuntimeDefinitionSeedHash(input.Facts.DefinitionSeed)
			input.Facts.Coverage[0].PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{settled.ID}
		}, disposition: RuntimeCompleteAdmissionPrerequisiteBlocked, code: "cwi_prerequisite_blocked"},
		{name: "owner feasibility missing", mutate: func(input *RuntimeCompleteAdmissionInput) {
			input.Attempt.Feasibilities = nil
		}, disposition: RuntimeCompleteAdmissionOwnerInfeasible, code: "owner_feasibility_incomplete"},
		{name: "proposal missing", mutate: func(input *RuntimeCompleteAdmissionInput) {
			input.Attempt.Proposal = nil
		}, disposition: RuntimeCompleteAdmissionStaleConflict, code: "candidate_proposal_mismatch"},
		{name: "run-086 prospective binding target cannot dispatch to main go", mutate: func(input *RuntimeCompleteAdmissionInput) {
			input.Candidate.StateChange.Operations[0].Operation.TargetRefs[0] = "cmd/lsp-bridge/main.go"
			input.Attempt.Proposal.StateChange.Operations[0].Operation.TargetRefs[0] = "cmd/lsp-bridge/main.go"
		}, disposition: RuntimeCompleteAdmissionCorrectableOnce, code: "candidate_manifest_violation"},
		{name: "attempt already closed", mutate: func(input *RuntimeCompleteAdmissionInput) {
			input.Attempt.State = RuntimeCandidateAttemptStateAccepted
		}, disposition: RuntimeCompleteAdmissionDuplicateNoop, code: "candidate_attempt_not_pending"},
		{name: "duplicate dispatch", mutate: func(input *RuntimeCompleteAdmissionInput) {
			input.ActiveDispatch = runtimeCompleteAdmissionDispatch(input)
		}, disposition: RuntimeCompleteAdmissionDuplicateNoop, code: "dispatch_already_sealed"},
		{name: "conflicting dispatch", mutate: func(input *RuntimeCompleteAdmissionInput) {
			input.ActiveDispatch = runtimeCompleteAdmissionDispatch(input)
			input.ActiveDispatch.CandidateHash = "candidate-other"
		}, disposition: RuntimeCompleteAdmissionStaleConflict, code: "active_dispatch_conflict"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base.Clone()
			if test.mutate != nil {
				test.mutate(&input)
			}
			decision := EvaluateRuntimeCompleteAdmission(input)
			if decision.Disposition != test.disposition || decision.Code != test.code {
				t.Fatalf("decision = %+v, want disposition=%q code=%q", decision, test.disposition, test.code)
			}
		})
	}
}

func TestEvaluateRuntimeCompleteAdmissionDetachesAndDoesNotMutateInput(t *testing.T) {
	input := runtimeCompleteAdmissionFixture(t).Clone()
	original := input.Clone()
	cloned := original.Clone()
	cloned.Facts.Focus.TargetRefs[0] = "mutated.go"
	cloned.Decision.RequiredOutput.TargetRefs[0] = "mutated.go"
	cloned.SemanticPlanning.Manifest.OwnerCatalogs[0].TargetRefs[0] = "mutated.go"
	cloned.Reconciliation.History = append(cloned.Reconciliation.History, RuntimeCWIReconciliationGeneration{})
	cloned.Manifest.Bindings[0].TargetRefs[0] = "mutated.go"
	cloned.Candidate.StateChange.Operations[0].Operation.TargetRefs[0] = "mutated.go"
	cloned.Attempt.Proposal.StateChange.Operations[0].Operation.TargetRefs[0] = "mutated.go"
	cloned.Attempt.Feasibilities[0].Observation.Outcome = RuntimeOwnerFeasibilityInfeasible
	if !reflect.DeepEqual(input, original) {
		t.Fatal("RuntimeCompleteAdmissionInput.Clone aliases owner state")
	}
	before := input.Clone()
	if decision := EvaluateRuntimeCompleteAdmission(input); !decision.Admitted() {
		t.Fatalf("valid input rejected: %+v", decision)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("EvaluateRuntimeCompleteAdmission mutated its input")
	}
}

func TestEvaluateRuntimeCompleteAdmissionRejectsRetirementAfterAcceptedEffect(t *testing.T) {
	input := runtimeCompleteAdmissionFixture(t)
	current := input.Facts.Coverage[0].CurrentBindingRef.Clone()
	ref := current.Clone()
	ref.BindingID = "binding-retired"
	ref.BindingHash = "binding-retired-hash"
	ref.FocusID = "focus-retired"
	input.Facts.Coverage[0].BindingRetirementLineage = []RuntimeCWIBindingRetirementLineage{{
		RetiredBindingRef: ref, CausationRef: "catalog/retired", Reason: "catalog changed before execution",
	}}
	input.Facts.Coverage[0].BindingLineage = []RuntimeCWIBindingRef{ref, current}
	dispatch := runtimeCompleteAdmissionDispatch(&input)
	dispatch.Transaction.FocusID = ref.FocusID
	dispatch.Candidate.StateChange.Operations[0].Operation.BindingRefs = []string{ref.BindingID}
	dispatch.OperationExecutions[0].State = OperationExecutionOwnerCommitted
	dispatch.OperationExecutions[0].Fence = 1
	dispatch.OperationExecutions[0].OwnerReceiptRef = "receipt-1"
	dispatch.OperationExecutions[0].OwnerReceiptHash = "receipt-hash-1"
	input.CompletedDispatches = []RuntimeDispatchRecord{*dispatch}
	input.OperationReceipts = []RuntimeOperationReceipt{{
		ExecutionKey: dispatch.OperationExecutions[0].ExecutionKey, Fence: 1, OperationID: "write-main",
		Outcome: RuntimeOperationOutcomeAccepted, ObservedEffectScope: RuntimeEffectWorkspaceDurable, Changed: true, TargetRefs: []string{"main.go"},
	}}
	decision := EvaluateRuntimeCompleteAdmission(input)
	if decision.Disposition != RuntimeCompleteAdmissionTerminalContractFailure || !strings.Contains(decision.Message, "retired after accepted durable execution") {
		t.Fatalf("retirement/effect decision = %+v", decision)
	}
}

func TestEvaluateRuntimeCompleteAdmissionOwnerOutcomeMatrix(t *testing.T) {
	tests := []struct {
		name        string
		observation func(RuntimeCompleteAdmissionInput) RuntimeOwnerFeasibilityObservation
		disposition RuntimeCompleteAdmissionDisposition
		code        string
	}{
		{
			name: "stale", observation: func(input RuntimeCompleteAdmissionInput) RuntimeOwnerFeasibilityObservation {
				inventory := input.Facts.Resources.OperationAdmissibilities[0]
				return RuntimeOwnerFeasibilityObservation{Outcome: RuntimeOwnerFeasibilityStale, Stale: &RuntimePrerequisiteStale{
					PrerequisiteID: inventory.PrerequisiteID, ObservationID: "observation-stale", Domain: RuntimeResourceWorkspaceTree,
					OwnerModuleID: inventory.OwnerModuleID, SubjectRef: "workspace_deliverable", ExpectedRevision: inventory.ObservedRevision,
					ObservedRevision: inventory.ObservedRevision + "-new", Reason: "owner catalog changed during feasibility",
				}}
			}, disposition: RuntimeCompleteAdmissionOwnerStale, code: "owner_stale",
		},
		{
			name: "prerequisite required", observation: func(input RuntimeCompleteAdmissionInput) RuntimeOwnerFeasibilityObservation {
				operation := input.Candidate.StateChange.Operations[0].Operation
				feedback := runtimeCompleteAdmissionOwnerFeedback(t, input, RuntimeOwnerFeasibilityCategoryResourceAbsent, "target_snapshot_required")
				return RuntimeOwnerFeasibilityObservation{Outcome: RuntimeOwnerFeasibilityPrerequisiteRequired, Feedback: &feedback, PrerequisiteRequest: &RuntimeOperationPrerequisiteRequest{
					OperationID: operation.ID, Kind: PrerequisiteKindTargetSnapshot, OwnerModuleID: RuntimeModuleFile,
					SubjectRef: operation.TargetRefs[0], Reason: "owner requires a current target snapshot",
				}}
			}, disposition: RuntimeCompleteAdmissionOwnerPrerequisite, code: "owner_prerequisite_required",
		},
		{
			name: "infeasible", observation: func(input RuntimeCompleteAdmissionInput) RuntimeOwnerFeasibilityObservation {
				feedback := runtimeCompleteAdmissionOwnerFeedback(t, input, RuntimeOwnerFeasibilityCategoryPolicyDisallowed, "operation_policy_disallowed")
				return RuntimeOwnerFeasibilityObservation{Outcome: RuntimeOwnerFeasibilityInfeasible, Feedback: &feedback}
			}, disposition: RuntimeCompleteAdmissionOwnerInfeasible, code: "owner_infeasible",
		},
		{
			name: "contract defect", observation: func(input RuntimeCompleteAdmissionInput) RuntimeOwnerFeasibilityObservation {
				feedback := runtimeCompleteAdmissionOwnerFeedback(t, input, RuntimeOwnerFeasibilityCategoryOwnerContractDefect, "owner_unregistered_code")
				return RuntimeOwnerFeasibilityObservation{Outcome: RuntimeOwnerFeasibilityContractDefect, Feedback: &feedback}
			}, disposition: RuntimeCompleteAdmissionTerminalContractFailure, code: "owner_contract_defect",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := runtimeCompleteAdmissionFixture(t)
			observation := test.observation(input)
			input.Attempt.Feasibilities[0].Observation = &observation
			decision := EvaluateRuntimeCompleteAdmission(input)
			if decision.Disposition != test.disposition || decision.Code != test.code {
				t.Fatalf("decision = %+v, want disposition=%q code=%q", decision, test.disposition, test.code)
			}
		})
	}
}

func TestEvaluateRuntimeCompleteAdmissionOwnerIdentityMatrix(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*RuntimeCompleteAdmissionInput)
		disposition RuntimeCompleteAdmissionDisposition
		code        string
	}{
		{name: "manifest drift", mutate: func(input *RuntimeCompleteAdmissionInput) {
			input.Attempt.Feasibilities[0].CandidateManifestHash = "manifest-stale"
		}, disposition: RuntimeCompleteAdmissionOwnerStale, code: "owner_feasibility_manifest_stale"},
		{name: "catalog drift", mutate: func(input *RuntimeCompleteAdmissionInput) {
			input.Attempt.Feasibilities[0].AcquisitionBinding = "catalog-binding-stale"
		}, disposition: RuntimeCompleteAdmissionOwnerStale, code: "owner_feasibility_catalog_stale"},
		{name: "replay proof owner mismatch", mutate: func(input *RuntimeCompleteAdmissionInput) {
			inventory := input.Facts.Resources.OperationAdmissibilities[0]
			input.Attempt.Feasibilities[0].ReplayProof = &RuntimeOperationReplayProof{
				OwnerModuleID: RuntimeModuleShell, OperationID: "write-main", TargetRefs: []string{"main.go"}, BeforeHash: "before", AfterHash: "after",
				AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
			}
		}, disposition: RuntimeCompleteAdmissionTerminalContractFailure, code: "owner_replay_proof_mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := runtimeCompleteAdmissionFixture(t)
			test.mutate(&input)
			decision := EvaluateRuntimeCompleteAdmission(input)
			if decision.Disposition != test.disposition || decision.Code != test.code {
				t.Fatalf("decision = %+v, want disposition=%q code=%q", decision, test.disposition, test.code)
			}
		})
	}
}

func TestEvaluateRuntimeCompleteAdmissionPermutationAndHashStability(t *testing.T) {
	input := runtimeCompleteAdmissionFixture(t)
	baselineCandidateHash := StableRuntimeCandidateHash(input.Candidate)
	baselineManifestHash := StableRuntimeCandidateContractManifestHash(input.Manifest)
	permuted := input.Clone()
	permuted.Candidate.StateChange.Operations[0].Operation.TargetRefs = []string{"main.go", "main.go"}
	permuted.Attempt.Proposal.StateChange.Operations[0].Operation.TargetRefs = []string{"main.go", "main.go"}
	permuted.Manifest.TargetRefs = append([]string{"main.go"}, permuted.Manifest.TargetRefs...)
	permuted.Manifest.Hash = StableRuntimeCandidateContractManifestHash(permuted.Manifest)
	permuted.Attempt.ManifestHash = permuted.Manifest.Hash
	permuted.Attempt.Feasibilities[0].CandidateManifestHash = permuted.Manifest.Hash
	if StableRuntimeCandidateHash(permuted.Candidate) != baselineCandidateHash {
		t.Fatal("set-like target permutation changed candidate identity")
	}
	if StableRuntimeCandidateContractManifestHash(permuted.Manifest) != baselineManifestHash {
		t.Fatal("set-like target permutation changed manifest identity")
	}
	if decision := EvaluateRuntimeCompleteAdmission(permuted); !decision.Admitted() {
		t.Fatalf("normalized permutation rejected: %+v", decision)
	}
}

func runtimeCompleteAdmissionOwnerFeedback(t *testing.T, input RuntimeCompleteAdmissionInput, category RuntimeOwnerFeasibilityCategory, code string) RuntimeFeedback {
	t.Helper()
	operation := input.Candidate.StateChange.Operations[0].Operation
	failure := RuntimeOperationInfeasibleFailure(operation, code, "owner feasibility rejected the operation")
	feedback, err := RuntimeFeedbackFromFailureFact(RuntimeFeedbackScope{
		WorkspaceID: input.Facts.WorkspaceID, RunID: input.Facts.RunID, AuthoritySnapshotID: input.Decision.AuthoritySnapshot.ID,
	}, RuntimeFeedbackDomainFeasibility, RuntimeFeedbackCategory(category), RuntimeModuleFile, *failure)
	if err != nil {
		t.Fatal(err)
	}
	feedback.OriginalDiagnostic.OperationID = operation.ID
	return feedback
}

func runtimeCompleteAdmissionFixture(t *testing.T) RuntimeCompleteAdmissionInput {
	t.Helper()
	semantic := semanticSealedLedgerFixture(t)
	focuses, reconciliation, err := CompileRuntimeSemanticBindings(*semantic.Manifest, semantic.LogicalRequestID, *semantic.Document, semantic.Feasibilities)
	if err != nil || len(focuses) != 1 {
		t.Fatalf("compile semantic fixture: focuses=%d err=%v", len(focuses), err)
	}
	semantic.SealedBindingSetHash = reconciliation.BindingSetHash
	semantic.SealedLineageRefs[len(semantic.SealedLineageRefs)-1] = reconciliation.BindingSetHash
	if err := semantic.Validate(); err != nil {
		t.Fatal(err)
	}
	focus := focuses[0]
	binding := focus.CoverageEvidenceBindings[0]
	ref := RuntimeCWIBindingRefForCoverageBinding(binding, semantic.Manifest.Hash)
	facts := RuntimeFacts{
		WorkspaceID: "ws-history", RunID: "run-history", DefinitionRevision: semantic.DefinitionSeed.DefinitionRevision,
		CWILedgerSchemaVersion: RuntimeCWILedgerSchemaCurrent, DefinitionSeed: semantic.DefinitionSeed.Clone(), EvidenceRevision: "evidence-1",
		Focus: focus, ReconciliationContract: reconciliation,
		Coverage: []RuntimeCoverageItemFact{{
			ID: binding.CoverageWorkItemID, SliceID: focus.SliceID, SourceScope: focus.AssessmentScope, SourceID: focus.AssessmentID,
			TargetText: focus.AssessmentText, State: RuntimeCoverageItemStateOpen, CWIConvergenceState: RuntimeCWIConvergenceStateBound,
			CurrentBindingRef: &ref, BindingLineage: []RuntimeCWIBindingRef{ref}, FocusID: focus.ID,
		}},
		Slices: semantic.DefinitionSeed.Slices,
		Activation: RuntimeActivationContext{
			ProviderSelection: ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-test"}},
			ExecutionContract: RuntimeExecutionContract{OperationContractRevision: RuntimeOperationContractRevision()},
		},
		Capabilities: RuntimeCapabilities{StateChangeKinds: []string{StepKindFileWrite}},
	}
	inventory := semantic.Manifest.OwnerCatalogs[0].Normalized()
	prerequisite := RuntimePrerequisite{
		ID: inventory.PrerequisiteID, Kind: PrerequisiteKindOperationAdmissibility, Scope: RuntimePrerequisiteScopeGlobal,
		OwnerModuleID: inventory.OwnerModuleID, RequiredCapabilities: []string{StepKindFileWrite}, Reason: "owner catalog",
	}
	facts.Prerequisites.Satisfied = []RuntimePrerequisite{prerequisite}
	facts.Resources.OperationAdmissibilities = []RuntimeOperationAdmissibility{inventory}
	decision, err := SealRuntimeDecision(facts, RuntimeDecision{
		State: ConvergenceStateNeedStateChange, Focus: focus,
		RequiredOutput: RequiredOutputContract{
			AcceptedKinds: []string{StepKindFileWrite}, AcceptedEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, RequiredEvidenceKinds: []string{EvidenceKindStateChange},
			CoverageWorkItemIDs: focus.CoverageWorkItemIDs, TargetRefs: []string{"main.go"},
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := BuildRuntimeCandidateContractManifest(decision, RuntimeCandidateManifestOwnerFactsFromContext(RuntimeCandidateContextForDecision(facts, decision, nil)))
	if err != nil {
		t.Fatal(err)
	}
	content := "package main"
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID: "write-main", Kind: StepKindFileWrite, BindingRefs: []string{binding.ID}, TargetRefs: []string{"main.go"},
		Parameters: []OperationParameter{{Name: "content", String: &content}},
	}}}}}
	if err := ValidateRuntimeCandidateAgainstManifest(manifest, candidate); err != nil {
		t.Fatal(err)
	}
	transaction := decision.AuthorityTransaction()
	proposal := cloneRuntimeCandidate(candidate)
	attempt := RuntimeCandidateAttemptRecord{
		AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction), Transaction: transaction,
		Attempt: 0, StablePrefixHash: "prefix-1", ManifestHash: manifest.Hash, LogicalRequestID: "request-1",
		State: RuntimeCandidateAttemptStatePending, Proposal: &proposal,
		Feasibilities: []RuntimeCandidateFeasibilityRecord{{
			OperationID: "write-main", AdmissibilitySnapshotID: inventory.SnapshotID, AcquisitionBinding: inventory.AcquisitionBinding,
			CandidateManifestHash: manifest.Hash,
			Observation:           &RuntimeOwnerFeasibilityObservation{Outcome: RuntimeOwnerFeasibilityFeasible},
		}},
	}
	return RuntimeCompleteAdmissionInput{
		Facts: facts, Decision: decision, SemanticPlanning: semantic, Manifest: manifest, Candidate: candidate, Attempt: attempt,
	}
}

func runtimeCompleteAdmissionDispatch(input *RuntimeCompleteAdmissionInput) *RuntimeDispatchRecord {
	transaction := input.Decision.AuthorityTransaction()
	return &RuntimeDispatchRecord{
		ID: "dispatch-1", WorkspaceID: input.Facts.WorkspaceID, RunID: input.Facts.RunID, Transaction: transaction,
		CandidateHash: StableRuntimeCandidateHash(input.Candidate), Candidate: cloneRuntimeCandidate(input.Candidate), State: RuntimeDispatchStateSealed,
		OperationExecutions: []RuntimeOperationExecutionRecord{{
			OperationID: "write-main", ExecutionKey: StableRuntimeOperationExecutionKey(input.Facts.WorkspaceID, input.Facts.RunID, "dispatch-1", "write-main"),
			ExpectedEffectScope: RuntimeEffectWorkspaceDurable, State: OperationExecutionSealed,
		}},
	}
}
