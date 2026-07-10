package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func compressedContextDependencies(deps readModelDependencies) runports.CompressedContextDependencies {
	return runports.CompressedContextDependencies{
		Runs:          deps.runs,
		Tickets:       deps.tickets,
		Resolutions:   deps.resolutions,
		Approvals:     deps.approvals,
		MemoryRecalls: deps.memoryRecalls,
		CommandHub:    deps.commandHub,
		EventHub:      deps.eventHub,
		RecoverySnapshot: func(snapshotCtx context.Context, snapshotRunID common.RunID) (RunSnapshot, error) {
			return readRunRecoverySnapshot(snapshotCtx, deps, snapshotRunID)
		},
	}
}
