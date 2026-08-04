package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func resultProgressStateFromOutputs(snapshot readmodelcore.RunSnapshot, results []readmodelcore.Result, artifacts []projectioncontract.Artifact) string {
	if len(results) == 0 && len(artifacts) == 0 {
		if IsTerminalRunState(snapshot.Run.State) {
			return "failed"
		}
		return "pending"
	}
	for _, item := range results {
		if item.State == string(common.ResultStateAccepted) && isTerminalOutcomeEligibleResultForSnapshot(item, snapshot) {
			return "completed"
		}
	}
	if IsTerminalRunState(snapshot.Run.State) {
		for _, item := range results {
			if item.State == string(common.ResultStateRejected) && isTerminalOutcomeEligibleResultForSnapshot(item, snapshot) {
				return "failed"
			}
		}
		if snapshot.Run.State == string(common.RunStateCompleted) {
			if snapshotHasActiveCoverageWorkItems(snapshot) {
				return "failed"
			}
			return "completed"
		}
		return "failed"
	}
	return "running"
}
