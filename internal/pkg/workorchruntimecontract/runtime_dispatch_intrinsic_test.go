package workorchruntimecontract

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeDispatchRecordValidateIntrinsicEnforcesOperationExecutionBijectionAndHashes(t *testing.T) {
	baseline := runtimeDispatchIntrinsicFixture(t, 0)

	tests := []struct {
		name    string
		mutate  func(*RuntimeDispatchRecord)
		wantErr string
	}{
		{
			name: "missing operation execution",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationExecutions = nil
				dispatch.OperationAuthorityHash = ""
			},
			wantErr: "exactly one operation execution",
		},
		{
			name: "execution belongs to unknown operation",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationExecutions[0].OperationID = "op-other"
				dispatch.OperationExecutions[0].AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(dispatch.OperationExecutions[0])
				dispatch.OperationAuthorityHash = StableRuntimeOperationExecutionAuthoritiesHash(dispatch.OperationExecutions)
			},
			wantErr: "not present in candidate",
		},
		{
			name: "execution authority hash drift",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationExecutions[0].AuthorityHash = "drifted"
			},
			wantErr: "authority hash mismatch",
		},
		{
			name: "dispatch operation authority hash drift",
			mutate: func(dispatch *RuntimeDispatchRecord) {
				dispatch.OperationAuthorityHash = "drifted"
			},
			wantErr: "operation authority hash mismatch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dispatch := baseline.Clone()
			test.mutate(&dispatch)
			err := dispatch.ValidateIntrinsic()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateIntrinsic() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestRuntimeDispatchRecordValidateIntrinsicRejectsDuplicateCompletedOperationAndRecipe(t *testing.T) {
	t.Run("completed operation id", func(t *testing.T) {
		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		dispatch.CompletedOperationIDs = []string{"op-1", "op-1"}
		if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "completed operation \"op-1\" is duplicated") {
			t.Fatalf("duplicate completed operation error = %v", err)
		}
	})

	t.Run("non-canonical completed operation id", func(t *testing.T) {
		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		dispatch.CompletedOperationIDs = []string{" op-1 "}
		if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "must be canonical") {
			t.Fatalf("non-canonical completed operation error = %v", err)
		}
	})

	t.Run("completed marker without evidence commit", func(t *testing.T) {
		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		dispatch.CompletedOperationIDs = []string{"op-1"}
		if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "completed marker/evidence state mismatch") {
			t.Fatalf("premature completed operation error = %v", err)
		}
	})

	t.Run("evidence commit without completed marker", func(t *testing.T) {
		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		dispatch.OperationExecutions[0].State = OperationExecutionEvidenceCommitted
		dispatch.OperationExecutions[0].Fence = 1
		dispatch.OperationExecutions[0].ClaimedAt = RuntimeOperationClaimMarkerInitial
		dispatch.OperationExecutions[0].OwnerReceiptRef = "receipt://op-1"
		dispatch.OperationExecutions[0].OwnerReceiptHash = "receipt-hash-1"
		dispatch.OperationExecutions[0].EvidenceFactID = "evidence-op-1"
		dispatch.OperationExecutions[0].EvidenceFactHash = "evidence-hash-1"
		if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "completed marker/evidence state mismatch") {
			t.Fatalf("unmarked evidence commit error = %v", err)
		}
	})

	t.Run("validation recipe id", func(t *testing.T) {
		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		recipe := RuntimeValidationRecipe{ID: "recipe-1", Command: "true"}
		dispatch.ValidationRecipes = []RuntimeValidationRecipe{recipe, recipe}
		if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "duplicates recipe \"recipe-1\"") {
			t.Fatalf("duplicate validation recipe error = %v", err)
		}
	})
}

func TestRuntimeDispatchRecordValidateIntrinsicRejectsUnsealedOperationAuthority(t *testing.T) {
	t.Run("legacy coverage work item ids", func(t *testing.T) {
		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		dispatch.Candidate.StateChange.Operations[0].Operation.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-legacy"}
		if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "cannot carry coverage work item ids") {
			t.Fatalf("legacy operation coverage error = %v", err)
		}
	})

	t.Run("unknown operation kind", func(t *testing.T) {
		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		dispatch.Candidate.StateChange.Operations[0].Operation.Kind = "custom/unknown"
		if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "has unknown kind") {
			t.Fatalf("unknown operation kind error = %v", err)
		}
	})

	t.Run("operation kind from another convergence state", func(t *testing.T) {
		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		dispatch.Candidate.StateChange.Operations[0].Operation.Kind = StepKindFileRead
		if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "is not allowed in convergence state") {
			t.Fatalf("cross-state operation kind error = %v", err)
		}
	})
}

func TestRuntimeDispatchRecordValidateIntrinsicRejectsNonCanonicalRawCandidateShape(t *testing.T) {
	stringValue := "ignored"
	for name, mutate := range map[string]func(*RuntimeOperation){
		"empty parameter name": func(operation *RuntimeOperation) {
			operation.Parameters = append(operation.Parameters, OperationParameter{Name: "", String: &stringValue})
		},
		"blank dependency": func(operation *RuntimeOperation) {
			operation.DependsOn = append(operation.DependsOn, "")
		},
		"duplicate binding": func(operation *RuntimeOperation) {
			operation.BindingRefs = []string{"binding-1", "binding-1"}
		},
		"blank target": func(operation *RuntimeOperation) {
			operation.TargetRefs = append(operation.TargetRefs, " ")
		},
	} {
		t.Run(name, func(t *testing.T) {
			dispatch := runtimeDispatchIntrinsicFixture(t, 0)
			mutate(&dispatch.Candidate.StateChange.Operations[0].Operation)
			// These additions were historically erased by Normalized(), leaving
			// every hash unchanged. Intrinsic load must reject the raw DTO itself.
			if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "canonical raw shape") {
				t.Fatalf("non-canonical candidate error = %v", err)
			}
		})
	}
}

func TestRuntimeDispatchRecordValidateIntrinsicRequiresCurrentTransactionContract(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeDispatchRecord){
		"missing capability hash": func(dispatch *RuntimeDispatchRecord) { dispatch.Transaction.CapabilityStateHash = "" },
		"missing freshness hash":  func(dispatch *RuntimeDispatchRecord) { dispatch.Transaction.PrerequisiteFreshnessHash = "" },
		"missing operation revision": func(dispatch *RuntimeDispatchRecord) {
			dispatch.Transaction.OperationContractRevision = ""
		},
		"stale operation revision": func(dispatch *RuntimeDispatchRecord) {
			dispatch.Transaction.OperationContractRevision = "runtime-operation-contract/stale"
		},
	} {
		t.Run(name, func(t *testing.T) {
			dispatch := runtimeDispatchIntrinsicFixture(t, 0)
			mutate(&dispatch)
			dispatch.Transaction.AuthoritySnapshotID = StableAuthoritySnapshotIDForTransaction(dispatch.WorkspaceID, dispatch.RunID, dispatch.Transaction)
			dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(dispatch)
			if err := dispatch.ValidateIntrinsic(); err == nil {
				t.Fatalf("runtime dispatch accepted invalid transaction: %+v", dispatch.Transaction)
			}
		})
	}
}

func TestRuntimeDispatchRecordValidateIntrinsicRejectsCoordinatedAuthoritySubstitution(t *testing.T) {
	t.Run("candidate and operation hashes", func(t *testing.T) {
		dispatch := runtimeDispatchIntrinsicFixture(t, 0)
		content := "substituted content"
		dispatch.Candidate.StateChange.Operations[0].Operation.Parameters[0].String = &content
		dispatch.CandidateHash = StableRuntimeCandidateHash(dispatch.Candidate)
		dispatch.ID = StableRuntimeDispatchID(dispatch.Transaction, dispatch.CandidateHash)
		dispatch.OperationExecutions[0].OperationHash = StableRuntimeOperationHash(dispatch.Candidate.StateChange.Operations[0].Operation)
		dispatch.OperationExecutions[0].ExecutionKey = StableRuntimeOperationExecutionKey(dispatch.WorkspaceID, dispatch.RunID, dispatch.ID, "op-1")
		dispatch.OperationExecutions[0].AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(dispatch.OperationExecutions[0])
		dispatch.OperationAuthorityHash = StableRuntimeOperationExecutionAuthoritiesHash(dispatch.OperationExecutions)
		if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "target authorization hash mismatch") {
			t.Fatalf("coordinated candidate substitution error = %v", err)
		}
	})

	t.Run("selected recipe body", func(t *testing.T) {
		dispatch := runtimeManagedValidationDispatchIntrinsicFixture(t)
		dispatch.ValidationRecipes[0].Args = []string{"-c", "false"}
		if err := dispatch.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "dispatch authority hash mismatch") {
			t.Fatalf("recipe body substitution error = %v", err)
		}
	})
}

func TestRuntimeDispatchRecordSealsCanonicalFocusTargetRole(t *testing.T) {
	dispatch := runtimeDispatchIntrinsicFixture(t, 0)
	dispatch.FocusTargetRole = "implementation"
	dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(dispatch)
	if err := dispatch.ValidateIntrinsic(); err != nil {
		t.Fatalf("sealed focus target role: %v", err)
	}

	drifted := dispatch.Clone()
	drifted.FocusTargetRole = "prospective_implementation"
	if err := drifted.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "dispatch authority hash mismatch") {
		t.Fatalf("focus target role drift error = %v", err)
	}

	nonCanonical := dispatch.Clone()
	nonCanonical.FocusTargetRole = " implementation "
	nonCanonical.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(nonCanonical)
	if err := nonCanonical.ValidateIntrinsic(); err == nil || !strings.Contains(err.Error(), "focus target role must be canonical") {
		t.Fatalf("non-canonical focus target role error = %v", err)
	}
}

func TestValidateRuntimeDispatchHistoryShapeEnforcesActiveCompletedPartition(t *testing.T) {
	active := runtimeDispatchIntrinsicFixture(t, 0)
	completed := runtimeDispatchIntrinsicFixture(t, 1)
	runtimeDispatchMarkCommittedForTest(&completed)
	if err := ValidateRuntimeDispatchHistoryShape(active.WorkspaceID, active.RunID, &active, []RuntimeDispatchRecord{completed}); err != nil {
		t.Fatalf("valid active/completed dispatch history: %v", err)
	}

	t.Run("active state", func(t *testing.T) {
		invalid := active.Clone()
		invalid.State = RuntimeDispatchStateCommitted
		if err := ValidateRuntimeDispatchHistoryShape(active.WorkspaceID, active.RunID, &invalid, []RuntimeDispatchRecord{completed}); err == nil {
			t.Fatal("committed dispatch was accepted as active")
		}
	})

	t.Run("completed state", func(t *testing.T) {
		invalid := completed.Clone()
		invalid.State = RuntimeDispatchStateSealed
		if err := ValidateRuntimeDispatchHistoryShape(active.WorkspaceID, active.RunID, &active, []RuntimeDispatchRecord{invalid}); err == nil {
			t.Fatal("sealed dispatch was accepted in completed history")
		}
	})

	t.Run("active completed identity conflict", func(t *testing.T) {
		conflict := active.Clone()
		runtimeDispatchMarkCommittedForTest(&conflict)
		if err := ValidateRuntimeDispatchHistoryShape(active.WorkspaceID, active.RunID, &active, []RuntimeDispatchRecord{conflict}); err == nil {
			t.Fatal("one dispatch identity was accepted in both active and completed history")
		}
	})

	t.Run("duplicate completed identity", func(t *testing.T) {
		if err := ValidateRuntimeDispatchHistoryShape(active.WorkspaceID, active.RunID, &active, []RuntimeDispatchRecord{completed, completed.Clone()}); err == nil {
			t.Fatal("duplicate completed dispatch identity was accepted")
		}
	})
}

func runtimeDispatchIntrinsicFixture(t testing.TB, attempt int) RuntimeDispatchRecord {
	t.Helper()

	transaction := AuthorityTransaction{
		FocusID:                   "focus-1",
		State:                     ConvergenceStateNeedStateChange,
		RequiredOutputHash:        "required-output-1",
		DefinitionRevision:        "definition-1",
		EvidenceRevision:          "evidence-1",
		CapabilityStateHash:       "capability-1",
		PrerequisiteFreshnessHash: "freshness-1",
		OperationContractRevision: RuntimeOperationContractRevision(),
		AuthorityInputID:          "authority-input-1",
		Attempt:                   attempt,
	}
	workspaceID := common.WorkspaceID("ws-dispatch-intrinsic")
	runID := common.RunID("run-dispatch-intrinsic")
	transaction.AuthoritySnapshotID = StableAuthoritySnapshotIDForTransaction(workspaceID, runID, transaction)
	content := "sealed content"
	candidate := RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{{Operation: RuntimeOperation{
		ID:         "op-1",
		Kind:       StepKindFileWrite,
		TargetRefs: []string{"target.txt"},
		Parameters: []OperationParameter{{Name: "content", String: &content}},
	}}}}}
	operation := candidate.StateChange.Operations[0].Operation
	candidateHash := StableRuntimeCandidateHash(candidate)
	dispatchID := StableRuntimeDispatchID(transaction, candidateHash)
	execution := RuntimeOperationExecutionRecord{
		OperationID:         "op-1",
		OperationHash:       StableRuntimeOperationHash(candidate.StateChange.Operations[0].Operation),
		ExecutionKey:        StableRuntimeOperationExecutionKey(workspaceID, runID, dispatchID, "op-1"),
		ExpectedEffectScope: RuntimeEffectWorkspaceDurable,
		IdempotencyClass:    RuntimeOperationIdempotencyAtMostOnce,
		State:               OperationExecutionSealed,
	}
	execution.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(execution)
	dispatch := RuntimeDispatchRecord{
		ID:                    dispatchID,
		WorkspaceID:           workspaceID,
		RunID:                 runID,
		Transaction:           transaction,
		CandidateHash:         candidateHash,
		Candidate:             candidate,
		CandidateManifestHash: "manifest-dispatch-intrinsic",
		OperationTargetAuthorizations: []RuntimeDispatchOperationTargetAuthorization{{
			OperationID: operation.ID, OperationHash: StableRuntimeOperationHash(operation), RequireTargets: true,
			Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"}},
		}},
		State:                  RuntimeDispatchStateSealed,
		CoverageBindingHash:    StableRuntimeCoverageEvidenceBindingSnapshotHash(nil),
		OperationExecutions:    []RuntimeOperationExecutionRecord{execution},
		OperationAuthorityHash: StableRuntimeOperationExecutionAuthoritiesHash([]RuntimeOperationExecutionRecord{execution}),
	}
	dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(dispatch)
	if err := dispatch.ValidateIntrinsic(); err != nil {
		t.Fatalf("runtime dispatch fixture: %v", err)
	}
	return dispatch
}

func runtimeManagedValidationDispatchIntrinsicFixture(t testing.TB) RuntimeDispatchRecord {
	t.Helper()
	dispatch := runtimeDispatchIntrinsicFixture(t, 0)
	dispatch.Transaction.State = ConvergenceStateNeedValidation
	dispatch.Transaction.AuthoritySnapshotID = StableAuthoritySnapshotIDForTransaction(dispatch.WorkspaceID, dispatch.RunID, dispatch.Transaction)
	recipeID := "recipe-1"
	operation := RuntimeOperation{
		ID: "op-1", Kind: RuntimeBlockKindManagedValidationSession,
		BindingRefs: []string{"binding-1"}, TargetRefs: []string{"target.txt"},
		Parameters: []OperationParameter{{Name: "recipeId", String: &recipeID}},
	}
	dispatch.Candidate = RuntimeCandidate{Validation: &ValidationCandidate{Operations: []ValidationOperation{{Operation: operation}}}}
	binding := CoverageEvidenceBinding{
		ID: "binding-1", CoverageWorkItemID: "cwi-1", FocusID: dispatch.Transaction.FocusID,
		DefinitionRevision: dispatch.Transaction.DefinitionRevision,
		Proofs: []CoverageEvidenceProof{{
			EvidenceKind: EvidenceKindValidation, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectEphemeralSession},
			AllowedOperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: recipeID,
			ValidationProofRequirementHash: "runtime-validation-proof-requirement-dispatch-1", RequiredCheckRefs: []string{"workorch-cwi-check-dispatch-1"},
			ValidationCheckAttestations: runtimeValidationCheckAttestationsForTest([]string{"workorch-cwi-check-dispatch-1"}),
		}},
		TargetRefs: []string{"target.txt"}, DeclarationRef: "test/runtime-dispatch-intrinsic",
	}.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	dispatch.CoverageBindings = []CoverageEvidenceBinding{binding}
	dispatch.CoverageBindingHash = StableRuntimeCoverageEvidenceBindingSnapshotHash(dispatch.CoverageBindings)
	dispatch.OperationTargetAuthorizations = []RuntimeDispatchOperationTargetAuthorization{{
		OperationID: operation.ID, OperationHash: StableRuntimeOperationHash(operation), RequireTargets: true,
		Target: RuntimeOperationTargetAuthorization{Policy: RuntimeOperationTargetPolicyExactTarget, ExactTargetRefs: []string{"target.txt"}},
	}}
	dispatch.ValidationRecipes = []RuntimeValidationRecipe{{ID: recipeID, Command: "sh", Args: []string{"-c", "true"}, TimeoutMillis: 5000}}
	dispatch.CandidateHash = StableRuntimeCandidateHash(dispatch.Candidate)
	dispatch.ID = StableRuntimeDispatchID(dispatch.Transaction, dispatch.CandidateHash)
	dispatch.OperationExecutions[0] = RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: StableRuntimeOperationHash(operation),
		ExecutionKey:        StableRuntimeOperationExecutionKey(dispatch.WorkspaceID, dispatch.RunID, dispatch.ID, operation.ID),
		ExpectedEffectScope: ResolveExpectedEffectScope(operation, nil),
		IdempotencyClass:    ResolveOperationIdempotencyClass(operation, nil), State: OperationExecutionSealed,
	}
	dispatch.OperationExecutions[0].AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(dispatch.OperationExecutions[0])
	dispatch.OperationAuthorityHash = StableRuntimeOperationExecutionAuthoritiesHash(dispatch.OperationExecutions)
	dispatch.DispatchAuthorityHash = StableRuntimeDispatchAuthorityHash(dispatch)
	if err := dispatch.ValidateIntrinsic(); err != nil {
		t.Fatalf("managed validation dispatch fixture: %v", err)
	}
	return dispatch
}

func runtimeDispatchMarkCommittedForTest(dispatch *RuntimeDispatchRecord) {
	dispatch.State = RuntimeDispatchStateCommitted
	dispatch.CompletedOperationIDs = []string{"op-1"}
	dispatch.OperationExecutions[0].State = OperationExecutionEvidenceCommitted
	dispatch.OperationExecutions[0].Fence = 1
	dispatch.OperationExecutions[0].OwnerReceiptRef = "receipt://op-1"
	dispatch.OperationExecutions[0].OwnerReceiptHash = "receipt-hash-1"
	dispatch.OperationExecutions[0].EvidenceFactID = "evidence-op-1"
	dispatch.OperationExecutions[0].EvidenceFactHash = "evidence-hash-1"
	dispatch.OperationExecutions[0].ClaimedAt = RuntimeOperationClaimMarkerInitial
}
