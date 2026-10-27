package workspace

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationProviderExpiryProjectionPreservesOriginalAndNeverQueries(t *testing.T) {
	for _, scenario := range []string{"fresh", "expired", "miss"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, root := context.Background(), t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			w := seedPreparationEligible(t, owners, "ws-provider-expiry")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			installPreparationReadFixtures(t, owners, w.ID, scenario, &calls, nil)
			app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
			if _, err := app.CheckStartupPreparation(ctx, w.ID, request, false); err != nil {
				t.Fatal(err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || len(stored.PreparationProviderCacheEvidence) != 1 {
				t.Fatal("original archive", err)
			}
			original := stored.StartupPreparation.Clone()
			archive := workspace.ClonePreparationProviderCacheEvidence(stored.PreparationProviderCacheEvidence)
			expires := archive[0].Facts.Observation.Catalog.ExpiresAt
			points := []time.Time{time.Now().UTC()}
			if scenario == "fresh" {
				points = []time.Time{expires.Add(-time.Nanosecond), expires, expires.Add(time.Nanosecond)}
			}
			for _, now := range points {
				state, err := preparationStateFromPersistence(stored, now)
				want := "not_evaluated"
				if !expires.IsZero() && !now.Before(expires) {
					want = "stale"
				}
				if err != nil || state.CurrentValidity != want || state.State != "blocked" || !reflect.DeepEqual(*state.Record, original) || !reflect.DeepEqual(stored.PreparationProviderCacheEvidence, archive) || calls.Load() != 2 {
					t.Fatalf("expiry changed history, repeated owner query, or wrong boundary (%s): %+v %v", now, state, err)
				}
			}
			if scenario == "fresh" {
				for _, change := range []string{"missing_archive", "missing_ref", "duplicate", "scope", "request", "payload", "binding", "hash"} {
					t.Run(change, func(t *testing.T) {
						bad := *stored
						record := original.Clone()
						bad.StartupPreparation = &record
						bad.PreparationProviderCacheEvidence = workspace.ClonePreparationProviderCacheEvidence(archive)
						e := &bad.PreparationProviderCacheEvidence[0]
						switch change {
						case "missing_archive":
							bad.PreparationProviderCacheEvidence = nil
						case "missing_ref":
							bad.StartupPreparation.Attempts[0].ProviderCacheEvidenceRef = ""
						case "duplicate":
							bad.PreparationProviderCacheEvidence = append(bad.PreparationProviderCacheEvidence, e.Clone())
						case "scope":
							e.Facts.Observation.WorkspaceID = "other"
						case "request":
							e.RequestID = "other"
						case "payload":
							e.PayloadHash = "other"
						case "binding":
							e.Binding.StartupInputHash = "other"
						case "hash":
							e.ContentHash = "other"
						}
						if change != "hash" {
							e.ContentHash = e.Hash()
						}
						if _, err := preparationStateFromPersistence(&bad, expires); err == nil {
							t.Fatal("invalid original material became a currentness projection")
						}
					})
				}
			}
			owners.close()
			reopened := openFileDefinitionAdmissionOwners(t, root)
			defer reopened.close()
			// No LLM/Run observers exist on the reopened Hub. Both the real query
			// and replay/recovery must read the old operation without observing again.
			view, err := reopened.workspace.queryWorkspacePreparationState(ctx, workspaceevents.QueryWorkspacePreparationStateCommand{WorkspaceID: w.ID})
			want := "not_evaluated"
			if scenario == "expired" {
				want = "stale"
			}
			if err != nil || view.CurrentValidity != want || !reflect.DeepEqual(*view.Record, original) {
				t.Fatal("reopen/query changed operation or refreshed cache", err)
			}
			if err := reopened.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			reopenedApp := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, reopened.hub, nil)}
			for _, recoverOriginal := range []bool{false, true} {
				op, err := reopenedApp.CheckStartupPreparation(ctx, w.ID, request, recoverOriginal)
				if err != nil || op.Preparation.CurrentValidity != want || op.Preparation.OperationState != "blocked" || calls.Load() != 2 {
					t.Fatal("replay/recovery changed history or re-observed", err)
				}
			}
			if scenario == "fresh" {
				current, _ := reopened.repositories.Workspaces.Get(ctx, w.ID)
				// A new accepted operation has no provider observation. Expired
				// older history must not be attributed to that operation or settle it.
				next := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "next-observation", ExpectedWorkspaceRevision: current.Revision}
				if _, err := reopened.workspace.beginWorkspacePreparation(ctx, next, false, true); err != nil {
					t.Fatal(err)
				}
				current, err = reopened.repositories.Workspaces.Get(ctx, w.ID)
				if err != nil {
					t.Fatal(err)
				}
				state, err := preparationStateFromPersistence(current, expires.Add(time.Second))
				if err != nil || state.State != "accepted" || state.CurrentValidity != "not_evaluated" || len(current.PreparationProviderCacheEvidence) != 1 || calls.Load() != 2 {
					t.Fatal("expired predecessor invalidated or settled unobserved successor", err)
				}
			}
			runs, err := reopened.repositories.Runs.List(ctx)
			if err != nil || len(runs) != 0 {
				t.Fatal("expiry/query/replay created Run", err)
			}
		})
	}
}
