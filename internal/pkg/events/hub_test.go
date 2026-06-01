package events

import (
	"context"
	"testing"
)

func TestMemoryHubAssignsWorkspaceRevision(t *testing.T) {
	hub := NewMemoryHub()

	if err := hub.Publish(context.Background(), Envelope{Topic: TopicWorkspaceStateChanged, WorkspaceID: "ws-a"}); err != nil {
		t.Fatalf("publish first workspace event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{Topic: TopicRunStateChanged, WorkspaceID: "ws-a", RunID: "run-a"}); err != nil {
		t.Fatalf("publish second workspace event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{Topic: TopicWorkspaceCreated, WorkspaceID: "ws-b"}); err != nil {
		t.Fatalf("publish other workspace event: %v", err)
	}
	if err := hub.Publish(context.Background(), Envelope{Topic: TopicCLICommand}); err != nil {
		t.Fatalf("publish global event: %v", err)
	}

	published := hub.Published()
	if len(published) != 4 {
		t.Fatalf("published count = %d, want 4", len(published))
	}
	if published[0].Revision != 1 || published[1].Revision != 2 || published[2].Revision != 1 || published[3].Revision != 0 {
		t.Fatalf("revisions = %d/%d/%d/%d, want workspace-scoped monotonic revisions and zero global revision",
			published[0].Revision,
			published[1].Revision,
			published[2].Revision,
			published[3].Revision,
		)
	}
}
