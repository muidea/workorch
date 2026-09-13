package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func workScopeManifestFixture() RuntimeWorkspaceManifestBinding {
	observation := RuntimePrerequisiteObservation{PrerequisiteID: "manifest-1", Kind: PrerequisiteKindManifest, OwnerModuleID: RuntimeModuleFile,
		ContentHash: "tree-1", Dependencies: []RuntimeFreshnessDependency{{Domain: RuntimeResourceWorkspaceTree,
			OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-1"}}}
	observation.ObservationID = StableRuntimePrerequisiteObservationID(observation)
	return RuntimeWorkspaceManifestBinding{Manifest: WorkspaceManifest{Revision: "tree-1", TargetRefs: []string{"resource-a"}}, Observation: observation}
}

func TestImplementationWorkScopeIndependentSourceFence(t *testing.T) {
	seed, focuses, profiles := definitionImplementationWorkFixture(t)
	source := RuntimeImplementationWorkScopeSources{WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1",
		Seed: seed, Focuses: focuses, Profiles: profiles, CoverageWorkItemID: "cwi-1", ManifestBinding: workScopeManifestFixture()}
	before, _ := json.Marshal(source)
	base := workScopeObservationFixture(t)
	if err := base.ValidateForSources(source); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeImplementationWorkScopeRequest){
		"policy expansion": func(r *RuntimeImplementationWorkScopeRequest) { r.Policy.MaxRoundsPerCWI = 8 },
		"workspace":        func(r *RuntimeImplementationWorkScopeRequest) { r.WorkspaceID = "other-workspace" },
		"run":              func(r *RuntimeImplementationWorkScopeRequest) { r.RunID = "other-run" },
		"authority":        func(r *RuntimeImplementationWorkScopeRequest) { r.AuthoritySnapshotID = "other-authority" },
		"definition":       func(r *RuntimeImplementationWorkScopeRequest) { r.DefinitionSeedHash = "other-definition" },
		"CWI":              func(r *RuntimeImplementationWorkScopeRequest) { r.CoverageWorkItemID = "other-cwi" },
	} {
		t.Run(name, func(t *testing.T) {
			o := base.Clone()
			mutate(&o.Request)
			o.Request.RequestID = implementationWorkScopeRequestID(o.Request)
			o.Selection.PrerequisiteID = o.Request.RequestID
			o.Selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(o.Selection)
			o.Observation.PrerequisiteID = o.Request.RequestID
			o.Observation.ContentHash = o.Selection.SnapshotID
			o.Observation.ObservationID = StableRuntimePrerequisiteObservationID(o.Observation)
			if err := o.ValidateForRequest(o.Request); err != nil {
				t.Fatalf("fixture is not self-consistent: %v", err)
			}
			if err := o.Request.ValidateForSources(source); err == nil {
				t.Fatal("self-rehashed request bypassed sealed source")
			}
			if err := o.ValidateForSources(source); err == nil {
				t.Fatal("self-rehashed observation bypassed sealed source")
			}
		})
	}
	var replay RuntimeImplementationWorkScopeObservation
	data, _ := json.Marshal(base)
	if err := json.Unmarshal(data, &replay); err != nil {
		t.Fatal(err)
	}
	if err := replay.ValidateForSources(source); err != nil {
		t.Fatal(err)
	}
	omitted := source
	omitted.Profiles = nil
	if err := replay.ValidateForSources(omitted); err == nil {
		t.Fatal("missing profile source accepted")
	}
	current := source
	current.ManifestBinding = source.ManifestBinding.Clone()
	current.ManifestBinding.Manifest.Revision = "tree-2"
	current.ManifestBinding.Observation.ContentHash = "tree-2"
	current.ManifestBinding.Observation.Dependencies[0].Revision = "tree-2"
	current.ManifestBinding.Observation.ObservationID = StableRuntimePrerequisiteObservationID(current.ManifestBinding.Observation)
	if _, err := current.Compile(); err != nil {
		t.Fatal(err)
	}
	if err := replay.ValidateForSources(current); err == nil {
		t.Fatal("old observation accepted against current tree")
	}
	after, _ := json.Marshal(source)
	if string(before) != string(after) {
		t.Fatal("source validation mutated sealed input")
	}
}

func workScopeRequestFixture(t *testing.T) RuntimeImplementationWorkScopeRequest {
	t.Helper()
	seed, focuses, profiles := definitionImplementationWorkFixture(t)
	request, err := CompileRuntimeImplementationWorkScopeRequest("ws-1", "run-1", "authority-1", seed, focuses, profiles, "cwi-1", workScopeManifestFixture())
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func workScopeObservationFixture(t *testing.T) RuntimeImplementationWorkScopeObservation {
	t.Helper()
	request := workScopeRequestFixture(t)
	p := request.Policy
	selection := RuntimeTargetSelection{
		PrerequisiteID: request.RequestID, OwnerModuleID: p.OwnerModuleID, TargetRole: "implementation_work",
		SelectorRef: p.TargetSelectorRef, SelectorOwnerRef: p.TargetSelectorOwnerRef, SelectorSubjectRef: p.TargetSelectorSubjectRef,
		SelectorCandidateRefs:                append([]string(nil), p.TargetSelectorCandidateRefs...),
		ProspectiveTargetAllOfConstraintRefs: append([]string(nil), p.ProspectiveTargetAllOfConstraintRefs...),
		Disposition:                          RuntimeTargetSelectionSelected, TargetRefs: []string{"resource-a"}, CandidateTargetRefs: []string{"resource-a"},
		CandidateCount: 1, ObservedRevision: "tree-1",
	}
	selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(selection)
	fact := request.ManifestBinding.Observation
	fact.PrerequisiteID, fact.Kind, fact.ContentHash = request.RequestID, PrerequisiteKindTargetSelection, selection.SnapshotID
	fact.ObservationID = StableRuntimePrerequisiteObservationID(fact)
	return RuntimeImplementationWorkScopeObservation{Request: request, Selection: selection, Observation: fact}
}

func TestImplementationWorkScopeRequestBindsExactDefinitionAndPolicy(t *testing.T) {
	seed, focuses, profiles := definitionImplementationWorkFixture(t)
	before, _ := json.Marshal(focuses)
	request := workScopeRequestFixture(t)
	manifest := workScopeManifestFixture()
	manifest.Observation.ObservedAt = "2026-09-12T00:00:00Z"
	again, err := CompileRuntimeImplementationWorkScopeRequest("ws-1", "run-1", "authority-1", seed, focuses, profiles, "cwi-1", manifest)
	if err != nil || !reflect.DeepEqual(request, again) {
		t.Fatalf("audit timestamp changed scope identity: %v", err)
	}
	clone := request.Clone()
	clone.Policy.TargetSelectorCandidateRefs[0] = "changed"
	clone.ManifestBinding.Manifest.TargetRefs[0] = "changed"
	clone.ManifestBinding.Observation.Dependencies[0].Revision = "changed"
	if request.Policy.TargetSelectorCandidateRefs[0] == "changed" || request.ManifestBinding.Manifest.TargetRefs[0] == "changed" || request.ManifestBinding.Observation.Dependencies[0].Revision == "changed" {
		t.Fatal("request clone aliases original")
	}
	for _, id := range []common.CoverageWorkItemID{"", "*", "other-cwi"} {
		if _, err := CompileRuntimeImplementationWorkScopeRequest("ws-1", "run-1", "authority-1", seed, focuses, profiles, id, workScopeManifestFixture()); err == nil {
			t.Fatalf("unknown CWI %q accepted", id)
		}
	}
	if _, err := CompileRuntimeImplementationWorkScopeRequest("ws-1", "run-1", "authority-1", seed, focuses, nil, "cwi-1", workScopeManifestFixture()); err == nil {
		t.Fatal("policy without Profile source accepted")
	}
	invalidManifest := workScopeManifestFixture()
	invalidManifest.Observation.OwnerModuleID = " " + RuntimeModuleFile
	if _, err := CompileRuntimeImplementationWorkScopeRequest("ws-1", "run-1", "authority-1", seed, focuses, profiles, "cwi-1", invalidManifest); err == nil {
		t.Fatal("compiler normalized an invalid manifest owner before admission")
	}
	after, _ := json.Marshal(focuses)
	if string(before) != string(after) {
		t.Fatal("scope compiler changed original Definition/proof")
	}
	for name, mutate := range map[string]func(*RuntimeImplementationWorkScopeRequest){
		"workspace":  func(r *RuntimeImplementationWorkScopeRequest) { r.WorkspaceID = "ws-2" },
		"run":        func(r *RuntimeImplementationWorkScopeRequest) { r.RunID = "run-2" },
		"authority":  func(r *RuntimeImplementationWorkScopeRequest) { r.AuthoritySnapshotID = "authority-2" },
		"definition": func(r *RuntimeImplementationWorkScopeRequest) { r.DefinitionSeedHash = "seed-2" },
		"CWI":        func(r *RuntimeImplementationWorkScopeRequest) { r.CoverageWorkItemID = "cwi-2" },
		"policy":     func(r *RuntimeImplementationWorkScopeRequest) { r.Policy.MaxRoundsPerCWI++ },
		"manifest":   func(r *RuntimeImplementationWorkScopeRequest) { r.ManifestBinding.Manifest.Revision = "tree-2" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := request.Clone()
			mutate(&changed)
			if changed.Validate() == nil || implementationWorkScopeRequestID(changed) == request.RequestID {
				t.Fatal("request fence did not bind changed input")
			}
		})
	}
}

func TestImplementationWorkScopeRequestSeparatesCWIAndClonesRawObservation(t *testing.T) {
	seed, focuses, profiles := definitionImplementationWorkFixture(t)
	seed.CoverageItems = append(seed.CoverageItems, RuntimeCoverageItem{ID: "cwi-2", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-2", TargetText: "second criterion"})
	seed.Slices[0].CoverageWorkItemIDs = append(seed.Slices[0].CoverageWorkItemIDs, "cwi-2")
	seed = seed.Normalized()
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)
	second := focuses[0].Clone()
	second.ID, second.AssessmentID, second.AssessmentText = "focus-2", "dod-2", "second criterion"
	second.EvidenceDeclarationRef = "definition/dodItem/dod-2"
	second.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-2"}
	focuses = append(focuses, second)
	first, err := CompileRuntimeImplementationWorkScopeRequest("ws-1", "run-1", "authority-1", seed, focuses, profiles, "cwi-1", workScopeManifestFixture())
	if err != nil {
		t.Fatal(err)
	}
	other, err := CompileRuntimeImplementationWorkScopeRequest("ws-1", "run-1", "authority-1", seed, focuses, profiles, "cwi-2", workScopeManifestFixture())
	if err != nil || first.RequestID == other.RequestID {
		t.Fatalf("CWI observations shared identity: %v", err)
	}
	base := workScopeObservationFixture(t)
	empty := base.Clone()
	empty.Selection.TargetRefs = []string{}
	empty.Selection.SelectorCandidateRefs = []string{}
	empty.Observation.Dependencies = []RuntimeFreshnessDependency{}
	empty.Request.ManifestBinding.Manifest.TargetRefs = []string{}
	if !reflect.DeepEqual(empty, empty.Clone()) {
		t.Fatal("raw clone changed empty slices to nil")
	}
	base.Selection.TargetRefs = []string{"", "resource-a", "resource-a"}
	base.Observation.OwnerModuleID = " invalid owner "
	clone := base.Clone()
	if !reflect.DeepEqual(base, clone) {
		t.Fatal("clone repaired invalid raw observation")
	}
	clone.Selection.TargetRefs[0] = "changed"
	clone.Selection.CandidateTargetRefs[0] = "changed"
	clone.Selection.SelectorCandidateRefs[0] = "changed"
	clone.Selection.ProspectiveTargetAllOfConstraintRefs[0] = "changed"
	clone.Observation.Dependencies[0].Revision = "changed"
	if base.Selection.TargetRefs[0] != "" || base.Selection.CandidateTargetRefs[0] == "changed" || base.Selection.SelectorCandidateRefs[0] == "changed" || base.Selection.ProspectiveTargetAllOfConstraintRefs[0] == "changed" || base.Observation.Dependencies[0].Revision == "changed" {
		t.Fatal("observation clone aliases owner preimage")
	}
}

func TestImplementationWorkScopeObservationRejectsStaleAndCrossScopeFacts(t *testing.T) {
	base := workScopeObservationFixture(t)
	if err := base.ValidateForRequest(base.Request); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(base)
	var replay RuntimeImplementationWorkScopeObservation
	if err := json.Unmarshal(data, &replay); err != nil || replay.ValidateForRequest(base.Request) != nil || !reflect.DeepEqual(base, replay) {
		t.Fatalf("exact current-version replay failed: %v", err)
	}
	for name, mutate := range map[string]func(*RuntimeImplementationWorkScopeObservation){
		"wrong owner":    func(o *RuntimeImplementationWorkScopeObservation) { o.Selection.OwnerModuleID = RuntimeModuleShell },
		"wrong selector": func(o *RuntimeImplementationWorkScopeObservation) { o.Selection.SelectorRef = "other" },
		"wrong role":     func(o *RuntimeImplementationWorkScopeObservation) { o.Selection.TargetRole = "terminal_proof" },
		"wrong request":  func(o *RuntimeImplementationWorkScopeObservation) { o.Selection.PrerequisiteID = "other" },
		"cross CWI": func(o *RuntimeImplementationWorkScopeObservation) {
			o.Request.CoverageWorkItemID = "cwi-2"
			o.Request.RequestID = implementationWorkScopeRequestID(o.Request)
		},
		"new tree with valid hashes": func(o *RuntimeImplementationWorkScopeObservation) {
			o.Selection.ObservedRevision = "tree-2"
			o.Observation.Dependencies[0].Revision = "tree-2"
		},
		"missing revision proof":      func(o *RuntimeImplementationWorkScopeObservation) { o.Observation.Dependencies = nil },
		"invented prospective target": func(o *RuntimeImplementationWorkScopeObservation) { o.Selection.ProspectiveTargetRef = "invented" },
		"invented existing target": func(o *RuntimeImplementationWorkScopeObservation) {
			o.Selection.TargetRefs, o.Selection.CandidateTargetRefs = []string{"invented"}, []string{"invented"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var changed RuntimeImplementationWorkScopeObservation
			if err := json.Unmarshal(data, &changed); err != nil {
				t.Fatal(err)
			}
			mutate(&changed)
			changed.Selection.SnapshotID = StableRuntimeTargetSelectionSnapshotID(changed.Selection)
			changed.Observation.ContentHash = changed.Selection.SnapshotID
			changed.Observation.ObservationID = StableRuntimePrerequisiteObservationID(changed.Observation)
			if changed.ValidateForRequest(base.Request) == nil {
				t.Fatal("rehashed incompatible observation accepted")
			}
		})
	}
}
