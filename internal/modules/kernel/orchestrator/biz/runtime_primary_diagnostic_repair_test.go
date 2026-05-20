package orchestrator

import (
	"errors"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateLLMExecutionOutputContractRejectsPatchThatMissesPrimaryDiagnostics(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-primary-diagnostic-miss",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := primaryDiagnosticRepairStep("run-primary-diagnostic-miss/repair-001")
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "secondary-assertion-cleanup",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "unified_diff",
					"targetRef": "main_test.go",
					"diff": strings.Join([]string{
						"--- a/main_test.go",
						"+++ b/main_test.go",
						"@@ -140,3 +140,3 @@",
						" old assertion text",
						"-want old",
						"+want new",
						"",
					}, "\n"),
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "primary diagnostic repair requires") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want primary diagnostic rejection", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsPatchOverPrimaryDiagnosticLines(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-primary-diagnostic-line-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := primaryDiagnosticRepairStep("run-primary-diagnostic-line-repair/repair-001")
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-primary-shape-lines",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "unified_diff",
					"targetRef": "main_test.go",
					"diff": strings.Join([]string{
						"--- a/main_test.go",
						"+++ b/main_test.go",
						"@@ -30,4 +30,2 @@",
						" func configure() {",
						"-\t_ = Vendor{ModelRouting: nil}",
						"-\t_ = Vendor{Headers: nil}",
						" }",
						"",
					}, "\n"),
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want primary diagnostic line patch allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsRelatedPatchWithPrimaryDiagnosticDeclaration(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-primary-diagnostic-related-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := primaryDiagnosticRepairStep("run-primary-diagnostic-related-repair/repair-001")
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-related-contract-resource",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType":             "unified_diff",
					"targetRef":             "vendor_contract.txt",
					"relatedFailureTargets": []string{"main_test.go"},
					"primaryDiagnosticResolution": map[string]any{
						"targetRef":   "main_test.go",
						"line":        30,
						"identifier":  "ModelRouting",
						"explanation": "align related resource contract with reported primary diagnostic",
					},
					"diff": strings.Join([]string{
						"--- a/vendor_contract.txt",
						"+++ b/vendor_contract.txt",
						"@@ -1,2 +1,3 @@",
						" Vendor",
						"+ModelRouting",
						" Headers",
						"",
					}, "\n"),
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want declared related primary diagnostic repair allowed", err)
	}
}

func primaryDiagnosticRepairStep(id common.StepID) *runmodel.Step {
	return &runmodel.Step{
		ID:   id,
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeySoulStage:             soulStageExecution,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"failureTargetScope":              "failureTargets=main_test.go",
			stepInputKeyRepairOfFailureSig:    "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|exitCode=1",
			"failureTargetRepairContext": []map[string]any{{
				"targetRef":        "main_test.go",
				"exists":           true,
				"contentHash":      "hash-main-test",
				"contentComplete":  true,
				"safeForFileWrite": true,
				"content":          primaryDiagnosticMainTestContent(),
			}},
			"primaryDiagnosticRepair": map[string]any{
				"required": true,
				"targetRefs": []string{
					"main_test.go",
				},
				"lineRefs": []map[string]any{
					{"targetRef": "main_test.go", "line": 30, "column": 4},
					{"targetRef": "main_test.go", "line": 31, "column": 4},
				},
				"identifiers": []string{"ModelRouting", "Headers", "Vendor"},
			},
			"repairActionPackage": map[string]any{
				"primaryDiagnosticRepair": map[string]any{
					"required":    true,
					"targetRefs":  []string{"main_test.go"},
					"lineRefs":    []map[string]any{{"targetRef": "main_test.go", "line": 30}, {"targetRef": "main_test.go", "line": 31}},
					"identifiers": []string{"ModelRouting", "Headers", "Vendor"},
				},
				"targetSnapshots": []map[string]any{{
					"targetRef": "main_test.go",
					"content":   primaryDiagnosticMainTestContent(),
				}},
			},
		},
	}
}

func primaryDiagnosticMainTestContent() string {
	lines := make([]string, 0, 150)
	for idx := 1; idx <= 150; idx++ {
		switch idx {
		case 30:
			lines = append(lines, "\t_ = Vendor{ModelRouting: nil}")
		case 31:
			lines = append(lines, "\t_ = Vendor{Headers: nil}")
		case 140:
			lines = append(lines, "want old")
		default:
			lines = append(lines, "line")
		}
	}
	return strings.Join(lines, "\n")
}
