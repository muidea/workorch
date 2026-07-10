package clicore

import (
	"context"
	"time"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
)

func (d *Dispatcher) dispatchCommand(ctx context.Context, cmd Command) workorchdevents.CLIResult {
	result := workorchdevents.CLIResult{
		CorrelationID: cmd.CorrelationID,
		CommandID:     cmd.CommandID,
		OccurredAt:    time.Now().UTC(),
		WorkspaceID:   cmd.WorkspaceID,
		RunID:         cmd.RunID,
	}
	resource, resources, err := d.handler(ctx, cmd)
	if err != nil {
		result.OK = false
		result.Error = err.Error()
		return result
	}
	result.OK = true
	assignCLIResultResource(&result, typedResource(resource))
	for _, item := range typedResources(resources) {
		assignCLIResultResource(&result, item)
	}
	return result
}

func typedResource(resource map[string]any) any {
	if resource == nil {
		return nil
	}
	return TypedResource(resource)
}

func typedResources(resources []map[string]any) []any {
	if len(resources) == 0 {
		return nil
	}
	typed := make([]any, 0, len(resources))
	for _, resource := range resources {
		item := typedResource(resource)
		if item != nil {
			typed = append(typed, item)
		}
	}
	return typed
}

func assignCLIResultResource(result *workorchdevents.CLIResult, resource any) {
	if result == nil || resource == nil {
		return
	}
	switch typed := resource.(type) {
	case workorchport.WorkspaceInfo:
		result.Workspace = &typed
	case workorchport.WorkspaceSnapshotInfo:
		result.WorkspaceSnapshot = &typed
	case workorchport.GoalInfo:
		result.Goal = &typed
	case workorchport.DoDInfo:
		result.DoD = &typed
	case workorchport.RunInfo:
		result.Run = &typed
	case workorchport.RunStatusInfo:
		result.RunStatus = &typed
	case workorchport.DecisionInfo:
		result.Decision = &typed
	case workorchport.ApprovalInfo:
		result.Approval = &typed
	case workorchport.SoulInfo:
		result.Soul = &typed
	case workorchport.SkillInfo:
		result.Skill = &typed
	case workorchport.MCPServerInfo:
		result.MCPServer = &typed
	case workorchport.MCPCapabilityInfo:
		result.MCPCapabilities = append(result.MCPCapabilities, typed)
	case workorchport.MemoryItemInfo:
		result.Memory = &typed
	case workorchport.TemplateInfo:
		result.Template = &typed
	case workorchport.ServiceConfig:
		result.Configuration = &typed
	}
	appendCLIResultCollection(result, resource)
}

func appendCLIResultCollection(result *workorchdevents.CLIResult, resource any) {
	switch typed := resource.(type) {
	case workorchport.WorkspaceInfo:
		result.Workspaces = append(result.Workspaces, typed)
	case workorchport.GoalInfo:
		result.Goals = append(result.Goals, typed)
	case workorchport.DoDInfo:
		result.DoDs = append(result.DoDs, typed)
	case workorchport.RunInfo:
		result.Runs = append(result.Runs, typed)
	case workorchport.DecisionInfo:
		result.Decisions = append(result.Decisions, typed)
	case workorchport.ApprovalInfo:
		result.Approvals = append(result.Approvals, typed)
	case workorchport.SoulInfo:
		result.Souls = append(result.Souls, typed)
	case workorchport.SkillInfo:
		result.Skills = append(result.Skills, typed)
	case workorchport.MCPServerInfo:
		result.MCPServers = append(result.MCPServers, typed)
	case workorchport.MemoryItemInfo:
		result.Memories = append(result.Memories, typed)
	case workorchport.TemplateInfo:
		result.Templates = append(result.Templates, typed)
	}
}
