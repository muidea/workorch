package events

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestApplyRunProjectionFactCommandValidate(t *testing.T) {
	cmd := ApplyRunProjectionFactCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "run-state/run-1/1",
		RunState: common.RunStateActive, AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1",
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid projection: %v", err)
	}
	cmd.RunState = common.RunStateCompleted
	if err := cmd.Validate(); err == nil {
		t.Fatal("expected unsupported terminal run state")
	}
}

func TestWorkspaceApplyRuntimeTerminalDeliveryCommandValidate(t *testing.T) {
	cmd := ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1",
		Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid terminal: %v", err)
	}
	if err := (ApplyRuntimeTerminalDeliveryCommand{}).Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty terminal err = %v", err)
	}
}
