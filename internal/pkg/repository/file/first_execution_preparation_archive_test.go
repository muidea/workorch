package file

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The production verifier explicitly runs this against its complete isolated
// archive. Neither an archive nor a retrospective clock authorizes execution:
// this test only exercises the pure compiler, and never writes an owner file.
func TestFirstExecutionPreparationArchivedSources(t *testing.T) {
	root := os.Getenv("WORKORCH_PREPARATION_VERIFIER_ROOT")
	if root == "" {
		t.Skip("requires isolated production verifier sources")
	}
	for _, name := range []string{"original-owner-record.bin", "final-owner-record.bin"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, name)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := loadWorkspacePersistenceMap(path)
			if err != nil || len(rows) != 1 {
				t.Fatal("expected complete single Workspace archive", err)
			}
			var w *workspace.Workspace
			for _, row := range rows {
				w = &row
			}
			internal := filepath.Join(root, "data/internal/workspaces", string(w.ID))
			// The scoped Workspace repository joins real Soul sidecar values;
			// lower metadata intentionally omits them. Read the actual sidecar,
			// never copy these declarations from historical seal evidence.
			var bindings struct {
				DefaultSoulID common.SoulID            `json:"defaultSoulId,omitempty"`
				Bindings      map[string]common.SoulID `json:"bindings,omitempty"`
			}
			sidecar, err := os.ReadFile(filepath.Join(internal, "config/soul_bindings.json"))
			if err != nil || decodeCurrentJSON(sidecar, &bindings) != nil {
				t.Fatal("actual Soul sidecar unavailable", err)
			}
			w.DefaultSoulID, w.SoulBindings = bindings.DefaultSoulID, bindings.Bindings
			goals, err := loadMap[common.GoalID, workspace.Goal](filepath.Join(internal, "goals.json"))
			if err != nil || len(goals) != 1 {
				t.Fatal("expected actual Goal row", err)
			}
			dods, err := loadMap[common.DoDID, workspace.DoD](filepath.Join(internal, "dods.json"))
			if err != nil || len(dods) != 1 {
				t.Fatal("expected actual DoD row", err)
			}
			var goal workspace.Goal
			var dod workspace.DoD
			for _, row := range goals {
				goal = row
			}
			for _, row := range dods {
				dod = row
			}
			a := w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1]
			original, err := preparationcompiler.CompileOriginalPreparedBootstrap(w, a)
			if err != nil || original.Compilation == nil || original.FailureCode != "" {
				t.Fatal("complete bootstrap absent", err)
			}
			bootstrap := original.Compilation.Bootstrap
			now, epoch := a.CompletedAt, workspace.PreparationFileObservationEpoch(w)
			before := cloneWorkspace(w)
			compiled, err := preparationcompiler.CompileFirstExecutionPreparation(w, &goal, &dod, bootstrap, now, epoch)
			if name == "final-owner-record.bin" {
				// The verifier subsequently performed a distinct initialization,
				// advancing revision. An older success cannot authorize that image.
				if err == nil {
					t.Fatal("historical completion accepted for changed current revision")
				}
				return
			}
			if err != nil || compiled.Version != preparationcompiler.FirstExecutionPreparationVersion || compiled.RequestID != a.RequestID || compiled.ExpectedWorkspaceRevision != w.Revision || !reflect.DeepEqual(compiled.Compilation, *original.Compilation) {
				t.Fatal("complete original preimage rejected", err)
			}
			var filesFound bool
			for _, d := range w.PreparationDirectoryEvidence {
				if d.ContentHash != a.DirectoryEvidenceHash || d.Revalidation == nil {
					continue
				}
				filesFound = true
				files := d.Revalidation.Clone()
				files.StartedAt, files.ObservedAt, files.ExpiresAt = now, now, now.Add(30*time.Second)
				if _, err := preparationcompiler.CompileObservedFirstExecutionPreparation(w, &goal, &dod, bootstrap, now, files); err != nil {
					t.Fatal("complete current file observation rejected", err)
				}
				for _, arm := range []string{"epoch", "scope", "directory", "resource", "config", "age", "future"} {
					t.Run("file_"+arm, func(t *testing.T) {
						bad, at := files.Clone(), now
						switch arm {
						case "epoch":
							bad.OwnerEpoch = "file-preparation-epoch:AAAAAAAAAAAAAAAAAAAAAAAAAA"
						case "scope":
							bad.Scope.RequestID += "other"
						case "directory":
							bad.Internal.Path += "other"
						case "resource":
							bad.Resources = nil
						case "config":
							bad.Configs = nil
						case "age":
							at = bad.ExpiresAt
						case "future":
							at = now.Add(-time.Nanosecond)
						}
						if _, err := preparationcompiler.CompileObservedFirstExecutionPreparation(w, &goal, &dod, bootstrap, at, bad); err == nil {
							t.Fatal("changed/current-unproven File source accepted")
						}
					})
				}
			}
			if !filesFound {
				t.Fatal("original File observation missing")
			}
			consumed := cloneWorkspace(w)
			hash, err := runmodel.ExecutionIntentPayloadHash(w.ID, "first-execution", bootstrap)
			if err != nil {
				t.Fatal(err)
			}
			consumed.PreparationFence.Pending = &workspace.WorkspaceFreshIntentReservation{RequestID: "first-execution", Kind: runtimecontract.RuntimeExecutionIntentRestart, StartupInputHash: w.StartupInput.StartupInputHash, PayloadHash: strings.Repeat("b", 64), RunPayloadHash: hash, PreparationCompletionID: compiled.CompletionID, PreparationRevision: w.Revision, PreparationConsumedAt: now}
			consumed.PreparationFence.PreparationEpoch++
			files := w.PreparationDirectoryEvidence[len(w.PreparationDirectoryEvidence)-1].Revalidation.Clone()
			files.StartedAt, files.ObservedAt, files.ExpiresAt = now, now, now.Add(30*time.Second)
			consumed.FirstExecutionPreparationEvidence = &workspace.WorkspaceFirstExecutionPreparationEvidence{Version: workspace.FirstExecutionPreparationEvidenceVersion, WorkspaceID: w.ID, RequestID: "first-execution", Files: files}
			if err := preparationcompiler.ValidatePreparedPersistence(consumed); err != nil {
				t.Fatal("consumed original does not reload", err)
			}
			for _, arm := range []string{"completion", "revision", "time", "payload", "orphan", "missing_files", "file_scope", "file_bytes"} {
				t.Run("consumption_"+arm, func(t *testing.T) {
					bad := cloneWorkspace(consumed)
					switch arm {
					case "completion":
						bad.PreparationFence.Pending.PreparationCompletionID += "changed"
					case "revision":
						bad.PreparationFence.Pending.PreparationRevision++
					case "time":
						bad.PreparationFence.Pending.PreparationConsumedAt = now.Add(30 * time.Second)
					case "payload":
						bad.PreparationFence.Pending.RunPayloadHash = strings.Repeat("a", 64)
					case "orphan":
						bad.StartupPreparation = nil
					case "missing_files":
						bad.FirstExecutionPreparationEvidence = nil
					case "file_scope":
						bad.FirstExecutionPreparationEvidence.Files.Scope.RequestID += "other"
					case "file_bytes":
						bad.FirstExecutionPreparationEvidence.Files.Configs[0].ContentSHA256 = strings.Repeat("a", 64)
					}
					if preparationcompiler.ValidatePreparedPersistence(bad) == nil {
						t.Fatal("consumption lineage drift accepted")
					}
				})
			}
			// Returned products must not alias the caller's full bootstrap.
			compiled.Compilation.Bootstrap.Activation.MCPServerIDs[0] = "detached-output"
			if !reflect.DeepEqual(before, w) || bootstrap.Activation.MCPServerIDs[0] == "detached-output" {
				t.Fatal("pure compilation changed an original")
			}
			for _, kind := range []string{"nil", "revision", "run", "epoch", "file_epoch", "file_epoch_missing", "expired", "clock_before", "goal", "dod", "bootstrap", "missing_source", "duplicate_attempt", "duplicate_completion", "latest_blocked", "configuration"} {
				t.Run(kind, func(t *testing.T) {
					bad := cloneWorkspace(w)
					g, d, b, at, fileEpoch := goal, *workspace.CloneDoD(&dod), bootstrap.Clone(), now, epoch
					switch kind {
					case "nil":
						bad = nil
					case "revision":
						bad.Revision++
					case "run":
						bad.ActiveRunID = "other-run"
					case "epoch":
						bad.PreparationFence.PreparationEpoch++
					case "file_epoch":
						fileEpoch += "changed"
					case "file_epoch_missing":
						fileEpoch = ""
					case "expired":
						at = now.Add(30 * time.Second)
					case "clock_before":
						at = now.Add(-time.Nanosecond)
					case "goal":
						g.Text += "changed"
					case "dod":
						d.Items[0] += "changed"
					case "bootstrap":
						b.Definition.GoalDefinition += "changed"
					case "missing_source":
						bad.PreparationSoulEvidence = nil
					case "duplicate_attempt":
						bad.StartupPreparation.Attempts = append(bad.StartupPreparation.Attempts, a)
					case "duplicate_completion":
						bad.StartupPreparation.Completions = append(bad.StartupPreparation.Completions, bad.StartupPreparation.Completions[0])
					case "latest_blocked":
						last := len(bad.StartupPreparation.Completions) - 1
						bad.StartupPreparation.Completions = bad.StartupPreparation.Completions[:last]
					case "configuration":
						bad.DefaultSoulID = "other-soul"
					}
					if _, err := preparationcompiler.CompileFirstExecutionPreparation(bad, &g, &d, b, at, fileEpoch); err == nil {
						t.Fatal("incomplete/stale/foreign preimage accepted")
					}
				})
			}
			still, err := os.ReadFile(path)
			if err != nil || !reflect.DeepEqual(raw, still) || !reflect.DeepEqual(w, before) {
				t.Fatal("archive or input changed", err)
			}
		})
	}
}

func TestFirstExecutionPreparationRejectsAbsentSources(t *testing.T) {
	for _, w := range []*workspace.Workspace{nil, {ID: "ws", Revision: 1}} {
		if _, err := preparationcompiler.CompileFirstExecutionPreparation(w, nil, nil, runtimecontract.RuntimeBootstrap{}, time.Now(), "epoch"); err == nil {
			t.Fatal("missing preparation accepted")
		}
	}
}
