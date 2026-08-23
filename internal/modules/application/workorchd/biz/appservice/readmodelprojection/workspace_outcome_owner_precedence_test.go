package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestWorkspaceOutcomeAndActivityPreferCurrentTerminalRunOverDefinitionHint(t *testing.T) {
	definitionRevision := "goal:goal-current@goal:definition-current|dod:dod-current@dod:definition-current"
	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-current-terminal", State: string(common.WorkspaceStateActive)},
		Goals:     []workorchmanagementcontract.Goal{{ID: "goal-current", DefinitionRevision: "goal:definition-current", PreviousDefinitionHash: "goal-before"}},
		DoDs:      []workorchmanagementcontract.DoD{{ID: "dod-current", DefinitionRevision: "dod:definition-current", PreviousDefinitionHash: "dod-before"}},
		Runs: []appmodelcore.Run{{
			ID: "run-current", WorkspaceID: "ws-current-terminal", GoalID: "goal-current", DoDID: "dod-current",
			DefinitionRevision: definitionRevision, State: string(common.RunStateFailed), FailureReason: "semantic contract rejected",
		}},
	}
	snapshot.DisplayOutcome = mapWorkspaceOutcome(snapshot)
	if snapshot.DisplayOutcome.RunID != "run-current" || snapshot.DisplayOutcome.State != string(common.RunStateFailed) || !snapshot.DisplayOutcome.Terminal || snapshot.DisplayOutcome.Reason != "semantic contract rejected" {
		t.Fatalf("workspace outcome = %+v", snapshot.DisplayOutcome)
	}
	feed := buildWorkspaceActivityFeedContent(snapshot)
	found := false
	for _, item := range feed.Items {
		if item.Title == "outcome" {
			found = true
			if item.RunID != snapshot.DisplayOutcome.RunID || item.State != snapshot.DisplayOutcome.State || item.Summary != snapshot.DisplayOutcome.Reason {
				t.Fatalf("activity outcome = %+v, canonical = %+v", item, snapshot.DisplayOutcome)
			}
		}
	}
	if !found {
		t.Fatal("canonical terminal outcome was absent from activity feed")
	}
}
