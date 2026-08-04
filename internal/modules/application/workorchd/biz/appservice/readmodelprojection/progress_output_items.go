package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func resultProgressItem(item readmodelcore.Result) readmodelcore.ProgressItem {
	return readmodelcore.ProgressItem{
		ID:           item.ID,
		Kind:         "result",
		Title:        firstNonEmptyTimelineString(item.Title, item.SourceType, item.ID),
		State:        item.State,
		EvidenceRefs: compactUniqueStrings(append([]string{item.ID, item.SourceRef}, item.EvidenceRefs...)),
	}
}

func artifactProgressItem(item projectioncontract.Artifact) readmodelcore.ProgressItem {
	return readmodelcore.ProgressItem{
		ID:           item.ID,
		Kind:         "artifact",
		Title:        firstNonEmptyTimelineString(item.Name, item.URI, item.ArtifactType, item.ID),
		State:        item.State,
		EvidenceRefs: compactUniqueStrings(append([]string{item.ID, item.ResultID, item.StepID, item.ToolCallID, item.PatchID, item.ValidationResultID}, item.EvidenceRefs...)),
	}
}
