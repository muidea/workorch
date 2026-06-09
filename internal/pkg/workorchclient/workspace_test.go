package workorchclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientDecodesWorkspaceProjectionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workorch/workspaces":
			_, _ = w.Write([]byte(`{
				"service":"workorch",
				"projection":{"state":"ready","globalRevision":12,"workspaceTotal":1,"sourceEventRevision":88},
				"workspaces":[{"id":"ws-1","title":"A","state":"active"}]
			}`))
		case "/api/v1/workorch/workspaces/ws-1/snapshot":
			_, _ = w.Write([]byte(`{
				"service":"workorch",
				"projection":{"state":"ready","globalRevision":12,"workspaceRevision":7,"sourceEventRevision":91,"degraded":false},
				"snapshot":{"workspace":{"id":"ws-1","title":"A","state":"running"}}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL)
	list, err := client.ListWorkspaceProjection(context.Background())
	if err != nil {
		t.Fatalf("list workspace projection: %v", err)
	}
	if list.Projection.State != "ready" || list.Projection.GlobalRevision != 12 || list.Projection.SourceEventRevision != 88 || len(list.Workspaces) != 1 {
		t.Fatalf("list projection = %+v, want ready revision 12 with one workspace", list)
	}

	detail, err := client.GetWorkspaceSnapshotProjection(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("workspace snapshot projection: %v", err)
	}
	if detail.Projection.WorkspaceRevision != 7 || detail.Projection.SourceEventRevision != 91 || detail.Snapshot.Workspace.ID != "ws-1" {
		t.Fatalf("detail projection = %+v, want workspace revision 7 for ws-1", detail)
	}
}
