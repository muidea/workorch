package repository_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestPreparationWorkflowRepositoryCASHistoryCloneAndReopen(t *testing.T) {
	for _, backend := range []string{"memory", "file", "scoped"} {
		t.Run(backend, func(t *testing.T) {
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
			w := &workspace.Workspace{ID: "ws", PreparationFence: &workspace.WorkspacePreparationFence{Version: workspace.WorkspacePreparationFenceVersion, WorkspaceID: "ws", CreationEpoch: strings.Repeat("a", 32)}}
			if err := repo.Create(ctx, w); err != nil {
				t.Fatal(err)
			}
			w.StartupPreparation = &workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationWorkflowVersion, WorkspaceID: w.ID, Attempts: []workspace.WorkspacePreparationAttempt{{
				RequestID: "original", PayloadHash: strings.Repeat("b", 64), ExpectedWorkspaceRevision: w.Revision, CommittedWorkspaceRevision: w.Revision + 1, CreationEpoch: w.PreparationFence.CreationEpoch,
				State: workspace.WorkspacePreparationAccepted, StartedAt: time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC),
				Binding: workspace.WorkspacePreparationBinding{AuthoringRef: workspace.DefinitionAuthoringAdmissionRef{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec"}, CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup", AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "policy", CompilerVersion: "compiler"},
				Checks:  []workspace.WorkspacePreparationCheckResult{{ID: "exact_seal", Owner: "workspace", Required: true, State: "pending", Reason: "not_observed"}, {ID: "managed_configuration", Owner: "file", Required: true, State: "pending", Reason: "not_observed"}, {ID: "resource/input", Owner: "file", Required: true, State: "pending", Reason: "not_observed"}},
			}}}
			w.StartupPreparation.Attempts[0].Checks = append(w.StartupPreparation.Attempts[0].Checks, workspace.WorkspacePreparationCheckResult{ID: "skill_selection", Owner: "skill", Required: true, State: "pending", Reason: "not_observed"}, workspace.WorkspacePreparationCheckResult{ID: "soul_selection", Owner: "soul", Required: true, State: "pending", Reason: "not_observed"})
			if err := repo.Update(ctx, w); !errors.Is(err, common.ErrConflict) {
				t.Fatal("ordinary update introduced a workflow")
			}
			if err := repo.CompareAndSwap(ctx, w, w.Revision); err != nil {
				t.Fatal(err)
			}
			// Mutating a returned nested check must not change the stored row.
			w.StartupPreparation.Attempts[0].Checks[0].Owner = "caller"
			current, err := repo.Get(ctx, w.ID)
			if err != nil || current.StartupPreparation.Attempts[0].Checks[0].Owner != "workspace" {
				t.Fatal("accepted row aliases caller check")
			}
			a := &current.StartupPreparation.Attempts[0]
			a.State, a.CompletedAt, a.CommittedWorkspaceRevision = workspace.WorkspacePreparationBlocked, a.StartedAt.Add(time.Second), current.Revision+1
			a.Checks[0].State, a.Checks[0].Reason = "blocked", "check_pipeline_incomplete"
			a.Checks[1].State, a.Checks[1].Reason = "blocked", "managed_config_observation_incomplete"
			a.Checks[2].State, a.Checks[2].Reason = "blocked", "resource_observation_incomplete"
			a.Checks[3].State, a.Checks[3].Reason = "blocked", "skill_registry_observed_selection_pending"
			a.Checks[4].State, a.Checks[4].Reason = "blocked", "souls_observed_remaining_checks"
			originalBytes := []byte{0xff, 0, '\r', '\n', 'x'}
			directory := fileobservation.Directory{Path: filepath.Join(root, "unavailable"), Diagnostic: originalBytes, DiagnosticBytes: len(originalBytes), DiagnosticSHA256: fileobservation.Digest(originalBytes)}
			evidence := workspace.WorkspacePreparationDirectoryEvidence{Version: workspace.PreparationDirectoryEvidenceVersion, WorkspaceID: w.ID, RequestID: a.RequestID, PayloadHash: a.PayloadHash, ObservedWorkspaceRevision: a.ExpectedWorkspaceRevision + 1, StartedAt: a.StartedAt.Add(time.Millisecond), CompletedAt: a.StartedAt.Add(2 * time.Millisecond), Internal: directory.Clone(), Deliverable: directory.Clone()}
			evidence.ContentHash = evidence.Hash()
			a.DirectoryEvidenceHash = evidence.ContentHash
			current.PreparationDirectoryEvidence = []workspace.WorkspacePreparationDirectoryEvidence{evidence}
			resource := fileobservation.Resource{Ref: "input", ContentSHA256: fileobservation.Digest(nil), Diagnostic: append([]byte(nil), originalBytes...), DiagnosticBytes: len(originalBytes), DiagnosticSHA256: fileobservation.Digest(originalBytes)}
			resourceEvidence := workspace.WorkspacePreparationResourceEvidence{Version: workspace.PreparationResourceEvidenceVersion, WorkspaceID: w.ID, RequestID: a.RequestID, PayloadHash: a.PayloadHash, ObservedWorkspaceRevision: a.ExpectedWorkspaceRevision + 1, SpecificationHash: a.Binding.AuthoringRef.SpecificationHash, DeliverableDirectory: root, StartedAt: evidence.StartedAt, CompletedAt: evidence.CompletedAt, Resources: []workspace.WorkspacePreparationResourceObservation{{ID: "input", Required: true, ExpectedContentHash: "sha256:" + fileobservation.Digest(originalBytes), Observation: resource}}}
			resourceEvidence.ContentHash = resourceEvidence.Hash()
			a.ResourceEvidenceHash = resourceEvidence.ContentHash
			current.PreparationResourceEvidence = []workspace.WorkspacePreparationResourceEvidence{resourceEvidence}
			configEvidence := workspace.WorkspacePreparationManagedConfigEvidence{Version: workspace.PreparationManagedConfigEvidenceVersion, Ref: workspace.PreparationManagedConfigRefPrefix + strings.Repeat("d", 32), WorkspaceID: w.ID, RequestID: a.RequestID, PayloadHash: a.PayloadHash, ObservedWorkspaceRevision: a.ExpectedWorkspaceRevision + 1, ConfigurationHash: a.Binding.AuthoringConfigurationHash, InternalDirectory: root, StartedAt: evidence.StartedAt, CompletedAt: evidence.CompletedAt, Files: []workspace.WorkspacePreparationManagedConfigObservation{{Scope: "workspace", Path: "input", Policy: "workspace_preserve", DefaultsJSON: []byte(`{"n":9007199254740993}`), Observation: resource.Clone()}}}
			configEvidence.ContentHash = configEvidence.Hash()
			a.ManagedConfigEvidenceRef = configEvidence.Ref
			current.PreparationManagedConfigEvidence = []workspace.WorkspacePreparationManagedConfigEvidence{configEvidence}
			s := &soulmodel.Soul{ID: "soul", Revision: 1, Name: "Original", SoulMD: "原始 Soul\r\n", SourceProvenance: soulmodel.SoulSourceUserManaged, AllowedSkillRefs: []string{"skill"}, AllowedToolKinds: []string{}}
			s.ContentHash = soulmodel.StableSoulContentHash(s)
			plan, lookup, _ := workspace.PreparationSoulPlan("soul", nil, "")
			facts := workspace.PreparationSoulFacts{Version: "workorch-preparation-souls/v1", WorkspaceID: w.ID, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, SnapshotHash: "observed-snapshot", ReobservedSnapshotHash: "observed-snapshot", ConfigurationHash: a.Binding.AuthoringConfigurationHash, WorkspaceDefaultSoulID: "soul", DefaultLookupSoulID: lookup, Selections: plan, Souls: []*soulmodel.Soul{s}, ReobservedSouls: []*soulmodel.Soul{soulmodel.CloneSoul(s)}, StartedAt: evidence.StartedAt, CompletedAt: evidence.CompletedAt}
			soulEvidence := workspace.WorkspacePreparationSoulEvidence{Version: workspace.PreparationSoulEvidenceVersion, Ref: workspace.PreparationSoulRefPrefix + strings.Repeat("e", 32), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Facts: facts}
			soulEvidence.ContentHash = soulEvidence.Hash()
			a.SoulEvidenceRef = soulEvidence.Ref
			current.PreparationSoulEvidence = []workspace.WorkspacePreparationSoulEvidence{soulEvidence}
			skill := &skillmodel.Skill{ID: "skill", Name: "Original", Version: "v1", Instruction: "原始目录指令", TriggerRules: []string{"go"}, Constraints: []string{}}
			skill.Provenance = workorchskillcontract.Provenance{Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal, ContentHash: workorchskillcontract.StableContentHash(skill.Content())}
			catalog := skillmodel.PreparationRegistryCatalog{Version: skillmodel.RegistryCatalogVersion, WorkspaceID: w.ID, OwnerEpoch: "skill-preparation-epoch:" + strings.Repeat("A", 26), ObservedAt: evidence.StartedAt, Skills: []*skillmodel.Skill{skill}}
			catalog.CatalogHash, _ = skillmodel.RegistryCatalogHash(catalog.Skills)
			registry := workspace.WorkspacePreparationSkillRegistryEvidence{Version: workspace.PreparationSkillRegistryEvidenceVersion, Ref: workspace.PreparationSkillRegistryRefPrefix + strings.Repeat("f", 32), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Facts: workspace.PreparationSkillCatalogFacts{Version: "workorch-preparation-skill-registry-facts/v1", WorkspaceID: w.ID, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, SnapshotHash: "observed-snapshot", ReobservedSnapshotHash: "observed-snapshot", Catalog: catalog, ReobservedCatalog: catalog.Clone(), StartedAt: evidence.StartedAt, CompletedAt: evidence.CompletedAt}}
			registry.ContentHash = registry.Hash()
			a.SkillRegistryEvidenceRef = registry.Ref
			current.PreparationSkillRegistryEvidence = []workspace.WorkspacePreparationSkillRegistryEvidence{registry}
			if err := repo.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			baseline, _ := repo.Get(ctx, w.ID)
			current.PreparationDirectoryEvidence[0].Internal.Diagnostic[0] = 'a'
			current.PreparationResourceEvidence[0].Resources[0].Observation.Diagnostic[0] = 'c'
			current.PreparationManagedConfigEvidence[0].Files[0].Observation.Diagnostic[0] = 'd'
			current.PreparationManagedConfigEvidence[0].Files[0].DefaultsJSON[0] = 'x'
			current.PreparationSoulEvidence[0].Facts.Souls[0].AllowedSkillRefs[0] = "caller"
			current.PreparationSoulEvidence[0].Facts.ReobservedSouls[0].SoulMD = "caller"
			current.PreparationSkillRegistryEvidence[0].Facts.Catalog.Skills[0].TriggerRules[0] = "caller"
			current.PreparationSkillRegistryEvidence[0].Facts.ReobservedCatalog.Skills[0].Instruction = "caller"
			public, err := json.Marshal(baseline)
			if bytes.Contains(public, []byte("PreparationSkillRegistryEvidence")) || bytes.Contains(public, []byte("原始目录指令")) {
				t.Fatal("private registry leaked")
			}
			if bytes.Contains(public, []byte("PreparationSoulEvidence")) || bytes.Contains(public, []byte("原始 Soul")) {
				t.Fatal("private Soul leaked")
			}
			if err != nil || bytes.Contains(public, []byte("PreparationDirectoryEvidence")) || bytes.Contains(public, []byte("PreparationResourceEvidence")) || bytes.Contains(public, []byte("PreparationManagedConfigEvidence")) || bytes.Contains(public, []byte("DiagnosticSHA256")) {
				t.Fatal("private material leaked via ordinary Workspace JSON")
			}
			if backend == "file" {
				info, err := os.Stat(filepath.Join(root, "workspaces.json"))
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("private preparation material not restricted to 0600", err)
				}
			}
			if backend != "memory" {
				repo = open()
			}
			for _, operation := range []string{"erase_registry", "rewrite_registry_ref", "rehash_registry", "erase", "rewrite", "required", "check_omission", "ready", "metadata", "erase_material", "rewrite_material", "rewrite_evidence_ref", "erase_resource_material", "rewrite_resource_material", "rewrite_resource_ref", "erase_config_material", "rewrite_config_material", "rewrite_config_ref", "rewrite_config_defaults", "erase_soul_material", "rewrite_soul_material", "rewrite_soul_ref"} {
				read, err := repo.Get(ctx, w.ID)
				if err != nil || !reflect.DeepEqual(read, baseline) {
					t.Fatal("reopen changed original record")
				}
				switch operation {
				case "erase_registry":
					read.PreparationSkillRegistryEvidence = nil
				case "rewrite_registry_ref":
					read.StartupPreparation.Attempts[0].SkillRegistryEvidenceRef = ""
				case "rehash_registry":
					e := &read.PreparationSkillRegistryEvidence[0]
					for _, c := range []*skillmodel.PreparationRegistryCatalog{&e.Facts.Catalog, &e.Facts.ReobservedCatalog} {
						c.Skills[0].Instruction = "rewritten"
						c.Skills[0].Provenance.ContentHash = workorchskillcontract.StableContentHash(c.Skills[0].Content())
						c.CatalogHash, _ = skillmodel.RegistryCatalogHash(c.Skills)
					}
					e.ContentHash = e.Hash()
				case "erase_soul_material":
					read.PreparationSoulEvidence = nil
				case "rewrite_soul_material":
					e := &read.PreparationSoulEvidence[0]
					for _, s := range []*soulmodel.Soul{e.Facts.Souls[0], e.Facts.ReobservedSouls[0]} {
						s.Name = "Changed"
						s.ContentHash = soulmodel.StableSoulContentHash(s)
					}
					e.ContentHash = e.Hash()
				case "rewrite_soul_ref":
					read.StartupPreparation.Attempts[0].SoulEvidenceRef = ""
				case "erase_config_material":
					read.PreparationManagedConfigEvidence = nil
				case "rewrite_config_material":
					e := &read.PreparationManagedConfigEvidence[0]
					e.Files[0].Observation.Diagnostic[0] = 'z'
					e.Files[0].Observation.DiagnosticSHA256 = fileobservation.Digest(e.Files[0].Observation.Diagnostic)
					e.ContentHash = e.Hash()
				case "rewrite_config_ref":
					read.StartupPreparation.Attempts[0].ManagedConfigEvidenceRef = ""
				case "rewrite_config_defaults":
					e := &read.PreparationManagedConfigEvidence[0]
					e.Files[0].DefaultsJSON = []byte(`{"replaced":true}`)
					e.ContentHash = e.Hash()
				case "erase_resource_material":
					read.PreparationResourceEvidence = nil
				case "rewrite_resource_material":
					e := &read.PreparationResourceEvidence[0]
					e.Resources[0].Observation.Diagnostic[0] = 'z'
					e.Resources[0].Observation.DiagnosticSHA256 = fileobservation.Digest(e.Resources[0].Observation.Diagnostic)
					e.ContentHash = e.Hash()
					read.StartupPreparation.Attempts[0].ResourceEvidenceHash = e.ContentHash
				case "rewrite_resource_ref":
					read.StartupPreparation.Attempts[0].ResourceEvidenceHash = ""
				case "erase_material":
					read.PreparationDirectoryEvidence = nil
				case "rewrite_material":
					read.PreparationDirectoryEvidence[0].Internal.Diagnostic[0] = 'z'
					read.PreparationDirectoryEvidence[0].Internal.DiagnosticSHA256 = fileobservation.Digest(read.PreparationDirectoryEvidence[0].Internal.Diagnostic)
					read.PreparationDirectoryEvidence[0].ContentHash = read.PreparationDirectoryEvidence[0].Hash()
					read.StartupPreparation.Attempts[0].DirectoryEvidenceHash = read.PreparationDirectoryEvidence[0].ContentHash
				case "rewrite_evidence_ref":
					read.StartupPreparation.Attempts[0].DirectoryEvidenceHash = ""
				case "erase":
					read.StartupPreparation = nil
				case "rewrite":
					read.StartupPreparation.Attempts[0].PayloadHash = "forged"
				case "required":
					read.StartupPreparation.Attempts[0].Checks[0].Required = false
				case "check_omission":
					read.StartupPreparation.Attempts[0].Checks = nil
				case "ready":
					read.StartupPreparation.Attempts[0].State = "ready"
				case "metadata":
					read.StartupPreparation.Attempts[0].CompletedAt = a.CompletedAt.Add(time.Second)
					if err := repo.Update(ctx, read); !errors.Is(err, common.ErrConflict) {
						t.Fatal("non-CAS settlement accepted")
					}
					continue
				}
				if err := repo.CompareAndSwap(ctx, read, read.Revision); !errors.Is(err, common.ErrConflict) {
					t.Fatalf("%s rewrote completed history", operation)
				}
			}
			items, err := repo.List(ctx)
			if err != nil || len(items) != 1 || !reflect.DeepEqual(items[0], baseline) {
				t.Fatal("rejected write changed stored history")
			}
			items[0].StartupPreparation.Attempts[0].Checks[0].ID = "caller"
			read, _ := repo.Get(ctx, w.ID)
			if !reflect.DeepEqual(read, baseline) {
				t.Fatal("list aliases checks")
			}
			items[0].PreparationDirectoryEvidence[0].Deliverable.Diagnostic[0] = 'b'
			items[0].PreparationResourceEvidence[0].Resources[0].Observation.Diagnostic[0] = 'b'
			items[0].PreparationManagedConfigEvidence[0].Files[0].DefaultsJSON[0] = 'x'
			items[0].PreparationManagedConfigEvidence[0].Files[0].Observation.Diagnostic[0] = 'b'
			items[0].PreparationSoulEvidence[0].Facts.ReobservedSouls[0].AllowedSkillRefs[0] = "caller"
			read, _ = repo.Get(ctx, w.ID)
			if !reflect.DeepEqual(read, baseline) || !bytes.Equal(read.PreparationDirectoryEvidence[0].Internal.Diagnostic, originalBytes) || !bytes.Equal(read.PreparationResourceEvidence[0].Resources[0].Observation.Diagnostic, originalBytes) {
				t.Fatal("material bytes not isolated/preserved")
			}
			if backend == "file" {
				path := filepath.Join(root, "workspaces.json")
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
				if _, err := open().Get(ctx, w.ID); err == nil {
					t.Fatal("publicly readable material accepted on reopen")
				}
			}
		})
	}
}
