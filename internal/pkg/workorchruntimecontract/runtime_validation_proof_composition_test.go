package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func compositionFixture(t *testing.T) (RuntimeValidationProofRequirement, RuntimeValidationProofRequirementBinding, RuntimeValidationInputReview) {
	t.Helper()
	item := RuntimeCoverageItem{ID: "cwi-composite", SourceScope: "dodItem", SourceID: "original", TargetText: "CLI 输出 4；两轮工作；三方交付。"}
	rules := []RuntimeLineageAssertionRule{
		{Ref: "rounds", Kind: RuntimeLineageCompletedWorkRounds, ConsumptionPoint: RuntimeProofPhaseExecution, MinCompletedRounds: 2},
		{Ref: "delivery", Kind: RuntimeLineageDeliveryAcknowledgements, ConsumptionPoint: RuntimeProofPhasePostTerminalDelivery},
	}
	requirements, err := CompileRuntimeValidationProofRequirements("definition", []RuntimeCoverageItem{item}, []RuntimeDefinitionFocus{{
		ID: "focus", AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{EvidenceKindValidation},
		AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}, EvidenceDeclarationRef: CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID), LineageAssertions: rules,
	}})
	if err != nil || len(requirements) != 1 {
		t.Fatalf("compile: %v", err)
	}
	requirement := requirements[0]
	binding, err := RuntimeValidationProofRequirementBindingForRecipe(requirement, "recipe", RuntimeValidationCheckAttestationBinding{RequiredCheckRef: requirement.RequiredCheckRefs[0], ObservationProtocolRef: "owner/native", NativeCheckRef: "native"})
	if err != nil {
		t.Fatal(err)
	}
	content := "complete original input bytes"
	binding.InputReview = &RuntimeValidationInputReviewSubject{ProtocolRef: RuntimeValidationInputReviewProtocolV3, Files: []RuntimeValidationInputReviewFile{{TargetRef: "checks/input_test.go", Content: content, ContentHash: RuntimeValidationInputContentHash(content)}}, LineageAssertions: slices.Clone(requirement.LineageAssertions)}
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	report := RuntimeValidationInputReview{SubjectHash: binding.InputReview.Hash, Checks: []RuntimeValidationInputCheckReview{{RequiredCheckRef: binding.RequiredCheckRefs[0], Conclusion: RuntimeValidationInputConsistent, Reason: "joint review only; no execution credit", InputTargetRefs: []string{"checks/input_test.go"}, Obligations: []RuntimeValidationInputObligationReview{{CriterionText: item.TargetText, Finding: RuntimeValidationInputCovered, Reason: "actual input assertions AND independently consumed sealed rules", InputTargetRefs: []string{"checks/input_test.go"}, ProofConsumerRefs: RuntimeValidationProofConsumerRefs(*binding.InputReview)}}}}}
	return requirement, binding, report
}

func TestValidationProofCompositionExactMappingCloneReloadAndHash(t *testing.T) {
	requirement, binding, report := compositionFixture(t)
	if err := binding.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := report.ValidateAgainstBinding(binding); err != nil {
		t.Fatal(err)
	}
	if !runtimeValidationCompositionMatchesRequirement(binding.InputReview, requirement) {
		t.Fatal("original source lost")
	}
	original, _ := json.Marshal(report)
	var loaded RuntimeValidationInputReview
	if err := json.Unmarshal(original, &loaded); err != nil || loaded.ValidateAgainstBinding(binding) != nil || !reflect.DeepEqual(loaded, report) {
		t.Fatal("mapping reload drift")
	}
	clone := report.Clone()
	clone.Checks[0].Obligations[0].ProofConsumerRefs[0] = "changed"
	if reflect.DeepEqual(clone, report) || StableRuntimeValidationInputReviewHash(clone) == StableRuntimeValidationInputReviewHash(report) {
		t.Fatal("mapping aliases or is unhashed")
	}
	clonedRequirement := requirement.Clone()
	clonedRequirement.LineageAssertions[0].Rule.MinCompletedRounds++
	if clonedRequirement.Validate() == nil || StableRuntimeValidationProofRequirementHash(clonedRequirement) == requirement.Hash {
		t.Fatal("requirement ignores declaration")
	}
	feedback, err := ProjectRuntimeValidationInputReviewFeedback("runtime-input-review-test", binding, report)
	if err != nil || feedback.Validate() != nil {
		t.Fatalf("feedback: %v", err)
	}
	feedback.Report.Checks[0].Obligations[0].ProofConsumerRefs[0] = "changed"
	if feedback.Validate() == nil {
		t.Fatal("feedback substituted mapping")
	}
}

func TestValidationProofCompositionRejectsMissingForeignAndLegacyMappings(t *testing.T) {
	_, binding, report := compositionFixture(t)
	before := report.Clone()
	for name, refs := range map[string][]string{
		"missing": nil, "lineage instead of behavior": {"delivery", "rounds"}, "missing required guard": {RuntimeValidationInputConsumer},
		"foreign": {"foreign", RuntimeValidationInputConsumer}, "duplicate": {"delivery", "delivery", "rounds", RuntimeValidationInputConsumer}, "noncanonical": {RuntimeValidationInputConsumer, "rounds", "delivery"},
	} {
		t.Run(name, func(t *testing.T) {
			changed := report.Clone()
			changed.Checks[0].Obligations[0].ProofConsumerRefs = refs
			if changed.ValidateAgainstBinding(binding) == nil {
				t.Fatal("invalid mapping accepted")
			}
		})
	}
	for _, protocol := range []string{RuntimeValidationInputReviewProtocol, RuntimeValidationInputReviewProtocolV2} {
		changed := binding.Clone()
		changed.InputReview.ProtocolRef = protocol
		changed.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*changed.InputReview, changed)
		if changed.Validate() == nil {
			t.Fatal("legacy subject accepts composition declarations")
		}
		changed.InputReview.LineageAssertions = nil
		changed.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*changed.InputReview, changed)
		bad := report.Clone()
		bad.SubjectHash = changed.InputReview.Hash
		if bad.ValidateAgainstBinding(changed) == nil {
			t.Fatal("legacy report accepts composition extension")
		}
	}
	if !reflect.DeepEqual(before, report) {
		t.Fatal("validation mutated original")
	}
}

func TestValidationProofCompositionRejectsRehashedOwnerSourceSubstitution(t *testing.T) {
	requirement, binding, _ := compositionFixture(t)
	for _, mutation := range []func(*RuntimeValidationInputReviewSubject){
		func(s *RuntimeValidationInputReviewSubject) { s.LineageAssertions = nil },
		func(s *RuntimeValidationInputReviewSubject) {
			s.LineageAssertions[0].Rule.MinCompletedRounds = 1
			s.LineageAssertions[0].RuleHash = StableRuntimeLineageAssertionRuleHash(s.LineageAssertions[0].Rule)
		},
		func(s *RuntimeValidationInputReviewSubject) {
			s.LineageAssertions[0].SourceTextHash = RuntimeLineageSourceTextHash("replacement")
		},
	} {
		changed := binding.Clone()
		mutation(changed.InputReview)
		changed.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*changed.InputReview, changed)
		if changed.Validate() != nil {
			t.Fatal("substitution must be internally self-consistent to exercise reverse proof")
		}
		surface := RuntimeValidationSurface{ID: binding.ValidationRecipeID, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession, ProofApplicability: RuntimeValidationProofDirect, ProofRequirementBindings: []RuntimeValidationProofRequirementBinding{changed}}
		if _, _, found := RuntimeValidationSurfaceDeclaringProofRequirement([]RuntimeValidationSurface{surface}, requirement); found {
			t.Fatal("rehashed owner declarations replaced original Definition")
		}
	}
}

func TestValidationProofCompositionV3ClosedSemanticWireAndNegativeObservations(t *testing.T) {
	manifest, document, binding := semanticInputReviewV2Fixture(t)
	binding.InputReview.ProtocolRef = RuntimeValidationInputReviewProtocolV3
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	for i := range manifest.OwnerCatalogs {
		catalog := &manifest.OwnerCatalogs[i]
		if catalog.OwnerModuleID != RuntimeModuleShell {
			continue
		}
		catalog.ValidationSurfaces[0].ProofRequirementBindings[0] = binding.Clone()
		catalog.ValidationRecipes[0].InputReviewProtocolRef = RuntimeValidationInputReviewProtocolV3
		catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	document.ManifestHash = manifest.Hash
	report := document.Proposals[1].ValidationInputReview
	report.SubjectHash = binding.InputReview.Hash
	for i := range report.Checks {
		report.Checks[i].Obligations[0].ProofConsumerRefs = []string{RuntimeValidationInputConsumer}
	}
	payload, err := EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, payload)
	if err != nil || !reflect.DeepEqual(decoded.Normalized(), document.Normalized()) {
		t.Fatalf("v3 wire: %v", err)
	}
	for _, finding := range []string{RuntimeValidationInputMissing, RuntimeValidationInputConflicting, RuntimeValidationInputUnknown} {
		changed := cloneRuntimeSemanticPlanDocumentRaw(document)
		check := &changed.Proposals[1].ValidationInputReview.Checks[0]
		check.Obligations[0].Finding = finding
		check.Conclusion = RuntimeValidationInputConflict
		if finding == RuntimeValidationInputUnknown {
			check.Conclusion = RuntimeValidationInputInconclusive
		}
		if err := changed.ValidateAgainstManifest(manifest); err != nil {
			t.Fatal(err)
		}
		if ValidateRuntimeSemanticInputReviewConsistency(changed) == nil {
			t.Fatal("negative composition review acquired proof authority")
		}
	}
}

func TestValidationProofCompositionLegacyWireRejectsExtensionIncludingNull(t *testing.T) {
	manifest, document, _ := semanticInputReviewV2Fixture(t)
	payload, err := EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"null", "[]", `["validation_input"]`, `"validation_input"`, `[123]`} {
		t.Run(value, func(t *testing.T) {
			changed := strings.Replace(string(payload), `"criterionText":`, `"proofConsumerRefs":`+value+`,"criterionText":`, 1)
			if changed == string(payload) {
				t.Fatal("fixture has no obligation mapping")
			}
			if _, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, []byte(changed)); err == nil {
				t.Fatal("legacy wire accepted composition extension")
			}
		})
	}
}

func TestValidationProofCompositionPreservesUndeclaredRequirementIdentity(t *testing.T) {
	requirement, _, _ := compositionFixture(t)
	requirement.LineageAssertions = nil
	// The original encoding, independent of the composition hash branch.
	h := sha256.New()
	writeHashString(h, RuntimeValidationProofRequirementSchemaCurrent)
	writeHashString(h, requirement.DefinitionRevision)
	writeHashString(h, string(requirement.CoverageWorkItemID))
	writeHashString(h, requirement.SourceScope)
	writeHashString(h, requirement.SourceID)
	writeHashString(h, requirement.SourceTextHash)
	writeHashString(h, requirement.DeclarationRef)
	writeHashString(h, requirement.ValidationRecipeID)
	writeHashStrings(h, requirement.RequiredCheckRefs)
	want := RuntimeValidationProofRequirementHashPrefix + hex.EncodeToString(h.Sum(nil))[:20]
	for _, assertions := range [][]RuntimeLineageAssertion{nil, {}} {
		requirement.LineageAssertions = assertions
		body, err := json.Marshal(requirement)
		if err != nil || strings.Contains(string(body), "LineageAssertions") || StableRuntimeValidationProofRequirementHash(requirement) != want {
			t.Fatal("absent composition changed legacy JSON/hash")
		}
	}
}

func TestValidationProofCompositionNormalizedConsumersRejectSourceSubstitution(t *testing.T) {
	requirement, binding, _ := compositionFixture(t)
	surface := RuntimeValidationSurface{ID: binding.ValidationRecipeID, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession, Readiness: RuntimeValidationSurfaceReady, ProofApplicability: RuntimeValidationProofDirect, ProofRequirementBindings: []RuntimeValidationProofRequirementBinding{binding}}
	if !runtimeValidationSurfacesDeclareNormalizedProofRequirement([]RuntimeValidationSurface{surface}, requirement) {
		t.Fatal("original composition did not match normalized consumer")
	}
	if _, found := runtimeValidationSurfaceProofRequirementBindingNormalized(surface, requirement); !found {
		t.Fatal("original composition did not match binding compiler")
	}
	// Internally valid subject/hash; only the original requirement exposes the
	// erased declaration. Both optimized semantic consumer paths must reject.
	surface.ProofRequirementBindings[0] = binding.Clone()
	changed := &surface.ProofRequirementBindings[0]
	changed.InputReview.LineageAssertions = nil
	changed.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*changed.InputReview, *changed)
	if changed.Validate() != nil {
		t.Fatal("fixture substitution is not self-consistent")
	}
	if runtimeValidationSurfacesDeclareNormalizedProofRequirement([]RuntimeValidationSurface{surface}, requirement) {
		t.Fatal("normalized manifest consumer omitted source reverse proof")
	}
	if _, found := runtimeValidationSurfaceProofRequirementBindingNormalized(surface, requirement); found {
		t.Fatal("normalized binding compiler omitted source reverse proof")
	}
}

func TestValidationProofCompositionRejectsUnsafeAndReservedConsumerRefs(t *testing.T) {
	_, binding, _ := compositionFixture(t)
	for _, ref := range []string{RuntimeValidationInputConsumer, "unsafe\nref", "unsafe\x00ref", "invalid-\xff"} {
		changed := binding.Clone()
		assertion := &changed.InputReview.LineageAssertions[0]
		assertion.Rule.Ref = ref
		assertion.RuleHash = StableRuntimeLineageAssertionRuleHash(assertion.Rule)
		changed.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*changed.InputReview, changed)
		if changed.Validate() == nil {
			t.Fatalf("unsafe/reserved composition consumer %q accepted", ref)
		}
	}
}

func TestValidationProofCompositionManifestRejectsRehashedRequirementReplacement(t *testing.T) {
	manifest, _, binding := semanticInputReviewV2Fixture(t)
	for i := range manifest.ProofObligations {
		obligation := &manifest.ProofObligations[i]
		if obligation.ValidationProofRequirement == nil {
			continue
		}
		obligation.LineageAssertions = []RuntimeLineageAssertionRule{{Ref: "rounds", Kind: RuntimeLineageCompletedWorkRounds, ConsumptionPoint: RuntimeProofPhaseExecution, MinCompletedRounds: 2}}
		for _, item := range manifest.CoverageItems {
			if item.ID == obligation.CoverageWorkItemID {
				var err error
				obligation.ValidationProofRequirement.LineageAssertions, err = CompileRuntimeLineageAssertions(obligation.LineageAssertions, obligation.DeclarationRef, item)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		obligation.ValidationProofRequirement.Hash = StableRuntimeValidationProofRequirementHash(*obligation.ValidationProofRequirement)
		binding.RequirementHash = obligation.ValidationProofRequirement.Hash
		binding.InputReview.ProtocolRef = RuntimeValidationInputReviewProtocolV3
		binding.InputReview.LineageAssertions = slices.Clone(obligation.ValidationProofRequirement.LineageAssertions)
		binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	}
	for i := range manifest.OwnerCatalogs {
		catalog := &manifest.OwnerCatalogs[i]
		if catalog.OwnerModuleID == RuntimeModuleShell {
			catalog.ValidationSurfaces[0].ProofRequirementBindings[0] = binding.Clone()
			catalog.ValidationRecipes[0].InputReviewProtocolRef = RuntimeValidationInputReviewProtocolV3
			catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
		}
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatalf("original composed manifest: %v", err)
	}
	for _, remove := range []bool{false, true} {
		changed := manifest.Clone()
		for i := range changed.ProofObligations {
			requirement := changed.ProofObligations[i].ValidationProofRequirement
			if requirement == nil {
				continue
			}
			if remove {
				requirement.LineageAssertions = nil
			} else {
				requirement.LineageAssertions[0].Rule.MinCompletedRounds = 1
				requirement.LineageAssertions[0].RuleHash = StableRuntimeLineageAssertionRuleHash(requirement.LineageAssertions[0].Rule)
			}
			requirement.Hash = StableRuntimeValidationProofRequirementHash(*requirement)
			if requirement.Validate() != nil {
				t.Fatal("rehashed requirement is not internally valid")
			}
			for j := range changed.OwnerCatalogs {
				catalog := &changed.OwnerCatalogs[j]
				if catalog.OwnerModuleID != RuntimeModuleShell {
					continue
				}
				b := &catalog.ValidationSurfaces[0].ProofRequirementBindings[0]
				b.RequirementHash = requirement.Hash
				b.InputReview.LineageAssertions = slices.Clone(requirement.LineageAssertions)
				b.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*b.InputReview, *b)
				catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
				if catalog.Validate() != nil {
					t.Fatal("rehashed owner catalog is not internally valid")
				}
			}
		}
		changed.Hash = StableRuntimeSemanticPlanManifestHash(changed)
		if changed.Validate() == nil {
			t.Fatal("coordinated owner/requirement replacement erased original obligation")
		}
	}
}
