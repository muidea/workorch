package repository_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationMemoryInventoryRepositoryPrivateCloneAndImmutableHistory(t *testing.T) {
	for _, variant := range []string{"memory", "file", "scoped", "memory-selection", "file-selection", "scoped-selection"} {
		t.Run(variant, func(t *testing.T) {
			backend, withSelection := strings.TrimSuffix(variant, "-selection"), strings.HasSuffix(variant, "-selection")
			ctx, root := context.Background(), t.TempDir()
			open := func() repository.WorkspaceRepository {
				switch backend {
				case "file":
					return file.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
				case "scoped":
					return workorchrepo.NewWorkspaceScopedFileRepositorySet(root).Workspaces
				default:
					return memory.NewWorkspaceRepository()
				}
			}
			repo := open()
			w := &workspace.Workspace{ID: "ws-memory", PreparationFence: &workspace.WorkspacePreparationFence{Version: workspace.WorkspacePreparationFenceVersion, WorkspaceID: "ws-memory", CreationEpoch: strings.Repeat("a", 32)}}
			if err := repo.Create(ctx, w); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			w.StartupPreparation = &workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationWorkflowVersion, WorkspaceID: w.ID, Attempts: []workspace.WorkspacePreparationAttempt{{RequestID: "original", PayloadHash: "payload", ExpectedWorkspaceRevision: w.Revision, CommittedWorkspaceRevision: w.Revision + 1, CreationEpoch: w.PreparationFence.CreationEpoch, State: workspace.WorkspacePreparationAccepted, StartedAt: now, Binding: workspace.WorkspacePreparationBinding{AuthoringRef: workspace.DefinitionAuthoringAdmissionRef{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec"}, CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup", AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "policy", CompilerVersion: "compiler"}, Checks: []workspace.WorkspacePreparationCheckResult{{ID: "memory_selection", Owner: "memory", Required: true, State: "pending", Reason: "not_observed"}}}}}
			if withSelection {
				a := &w.StartupPreparation.Attempts[0]
				a.EvaluationVersion, a.ObservationSnapshotHash = workspace.PreparationCheckEvaluationVersion, "observed-snapshot"
			}
			if err := repo.CompareAndSwap(ctx, w, w.Revision); err != nil {
				t.Fatal(err)
			}
			current, err := repo.Get(ctx, w.ID)
			if err != nil {
				t.Fatal(err)
			}
			a := &current.StartupPreparation.Attempts[0]
			a.State, a.CompletedAt, a.CommittedWorkspaceRevision = workspace.WorkspacePreparationBlocked, now.Add(time.Second), current.Revision+1
			a.Checks[0].State, a.Checks[0].Reason = "blocked", "memory_inventory_observed_selection_pending"
			e := workspace.WorkspacePreparationMemoryInventoryEvidence{Version: workspace.PreparationMemoryInventoryEvidenceVersion, Ref: workspace.PreparationMemoryInventoryRefPrefix + strings.Repeat("b", 32), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Binding: a.Binding, Facts: workspace.PreparationMemoryInventoryFacts{Version: workspace.PreparationMemoryInventoryEvidenceVersion, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, Observation: memorymodel.PreparationInventory{Version: memorymodel.PreparationInventoryVersion, WorkspaceID: w.ID, OwnerEpoch: "memory-preparation-epoch:ABCDEFGHIJKLMNOPQRSTUVWXYZ", ObservedAt: now.Add(time.Millisecond), Items: []*memorymodel.MemoryItem{{ID: "original-memory", WorkspaceID: w.ID, SourceType: "user", MemoryType: "project_context", Summary: "原摘要", Content: "原文", Tags: []string{"original"}, Vector: []float64{0.25}}}}}}
			if withSelection {
				before := workspace.PreparationMemoryModeObservation{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ConfigurationRevision: "original-config"}
				e.Facts.SelectionSource = &workspace.PreparationMemorySelectionSource{Version: workspace.PreparationMemorySelectionVersion, WorkspaceID: w.ID, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, SnapshotHash: a.ObservationSnapshotHash, ReobservedSnapshotHash: a.ObservationSnapshotHash, Before: before, After: before.Clone(), StartedAt: now, CompletedAt: a.CompletedAt}
				compiled := workspace.CompilePreparationMemorySelection(e.Facts)
				e.Selection = &compiled
			}
			e.ContentHash = e.Hash()
			current.PreparationMemoryInventoryEvidence, a.MemoryInventoryEvidenceRef = []workspace.WorkspacePreparationMemoryInventoryEvidence{e}, e.Ref
			if withSelection {
				a.Checks = workspace.EvaluatePreparationMemoryCheck(*a, &e)
			}
			if err := repo.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			baseline, _ := repo.Get(ctx, w.ID)
			if withSelection && (baseline.StartupPreparation.Attempts[0].Checks[0].State != "passed" || baseline.PreparationMemoryInventoryEvidence[0].Selection.Mode != "list_only") {
				t.Fatal("exact original Memory result not persisted")
			}
			if backend == "file" {
				info, err := os.Stat(filepath.Join(root, "workspaces.json"))
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("private cache observation not restricted to 0600", err)
				}
			}
			current.PreparationMemoryInventoryEvidence[0].Facts.Observation.Items[0].Content = "caller"
			current.PreparationMemoryInventoryEvidence[0].Facts.Observation.Items[0].Tags[0] = "caller"
			current.PreparationMemoryInventoryEvidence[0].Facts.Observation.Items[0].Vector[0] = 99
			if withSelection {
				current.PreparationMemoryInventoryEvidence[0].Selection.Selection.Selected[0].Content = "caller-selection"
				current.PreparationMemoryInventoryEvidence[0].Facts.SelectionSource.Before.ConfigurationRevision = "caller-config"
			}
			public, _ := json.Marshal(baseline)
			if bytes.Contains(public, []byte("PreparationMemoryInventoryEvidence")) || bytes.Contains(public, []byte("原文")) || bytes.Contains(public, []byte("OwnerEpoch")) {
				t.Fatal("private cache material leaked")
			}
			if backend != "memory" {
				repo = open()
			}
			read, err := repo.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(read, baseline) {
				t.Fatal("clone/reopen changed historical value", err)
			}
			changes := []string{"erase", "epoch", "reason", "rebind"}
			if withSelection {
				changes = append(changes, "selection", "configuration", "snapshot")
			}
			for _, change := range changes {
				bad, _ := repo.Get(ctx, w.ID)
				switch change {
				case "erase":
					bad.PreparationMemoryInventoryEvidence = nil
					bad.StartupPreparation.Attempts[0].MemoryInventoryEvidenceRef = ""
				case "epoch":
					bad.PreparationMemoryInventoryEvidence[0].Facts.Observation.OwnerEpoch = "memory-preparation-epoch:BBBBBBBBBBBBBBBBBBBBBBBBBB"
				case "reason":
					bad.StartupPreparation.Attempts[0].Checks[0].Reason = "memory_inventory_unavailable_not_saved"
				case "rebind":
					bad.PreparationMemoryInventoryEvidence[0].Binding.StartupInputHash = "other"
				case "selection":
					bad.PreparationMemoryInventoryEvidence[0].Selection.Selection.Selected[0].Content = "other"
				case "configuration":
					bad.PreparationMemoryInventoryEvidence[0].Facts.SelectionSource.Before.ConfigurationRevision = "other"
				case "snapshot":
					bad.StartupPreparation.Attempts[0].ObservationSnapshotHash = "other"
				}
				if len(bad.PreparationMemoryInventoryEvidence) != 0 {
					bad.PreparationMemoryInventoryEvidence[0].ContentHash = bad.PreparationMemoryInventoryEvidence[0].Hash()
				}
				if err := repo.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
					t.Fatal("rewrote completed cache material", change)
				}
			}
		})
	}
}
