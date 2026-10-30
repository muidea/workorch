package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func inputRevisionProviderFixture(t *testing.T) *runtimecontract.RuntimeValidationInputRevision {
	request := runtimeInputReviewProviderFixture(t)
	binding := request.SemanticManifest.OwnerCatalogs[0].ValidationSurfaces[0].ProofRequirementBindings[0]
	report := runtimecontract.RuntimeValidationInputReview{SubjectHash: binding.InputReview.Hash}
	for _, check := range binding.RequiredCheckRefs {
		report.Checks = append(report.Checks, runtimecontract.RuntimeValidationInputCheckReview{RequiredCheckRef: check, Conclusion: runtimecontract.RuntimeValidationInputConflict, Reason: "The input omits the original required assertion.", InputTargetRefs: []string{binding.InputReview.Files[0].TargetRef}})
	}
	v := &runtimecontract.RuntimeValidationInputRevision{WorkspaceID: request.SemanticContext.WorkspaceID, RunID: request.SemanticContext.RunID, DefinitionSeedHash: request.SemanticContext.DefinitionSeed.Hash, SourceManifestHash: "original-manifest", SourceLogicalRequestID: "original-request", SourceDocumentHash: "original-rejected-report", OwnerModuleID: runtimecontract.RuntimeModuleFile, Binding: binding, Report: report, InputTargetRefs: []string{binding.InputReview.Files[0].TargetRef}}
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRuntimeInputRevisionCompleteC3StablePrefixAndClosedPurpose(t *testing.T) {
	for _, purpose := range []bool{true, false} {
		t.Run(map[bool]string{true: "purpose", false: "work"}[purpose], func(t *testing.T) {
			v := inputRevisionProviderFixture(t)
			request := runtimeSemanticWorkProviderRequestForTest(t, purpose, v)
			provider, err := runtimeSemanticPlanProviderRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			var authority runtimeSemanticAuthorityProjection
			if err := json.Unmarshal([]byte(provider.PromptContext.Sections[3].Content), &authority); err != nil {
				t.Fatal(err)
			}
			projected := authority.Manifest.ImplementationPurpose
			if !purpose {
				projected = authority.Manifest.ProofObligations[0].ImplementationWork
			}
			if projected == nil || projected.InputRevision == nil || projected.TerminalEvidenceCredit || !reflect.DeepEqual(projected.InputRevision, request.SemanticManifest.ProofObligations[0].ImplementationWork.Scope.Request.InputRevision) {
				t.Fatal("C3 lost exact original input/review or claimed credit")
			}
			if purpose && strings.Contains(string(provider.Schema), `"terminal_proof"`) {
				t.Fatal("unchanged re-review can bypass required real effect")
			}
			v.Report.Checks[0].Reason = "The exact original input still lacks its behavioral assertion."
			changed := runtimeSemanticWorkProviderRequestForTest(t, purpose, v)
			after, err := runtimeSemanticPlanProviderRequest(changed)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				if provider.PromptContext.Sections[i].Content != after.PromptContext.Sections[i].Content {
					t.Fatal("volatile review drifted stable C0-C2")
				}
			}
			if provider.PromptContext.Sections[3].Content == after.PromptContext.Sections[3].Content {
				t.Fatal("fresh findings omitted from C3")
			}
			projected.InputRevision.Report.Checks[0].Reason = "mutated projection"
			if request.SemanticManifest.ProofObligations[0].ImplementationWork.Scope.Request.InputRevision.Report.Checks[0].Reason == "mutated projection" {
				t.Fatal("provider projection aliases owner authority")
			}
		})
	}
}

func TestRuntimeInputRevisionOperationsReceiveFullOriginalInputAndFeedback(t *testing.T) {
	v := inputRevisionProviderFixture(t)
	projection := runtimeCandidateOperationContractsForManifest([]runtimecontract.RuntimeCandidateOperationManifest{{Kind: runtimecontract.StepKindFileEdit, InputRevision: v}}, runtimecontract.RuntimeCandidateWireContract{})
	raw, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"inputRevision", v.SourceDocumentHash, v.Report.Checks[0].Reason, v.Binding.InputReview.Files[0].ContentHash, "fresh independent input review", "no validation credit"} {
		if !strings.Contains(string(raw), value) {
			t.Fatalf("operations C3 omitted %s", value)
		}
	}
	projection[0].InputRevision.Binding.InputReview.Files[0].Content = "mutated"
	if v.Binding.InputReview.Files[0].Content == "mutated" {
		t.Fatal("operation projection aliases raw input")
	}
}
