package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
)

func FetchAll(be backend.Backend) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		workspaces, _ := be.Workspace().List(ctx)
		return WorkspaceListMsg{Workspaces: workspaces}
	}
}
