package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/client"
)

// FetchWorkspaces fetches the workspace list from the API.
func FetchWorkspaces(cl *client.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		workspaces, err := cl.ListWorkspaces(ctx)
		return WorkspaceListMsg{Workspaces: workspaces, Error: err}
	}
}

// FetchDecisions fetches all open decisions from the API.
func FetchDecisions(cl *client.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		decisions, err := cl.ListDecisions(ctx, "", "")
		return DecisionListMsg{Decisions: decisions, Error: err}
	}
}

// FetchApprovals fetches all pending approvals from the API.
func FetchApprovals(cl *client.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		approvals, err := cl.ListApprovals(ctx, "", "")
		return ApprovalListMsg{Approvals: approvals, Error: err}
	}
}

// FetchWorkspaceSnapshot fetches the full snapshot for a workspace.
func FetchWorkspaceSnapshot(cl *client.Client, workspaceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		snap, err := cl.GetWorkspaceSnapshot(ctx, workspaceID)
		return WorkspaceSnapshotMsg{WorkspaceID: workspaceID, Snapshot: snap, Error: err}
	}
}

// FetchRunSnapshot fetches the full run snapshot.
func FetchRunSnapshot(cl *client.Client, runID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		snap, err := cl.GetRunSnapshot(ctx, runID)
		return RunSnapshotMsg{RunID: runID, Snapshot: snap, Error: err}
	}
}

// FetchRunObservability fetches the run observability with timeline.
func FetchRunObservability(cl *client.Client, runID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		obs, err := cl.GetRunObservability(ctx, runID)
		return RunObservabilityMsg{RunID: runID, Observability: obs, Error: err}
	}
}

// PollTick returns a tick message after the given interval.
func PollTick(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// FetchAll fetches workspace list, decisions, and approvals in parallel.
func FetchAll(cl *client.Client) tea.Cmd {
	return tea.Batch(
		FetchWorkspaces(cl),
		FetchDecisions(cl),
		FetchApprovals(cl),
	)
}
