package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

func StatusView(workspaces []backend.WorkspaceInfo, cursor, width int) string {
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
			truncateStr(string(ws.ID), 22),
			truncateStr(ws.Title, 28),
			StateStyle(string(ws.State)),
			Dim.Render(truncateStr(string(ws.ActiveRunID), 16)),
		)
		rows = append(rows, line)
	}

	table := fmt.Sprintf("%s%-24s %-30s %-28s %s\n%s\n",
		"  ", Heading.Render("ID"), Heading.Render("Title"), Heading.Render("State"), Heading.Render("Active Run"),
		strings.Repeat("─", width-4),
	)

	return header + table + strings.Join(rows, "\n") + "\n"
}

func WorkspaceDetailView(ws *backend.WorkspaceInfo, cursor, width int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Padding(0, 1)
	if ws == nil {
		return titleStyle.Render("Workspace") + "\n" + Dim.Render("  No workspace selected.") + "\n"
	}
	header := titleStyle.Render(fmt.Sprintf("Workspace: %s - %s", string(ws.ID), ws.Title)) + "\n"

	stateLine := fmt.Sprintf("  State: %s", StateStyle(string(ws.State)))
	if string(ws.ActiveRunID) != "" {
		stateLine += fmt.Sprintf("  |  Active Run: %s", string(ws.ActiveRunID))
	}
	header += stateLine + "\n\n"

	if ws.ActiveRunID != "" {
		header += Heading.Render("  Active Run:") + fmt.Sprintf(" %s\n", string(ws.ActiveRunID))
	}
	header += Dim.Render("  (Run details available via 'workorch run show')\n")
	return header
}

func filterOpenDecisions(items []backend.DecisionInfo) []backend.DecisionInfo {
	var out []backend.DecisionInfo
	for _, d := range items {
		if d.State == "open" {
			out = append(out, d)
		}
	}
	return out
}

func filterPendingApprovals(items []backend.ApprovalInfo) []backend.ApprovalInfo {
	var out []backend.ApprovalInfo
	for _, a := range items {
		if a.State == "pending" {
			out = append(out, a)
		}
	}
	return out
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
