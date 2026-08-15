package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapObserved(items []observed.SnapshotItem) []projectioncontract.ObservedView {
	if len(items) == 0 {
		return nil
	}
	out := make([]projectioncontract.ObservedView, 0, len(items))
	for _, item := range items {
		out = append(out, projectioncontract.ObservedView{
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
