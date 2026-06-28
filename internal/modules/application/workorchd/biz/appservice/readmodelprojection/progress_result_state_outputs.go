package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func resultProgressStateFromOutputs(snapshot readmodelcore.RunSnapshot, results []readmodelcore.Result, artifacts []readmodelcore.Artifact) string {
	if len(results) == 0 && len(artifacts) == 0 {
		if IsTerminalRunState(snapshot.Run.State) {
			return "failed"
		}
		return "pending"
	}
	for _, item := range results {
		if item.State == string(common.ResultStateAccepted) && isTerminalOutcomeEligibleResult(item, snapshot.Run) {
			return "completed"
		}
	}
	if IsTerminalRunState(snapshot.Run.State) {
		for _, item := range results {
			if item.State == string(common.ResultStateRejected) && isTerminalOutcomeEligibleResult(item, snapshot.Run) {
				return "failed"
			}
		}
		if snapshot.Run.State == string(common.RunStateCompleted) {
			return "completed"
		}
		return "failed"
	}
	return "running"
}
