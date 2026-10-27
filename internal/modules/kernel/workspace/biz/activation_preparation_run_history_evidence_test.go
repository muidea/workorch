package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sync/atomic"
	"testing"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationRunHistoryArchiveActualOwnerAndStoppedOwnerReplay(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-real-history-archive")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	runs := runbiz.New()
	if err := runs.Configure(runbiz.Dependencies{OwnerRepositories: owners.repositories.RunOwner, EventHub: owners.hub}); err != nil {
		t.Fatal(err)
	}
	if err := runs.Start(); err != nil {
		t.Fatal(err)
	}
	defer runs.Stop(ctx)
	app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
	request := management.PreparationCheckRequest{RequestID: "original-real-history", ExpectedWorkspaceRevision: w.Revision}
	op, err := app.CheckStartupPreparation(ctx, w.ID, request, false)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || len(stored.PreparationRunHistoryEvidence) != 1 {
		t.Fatal("actual Run owner material missing", err)
	}
	e := stored.PreparationRunHistoryEvidence[0]
	if e.Facts.BlockedReason() != "run_history_empty_fence_not_final" || e.Facts.Observation.History.GenerationHead != 0 || op.Preparation.Record.Attempts[0].RunHistoryEvidenceRef != e.Ref || op.Preparation.OperationState != "blocked" {
		t.Fatal("empty ordered history was not preserved or granted credit")
	}
	a := stored.StartupPreparation.Attempts[0]
	if e.FinalFence == nil || e.FinalFence.WorkspaceRevision != w.Revision+1 || !reflect.DeepEqual(e.FinalFence.Fence, *w.PreparationFence) || a.NoRunEvaluationVersion != workspace.PreparationNoRunFinalVersion || op.Preparation.Record.Attempts[0].NoRunEvaluationVersion != a.NoRunEvaluationVersion || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
		t.Fatal("actual history and final CAS preimage were not jointly preserved")
	}
	for _, c := range a.Checks {
		if c.ID == "first_activation_no_run" && (c.State != "passed" || c.Reason != workspace.PreparationNoRunFinalReason) {
			t.Fatal("original no-Run final fence was not evaluated", c)
		}
	}
	public, _ := json.Marshal(op)
	ordinary, _ := json.Marshal(stored)
	if bytes.Contains(public, []byte(`"FinalFence":`)) || bytes.Contains(ordinary, []byte(`"FinalFence":`)) || bytes.Contains(public, []byte(e.ContentHash)) {
		t.Fatal("private CAS preimage leaked")
	}
	if err := runs.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	for _, recoverOriginal := range []bool{false, true} {
		again, err := app.CheckStartupPreparation(ctx, w.ID, request, recoverOriginal)
		if err != nil || !reflect.DeepEqual(op, again) {
			t.Fatal("replay queried stopped Run owner or changed original", err)
		}
	}
	inventory, err := owners.repositories.Runs.List(ctx)
	if err != nil || len(inventory) != 0 {
		t.Fatal("preparation created Run", err)
	}
}

func TestPreparationRunHistoryArchiveSettlementCutsAndReopen(t *testing.T) {
	for _, scenario := range []string{"miss", "historical", "retired", "wrong_scope", "wrong_type", "write_failed", "reply_lost", "revision_drift"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, root := context.Background(), t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			w := seedPreparationEligible(t, owners, "ws-history-archive")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			installPreparationReadFixtures(t, owners, w.ID, scenario, &calls, nil)
			if scenario == "write_failed" || scenario == "reply_lost" {
				owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 2, persistFirst: scenario == "reply_lost"}
			}
			if scenario == "revision_drift" {
				owners.workspace.workspaces = &preparationDirectoryDriftRepository{WorkspaceRepository: owners.workspace.workspaces}
			}
			app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
			op, err := app.CheckStartupPreparation(ctx, w.ID, request, false)
			if (err != nil) != (scenario == "write_failed" || scenario == "reply_lost") {
				t.Fatal("settlement", err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
				t.Fatal("invalid persisted material", err)
			}
			complete := scenario != "write_failed" && scenario != "wrong_scope" && scenario != "wrong_type"
			if complete {
				if len(stored.PreparationRunHistoryEvidence) != 1 || stored.StartupPreparation.Attempts[0].RunHistoryEvidenceRef != stored.PreparationRunHistoryEvidence[0].Ref {
					t.Fatal("complete original history not saved")
				}
				e := stored.PreparationRunHistoryEvidence[0]
				if e.RequestID != request.RequestID || e.CreationEpoch != stored.StartupPreparation.Attempts[0].CreationEpoch || e.PreparationEpoch != stored.StartupPreparation.Attempts[0].PreparationEpoch {
					t.Fatal("original identity lost")
				}
				if scenario == "historical" && e.Facts.Observation.History.GenerationHead != 2 || scenario == "retired" && e.Facts.Observation.History.Retirement == nil {
					t.Fatal("historical watermark or retirement discarded")
				}
				if scenario == "revision_drift" && op.Preparation.CurrentValidity != "stale" {
					t.Fatal("drift granted currentness")
				}
				public, _ := json.Marshal(op)
				ordinary, _ := json.Marshal(stored)
				for _, private := range []string{"PreparationRunHistoryEvidence", "GenerationHead", "WorkspaceIntentHash", e.ContentHash} {
					if bytes.Contains(public, []byte(private)) || bytes.Contains(ordinary, []byte(private)) {
						t.Fatal("private Run history leaked", private)
					}
				}
			} else if len(stored.PreparationRunHistoryEvidence) != 0 || stored.StartupPreparation.Attempts[0].RunHistoryEvidenceRef != "" {
				t.Fatal("unavailable history fabricated")
			}
			for _, recoverOriginal := range []bool{false, true, false} {
				if _, err := app.CheckStartupPreparation(ctx, w.ID, request, recoverOriginal); err != nil {
					t.Fatal(err)
				}
			}
			owners.close()
			reopened := openFileDefinitionAdmissionOwners(t, root)
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(stored.PreparationRunHistoryEvidence, read.PreparationRunHistoryEvidence) || calls.Load() != 2 {
				t.Fatal("replay/recovery/reopen repeated query or changed history", err)
			}
		})
	}
}
