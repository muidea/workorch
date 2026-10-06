package tui

import (
	"strings"
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestInspectNoRunDoesNotPromoteWorkspaceProgressToRunOwnerState(t *testing.T) {
	m := NewAppModel(newFake(t), "ws", "")
	m.activeWorkspaceID = "ws"
	m.applyWorkspaceRuntimeView(projectioncontract.WorkspaceRuntimeView{
		WorkspaceID:  "ws",
		TaskProgress: projectioncontract.TaskProgress{WorkspaceID: "ws", State: "active", Summary: "创作失败，不存在 Run"},
	}, true)
	text := m.renderInspectStatus()
	if strings.Contains(text, "run owner state:") || strings.Contains(text, "- run: ") {
		t.Fatalf("Workspace progress 冒充 Run: %s", text)
	}
}
