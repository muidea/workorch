package events

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResultApplyRuntimeTerminalDeliveryCommandValidate(t *testing.T) {
	cmd := ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator", CausationID: "terminal-delivery/td-1",
		TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
		Material: ResultMaterial{ID: "ws-1/run-1/result/dod-assessment", WorkspaceID: "ws-1", RunID: "run-1", GoalID: "goal-1", DoDID: "dod-1", Summary: "done"},
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid terminal: %v", err)
	}
	cmd.Material.WorkspaceID = "ws-other"
	if err := cmd.Validate(); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mismatched result state err = %v", err)
	}
	cmd.Material = ResultMaterial{}
	if err := cmd.Validate(); err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("missing result err = %v", err)
	}
}
