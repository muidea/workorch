package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	mcpowner "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/biz"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func TestPreparationMCPRegistryOriginalCutsAndReopen(t *testing.T) {
	for _, scenario := range []string{"complete", "empty", "wrong_type", "drift", "oversize", "write_failed", "reply_lost", "revision_drift"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, root := context.Background(), t.TempDir()
			owners := openFileDefinitionAdmissionOwners(t, root)
			defer owners.close()
			w := seedPreparationEligible(t, owners, "ws-mcp-registry")
			if err := owners.workspace.Start(); err != nil {
				t.Fatal(err)
			}
			var reads atomic.Int32
			registry := memoryrepo.NewMCPRegistryRepository()
			mcp, err := mcpowner.New(owners.hub, nil, mcpowner.Dependencies{Registry: registry, EventHub: owners.hub})
			if err != nil {
				t.Fatal(err)
			}
			if err := mcp.Start(); err != nil {
				t.Fatal(err)
			}
			defer mcp.Teardown(ctx)
			if scenario != "empty" {
				ev := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandEnsureDefaults, "kernel/system", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.EnsureDefaultsCommand{})
				if r := owners.hub.Send(ev); r == nil || r.Error() != nil {
					t.Fatal("fixture defaults")
				}
			}
			value := readPreparationMCPFixture(t, registry)
			drifted := value.Clone()
			if scenario == "drift" {
				snapshot, err := registry.Load(ctx)
				if err != nil {
					t.Fatal(err)
				}
				previous := snapshot.Revision
				snapshot.Revision++
				snapshot.Servers[0].Name += "changed"
				if err := registry.Commit(ctx, previous, snapshot); err != nil {
					t.Fatal(err)
				}
				drifted = readPreparationMCPFixture(t, registry)
			}
			// Count and inject concrete reply faults only; the registry itself is
			// the real owner and all original values come from its complete query.
			if err := mcp.Teardown(ctx); err != nil {
				t.Fatal(err)
			}
			proxy := magicevent.NewSimpleObserver(mcpevents.CommandDestination, owners.hub)
			if err := proxy.Subscribe(mcpevents.TopicMCPCommandReadRegistrySnapshot, func(ev magicevent.Event, r magicevent.Result) {
				n := reads.Add(1)
				if scenario == "wrong_type" {
					r.Set(struct{}{}, nil)
					return
				}
				// Both values were read from a real owner before installing this
				// reply-fault proxy; never synthesize a content revision.
				value := value.Clone()
				if scenario == "drift" && n == 2 {
					value = drifted.Clone()
				}
				if scenario == "oversize" {
					value.Servers[0].Name = strings.Repeat("x", mcpevents.PreparationSnapshotMaxBytes+1)
				}
				r.Set(value, nil)
			}); err != nil {
				t.Fatal(err)
			}
			if scenario == "write_failed" || scenario == "reply_lost" {
				owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 2, persistFirst: scenario == "reply_lost"}
			}
			if scenario == "revision_drift" {
				owners.workspace.workspaces = &preparationDirectoryDriftRepository{WorkspaceRepository: owners.workspace.workspaces}
			}
			app := &applicationbiz.Biz{Base: basebiz.New(applicationbiz.ModuleID, owners.hub, nil)}
			intent := management.PreparationCheckRequest{RequestID: "original-mcp", ExpectedWorkspaceRevision: w.Revision}
			first, err := app.CheckStartupPreparation(ctx, w.ID, intent, false)
			if (err != nil) != (scenario == "write_failed" || scenario == "reply_lost") {
				t.Fatal("settlement", err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
				t.Fatal("persistence", err)
			}
			complete := scenario == "complete" || scenario == "empty" || scenario == "reply_lost" || scenario == "revision_drift"
			if complete {
				if len(stored.PreparationMCPRegistryEvidence) != 1 || stored.StartupPreparation.Attempts[0].MCPRegistryEvidenceRef != stored.PreparationMCPRegistryEvidence[0].Ref {
					t.Fatal("missing complete original history")
				}
				if scenario == "revision_drift" && first.Preparation.CurrentValidity != "stale" {
					t.Fatal("drift credited")
				}
				selection := stored.PreparationMCPRegistryEvidence[0].Selection
				if scenario == "revision_drift" {
					if selection != nil {
						t.Fatal("stale settlement compiled current selection")
					}
				} else if selection == nil || selection.FailureCode != "skill_source_incomplete" {
					t.Fatal("missing Skill source silently became empty selection")
				}
				for _, c := range stored.StartupPreparation.Attempts[0].Checks {
					want := "mcp_selection_skill_source_incomplete"
					if scenario == "revision_drift" {
						want = "binding_changed"
					}
					if c.ID == "mcp_selection" && (c.State != "blocked" || c.Reason != want) {
						t.Fatal("registry became connection proof", c)
					}
				}
				pub, _ := json.Marshal(first)
				ordinary, _ := json.Marshal(stored)
				if bytes.Contains(pub, []byte("EnvironmentRefs")) || bytes.Contains(ordinary, []byte("PreparationMCPRegistryEvidence")) {
					t.Fatal("private registry leaked")
				}
				for _, mutation := range []string{"erase", "rebind", "snapshot", "reason"} {
					bad, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
					switch mutation {
					case "erase":
						bad.PreparationMCPRegistryEvidence = nil
						bad.StartupPreparation.Attempts[0].MCPRegistryEvidenceRef = ""
					case "rebind":
						bad.PreparationMCPRegistryEvidence[0].Binding.StartupInputHash = "other"
					case "snapshot":
						bad.PreparationMCPRegistryEvidence[0].Facts.SnapshotHash = "other"
						bad.PreparationMCPRegistryEvidence[0].Facts.ReobservedSnapshotHash = "other"
					case "reason":
						bad.StartupPreparation.Attempts[0].Checks[0].Reason = "ready"
					}
					if len(bad.PreparationMCPRegistryEvidence) > 0 {
						bad.PreparationMCPRegistryEvidence[0].ContentHash = bad.PreparationMCPRegistryEvidence[0].Hash()
					}
					if owners.repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision) == nil {
						t.Fatal("history rewrite", mutation)
					}
				}
			} else if len(stored.PreparationMCPRegistryEvidence) != 0 {
				t.Fatal("invalid or unsaved history invented")
			}
			count := reads.Load()
			for _, recover := range []bool{false, true, false} {
				if _, err := app.CheckStartupPreparation(ctx, w.ID, intent, recover); err != nil {
					t.Fatal(err)
				}
			}
			owners.close()
			reopened := openFileDefinitionAdmissionOwners(t, root)
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(stored.PreparationMCPRegistryEvidence, read.PreparationMCPRegistryEvidence) || reads.Load() != count {
				t.Fatal("reopen/recovery reread registry", err)
			}
			runs, err := reopened.repositories.Runs.List(ctx)
			if err != nil || len(runs) != 0 {
				t.Fatal("preparation created Run", err)
			}
		})
	}
}

func readPreparationMCPFixture(t *testing.T, registry repository.MCPRegistryRepository) mcpevents.RegistrySnapshotResult {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	owner, err := mcpowner.New(hub, nil, mcpowner.Dependencies{Registry: registry, EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)
	ev := magicevent.NewEventWithContext(mcpevents.TopicMCPCommandReadRegistrySnapshot, "application/workorchd", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.ReadRegistrySnapshotCommand{})
	r := hub.Send(ev)
	if r == nil || r.Error() != nil {
		t.Fatal("real owner read", r)
	}
	v, getErr := r.Get()
	if getErr != nil {
		t.Fatal(getErr)
	}
	return v.(mcpevents.RegistrySnapshotResult)
}
