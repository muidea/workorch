// Command seed-runtime-run writes a durable Run with Runtime state into a
// file-backed workorch data directory. Used by verify-tui-runtime-refresh-history-live
// (FR04) so runtime-view projections expose non-zero RuntimeStateRevision.
//
// usage: go run ./scripts/seed-runtime-run.go <dataRoot> <workspaceID> <runID> <stateRevision> <focusID> <convergenceState>
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/testkit"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func main() {
	if len(os.Args) != 7 {
		fmt.Fprintln(os.Stderr, "usage: seed-runtime-run <dataRoot> <workspaceID> <runID> <stateRevision> <focusID> <convergenceState>")
		os.Exit(2)
	}
	dataRoot, workspaceID, runID := os.Args[1], os.Args[2], os.Args[3]
	rev, err := strconv.ParseUint(os.Args[4], 10, 64)
	if err != nil {
		panic(err)
	}
	focusID, state := os.Args[5], runtimecontract.ConvergenceState(os.Args[6])
	ctx := context.Background()
	repos := testkit.OpenRepositories("file", dataRoot)
	wsID := common.WorkspaceID(workspaceID)
	rID := common.RunID(runID)
	runCtx := common.ContextWithRuntimeWorkspaceID(ctx, wsID)

	ws, err := repos.Workspaces.Get(runCtx, wsID)
	if err != nil {
		panic(err)
	}
	ws.ActiveRunID = rID
	ws.State = common.WorkspaceStateActive
	if err := repos.Workspaces.Update(runCtx, ws); err != nil {
		panic(err)
	}

	runtimeState := &runtimecontract.RuntimeStateRecord{
		StateRevision:    rev,
		EvidenceSequence: rev,
		Facts: runtimecontract.RuntimeFacts{
			WorkspaceID:        wsID,
			RunID:              rID,
			DefinitionRevision: "def-live-1",
			EvidenceRevision:   fmt.Sprintf("ev-%d", rev),
			Focus: runtimecontract.CoverageFocus{
				ID:                    focusID,
				CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-live-1"},
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			},
			Evidence: runtimecontract.EvidenceLedger{
				Accepted: []runtimecontract.EvidenceFact{{
					ID:                  "acc-live-1",
					Kind:                runtimecontract.EvidenceKindStateChange,
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-live-1"},
				}},
			},
		},
		Decision: runtimecontract.RuntimeDecision{
			State: state,
			AuthoritySnapshot: runtimecontract.AuthoritySnapshot{
				ID:          "authority-live-1",
				WorkspaceID: wsID,
				RunID:       rID,
			},
		},
		CompletedDispatches: []runtimecontract.RuntimeDispatchRecord{
			{ID: fmt.Sprintf("dispatch-%d", rev)},
		},
		CandidateAttempts: []runtimecontract.RuntimeCandidateAttemptRecord{
			{Attempt: 0, State: runtimecontract.RuntimeCandidateAttemptStateAccepted},
		},
	}

	existing, getErr := repos.Runs.Get(runCtx, rID)
	if getErr != nil {
		run := &runmodel.Run{
			ID:           rID,
			WorkspaceID:  wsID,
			State:        common.RunStateActive,
			RuntimeState: runtimeState,
		}
		if err := repos.Runs.Create(runCtx, run); err != nil {
			panic(err)
		}
	} else {
		existing.State = common.RunStateActive
		existing.RuntimeState = runtimeState
		if err := repos.Runs.Update(runCtx, existing); err != nil {
			panic(err)
		}
	}
	fmt.Printf("seeded run %s workspace %s revision %d focus %s state %s\n", runID, workspaceID, rev, focusID, state)
}
