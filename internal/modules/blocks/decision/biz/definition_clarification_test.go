package decision

import (
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
)

func TestWorkspaceDefinitionClarificationQuestionMatchesRequiredAction(t *testing.T) {
	tests := []struct {
		name   string
		action string
		want   string
	}{
		{
			name:   "definition relation",
			action: decisionevents.DefinitionClarificationActionRewriteAllOf,
			want:   "Revise the Definition into explicit all-of semantics",
		},
		{
			name:   "profile proof contract",
			action: decisionevents.DefinitionClarificationActionFixTemplateProfile,
			want:   "Select or fix the Workspace Template/Profile proof contract",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workspaceDefinitionClarificationQuestion(tt.action); got != tt.want {
				t.Fatalf("workspaceDefinitionClarificationQuestion(%q) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}
