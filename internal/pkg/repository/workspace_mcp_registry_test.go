package repository_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	mcpowner "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/biz"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	file "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func TestPreparationMCPRegistryRepositoryOriginalCASCloneAndReopen(t *testing.T) {
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
			w := &workspace.Workspace{ID: "ws-mcp", PreparationFence: &workspace.WorkspacePreparationFence{Version: workspace.WorkspacePreparationFenceVersion, WorkspaceID: "ws-mcp", CreationEpoch: strings.Repeat("a", 32)}}
			w.DefinitionAuthoring = &workspace.DefinitionAuthoringRecord{}
			// Scoped storage detaches these from metadata; the original accepted
			// configuration binding must survive that internal representation.
			w.DefaultSoulID = "fixture-soul"
			w.SoulBindings = map[string]common.SoulID{"review": "fixture-review-soul"}
			if err := repo.Create(ctx, w); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			w.StartupPreparation = &workspace.WorkspacePreparationRecord{Version: workspace.WorkspacePreparationWorkflowVersion, WorkspaceID: w.ID, Attempts: []workspace.WorkspacePreparationAttempt{{RequestID: "original", PayloadHash: "payload", ExpectedWorkspaceRevision: w.Revision, CommittedWorkspaceRevision: w.Revision + 1, CreationEpoch: w.PreparationFence.CreationEpoch, State: workspace.WorkspacePreparationAccepted, StartedAt: now, EvaluationVersion: workspace.PreparationCheckEvaluationVersion, ObservationSnapshotHash: "observed-snapshot", Binding: workspace.WorkspacePreparationBinding{AuthoringRef: workspace.DefinitionAuthoringAdmissionRef{InputHash: "input", DerivationPermitID: "permit", ProposalHash: "proposal", SpecificationHash: "spec"}, CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: "definition", StartupInputHash: "startup", AuthoringConfigurationHash: "config", SnapshotHash: "snapshot", ManifestHash: "manifest", CheckPolicyVersion: "policy", CompilerVersion: "compiler"}, Checks: []workspace.WorkspacePreparationCheckResult{{ID: "mcp_selection", Owner: "mcpregistry", Required: true, State: "pending", Reason: "not_observed"}}}}}
			w.StartupPreparation.Attempts[0].Binding.AuthoringConfigurationHash = workspace.WorkspaceConfigurationHash(w)
			if err := repo.CompareAndSwap(ctx, w, w.Revision); err != nil {
				t.Fatal(err)
			}
			current, _ := repo.Get(ctx, w.ID)
			a := &current.StartupPreparation.Attempts[0]
			a.State, a.CompletedAt, a.CommittedWorkspaceRevision = workspace.WorkspacePreparationBlocked, now.Add(time.Second), current.Revision+1
			a.Checks[0].State, a.Checks[0].Reason = "blocked", "mcp_registry_observed_connection_pending"
			value := preparationMCPRepositoryFixture(t)
			f := workspace.PreparationMCPObservationFacts{Version: workspace.PreparationMCPRegistryEvidenceVersion, WorkspaceID: w.ID, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, SnapshotHash: a.ObservationSnapshotHash, ReobservedSnapshotHash: a.ObservationSnapshotHash, Before: value, After: value.Clone(), StartedAt: now, CompletedAt: a.CompletedAt}
			e := workspace.WorkspacePreparationMCPRegistryEvidence{Version: workspace.PreparationMCPRegistryEvidenceVersion, Ref: workspace.PreparationMCPRegistryRefPrefix + strings.Repeat("b", 32), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Binding: a.Binding, Facts: f}
			compiled := workspace.CompilePreparationMCPSelection(workspace.PreparationMCPDefinition(current), e, nil)
			if compiled.FailureCode != "skill_source_incomplete" {
				t.Fatal("missing Skill became empty selection")
			}
			e.Selection = &compiled
			e.SelectionEvaluationVersion = workspace.PreparationMCPSelectionEvaluationVersion
			e.ContentHash = e.Hash()
			a.Checks[0].Reason = e.BlockedReason()
			a.MCPRegistryEvidenceRef, current.PreparationMCPRegistryEvidence = e.Ref, []workspace.WorkspacePreparationMCPRegistryEvidence{e}
			// Older compiled history remains independently readable, but a new
			// archive must consume the compilation under the current contract.
			legacy := *current
			legacyRecord := current.StartupPreparation.Clone()
			legacy.StartupPreparation = &legacyRecord
			legacy.PreparationMCPRegistryEvidence = workspace.ClonePreparationMCPRegistryEvidence(current.PreparationMCPRegistryEvidence)
			legacy.PreparationMCPRegistryEvidence[0].SelectionEvaluationVersion = ""
			legacy.PreparationMCPRegistryEvidence[0].ContentHash = legacy.PreparationMCPRegistryEvidence[0].Hash()
			legacy.StartupPreparation.Attempts[0].Checks[0].Reason = "mcp_registry_observed_connection_pending"
			if workspace.ValidatePreparationEvidencePersistence(&legacy) != nil || repo.CompareAndSwap(ctx, &legacy, current.Revision) == nil {
				t.Fatal("new archive bypassed the selection consumer or old history became unreadable")
			}
			// A complete, self-consistently recompiled alternative declaration is
			// not the accepted owner's source. Reject before the first archive CAS.
			forged := *current
			forged.PreparationMCPRegistryEvidence = workspace.ClonePreparationMCPRegistryEvidence(current.PreparationMCPRegistryEvidence)
			alternative := compiled.Definition.Clone()
			alternative.DefaultServerIDs = []string{"not-in-original"}
			v := workspace.CompilePreparationMCPSelection(alternative, e, nil)
			forged.PreparationMCPRegistryEvidence[0].Selection = &v
			forged.PreparationMCPRegistryEvidence[0].ContentHash = forged.PreparationMCPRegistryEvidence[0].Hash()
			if workspace.ValidatePreparationEvidencePersistence(&forged) != nil || repo.CompareAndSwap(ctx, &forged, current.Revision) == nil {
				t.Fatal("original preimage not independently checked")
			}
			if err := repo.CompareAndSwap(ctx, current, current.Revision); err != nil {
				t.Fatal(err)
			}
			baseline, _ := repo.Get(ctx, w.ID)
			if repo.CompareAndSwap(ctx, &legacy, baseline.Revision) == nil {
				t.Fatal("downgraded completed selection consumer")
			}
			current.PreparationMCPRegistryEvidence[0].Facts.Before.Servers[0].Args = append(current.PreparationMCPRegistryEvidence[0].Facts.Before.Servers[0].Args, "caller")
			current.PreparationMCPRegistryEvidence[0].Facts.After.Capabilities[0].Description = "caller"
			pub, _ := json.Marshal(baseline)
			if bytes.Contains(pub, []byte("PreparationMCPRegistryEvidence")) || bytes.Contains(pub, []byte("EnvironmentRefs")) {
				t.Fatal("private registry leaked")
			}
			if backend != "memory" {
				repo = open()
			}
			read, err := repo.Get(ctx, w.ID)
			if err != nil || !reflect.DeepEqual(read, baseline) {
				t.Fatal("clone/reopen changed history", err)
			}
			for _, change := range []string{"erase", "snapshot", "binding", "reason", "owner", "required", "passed", "subset", "source", "evaluation", "scope", "before_acceptance", "after_settlement", "ref", "duplicate", "legacy", "selection", "selection_version", "selection_evaluation", "selection_evaluation_erase", "selection_reason", "selection_erase", "selection_unknown"} {
				bad, _ := repo.Get(ctx, w.ID)
				e := &bad.PreparationMCPRegistryEvidence[0]
				c := &bad.StartupPreparation.Attempts[0].Checks[0]
				switch change {
				case "selection_evaluation":
					e.SelectionEvaluationVersion = "unknown"
				case "selection_evaluation_erase":
					e.SelectionEvaluationVersion = ""
				case "selection_reason":
					c.Reason = "mcp_selection_server_missing"
				case "selection_erase":
					e.Selection = nil
				case "selection_unknown":
					e.Selection.FailureCode = "unknown"
				case "selection":
					e.Selection.FailureCode = ""
				case "selection_version":
					e.Selection.Version = "unknown"
				case "erase":
					bad.PreparationMCPRegistryEvidence = nil
					bad.StartupPreparation.Attempts[0].MCPRegistryEvidenceRef = ""
				case "snapshot":
					e.Facts.SnapshotHash, e.Facts.ReobservedSnapshotHash = "other", "other"
				case "binding":
					e.Binding.StartupInputHash = "other"
				case "reason":
					c.Reason = "mcp_registry_observation_unavailable_not_saved"
				case "owner":
					c.Owner = "caller"
				case "required":
					c.Required = false
				case "passed":
					c.State = "passed"
				case "subset":
					e.Facts.Before.Capabilities = nil
					e.Facts.After.Capabilities = nil
				case "source":
					e.Facts.Before.Servers[0].Name = "other"
					e.Facts.After.Servers[0].Name = "other"
				case "evaluation":
					bad.StartupPreparation.Attempts[0].EvaluationVersion = ""
				case "scope":
					e.Facts.WorkspaceID = "another-workspace"
				case "before_acceptance":
					e.Facts.StartedAt = now.Add(-time.Second)
				case "after_settlement":
					e.Facts.CompletedAt = a.CompletedAt.Add(time.Second)
				case "ref":
					e.Ref = workspace.PreparationMCPRegistryRefPrefix + strings.Repeat("c", 32)
				case "duplicate":
					bad.PreparationMCPRegistryEvidence = append(bad.PreparationMCPRegistryEvidence, e.Clone())
				case "legacy":
					bad.StartupPreparation.Version = workspace.WorkspacePreparationRecordVersion
				}
				if len(bad.PreparationMCPRegistryEvidence) != 0 {
					e.ContentHash = e.Hash()
				}
				if workspace.ValidatePreparationEvidencePersistence(bad) == nil {
					t.Fatal("invalid self-consistently rehashed material passed independent read validation", change)
				}
				if repo.CompareAndSwap(ctx, bad, bad.Revision) == nil {
					t.Fatal("rewrote completed registry history", change)
				}
			}
		})
	}
}

func preparationMCPRepositoryFixture(t *testing.T) mcpevents.RegistrySnapshotResult {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	owner, err := mcpowner.New(hub, nil, mcpowner.Dependencies{Registry: memory.NewMCPRegistryRepository(), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer owner.Teardown(ctx)
	r := hub.Send(magicevent.NewEventWithContext(mcpevents.TopicMCPCommandEnsureDefaults, "kernel/system", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.EnsureDefaultsCommand{}))
	if r == nil || r.Error() != nil {
		t.Fatal("fixture defaults")
	}
	r = hub.Send(magicevent.NewEventWithContext(mcpevents.TopicMCPCommandReadRegistrySnapshot, "application/workorchd", mcpevents.CommandDestination, magicevent.NewHeader(), ctx, mcpevents.ReadRegistrySnapshotCommand{}))
	if r == nil || r.Error() != nil {
		t.Fatal("fixture read")
	}
	raw, getErr := r.Get()
	if getErr != nil {
		t.Fatal(getErr)
	}
	return raw.(mcpevents.RegistrySnapshotResult)
}
