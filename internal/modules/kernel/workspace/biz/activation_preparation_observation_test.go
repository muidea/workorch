package workspace

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationObservationWholeApplicationUsesActualResourceOwners(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-actual-observation")
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
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL, cfg.AIProxy.APIKey = "http://provider.invalid", "non-sensitive-fixture"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "fixture-chat"}
	llm, err := llmbiz.New(owners.hub, nil, llmbiz.Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return cfg, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Teardown(ctx)
	b := &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
	request := management.PreparationCheckRequest{RequestID: "real-owners", ExpectedWorkspaceRevision: w.Revision}
	operation, err := b.CheckStartupPreparation(ctx, w.ID, request, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range operation.Preparation.Record.Attempts[0].Checks {
		if check.State != expectedPreparationCheckState(check.ID, check.Reason) {
			t.Fatal("incorrect per-check judgment", check)
		}
		if check.ID == "provider_cache" && check.Reason != "provider_cache_miss" {
			t.Fatal("real LLM owner not consumed")
		}
		if check.ID == "first_activation_no_run" && check.Reason != "run_history_empty_final_fence_observed" {
			t.Fatal("real Run owner not consumed")
		}
	}
	for _, recovery := range []bool{false, true} {
		again, err := b.CheckStartupPreparation(ctx, w.ID, request, recovery)
		if err != nil || !reflect.DeepEqual(operation, again) {
			t.Fatal("real owner original changed", err)
		}
	}
	inventory, err := owners.repositories.Runs.List(ctx)
	if err != nil || len(inventory) != 0 {
		t.Fatal("preparation created a Run", err)
	}
}

// Capability fixtures model declared cache/history facts, never readiness.
// Workspace/Application/EventHub/file persistence are the production path.
func installPreparationReadFixtures(t *testing.T, owners *fileDefinitionAdmissionOwners, wID common.WorkspaceID, scenario string, calls *atomic.Int32, beforeQuery func()) {
	t.Helper()
	llm := magicevent.NewSimpleObserver(llmevents.CommandDestination, owners.hub)
	if err := llm.Subscribe(llmevents.TopicObserveProviderCatalogCache, func(ev magicevent.Event, result magicevent.Result) {
		calls.Add(1)
		c, ok := ev.Data().(llmevents.ObserveProviderCatalogCacheCommand)
		stored, err := owners.repositories.Workspaces.Get(ev.Context(), wID)
		if !ok || c.WorkspaceID != wID || err != nil || stored.StartupPreparation == nil || stored.StartupPreparation.Attempts[0].State != "accepted" {
			t.Error("query ran before durable acceptance or outside original scope")
		}
		if beforeQuery != nil {
			beforeQuery()
		}
		now := time.Now().UTC()
		r := llmevents.ObserveProviderCatalogCacheResult{WorkspaceID: wID, OwnerEpoch: "llm-preparation-epoch:ABCDEFGHIJKLMNOPQRSTUVWXYZ", ObservedAt: now,
			Catalog: llmmodel.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", State: llmmodel.ProviderCatalogStateUnavailable, ErrorCode: "cache_miss"}}
		if scenario == "fresh" || scenario == "expired" {
			r.Catalog.State, r.Catalog.ErrorCode, r.Catalog.Revision = llmmodel.ProviderCatalogStateFresh, "", "cached-revision"
			r.Catalog.FetchedAt, r.Catalog.ExpiresAt = now.Add(-time.Minute), now.Add(time.Minute)
			if scenario == "expired" {
				r.Catalog.State, r.Catalog.Stale, r.Catalog.ErrorCode, r.Catalog.ExpiresAt = llmmodel.ProviderCatalogStateStale, true, "cache_expired", now.Add(-time.Second)
			}
		}
		if scenario == "wrong_scope" {
			r.WorkspaceID = "other"
		}
		if scenario == "wrong_type" {
			result.Set(llmevents.GetProviderModelCatalogResult{}, nil)
			return
		}
		result.Set(r, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = llm.Unsubscribe(llmevents.TopicObserveProviderCatalogCache) })
	run := magicevent.NewSimpleObserver(runevents.CommandDestination, owners.hub)
	if err := run.Subscribe(runevents.TopicObserveWorkspacePreparationHistory, func(ev magicevent.Event, result magicevent.Result) {
		calls.Add(1)
		c, ok := ev.Data().(runevents.ObserveWorkspacePreparationHistoryCommand)
		if !ok || c.WorkspaceID != wID || c.RequestID != "original-observation" {
			t.Error("history not tied to original observation")
		}
		r := runevents.ObserveWorkspacePreparationHistoryResult{RequestID: c.RequestID, ObservedAt: time.Now().UTC(), History: runmodel.WorkspacePreparationHistory{WorkspaceID: wID}}
		if scenario == "historical" {
			r.History.GenerationHead = 2
		}
		if scenario == "retired" {
			r.History.Retirement = &runmodel.WorkspaceRetirementFence{WorkspaceID: wID, DeletionID: "retirement", DeletionGeneration: 1, DataDispositionIntent: "retain", WorkspaceIntentHash: "intent", Phase: "draining", FenceRevision: 1}
		}
		if scenario == "wrong_scope" {
			r.History.WorkspaceID = "other"
		}
		if scenario == "wrong_type" {
			result.Set("not history", nil)
			return
		}
		result.Set(r, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.Unsubscribe(runevents.TopicObserveWorkspacePreparationHistory) })
}

func TestPreparationObservationProductionNegativeClassificationAndReadOnlyReplay(t *testing.T) {
	for _, scenario := range []string{"miss", "fresh", "expired", "historical", "retired", "wrong_scope", "wrong_type"} {
		t.Run(scenario, func(t *testing.T) {
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			w := seedPreparationEligible(t, owners, "ws-read-observation")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			installPreparationReadFixtures(t, owners, w.ID, scenario, &calls, nil)
			b := &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
			operation, err := b.CheckStartupPreparation(context.Background(), w.ID, request, false)
			if err != nil || operation.Preparation.OperationState != "blocked" || calls.Load() != 2 {
				t.Fatalf("actual production continuation: %v %+v calls=%d", err, operation, calls.Load())
			}
			providerReason, runReason := "provider_cache_miss", "run_history_empty_final_fence_observed"
			switch scenario {
			case "fresh":
				providerReason = "provider_declaration_only"
			case "expired":
				providerReason = "provider_cache_expired"
			case "historical":
				runReason = "run_history_present"
			case "retired":
				runReason = "run_history_retired"
			case "wrong_scope", "wrong_type":
				providerReason, runReason = "provider_cache_unavailable", "run_history_unavailable"
			}
			for _, check := range operation.Preparation.Record.Attempts[0].Checks {
				want := "check_pipeline_incomplete"
				wantState := "blocked"
				if check.ID == "exact_seal" {
					wantState, want = "passed", "seal_observed_remaining_checks"
				}
				if check.ID == "persistence_integrity" {
					wantState, want = "passed", "persistence_binding_observed_remaining_checks"
				}
				if check.ID == "mcp_selection" {
					want = "mcp_registry_observation_unavailable_not_saved"
				}
				if check.ID == "memory_selection" {
					want = "memory_inventory_unavailable_not_saved"
				}
				if check.ID == "provider_cache" {
					want = providerReason
				}
				if check.ID == "first_activation_no_run" {
					want = runReason
					if runReason == "run_history_empty_final_fence_observed" {
						wantState = "passed"
					}
				}
				if check.ID == "directory_boundary" {
					// This fixture has no File receiver and relative directories.
					want = "directory_observation_unavailable"
				}
				if check.ID == "managed_configuration" {
					want = "managed_config_observation_unavailable"
				}
				if check.ID == "soul_selection" {
					want = "soul_observation_unavailable_not_saved"
				}
				if check.ID == "skill_selection" {
					want = "skill_registry_observation_unavailable_not_saved"
				}
				if check.State != wantState || check.Reason != want {
					t.Fatalf("false credit or inaccurate reason: %+v want %s", check, want)
				}
			}
			for _, recoverOriginal := range []bool{false, true} {
				again, err := b.CheckStartupPreparation(context.Background(), w.ID, request, recoverOriginal)
				if err != nil || !reflect.DeepEqual(operation, again) {
					t.Fatal("duplicate/recovery changed terminal original", err)
				}
			}
			if _, err := b.ReadStartupPreparationView(context.Background(), w.ID); err != nil || calls.Load() != 2 {
				t.Fatal("GET/recovery/duplicate replayed resource reads", err)
			}
		})
	}
}

func TestPreparationObservationDurableCutsAndDriftNeverRequeryOnRecovery(t *testing.T) {
	for _, scenario := range []string{"accept_reply_lost", "settle_write_failed", "settle_reply_lost", "drift", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			root := t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer func() { owners.close() }()
			w := seedPreparationEligible(t, owners, "ws-observation-cut")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			cut := &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 1, persistFirst: true}
			if scenario == "settle_write_failed" || scenario == "settle_reply_lost" {
				cut.failAt, cut.persistFirst = 2, scenario == "settle_reply_lost"
			}
			if scenario == "drift" || scenario == "cancelled" {
				cut.failAt = 0
			}
			owners.workspace.workspaces = cut
			var calls atomic.Int32
			installPreparationReadFixtures(t, owners, w.ID, "miss", &calls, func() {
				if scenario == "cancelled" {
					cancel()
				}
				if scenario == "drift" {
					current, err := owners.repositories.Workspaces.Get(ctx, w.ID)
					if err != nil {
						t.Error(err)
						return
					}
					current.Title = "changed while observing"
					if err := owners.repositories.Workspaces.CompareAndSwap(ctx, current, current.Revision); err != nil {
						t.Error(err)
					}
				}
			})
			b := &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
			operation, err := b.CheckStartupPreparation(ctx, w.ID, request, false)
			if scenario == "drift" {
				if err != nil || operation.Preparation.CurrentValidity != "stale" {
					t.Fatal("drift not stale", err)
				}
				for _, check := range operation.Preparation.Record.Attempts[0].Checks {
					if check.Reason != "binding_changed" {
						t.Fatal("late observation kept current classification")
					}
				}
			} else if err == nil {
				t.Fatal("ambiguous write reported success")
			}
			wantCalls := int32(2)
			if scenario == "accept_reply_lost" {
				wantCalls = 0
			}
			if scenario == "cancelled" {
				wantCalls = 1
				ctx = context.Background()
			}
			if calls.Load() != wantCalls {
				t.Fatal("queries ran without unambiguous acceptance")
			}
			// Persisted original is read-only until explicit recovery; no new key.
			if _, err := b.CheckStartupPreparation(ctx, w.ID, request, false); err != nil || calls.Load() != wantCalls {
				t.Fatal("duplicate continued interrupted observations", err)
			}
			owners.close()
			owners = openFileDefinitionAdmissionOwners(t, root)
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			b = &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
			recovered, err := b.CheckStartupPreparation(ctx, w.ID, request, true)
			if err != nil || recovered.Preparation.OperationState != "blocked" || calls.Load() != wantCalls {
				t.Fatal("reopen recovery lost original or replayed reads", err)
			}
			if scenario == "accept_reply_lost" || scenario == "settle_write_failed" || scenario == "cancelled" {
				for _, check := range recovered.Preparation.Record.Attempts[0].Checks {
					if check.Reason != "original_check_interrupted" {
						t.Fatal("interrupted observation manufactured proof")
					}
				}
			}
		})
	}
}

func TestPreparationObservationContinuationOnlyInitialWinnerAndLateSettlementNoop(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-observation-owner")
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original", ExpectedWorkspaceRevision: w.Revision}
	first, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true)
	if err != nil || !first.ObservationRequired || first.ValidateFor(c) != nil {
		t.Fatal("initial winner missing continuation", err)
	}
	again, err := owners.workspace.beginWorkspacePreparation(ctx, c, false, true)
	if err != nil || again.ObservationRequired {
		t.Fatal("duplicate obtained continuation", err)
	}
	recovered, err := owners.workspace.beginWorkspacePreparation(ctx, c, true, true)
	if err != nil || recovered.ObservationRequired || recovered.State.State != "blocked" {
		t.Fatal("recovery acquired continuation", err)
	}
	late, err := owners.workspace.finishBlockedPreparation(ctx, workspacePreparationBlocked{Command: c, Reason: "check_pipeline_incomplete", ProviderCacheReason: "provider_declaration_only", RunHistoryReason: "run_history_empty_fence_not_final", At: time.Now().UTC()})
	if err != nil || !reflect.DeepEqual(recovered, late) {
		t.Fatal("late result rewrote original recovery", err)
	}
	owners.workspace.handlePreparationBlockedSettlement(nil, nil)
}
