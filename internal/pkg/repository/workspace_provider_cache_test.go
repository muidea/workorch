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

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationProviderCacheRepositoryPrivateCloneAndImmutableHistory(t *testing.T) {
	for _, scenario := range []string{"memory_legacy", "file_legacy", "scoped_legacy", "memory_compiled", "file_compiled", "scoped_compiled"} {
		backend, mode, _ := strings.Cut(scenario, "_")
		t.Run(scenario, func(t *testing.T) {
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
			w := &workspace.Workspace{ID: "ws-provider", PreparationFence: &workspace.WorkspacePreparationFence{Version: workspace.WorkspacePreparationFenceVersion, WorkspaceID: "ws-provider", CreationEpoch: strings.Repeat("a", 32)}}
			if err := repo.Create(ctx, w); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			w.StartupPreparation = &workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationWorkflowVersion, WorkspaceID: w.ID, Attempts: []workspace.WorkspacePreparationAttempt{{RequestID: "original", PayloadHash: "payload", ExpectedWorkspaceRevision: w.Revision, CommittedWorkspaceRevision: w.Revision + 1, CreationEpoch: w.PreparationFence.CreationEpoch, State: workspace.WorkspacePreparationAccepted, StartedAt: now, Binding: workspace.WorkspacePreparationBinding{AuthoringRef: workspace.DefinitionAuthoringAdmissionRef{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec"}, CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup", AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "policy", CompilerVersion: "compiler"}, Checks: []workspace.WorkspacePreparationCheckResult{{ID: "provider_cache", Owner: "llm", Required: true, State: "pending", Reason: "not_observed"}}}}}
			if mode == "compiled" {
				w.StartupPreparation.Attempts[0].EvaluationVersion = workspace.PreparationCheckEvaluationVersion
				w.StartupPreparation.Attempts[0].ObservationSnapshotHash = "observed-snapshot"
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
			a.Checks[0].State, a.Checks[0].Reason = "blocked", "provider_declaration_only"
			e := workspace.WorkspacePreparationProviderCacheEvidence{Version: workspace.PreparationProviderCacheEvidenceVersion, Ref: workspace.PreparationProviderCacheRefPrefix + strings.Repeat("b", 32), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Binding: a.Binding, Facts: workspace.PreparationProviderCacheFacts{Version: workspace.PreparationProviderCacheEvidenceVersion, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, Observation: llmevents.ObserveProviderCatalogCacheResult{WorkspaceID: w.ID, OwnerEpoch: "llm-preparation-epoch:ABCDEFGHIJKLMNOPQRSTUVWXYZ", ObservedAt: now.Add(time.Millisecond), Catalog: llmmodel.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", State: llmmodel.ProviderCatalogStateFresh, Revision: "private-catalog-revision", FetchedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute), Models: []llmmodel.ProviderModelDescriptor{{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "original-model"}, RouteOwner: "ai-proxy", CapabilityRevision: "capability", SupportedEndpoints: []string{"/v1/responses"}, Operations: []string{"generate"}, Reasoning: llmmodel.ProviderReasoningCapability{Efforts: []string{"original-effort"}}}}}}}}
			if mode == "compiled" {
				e.Facts.Observation.Configuration = &llmevents.PreparationProviderConfiguration{Version: llmevents.PreparationProviderConfigurationSnapshotVersion, DefaultChat: e.Facts.Observation.Catalog.Models[0].Target}
				e.Facts.Observation.Catalog.Models[0].Operations = []string{"responses"}
				e.Facts.Observation.Catalog.Models[0].CapabilityReady = true
				e.Facts.Observation.Catalog.Models[0].ContextWindowTokens = 32000
				e.Facts.Observation.Catalog.Models[0].MaxOutputTokens = 4000
				d := workspace.PreparationProviderDefinition(current)
				d.ConfigurationHash = a.Binding.AuthoringConfigurationHash
				compiled := workspace.CompilePreparationProviderSelection(d, e)
				if compiled.FailureCode != "" {
					t.Fatal("valid selected source rejected", compiled.FailureCode)
				}
				e.Selection = &compiled
			}
			e.ContentHash = e.Hash()
			a.Checks[0].Reason = e.BlockedReason()
			if mode == "compiled" {
				a.Checks[0].State = "passed"
			}
			current.PreparationProviderCacheEvidence, a.ProviderCacheEvidenceRef = []workspace.WorkspacePreparationProviderCacheEvidence{e}, e.Ref
			if mode == "compiled" {
				bad := *current
				forged := e.Clone()
				d := forged.Selection.Definition.Clone()
				// Same effective target and reason, but caller forges a Workspace
				// declaration where the actual preimage uses the owner default.
				d.WorkspaceDefault = &runtimecontract.ProviderInferenceSelection{Target: forged.Facts.Observation.Configuration.DefaultChat}
				compiled := workspace.CompilePreparationProviderSelection(d, forged)
				forged.Selection = &compiled
				forged.ContentHash = forged.Hash()
				bad.PreparationProviderCacheEvidence = []workspace.WorkspacePreparationProviderCacheEvidence{forged}
				if forged.Validate() != nil {
					t.Fatal("forgery fixture is not independently self-consistent")
				}
				if repo.CompareAndSwap(ctx, &bad, bad.Revision) == nil {
					t.Fatal("first CAS accepted caller-selected target instead of original Workspace source")
				}
			}
			if err := repo.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			baseline, _ := repo.Get(ctx, w.ID)
			if backend == "file" {
				info, err := os.Stat(filepath.Join(root, "workspaces.json"))
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("private cache observation not restricted to 0600", err)
				}
			}
			current.PreparationProviderCacheEvidence[0].Facts.Observation.Catalog.Models[0].SupportedEndpoints[0] = "caller"
			current.PreparationProviderCacheEvidence[0].Facts.Observation.Catalog.Models[0].Operations[0] = "caller"
			current.PreparationProviderCacheEvidence[0].Facts.Observation.Catalog.Models[0].Reasoning.Efforts[0] = "caller"
			if mode == "compiled" {
				current.PreparationProviderCacheEvidence[0].Selection.Selection.Target.ModelID = "caller"
				current.PreparationProviderCacheEvidence[0].Facts.Observation.Configuration.DefaultChat.ModelID = "caller"
			}
			public, _ := json.Marshal(baseline)
			if bytes.Contains(public, []byte("PreparationProviderCacheEvidence")) || bytes.Contains(public, []byte("private-catalog-revision")) || bytes.Contains(public, []byte("OwnerEpoch")) {
				t.Fatal("private cache material leaked")
			}
			if backend != "memory" {
				repo = open()
			}
			read, err := repo.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(read, baseline) {
				t.Fatal("clone/reopen changed historical value", err)
			}
			for _, change := range []string{"erase", "epoch", "reason", "rebind", "selection", "downgrade"} {
				bad, _ := repo.Get(ctx, w.ID)
				switch change {
				case "erase":
					bad.PreparationProviderCacheEvidence = nil
					bad.StartupPreparation.Attempts[0].ProviderCacheEvidenceRef = ""
				case "epoch":
					bad.PreparationProviderCacheEvidence[0].Facts.Observation.OwnerEpoch = "llm-preparation-epoch:BBBBBBBBBBBBBBBBBBBBBBBBBB"
				case "reason":
					bad.StartupPreparation.Attempts[0].Checks[0].Reason = "provider_cache_miss"
				case "rebind":
					bad.PreparationProviderCacheEvidence[0].Binding.StartupInputHash = "other"
				case "selection":
					if mode != "compiled" {
						continue
					}
					bad.PreparationProviderCacheEvidence[0].Selection.Selection.Target.ModelID = "other"
				case "downgrade":
					if mode != "compiled" {
						continue
					}
					bad.PreparationProviderCacheEvidence[0].Selection = nil
					bad.PreparationProviderCacheEvidence[0].Facts.Observation.Configuration = nil
				}
				if len(bad.PreparationProviderCacheEvidence) != 0 {
					bad.PreparationProviderCacheEvidence[0].ContentHash = bad.PreparationProviderCacheEvidence[0].Hash()
				}
				if err := repo.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
					t.Fatal("rewrote completed cache material", change)
				}
			}
		})
	}
}
