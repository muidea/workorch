package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	applicationbiz "github.com/muidea/workorch/internal/modules/application/workorchd/biz"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func seedPreparationResourceEligible(t *testing.T, owners *fileDefinitionAdmissionOwners, directory, content string) *workspace.Workspace {
	t.Helper()
	ctx := context.Background()
	confirmation := seedResourceBoundAuthoringConfirmationConfigured(t, owners, "ws-preparation-resource", directory, content, true, func(w *workspace.Workspace) {
		w.InternalDirectory = t.TempDir()
		w.PreparationFence = &workspace.WorkspacePreparationFence{Version: workspace.WorkspacePreparationFenceVersion, WorkspaceID: w.ID, CreationEpoch: strings.Repeat("a", 32)}
	})
	if _, err := sendResourceConfirmation(t, owners, confirmation); err != nil {
		t.Fatal(err)
	}
	w, err := owners.repositories.Workspaces.Get(ctx, confirmation.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := authoringAdmissionCandidateFromCurrentConfiguration(t, w)
	if _, err := sendResourceAdmission(t, owners, workspaceevents.AdmitDefinitionAuthoringCommand{Candidate: candidate, ExpectedWorkspaceRevision: w.Revision, ExpectedConfigurationHash: workspace.WorkspaceConfigurationHash(w)}); err != nil {
		t.Fatal(err)
	}
	w, err = owners.repositories.Workspaces.Get(ctx, w.ID)
	if err != nil || w.ActivationAdmission == nil || w.ActivationAdmission.State != workspace.WorkspaceActivationEligible {
		t.Fatal("seed not eligible", err)
	}
	return w
}

func TestPreparationResourceProductionOriginalBytesReopenAndNoReplay(t *testing.T) {
	for _, scenario := range []string{"exact", "changed", "missing", "oversized", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, storage, directory := context.Background(), t.TempDir(), t.TempDir()
			path := filepath.Join(directory, "requirements.txt")
			original := []byte("原始资源\r\nnext\n")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			owners, file := openResourceAdmissionOwners(t, storage)
			defer closeResourceAdmissionOwners(owners, file)
			w := seedPreparationResourceEligible(t, owners, directory, string(original))
			observed := original
			want := "resource_observed_remaining_checks"
			switch scenario {
			case "changed":
				observed, want = []byte{0xff, 0, '\r', '\n', 'x'}, "resource_content_changed"
				if err := os.WriteFile(path, observed, 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				want = "resource_observation_incomplete"
				if err := os.WriteFile(path, bytes.Repeat([]byte("x"), fileobservation.ResourceMaxBytes+1), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing", "symlink":
				want = "resource_observation_incomplete"
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if scenario == "symlink" {
					outside := filepath.Join(t.TempDir(), "outside")
					if err := os.WriteFile(outside, original, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, path); err != nil {
						t.Fatal(err)
					}
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
			if err != nil || len(stored.PreparationResourceEvidence) != 1 || workspace.ValidatePreparationEvidencePersistence(stored) != nil {
				t.Fatal("missing resource material", err)
			}
			e := stored.PreparationResourceEvidence[0]
			if len(e.Resources) != 1 || e.DeliverableDirectory != directory || e.SpecificationHash != stored.StartupInput.SpecificationHash || e.RequestID != request.RequestID || e.ObservedWorkspaceRevision != w.Revision+1 || operation.Preparation.Record.Attempts[0].ResourceEvidenceHash != e.ContentHash {
				t.Fatal("resource material lost original scope")
			}
			r := e.Resources[0]
			if r.BlockedReason() != want {
				t.Fatalf("resource result = %s, want %s", r.BlockedReason(), want)
			}
			if scenario == "exact" || scenario == "changed" {
				if !r.Observation.Complete || !bytes.Equal(r.Observation.Content, observed) || r.Observation.ContentSHA256 != fileobservation.Digest(observed) {
					t.Fatal("raw content normalized or truncated")
				}
			} else if r.Observation.Complete || len(r.Observation.Content) != 0 || r.Observation.DiagnosticBytes == 0 {
				t.Fatal("incomplete content misrepresented")
			}
			found := false
			for _, check := range operation.Preparation.Record.Attempts[0].Checks {
				if check.State != expectedPreparationCheckState(check.ID, check.Reason) {
					t.Fatal("incorrect per-check judgment", check)
				}
				if strings.HasPrefix(check.ID, "resource/") {
					found = true
					if check.Reason != want {
						t.Fatal("resource classification not projected", check.Reason)
					}
				}
			}
			if !found {
				t.Fatal("resource check omitted")
			}
			public, _ := json.Marshal(operation)
			ordinary, _ := json.Marshal(stored)
			for _, secret := range []string{directory, "requirements.txt", "PreparationResourceEvidence", "ContentSHA256", string(original)} {
				if containsJSONText(public, secret) || containsJSONText(ordinary, "PreparationResourceEvidence") {
					t.Fatal("private material leaked")
				}
			}
			// Completed replay/GET/recovery must not read again: replacing the
			// resource with an oversized file cannot replace historical evidence.
			if scenario != "symlink" {
				if err := os.WriteFile(path, []byte("later"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, recovery := range []bool{false, true} {
				again, err := b.CheckStartupPreparation(ctx, w.ID, request, recovery)
				if err != nil || !reflect.DeepEqual(operation, again) {
					t.Fatal("replay changed original material", err)
				}
			}
			if _, err := b.ReadStartupPreparationView(ctx, w.ID); err != nil {
				t.Fatal(err)
			}
			reopened := openFileDefinitionAdmissionOwners(t, storage)
			defer reopened.close()
			read, err := reopened.repositories.Workspaces.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(stored.PreparationResourceEvidence, read.PreparationResourceEvidence) || calls.Load() != 2 {
				t.Fatal("reopen/replay replaced material or repeated external queries", err)
			}
			runs, _ := owners.repositories.Runs.List(ctx)
			if len(runs) != 0 {
				t.Fatal("preparation started Run")
			}
		})
	}
}

func TestPreparationResourceMaterialAtomicCutsAndDrift(t *testing.T) {
	for _, scenario := range []string{"write_failed", "reply_lost", "revision_drift"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, directory := context.Background(), t.TempDir()
			content := "资源\r\n"
			if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			owners, file := openResourceAdmissionOwners(t, t.TempDir())
			defer closeResourceAdmissionOwners(owners, file)
			w := seedPreparationResourceEligible(t, owners, directory, content)
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
				t.Fatal("unexpected CAS outcome", err)
			}
			stored, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			a := stored.StartupPreparation.Attempts[0]
			if scenario == "write_failed" {
				if len(stored.PreparationResourceEvidence) != 0 || a.ResourceEvidenceHash != "" || a.State != "accepted" {
					t.Fatal("partially committed evidence")
				}
			} else if len(stored.PreparationResourceEvidence) != 1 || a.ResourceEvidenceHash == "" || a.State != "blocked" {
				t.Fatal("material/reference not atomic")
			}
			if scenario == "revision_drift" {
				for _, check := range a.Checks {
					if check.Reason != "binding_changed" {
						t.Fatal("late read retained as current")
					}
				}
			}
			if _, err := b.CheckStartupPreparation(ctx, w.ID, request, true); err != nil {
				t.Fatal(err)
			}
			again, _ := owners.repositories.Workspaces.Get(ctx, w.ID)
			if !reflect.DeepEqual(stored.PreparationResourceEvidence, again.PreparationResourceEvidence) || calls.Load() != 2 {
				t.Fatal("recovery repeated resource observation")
			}
		})
	}
}
