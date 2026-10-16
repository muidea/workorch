package workorchruntimecontract

import (
	"fmt"
	"reflect"
)

// RuntimeTerminalMaterialSeal 是首次交付前由 SM-4 同一次 CAS 封存的不可变材料。
// GenerationStateRevision 指该 CAS 的 after-image revision，不是后续回执自报的 revision。
type RuntimeTerminalMaterialSeal struct {
	Identity                RuntimeTerminalDeliveryIdentity
	GenerationStateRevision uint64
	RecoveryPolicy          RuntimeRecoveryPolicy
	RecoveryLedger          RuntimeRecoveryLedger
	Material                RuntimeTerminalResultMaterial
	MaterialHash            string
}

func (s RuntimeTerminalMaterialSeal) CloneRaw() RuntimeTerminalMaterialSeal {
	s.Material = s.Material.CloneRaw()
	return s
}

func (s RuntimeTerminalMaterialSeal) ValidateRaw() error {
	if s.Identity != s.Identity.Normalized() || s.GenerationStateRevision == 0 {
		return fmt.Errorf("terminal material seal requires canonical identity and generation revision")
	}
	if err := s.Identity.Validate(); err != nil {
		return err
	}
	if err := s.Material.ValidateRaw(); err != nil {
		return fmt.Errorf("terminal material seal: %w", err)
	}
	body := s.Material.RuntimeCloseout
	if body == nil || body.CloseoutID == "" || body.RuntimeStateRevision != s.GenerationStateRevision ||
		body.DefinitionRevision != s.Identity.DefinitionRevision || body.TerminalOutcome != s.Identity.Outcome ||
		s.Material.WorkspaceID != s.Identity.WorkspaceID || s.Material.RunID != s.Identity.RunID ||
		!reflect.DeepEqual(body.RecoveryPolicy, s.RecoveryPolicy) || !reflect.DeepEqual(body.RecoveryLedger, s.RecoveryLedger) {
		return fmt.Errorf("terminal material seal generation, recovery or material identity mismatch")
	}
	hash, err := s.Material.CanonicalHash()
	if err != nil {
		return err
	}
	if hash != s.MaterialHash {
		return fmt.Errorf("terminal material seal material hash mismatch")
	}
	return nil
}

// ValidateRuntimeTerminalMaterialSeal 不调用 aggregate 校验，供 fresh-load、回执与完成谓词共用。
// 尚未首次 seal 的 terminal 可以等待交付，但不能拥有回执。
func ValidateRuntimeTerminalMaterialSeal(seal *RuntimeTerminalMaterialSeal, source RuntimeStateRecord) error {
	revision, facts, decision, termination, acknowledgements := source.StateRevision, source.Facts, source.Decision, source.Termination, source.DeliveryAcknowledgements
	if err := ValidateRuntimeStateTerminalDeliveryLedger(decision, termination, acknowledgements, facts.Recovery); err != nil {
		return err
	}
	if seal == nil {
		if len(acknowledgements) != 0 {
			return fmt.Errorf("terminal delivery acknowledgement requires original material seal")
		}
		return nil
	}
	if err := seal.ValidateRaw(); err != nil {
		return err
	}
	if decision.AuthoritySnapshot.WorkspaceID != facts.WorkspaceID || decision.AuthoritySnapshot.RunID != facts.RunID ||
		decision.AuthoritySnapshot.DefinitionRevision != facts.DefinitionRevision || decision.AuthoritySnapshot.EvidenceRevision != facts.EvidenceRevision {
		return fmt.Errorf("terminal material seal authority scope or revision mismatch")
	}
	outcome, terminal, err := DeriveRuntimeTerminalOutcome(decision, termination)
	if err != nil {
		return err
	}
	expected := RuntimeTerminalDeliveryIdentity{
		TerminalDeliveryID: StableTerminalDeliveryID(facts.WorkspaceID, facts.RunID, decision.AuthoritySnapshot.ID, outcome),
		WorkspaceID:        facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: decision.AuthoritySnapshot.ID,
		DefinitionRevision: facts.DefinitionRevision, EvidenceRevision: facts.EvidenceRevision,
		DispatchID: decision.AuthoritySnapshot.ID, Outcome: outcome,
	}
	if !terminal || seal.Identity != expected || seal.GenerationStateRevision > revision {
		return fmt.Errorf("terminal material seal does not match sealed terminal authority")
	}
	// 历史 recovery 与 generation 仅从 owner seal 取；绝不以待验证回执填回期望值。
	preimage := source
	preimage.StateRevision = seal.GenerationStateRevision
	preimage.Facts = cloneRuntimeFactsRaw(source.Facts)
	preimage.Facts.RecoveryPolicy = seal.RecoveryPolicy
	preimage.Facts.Recovery = seal.RecoveryLedger
	material, err := DeriveRuntimeTerminalResultMaterialFromState(preimage)
	if err != nil {
		return err
	}
	material, err = ApplyRuntimeTerminalSnapshotProofs(preimage, material, seal.Material.RuntimeCloseout.TerminalSnapshotProofs)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(material, seal.Material) {
		return fmt.Errorf("terminal material seal does not match original authority material")
	}
	if err := ValidateRuntimeTerminalProofMaterial(preimage, seal.Material); err != nil {
		return err
	}
	for _, acknowledgement := range acknowledgements {
		if acknowledgement.Owner == RuntimeDeliveryOwnerResult &&
			(acknowledgement.ResultCloseout == nil || !reflect.DeepEqual(*acknowledgement.ResultCloseout, *seal.Material.RuntimeCloseout)) {
			return fmt.Errorf("Result acknowledgement material does not match original terminal seal")
		}
	}
	return nil
}
