package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/client"
)

func StatusView(workspaces []client.Workspace, cursor, width int) string {
	header := Title.Render("WorkOrch Status Dashboard") + "\n\n"

	if len(workspaces) == 0 {
		return header + Dim.Render("  No workspaces found.") + "\n"
	}

	rows := []string{}
	for i, ws := range workspaces {
		prefix := "  "
		if i == cursor {
			prefix = Selection.Render("▶ ")
		}
		line := fmt.Sprintf("%s%-24s %-30s %-28s %s",
			prefix,
			truncateStr(ws.ID, 22),
			truncateStr(ws.Title, 28),
			StateStyle(ws.State),
			Dim.Render(truncateStr(ws.ActiveRunID, 16)),
		)
		rows = append(rows, line)
	}

	table := fmt.Sprintf("%s%-24s %-30s %-28s %s\n%s\n",
		"  ", Heading.Render("ID"), Heading.Render("Title"), Heading.Render("State"), Heading.Render("Active Run"),
		strings.Repeat("─", width-4),
	)

	return header + table + strings.Join(rows, "\n") + "\n"
}

func WorkspaceDetailView(ws *client.Workspace, snap *client.WorkspaceSnapshot, cursor, width int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Padding(0, 1)
	header := titleStyle.Render(fmt.Sprintf("Workspace: %s - %s", ws.ID, ws.Title)) + "\n"

	stateLine := fmt.Sprintf("  State: %s", StateStyle(ws.State))
	if ws.ActiveRunID != "" {
		stateLine += fmt.Sprintf("  |  Active Run: %s", ws.ActiveRunID)
	}
	header += stateLine + "\n\n"

	if snap == nil {
		return header + Dim.Render("  Loading...") + "\n"
	}

	if len(snap.Steps) > 0 {
		header += Heading.Render("  Recent Steps:") + "\n"
		for _, s := range snap.Steps {
			icon := "•"
			switch s.State {
			case "completed":
				icon = Success.Render("✓")
			case "failed":
				icon = Error.Render("✗")
			case "dispatched", "pending":
				icon = Warning.Render("⏳")
			}
			header += fmt.Sprintf("    %s %-30s %s\n", icon, s.Kind, Dim.Render(s.State))
		}
		header += "\n"
	}

	openDecisions := filterOpenDecisions(snap.Decisions)
	if len(openDecisions) > 0 {
		header += Warning.Render(fmt.Sprintf("  ⚠ Decisions (%d open):", len(openDecisions))) + "\n"
		for i, d := range openDecisions {
			prefix := "   "
			if i == cursor {
				prefix = Selection.Render(" ▶ ")
			}
			header += fmt.Sprintf("%s%s: %s\n", prefix, d.ID, truncateStr(d.Question, 60))
		}
		header += "\n"
	}

	pendingApprovals := filterPendingApprovals(snap.Approvals)
	if len(pendingApprovals) > 0 {
		header += Alert.Render(fmt.Sprintf("  ⚡ Approvals (%d pending):", len(pendingApprovals))) + "\n"
		for i, a := range pendingApprovals {
			prefix := "   "
			if i+len(openDecisions) == cursor {
				prefix = Selection.Render(" ▶ ")
			}
			header += fmt.Sprintf("%s%s: %s\n", prefix, a.ID, truncateStr(a.Reason, 60))
		}
		header += "\n"
	}

	return header
}

func filterOpenDecisions(items []client.Decision) []client.Decision {
	var out []client.Decision
	for _, d := range items {
		if d.State == "open" {
			out = append(out, d)
		}
	}
	return out
}

func filterPendingApprovals(items []client.Approval) []client.Approval {
	var out []client.Approval
	for _, a := range items {
		if a.State == "pending" {
			out = append(out, a)
		}
	}
	return out
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
