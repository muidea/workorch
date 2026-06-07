package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func executionEvidenceRefsByStep(snapshot readmodelcore.RunSnapshot) executionEvidenceRefs {
	ret := newExecutionEvidenceRefs()
	for _, item := range snapshot.Decisions {
		ret.add(ret.decisionsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.Approvals {
		ret.add(ret.approvalsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.ToolCalls {
		ret.add(ret.toolCallsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.Patches {
		ret.add(ret.patchesByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.ValidationResults {
		ret.add(ret.validationsByStep, item.StepID, item.ID, item.EvidenceRef)
	}
	for _, item := range snapshot.Results {
		if item.SourceType == "dod_assessment" {
			ret.add(ret.resultsByStep, item.SourceRef, append([]string{item.ID}, item.EvidenceRefs...)...)
		}
	}
	for _, item := range snapshot.Artifacts {
		ret.add(ret.artifactsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.MemoryRecallTraces {
		ret.add(ret.memoryRecallsByStep, item.StepID, item.ID, item.EvidenceRef)
		ret.add(ret.memoryIDsByStep, item.StepID, item.RetrievedMemoryIDs...)
	}
	ret.compact()
	return ret
}
