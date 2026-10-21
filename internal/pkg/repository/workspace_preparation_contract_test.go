package repository_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestWorkspacePreparationPersistenceAndCloneIsolation(t *testing.T) {
	for _, backend := range []string{"memory", "file"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "workspaces.json")
			var repo repository.WorkspaceRepository = memory.NewWorkspaceRepository()
			if backend == "file" {
				repo = file.NewWorkspaceRepository(path)
			}
			record := &workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationRecordVersion, WorkspaceID: "ws", Attempts: []workspace.WorkspacePreparationAttempt{{
				RequestID: "request", PayloadHash: "payload", ExpectedWorkspaceRevision: 1, CommittedWorkspaceRevision: 2, State: workspace.WorkspacePreparationUnknown, StartedAt: time.Now().UTC(),
				Binding: workspace.WorkspacePreparationBinding{AuthoringRef: workspace.DefinitionAuthoringAdmissionRef{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec"}, CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup", AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "policy", CompilerVersion: "compiler"},
			}}}
			if record.Validate("ws") != nil {
				t.Fatal("invalid fixture")
			}
			w := &workspace.Workspace{ID: "ws", Revision: 2, StartupPreparation: record}
			expected := record.Clone()
			if err := repo.Create(ctx, w); err != nil {
				t.Fatal(err)
			}
			record.Attempts[0].RequestID = "caller-mutated-create"
			read, err := repo.Get(ctx, "ws")
			if err != nil || !reflect.DeepEqual(*read.StartupPreparation, expected) {
				t.Fatalf("create clone: %v", err)
			}
			read.StartupPreparation.Attempts[0].Binding.AuthoringRef.ProposalHash = "caller-mutated-get"
			listed, err := repo.List(ctx)
			if err != nil || len(listed) != 1 || !reflect.DeepEqual(*listed[0].StartupPreparation, expected) {
				t.Fatalf("list clone: %v", err)
			}
			listed[0].StartupPreparation.Attempts[0].State = workspace.WorkspacePreparationFailed
			read, err = repo.Get(ctx, "ws")
			if err != nil || !reflect.DeepEqual(*read.StartupPreparation, expected) {
				t.Fatalf("get clone: %v", err)
			}
			read.Title = "unrelated metadata"
			if err := repo.CompareAndSwap(ctx, read, read.Revision); err != nil {
				t.Fatal(err)
			}
			read.StartupPreparation.Attempts[0].RequestID = "caller-mutated-cas"
			if backend == "file" {
				repo = file.NewWorkspaceRepository(path)
			}
			restored, err := repo.Get(ctx, "ws")
			if err != nil || !reflect.DeepEqual(*restored.StartupPreparation, expected) || restored.Revision != 3 {
				t.Fatalf("CAS/reopen changed original unresolved record: %+v %v", restored, err)
			}
		})
	}
}
