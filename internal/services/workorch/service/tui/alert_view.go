package tui

import (
	"fmt"
	"strings"

	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

func AlertView(workspaces []backend.WorkspaceInfo, decisions []backend.DecisionInfo, approvals []backend.ApprovalInfo, visible bool, width int) string {
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
			c := counts[string(string(d.WorkspaceID))]
			c.decisions++
			counts[string(string(d.WorkspaceID))] = c
		}
	}
	for _, a := range approvals {
		if a.State == "pending" {
			c := counts[string(string(a.WorkspaceID))]
			c.approvals++
			counts[string(string(a.WorkspaceID))] = c
		}
	}

	if len(counts) == 0 {
		return ""
	}

	var lines []string
	for _, ws := range workspaces {
		c, ok := counts[string(string(ws.ID))]
		if !ok {
			continue
		}
		shortID := string(string(ws.ID))
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
	return AlertBorder.Width(width).Render(content)
}
