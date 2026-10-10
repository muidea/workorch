package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeAssessmentOwnerFactSourcesPreserveCatalogAndScopedObservation(t *testing.T) {
	manifest, _, binding := semanticInputReviewV2Fixture(t)
	requirement := RuntimeOwnerFactRequirement{OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession, ValidationProofRequirementHash: binding.RequirementHash}
	var catalog RuntimeOperationAdmissibility
	for _, value := range manifest.OwnerCatalogs {
		if value.OwnerModuleID == RuntimeModuleShell {
			catalog = value.Normalized()
		}
	}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	authority := RuntimeAssessmentAuthorityInput{Focus: CoverageFocus{RequiredOwnerFacts: []RuntimeOwnerFactRequirement{requirement}}, OperationAdmissibilities: []RuntimeOperationAdmissibility{catalog}}
	sources, err := deriveRuntimeAssessmentOwnerFactSources("definition", "ws", authority)
	if err != nil || len(sources) != 1 || sources[0].OwnerRevisionSet == nil || sources[0].CatalogSnapshotID != catalog.SnapshotID {
		t.Fatalf("catalog source: %v %+v", err, sources)
	}
	selector, _ := json.Marshal(struct {
		WorkspaceID string
		Requirement RuntimeOwnerFactRequirement
	}{"ws", requirement})
	selectorDigest := sha256.Sum256(append([]byte("runtime-assessment-owner-fact-selector/v1\x00"), selector...))
	observation, _ := json.Marshal([]struct {
		ID                 string
		Readiness          RuntimeValidationSurfaceReadiness
		ProofApplicability RuntimeValidationProofApplicability
		Published          bool
		RequirementBound   bool
	}{{binding.ValidationRecipeID, RuntimeValidationSurfaceReady, RuntimeValidationProofDirect, true, true}})
	observationDigest := sha256.Sum256(append([]byte("runtime-assessment-owner-fact-observation/v1\x00"), observation...))
	entry := sources[0].OwnerRevisionSet.Entries[0]
	if entry.OwnerInstanceID != RuntimeModuleShell+"/ws" || entry.ResourceID != hex.EncodeToString(selectorDigest[:]) || entry.DependencyHash != entry.ResourceID || entry.Revision != hex.EncodeToString(observationDigest[:]) || entry.SnapshotID != "" {
		t.Fatal("dependency differs from independent consumed-field oracle")
	}
	unrelated := authority.Clone()
	unrelated.OperationAdmissibilities[0].ObservedRevision = "new-owner-revision"
	unrelated.OperationAdmissibilities[0].TargetRefs = []string{"unrelated.txt"}
	unrelated.OperationAdmissibilities[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(unrelated.OperationAdmissibilities[0])
	otherSources, err := deriveRuntimeAssessmentOwnerFactSources("definition", "ws", unrelated)
	if err != nil || !reflect.DeepEqual(otherSources[0].OwnerRevisionSet, sources[0].OwnerRevisionSet) || otherSources[0].CatalogSnapshotID == sources[0].CatalogSnapshotID {
		t.Fatal("unrelated catalog mutation changed scoped dependency or lost source snapshot")
	}
	changed := authority.Clone()
	changed.OperationAdmissibilities[0].ValidationRecipeRefs = nil
	changed.OperationAdmissibilities[0].ValidationRecipes = nil
	changed.OperationAdmissibilities[0].ValidationSurfaces[0] = RuntimeValidationSurface{
		ID: binding.ValidationRecipeID, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Readiness: RuntimeValidationSurfaceUnavailable, ReadinessReason: "unavailable", InputSchemaHash: "schema", ProofApplicability: RuntimeValidationProofDirect,
		EffectScope: RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}
	changed.OperationAdmissibilities[0].SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(changed.OperationAdmissibilities[0])
	changedSources, err := deriveRuntimeAssessmentOwnerFactSources("definition", "ws", changed)
	if err != nil || changedSources[0].OwnerRevisionSet.SetHash == sources[0].OwnerRevisionSet.SetHash {
		t.Fatalf("intersecting postcondition change ignored: %v", err)
	}
	absent := authority.Clone()
	absent.OperationAdmissibilities = nil
	missing, err := deriveRuntimeAssessmentOwnerFactSources("definition", "ws", absent)
	if err != nil || len(missing) != 1 || missing[0].OwnerRevisionSet != nil || missing[0].CatalogSnapshotID != "" {
		t.Fatal("missing catalog manufactured an absence dependency")
	}
	original := RuntimeAssessmentAdjudication{DispatchID: "assessment", AuthorityInputID: "authority", FocusID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}, Disposition: AssessmentDispositionRepairableGap, OwnerFactSources: sources}
	if err := original.ValidateRaw(); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(original)
	var loaded RuntimeAssessmentAdjudication
	if err := json.Unmarshal(encoded, &loaded); err != nil || !reflect.DeepEqual(loaded, original) {
		t.Fatalf("source reload: %v", err)
	}
	for _, clone := range []RuntimeAssessmentAdjudication{original.Normalized(), original.Clone(), cloneRuntimeAssessmentAdjudicationRaw(original), loaded} {
		clone.OwnerFactSources[0].OwnerRevisionSet.Entries[0].Revision = "caller-change"
		if sources[0].OwnerRevisionSet.Entries[0].Revision == "caller-change" {
			t.Fatal("mutable source dependency shared")
		}
		if StableRuntimeAssessmentAdjudicationHash(clone) == StableRuntimeAssessmentAdjudicationHash(original) {
			t.Fatal("adjudication ignores full source body with unchanged SetHash")
		}
	}
	for _, field := range []string{"missing catalog", "missing dependency", "duplicate", "raw requirement", "raw entry", "hash", "catalog"} {
		bad := original.Clone()
		value := &bad.OwnerFactSources[0]
		switch field {
		case "missing catalog":
			value.CatalogSnapshotID = ""
		case "missing dependency":
			value.OwnerRevisionSet = nil
		case "duplicate":
			bad.OwnerFactSources = append(bad.OwnerFactSources, bad.OwnerFactSources[0])
		case "raw requirement":
			value.Requirement.OwnerModuleID = " " + RuntimeModuleShell
		case "raw entry":
			value.OwnerRevisionSet.Entries[0].ResourceID = " " + value.OwnerRevisionSet.Entries[0].ResourceID
		case "hash":
			value.OwnerRevisionSet.SetHash = "forged"
		case "catalog":
			value.CatalogSnapshotID = " " + value.CatalogSnapshotID
		}
		if err := bad.ValidateRaw(); err == nil {
			t.Fatalf("malformed owner-fact source accepted: %s", field)
		}
		if !reflect.DeepEqual(bad.OwnerFactSources, bad.Normalized().OwnerFactSources) {
			t.Fatal("normalization laundered raw owner-fact source")
		}
	}
	for _, scenario := range []string{"ready", "not published", "auxiliary", "wrong operation", "missing requirement", "no requirement", "unknown fact", "later matching surface"} {
		value := catalog.Normalized()
		query := requirement
		switch scenario {
		case "not published":
			value.ValidationRecipeRefs = nil
		case "auxiliary":
			value.ValidationSurfaces[0].ProofApplicability = RuntimeValidationProofAuxiliary
		case "wrong operation":
			value.ValidationSurfaces[0].OperationKind = StepKindFileRead
		case "missing requirement":
			query.ValidationProofRequirementHash = "other-requirement"
		case "no requirement":
			query.ValidationProofRequirementHash = ""
		case "unknown fact":
			query.FactKind = "unknown"
		case "later matching surface":
			first := value.ValidationSurfaces[0]
			first.ProofApplicability = RuntimeValidationProofAuxiliary
			value.ValidationSurfaces = append([]RuntimeValidationSurface{first}, value.ValidationSurfaces...)
		}
		_, observedSatisfied := observeRuntimeOwnerFactRequirementCatalog(query, value)
		if observedSatisfied != assessmentOwnerFactSatisfiedReference(query, value) {
			t.Fatalf("observation changed original classifier: %s", scenario)
		}
	}
	for _, raw := range [][]RuntimeAssessmentOwnerFactSource{nil, {}} {
		value := original.Clone()
		value.OwnerFactSources = raw
		for _, clone := range []RuntimeAssessmentAdjudication{value.Normalized(), value.Clone(), cloneRuntimeAssessmentAdjudicationRaw(value)} {
			if !reflect.DeepEqual(clone.OwnerFactSources, raw) {
				t.Fatal("source nil/empty representation changed")
			}
		}
	}
	for _, invalid := range []string{"raw", "hash", "duplicate", "missing snapshot", "wrong snapshot", "definition", "workspace", "duplicate owner", "raw catalog"} {
		t.Run(invalid, func(t *testing.T) {
			bad := authority.Clone()
			definition, workspace := "definition", "ws"
			switch invalid {
			case "raw":
				bad.Focus.RequiredOwnerFacts[0].OwnerModuleID = " " + RuntimeModuleShell
			case "hash":
				bad.OperationAdmissibilities[0].SnapshotID = "forged"
			case "duplicate":
				bad.Focus.RequiredOwnerFacts = append(bad.Focus.RequiredOwnerFacts, requirement)
			case "missing snapshot":
				bad.OperationAdmissibilities[0].SnapshotID = ""
			case "wrong snapshot":
				bad.OperationAdmissibilities[0].ObservedRevision = "unhashed-change"
			case "definition":
				definition = " definition"
			case "workspace":
				workspace = " ws"
			case "duplicate owner":
				bad.OperationAdmissibilities = append(bad.OperationAdmissibilities, bad.OperationAdmissibilities[0])
			case "raw catalog":
				bad.OperationAdmissibilities[0].OwnerModuleID = " " + RuntimeModuleShell
			}
			if _, err := deriveRuntimeAssessmentOwnerFactSources(definition, workspace, bad); err == nil {
				t.Fatal("invalid sealed source accepted")
			}
		})
	}
}
