package clicore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
)

func ResourceMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	switch typed := v.(type) {
	case readmodelcore.Run:
		return runResourceMap(typed)
	case *readmodelcore.Run:
		if typed == nil {
			return nil
		}
		return runResourceMap(*typed)
	case readmodelcore.WorkspaceSnapshot:
		return workspaceSnapshotResourceMap(typed)
	case *readmodelcore.WorkspaceSnapshot:
		if typed == nil {
			return nil
		}
		return workspaceSnapshotResourceMap(*typed)
	case appmodelcore.Workspace:
		return typedResourceMap(workspaceResourceMap(typed), readmodelports.PortWorkspaceInfo(typed))
	case *appmodelcore.Workspace:
		if typed == nil {
			return nil
		}
		return typedResourceMap(workspaceResourceMap(*typed), readmodelports.PortWorkspaceInfo(*typed))
	case appmodelcore.Goal:
		return typedResourceMap(goalResourceMap(typed), readmodelports.PortGoalInfo(typed))
	case *appmodelcore.Goal:
		if typed == nil {
			return nil
		}
		return typedResourceMap(goalResourceMap(*typed), readmodelports.PortGoalInfo(*typed))
	case appmodelcore.DoD:
		return typedResourceMap(dodResourceMap(typed), readmodelports.PortDoDInfo(typed))
	case *appmodelcore.DoD:
		if typed == nil {
			return nil
		}
		return typedResourceMap(dodResourceMap(*typed), readmodelports.PortDoDInfo(*typed))
	case appmodelcore.Decision:
		return typedResourceMap(decisionResourceMap(typed), readmodelports.PortDecisionInfo(typed))
	case *appmodelcore.Decision:
		if typed == nil {
			return nil
		}
		return typedResourceMap(decisionResourceMap(*typed), readmodelports.PortDecisionInfo(*typed))
	case appmodelcore.Approval:
		return typedResourceMap(approvalResourceMap(typed), readmodelports.PortApprovalInfo(typed))
	case *appmodelcore.Approval:
		if typed == nil {
			return nil
		}
		return typedResourceMap(approvalResourceMap(*typed), readmodelports.PortApprovalInfo(*typed))
	case readmodelcore.RunStatus:
		return typedResourceMap(runStatusResourceMap(typed), readmodelports.PortRunStatusInfo(typed))
	case *readmodelcore.RunStatus:
		if typed == nil {
			return nil
		}
		return typedResourceMap(runStatusResourceMap(*typed), readmodelports.PortRunStatusInfo(*typed))
	case contextregistrycore.Soul:
		return typedResourceMap(soulResourceMap(typed), readmodelports.PortSoulInfo(typed))
	case *contextregistrycore.Soul:
		if typed == nil {
			return nil
		}
		return typedResourceMap(soulResourceMap(*typed), readmodelports.PortSoulInfo(*typed))
	case contextregistrycore.Skill:
		return typedResourceMap(skillResourceMap(typed), readmodelports.PortSkillInfo(typed))
	case *contextregistrycore.Skill:
		if typed == nil {
			return nil
		}
		return typedResourceMap(skillResourceMap(*typed), readmodelports.PortSkillInfo(*typed))
	case contextregistrycore.MCPServer:
		return typedResourceMap(mcpServerResourceMap(typed), readmodelports.PortMCPServerInfo(typed))
	case *contextregistrycore.MCPServer:
		if typed == nil {
			return nil
		}
		return typedResourceMap(mcpServerResourceMap(*typed), readmodelports.PortMCPServerInfo(*typed))
	case contextregistrycore.MCPCapability:
		return typedResourceMap(mcpCapabilityResourceMap(typed), readmodelports.PortMCPCapabilityInfo(typed))
	case *contextregistrycore.MCPCapability:
		if typed == nil {
			return nil
		}
		return typedResourceMap(mcpCapabilityResourceMap(*typed), readmodelports.PortMCPCapabilityInfo(*typed))
	case contextregistrycore.MemoryItem:
		return typedResourceMap(memoryResourceMap(typed), readmodelports.PortMemoryItemInfo(typed))
	case *contextregistrycore.MemoryItem:
		if typed == nil {
			return nil
		}
		return typedResourceMap(memoryResourceMap(*typed), readmodelports.PortMemoryItemInfo(*typed))
	case workspacecore.WorkspaceTemplate:
		return typedResourceMap(templateResourceMap(typed), readmodelports.PortTemplateInfo(typed))
	case *workspacecore.WorkspaceTemplate:
		if typed == nil {
			return nil
		}
		return typedResourceMap(templateResourceMap(*typed), readmodelports.PortTemplateInfo(*typed))
	}
	return nil
}

func runResourceMap(run readmodelcore.Run) map[string]any {
	resource := map[string]any{
		"id":                 run.ID,
		"workspaceId":        run.WorkspaceID,
		"goalId":             run.GoalID,
		"doDId":              run.DoDID,
		"initialStepKind":    run.InitialStepKind,
		"state":              run.State,
		"priority":           run.Priority,
		"blockingTicketId":   run.BlockingTicketID,
		"blockingApprovalId": run.BlockingApprovalID,
		"failureReason":      run.FailureReason,
	}
	if contextMap := CompactExecutionContext(run.ExecutionContext); len(contextMap) > 0 {
		resource["executionContext"] = contextMap
	}
	return typedResourceMap(resource, readmodelports.PortRunInfo(run))
}

func workspaceSnapshotResourceMap(snapshot readmodelcore.WorkspaceSnapshot) map[string]any {
	runs := make([]map[string]any, 0, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		runs = append(runs, runResourceMap(run))
	}
	eventRecords := make([]map[string]any, 0, len(snapshot.EventRecords))
	for _, record := range snapshot.EventRecords {
		eventRecords = append(eventRecords, map[string]any{
			"id":             record.ID,
			"version":        record.Version,
			"topic":          record.Topic,
			"occurredAt":     record.OccurredAt,
			"sourceModule":   record.SourceModule,
			"subjectType":    record.SubjectType,
			"subjectId":      record.SubjectID,
			"workspaceId":    record.WorkspaceID,
			"runId":          record.RunID,
			"stepId":         record.StepID,
			"payloadSummary": CloneResultValue(record.PayloadSummary, 0),
			"payloadRef":     record.PayloadRef,
		})
	}
	resource := map[string]any{
		"workspace":             snapshot.Workspace,
		"viewModel":             snapshot.ViewModel,
		"outcome":               snapshot.Outcome,
		"taskProgress":          snapshot.TaskProgress,
		"runHeartbeat":          snapshot.RunHeartbeat,
		"coverageSummary":       CloneResultValue(snapshot.CoverageSummary, 0),
		"gapBaseline":           CloneResultValue(snapshot.GapBaseline, 0),
		"taskDecompositions":    snapshot.TaskDecompositions,
		"goals":                 snapshot.Goals,
		"dods":                  snapshot.DoDs,
		"runs":                  runs,
		"results":               snapshot.Results,
		"artifacts":             snapshot.Artifacts,
		"artifactContinuations": snapshot.ArtifactContinuations,
		"patches":               snapshot.Patches,
		"webSources":            snapshot.WebSources,
		"llmUsageTraces":        snapshot.LLMUsageTraces,
		"llmUsageSummary":       snapshot.LLMUsageSummary,
		"eventRecords":          eventRecords,
		"decisions":             snapshot.Decisions,
		"approvals":             snapshot.Approvals,
	}
	return typedResourceMap(resource, readmodelports.PortWorkspaceSnapshotInfo(snapshot))
}

func ResourceMaps[T any](items []T) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, ResourceMap(item))
	}
	return out
}

func typedResourceMap(resource map[string]any, typed any) map[string]any {
	if resource == nil {
		resource = map[string]any{}
	}
	resource[typedResourceKey] = typed
	return resource
}

func workspaceResourceMap(item appmodelcore.Workspace) map[string]any {
	return map[string]any{
		"id":                   item.ID,
		"title":                item.Title,
		"description":          item.Description,
		"deliverableDirectory": item.DeliverableDirectory,
		"state":                item.State,
		"activeRunId":          item.ActiveRunID,
		"blockingTicketId":     item.BlockingTicketID,
		"blockingApprovalId":   item.BlockingApprovalID,
		"templateId":           item.TemplateID,
		"profileId":            item.ProfileID,
		"defaultSoulId":        item.DefaultSoulID,
		"soulBindings":         cloneStringMap(item.SoulBindings),
	}
}

func goalResourceMap(item appmodelcore.Goal) map[string]any {
	return map[string]any{"id": item.ID, "workspaceId": item.WorkspaceID, "text": item.Text}
}

func dodResourceMap(item appmodelcore.DoD) map[string]any {
	return map[string]any{"id": item.ID, "workspaceId": item.WorkspaceID, "items": append([]string(nil), item.Items...), "state": item.State}
}

func decisionResourceMap(item appmodelcore.Decision) map[string]any {
	return map[string]any{
		"id":                 item.ID,
		"workspaceId":        item.WorkspaceID,
		"runId":              item.RunID,
		"reason":             item.Reason,
		"question":           item.Question,
		"context":            CloneResultValue(item.Context, 0),
		"state":              item.State,
		"resolutionType":     item.ResolutionType,
		"continuationAction": item.ContinuationAction,
		"resumeRunId":        item.ResumeRunID,
		"successorRunId":     item.SuccessorRunID,
	}
}

func approvalResourceMap(item appmodelcore.Approval) map[string]any {
	return map[string]any{
		"id":          item.ID,
		"workspaceId": item.WorkspaceID,
		"runId":       item.RunID,
		"reason":      item.Reason,
		"action":      item.Action,
		"context":     CloneResultValue(item.Context, 0),
		"state":       item.State,
	}
}

func runStatusResourceMap(item readmodelcore.RunStatus) map[string]any {
	return map[string]any{
		"id":                   item.ID,
		"workspaceId":          item.WorkspaceID,
		"state":                item.State,
		"currentStepId":        item.CurrentStepID,
		"currentStepKind":      item.CurrentStepKind,
		"currentStepState":     item.CurrentStepState,
		"currentActionLabel":   item.CurrentActionLabel,
		"diagnosticLineage":    item.DiagnosticLineage,
		"stepCount":            item.StepCount,
		"decisionCount":        item.DecisionCount,
		"approvalCount":        item.ApprovalCount,
		"blockingTicketId":     item.BlockingTicketID,
		"blockingApprovalId":   item.BlockingApprovalID,
		"failureReason":        item.FailureReason,
		"runtimeExecution":     item.RuntimeExecution,
		"convergenceStatus":    item.ConvergenceStatus,
		"schedulerDiagnostics": item.SchedulerDiagnostics,
	}
}

func soulResourceMap(item contextregistrycore.Soul) map[string]any {
	return map[string]any{
		"id":              item.ID,
		"name":            item.Name,
		"description":     item.Description,
		"businessDomains": append([]string(nil), item.BusinessDomains...),
		"soulMd":          item.SoulMD,
	}
}

func skillResourceMap(item contextregistrycore.Skill) map[string]any {
	return map[string]any{
		"id":          item.ID,
		"name":        item.Name,
		"version":     item.Version,
		"description": item.Description,
		"instruction": item.Instruction,
	}
}

func mcpServerResourceMap(item contextregistrycore.MCPServer) map[string]any {
	return map[string]any{
		"id":        item.ID,
		"name":      item.Name,
		"transport": item.Transport,
		"endpoint":  item.Endpoint,
		"enabled":   item.Enabled,
	}
}

func mcpCapabilityResourceMap(item contextregistrycore.MCPCapability) map[string]any {
	return map[string]any{
		"id":          item.ID,
		"serverId":    item.ServerID,
		"kind":        item.Kind,
		"name":        item.Name,
		"displayName": item.DisplayName,
		"description": item.Description,
	}
}

func memoryResourceMap(item contextregistrycore.MemoryItem) map[string]any {
	return map[string]any{
		"id":                  item.ID,
		"workspaceId":         item.WorkspaceID,
		"runId":               item.RunID,
		"sourceType":          item.SourceType,
		"sourceRef":           item.SourceRef,
		"memoryType":          item.MemoryType,
		"content":             item.Content,
		"summary":             item.Summary,
		"tags":                append([]string(nil), item.Tags...),
		"importance":          item.Importance,
		"confidence":          item.Confidence,
		"visibility":          item.Visibility,
		"embeddingProviderId": item.EmbeddingProviderID,
		"embeddingModel":      item.EmbeddingModel,
		"embeddingText":       item.EmbeddingText,
		"vectorReady":         item.VectorReady,
		"vectorDimension":     item.VectorDimension,
		"vectorScore":         item.VectorScore,
	}
}

func templateResourceMap(item workspacecore.WorkspaceTemplate) map[string]any {
	return map[string]any{
		"id":               item.ID,
		"name":             item.Name,
		"description":      item.Description,
		"goalText":         item.GoalText,
		"doDItems":         append([]string(nil), item.DoDItems...),
		"defaultSoulId":    item.DefaultSoulID,
		"soulBindings":     cloneStringMap(item.SoulBindings),
		"evidencePolicies": CloneResultValue(item.EvidencePolicies, 0),
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
