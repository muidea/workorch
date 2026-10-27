package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationMemoryInventoryArchiveOriginalCutsAndReopen(t *testing.T) {
	for _, scenario := range []string{"valid", "empty", "wrong_scope", "wrong_type", "past", "future", "write_failed", "reply_lost", "revision_drift", "mode/list", "mode/semantic", "mode/semantic_empty", "mode/empty", "mode/unavailable", "mode/wrong_type", "mode/after_unavailable", "mode/drift", "mode/target_drift", "mode/write_failed", "mode/reply_lost", "mode/revision_drift"} {
		t.Run(scenario, func(t *testing.T) {
			mode := strings.HasPrefix(scenario, "mode/")
			kind := strings.TrimPrefix(scenario, "mode/")
			ctx, root := context.Background(), t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			w := seedPreparationEligible(t, owners, "ws-memory-archive")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			var modeCalls atomic.Int32
			if mode {
				config := magicevent.NewSimpleObserver(llmevents.CommandDestination, owners.hub)
				if err := config.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, r magicevent.Result) {
					n := modeCalls.Add(1)
					if kind == "unavailable" || kind == "after_unavailable" && n == 2 {
						r.Set(nil, cd.NewError(cd.InvalidOperation, "isolated active configuration unavailable"))
						return
					}
					if kind == "wrong_type" {
						r.Set(struct{}{}, nil)
						return
					}
					v := llmevents.GetActiveConfigurationResult{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ConfigurationRevision: "original-mode", DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ModelID: "chat"}}
					if kind == "semantic" || kind == "semantic_empty" {
						v.DefaultEmbedding = &runtimecontract.ProviderModelTarget{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ModelID: "embedding"}
					}
					if kind == "drift" && n == 2 {
						v.ConfigurationRevision = "changed-mode"
					}
					if kind == "target_drift" && n == 2 {
						v.DefaultEmbedding = &runtimecontract.ProviderModelTarget{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ModelID: "embedding"}
					}
					r.Set(v, nil)
				}); err != nil {
					t.Fatal(err)
				}
			}
			o := magicevent.NewSimpleObserver(memoryevents.CommandDestination, owners.hub)
			if err := o.Subscribe(memoryevents.TopicObservePreparationInventory, func(ev magicevent.Event, r magicevent.Result) {
				calls.Add(1)
				c, ok := ev.Data().(memoryevents.ObservePreparationInventoryCommand)
				if !ok {
					t.Error("wrong command")
					return
				}
				v := memorymodel.PreparationInventory{Version: memorymodel.PreparationInventoryVersion, WorkspaceID: c.WorkspaceID, OwnerEpoch: "memory-preparation-epoch:" + strings.Repeat("A", 26), ObservedAt: time.Now().UTC(), Items: []*memorymodel.MemoryItem{{ID: "original", WorkspaceID: c.WorkspaceID, SourceType: "user", MemoryType: "project_context", Summary: "原摘要", Content: "原库存正文\r\n", Tags: []string{"original"}, Vector: []float64{0.5}}}}
				switch kind {
				case "empty", "semantic_empty":
					v.Items = []*memorymodel.MemoryItem{}
				case "wrong_scope":
					v.WorkspaceID = "other"
				case "past":
					v.ObservedAt = v.ObservedAt.Add(-time.Hour)
				case "future":
					v.ObservedAt = v.ObservedAt.Add(time.Hour)
				case "wrong_type":
					if mode {
						break
					}
					r.Set(struct{}{}, nil)
					return
				}
				r.Set(memoryevents.ObservePreparationInventoryResult{Inventory: v}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			if kind == "write_failed" || kind == "reply_lost" {
				owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 2, persistFirst: kind == "reply_lost"}
			}
			if kind == "revision_drift" {
				owners.workspace.workspaces = &preparationDirectoryDriftRepository{WorkspaceRepository: owners.workspace.workspaces}
			}
			app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "original-memory", ExpectedWorkspaceRevision: w.Revision}
			first, err := app.CheckStartupPreparation(ctx, w.ID, request, false)
			if (err != nil) != (kind == "write_failed" || kind == "reply_lost") {
				t.Fatal("settlement", err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
				t.Fatal("invalid persistence", err)
			}
			complete := scenario == "valid" || scenario == "empty" || scenario == "reply_lost" || scenario == "revision_drift" || mode && kind != "write_failed"
			if complete {
				if len(stored.PreparationMemoryInventoryEvidence) != 1 || stored.StartupPreparation.Attempts[0].MemoryInventoryEvidenceRef != stored.PreparationMemoryInventoryEvidence[0].Ref {
					t.Fatal("missing original history")
				}
				if kind == "revision_drift" && first.Preparation.CurrentValidity != "stale" {
					t.Fatal("drift credited")
				}
				pub, _ := json.Marshal(first)
				ordinary, _ := json.Marshal(stored)
				for _, private := range []string{"原库存正文", "memory-preparation-epoch:", "PreparationMemoryInventoryEvidence"} {
					if bytes.Contains(pub, []byte(private)) || bytes.Contains(ordinary, []byte(private)) {
						t.Fatal("private material leaked")
					}
				}
				for _, mutation := range []string{"erase", "rehash", "rebind"} {
					bad, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
					switch mutation {
					case "erase":
						bad.PreparationMemoryInventoryEvidence = nil
						bad.StartupPreparation.Attempts[0].MemoryInventoryEvidenceRef = ""
					case "rehash":
						bad.PreparationMemoryInventoryEvidence[0].Facts.Observation.OwnerEpoch = "memory-preparation-epoch:" + strings.Repeat("B", 26)
					case "rebind":
						bad.PreparationMemoryInventoryEvidence[0].Binding.StartupInputHash = "other"
					}
					if len(bad.PreparationMemoryInventoryEvidence) > 0 {
						bad.PreparationMemoryInventoryEvidence[0].ContentHash = bad.PreparationMemoryInventoryEvidence[0].Hash()
					}
					if owners.repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision) == nil {
						t.Fatal("history rewrite allowed", mutation)
					}
				}
			} else if len(stored.PreparationMemoryInventoryEvidence) != 0 {
				t.Fatal("invalid/unsaved facts fabricated")
			}
			if mode && complete {
				e := stored.PreparationMemoryInventoryEvidence[0]
				sourceComplete := kind != "unavailable" && kind != "wrong_type" && kind != "after_unavailable" && kind != "drift" && kind != "target_drift"
				if (e.Facts.SelectionSource != nil) != sourceComplete || (e.Selection != nil) != sourceComplete {
					t.Fatal("configuration failure fabricated list-only source or lost complete source")
				}
				for _, c := range stored.StartupPreparation.Attempts[0].Checks {
					if c.ID != "memory_selection" {
						continue
					}
					wantState, wantReason := workspace.WorkspacePreparationCheckState("blocked"), "memory_inventory_observed_selection_pending"
					if sourceComplete {
						wantState, wantReason = e.Selection.CheckResult()
					}
					if kind == "revision_drift" {
						wantState, wantReason = "blocked", "binding_changed"
					}
					if c.State != wantState || c.Reason != wantReason {
						t.Fatalf("mode judgment: %+v, want %s/%s", c, wantState, wantReason)
					}
				}
				if stored.StartupPreparation.Attempts[0].State != workspace.WorkspacePreparationBlocked {
					t.Fatal("single Memory result granted readiness")
				}
			}
			reads := modeCalls.Load()
			if mode {
				want := int32(2)
				if kind == "unavailable" || kind == "wrong_type" {
					want = 1
				}
				if reads != want {
					t.Fatalf("mode owner reads %d, want %d", reads, want)
				}
			}
			for _, recover := range []bool{false, true, false} {
				if _, err := app.CheckStartupPreparation(ctx, w.ID, request, recover); err != nil {
					t.Fatal(err)
				}
			}
			owners.close()
			reopened := openFileDefinitionAdmissionOwners(t, root)
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(stored.PreparationMemoryInventoryEvidence, read.PreparationMemoryInventoryEvidence) || calls.Load() != 1 || modeCalls.Load() != reads {
				t.Fatal("recovery/reopen changed history or repeated read", err)
			}
			runs, err := reopened.repositories.Runs.List(ctx)
			if err != nil || len(runs) != 0 {
				t.Fatal("preparation created Run", err)
			}
		})
	}
}
