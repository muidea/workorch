package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func TaskProgressItems(snapshot readmodelcore.RunSnapshot) []readmodelcore.ProgressItem {
	results := RunScopedResults(snapshot)
	artifacts := RunScopedArtifacts(snapshot)
	ret := make([]readmodelcore.ProgressItem, 0, len(snapshot.Steps)+len(snapshot.ToolCalls)+len(snapshot.ValidationResults)+len(results)+len(artifacts)+len(snapshot.Decisions)+len(snapshot.Approvals))
	recoveredHistoricalFailures := AcceptedTerminalRun(snapshot)
	for _, item := range snapshot.Steps {
		ret = append(ret, stepProgressItem(item, recoveredHistoricalFailures))
	}
	for _, item := range snapshot.ToolCalls {
		ret = append(ret, toolCallProgressItem(item, recoveredHistoricalFailures))
	}
	for _, item := range snapshot.Patches {
		ret = append(ret, patchProgressItem(item))
	}
	for _, item := range snapshot.ValidationResults {
		ret = append(ret, validationProgressItem(item, recoveredHistoricalFailures))
	}
	for _, item := range results {
		ret = append(ret, resultProgressItem(item))
	}
	for _, item := range artifacts {
		ret = append(ret, artifactProgressItem(item))
	}
	for _, item := range snapshot.Decisions {
		ret = append(ret, decisionProgressItem(item))
	}
	for _, item := range snapshot.Approvals {
		ret = append(ret, approvalProgressItem(item))
	}
	for idx := range ret {
		ret[idx].EvidenceRefs = compactUniqueStrings(ret[idx].EvidenceRefs)
	}
	return ret
}
