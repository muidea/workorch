package presenter

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

type StepView = readmodelcore.StepView
type ObservedView = readmodelcore.ObservedView

func MapStep(item *runmodel.Step) *StepView {
	if item == nil {
		return nil
	}
	return &StepView{
		ID:     string(item.ID),
		Kind:   llmcommon.CanonicalRuntimeBlockKind(item.Kind),
		State:  string(item.State),
		Input:  clonePresenterMap(item.Input),
		Output: clonePresenterMap(item.Output),
	}
}

func MapSteps(items []*runmodel.Step) []StepView {
	if len(items) == 0 {
		return nil
	}
	out := make([]StepView, 0, len(items))
	for _, item := range items {
		mapped := MapStep(item)
		if mapped == nil {
			continue
		}
		out = append(out, *mapped)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func MapObserved(items []observed.SnapshotItem) []ObservedView {
	if len(items) == 0 {
		return nil
	}
	out := make([]ObservedView, 0, len(items))
	for _, item := range items {
		out = append(out, ObservedView{
			ID:           item.ID,
			Version:      item.Version,
			Topic:        item.Topic,
			OccurredAt:   item.OccurredAt,
			SourceModule: item.SourceModule,
			SubjectType:  item.SubjectType,
			SubjectID:    item.SubjectID,
			WorkspaceID:  item.WorkspaceID,
			RunID:        item.RunID,
			StepID:       item.StepID,
			Payload:      item.PayloadMap(),
		})
	}
	return out
}

func clonePresenterMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = clonePresenterValue(v)
	}
	return out
}

func clonePresenterSlice(input []any) []any {
	if input == nil {
		return nil
	}
	out := make([]any, len(input))
	for i, v := range input {
		out[i] = clonePresenterValue(v)
	}
	return out
}

func clonePresenterValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return clonePresenterMap(typed)
	case []any:
		return clonePresenterSlice(typed)
	default:
		return typed
	}
}
