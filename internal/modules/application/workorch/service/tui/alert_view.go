package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/client"
)

func AlertView(workspaces []client.Workspace, decisions []client.Decision, approvals []client.Approval, visible bool, width int) string {
	if !visible {
		return ""
	}

	type alertCount struct {
		decisions int
		approvals int
		failed    int
	}

	counts := map[string]alertCount{}
	for _, d := range decisions {
		if d.State == "open" {
			c := counts[d.WorkspaceID]
			c.decisions++
			counts[d.WorkspaceID] = c
		}
	}
	for _, a := range approvals {
		if a.State == "pending" {
			c := counts[a.WorkspaceID]
			c.approvals++
			counts[a.WorkspaceID] = c
		}
	}

	if len(counts) == 0 {
		return ""
	}

	var lines []string
	for _, ws := range workspaces {
		c, ok := counts[ws.ID]
		if !ok {
			continue
		}
		shortID := ws.ID
		if len(shortID) > 6 {
			shortID = shortID[:6]
		}
		if c.approvals > 0 {
			lines = append(lines, fmt.Sprintf("%s %s(%d)", Alert.Render("⚡"), shortID, c.approvals))
		}
		if c.decisions > 0 {
			lines = append(lines, fmt.Sprintf("%s %s(%d)", Warning.Render("⚠"), shortID, c.decisions))
		}
	}

	content := strings.Join(lines, "\n") + "\n" + Dim.Render(" Ctrl+T")
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("208")).Width(width).Render(content)
}
