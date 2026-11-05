package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func inputObservationFixture(t *testing.T) (*RuntimeValidationInputRevision, RuntimeFacts) {
	t.Helper()
	r := runtimeValidationProofRequirementForTest(t, "definition", RuntimeCoverageItem{}, "", "recipe", nil)
	s := bindRuntimeValidationSurfaceForTest(runtimeExactProofCloneReadySurfaceFixture("recipe"), r)
	binding := s.ProofRequirementBindings[0].Clone()
	f := RuntimeValidationInputReviewFile{TargetRef: "checks/input", Content: "original malformed bytes\n", ContentHash: RuntimeValidationInputContentHash("original malformed bytes\n")}
	m := &RuntimeValidationInputMaintenance{Recipe: &RuntimeValidationRecipe{ID: "recipe", Command: "check", TimeoutMillis: 60000}, Binding: binding, Files: []RuntimeValidationInputReviewFile{f}, TargetRefs: []string{f.TargetRef}}
	s.CheckRefs, s.ProofRequirementBindings = nil, nil
	s.InputTargetOwnerModuleID, s.InputTargetRefs = RuntimeModuleFile, []string{f.TargetRef}
	s.ProofRequirementUnavailabilities = []RuntimeValidationProofUnavailability{{Requirement: r, ValidationRecipeID: "recipe", Maintenance: m, OriginalDiagnostic: &RuntimeOriginalDiagnostic{SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleShell, ObservedRevision: "revision", OriginalClass: "validation_readiness", OriginalCode: "structural", OriginalMessage: " exact original\n", Signature: "owner/input", TargetRefs: []string{f.TargetRef}}}}
	c := RuntimeOperationAdmissibility{PrerequisiteID: "catalog", OwnerModuleID: RuntimeModuleShell, ObservedRevision: "revision", OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationSurfaces: []RuntimeValidationSurface{s}}
	c.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(c)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	o := RuntimePrerequisiteObservation{PrerequisiteID: c.PrerequisiteID, Kind: PrerequisiteKindOperationAdmissibility, OwnerModuleID: c.OwnerModuleID, ContentHash: c.SnapshotID, Dependencies: []RuntimeFreshnessDependency{{Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree"}}}
	o.ObservationID = StableRuntimePrerequisiteObservationID(o)
	facts := RuntimeFacts{WorkspaceID: "workspace", RunID: "run", DefinitionSeed: RuntimeDefinitionSeed{Hash: "original-seed"}, Focus: CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{r.CoverageWorkItemID}}, Resources: RuntimeResourceFacts{OperationAdmissibilities: []RuntimeOperationAdmissibility{c}, PrerequisiteObservations: []RuntimePrerequisiteObservation{o}}}
	v, err := CompileRuntimeOwnerInputRevision(facts)
	if err != nil {
		t.Fatal(err)
	}
	return v, facts
}

func TestStructuralRevisionRetainsExactOwnerSourceWithoutSemanticReport(t *testing.T) {
	v, f := inputObservationFixture(t)
	g := RuntimeSemanticPlanningGeneration{Reason: RuntimeSemanticPlanningRetirementInputRevision, OwnerInputRevision: v}
	l := RuntimeSemanticPlanningLedger{History: []RuntimeSemanticPlanningGeneration{g}}
	original, _ := json.Marshal(v)
	if err := ValidateRuntimeInputRevisionSource(v, l); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeInputRevisionObservationSources(l, f, nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeInputRevisionRequestSources(l, nil); err != nil {
		t.Fatal("owner source invented provider session", err)
	}
	// Resource invalidation replaces the current observation, but retains exact
	// original identity in the append-only journal. A fresh JSON load joins it.
	o := v.OwnerObservation.Observation
	f.Resources.PrerequisiteObservations = nil
	invalidations := []RuntimePrerequisiteInvalidationRecord{{PrerequisiteIDs: []string{o.PrerequisiteID}, ObservationIDs: []string{o.ObservationID}}}
	if ValidateRuntimeInputRevisionObservationSources(l, f, nil) == nil {
		t.Fatal("lost original source accepted")
	}
	if err := ValidateRuntimeInputRevisionObservationSources(l, f, invalidations); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(l)
	var restored RuntimeSemanticPlanningLedger
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeInputRevisionSource(v, restored); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeInputRevisionObservationSources(restored, f, invalidations); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RuntimeValidationInputRevision){
		"wrong workspace": func(v *RuntimeValidationInputRevision) { v.WorkspaceID = "another" },
		"fake report":     func(v *RuntimeValidationInputRevision) { v.Report.SubjectHash = "invented" },
		"fake request":    func(v *RuntimeValidationInputRevision) { v.SourceLogicalRequestID = "invented" },
		"stale catalog":   func(v *RuntimeValidationInputRevision) { v.OwnerObservation.Observation.ContentHash = "stale" },
		"wrong target":    func(v *RuntimeValidationInputRevision) { v.InputTargetRefs = []string{"delivery"} },
		"recipe omitted":  func(v *RuntimeValidationInputRevision) { v.Recipe = nil },
		"recipe deadline": func(v *RuntimeValidationInputRevision) { v.Recipe.TimeoutMillis = 300000 },
		"original bytes": func(v *RuntimeValidationInputRevision) {
			v.OwnerObservation.Catalog.ValidationSurfaces[0].ProofRequirementUnavailabilities[0].Maintenance.Files[0].Content = "changed"
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := v.Clone()
			mutate(bad)
			if ValidateRuntimeInputRevisionSource(bad, l) == nil {
				t.Fatal("forged source accepted")
			}
		})
	}
	duplicate := l
	duplicate.History = append(append([]RuntimeSemanticPlanningGeneration(nil), l.History...), g)
	if ValidateRuntimeInputRevisionSource(v, duplicate) == nil {
		t.Fatal("duplicate source accepted")
	}
	pending, err := PendingRuntimeValidationInputRevision(l, RuntimeImplementationWorkLedger{}, v.Binding.CoverageWorkItemID)
	if err != nil || !reflect.DeepEqual(pending, v) {
		t.Fatal("lost pending structural source", err)
	}
	copied := v.Clone()
	copied.OwnerObservation.Catalog.ValidationSurfaces[0].ProofRequirementUnavailabilities[0].Maintenance.Files[0].Content = "mutated"
	after, _ := json.Marshal(v)
	if string(original) != string(after) {
		t.Fatal("clone aliases complete original owner fact")
	}
}

func TestInputMaintenanceOpaqueOwnerDoesNotRequireExecutableMetadata(t *testing.T) {
	v, f := inputObservationFixture(t)
	c := &f.Resources.OperationAdmissibilities[0]
	c.ValidationSurfaces[0].ProofRequirementUnavailabilities[0].Maintenance.Recipe = nil
	c.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*c)
	o := &f.Resources.PrerequisiteObservations[0]
	o.ContentHash = c.SnapshotID
	o.ObservationID = StableRuntimePrerequisiteObservationID(*o)
	got, err := CompileRuntimeOwnerInputRevision(f)
	if err != nil || got.Recipe != nil || got.Binding.ValidationRecipeID != v.Binding.ValidationRecipeID {
		t.Fatal("opaque owner recipe acquired an executable-metadata requirement", err)
	}
}
