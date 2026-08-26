package events

import (
	"slices"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestContractsDeclareShellAsExecutionReceiver(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 7 {
		t.Fatalf("contracts = %+v", contracts)
	}
	for _, contract := range contracts {
		if contract.Owner != "shell" || !slices.Equal(contract.ReceiverOwners, []string{"shell"}) {
			t.Fatalf("contract = %+v", contract)
		}
	}
	if len(MessageContracts()) != 7 {
		t.Fatalf("message contract count = %d, want 7", len(MessageContracts()))
	}
}

func TestManagedValidationSessionOperationData(t *testing.T) {
	recipeID := "go-test"
	data, err := OperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "validate-1", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.ValidationRecipeID != recipeID || data.Command != "" || len(data.Args) != 0 || data.TimeoutMillis != 0 {
		t.Fatalf("managed session shell data = %+v", data)
	}
	if !data.ManagedValidation.RuntimeManaged || data.ManagedValidation.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession || !data.ManagedValidation.AutoCleanup {
		t.Fatalf("managed validation metadata = %+v", data.ManagedValidation)
	}
}

// SM-3 consumes one owner-specific payload. These detached values all affect
// process execution or failure interpretation, so none may remain mutable
// beside the sealed RuntimeOperation.
func TestShellSealCoversOperationRecipeScopeRoleAndEvidencePolicy(t *testing.T) {
	operation, catalog, observation, input, executionContext := shellSealFixture(t)
	if _, err := SealRuntimeOperation(operation, runtimecontract.RuntimeOperationAdmissibility{}, input); err == nil {
		t.Fatal("Shell seal accepted a missing owner catalog")
	}
	missingObservation := input
	missingObservation.CatalogObservation = runtimecontract.RuntimePrerequisiteObservation{}
	if _, err := SealRuntimeOperation(operation, catalog, missingObservation); err == nil {
		t.Fatal("Shell seal accepted a missing owner catalog observation")
	}
	wrongCatalog := catalog
	wrongCatalog.OwnerModuleID = runtimecontract.RuntimeModuleWeb
	wrongCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(wrongCatalog)
	if _, err := SealRuntimeOperation(operation, wrongCatalog, input); err == nil {
		t.Fatal("Shell seal accepted another owner's catalog")
	}

	for name, mutate := range map[string]func(*SealedRuntimeOperation){
		"runtime_operation": func(value *SealedRuntimeOperation) {
			value.Operation.Parameters[0].String = shellString("other-recipe")
		},
		"owner_recipe": func(value *SealedRuntimeOperation) {
			value.Authorization.ValidationRecipes[0].Args = []string{"-c", "touch unexpected"}
		},
		"validation_scope": func(value *SealedRuntimeOperation) {
			value.Authorization.ValidationScopeTargetRefs = []string{"other.go"}
		},
		"target_role": func(value *SealedRuntimeOperation) {
			value.Authorization.TargetRole = "other-role"
		},
		"evidence_policy": func(value *SealedRuntimeOperation) {
			value.Authorization.EvidencePolicies[0].Description = "tampered"
		},
	} {
		t.Run(name, func(t *testing.T) {
			sealed, err := SealRuntimeOperation(operation, catalog, input)
			if err != nil {
				t.Fatal(err)
			}
			mutate(&sealed)
			if _, _, err := sealed.ValidateForExecution(executionContext); err == nil {
				t.Fatalf("tampered %s was accepted", name)
			}
		})
	}

	sealed, err := SealRuntimeOperation(operation, catalog, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, authorization, err := sealed.ValidateForExecution(executionContext); err != nil ||
		len(authorization.ValidationRecipes) != 1 || authorization.ValidationRecipes[0].Command != "sh" {
		t.Fatalf("valid Shell owner payload = %+v err=%v", authorization, err)
	}
	idempotencyDrift := executionContext
	idempotencyDrift.Execution.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencySafeToReplay
	if _, _, err := sealed.ValidateForExecution(idempotencyDrift); err == nil || !strings.Contains(err.Error(), "idempotency class mismatch") {
		t.Fatalf("sealed Shell operation accepted idempotency drift: %v", err)
	}
	authorityDrift := executionContext
	authorityDrift.Execution.AuthorityHash = "forged"
	if _, _, err := sealed.ValidateForExecution(authorityDrift); err == nil || !strings.Contains(err.Error(), "authority hash mismatch") {
		t.Fatalf("sealed Shell operation accepted authority hash drift: %v", err)
	}
	_ = observation
}

func shellSealFixture(t *testing.T) (runtimecontract.RuntimeOperation, runtimecontract.RuntimeOperationAdmissibility, runtimecontract.RuntimePrerequisiteObservation, RuntimeExecutionSealInput, runtimecontract.RuntimeOperationExecutionContext) {
	t.Helper()
	recipeID := "managed-check"
	operation := runtimecontract.RuntimeOperation{
		ID: "validate-1", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, TargetRefs: []string{"entry.go"},
		Parameters: []runtimecontract.OperationParameter{{Name: "recipeId", String: &recipeID}},
	}
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility(
		"shell-admissibility", runtimecontract.RuntimeModuleShell, []string{operation.Kind}, operation.TargetRefs, nil,
		[]string{"owner-execution-policy"}, "shell-policy-v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog.ValidationRecipes = []runtimecontract.RuntimeValidationRecipe{{ID: recipeID, Command: "sh", Args: []string{"-c", "exit 0"}}}
	catalog.ValidationRecipeRefs = []string{recipeID}
	catalog.OperationPolicies = []runtimecontract.RuntimeOperationPolicyFact{{
		ID: "shell-managed-policy", OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: operation.Kind,
		SubjectRef: "managed-validation", EffectScope: runtimecontract.RuntimeEffectEphemeralSession,
		IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell,
			SubjectRef: "shell_runtime_policy", Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	input := RuntimeExecutionSealInput{
		CatalogObservation: observation, TargetRole: "implementation", ValidationScopeTargetRefs: []string{"entry.go"},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{ID: "managed-evidence", AppliesTo: runtimecontract.StepKindShellCommand, Description: "sealed"}},
	}
	executionContext := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch",
		Execution: testExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws/run/dispatch/validate-1", Fence: 1,
			ExpectedEffectScope:     runtimecontract.RuntimeEffectEphemeralSession,
			AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
		}, catalog.OperationPolicies),
		OperationID: operation.ID, TargetRefs: operation.TargetRefs,
	}
	return operation, catalog, observation, input, executionContext
}

func shellString(value string) *string {
	value = strings.TrimSpace(value)
	return &value
}
