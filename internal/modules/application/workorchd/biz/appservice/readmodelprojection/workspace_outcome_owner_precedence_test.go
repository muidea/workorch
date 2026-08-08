package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestWorkspaceOutcomeAndActivityPreferCurrentTerminalRunOverDefinitionHint(t *testing.T) {
	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-current-terminal", State: string(common.WorkspaceStateActive)},
		Goals:     []workorchmanagementcontract.Goal{{ID: "goal-current", DefinitionRevision: "definition-current", PreviousDefinitionHash: "goal-before"}},
		DoDs:      []workorchmanagementcontract.DoD{{ID: "dod-current", DefinitionRevision: "definition-current", PreviousDefinitionHash: "dod-before"}},
		Runs: []appmodelcore.Run{{
			ID: "run-current", WorkspaceID: "ws-current-terminal", GoalID: "goal-current", DoDID: "dod-current",
			DefinitionRevision: "definition-current", State: string(common.RunStateFailed), FailureReason: "semantic contract rejected",
		}},
	}
	snapshot.Outcome = MapWorkspaceOutcome(snapshot)
	if snapshot.Outcome.RunID != "run-current" || snapshot.Outcome.State != string(common.RunStateFailed) || !snapshot.Outcome.Terminal || snapshot.Outcome.Reason != "semantic contract rejected" {
		t.Fatalf("workspace outcome = %+v", snapshot.Outcome)
	}
	feed := BuildWorkspaceActivityFeedContent(snapshot)
	found := false
	for _, item := range feed.Items {
		if item.Title == "outcome" {
			found = true
			if item.RunID != snapshot.Outcome.RunID || item.State != snapshot.Outcome.State || item.Summary != snapshot.Outcome.Reason {
				t.Fatalf("activity outcome = %+v, canonical = %+v", item, snapshot.Outcome)
			}
		}
	}
	if !found {
		t.Fatal("canonical terminal outcome was absent from activity feed")
	}
}
