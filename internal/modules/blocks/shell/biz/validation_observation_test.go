package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapterPersistsZeroWorkGoTestObservation(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := newGoValidationWorkspace(t, `package fixture

import "testing"

func TestExisting(t *testing.T) {}
`)
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "go-test-zero-work",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true,
			Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command:    "go",
		Args:       []string{"test", "-v", "-run", "TestMissing", "./..."},
		TargetRefs: []string{"fixture_test.go"},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "go-test-zero-work", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "go",
			ArgsContain: []string{"test"}, ZeroWorkMarkers: []string{"no tests to run", "[no tests to run]"},
			WorkEvidenceMarkers: []string{"=== RUN", "--- PASS:"}, QualityOnZeroWork: string(common.EvidenceQualityInconclusive),
			EvidenceReason: "configured zero-work marker matched", EvidenceTags: []string{"validation", "zero_work"},
		}},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if !result.ZeroWork || result.EvidenceQuality != string(common.EvidenceQualityInconclusive) ||
		result.Evidence.Quality != common.EvidenceQualityInconclusive || !result.Evidence.ZeroWork ||
		result.Evidence.EvidenceReason != "configured zero-work marker matched" {
		t.Fatalf("zero-work quality was not persisted: %+v", result)
	}
	if result.Evidence.Validation == nil || len(result.Evidence.Validation.PassedChecks) != 0 || result.Evidence.Validation.OutputPreview == "" {
		t.Fatalf("zero-work validation observation = %+v", result.Evidence.Validation)
	}
}

func TestRuntimeAdapterMarksUnconfiguredGoTestZeroWorkAsInconclusive(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := newGoValidationWorkspace(t, `package fixture

import "testing"

func TestExisting(t *testing.T) {}
`)
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "go-test-unconfigured-zero-work",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true,
			Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command:    "go",
		Args:       []string{"test", "-v", "-run", "TestMissing", "./..."},
		TargetRefs: []string{"fixture_test.go"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if !result.ZeroWork || result.Evidence.Quality != common.EvidenceQualityInconclusive ||
		result.Evidence.EvidenceReason != "validation command reported no executed checks" {
		t.Fatalf("unconfigured zero-work evidence = %+v", result.Evidence)
	}
	if result.Evidence.Validation == nil || len(result.Evidence.Validation.ExecutedChecks) != 0 ||
		len(result.Evidence.Validation.PassedChecks) != 0 || len(result.Evidence.Validation.SkippedChecks) != 0 {
		t.Fatalf("unconfigured zero-work observation = %+v", result.Evidence.Validation)
	}
}

func TestRuntimeAdapterObservesTopLevelPassedAndSkippedGoChecks(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := newGoValidationWorkspace(t, `package fixture

import "testing"

func TestBehavior(t *testing.T) {
	t.Run("subcase", func(t *testing.T) {})
}

func TestSkipped(t *testing.T) {
	t.Skip("not applicable")
}
`)
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "go-test-check-observation",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true,
			Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command:    "go",
		Args:       []string{"test", "-v", "./..."},
		TargetRefs: []string{"fixture_test.go"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	observation := result.Evidence.Validation
	if result.Evidence.Quality != common.EvidenceQualityValid || observation == nil {
		t.Fatalf("validation evidence = %+v", result.Evidence)
	}
	if !equalValidationChecks(observation.ExecutedChecks, []string{"TestBehavior", "TestSkipped"}) ||
		!equalValidationChecks(observation.PassedChecks, []string{"TestBehavior"}) ||
		!equalValidationChecks(observation.SkippedChecks, []string{"TestSkipped"}) || len(observation.FailedChecks) != 0 {
		t.Fatalf("top-level validation observation = %+v", observation)
	}
}

func TestRuntimeAdapterMarksAllSkippedGoChecksAsSkippedEvidence(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := newGoValidationWorkspace(t, `package fixture

import "testing"

func TestSkippedOne(t *testing.T) { t.Skip("not applicable") }
func TestSkippedTwo(t *testing.T) { t.Skip("not applicable") }
`)
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "go-test-all-skipped",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true,
			Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command:    "go",
		Args:       []string{"test", "-v", "./..."},
		TargetRefs: []string{"fixture_test.go"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	observation := result.Evidence.Validation
	if result.Evidence.Quality != common.EvidenceQualitySkipped || observation == nil ||
		result.Evidence.EvidenceReason != "all observed validation checks were skipped" {
		t.Fatalf("all-skipped evidence = %+v", result.Evidence)
	}
	if !equalValidationChecks(observation.ExecutedChecks, []string{"TestSkippedOne", "TestSkippedTwo"}) ||
		!equalValidationChecks(observation.SkippedChecks, []string{"TestSkippedOne", "TestSkippedTwo"}) ||
		len(observation.PassedChecks) != 0 || len(observation.FailedChecks) != 0 {
		t.Fatalf("all-skipped observation = %+v", observation)
	}
}

func newGoValidationWorkspace(t *testing.T, testSource string) string {
	t.Helper()
	workspace := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":          "module example.com/workorch-validation-fixture\n\ngo 1.24\n",
		"fixture.go":      "package fixture\n",
		"fixture_test.go": testSource,
	} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return workspace
}

func equalValidationChecks(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for idx := range got {
		if got[idx] != want[idx] {
			return false
		}
	}
	return true
}
