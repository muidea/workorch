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
	ExpectedEffectScope     RuntimeEffectScope
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
	if strings.TrimSpace(e.ExecutionKey) == "" || e.Fence == 0 || !ValidRuntimeEffectScope(e.ExpectedEffectScope) {
		return fmt.Errorf("runtime operation execution envelope requires key, fence and expected effect")
	}
	if strings.TrimSpace(e.AdmissibilitySnapshotID) == "" && strings.TrimSpace(e.AcquisitionBinding) != "" {
		return fmt.Errorf("runtime operation execution acquisition binding requires admissibility snapshot")
	}
	if e.ReplayProof != nil {
		if err := e.ReplayProof.Validate(); err != nil {
			return fmt.Errorf("runtime operation execution replay proof: %w", err)
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
