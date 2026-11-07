package orchestrator

import (
	"reflect"
	"strings"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	_ "gopkg.in/yaml.v3" // The native fixture uses this declared module; CI fetches it with test dependencies.
)

// Public checksums of the test dependency, not captured business diagnostics.
const checksumFixtureModuleSum = "gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=\n"
const checksumFixtureFullSum = "gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=\n" + checksumFixtureModuleSum

func shippedChecksumRepairConvergence(t *testing.T) runtimecontract.RuntimeValidationFailureRepairConvergence {
	t.Helper()
	templates, err := workorchdefaults.BuiltinWorkspaceTemplateDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, template := range templates {
		if template.ID != "software-dev" {
			continue
		}
		profiles, _ := template.ContextHints["profileContracts"].(map[string]any)
		for _, raw := range profiles {
			profile, _ := raw.(map[string]any)
			policies, _ := runtimeprofiledecode.ObjectSliceExact(profile["evidenceRequirements"])
			for _, policy := range policies {
				convergence := runtimeprofiledecode.ValidationFailureRepairConvergence(policy)
				if convergence != nil {
					if err := convergence.Validate(); err != nil {
						t.Fatal(err)
					}
					return convergence.Clone()
				}
			}
		}
	}
	t.Fatal("shipped software-dev convergence missing")
	return runtimecontract.RuntimeValidationFailureRepairConvergence{}
}

// All resource owners and native build/test/race/behavior stages are real.
// Only the provider's semantic/candidate/assessment responses are scripted.
func TestDriveRuntimeProjectChecksumRepair(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1}
	for _, mode := range []string{"project_checksum_missing", "project_checksum_existing"} {
		t.Run(mode, func(t *testing.T) {
			baseline := runValidationInputReviewMainline(t, mode, "", options)
			for _, cut := range []string{"after_project_receipt", "after_repair_receipt"} {
				t.Run(cut, func(t *testing.T) {
					if calls := runValidationInputReviewMainline(t, mode, cut, options); calls != baseline {
						t.Fatalf("recovery repeated provider consumption: %d != %d", calls, baseline)
					}
				})
			}
		})
	}
}

func assertChecksumRepairOwnerFacts(t *testing.T, record runtimecontract.RuntimeStateRecord, mode string) {
	t.Helper()
	repairs, rejected := 0, 0
	var rejectedSubject, acceptedSubject string
	for _, receipt := range record.OperationReceipts {
		if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected {
			rejected++
			if receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil || receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic == nil {
				t.Fatal("original rejected build lost owner diagnostic")
			}
			diagnostic := receipt.Feedback.OriginalDiagnostic.ValidationDiagnostic
			if diagnostic.Class != runtimecontract.ValidationDiagnosticDependencyMismatch || diagnostic.Stage != "project-build" || diagnostic.ExitCode != 1 || len(diagnostic.TargetRefs) != 0 && !reflect.DeepEqual(diagnostic.TargetRefs, []string{"go.sum"}) {
				t.Fatalf("dependency failure targeted importing implementation: %+v", diagnostic)
			}
		}
		if receipt.Evidence.Validation != nil {
			for _, dispatch := range record.CompletedDispatches {
				matched := false
				for _, execution := range dispatch.OperationExecutions {
					matched = matched || execution.ExecutionKey == receipt.ExecutionKey && execution.Fence == receipt.Fence
				}
				if !matched {
					continue
				}
				for _, catalog := range dispatch.OperationAdmissibilities {
					for _, surface := range catalog.ValidationSurfaces {
						for _, binding := range surface.ProofRequirementBindings {
							if binding.ValidationRecipeID != receipt.Evidence.Validation.RecipeID || binding.InputReview == nil {
								continue
							}
							if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected {
								rejectedSubject = binding.InputReview.Hash
								continue
							}
							acceptedSubject = binding.InputReview.Hash
							found := false
							for _, file := range binding.InputReview.Files {
								if file.TargetRef == "go.sum" && file.Content == checksumFixtureFullSum {
									found = true
								}
							}
							if !found {
								t.Fatal("fresh validation reused an input subject without the repaired checksum")
							}
						}
					}
				}
			}
		}
	}
	for _, dispatch := range record.CompletedDispatches {
		if dispatch.Candidate.StateChange == nil {
			continue
		}
		for _, entry := range dispatch.Candidate.StateChange.Operations {
			operation := entry.Operation
			if !reflect.DeepEqual(operation.TargetRefs, []string{"go.sum"}) {
				continue
			}
			repairs++
			want := runtimecontract.StepKindFileWrite
			if mode == "project_checksum_existing" {
				want = runtimecontract.StepKindFileEdit
			}
			if operation.Kind != want || len(dispatch.OperationExecutions) != 1 {
				t.Fatal("checksum repair widened its sealed operation")
			}
			execution := dispatch.OperationExecutions[0]
			receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(record.OperationReceipts, execution.ExecutionKey, execution.Fence)
			if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !receipt.Changed || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
				t.Fatal("File repair lacked unique committed receipt")
			}
			facts := 0
			for _, fact := range record.Facts.Evidence.Accepted {
				if fact.DispatchID == dispatch.ID && fact.OperationID == operation.ID {
					facts++
					if fact.Kind != runtimecontract.EvidenceKindStateChange {
						t.Fatal("repair became validation evidence")
					}
					// Prospective repair keeps its historical CWI audit binding;
					// current coverage must consume fresh validation instead.
					for _, item := range record.Facts.Coverage {
						for _, ref := range item.EvidenceRefs {
							if ref == string(fact.ID) {
								t.Fatal("checksum effect supplied current coverage credit")
							}
						}
					}
				}
			}
			if facts != 1 {
				t.Fatal("checksum repair evidence was missing or duplicated")
			}
		}
	}
	if repairs != 1 || rejected != 1 {
		t.Fatalf("expected one repair after one real failure; repairs=%d rejected=%d", repairs, rejected)
	}
	if strings.TrimSpace(rejectedSubject) == "" || strings.TrimSpace(acceptedSubject) == "" || rejectedSubject == acceptedSubject {
		t.Fatal("checksum repair did not independently review the new input subject")
	}
}
