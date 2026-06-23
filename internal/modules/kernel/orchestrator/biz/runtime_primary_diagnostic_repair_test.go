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

func TestValidateLLMExecutionOutputContractAllowsInferredRelatedShapePatch(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-primary-diagnostic-inferred-related-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := primaryDiagnosticRepairStep("run-primary-diagnostic-inferred-related-repair/repair-001")
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-related-resource-shape",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "unified_diff",
					"targetRef": "main.go",
					"diff": strings.Join([]string{
						"--- a/main.go",
						"+++ b/main.go",
						"@@ -10,3 +10,6 @@",
						" type Vendor struct {",
						" \tBaseURL string",
						"+\tModelRouting map[string]string",
						"+\tHeaders map[string]string",
						" }",
						"",
					}, "\n"),
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want inferred related primary diagnostic repair allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsTextReplacePrimaryDiagnosticPatch(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-primary-diagnostic-text-replace",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := primaryDiagnosticRepairStep("run-primary-diagnostic-text-replace/repair-001")
	step.Input["failureTargetScope"] = "failureTargets=main.go"
	step.Input[stepInputKeyRepairOfFailureSig] = "class=shell_command_failed|kind=shell/command|failureTargets=main.go|exitCode=1"
	step.Input["primaryDiagnosticRepair"] = map[string]any{
		"required":   true,
		"targetRefs": []string{"main.go"},
		"lineRefs":   []map[string]any{{"targetRef": "main.go", "line": 12, "column": 2}},
		"identifiers": []string{
			"mcp",
			"CallToolRequest",
		},
	}
	step.Input["repairActionPackage"] = map[string]any{
		"primaryDiagnosticRepair": step.Input["primaryDiagnosticRepair"],
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "repair-import",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "text_replace",
					"targetRef": "main.go",
					"oldText": strings.Join([]string{
						"import (",
						"\t\"context\"",
						")",
					}, "\n"),
					"newText": strings.Join([]string{
						"import (",
						"\t\"context\"",
						"\t\"github.com/mark3labs/mcp-go/mcp\"",
						")",
					}, "\n"),
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want text_replace primary diagnostic patch allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsTextReplaceDeletingPrimaryDiagnosticLine(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-primary-diagnostic-text-replace-delete-line",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := primaryDiagnosticRepairStep("run-primary-diagnostic-text-replace-delete-line/repair-001")
	step.Input["failureTargetScope"] = "failureTargets=main.go"
	step.Input[stepInputKeyRepairOfFailureSig] = "class=shell_command_failed|kind=shell/command|failureTargets=main.go|exitCode=1"
	step.Input["failureTargetRepairContext"] = []map[string]any{{
		"targetRef":        "main.go",
		"exists":           true,
		"contentComplete":  true,
		"safeForFileWrite": true,
		"content": strings.Join([]string{
			"package main",
			"",
			"import (",
			"\t\"bufio\"",
			"\t\"context\"",
			"\t\"encoding/json\"",
			"\t\"fmt\"",
			"\t\"log\"",
			"\t\"os\"",
			"\t\"os/exec\"",
			"\t\"strings\"",
			"\t\"sync\"",
			"\t\"time\"",
			")",
		}, "\n"),
	}}
	step.Input["primaryDiagnosticRepair"] = map[string]any{
		"required":    true,
		"targetRefs":  []string{"main.go"},
		"lineRefs":    []map[string]any{{"targetRef": "main.go", "line": 11, "column": 2}},
		"identifiers": []string{"strings"},
		"messages":    []string{`./main.go:11:2: "strings" imported and not used`},
	}
	step.Input["repairActionPackage"] = map[string]any{
		"primaryDiagnosticRepair": step.Input["primaryDiagnosticRepair"],
		"targetSnapshots": []map[string]any{{
			"targetRef": "main.go",
			"content":   mapSliceFromValue(step.Input["failureTargetRepairContext"])[0]["content"],
		}},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "remove-unused-import",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "text_replace",
					"targetRef": "main.go",
					"oldText":   "\t\"strings\"\n",
					"newText":   "",
					"summary":   `remove unused "strings" import reported by the primary diagnostic`,
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want text_replace deletion on primary diagnostic line allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsImportPatchForLinePayloadShortIdentifier(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-primary-diagnostic-line-payload-import",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := primaryDiagnosticRepairStep("run-primary-diagnostic-line-payload-import/repair-001")
	step.Input["failureTargetScope"] = "failureTargets=main.go"
	step.Input[stepInputKeyRepairOfFailureSig] = "class=shell_command_failed|kind=shell/command|failureTargets=main.go|exitCode=1"
	step.Input["primaryDiagnosticRepair"] = map[string]any{
		"required":   true,
		"targetRefs": []string{"main.go"},
		"lineRefs": []map[string]any{
			{"targetRef": "main.go", "line": 23, "column": 11},
			{"targetRef": "main.go", "line": 24, "column": 11},
		},
		"messages": []string{
			"./main.go:23:11: undefined: io",
			"./main.go:24:11: undefined: io",
		},
	}
	step.Input["repairActionPackage"] = map[string]any{
		"primaryDiagnosticRepair": step.Input["primaryDiagnosticRepair"],
		"targetSnapshots": []map[string]any{{
			"targetRef": "main.go",
			"content": strings.Join([]string{
				"package main",
				"",
				"import (",
				"\t\"context\"",
				"\t\"fmt\"",
				"\t\"log\"",
				")",
				"",
				"type LSPClient struct {",
				"\tstdin io.WriteCloser",
				"\tstdout io.ReadCloser",
				"}",
			}, "\n"),
		}},
	}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "fix-import",
					"kind": "patch/apply",
					"input": map[string]any{
						"patchType": "text_replace",
						"targetRef": "main.go",
						"oldText": strings.Join([]string{
							"import (",
							"\t\"context\"",
							"\t\"fmt\"",
							"\t\"log\"",
							")",
						}, "\n"),
						"newText": strings.Join([]string{
							"import (",
							"\t\"context\"",
							"\t\"fmt\"",
							"\t\"io\"",
							"\t\"log\"",
							")",
						}, "\n"),
						"summary": "add io import for io.WriteCloser and io.ReadCloser",
					},
				},
				{
					"dependsOn": []string{"fix-import"},
					"id":        "assign-pipes",
					"kind":      "patch/apply",
					"input": map[string]any{
						"patchType": "text_replace",
						"targetRef": "main.go",
						"oldText": strings.Join([]string{
							"\tstdout, err := cmd.StdoutPipe()",
							"\tif err != nil {",
							"\t\treturn err",
							"\t}",
						}, "\n"),
						"newText": strings.Join([]string{
							"\tstdout, err := cmd.StdoutPipe()",
							"\tif err != nil {",
							"\t\treturn err",
							"\t}",
							"\tclient.stdin = stdin",
							"\tclient.stdout = stdout",
						}, "\n"),
						"summary": "assign io pipes to client after creation",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want import patch for line payload short identifier allowed", err)
	}
}

func TestValidateLLMExecutionOutputContractAllowsGraphNodeAfterDependencyAddressesPrimaryDiagnostic(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-primary-diagnostic-dependency-addressed",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := primaryDiagnosticRepairStep("run-primary-diagnostic-dependency-addressed/graph-001/002-secondary-shape")
	step.Input[stepInputKeyGraphNodeID] = "secondary-shape"
	step.Input[stepInputKeyGraphCompletedNodes] = []string{"fix-primary-shape"}
	step.Input[stepInputKeyGraphEvidence] = []map[string]any{{
		"nodeId": "fix-primary-shape",
		"stepId": "run-primary-diagnostic-dependency-addressed/graph-001/001-fix-primary-shape",
		"kind":   "patch/apply",
		"output": map[string]any{
			"status": "completed",
			"stage":  "patch_apply",
			"patches": []map[string]any{{
				"patchType": "text_replace",
				"targetRef": "main_test.go",
				"oldText":   "\t_ = Vendor{ModelRouting: nil}",
				"newText":   "\t_ = Vendor{ModelRouting: map[string]string{}}",
				"summary":   "repair ModelRouting shape reported by the primary diagnostic",
			}},
		},
	}}
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "secondary-shape",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "text_replace",
					"targetRef": "main_test.go",
					"oldText":   "want old",
					"newText": strings.Join([]string{
						"type SupplementalShape struct{}",
						"",
						"want old",
					}, "\n"),
					"summary": "apply secondary shape prerequisite in the same diagnostic target after dependency repair",
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want dependency-addressed graph node patch allowed", err)
	}
}

func TestStructuredFailureSignalsExtractShortLinePayloadIdentifier(t *testing.T) {
	output := map[string]any{
		"stderr": strings.Join([]string{
			"./main.go:23:11: undefined: io",
			"./main.go:24:11: undefined: io",
		}, "\n"),
		failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=main.go|exitCode=1",
	}

	signals := structuredFailureSignalsFromFailureOutput(output, errors.New("exit status 1"), failureClassShellCommand, "")

	if !stringSliceContains(stringSliceFromValue(signals["identifiers"]), "io") {
		t.Fatalf("structuredFailureSignals = %#v, want short line payload identifier io", signals)
	}
}

func TestRepairActionOutputDecisionContractNarrowsPrimaryDiagnosticToPatchApply(t *testing.T) {
	pkg := map[string]any{
		"primaryDiagnosticRepair": map[string]any{"required": true},
		"targetSnapshots":         []map[string]any{{"targetRef": "main.go", "content": "package main\n"}},
	}
	required := stateChangingRepairRequiredOutput()
	required["mustIncludeStateChangingNode"] = true

	contract := repairActionOutputDecisionContract(map[string]any{}, pkg, required)

	if contract["requiredMode"] != "state_changing_patch_apply" {
		t.Fatalf("outputDecisionContract = %+v, want state_changing_patch_apply", contract)
	}
	forbidden := strings.Join(stringSliceFromValue(contract["forbiddenOutputs"]), "\n")
	if !strings.Contains(forbidden, "shell_only_validation_for_state_change") {
		t.Fatalf("forbiddenOutputs = %q, want shell-only validation forbidden", forbidden)
	}
}

func TestValidateLLMExecutionOutputContractRejectsInferredRelatedCommentOnlyPatch(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-primary-diagnostic-inferred-related-comment",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := primaryDiagnosticRepairStep("run-primary-diagnostic-inferred-related-comment/repair-001")
	output := map[string]any{
		"status": "completed",
		"stage":  "llm_execution_graph_plan",
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "comment-related-resource",
				"kind": "patch/apply",
				"input": map[string]any{
					"patchType": "unified_diff",
					"targetRef": "main.go",
					"diff": strings.Join([]string{
						"--- a/main.go",
						"+++ b/main.go",
						"@@ -1,2 +1,3 @@",
						" package main",
						"+// ModelRouting and Headers are mentioned here without changing resource shape.",
						" type Vendor struct {}",
						"",
					}, "\n"),
				},
			}},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		(!strings.Contains(err.Error(), "primary diagnostic repair requires") && !strings.Contains(err.Error(), "not related to failure target")) {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want comment-only inferred repair rejection", err)
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
