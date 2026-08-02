package contracts

import "testing"

func TestExecutionDefinitionRevisionIncludesOwnerContentRevisions(t *testing.T) {
	first := ExecutionDefinitionRevision("goal-1", "goal:content-a", "dod-1", "dod:content-a")
	second := ExecutionDefinitionRevision("goal-1", "goal:content-b", "dod-1", "dod:content-a")
	if first == "" || second == "" || first == second {
		t.Fatalf("definition revisions = %q / %q, want same resource IDs separated by content revision", first, second)
	}
	if got := ExecutionDefinitionRevision("goal-1", "", "dod-1", "dod:content-a"); got != "" {
		t.Fatalf("incomplete definition revision = %q, want empty", got)
	}
}
