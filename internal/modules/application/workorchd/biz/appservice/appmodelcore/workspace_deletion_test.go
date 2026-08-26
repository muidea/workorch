package appmodelcore

import "testing"

func TestDeleteWorkspaceCommandDefaultRequestIDIsStableAndModeIndependent(t *testing.T) {
	retain := (DeleteWorkspaceCommand{}).Normalized("ws-stable-delete")
	purge := (DeleteWorkspaceCommand{Purge: true}).Normalized("ws-stable-delete")
	if retain.RequestID == "" || retain.RequestID != purge.RequestID {
		t.Fatalf("retain=%q purge=%q, want one stable Workspace-scoped request ID", retain.RequestID, purge.RequestID)
	}
	other := (DeleteWorkspaceCommand{}).Normalized("ws-other-delete")
	if other.RequestID == retain.RequestID {
		t.Fatalf("different Workspace IDs shared request ID %q", other.RequestID)
	}
}
