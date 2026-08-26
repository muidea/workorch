package workorchdeletioncontract

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceRuntimeQuiescenceTokenRequiresTerminalDeliveryAcknowledgement(t *testing.T) {
	intent := adversarialRuntimeQuiescenceIntent("ws-ack-required", "deletion-ack-required", ModeRetain, 1, 1)
	token := adversarialRuntimeQuiescenceToken(intent, "run-001", common.RunStateCompleted, 7, false)

	if err := token.Validate(); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("unacknowledged terminal-delivery token error = %v, want conflict", err)
	}
	if err := token.ValidateFor(intent); err == nil {
		t.Fatal("unacknowledged terminal-delivery token unexpectedly validates for retirement")
	}
}

func TestWorkspaceRuntimeQuiescenceTokenBindsWorkspaceWithSameRunID(t *testing.T) {
	leftIntent := adversarialRuntimeQuiescenceIntent("ws-left", "deletion-left", ModePurge, 1, 1)
	rightIntent := adversarialRuntimeQuiescenceIntent("ws-right", "deletion-right", ModePurge, 1, 1)
	left := adversarialRuntimeQuiescenceToken(leftIntent, "run-001", common.RunStateFailed, 9, true)
	right := adversarialRuntimeQuiescenceToken(rightIntent, "run-001", common.RunStateFailed, 9, true)

	if err := left.ValidateFor(leftIntent); err != nil {
		t.Fatalf("left token validate: %v", err)
	}
	if err := right.ValidateFor(rightIntent); err != nil {
		t.Fatalf("right token validate: %v", err)
	}
	if left.QuiescenceID == right.QuiescenceID {
		t.Fatal("same RunID in different Workspaces produced the same quiescence token hash")
	}
	if err := left.ValidateFor(rightIntent); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("cross-Workspace token error = %v, want conflict", err)
	}
}

func TestWorkspaceRuntimeQuiescenceTokenAllowsDurableGenerationGaps(t *testing.T) {
	intent := adversarialRuntimeQuiescenceIntent("ws-generation-gap", "deletion-generation-gap", ModeRetain, 1, 3)
	emptyInventory := WorkspaceRuntimeQuiescenceToken{
		WorkspaceID: intent.WorkspaceID, DeletionID: intent.DeletionID, DeletionGeneration: intent.DeletionGeneration,
		DataDispositionIntent: intent.DataDispositionIntent, WorkspaceIntentHash: intent.WorkspaceIntentHash,
		RunDrainingRevision: intent.RunDrainingRevision, WorkspaceRunGeneration: intent.WorkspaceRunGeneration,
		OrchestratorQuiescenceRevision: 2,
	}
	sealAdversarialRuntimeQuiescenceToken(&emptyInventory)
	if err := emptyInventory.ValidateFor(intent); err != nil {
		t.Fatalf("head generation 3 with empty current inventory validate: %v", err)
	}

	onlyGenerationTwo := adversarialRuntimeQuiescenceToken(intent, "run-002", common.RunStateCompleted, 7, true)
	onlyGenerationTwo.Runs[0].WorkspaceRunGeneration = 2
	sealAdversarialRuntimeQuiescenceToken(&onlyGenerationTwo)
	if err := onlyGenerationTwo.ValidateFor(intent); err != nil {
		t.Fatalf("head generation 3 with only generation 2 validate: %v", err)
	}

	firstAndThird := adversarialRuntimeQuiescenceToken(intent, "run-003", common.RunStateCompleted, 9, true)
	first := firstAndThird.Runs[0]
	first.RunID = "run-001"
	first.WorkspaceRunGeneration = 1
	first.RuntimeStateRef = "runtime-state/ws-generation-gap/run-001/9"
	first.TerminalDeliveryID = "terminal-delivery-generation-1"
	firstAndThird.Runs = append([]WorkspaceRuntimeQuiescenceRun{first}, firstAndThird.Runs...)
	sealAdversarialRuntimeQuiescenceToken(&firstAndThird)
	if err := firstAndThird.ValidateFor(intent); err != nil {
		t.Fatalf("head generation 3 with generations 1 and 3 validate: %v", err)
	}
}

func TestWorkspaceRuntimeQuiescenceTokenRejectsStaleDrainingAndPhysicalSnapshot(t *testing.T) {
	intent := adversarialRuntimeQuiescenceIntent("ws-stale", "deletion-stale", ModeRetain, 3, 2)
	token := adversarialRuntimeQuiescenceToken(intent, "run-002", common.RunStateCancelled, 11, true)
	if err := token.ValidateFor(intent); err != nil {
		t.Fatalf("valid token: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*WorkspaceRuntimeQuiescenceToken)
	}{
		{name: "draining revision", mutate: func(value *WorkspaceRuntimeQuiescenceToken) { value.RunDrainingRevision++ }},
		{name: "Workspace generation", mutate: func(value *WorkspaceRuntimeQuiescenceToken) { value.WorkspaceRunGeneration++ }},
		{name: "Run revision", mutate: func(value *WorkspaceRuntimeQuiescenceToken) { value.Runs[0].RunRevision++ }},
		{name: "Runtime revision", mutate: func(value *WorkspaceRuntimeQuiescenceToken) { value.Runs[0].RuntimeStateRevision++ }},
		{name: "Runtime hash", mutate: func(value *WorkspaceRuntimeQuiescenceToken) { value.Runs[0].RuntimeStateHash = "runtime-hash:stale" }},
		{name: "terminal delivery", mutate: func(value *WorkspaceRuntimeQuiescenceToken) {
			value.Runs[0].TerminalDeliveryID = "terminal-delivery-stale"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := CloneWorkspaceRuntimeQuiescenceToken(token)
			tt.mutate(&changed)
			if err := changed.ValidateFor(intent); err == nil {
				t.Fatal("stale token unexpectedly validates without an exact hash")
			}
		})
	}
}

func adversarialRuntimeQuiescenceIntent(workspaceID common.WorkspaceID, deletionID string, mode Mode, drainingRevision, workspaceGeneration uint64) WorkspaceRuntimeQuiescenceIntent {
	intent := WorkspaceRuntimeQuiescenceIntent{
		WorkspaceID:            workspaceID,
		DeletionID:             deletionID,
		DeletionGeneration:     1,
		DataDispositionIntent:  mode,
		RunDrainingRevision:    drainingRevision,
		WorkspaceRunGeneration: workspaceGeneration,
	}
	intent.WorkspaceIntentHash = "workspace-intent-hash:" + deletionID
	return intent
}

func adversarialRuntimeQuiescenceToken(intent WorkspaceRuntimeQuiescenceIntent, runID common.RunID, state common.RunState, runtimeRevision uint64, acknowledged bool) WorkspaceRuntimeQuiescenceToken {
	token := WorkspaceRuntimeQuiescenceToken{
		WorkspaceID:                    intent.WorkspaceID,
		DeletionID:                     intent.DeletionID,
		DeletionGeneration:             intent.DeletionGeneration,
		DataDispositionIntent:          intent.DataDispositionIntent,
		WorkspaceIntentHash:            intent.WorkspaceIntentHash,
		RunDrainingRevision:            intent.RunDrainingRevision,
		WorkspaceRunGeneration:         intent.WorkspaceRunGeneration,
		OrchestratorQuiescenceRevision: 2,
		Runs: []WorkspaceRuntimeQuiescenceRun{{
			RunID:                  runID,
			WorkspaceRunGeneration: intent.WorkspaceRunGeneration,
			RunRevision:            5,
			RunState:               state,
			RuntimeStatePresent:    true,
			RuntimeStateRevision:   runtimeRevision,
			RuntimeStateRef:        "runtime-state/" + string(intent.WorkspaceID) + "/" + string(runID) + "/" + adversarialUint(runtimeRevision),
			RuntimeStateHash:       "runtime-state-hash:current",
			TerminalDeliveryID:     "terminal-delivery-current",
			TerminalDeliveryAcked:  acknowledged,
		}},
	}
	sealAdversarialRuntimeQuiescenceToken(&token)
	return token
}

func sealAdversarialRuntimeQuiescenceToken(token *WorkspaceRuntimeQuiescenceToken) {
	token.RuntimeInventoryHash = StableWorkspaceRuntimeInventoryHash(*token)
	token.QuiescenceID = StableWorkspaceRuntimeQuiescenceID(*token)
}

func adversarialUint(value uint64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	idx := len(digits)
	for value > 0 {
		idx--
		digits[idx] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[idx:])
}
