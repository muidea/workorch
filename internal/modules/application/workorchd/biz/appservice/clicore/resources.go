package clicore

import (
	"encoding/json"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/events"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
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
	}
	return ResourceMapAlias(v)
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
	return resource
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
	if typed := typedWorkspaceSnapshotInfo(snapshot); typed != nil {
		resource[events.CLIResultTypedResourceKey] = typed
	}
	return resource
}

func typedWorkspaceSnapshotInfo(snapshot readmodelcore.WorkspaceSnapshot) *port.WorkspaceSnapshotInfo {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil
	}
	var out port.WorkspaceSnapshotInfo
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return &out
}

func ResourceMapAlias(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func ResourceMaps[T any](items []T) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, ResourceMap(item))
	}
	return out
}
