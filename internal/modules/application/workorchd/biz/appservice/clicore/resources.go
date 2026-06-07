package clicore

import (
	"encoding/json"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
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
	resource := ResourceMapAlias(run)
	if resource == nil {
		return nil
	}
	if contextMap, ok := resource["executionContext"].(map[string]any); ok {
		resource["executionContext"] = CompactExecutionContext(contextMap)
	}
	return resource
}

func workspaceSnapshotResourceMap(snapshot readmodelcore.WorkspaceSnapshot) map[string]any {
	slim := snapshot
	slim.Runs = nil
	resource := ResourceMapAlias(slim)
	if resource == nil {
		return nil
	}
	runs := make([]map[string]any, 0, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		runs = append(runs, runResourceMap(run))
	}
	resource["runs"] = runs
	return resource
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
