package llm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func structuralRevisionProviderFixture(t *testing.T) *rc.RuntimeValidationInputRevision {
	t.Helper()
	r := runtimeInputReviewProviderFixture(t)
	c := r.SemanticManifest.OwnerCatalogs[0].Clone()
	s := &c.ValidationSurfaces[0]
	b := s.ProofRequirementBindings[0].Clone()
	b.InputReview = nil
	f := rc.RuntimeValidationInputReviewFile{TargetRef: "main.go", Content: "original malformed input\n"}
	f.ContentHash = rc.RuntimeValidationInputContentHash(f.Content)
	m := &rc.RuntimeValidationInputMaintenance{Binding: b, Recipe: &c.ValidationRecipes[0], Files: []rc.RuntimeValidationInputReviewFile{f}, TargetRefs: []string{f.TargetRef}}
	s.CheckRefs, s.ProofRequirementBindings = nil, nil
	s.InputTargetOwnerModuleID, s.InputTargetRefs = rc.RuntimeModuleFile, []string{f.TargetRef}
	s.ProofRequirementUnavailabilities = []rc.RuntimeValidationProofUnavailability{{
		Requirement: *r.SemanticManifest.ProofObligations[0].ValidationProofRequirement, ValidationRecipeID: s.ID, Maintenance: m,
		OriginalDiagnostic: &rc.RuntimeOriginalDiagnostic{SchemaVersion: rc.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: c.OwnerModuleID, ObservedRevision: c.ObservedRevision, OriginalClass: "validation_readiness", OriginalCode: "owner_structure_invalid", OriginalMessage: " exact owner diagnostic\n", Signature: "owner/input", TargetRefs: m.TargetRefs},
	}}
	c.ValidationRecipeRefs = rc.RuntimeSelectableValidationRecipeRefs(c.ValidationSurfaces)
	c.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(c)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	o := rc.RuntimePrerequisiteObservation{PrerequisiteID: c.PrerequisiteID, Kind: rc.PrerequisiteKindOperationAdmissibility, OwnerModuleID: c.OwnerModuleID, ContentHash: c.SnapshotID, Dependencies: []rc.RuntimeFreshnessDependency{{Domain: rc.RuntimeResourceWorkspaceTree, OwnerModuleID: rc.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree"}}}
	o.ObservationID = rc.StableRuntimePrerequisiteObservationID(o)
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	v := &rc.RuntimeValidationInputRevision{WorkspaceID: r.SemanticContext.WorkspaceID, RunID: r.SemanticContext.RunID, DefinitionSeedHash: r.SemanticContext.DefinitionSeed.Hash, OwnerModuleID: rc.RuntimeModuleFile, Binding: b, Recipe: m.Recipe, InputTargetRefs: m.TargetRefs, OwnerObservation: &rc.RuntimeValidationInputRevisionObservation{Catalog: c, Observation: o}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	return v
}

// A structural owner observation is not a semantic conflict report. Both real
// planning branches must survive projection, exact integrity and frozen reload.
func TestStructuralRevisionPurposeAndProofRequestRoundtrip(t *testing.T) {
	for _, purpose := range []bool{true, false} {
		t.Run(map[bool]string{true: "purpose", false: "proof"}[purpose], func(t *testing.T) {
			r := runtimeSemanticWorkProviderRequestForTest(t, purpose, structuralRevisionProviderFixture(t))
			before, _ := json.Marshal(r)
			p, err := runtimeProviderRequest(r)
			if err != nil {
				t.Fatal(err)
			}
			var authority runtimeSemanticAuthorityProjection
			if err := json.Unmarshal([]byte(p.PromptContext.Sections[3].Content), &authority); err != nil {
				t.Fatal(err)
			}
			work := semanticLocalWork(&authority, -1)
			if !purpose {
				work = semanticLocalWork(&authority, 0)
			}
			if work == nil || work.InputCoverageRevisionTasks != nil || work.InputRevision == nil || work.InputRevision.Report != nil || work.InputRevision.Observation == nil || work.TerminalEvidenceCredit || strings.Contains(p.PromptContext.Sections[3].Content, "inputCoverageRevisionTasks") {
				t.Fatal("structural request invented semantic findings or lost original observation")
			}
			expected, err := runtimeSemanticManifestForProvider(r.SemanticManifest, r.SemanticContext.PlanStep, r.SemanticContext.BatchCoverageWorkItemIDs)
			if err != nil || !reflect.DeepEqual(authority.Manifest, expected) {
				t.Fatal("current producer does not survive exact C3 codec", err)
			}
			// Real missing and altered structural facts must still fail closed.
			for _, kind := range []string{"observation", "file", "diagnostic", "invented_task"} {
				t.Run(kind, func(t *testing.T) {
					var bad runtimeSemanticAuthorityProjection
					if err := json.Unmarshal([]byte(p.PromptContext.Sections[3].Content), &bad); err != nil {
						t.Fatal(err)
					}
					w := semanticLocalWork(&bad, -1)
					if !purpose {
						w = semanticLocalWork(&bad, 0)
					}
					switch kind {
					case "observation":
						w.InputRevision.Observation = nil
					case "file":
						w.InputRevision.Observation.Files[0].Content += "altered"
					case "diagnostic":
						w.InputRevision.Observation.OriginalDiagnostic.OriginalMessage += "altered"
					case "invented_task":
						w.InputCoverageRevisionTasks = []runtimeInputCoverageRevisionTask{{RequiredCheckRef: "invented"}}
					}
					raw, err := json.Marshal(bad)
					if err != nil {
						t.Fatal(err)
					}
					prompt := *p.PromptContext
					prompt.Sections = append(prompt.Sections[:0:0], p.PromptContext.Sections...)
					prompt.Sections[3].Content = string(raw)
					if validateRuntimeSemanticContextIntegrity(r.SemanticContext, r.SemanticManifest, 0, nil, nil, p.ContextBudget, &prompt) == nil {
						t.Fatal("corrupt structural preimage passed integrity")
					}
				})
			}
			after, _ := json.Marshal(r)
			if !bytes.Equal(before, after) {
				t.Fatal("rendering mutated original owner transaction")
			}
			logical := "structural-frozen-" + string(r.SemanticContext.PlanStep)
			r.RequestIdentity.LogicalRequestID = logical
			e := rc.RuntimeExpectedDocumentContract{SchemaVersion: rc.RuntimeExpectedDocumentSchemaCurrent, DocumentKind: r.DocumentKind, PlanStep: r.SemanticContext.PlanStep, SemanticContextEncoding: rc.RuntimeSemanticContextEncoding, ManifestHash: r.SemanticManifest.Hash, DefinitionSeedHash: r.SemanticManifest.DefinitionSeedHash, LogicalRequestID: logical, StablePrefixHash: r.StablePrefixHash, MaxOutputTokens: r.SemanticContext.ProviderCeiling.EffectiveMaxOutputTokens, MaxRecoveryAttempts: 1}
			if !purpose {
				e.BatchID, e.BatchSequence, e.BatchCoverageWorkItemIDs = r.SemanticContext.BatchID, r.SemanticContext.BatchSequence, r.SemanticContext.BatchCoverageWorkItemIDs
				e.CoverageOrderHash = "accepted-order"
			}
			e.Hash = rc.StableRuntimeExpectedDocumentHash(e)
			r.ExpectedDocument = &e
			raw, ref, err := freezeRuntimeProviderInput(r, p)
			if err != nil {
				t.Fatal(err)
			}
			r.FrozenInput, r.FrozenInputRef = raw, ref
			messages, _ := json.Marshal(sectionMessages(p, false))
			r.SemanticManifest.ProofObligations[0].ImplementationWork.Scope.Request.InputRevision.OwnerObservation = nil
			for i := 0; i < 2; i++ {
				restored, err := runtimeProviderRequestForSubmission(r)
				if err != nil || !bytes.Equal(messages, responsesWireInputForRequest(&restored)) {
					t.Fatal("duplicate/recovery rerendered frozen structural input", err)
				}
			}
			foreign := r
			foreign.SemanticContext = r.SemanticContext.Clone()
			foreign.SemanticContext.WorkspaceID = "other-workspace"
			if _, err := runtimeProviderRequestForSubmission(foreign); err == nil {
				t.Fatal("foreign workspace consumed frozen structural request")
			}
		})
	}
}
