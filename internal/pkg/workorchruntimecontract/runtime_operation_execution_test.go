package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCloneRuntimeOperationExecutionContextDetachesNestedValues(t *testing.T) {
	proof := RuntimeOperationReplayProof{OwnerModuleID: RuntimeModuleFile, OperationID: "write-1", TargetRefs: []string{"a.go"}, BeforeHash: "before", AfterHash: "after", AdmissibilitySnapshotID: "admissibility-1"}
	envelope := runtimeOperationExecutionEnvelopeForTest(RuntimeOperation{ID: "write-1"}, "exec/ws/run/dispatch/write-1", 1, RuntimeEffectWorkspaceDurable, RuntimeOperationIdempotencyAtMostOnce, "admissibility-1", "", &proof)
	input := RuntimeOperationExecutionContext{
		Execution:   envelope,
		WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "write-1",
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"a.go"},
		EvidencePolicies: []RuntimeEvidencePolicy{{ID: "policy-1", AppliesTo: StepKindFileWrite, ArgsContain: []string{"arg"}, CoverageRequirements: []RuntimeCoverageEvidenceRequirement{{ID: "coverage-1", Scope: "direct", TargetRefs: []string{"a.go"}}}}},
	}

	cloned := CloneRuntimeOperationExecutionContext(input)
	cloned.CoverageWorkItemIDs[0] = "cwi-2"
	cloned.TargetRefs[0] = "b.go"
	cloned.Execution.ReplayProof.TargetRefs[0] = "b.go"
	cloned.EvidencePolicies[0].ArgsContain[0] = "changed"
	cloned.EvidencePolicies[0].CoverageRequirements[0].TargetRefs[0] = "b.go"

	if input.CoverageWorkItemIDs[0] != "cwi-1" || input.TargetRefs[0] != "a.go" || input.Execution.ReplayProof.TargetRefs[0] != "a.go" ||
		input.EvidencePolicies[0].ArgsContain[0] != "arg" || input.EvidencePolicies[0].CoverageRequirements[0].TargetRefs[0] != "a.go" {
		t.Fatalf("clone mutated source context: %+v", input)
	}
}

func TestRuntimeOperationExecutionEnvelopeRejectsUnboundAcquisitionIdentity(t *testing.T) {
	envelope := runtimeOperationExecutionEnvelopeForTest(RuntimeOperation{ID: "operation"}, "exec/ws/run/dispatch/operation", 1, RuntimeEffectNone, RuntimeOperationIdempotencySafeToReplay, "", "capability-state/ws/run/web/0", nil)
	if err := envelope.Validate(); err == nil {
		t.Fatal("execution envelope accepted acquisition binding without owner catalog snapshot")
	}
	envelope.AdmissibilitySnapshotID = "operation-admissibility-1"
	envelope.AuthorityHash = StableRuntimeOperationExecutionAuthorityHash(RuntimeOperationExecutionRecord{
		OperationID: "operation", OperationHash: envelope.OperationHash, ExecutionKey: envelope.ExecutionKey, ExpectedEffectScope: envelope.ExpectedEffectScope,
		IdempotencyClass: envelope.IdempotencyClass, AdmissibilitySnapshotID: envelope.AdmissibilitySnapshotID, AcquisitionBinding: envelope.AcquisitionBinding,
	})
	if err := envelope.Validate(); err != nil {
		t.Fatalf("bound execution envelope: %v", err)
	}
}

func TestRuntimeOperationExecutionEnvelopeRejectsUnresolvedEffectScope(t *testing.T) {
	content := "sealed content"
	operation := RuntimeOperation{ID: "write-1", Kind: StepKindFileWrite, TargetRefs: []string{"target.txt"}, Parameters: []OperationParameter{{Name: "content", String: &content}}}
	baseline := runtimeOperationExecutionEnvelopeForTest(operation, "exec/ws/run/dispatch/write-1", 1, RuntimeEffectWorkspaceDurable, RuntimeOperationIdempotencyAtMostOnce, "", "", nil)
	if err := baseline.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, scope := range map[string]RuntimeEffectScope{
		"missing": "", "invalid": "future_effect", "unknown": RuntimeEffectUnknown,
	} {
		t.Run(name, func(t *testing.T) {
			invalid := baseline
			invalid.ExpectedEffectScope = scope
			if err := invalid.Validate(); err == nil {
				t.Fatalf("execution envelope accepted unresolved effect scope %q", scope)
			}
		})
	}
	for name, idempotency := range map[string]RuntimeOperationIdempotencyClass{
		"missing": "", "invalid": "future_class", "unknown": RuntimeOperationIdempotencyUnknown,
	} {
		t.Run("idempotency "+name, func(t *testing.T) {
			invalid := baseline
			invalid.IdempotencyClass = idempotency
			if err := invalid.Validate(); err == nil {
				t.Fatalf("execution envelope accepted unresolved idempotency class %q", idempotency)
			}
		})
	}
}

func TestRuntimeOperationExecutionEnvelopeRejectsReplayProofAuthorityDrift(t *testing.T) {
	proof := RuntimeOperationReplayProof{
		OwnerModuleID: RuntimeModuleFile, OperationID: "write-1", TargetRefs: []string{"target.txt"},
		BeforeHash: "before", AfterHash: "after", AdmissibilitySnapshotID: "catalog-1", AcquisitionBinding: "binding-1",
	}
	envelope := runtimeOperationExecutionEnvelopeForTest(RuntimeOperation{ID: "write-1"}, "exec/ws/run/dispatch/write-1", 1, RuntimeEffectWorkspaceDurable, RuntimeOperationIdempotencyAtMostOnce, "catalog-1", "binding-1", &proof)
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}
	drifted := envelope
	drifted.AdmissibilitySnapshotID = "catalog-2"
	if err := drifted.Validate(); err == nil {
		t.Fatal("execution envelope accepted replay proof from another catalog")
	}
	drifted = envelope
	drifted.AcquisitionBinding = "binding-2"
	if err := drifted.Validate(); err == nil {
		t.Fatal("execution envelope accepted replay proof from another acquisition")
	}
}

func TestValidateRuntimeOperationExecutionAuthorityRejectsVariantAndHashDrift(t *testing.T) {
	patchType := "file_write"
	content := "updated"
	operation := RuntimeOperation{ID: "patch-1", Kind: RuntimeBlockKindPatchApply, TargetRefs: []string{"target.txt"}, Parameters: []OperationParameter{
		{Name: "patchType", String: &patchType}, {Name: "content", String: &content},
	}}
	envelope := runtimeOperationExecutionEnvelopeForTest(operation, "exec/ws/run/dispatch/patch-1", 1, RuntimeEffectWorkspaceDurable, RuntimeOperationIdempotencySafeToReplay, "", "", nil)
	if err := ValidateRuntimeOperationExecutionAuthority(operation, envelope, nil); err != nil {
		t.Fatal(err)
	}
	driftedClass := envelope
	driftedClass.IdempotencyClass = RuntimeOperationIdempotencyAtMostOnce
	if err := ValidateRuntimeOperationExecutionAuthority(operation, driftedClass, nil); err == nil {
		t.Fatal("owner preflight accepted idempotency drift")
	}
	driftedHash := envelope
	driftedHash.AuthorityHash = "different"
	if err := ValidateRuntimeOperationExecutionAuthority(operation, driftedHash, nil); err == nil {
		t.Fatal("owner preflight accepted authority hash drift")
	}
}

func runtimeOperationExecutionEnvelopeForTest(operation RuntimeOperation, executionKey string, fence uint64, effect RuntimeEffectScope, idempotency RuntimeOperationIdempotencyClass, catalogID, acquisition string, proof *RuntimeOperationReplayProof) RuntimeOperationExecutionEnvelope {
	operationHash := StableRuntimeOperationHash(operation)
	record := RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: operationHash, ExecutionKey: executionKey, ExpectedEffectScope: effect, IdempotencyClass: idempotency,
		AdmissibilitySnapshotID: catalogID, AcquisitionBinding: acquisition, ReplayProof: proof,
	}
	return RuntimeOperationExecutionEnvelope{
		ExecutionKey: executionKey, Fence: fence, OperationHash: operationHash, ExpectedEffectScope: effect, IdempotencyClass: idempotency,
		AuthorityHash: StableRuntimeOperationExecutionAuthorityHash(record), AdmissibilitySnapshotID: catalogID, AcquisitionBinding: acquisition, ReplayProof: proof,
	}
}
