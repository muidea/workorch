package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeOperationExecutionEnvelope is the owner-neutral sealed execution
// identity shared by capability-owner command contracts. It carries no topic,
// target owner state or transition instruction.
type RuntimeOperationExecutionEnvelope struct {
	ExecutionKey            string
	Fence                   uint64
	OperationHash           string
	ExpectedEffectScope     RuntimeEffectScope
	IdempotencyClass        RuntimeOperationIdempotencyClass
	AuthorityHash           string
	AdmissibilitySnapshotID string
	AcquisitionBinding      string
	ReplayProof             *RuntimeOperationReplayProof
	GovernanceGrant         *RuntimeOperationGovernanceGrant
}

// RuntimeOperationExecutionContext is the immutable owner-neutral scope of one
// sealed operation execution. Capability owners embed this value in their own
// concrete EventHub command roots and add only owner-specific operation data.
// It carries no topic, destination owner, reducer instruction or state
// transition authority.
type RuntimeOperationExecutionContext struct {
	Execution                     RuntimeOperationExecutionEnvelope
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	DispatchID                    string
	Transaction                   AuthorityTransaction
	OperationID                   string
	CoverageWorkItemIDs           []common.CoverageWorkItemID
	TargetRefs                    []string
	EvidenceKind                  string
	EvidencePolicies              []RuntimeEvidencePolicy
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
}

// ValidateRuntimeOperationExecutionContextScope binds the owner-neutral
// envelope to the concrete dispatch command that carries it. Envelope-only
// validation cannot prove ResumeDispatchID because the dispatch identity lives
// on this outer context.
func ValidateRuntimeOperationExecutionContextScope(value RuntimeOperationExecutionContext) error {
	if value.WorkspaceID == "" || value.RunID == "" || strings.TrimSpace(value.DispatchID) == "" || strings.TrimSpace(value.OperationID) == "" {
		return fmt.Errorf("runtime operation execution context requires workspace, run, dispatch and operation scope")
	}
	originID, err := RuntimeOperationExecutionKeyOrigin(value.WorkspaceID, value.RunID, value.Execution.ExecutionKey, value.OperationID)
	if err != nil {
		return err
	}
	if value.Execution.GovernanceGrant == nil {
		if originID != strings.TrimSpace(value.DispatchID) {
			return fmt.Errorf("runtime operation execution context has non-current key without governance grant")
		}
	} else {
		if originID == strings.TrimSpace(value.DispatchID) {
			return fmt.Errorf("runtime operation governance grant requires a prior execution-key origin")
		}
		if value.Execution.GovernanceGrant.Normalized().ResumeDispatchID != strings.TrimSpace(value.DispatchID) {
			return fmt.Errorf("runtime operation governance grant resume dispatch mismatch")
		}
	}
	return nil
}

// CloneRuntimeOperationExecutionContext detaches all collection and optional
// execution-envelope values before the context crosses an owner boundary.
func CloneRuntimeOperationExecutionContext(value RuntimeOperationExecutionContext) RuntimeOperationExecutionContext {
	ret := value
	ret.CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), value.CoverageWorkItemIDs...)
	ret.TargetRefs = append([]string(nil), value.TargetRefs...)
	ret.EvidencePolicies = CloneRuntimeEvidencePolicies(value.EvidencePolicies)
	if value.Execution.ReplayProof != nil {
		proof := value.Execution.ReplayProof.Clone()
		ret.Execution.ReplayProof = &proof
	}
	if value.Execution.GovernanceGrant != nil {
		grant := value.Execution.GovernanceGrant.Normalized()
		ret.Execution.GovernanceGrant = &grant
	}
	return ret
}

func (e RuntimeOperationExecutionEnvelope) Validate() error {
	if e.ExecutionKey != strings.TrimSpace(e.ExecutionKey) || e.OperationHash != strings.TrimSpace(e.OperationHash) ||
		e.AuthorityHash != strings.TrimSpace(e.AuthorityHash) || e.AdmissibilitySnapshotID != strings.TrimSpace(e.AdmissibilitySnapshotID) ||
		e.AcquisitionBinding != strings.TrimSpace(e.AcquisitionBinding) {
		return fmt.Errorf("runtime operation execution envelope requires canonical authority fields")
	}
	if strings.TrimSpace(e.ExecutionKey) == "" || e.Fence == 0 || strings.TrimSpace(e.OperationHash) == "" || !validSealedRuntimeEffectScope(e.ExpectedEffectScope) {
		return fmt.Errorf("runtime operation execution envelope requires key, fence, operation hash and expected effect")
	}
	if !validSealedRuntimeOperationIdempotencyClass(e.IdempotencyClass) {
		return fmt.Errorf("runtime operation execution envelope requires an explicit valid idempotency class")
	}
	if strings.TrimSpace(e.AuthorityHash) == "" {
		return fmt.Errorf("runtime operation execution envelope requires authority hash")
	}
	if strings.TrimSpace(e.AdmissibilitySnapshotID) == "" && strings.TrimSpace(e.AcquisitionBinding) != "" {
		return fmt.Errorf("runtime operation execution acquisition binding requires admissibility snapshot")
	}
	if e.ReplayProof != nil {
		if !canonicalRuntimeOperationReplayProof(*e.ReplayProof) {
			return fmt.Errorf("runtime operation execution replay proof requires canonical fields")
		}
		if err := e.ReplayProof.Validate(); err != nil {
			return fmt.Errorf("runtime operation execution replay proof: %w", err)
		}
		proof := e.ReplayProof.Normalized()
		if proof.AdmissibilitySnapshotID != strings.TrimSpace(e.AdmissibilitySnapshotID) ||
			proof.AcquisitionBinding != strings.TrimSpace(e.AcquisitionBinding) {
			return fmt.Errorf("runtime operation execution replay proof owner authority mismatch")
		}
	}
	if e.GovernanceGrant != nil {
		grant := e.GovernanceGrant.Normalized()
		if err := grant.Validate(); err != nil {
			return err
		}
		if grant.ExecutionKey != strings.TrimSpace(e.ExecutionKey) || grant.ResumeFence != e.Fence {
			return fmt.Errorf("runtime operation governance grant does not match execution envelope")
		}
	}
	return nil
}

// ValidateRuntimeOperationExecutionAuthority is the capability-owner preflight
// for the immutable SM-3 execution authority. It independently resolves the
// exact operation policy and verifies that the envelope is the authority seal
// for this operation; mutable claim/fence state is deliberately outside the
// hash. Capability owners must run this before attempting a physical effect.
func ValidateRuntimeOperationExecutionAuthority(operation RuntimeOperation, envelope RuntimeOperationExecutionEnvelope, policies []RuntimeOperationPolicyFact) error {
	operation = operation.Normalized()
	if operation.ID == "" || operation.Kind == "" {
		return fmt.Errorf("runtime operation execution authority requires operation id and kind")
	}
	if err := ValidateRuntimeOperationContract(operation); err != nil {
		return fmt.Errorf("runtime operation execution authority operation: %w", err)
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	if envelope.OperationHash != StableRuntimeOperationHash(operation) {
		return fmt.Errorf("runtime operation execution authority operation hash mismatch")
	}
	expectedScope := ResolveExpectedEffectScope(operation, policies)
	if expectedScope == RuntimeEffectUnknown || envelope.ExpectedEffectScope != expectedScope {
		return fmt.Errorf("runtime operation execution authority effect scope mismatch")
	}
	expectedIdempotency := ResolveOperationIdempotencyClass(operation, policies)
	if envelope.IdempotencyClass != expectedIdempotency {
		return fmt.Errorf("runtime operation execution authority idempotency class mismatch")
	}
	sealed := RuntimeOperationExecutionRecord{
		OperationID:             operation.ID,
		OperationHash:           envelope.OperationHash,
		ExecutionKey:            envelope.ExecutionKey,
		ExpectedEffectScope:     envelope.ExpectedEffectScope,
		IdempotencyClass:        envelope.IdempotencyClass,
		AuthorityHash:           envelope.AuthorityHash,
		AdmissibilitySnapshotID: envelope.AdmissibilitySnapshotID,
		AcquisitionBinding:      envelope.AcquisitionBinding,
		ReplayProof:             envelope.ReplayProof,
	}
	if envelope.AuthorityHash != StableRuntimeOperationExecutionAuthorityHash(sealed) {
		return fmt.Errorf("runtime operation execution authority hash mismatch")
	}
	if envelope.ReplayProof != nil {
		descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind)
		proof := envelope.ReplayProof.Normalized()
		if !known || proof.OwnerModuleID != descriptor.OwnerModuleID || proof.OperationID != operation.ID ||
			!sameRuntimeStringSet(proof.TargetRefs, operation.TargetRefs) {
			return fmt.Errorf("runtime operation execution replay proof does not match sealed operation")
		}
	}
	return nil
}
