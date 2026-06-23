package runtimecore

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func ContextIDs(items []map[string]any, key string) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		if id, _ := item[key].(string); strings.TrimSpace(id) != "" {
			ret = append(ret, strings.TrimSpace(id))
		}
	}
	return ret
}

func StringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func ContainsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func CloneContextMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func MapRun(item *runmodel.Run) appmodelcore.Run {
	if item == nil {
		return appmodelcore.Run{}
	}
	return appmodelcore.Run{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		GoalID:             string(item.GoalID),
		DoDID:              string(item.DoDID),
		DefinitionRevision: item.DefinitionRevision,
		InitialStepKind:    item.InitialStepKind,
		ExecutionContext:   CloneContextMap(item.ExecutionContext),
		State:              string(item.State),
		Priority:           item.Priority,
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		FailureReason:      item.FailureReason,
	}
}
