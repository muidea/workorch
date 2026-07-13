package clicore

import "testing"

func TestMemoryCommandFromPayloadUsesCompatibleIDKeys(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		wantID  string
	}{
		{
			name: "memoryId",
			payload: map[string]any{
				"memoryId": "mem-1",
				"summary":  "summary",
			},
			wantID: "mem-1",
		},
		{
			name: "ID",
			payload: map[string]any{
				"ID":      "mem-2",
				"summary": "summary",
			},
			wantID: "mem-2",
		},
		{
			name: "id",
			payload: map[string]any{
				"id":      "mem-3",
				"summary": "summary",
			},
			wantID: "mem-3",
		},
		{
			name: "decoded memoryId wins",
			payload: map[string]any{
				"memoryId": "mem-4",
				"id":       "ignored",
				"summary":  "summary",
			},
			wantID: "mem-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MemoryCommandFromPayload(tt.payload)
			if got.MemoryID != tt.wantID {
				t.Fatalf("MemoryID = %q, want %q", got.MemoryID, tt.wantID)
			}
			if got.Summary != "summary" {
				t.Fatalf("Summary = %q, want summary", got.Summary)
			}
		})
	}
}

func TestCommandHelpersFromPayload(t *testing.T) {
	workspace := CreateWorkspaceCommandFromPayload(map[string]any{
		"title":                "workspace",
		"description":          "description",
		"deliverableDirectory": "workspaces/ws",
		"templateId":           "template-1",
	})
	if workspace.Title != "workspace" || workspace.Description != "description" ||
		workspace.DeliverableDirectory != "workspaces/ws" || workspace.TemplateID != "template-1" {
		t.Fatalf("workspace command = %+v", workspace)
	}

	activation := ActivateWorkspaceExecutionCommandFromPayload(map[string]any{
		"goalId":         "goal-1",
		"doDId":          "dod-1",
		"llmProviderId":  "provider-1",
		"dialogLanguage": "zh",
		"soulId":         "soul-1",
		"soulBindings":   map[string]any{"validation": "soul-validation"},
		"skillIds":       []any{"skill-1", "skill-2"},
		"mcpServerIds":   []string{"mcp-1"},
		"capabilityIds":  []any{"capability-1", "capability-2"},
		"memoryIds":      []string{"memory-1"},
		"priority":       float64(7),
		"restart":        true,
	})
	if activation.GoalID != "goal-1" || activation.DoDID != "dod-1" ||
		activation.LLMProviderID != "provider-1" || activation.DialogLanguage != "zh" || activation.SoulID != "soul-1" ||
		activation.SoulBindings["validation"] != "soul-validation" ||
		len(activation.SkillIDs) != 2 || activation.SkillIDs[1] != "skill-2" ||
		len(activation.MCPServerIDs) != 1 || activation.MCPServerIDs[0] != "mcp-1" ||
		len(activation.CapabilityIDs) != 2 || activation.CapabilityIDs[1] != "capability-2" ||
		len(activation.MemoryIDs) != 1 || activation.MemoryIDs[0] != "memory-1" ||
		activation.Priority != 7 || !activation.Restart {
		t.Fatalf("activation command = %+v", activation)
	}

	decision := ResolveDecisionCommandFromPayload(map[string]any{
		"resolutionType": "provide_missing_info",
		"payload":        map[string]any{"answer": "done"},
	})
	if decision.ResolutionType != "provide_missing_info" || decision.ResolutionPayload["answer"] != "done" {
		t.Fatalf("decision command = %+v", decision)
	}

	batchDecision := BatchResolveDecisionCommandFromPayload(map[string]any{
		"ticketIds":         []any{"ticket-1", "ticket-2"},
		"resolutionType":    "replan",
		"resolutionPayload": map[string]any{"plan": "next"},
	})
	if len(batchDecision.TicketIDs) != 2 || batchDecision.TicketIDs[0] != "ticket-1" ||
		batchDecision.ResolutionType != "replan" || batchDecision.ResolutionPayload["plan"] != "next" {
		t.Fatalf("batch decision command = %+v", batchDecision)
	}

	approval := BatchResolveApprovalCommandFromPayload(map[string]any{
		"approvalIds": []any{"approval-1", "approval-2"},
		"action":      "approve",
	})
	if len(approval.ApprovalIDs) != 2 || approval.ApprovalIDs[1] != "approval-2" || approval.Action != "approve" {
		t.Fatalf("approval command = %+v", approval)
	}

	query := MemorySearchQueryFromPayload("ws-1", map[string]any{
		"query": "repo",
		"tags":  []any{"tag-1"},
		"types": []string{"note"},
		"limit": int64(3),
	})
	if query.WorkspaceID != "ws-1" || query.Query != "repo" ||
		len(query.Tags) != 1 || query.Tags[0] != "tag-1" ||
		len(query.Types) != 1 || query.Types[0] != "note" || query.Limit != 3 {
		t.Fatalf("memory query = %+v", query)
	}
}
