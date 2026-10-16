package orchestrator

import (
	"context"
	"fmt"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only capability adapters interpret concrete snapshot protocol/grammar. Core
// consumes bounded typed owner facts and independently re-proves their sources.
func (m *Biz) observeRuntimeTerminalSnapshotProofs(ctx context.Context, aggregate runtime.RuntimeAggregate) ([]runtimecontract.RuntimeTerminalSnapshotProof, error) {
	if aggregate.TerminalMaterialSeal != nil {
		return nil, nil
	}
	sources, err := runtime.CompileRuntimeTerminalSnapshotSourcesFromAggregate(aggregate)
	if err != nil {
		return nil, err
	}
	var proofs []runtimecontract.RuntimeTerminalSnapshotProof
	for _, source := range sources {
		if source.Assertion.Rule.OwnerModuleID != runtimecontract.RuntimeModuleGit {
			return nil, fmt.Errorf("terminal snapshot owner adapter unavailable")
		}
		input, err := runtime.CompileRuntimeValidationInputSnapshotSource(aggregate, source.Validation.DispatchID, source.Validation.OperationID, source.Validation.ReceiptHash)
		if err != nil {
			return nil, err
		}
		requestID := "terminal-snapshot/" + runtimecontract.StableRuntimeLineageAssertionRuleHash(source.Assertion.Rule) + "/" + string(source.Assertion.CoverageWorkItemID)
		observation, err := m.queryRuntimeGitCommitSnapshotWithInputs(ctx, aggregate, source.Snapshot.DispatchID, source.Snapshot.OperationID, source.Snapshot.ReceiptHash, requestID, &input, true)
		if err != nil {
			return nil, err
		}
		if observation.CurrentDependencies == nil || observation.WorkspaceClean == nil || !*observation.WorkspaceClean || observation.SnapshotHead != observation.ObservedHead {
			return nil, fmt.Errorf("terminal snapshot owner observed stale HEAD or dirty workspace/index")
		}
		proof := runtimecontract.RuntimeTerminalSnapshotProof{ProtocolRef: runtimecontract.RuntimeTerminalOwnerSnapshotProtocol, Source: source, ObservedAtStateRevision: aggregate.StateRevision, SnapshotID: observation.SnapshotHead, RootSnapshotID: observation.SnapshotRootTree, ObservedSnapshotID: observation.ObservedHead, WorkspaceClean: *observation.WorkspaceClean, CurrentDependencies: *runtimecontract.CloneRuntimeOwnerRevisionSet(observation.CurrentDependencies)}
		for _, member := range observation.InputMemberships {
			proof.InputMemberships = append(proof.InputMemberships, runtimecontract.RuntimeTerminalSnapshotMembership{TargetRef: member.TargetRef, ContentHash: member.ContentHash, ObjectID: member.BlobID})
		}
		for _, member := range observation.ValidationMemberships {
			proof.ValidationMemberships = append(proof.ValidationMemberships, runtimecontract.RuntimeTerminalSnapshotMembership{TargetRef: member.TargetRef, ContentHash: member.ContentHash, ObjectID: member.BlobID})
		}
		proof = proof.WithHash()
		if err := proof.ValidateRaw(); err != nil {
			return nil, err
		}
		proofs = append(proofs, proof)
	}
	return proofs, nil
}
