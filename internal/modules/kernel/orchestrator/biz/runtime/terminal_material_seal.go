package runtime

import (
	"context"
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// terminalMaterialSealFromAggregate 是唯一 SM-4 生成入口，传入的是独立加载的前像。
func terminalMaterialSealFromAggregate(previous RuntimeAggregate, observations ...[]runtimecontract.RuntimeTerminalSnapshotProof) (runtimecontract.RuntimeTerminalMaterialSeal, error) {
	if previous.TerminalMaterialSeal != nil || len(previous.DeliveryAcknowledgements) != 0 || previous.StateRevision == ^uint64(0) {
		return runtimecontract.RuntimeTerminalMaterialSeal{}, fmt.Errorf("%w: terminal material generation requires unsealed original aggregate", common.ErrConflict)
	}
	outcome, terminal, err := DeriveTerminalOutcome(previous.Decision, previous.Termination)
	if err != nil {
		return runtimecontract.RuntimeTerminalMaterialSeal{}, err
	}
	if !terminal {
		return runtimecontract.RuntimeTerminalMaterialSeal{}, fmt.Errorf("%w: terminal material generation requires sealed terminal", common.ErrConflict)
	}
	generation := previous.StateRevision + 1
	source := previous
	source.StateRevision = generation
	material, err := DeriveRuntimeTerminalResultMaterial(source)
	if err != nil {
		return runtimecontract.RuntimeTerminalMaterialSeal{}, err
	}
	var proofs []runtimecontract.RuntimeTerminalSnapshotProof
	if len(observations) > 1 {
		return runtimecontract.RuntimeTerminalMaterialSeal{}, common.ErrInvalidArgument
	}
	if len(observations) == 1 {
		proofs = observations[0]
	}
	material, err = runtimecontract.ApplyRuntimeTerminalSnapshotProofs(terminalMaterialSource(source), material, proofs)
	if err != nil {
		return runtimecontract.RuntimeTerminalMaterialSeal{}, err
	}
	if err := runtimecontract.ValidateRuntimeTerminalProofMaterial(terminalMaterialSource(source), material); err != nil {
		return runtimecontract.RuntimeTerminalMaterialSeal{}, err
	}
	hash, err := material.CanonicalHash()
	if err != nil {
		return runtimecontract.RuntimeTerminalMaterialSeal{}, err
	}
	seal := runtimecontract.RuntimeTerminalMaterialSeal{
		Identity: runtimecontract.RuntimeTerminalDeliveryIdentity{
			TerminalDeliveryID: runtimecontract.StableTerminalDeliveryID(previous.Facts.WorkspaceID, previous.Facts.RunID, previous.Decision.AuthoritySnapshot.ID, outcome),
			WorkspaceID:        previous.Facts.WorkspaceID, RunID: previous.Facts.RunID,
			AuthoritySnapshotID: previous.Decision.AuthoritySnapshot.ID,
			DefinitionRevision:  previous.Facts.DefinitionRevision, EvidenceRevision: previous.Facts.EvidenceRevision,
			DispatchID: previous.Decision.AuthoritySnapshot.ID, Outcome: outcome,
		},
		GenerationStateRevision: generation,
		RecoveryPolicy:          previous.Facts.RecoveryPolicy,
		RecoveryLedger:          previous.Facts.Recovery,
		Material:                material.CloneRaw(), MaterialHash: hash,
	}
	return seal, seal.ValidateRaw()
}

func (c *driveDeliveryComponent) sealTerminalMaterial(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, observations ...[]runtimecontract.RuntimeTerminalSnapshotProof) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	if len(observations) > 1 {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	var proofs []runtimecontract.RuntimeTerminalSnapshotProof
	if len(observations) == 1 {
		proofs = observations[0]
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.TerminalMaterialSeal != nil {
		if proofs != nil && !reflect.DeepEqual(proofs, aggregate.TerminalMaterialSeal.Material.RuntimeCloseout.TerminalSnapshotProofs) {
			return RuntimeAggregate{}, common.ErrConflict
		}
		return aggregate, nil
	}
	seal, err := terminalMaterialSealFromAggregate(aggregate, proofs)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	expected := aggregate.StateRevision
	aggregate.TerminalMaterialSeal = &seal
	aggregate.StateRevision++
	if err := c.transitions.Commit(ctx, aggregate, expected); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func terminalMaterialSource(aggregate RuntimeAggregate) runtimecontract.RuntimeStateRecord {
	var active *runtimecontract.RuntimeDispatchRecord
	if aggregate.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*aggregate.ActiveDispatch)
		active = &value
	}
	return runtimecontract.RuntimeStateRecord{
		Definition: aggregate.Definition, Facts: aggregate.Facts, Decision: aggregate.Decision,
		Termination: aggregate.Termination, StateRevision: aggregate.StateRevision,
		ResultFeedbackVersion: aggregate.ResultFeedbackVersion,
		ActiveDispatch:        active, CompletedDispatches: sealedDispatchesToRuntimeRecords(aggregate.CompletedDispatches),
		OperationReceipts: aggregate.OperationReceipts, DeliveryAcknowledgements: aggregate.DeliveryAcknowledgements,
		CoverageProofLineage: aggregate.CoverageProofLineage,
		SemanticPlanning:     aggregate.SemanticPlanning, ImplementationWork: aggregate.ImplementationWork,
		Reconciliation:  aggregate.Reconciliation,
		ExecutionBudget: aggregate.ExecutionBudget, EvidenceSequence: aggregate.EvidenceSequence,
		GovernanceResolutions: aggregate.GovernanceResolutions, OperationGovernanceBindings: aggregate.OperationGovernanceBindings,
		CandidateAttempts: aggregate.CandidateAttempts, CandidateRequestSessions: aggregate.CandidateRequestSessions,
		ProcessedEventIDs: aggregate.ProcessedEventIDs, RepairSurfaceConfirmations: aggregate.RepairSurfaceConfirmations,
	}
}

// validateTerminalMaterialSealTransition 反查独立持久前像，不接受 caller 提供的历史快照。
func validateTerminalMaterialSealTransition(previous, next RuntimeAggregate) error {
	if previous.TerminalMaterialSeal != nil {
		if !reflect.DeepEqual(previous.TerminalMaterialSeal, next.TerminalMaterialSeal) {
			return fmt.Errorf("%w: terminal material seal is immutable", common.ErrConflict)
		}
		return nil
	}
	if next.TerminalMaterialSeal == nil {
		return nil
	}
	if next.TerminalMaterialSeal.Material.RuntimeCloseout == nil {
		return common.ErrConflict
	}
	expected, err := terminalMaterialSealFromAggregate(previous, next.TerminalMaterialSeal.Material.RuntimeCloseout.TerminalSnapshotProofs)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, *next.TerminalMaterialSeal) {
		return fmt.Errorf("%w: terminal material seal does not match independent generation preimage", common.ErrConflict)
	}
	withoutSeal := next
	withoutSeal.TerminalMaterialSeal = nil
	withoutSeal.StateRevision = previous.StateRevision
	if !reflect.DeepEqual(previous, withoutSeal) {
		return fmt.Errorf("%w: terminal material generation must not change authority or recovery", common.ErrConflict)
	}
	return nil
}
