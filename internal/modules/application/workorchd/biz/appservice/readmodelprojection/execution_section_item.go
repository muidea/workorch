package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func executionSectionFromNode(snapshot readmodelcore.RunSnapshotState, decomposition projectioncontract.TaskDecomposition, node projectioncontract.TaskDecompositionNode, dodItems []string, refs executionEvidenceRefs) projectioncontract.ExecutionSection {
	section := projectioncontract.ExecutionSection{
		ID:              decomposition.ID + "/section/" + node.ID,
		WorkspaceID:     snapshot.Run.WorkspaceID,
		RunID:           snapshot.Run.ID,
		Title:           firstNonEmptyTimelineString(node.Title, node.ID, node.Kind),
		State:           node.State,
		GraphRootStepID: decomposition.RootStepID,
		GraphNodeID:     node.ID,
		StepID:          node.StepID,
		StepKind:        node.Kind,
		RelatedDoDItems: relatedDoDItemsForSection(node, dodItems),
		PlannedStepRefs: compactUniqueStrings([]string{decomposition.RootStepID, node.ID}),
		Summary:         executionSectionSummary(node),
	}
	section.ActualEvidenceRefs = compactUniqueStrings(append(cloneReadModelStringSlice(node.EvidenceRefs), refs.evidenceByStep[node.StepID]...))
	section.DecisionRefs = compactUniqueStrings(refs.decisionsByStep[node.StepID])
	section.ApprovalRefs = compactUniqueStrings(refs.approvalsByStep[node.StepID])
	section.ToolCallRefs = compactUniqueStrings(refs.toolCallsByStep[node.StepID])
	section.PatchRefs = compactUniqueStrings(refs.patchesByStep[node.StepID])
	section.ValidationRefs = compactUniqueStrings(refs.validationsByStep[node.StepID])
	section.ResultRefs = compactUniqueStrings(refs.resultsByStep[node.StepID])
	section.ArtifactRefs = compactUniqueStrings(refs.artifactsByStep[node.StepID])
	section.MemoryRecallRefs = compactUniqueStrings(refs.memoryRecallsByStep[node.StepID])
	section.MemoryIDs = compactUniqueStrings(refs.memoryIDsByStep[node.StepID])
	return section
}
