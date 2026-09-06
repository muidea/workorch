package workspace

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCurrentTemplateBindingReloadAndIsolation(t *testing.T) {
	old := &WorkspaceTemplate{ID: "template", Name: "v1", DefaultSoulID: "soul-old", SoulBindings: map[string]string{"planning": "planning-old"}}
	current := &WorkspaceTemplate{ID: "template", Name: "v2", DefaultSoulID: "soul-new", SoulBindings: map[string]string{"execution": "execution-new"}}
	for _, state := range []string{"before first Start", "old admission", "no binding provenance"} {
		t.Run(state, func(t *testing.T) {
			staleHints, _ := workorcheventpayload.FromMap(map[string]any{"removedPolicy": "old"})
			workspace := &Workspace{ID: "ws", Title: "user title", DeliverableDirectory: "/project", InternalDirectory: "/internal", State: common.WorkspaceStateActive,
				ActiveRunID: "run-old", TemplateID: old.ID, ProfileID: old.ID, DefaultSoulID: common.SoulID(old.DefaultSoulID), SoulBindings: map[string]common.SoulID{"planning": "planning-old"},
				ContextHints: staleHints, EvidencePolicies: []workorcheventpayload.Payload{staleHints}}
			switch state {
			case "before first Start":
				workspace.TemplateBindingHash = WorkspaceTemplateSnapshotHash(old)
			case "old admission":
				workspace.StartupInput = &WorkspaceStartupInputContract{TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(old), StartupInputHash: "sealed-old"}
			}
			next, err := WorkspaceForCurrentTemplate(workspace, current)
			if err != nil {
				t.Fatal(err)
			}
			if next.TemplateBindingHash != WorkspaceTemplateSnapshotHash(current) || next.DefaultSoulID != "soul-new" || next.SoulBindings["execution"] != "execution-new" || len(next.SoulBindings) != 1 || !next.ContextHints.Equal(current.ContextHints) || len(next.EvidencePolicies) != 0 {
				t.Fatalf("stale template binding survived: %+v", next)
			}
			if next.ID != workspace.ID || next.Title != workspace.Title || next.DeliverableDirectory != workspace.DeliverableDirectory || next.InternalDirectory != workspace.InternalDirectory || next.State != workspace.State || next.ActiveRunID != workspace.ActiveRunID || next.StartupInput != workspace.StartupInput {
				t.Fatal("binding reload changed identity, directory, lifecycle or old seal")
			}
			next.SoulBindings["execution"] = "caller mutation"
			if workspace.DefaultSoulID != "soul-old" || workspace.SoulBindings["planning"] != "planning-old" || current.SoulBindings["execution"] != "execution-new" {
				t.Fatal("detached binding mutated source")
			}
		})
	}
}

func TestUnchangedTemplatePreservesWorkspaceOverrides(t *testing.T) {
	template := &WorkspaceTemplate{ID: "template", Name: "current", DefaultSoulID: "template-soul"}
	workspace := &Workspace{ID: "ws", TemplateID: template.ID, TemplateBindingHash: WorkspaceTemplateSnapshotHash(template), DefaultSoulID: "explicit-workspace-soul"}
	next, err := WorkspaceForCurrentTemplate(workspace, template)
	if err != nil || !reflect.DeepEqual(next, workspace) {
		t.Fatalf("unchanged template discarded workspace settings: %+v %v", next, err)
	}
}

func TestTemplateSnapshotHashIncludesStageRouting(t *testing.T) {
	template := &WorkspaceTemplate{ID: "template", Name: "template", SoulBindings: map[string]string{"execution": "s1", "planning": "s2"}}
	before := WorkspaceTemplateSnapshotHash(template)
	template.SoulBindings = map[string]string{"planning": "s2", "execution": "s1"}
	if WorkspaceTemplateSnapshotHash(template) != before {
		t.Fatal("map order drifted template identity")
	}
	template.SoulBindings["planning"] = "new"
	if WorkspaceTemplateSnapshotHash(template) == before {
		t.Fatal("stage Soul change did not change template identity")
	}
}

func TestWorkspaceConfigurationHashIgnoresEmptyCollectionTransportShape(t *testing.T) {
	workspace := &Workspace{ID: "ws", TemplateID: "template"}
	before := WorkspaceConfigurationHash(workspace)
	workspace.SoulBindings = map[string]common.SoulID{}
	workspace.EvidencePolicies = []workorcheventpayload.Payload{}
	if WorkspaceConfigurationHash(workspace) != before {
		t.Fatal("empty collection representation broke duplicate admission identity")
	}
	workspace.DefaultLLMSelection = &runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}}
	if WorkspaceConfigurationHash(workspace) == before {
		t.Fatal("provider selection drift was not fenced")
	}
}

func TestArchivedDefinitionCandidateCloneDoesNotNormalize(t *testing.T) {
	old := DefinitionAdmissionCandidate{SchemaVersion: 1, CandidateHash: "historic-hash", AdmissionCompilerVersion: "retired-compiler",
		RuntimeProfileContracts: []runtimecontract.RuntimeProfileContract{{ID: " z ", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: " old ", TargetRefs: []string{"b", "a", "b"}}}}, {ID: "a"}},
		DoDInputs:               []DefinitionCandidateItem{}, ProposedRelationInputs: []DefinitionProposedRelationInput{{SourceItemRefs: []string{"b", "a"}}}}
	cloned := old.Clone()
	if !reflect.DeepEqual(cloned, old) {
		t.Fatalf("archive was canonicalized: %+v", cloned)
	}
	cloned.RuntimeProfileContracts[0].EvidenceRequirements[0].TargetRefs[0] = "changed"
	cloned.ProposedRelationInputs[0].SourceItemRefs[0] = "changed"
	if old.RuntimeProfileContracts[0].EvidenceRequirements[0].TargetRefs[0] != "b" || old.ProposedRelationInputs[0].SourceItemRefs[0] != "b" {
		t.Fatal("archive clone aliases nested fields")
	}
}
