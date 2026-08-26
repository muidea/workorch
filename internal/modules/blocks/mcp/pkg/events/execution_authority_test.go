package events

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

func testExecutionEnvelope(operation runtimecontract.RuntimeOperation, envelope runtimecontract.RuntimeOperationExecutionEnvelope, policies []runtimecontract.RuntimeOperationPolicyFact) runtimecontract.RuntimeOperationExecutionEnvelope {
	envelope.OperationHash = runtimecontract.StableRuntimeOperationHash(operation)
	envelope.IdempotencyClass = runtimecontract.ResolveOperationIdempotencyClass(operation, policies)
	envelope.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: envelope.OperationHash, ExecutionKey: envelope.ExecutionKey, ExpectedEffectScope: envelope.ExpectedEffectScope,
		IdempotencyClass: envelope.IdempotencyClass, AdmissibilitySnapshotID: envelope.AdmissibilitySnapshotID,
		AcquisitionBinding: envelope.AcquisitionBinding, ReplayProof: envelope.ReplayProof,
	})
	return envelope
}
