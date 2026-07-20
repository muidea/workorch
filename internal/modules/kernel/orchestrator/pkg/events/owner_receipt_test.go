package events

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOperationExecutionEnvelopeValidatesGovernanceGrantIdentity(t *testing.T) {
	grant := runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", TicketID: common.TicketID("ticket-1"),
		ExecutionKey: "exec/ws/run/dispatch/op", GovernanceFence: 1, ResumeFence: 2,
		DependencyKind: "approval", SubjectRef: "target.txt", GovernanceReceiptHash: "receipt-hash",
	}
	envelope := RuntimeOperationExecutionEnvelope{
		ExecutionKey: grant.ExecutionKey, Fence: grant.ResumeFence,
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, GovernanceGrant: &grant,
	}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("valid governance grant envelope: %v", err)
	}
	stale := grant
	stale.ResumeFence++
	envelope.GovernanceGrant = &stale
	if err := envelope.Validate(); err == nil {
		t.Fatal("stale governance grant fence must be rejected")
	}
}

func TestNewRuntimeOperationReceiptCarriesExecutionFence(t *testing.T) {
	execution := RuntimeOperationExecutionEnvelope{
		ExecutionKey: "exec/ws/run/dispatch/op", Fence: 3,
		ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
	}
	receipt := NewRuntimeOperationReceipt(execution, "op-1", runtimecontract.RuntimeOperationExecutionStatusAccepted, nil,
		runtimecontract.RuntimeOperationEvidence{Summary: "changed"}, nil, nil, "2026-07-20T00:00:00Z")
	if receipt.ExecutionKey != execution.ExecutionKey || receipt.Fence != execution.Fence {
		t.Fatalf("receipt identity = %+v", receipt)
	}
	if receipt.CompletedAt != "2026-07-20T00:00:00Z" {
		t.Fatalf("completedAt = %q", receipt.CompletedAt)
	}
}
