package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func PrimaryTaskProgressResult(items []readmodelcore.Result) *readmodelcore.Result {
	for idx := range items {
		if items[idx].State == string(common.ResultStateAccepted) {
			return &items[idx]
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &items[len(items)-1]
}

func ResultProgressState(snapshot readmodelcore.RunSnapshot) string {
	results := RunScopedResults(snapshot)
	artifacts := RunScopedArtifacts(snapshot)
	return resultProgressStateFromOutputs(snapshot, results, artifacts)
}

func HasAcceptedRunResult(snapshot readmodelcore.RunSnapshot) bool {
	for _, item := range RunScopedResults(snapshot) {
		if item.State == string(common.ResultStateAccepted) {
			return true
		}
	}
	return false
}
