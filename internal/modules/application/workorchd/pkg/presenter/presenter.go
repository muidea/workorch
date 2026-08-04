package presenter

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapStep(item *runmodel.Step) *workorchprojectioncontract.StepView {
	if item == nil {
		return nil
	}
	return &workorchprojectioncontract.StepView{
		ID:     string(item.ID),
		Kind:   llmcommon.CanonicalRuntimeBlockKind(item.Kind),
		State:  string(item.State),
		Input:  clonePresenterMap(item.Input.Map()),
		Output: clonePresenterMap(item.Output.Map()),
	}
}

func MapSteps(items []*runmodel.Step) []workorchprojectioncontract.StepView {
	if len(items) == 0 {
		return nil
	}
	out := make([]workorchprojectioncontract.StepView, 0, len(items))
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

func MapObserved(items []observed.SnapshotItem) []workorchprojectioncontract.ObservedView {
	if len(items) == 0 {
		return nil
	}
	out := make([]workorchprojectioncontract.ObservedView, 0, len(items))
	for _, item := range items {
		out = append(out, workorchprojectioncontract.ObservedView{
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
