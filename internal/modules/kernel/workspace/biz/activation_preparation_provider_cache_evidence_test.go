package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationProviderCacheArchiveWithActualLLMOwnerDoesNotFetch(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-actual-provider-archive")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer provider.Close()
	cfg := workorchconfig.DefaultLLMConfig()
	cfg.AIProxy.BaseURL, cfg.AIProxy.APIKey = provider.URL, "non-sensitive-fixture"
	cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "fixture"}
	llm, err := llmbiz.New(owners.hub, nil, llmbiz.Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return cfg, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if err := llm.Start(); err != nil {
		t.Fatal(err)
	}
	defer llm.Teardown(ctx)
	app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
	request := management.PreparationCheckRequest{RequestID: "actual-original", ExpectedWorkspaceRevision: w.Revision}
	first, err := app.CheckStartupPreparation(ctx, w.ID, request, false)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || len(stored.PreparationProviderCacheEvidence) != 1 || stored.PreparationProviderCacheEvidence[0].Facts.BlockedReason() != "provider_cache_miss" || requests.Load() != 0 {
		t.Fatal("actual owner observation missing or refreshed provider", err)
	}
	compiled := stored.PreparationProviderCacheEvidence[0].Selection
	if compiled == nil || compiled.Source != "llm_owner_default" || compiled.Selection.Target.ModelID != "fixture" || compiled.FailureCode != "cache_miss" || compiled.OwnerEpoch != stored.PreparationProviderCacheEvidence[0].Facts.Observation.OwnerEpoch {
		t.Fatal("real frozen owner default/cache facts were not compiled exactly")
	}
	if err := llm.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	for _, recover := range []bool{false, true} {
		again, err := app.CheckStartupPreparation(ctx, w.ID, request, recover)
		if err != nil || !reflect.DeepEqual(first, again) || requests.Load() != 0 {
			t.Fatal("recovery queried stopped owner or fetched provider", err)
		}
	}
	runs, err := owners.repositories.Runs.List(ctx)
	if err != nil || len(runs) != 0 {
		t.Fatal("preparation created Run", err)
	}
}

func TestPreparationProviderSelectionWarmActualOwnerPreservesSourceAndZeroInference(t *testing.T) {
	for _, workspaceTarget := range []bool{false, true} {
		t.Run(fmt.Sprint(workspaceTarget), func(t *testing.T) {
			ctx, root := context.Background(), t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			w := seedPreparationEligibleWithConfiguration(t, owners, "ws-warm-provider-selection", t.TempDir(), func(w *workspace.Workspace) {
				if workspaceTarget {
					w.DefaultLLMSelection = &runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "fixture"}}
				}
			})
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			var catalogCalls, otherCalls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
					otherCalls.Add(1)
					w.WriteHeader(http.StatusForbidden)
					return
				}
				catalogCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"fixture","supported_endpoints":["/v1/responses"],"contextWindowTokens":32000,"maxOutputTokens":4000}]}`))
			}))
			defer provider.Close()
			cfg := workorchconfig.DefaultLLMConfig()
			cfg.AIProxy.BaseURL, cfg.AIProxy.APIKey = provider.URL, "non-sensitive-warm-fixture"
			cfg.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "fixture"}
			if workspaceTarget {
				cfg.DefaultChat.ModelID = "not-selected-owner-default"
			}
			service := llmbiz.NewCatalogService()
			// Explicit fixture preparation, outside the checked operation. The real
			// owner owns the cache afterward; the preparation path must not refresh it.
			if _, err := service.GetCatalog(ctx, cfg, true); err != nil || catalogCalls.Load() != 1 {
				t.Fatal("fixture catalog setup", err)
			}
			owner, err := llmbiz.New(owners.hub, nil, llmbiz.Dependencies{CatalogService: service, LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return cfg, nil }})
			if err != nil || owner.Start() != nil {
				t.Fatal("LLM fixture owner failed", err)
			}
			defer owner.Teardown(ctx)
			app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "warm-original", ExpectedWorkspaceRevision: w.Revision}
			first, err := app.CheckStartupPreparation(ctx, w.ID, request, false)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || len(stored.PreparationProviderCacheEvidence) != 1 {
				t.Fatal("archive", err)
			}
			e := stored.PreparationProviderCacheEvidence[0]
			wantSource := "llm_owner_default"
			if workspaceTarget {
				wantSource = "workspace_default"
			}
			if e.Selection == nil || e.Selection.FailureCode != "" || e.Selection.Source != wantSource || e.Selection.Selection.Target.ModelID != "fixture" || e.Selection.Descriptor == nil || e.Selection.Descriptor.CapabilityRevision != e.Facts.Observation.Catalog.Models[0].CapabilityRevision || first.Preparation.OperationState != "blocked" {
				t.Fatal("exact warm selection lost or promoted to readiness")
			}
			if e.Selection.Budget == nil || !reflect.DeepEqual(e.Selection.Budget, e.Facts.Observation.Budget) || e.Selection.Budget.Target != e.Selection.Selection.Target || e.BlockedReason() != "provider_selection_budget_observed_authorization_pending" {
				t.Fatal("exact owner capacity not consumed or borrowed from default")
			}
			for _, check := range first.Preparation.Record.Attempts[0].Checks {
				if check.ID == "provider_cache" && (check.State != "blocked" || check.Reason != e.BlockedReason()) {
					t.Fatal("owner capacity conferred readiness", check)
				}
			}
			bad, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil {
				t.Fatal(err)
			}
			bad.PreparationProviderCacheEvidence[0].Facts.Observation.Budget.MaxInputTokens++
			bad.PreparationProviderCacheEvidence[0].Selection.Budget.MaxInputTokens++
			bad.PreparationProviderCacheEvidence[0].ContentHash = bad.PreparationProviderCacheEvidence[0].Hash()
			if owners.repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision) == nil {
				t.Fatal("completed budget history rewritten after rehash")
			}
			if err := owner.Teardown(ctx); err != nil {
				t.Fatal(err)
			}
			for _, recovery := range []bool{false, true} {
				again, err := app.CheckStartupPreparation(ctx, w.ID, request, recovery)
				if err != nil || !reflect.DeepEqual(first, again) {
					t.Fatal("replay changed original", err)
				}
			}
			owners.close()
			reopened := openFileDefinitionAdmissionOwners(t, root)
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(stored.PreparationProviderCacheEvidence, read.PreparationProviderCacheEvidence) {
				t.Fatal("owner reopen changed source", err)
			}
			runs, err := reopened.repositories.Runs.List(ctx)
			if err != nil || len(runs) != 0 || catalogCalls.Load() != 1 || otherCalls.Load() != 0 {
				t.Fatal("check/recovery performed HTTP, inference or created Run", err)
			}
		})
	}
}

func TestPreparationProviderCacheArchiveOriginalSettlementCutsAndReopen(t *testing.T) {
	for _, scenario := range []string{"miss", "fresh", "expired", "wrong_scope", "wrong_type", "write_failed", "reply_lost", "revision_drift"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, root := context.Background(), t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			w := seedPreparationEligible(t, owners, "ws-provider-archive")
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
				t.Fatal("persistence", err)
			}
			complete := scenario != "write_failed" && scenario != "wrong_scope" && scenario != "wrong_type"
			if complete {
				if scenario == "revision_drift" {
					if op.Preparation.CurrentValidity != "stale" {
						t.Fatal("revision drift did not invalidate currentness")
					}
					for _, check := range stored.StartupPreparation.Attempts[0].Checks {
						if check.Reason != "binding_changed" {
							t.Fatal("historical cache value granted current credit")
						}
					}
				}
				if len(stored.PreparationProviderCacheEvidence) != 1 || stored.StartupPreparation.Attempts[0].ProviderCacheEvidenceRef != stored.PreparationProviderCacheEvidence[0].Ref {
					t.Fatal("complete original cache value not saved")
				}
				e := stored.PreparationProviderCacheEvidence[0]
				if scenario == "revision_drift" {
					if e.Selection != nil {
						t.Fatal("stale settlement created selection compilation")
					}
				} else if e.Selection != nil {
					t.Fatal("old fixture without configuration was upgraded to a selection compilation")
				}
				if e.Facts.WorkspaceRevision != w.Revision+1 || e.RequestID != request.RequestID || e.Facts.Observation.OwnerEpoch == "" {
					t.Fatal("lost original scope/epoch")
				}
				public, _ := json.Marshal(op)
				ordinary, _ := json.Marshal(stored)
				for _, private := range []string{"PreparationProviderCacheEvidence", e.ContentHash, e.Facts.Observation.OwnerEpoch, "FetchedAt"} {
					if bytes.Contains(public, []byte(private)) || bytes.Contains(ordinary, []byte(private)) {
						t.Fatal("private owner catalog leaked", private)
					}
				}
				for _, change := range []string{"erase", "ref", "rehash", "binding"} {
					bad, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
					switch change {
					case "erase":
						bad.PreparationProviderCacheEvidence = nil
						bad.StartupPreparation.Attempts[0].ProviderCacheEvidenceRef = ""
					case "ref":
						bad.StartupPreparation.Attempts[0].ProviderCacheEvidenceRef = ""
					case "rehash":
						bad.PreparationProviderCacheEvidence[0].Facts.Observation.OwnerEpoch = "llm-preparation-epoch:BBBBBBBBBBBBBBBBBBBBBBBBBB"
					case "binding":
						bad.PreparationProviderCacheEvidence[0].Binding.StartupInputHash = "replacement"
					}
					if len(bad.PreparationProviderCacheEvidence) != 0 {
						bad.PreparationProviderCacheEvidence[0].ContentHash = bad.PreparationProviderCacheEvidence[0].Hash()
					}
					if err := owners.repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
						t.Fatal("completed cache history rewritten", change)
					}
				}
			} else if len(stored.PreparationProviderCacheEvidence) != 0 || stored.StartupPreparation.Attempts[0].ProviderCacheEvidenceRef != "" {
				t.Fatal("missing or stale binding fabricated archive")
			}
			for _, recovery := range []bool{false, true, false} {
				if _, err := app.CheckStartupPreparation(ctx, w.ID, request, recovery); err != nil {
					t.Fatal(err)
				}
			}
			owners.close()
			reopened := openFileDefinitionAdmissionOwners(t, root)
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(stored.PreparationProviderCacheEvidence, read.PreparationProviderCacheEvidence) || calls.Load() != 2 || read.ActiveRunID != "" {
				t.Fatal("replay/recovery/reopen changed original material or repeated queries", err)
			}
		})
	}
}

func TestPreparationProviderCacheBindingRejectsWrongOriginalScopeAndTime(t *testing.T) {
	ctx := context.Background()
	owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
	defer owners.close()
	w := seedPreparationEligible(t, owners, "ws-provider-bind")
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	installPreparationReadFixtures(t, owners, w.ID, "miss", &calls, nil)
	original := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: w.ID, RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
	if _, err := owners.workspace.beginWorkspacePreparation(ctx, original, false, true); err != nil {
		t.Fatal(err)
	}
	app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
	observed, err := app.ObservePreparationProviderCatalog(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	facts := workspace.PreparationProviderCacheFacts{Version: workspace.PreparationProviderCacheEvidenceVersion, WorkspaceRevision: w.Revision + 1, Observation: observed}
	c := workspaceevents.SettleWorkspacePreparationBlockedCommand{Original: original, ProviderCacheReason: "provider_cache_miss", RunHistoryReason: "run_history_unavailable", ProviderCacheFacts: &facts}
	if c.Validate() != nil || owners.workspace.bindOriginalPreparationProviderCache(ctx, c) == nil {
		t.Fatal("valid original binding rejected")
	}
	for _, change := range []func(*workspaceevents.SettleWorkspacePreparationBlockedCommand){
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) { c.Original.RequestID = "other" },
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.ProviderCacheFacts.Observation.WorkspaceID = "other"
		},
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.ProviderCacheFacts.WorkspaceRevision++
		},
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.ProviderCacheFacts.Observation.ObservedAt = time.Now().Add(-time.Hour)
		},
		func(c *workspaceevents.SettleWorkspacePreparationBlockedCommand) {
			c.ProviderCacheFacts.Observation.ObservedAt = time.Now().Add(time.Hour)
		},
	} {
		bad, f := c, facts.Clone()
		bad.ProviderCacheFacts = &f
		change(&bad)
		if owners.workspace.bindOriginalPreparationProviderCache(ctx, bad) != nil {
			t.Fatal("wrong request/scope/revision/time accepted")
		}
	}
}
