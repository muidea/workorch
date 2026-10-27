package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	filebiz "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	payload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func preparationManagedHints(merge bool) payload.Payload {
	return payload.New(payload.Field{Name: "managedWorkspaceConfigs", Value: payload.Value{Kind: payload.ValueArray, Array: []payload.Value{{Kind: payload.ValueObject, Object: []payload.Field{payload.StringField("path", "mcp/config.json"), payload.BoolField("mergeMissing", merge), payload.ObjectField("content", payload.New(payload.StringField("token", "fixture-config-value"), payload.ObjectField("nested", payload.New(payload.IntField("required", 1)))))}}}}})
}

func seedManagedPreparation(t *testing.T, owners *fileDefinitionAdmissionOwners, internal, scenario string) *workspace.Workspace {
	t.Helper()
	return seedPreparationEligibleWithConfiguration(t, owners, "ws-managed", t.TempDir(), func(w *workspace.Workspace) {
		w.InternalDirectory = internal
		w.ContextHints = w.ContextHints.Merge(preparationManagedHints(scenario == "merge_complete" || scenario == "merge_missing"))
		if scenario == "empty" {
			w.ContextHints = w.ContextHints.Merge(payload.New(payload.Field{Name: "managedWorkspaceConfigs", Value: payload.Value{Kind: payload.ValueArray}}))
		}
		if scenario == "invalid" {
			w.ContextHints = w.ContextHints.Merge(payload.New(payload.StringField("managedWorkspaceConfigs", "malformed-original")))
		}
	}, func(template *workspace.WorkspaceTemplate) {
		if scenario != "empty" {
			template.ContextHints = template.ContextHints.Merge(preparationManagedHints(false))
		}
	})
}

func startManagedPreparationOwners(t *testing.T, owners *fileDefinitionAdmissionOwners) {
	t.Helper()
	if err := owners.workspace.Start(); err != nil {
		t.Fatal(err)
	}
	f, err := filebiz.New(owners.hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Teardown(context.Background()) })
}

func writeManagedFixture(t *testing.T, root string, raw []byte) string {
	t.Helper()
	path := filepath.Join(root, "mcp", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPreparationManagedConfigsProductionOriginalEvidenceReopenNoReplay(t *testing.T) {
	for _, scenario := range []string{"exact", "preserve_override", "merge_complete", "merge_missing", "system_conflict", "invalid_json", "missing", "oversized", "symlink", "empty", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, storage := context.Background(), t.TempDir()
			internal := filepath.Join(t.TempDir(), "internal", "workspaces", "ws")
			owners := openFileDefinitionAdmissionOwners(t, storage)
			defer owners.close()
			w := seedManagedPreparation(t, owners, internal, scenario)
			startManagedPreparationOwners(t, owners)
			original := []byte("{\r\n\"nested\":{\"required\":1.0},\"token\":\"fixture-config-value\"}\n")
			systemRaw, workspaceRaw := bytes.Clone(original), bytes.Clone(original)
			want := "managed_configs_observed_remaining_checks"
			switch scenario {
			case "preserve_override":
				workspaceRaw = []byte(`{"custom":"preserved"}`)
			case "merge_complete":
				workspaceRaw = []byte(`{"token":null,"nested":{"required":"override"}}`)
			case "merge_missing":
				workspaceRaw = []byte(`{"token":null,"nested":{}}`)
				want = "managed_config_content_conflict"
			case "system_conflict":
				systemRaw = []byte(`{"nested":{"required":1},"token":"different"}`)
				want = "managed_config_content_conflict"
			case "invalid_json":
				workspaceRaw = []byte{0xff, 0, '\r', '\n'}
				want = "managed_config_content_invalid"
			case "missing", "oversized", "symlink":
				want = "managed_config_observation_incomplete"
			case "invalid":
				want = "managed_config_declaration_invalid"
			}
			systemPath := writeManagedFixture(t, workorchmanagedconfig.SystemDirectory(internal), systemRaw)
			workspacePath := writeManagedFixture(t, internal, workspaceRaw)
			if scenario == "missing" || scenario == "symlink" {
				if err := os.Remove(systemPath); err != nil {
					t.Fatal(err)
				}
				if scenario == "symlink" {
					if err := os.Symlink(workspacePath, systemPath); err != nil {
						t.Fatal(err)
					}
				}
			}
			if scenario == "oversized" {
				if err := os.WriteFile(systemPath, bytes.Repeat([]byte("x"), fileobservation.ResourceMaxBytes+1), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var calls atomic.Int32
			installPreparationReadFixtures(t, owners, w.ID, "miss", &calls, nil)
			b := &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
			operation, err := b.CheckStartupPreparation(ctx, w.ID, request, false)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := owners.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
				t.Fatal("invalid archive", err)
			}
			a := operation.Preparation.Record.Attempts[0]
			for _, c := range a.Checks {
				if c.State != expectedPreparationCheckState(c.ID, c.Reason) {
					t.Fatal("incorrect per-check judgment", c)
				}
				if c.ID == "managed_configuration" && c.Reason != want {
					t.Fatal("wrong managed classification", c.Reason, want)
				}
			}
			if scenario == "invalid" {
				if len(stored.PreparationManagedConfigEvidence) != 0 || a.ManagedConfigEvidenceRef != "" {
					t.Fatal("invalid declaration invented File evidence")
				}
			} else {
				if len(stored.PreparationManagedConfigEvidence) != 1 {
					t.Fatal("missing archive")
				}
				e := stored.PreparationManagedConfigEvidence[0]
				if a.ManagedConfigEvidenceRef != e.Ref || !workspace.ValidPreparationManagedConfigRef(e.Ref) || e.ConfigurationHash != a.Binding.AuthoringConfigurationHash || e.RequestID != request.RequestID || e.ObservedWorkspaceRevision != w.Revision+1 || e.BlockedReason() != want {
					t.Fatal("original config binding lost")
				}
				if scenario == "empty" {
					if len(e.Files) != 0 {
						t.Fatal("empty declaration acquired undeclared files")
					}
				} else {
					if len(e.Files) != 2 || e.Files[0].Scope != "system" || e.Files[1].Scope != "workspace" {
						t.Fatal("source roots mixed")
					}
					if !bytes.Equal(e.Files[1].Observation.Content, workspaceRaw) {
						t.Fatal("raw bytes normalized")
					}
					if scenario != "missing" && scenario != "symlink" && scenario != "oversized" && !bytes.Equal(e.Files[0].Observation.Content, systemRaw) {
						t.Fatal("system bytes normalized")
					}
				}
				public, _ := json.Marshal(operation)
				ordinary, _ := json.Marshal(stored)
				for _, secret := range []string{"fixture-config-value", "ContentSHA256", e.ContentHash, "mcp/config.json", internal, "PreparationManagedConfigEvidence"} {
					if containsJSONText(public, secret) {
						t.Fatal("private config leaked to API")
					}
				}
				if containsJSONText(ordinary, "PreparationManagedConfigEvidence") {
					t.Fatal("private config leaked to ordinary Workspace")
				}
			}
			// Replays, GET and reopened owners retain original evidence even after
			// the files disappear. Only a new explicit request can observe again.
			if err := os.Remove(workspacePath); err != nil {
				t.Fatal(err)
			}
			for _, recovery := range []bool{false, true} {
				again, err := b.CheckStartupPreparation(ctx, w.ID, request, recovery)
				if err != nil || !reflect.DeepEqual(operation, again) {
					t.Fatal("replay replaced history", err)
				}
			}
			if _, err := b.ReadStartupPreparationView(ctx, w.ID); err != nil {
				t.Fatal(err)
			}
			reopened := openFileDefinitionAdmissionOwners(t, storage)
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(stored.PreparationManagedConfigEvidence, read.PreparationManagedConfigEvidence) || calls.Load() != 2 {
				t.Fatal("reopen/replay repeated observation", err)
			}
			runs, _ := owners.repositories.Runs.List(ctx)
			if len(runs) != 0 {
				t.Fatal("preparation created Run")
			}
		})
	}
}

func TestPreparationManagedConfigsAtomicCutsAndBindingDrift(t *testing.T) {
	for _, scenario := range []string{"write_failed", "reply_lost", "revision_drift"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			owners := openFileDefinitionAdmissionOwners(t, t.TempDir())
			defer owners.close()
			internal := filepath.Join(t.TempDir(), "ws")
			w := seedManagedPreparation(t, owners, internal, "exact")
			startManagedPreparationOwners(t, owners)
			for _, root := range []string{internal, workorchmanagedconfig.SystemDirectory(internal)} {
				writeManagedFixture(t, root, []byte(`{"token":"fixture-config-value","nested":{"required":1}}`))
			}
			var calls atomic.Int32
			installPreparationReadFixtures(t, owners, w.ID, "miss", &calls, nil)
			if scenario == "revision_drift" {
				owners.workspace.workspaces = &preparationDirectoryDriftRepository{WorkspaceRepository: owners.workspace.workspaces}
			} else {
				owners.workspace.workspaces = &preparationWriteCutRepository{WorkspaceRepository: owners.workspace.workspaces, failAt: 2, persistFirst: scenario == "reply_lost"}
			}
			b := &applicationbiz.Biz{Base: basebiz.New("application/workorchd", owners.hub, nil)}
			request := management.PreparationCheckRequest{RequestID: "original-observation", ExpectedWorkspaceRevision: w.Revision}
			_, err := b.CheckStartupPreparation(ctx, w.ID, request, false)
			if scenario == "revision_drift" && err != nil || scenario != "revision_drift" && err == nil {
				t.Fatal("unexpected CAS result", err)
			}
			stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			a := stored.StartupPreparation.Attempts[0]
			if scenario == "write_failed" {
				if len(stored.PreparationManagedConfigEvidence) != 0 || a.ManagedConfigEvidenceRef != "" || a.State != "accepted" {
					t.Fatal("partial material commit")
				}
			} else {
				if len(stored.PreparationManagedConfigEvidence) != 1 || a.ManagedConfigEvidenceRef == "" || a.State != "blocked" {
					t.Fatal("material/ref not atomic")
				}
			}
			if scenario == "revision_drift" {
				for _, c := range a.Checks {
					if c.Reason != "binding_changed" {
						t.Fatal("stale fact retained as current")
					}
				}
			}
			if _, err := b.CheckStartupPreparation(ctx, w.ID, request, true); err != nil {
				t.Fatal(err)
			}
			again, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			if !reflect.DeepEqual(stored.PreparationManagedConfigEvidence, again.PreparationManagedConfigEvidence) || calls.Load() != 2 {
				t.Fatal("recovery reobserved file")
			}
		})
	}
}
