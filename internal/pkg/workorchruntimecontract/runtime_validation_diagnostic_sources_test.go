package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func diagnosticSourceReviewFixture(t *testing.T) (RuntimeValidationProofRequirementBinding, RuntimeValidationInputReview) {
	t.Helper()
	_, document, _, binding := semanticInputReviewFixture(t)
	binding.InputReview.ProtocolRef = RuntimeValidationInputReviewProtocolV3
	binding.InputReview.DiagnosticSourceContractRef = RuntimeValidationDiagnosticSourceReviewV1
	binding.InputReview.DiagnosticSources = []RuntimeValidationDiagnosticSource{{SourceRef: "source-a", NativeCaseRef: "owner-native-a", InputTargetRef: binding.InputReview.Files[0].TargetRef}, {SourceRef: "source-b", NativeCaseRef: "owner-native-b", InputTargetRef: binding.InputReview.Files[0].TargetRef}}
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	report := document.Proposals[1].ValidationInputReview.Clone()
	report.SubjectHash = binding.InputReview.Hash
	for i := range report.Checks {
		report.Checks[i].Obligations = []RuntimeValidationInputObligationReview{{CriterionText: "original", Finding: RuntimeValidationInputCovered, Reason: "original behavior checked", InputTargetRefs: report.Checks[i].InputTargetRefs, ProofConsumerRefs: []string{RuntimeValidationInputConsumer}}}
	}
	for _, source := range binding.InputReview.DiagnosticSources {
		report.DiagnosticSources = append(report.DiagnosticSources, RuntimeValidationDiagnosticSourceReview{SourceRef: source.SourceRef, Conclusion: RuntimeValidationInputConsistent, Reason: "Independently reviewed compatible original project assertion", InputTargetRefs: []string{source.InputTargetRef}})
	}
	if err := report.ValidateAgainstBinding(binding); err != nil {
		t.Fatal(err)
	}
	return binding, report
}

func TestDiagnosticSourceReviewIsCompleteIndependentAndVersioned(t *testing.T) {
	binding, report := diagnosticSourceReviewFixture(t)
	for name, mutate := range map[string]func(*RuntimeValidationInputReview){"missing": func(r *RuntimeValidationInputReview) { r.DiagnosticSources = nil }, "partial": func(r *RuntimeValidationInputReview) { r.DiagnosticSources = r.DiagnosticSources[:1] }, "duplicate": func(r *RuntimeValidationInputReview) { r.DiagnosticSources[1] = r.DiagnosticSources[0] }, "unknown": func(r *RuntimeValidationInputReview) { r.DiagnosticSources[0].SourceRef = "other" }, "missing input": func(r *RuntimeValidationInputReview) { r.DiagnosticSources[0].InputTargetRefs = nil }, "empty reason": func(r *RuntimeValidationInputReview) { r.DiagnosticSources[0].Reason = " " }, "unknown input": func(r *RuntimeValidationInputReview) { r.DiagnosticSources[0].InputTargetRefs = []string{"missing"} }} {
		t.Run(name, func(t *testing.T) {
			changed := report.Clone()
			mutate(&changed)
			if changed.ValidateAgainstBinding(binding) == nil {
				t.Fatal("incomplete independent review accepted")
			}
		})
	}
	for _, conclusion := range []string{RuntimeValidationInputConflict, RuntimeValidationInputInconclusive} {
		changed := report.Clone()
		changed.DiagnosticSources[0].Conclusion = conclusion
		if changed.ValidateAgainstBinding(binding) != nil || RuntimeValidationInputReviewConclusion(changed) != conclusion {
			t.Fatal("neighbor conclusion hidden by consistent original checks")
		}
	}
	legacy := binding.Clone()
	legacy.InputReview.DiagnosticSourceContractRef = ""
	legacy.InputReview.DiagnosticSources = nil
	legacy.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*legacy.InputReview, legacy)
	changed := report.Clone()
	changed.SubjectHash = legacy.InputReview.Hash
	if changed.ValidateAgainstBinding(legacy) == nil {
		t.Fatal("legacy source extension accepted")
	}
	for _, version := range []string{RuntimeValidationInputReviewProtocol, RuntimeValidationInputReviewProtocolV2} {
		changed := binding.Clone()
		changed.InputReview.ProtocolRef = version
		changed.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*changed.InputReview, changed)
		if changed.InputReview.ValidateAgainstBinding(changed) == nil {
			t.Fatal("old protocol silently upgraded")
		}
	}
}

func TestDiagnosticSourceReviewHashReloadAndDetachedCopies(t *testing.T) {
	binding, report := diagnosticSourceReviewFixture(t)
	bytes, _ := json.Marshal(binding)
	var reloaded RuntimeValidationProofRequirementBinding
	if json.Unmarshal(bytes, &reloaded) != nil || reloaded.InputReview.ValidateAgainstBinding(reloaded) != nil || !reflect.DeepEqual(reloaded, binding) {
		t.Fatal("reload lost source inventory")
	}
	changed := binding.Clone()
	changed.InputReview.DiagnosticSources[0].NativeCaseRef = "substituted"
	if changed.InputReview.Hash == StableRuntimeValidationInputReviewSubjectHash(*changed.InputReview, changed) || changed.InputReview.ValidateAgainstBinding(changed) == nil {
		t.Fatal("source identity drift accepted")
	}
	copy := report.Clone()
	copy.DiagnosticSources[0].InputTargetRefs[0] = "replacement"
	if report.DiagnosticSources[0].InputTargetRefs[0] == "replacement" || StableRuntimeValidationInputReviewHash(copy) == StableRuntimeValidationInputReviewHash(report) {
		t.Fatal("source review was not detached or hash bound")
	}
	raw, _ := json.Marshal(report)
	if err := validateRuntimeValidationInputReviewPayload("review", raw); err != nil {
		t.Fatal(err)
	}
	for _, replace := range []string{`"diagnosticSources":null,"ignored":1,`, `"diagnosticSources":[],"ignored":1,`} {
		changed := strings.Replace(string(raw), `"subjectHash":`, replace+`"subjectHash":`, 1)
		if validateRuntimeValidationInputReviewPayload("review", []byte(changed)) == nil {
			t.Fatal("invalid source wire accepted")
		}
	}
}

func TestDiagnosticExecutionReverseProvesOriginalSourceInventory(t *testing.T) {
	binding, _ := diagnosticSourceReviewFixture(t)
	observation := diagnosticExecutionFixture()
	c := observation.ExecutionConjunction
	observation.RecipeID = binding.ValidationRecipeID
	c.Stages[1].DiagnosticSourceRef = binding.InputReview.DiagnosticSources[0].SourceRef
	for i := range binding.CheckAttestations {
		binding.CheckAttestations[i].ObservationProtocolRef = c.ProtocolRef
		binding.CheckAttestations[i].ExecutionStagesHash = c.RequiredStagesHash
	}
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	scope := []string{"implementation"}
	if err := ValidateRuntimeValidationDiagnosticExecution(observation, binding, scope); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unknown-source", "missing-source", "extra-union", "wrong-scope", "wrong-recipe", "wrong-protocol", "wrong-stages", "legacy"} {
		t.Run(name, func(t *testing.T) {
			changed, bound := observation.Clone(), binding.Clone()
			targets := slices.Clone(scope)
			switch name {
			case "unknown-source":
				changed.ExecutionConjunction.Stages[1].DiagnosticSourceRef = "not-sealed"
			case "missing-source":
				bound.InputReview.DiagnosticSources = bound.InputReview.DiagnosticSources[1:]
				bound.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*bound.InputReview, bound)
			case "extra-union":
				changed.ExecutionConjunction.BehaviorTargetRefs = []string{"another", "implementation"}
			case "wrong-scope":
				targets = []string{"another"}
			case "wrong-recipe":
				changed.RecipeID = "another"
			case "wrong-protocol":
				changed.ObservationProtocolRef = "another"
			case "wrong-stages":
				changed.ExecutionConjunction.RequiredStagesHash = StableRuntimeValidationExecutionStagesHash(nil)
			case "legacy":
				bound.InputReview = nil
			}
			if ValidateRuntimeValidationDiagnosticExecution(changed, bound, targets) == nil {
				t.Fatal("diagnostic authority escaped original immutable binding")
			}
		})
	}
}

func TestDiagnosticSourceEnvelopeCannotUpgradeLegacyOrTruncate(t *testing.T) {
	binding, report := diagnosticSourceReviewFixture(t)
	for len(binding.InputReview.Files) < RuntimeValidationInputReviewMaxFiles+1 {
		ref := fmt.Sprintf("checks/source-%02d.txt", len(binding.InputReview.Files))
		binding.InputReview.Files = append(binding.InputReview.Files, RuntimeValidationInputReviewFile{TargetRef: ref, Content: "complete input", ContentHash: RuntimeValidationInputContentHash("complete input")})
	}
	slices.SortFunc(binding.InputReview.Files, func(a, b RuntimeValidationInputReviewFile) int { return strings.Compare(a.TargetRef, b.TargetRef) })
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	report.SubjectHash = binding.InputReview.Hash
	if report.ValidateAgainstBinding(binding) != nil {
		t.Fatal("explicit finite project envelope rejected")
	}
	legacy := binding.Clone()
	legacy.InputReview.DiagnosticSourceContractRef = ""
	legacy.InputReview.DiagnosticSources = nil
	legacy.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*legacy.InputReview, legacy)
	if legacy.InputReview.ValidateAgainstBinding(legacy) == nil {
		t.Fatal("legacy input envelope silently widened")
	}
	changed := binding.Clone()
	changed.InputReview.Files[0].Content = strings.Repeat("x", RuntimeValidationDiagnosticSourceMaxBytes+1)
	changed.InputReview.Files[0].ContentHash = RuntimeValidationInputContentHash(changed.InputReview.Files[0].Content)
	changed.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*changed.InputReview, changed)
	if changed.InputReview.ValidateAgainstBinding(changed) == nil {
		t.Fatal("over-capacity full input accepted")
	}
}

func TestDiagnosticFailureCannotEraseAttributionOrBorrowRepairTargets(t *testing.T) {
	binding, _ := diagnosticSourceReviewFixture(t)
	original := diagnosticExecutionFixture()
	for i := range original.ExecutionConjunction.Stages {
		original.ExecutionConjunction.Stages[i].Stdout.Storage = &RuntimeValidationOutputStorage{Reference: "owner-stdout"}
		original.ExecutionConjunction.Stages[i].Stderr.Storage = &RuntimeValidationOutputStorage{Reference: "owner-stderr"}
	}
	original.RecipeID = binding.ValidationRecipeID
	original.ExecutionConjunction.Stages[1].DiagnosticSourceRef = binding.InputReview.DiagnosticSources[0].SourceRef
	for i := range binding.CheckAttestations {
		binding.CheckAttestations[i].ObservationProtocolRef = original.ObservationProtocolRef
		binding.CheckAttestations[i].ExecutionStagesHash = original.ExecutionConjunction.RequiredStagesHash
	}
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	failure := RuntimeValidationFailureObservation{CandidateTargetRefs: []string{"implementation"}}
	scope := []string{"implementation"}
	if err := ValidateRuntimeValidationDiagnosticFailureObservation(&original, binding, scope, &failure); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"erase-fields", "erase-union", "erase-conjunction", "erase-observation", "borrow-target", "extra-target", "credit"} {
		t.Run(name, func(t *testing.T) {
			changed, candidate := original.Clone(), failure.Clone()
			observation := &changed
			switch name {
			case "erase-fields", "erase-union":
				for i := range changed.ExecutionConjunction.Stages {
					changed.ExecutionConjunction.Stages[i].DiagnosticSourceRef = ""
					changed.ExecutionConjunction.Stages[i].BehaviorTargetRefs = nil
				}
				if name == "erase-union" {
					changed.ExecutionConjunction.BehaviorTargetRefs = nil
				}
			case "erase-conjunction":
				changed.ExecutionConjunction = nil
			case "erase-observation":
				observation = nil
			case "borrow-target":
				candidate.CandidateTargetRefs = []string{"another"}
			case "extra-target":
				candidate.CandidateTargetRefs = []string{"another", "implementation"}
			case "credit":
				changed.ExecutionConjunction.Complete = true
			}
			if err := ValidateRuntimeValidationDiagnosticFailureObservation(observation, binding, scope, &candidate); err == nil {
				t.Fatal("repair authority survived source attribution downgrade")
			}
		})
	}
	unobserved := original.Clone()
	for i := range unobserved.ExecutionConjunction.Stages {
		unobserved.ExecutionConjunction.Stages[i].DiagnosticSourceRef = ""
		unobserved.ExecutionConjunction.Stages[i].BehaviorTargetRefs = nil
	}
	unobserved.ExecutionConjunction.BehaviorTargetRefs = nil
	if err := ValidateRuntimeValidationDiagnosticFailureObservation(&unobserved, binding, scope, nil); err != nil {
		t.Fatal("unobserved failure could not be preserved without repair authority", err)
	}
	// Successful ordinary behavior observed before a later storage/deadline
	// failure is also preservable, without becoming a repair-target union.
	unobserved.ExecutionConjunction.BehaviorTargetRefs = []string{"implementation"}
	if err := ValidateRuntimeValidationDiagnosticFailureObservation(&unobserved, binding, scope, nil); err != nil {
		t.Fatal("ordinary partial behavior could not be preserved", err)
	}
	if err := ValidateRuntimeValidationDiagnosticFailureObservation(&unobserved, binding, scope, &failure); err == nil {
		t.Fatal("ordinary partial behavior became source repair authority")
	}
	legacy := binding.Clone()
	legacy.InputReview = nil
	if err := ValidateRuntimeValidationDiagnosticFailureObservation(nil, legacy, scope, &failure); err != nil {
		t.Fatal("legacy failure was implicitly upgraded", err)
	}
}

func TestDiagnosticStorageOmissionPreservesHistoryButNeverRepairAuthority(t *testing.T) {
	binding, _ := diagnosticSourceReviewFixture(t)
	original := diagnosticExecutionFixture()
	original.RecipeID = binding.ValidationRecipeID
	original.ExecutionConjunction.Stages[1].DiagnosticSourceRef = binding.InputReview.DiagnosticSources[0].SourceRef
	for i := range binding.CheckAttestations {
		binding.CheckAttestations[i].ObservationProtocolRef = original.ObservationProtocolRef
		binding.CheckAttestations[i].ExecutionStagesHash = original.ExecutionConjunction.RequiredStagesHash
	}
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	failure := RuntimeValidationFailureObservation{CandidateTargetRefs: []string{"implementation"}}
	for _, missing := range []int{0, 1} {
		for _, stream := range []string{"stdout", "stderr", "all"} {
			t.Run(fmt.Sprintf("stage-%d/%s", missing, stream), func(t *testing.T) {
				changed := original.Clone()
				for i := range changed.ExecutionConjunction.Stages {
					changed.ExecutionConjunction.Stages[i].Stdout.Storage = &RuntimeValidationOutputStorage{Reference: "owner-stdout"}
					changed.ExecutionConjunction.Stages[i].Stderr.Storage = &RuntimeValidationOutputStorage{Reference: "owner-stderr"}
				}
				stage := &changed.ExecutionConjunction.Stages[missing]
				if stream == "stdout" {
					stage.Stdout.Storage = &RuntimeValidationOutputStorage{OmittedReason: "actual-publication-failed"}
				} else if stream == "stderr" {
					stage.Stderr.Storage = &RuntimeValidationOutputStorage{OmittedReason: "actual-operation-deadline"}
				} else {
					for i := range changed.ExecutionConjunction.Stages {
						changed.ExecutionConjunction.Stages[i].Stdout.Storage = nil
						changed.ExecutionConjunction.Stages[i].Stderr.Storage = nil
					}
				}
				body, err := json.Marshal(changed)
				var reloaded RuntimeValidationObservation
				if err != nil || json.Unmarshal(body, &reloaded) != nil || !reflect.DeepEqual(reloaded, changed) {
					t.Fatal("historical output identity changed on reload")
				}
				if err := ValidateRuntimeValidationDiagnosticFailureObservation(&reloaded, binding, []string{"implementation"}, nil); err != nil {
					t.Fatal("actual attribution history was lost on publication failure", err)
				}
				if err := ValidateRuntimeValidationDiagnosticFailureObservation(&reloaded, binding, []string{"implementation"}, &failure); err == nil {
					t.Fatal("omitted original output acquired repair authority")
				}
			})
		}
	}
}
