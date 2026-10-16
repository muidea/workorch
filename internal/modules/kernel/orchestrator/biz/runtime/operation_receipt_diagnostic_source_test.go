package runtime

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Commit and fresh-load deliberately use the same original-dispatch consumer.
// This fixture has no native syntax: Core compares opaque sealed identities.
func TestDiagnosticReceiptCannotDowngradeOriginalBindingAtIngressOrReload(t *testing.T) {
	dispatch, operation, catalog, binding := runtimeValidationExecutionBindingFixture(t)
	identities := []runtimecontract.RuntimeValidationExecutionStageIdentity{{ID: "project", Scope: "complete"}, {ID: "source", Scope: "independent"}}
	stagesHash := runtimecontract.StableRuntimeValidationExecutionStagesHash(identities)
	for i := range binding.CheckAttestations {
		binding.CheckAttestations[i].ExecutionStagesHash = stagesHash
	}
	binding.InputReview = &runtimecontract.RuntimeValidationInputReviewSubject{
		ProtocolRef:                 runtimecontract.RuntimeValidationInputReviewProtocolV3,
		Files:                       []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "check.go", Content: "original", ContentHash: runtimecontract.RuntimeValidationInputContentHash("original")}},
		DiagnosticSourceContractRef: runtimecontract.RuntimeValidationDiagnosticSourceReviewV1,
		DiagnosticSources:           []runtimecontract.RuntimeValidationDiagnosticSource{{SourceRef: "source-a", NativeCaseRef: "owner/native-a", InputTargetRef: "check.go"}},
	}
	binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	dispatch.CoverageBindings[0].Proofs[0].ValidationCheckAttestations = runtimecontract.CloneRuntimeValidationCheckAttestationBindings(binding.CheckAttestations)
	dispatch.CoverageBindings[0].Hash = runtimecontract.StableCoverageEvidenceBindingHash(dispatch.CoverageBindings[0])
	catalog.TargetRefs = []string{"implementation.go", "validation_target.go"}
	catalog.ValidationRecipes[0].InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocolV3
	catalog.ValidationRecipes[0].AdapterRef = "owner/opaque-review"
	catalog.ValidationSurfaces[0].ProofRequirementBindings[0] = binding
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal("source catalog fixture invalid", err)
	}
	ownerObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: catalog.OwnerModuleID, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: catalog.OwnerModuleID, SubjectRef: "owner-policy", Revision: catalog.ObservedRevision}},
	}
	ownerObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(ownerObservation)
	dispatch.Transaction.AuthoritySnapshotID = "authority-source"
	dispatch.Candidate = runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: operation}}}}
	dispatch.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	dispatch.OperationAdmissibilityObservations = []runtimecontract.RuntimePrerequisiteObservation{ownerObservation}
	dispatch.OperationExecutions = []runtimecontract.RuntimeOperationExecutionRecord{{OperationID: operation.ID, AdmissibilitySnapshotID: catalog.SnapshotID}}
	aggregate := RuntimeAggregate{Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-source", RunID: "run-source"}, ActiveDispatch: &dispatch}
	output := runtimecontract.RuntimeValidationOutputIdentity{SHA256: runtimecontract.RuntimeValidationInputContentHash(""), Complete: true, Storage: &runtimecontract.RuntimeValidationOutputStorage{Reference: "owner-original-stream"}}
	conjunction := runtimecontract.RuntimeValidationExecutionConjunction{ProtocolRef: binding.CheckAttestations[0].ObservationProtocolRef, RequiredStagesHash: stagesHash, BehaviorTargetRefs: []string{"implementation.go"}}
	for i, identity := range identities {
		conjunction.Stages = append(conjunction.Stages, runtimecontract.RuntimeValidationExecutionStage{
			Identity: identity, Argv: []string{"owner-tool"}, StartedAt: []string{"2026-10-14T00:00:00Z", "2026-10-14T00:00:02Z"}[i], FinishedAt: []string{"2026-10-14T00:00:01Z", "2026-10-14T00:00:03Z"}[i],
			Started: true, TerminalComplete: true, KnownFailure: true, ExitCode: 1, Stdout: output, Stderr: output, SourceBefore: stagesHash, SourceAfter: stagesHash,
		})
	}
	conjunction.Stages[1].DiagnosticSourceRef, conjunction.Stages[1].BehaviorTargetRefs = "source-a", []string{"implementation.go"}
	receipt := runtimecontract.RuntimeOperationReceipt{OperationID: operation.ID, Outcome: runtimecontract.RuntimeOperationOutcomeRejected,
		Feedback: runtimeOperationFeedbackForTest(t, aggregate, operation.ID, runtimecontract.FailureFact{Class: runtimecontract.FailureClassBuildFailure, Code: "owner-failed", Message: "original failure"}),
		Evidence: runtimecontract.RuntimeOperationEvidence{Validation: &runtimecontract.RuntimeValidationObservation{RecipeID: binding.ValidationRecipeID, ObservationProtocolRef: conjunction.ProtocolRef, ExecutionConjunction: &conjunction}},
	}
	check := func(a RuntimeAggregate, d SealedDispatch, r runtimecontract.RuntimeOperationReceipt, reload bool) error {
		if reload {
			return validateOperationReceiptFeedbackAgainstDispatch(a, d, common.WorkspaceID("ws-source"), common.RunID("run-source"), r)
		}
		return validateOperationReceiptFeedback(a, "ws-source", "run-source", d.ID, r)
	}
	for _, reload := range []bool{false, true} {
		for _, scenario := range []string{"original", "ordinary-partial", "source-raw-gap", "source-raw-gap-with-failure", "project-raw-gap-with-failure", "erase-fields", "erase-union-with-failure", "erase-conjunction-with-failure", "erase-observation-with-failure", "unknown-source"} {
			t.Run(map[bool]string{false: "ingress/", true: "reload/"}[reload]+scenario, func(t *testing.T) {
				a, d, r := aggregate, dispatch, receipt.Clone()
				if reload {
					body, err := json.Marshal(d)
					if err != nil || json.Unmarshal(body, &d) != nil {
						t.Fatal("dispatch reload failed", err)
					}
					a.ActiveDispatch = nil
					a.CompletedDispatches = []SealedDispatch{d}
				}
				switch scenario {
				case "source-raw-gap", "source-raw-gap-with-failure":
					r.Evidence.Validation.ExecutionConjunction.Stages[1].Stderr.Storage = &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: "original-publication-failed"}
				case "project-raw-gap-with-failure":
					r.Evidence.Validation.ExecutionConjunction.Stages[0].Stderr.Storage = &runtimecontract.RuntimeValidationOutputStorage{OmittedReason: "original-publication-failed"}
				case "ordinary-partial", "erase-fields", "erase-union-with-failure":
					r.Evidence.Validation.ExecutionConjunction.Stages[1].DiagnosticSourceRef = ""
					r.Evidence.Validation.ExecutionConjunction.Stages[1].BehaviorTargetRefs = nil
					if scenario == "erase-union-with-failure" {
						r.Evidence.Validation.ExecutionConjunction.BehaviorTargetRefs = nil
					}
				case "erase-conjunction-with-failure":
					r.Evidence.Validation.ExecutionConjunction = nil
				case "erase-observation-with-failure":
					r.Evidence.Validation = nil
				case "unknown-source":
					r.Evidence.Validation.ExecutionConjunction.Stages[1].DiagnosticSourceRef = "unknown"
				}
				if strings.HasSuffix(scenario, "with-failure") || scenario == "erase-fields" {
					r.Feedback.OriginalDiagnostic.ValidationFailureObservation = &runtimecontract.RuntimeValidationFailureObservation{
						WorkspaceID: "ws-source", RunID: "run-source", DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
						OperationID: operation.ID, FailureSignature: r.Feedback.OriginalDiagnostic.Signature, TargetRole: "implementation",
						DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed, DiagnosticHash: strings.Repeat("b", 64),
						CandidateTargetRefs: []string{"implementation.go"}, ValidationScopeTargetRefs: catalog.TargetRefs,
					}
					r.Feedback.OriginalDiagnostic.ValidationDiagnostic = &runtimecontract.ValidationFailureDiagnostic{Operation: operation.Kind, Class: runtimecontract.ValidationDiagnosticBuildFailed, TargetRefs: []string{"implementation.go"}}
				}
				err := check(a, d, r, reload)
				if scenario == "original" || scenario == "ordinary-partial" || scenario == "source-raw-gap" {
					if err != nil {
						t.Fatal("complete source attribution rejected", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "diagnostic") {
					t.Fatalf("source downgrade escaped original receipt consumer: %v", err)
				}
			})
		}
	}
}
