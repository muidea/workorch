package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeOperationExecutionEnvelope is the owner-neutral sealed execution
// identity shared by capability-owner command contracts. It carries no topic,
// target owner state or transition instruction.
type RuntimeOperationExecutionEnvelope struct {
	ExecutionKey        string
	Fence               uint64
	ExpectedEffectScope RuntimeEffectScope
	ReplayProof         *RuntimeOperationReplayProof
	GovernanceGrant     *RuntimeOperationGovernanceGrant
}

func (e RuntimeOperationExecutionEnvelope) Validate() error {
	if strings.TrimSpace(e.ExecutionKey) == "" || e.Fence == 0 || !ValidRuntimeEffectScope(e.ExpectedEffectScope) {
		return fmt.Errorf("runtime operation execution envelope requires key, fence and expected effect")
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
