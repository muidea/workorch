package events

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestOwnerContractAndOperationMapper(t *testing.T) {
	contracts := Contracts()
	if len(contracts) != 5 || len(MessageContracts()) != 5 {
		t.Fatalf("contracts=%+v", contracts)
	}
	for _, contract := range contracts {
		if contract.Owner != "ssh" || !slices.Equal(contract.ReceiverOwners, []string{"ssh"}) {
			t.Fatalf("contract=%+v", contract)
		}
	}
	sessionRef, credentialRef, command := "session-a", "credential-a", "uname -a"
	operation := runtimecontract.RuntimeOperation{ID: "ssh-1", Kind: runtimecontract.StepKindSSHCommand, TargetRefs: []string{"target-a"}, Parameters: []runtimecontract.OperationParameter{{Name: "sessionRef", String: &sessionRef}, {Name: "credentialRef", String: &credentialRef}, {Name: "command", String: &command}}}
	data, err := OperationDataFromRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	if data.TargetRef != "target-a" || data.SessionRef != sessionRef || data.CredentialRef != credentialRef || data.Command != command {
		t.Fatalf("data=%+v", data)
	}
	catalog := testSSHOwnerCatalog(t, operation)
	observation := testSSHCatalogObservation(catalog)
	if _, err := SealRuntimeOperation(operation, runtimecontract.RuntimeOperationAdmissibility{}, observation); err == nil {
		t.Fatal("SSH seal accepted missing owner catalog")
	}
	if _, err := SealRuntimeOperation(operation, catalog, runtimecontract.RuntimePrerequisiteObservation{}); err == nil {
		t.Fatal("SSH seal accepted missing catalog observation")
	}
	wrongCatalog := catalog
	wrongCatalog.OwnerModuleID = runtimecontract.RuntimeModuleWeb
	wrongCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(wrongCatalog)
	if _, err := SealRuntimeOperation(operation, wrongCatalog, observation); err == nil {
		t.Fatal("SSH seal accepted another owner's catalog")
	}
	sealed, err := SealRuntimeOperation(operation, catalog, observation)
	if err != nil {
		t.Fatal(err)
	}
	context := runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch",
		Execution: testExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: "exec/ws/run/dispatch/ssh-1", Fence: 1,
			ExpectedEffectScope:     runtimecontract.RuntimeEffectRemoteDurable,
			AdmissibilitySnapshotID: catalog.SnapshotID, AcquisitionBinding: catalog.AcquisitionBinding,
		}, []runtimecontract.RuntimeOperationPolicyFact{sealed.Authorization.Policy}),
		OperationID: operation.ID, TargetRefs: []string{"target-a"},
	}
	if _, _, err := sealed.ValidateForExecution(context); err != nil {
		t.Fatalf("validate sealed operation: %v", err)
	}
	idempotencyDrift := context
	idempotencyDrift.Execution.IdempotencyClass = runtimecontract.RuntimeOperationIdempotencySafeToReplay
	if _, _, err := sealed.ValidateForExecution(idempotencyDrift); err == nil || !strings.Contains(err.Error(), "idempotency class mismatch") {
		t.Fatalf("sealed SSH operation accepted idempotency drift: %v", err)
	}
	authorityDrift := context
	authorityDrift.Execution.AuthorityHash = "forged"
	if _, _, err := sealed.ValidateForExecution(authorityDrift); err == nil || !strings.Contains(err.Error(), "authority hash mismatch") {
		t.Fatalf("sealed SSH operation accepted authority hash drift: %v", err)
	}
	drifted := sealed
	drifted.Operation.Parameters[0].String = ptrString("session-b")
	if _, _, err := drifted.ValidateForExecution(context); err == nil || !strings.Contains(err.Error(), "operation hash mismatch") {
		t.Fatalf("drifted sealed operation err = %v, want operation authority conflict", err)
	}
	wire, err := json.Marshal(ExecuteOperationCommand{RuntimeOperationExecutionContext: context, SealedOperation: sealed})
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{"example.internal", "password", "privateKey", "username"} {
		if strings.Contains(string(wire), prohibited) {
			t.Fatalf("SSH EventHub wire contains prohibited endpoint/credential field %q: %s", prohibited, wire)
		}
	}
}

func testSSHOwnerCatalog(t *testing.T, operation runtimecontract.RuntimeOperation) runtimecontract.RuntimeOperationAdmissibility {
	t.Helper()
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility("ssh-admissibility", runtimecontract.RuntimeModuleSSH, []string{operation.Kind}, operation.TargetRefs, nil,
		[]string{"ssh-binding/target-a/session-a/credential-a", "owner-execution-policy"}, "ssh-policy-v1")
	if err != nil {
		t.Fatal(err)
	}
	catalog.OperationPolicies = []runtimecontract.RuntimeOperationPolicyFact{{
		ID: "ssh-policy", OwnerModuleID: runtimecontract.RuntimeModuleSSH, OperationKind: operation.Kind,
		SubjectRef: "configured-ssh-target", EffectScope: runtimecontract.RuntimeEffectRemoteDurable,
		IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func testSSHCatalogObservation(catalog runtimecontract.RuntimeOperationAdmissibility) runtimecontract.RuntimePrerequisiteObservation {
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleSSH, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleSSH, SubjectRef: "ssh_runtime_policy", Revision: catalog.ObservedRevision}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func ptrString(value string) *string { return &value }
