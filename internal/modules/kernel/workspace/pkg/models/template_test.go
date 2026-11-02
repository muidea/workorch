package workspace

import (
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
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
			old.ContextHints = staleHints.Clone()
			old.EvidencePolicies = workorcheventpayload.CloneSlice(workspace.EvidencePolicies)
			workspace.DefinitionAuthoring = &DefinitionAuthoringRecord{TemplateSnapshot: old.Clone()}
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
	if err != nil || next.DefaultSoulID != workspace.DefaultSoulID || !reflect.DeepEqual(next.ContextHints, workspace.ContextHints) || workspace.ProfileID != "" {
		t.Fatalf("unchanged template discarded workspace settings: %+v %v", next, err)
	}
}

func TestCurrentTemplateRemovesInheritedNestedDefaultsAndKeepsExplicitScopes(t *testing.T) {
	oldHints, _ := workorcheventpayload.FromMap(map[string]any{
		"profileContracts":    map[string]any{"removed": map[string]any{"description": "old"}},
		"nested":              map[string]any{"default": "old", "project": "default"},
		"defaultMCPServerIds": []any{"server"},
	})
	actual := oldHints.Map()
	actual["nested"].(map[string]any)["project"] = "explicit"
	projectHints, _ := workorcheventpayload.FromMap(actual)
	currentHints, _ := workorcheventpayload.FromMap(map[string]any{"nested": map[string]any{"default": "current"}})
	old := WorkspaceTemplate{ID: "t", ContextHints: oldHints, DefaultSoulID: "inherited"}
	w := &Workspace{ID: "w", TemplateID: "t", ContextHints: projectHints, DefaultSoulID: "project-soul", DefinitionAuthoring: &DefinitionAuthoringRecord{TemplateSnapshot: old}}
	before := CaptureWorkspaceConfiguration(w)
	current := WorkspaceTemplate{ID: "t", ContextHints: currentHints, DefaultSoulID: "current-soul"}
	for range 2 {
		selected, err := WorkspaceForCurrentTemplate(w, &current)
		if err != nil {
			t.Fatal(err)
		}
		hints := selected.ContextHints.Map()
		if _, retained := hints["profileContracts"]; retained {
			t.Fatal("removed inherited map survived as an empty invalid declaration")
		}
		if !reflect.DeepEqual(hints["nested"], map[string]any{"default": "current", "project": "explicit"}) || !reflect.DeepEqual(hints["defaultMCPServerIds"], []any{"server"}) || selected.DefaultSoulID != "project-soul" {
			t.Fatalf("current defaults or explicit project selection lost: %+v", hints)
		}
		if !reflect.DeepEqual(before, CaptureWorkspaceConfiguration(w)) {
			t.Fatal("projection changed the original owner configuration")
		}
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

func TestPreparationTemplateRetainsExactSourceAndRejectsTampering(t *testing.T) {
	for _, bindings := range []map[string]string{nil, {}, {"planning": "soul"}} {
		original := WorkspaceTemplate{ID: "t", Name: "selected", SoulBindings: bindings, DoDItems: []string{"original\r\ntext"}}
		selected := NewPreparationTemplate(original)
		if selected.Validate() != nil || !reflect.DeepEqual(original.Clone(), selected.Clone().Template()) {
			t.Fatalf("selected source changed across typed transport: %+v", selected)
		}
		changed := selected.Clone()
		changed.DoDItems[0] = "tampered"
		if changed.Validate() == nil || selected.DoDItems[0] != original.DoDItems[0] {
			t.Fatal("source mutation was accepted or clone aliases original")
		}
		cloned := changed.Clone()
		if cloned.Validate() == nil || !reflect.DeepEqual(cloned, changed) {
			t.Fatal("clone repaired a corrupt selected source")
		}
		changed = selected.Clone()
		changed.SoulBindings = append(changed.SoulBindings, soulbinding.Binding{Stage: "planning", SoulID: "one"}, soulbinding.Binding{Stage: "planning", SoulID: "two"})
		if changed.Validate() == nil || !reflect.DeepEqual(changed, changed.Clone()) {
			t.Fatal("clone removed duplicate source bindings")
		}
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

func TestPreparationOriginalTemplateUsesExactAuthoringEpoch(t *testing.T) {
	original := WorkspaceTemplate{ID: "original", Name: "historical source"}
	current := WorkspaceTemplate{ID: "current", Name: "current source"}
	candidate := DefinitionAdmissionCandidate{CandidateID: "candidate", CandidateRevision: 1, CandidateHash: "original-candidate", WorkspaceTemplateSnapshot: DefinitionTemplateSnapshot{TemplateRef: original.ID}}
	w := &Workspace{TemplateID: current.ID, DefinitionAuthoring: &DefinitionAuthoringRecord{TemplateSnapshot: current}, DefinitionAuthoringHistory: []DefinitionAuthoringConfirmedRevision{{Record: DefinitionAuthoringRecord{TemplateSnapshot: original}}}, DefinitionAdmissionCandidates: []DefinitionAdmissionCandidate{candidate}}
	a := WorkspacePreparationAttempt{Binding: WorkspacePreparationBinding{CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash}}
	got, err := OriginalPreparationTemplate(w, a)
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("original epoch replaced: %+v %v", got, err)
	}
	a.Binding.CandidateHash = "unknown"
	if _, err := OriginalPreparationTemplate(w, a); err == nil {
		t.Fatal("unknown epoch borrowed current declaration")
	}
}
