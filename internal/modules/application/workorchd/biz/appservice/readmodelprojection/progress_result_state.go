package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func PrimaryTaskProgressResult(items []projectioncontract.Result) *projectioncontract.Result {
	var selected *projectioncontract.Result
	for idx := range items {
		if items[idx].State == string(common.ResultStateAccepted) && isFinalOutcomeResult(items[idx]) {
			if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
				selected = &items[idx]
			}
		}
	}
	if selected != nil {
		return selected
	}
	for idx := range items {
		if isIntermediateAcceptedEvidence(items[idx]) {
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

func ResultProgressState(snapshot readmodelcore.RunSnapshotState) string {
	results := RunScopedResults(snapshot)
	artifacts := RunScopedArtifacts(snapshot)
	return resultProgressStateFromOutputs(snapshot, results, artifacts)
}

func HasAcceptedRunResult(snapshot readmodelcore.RunSnapshotState) bool {
	for _, item := range RunScopedResults(snapshot) {
		if item.State == string(common.ResultStateAccepted) && isTerminalOutcomeEligibleResultForSnapshot(item, snapshot) {
			return true
		}
	}
	return false
}
