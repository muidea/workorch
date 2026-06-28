package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func PrimaryTaskProgressResult(items []readmodelcore.Result) *readmodelcore.Result {
	var selected *readmodelcore.Result
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

func ResultProgressState(snapshot readmodelcore.RunSnapshot) string {
	results := RunScopedResults(snapshot)
	artifacts := RunScopedArtifacts(snapshot)
	return resultProgressStateFromOutputs(snapshot, results, artifacts)
}

func HasAcceptedRunResult(snapshot readmodelcore.RunSnapshot) bool {
	for _, item := range RunScopedResults(snapshot) {
		if item.State == string(common.ResultStateAccepted) && isTerminalOutcomeEligibleResultForSnapshot(item, snapshot) {
			return true
		}
	}
	return false
}
