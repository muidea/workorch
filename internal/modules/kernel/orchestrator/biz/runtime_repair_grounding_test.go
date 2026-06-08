package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestStructuredFailureSignalsExtractsNeutralPathLineIdentifier(t *testing.T) {
	output := map[string]any{
		"stderr": strings.Join([]string{
			`./provider_matrix_test.go:51:32: undefined: handleChatCompletions`,
			`./provider_matrix_test.go:22:42: unknown field expectedVendor in struct literal of type providerMatrixCase`,
		}, "\n"),
		failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=provider_matrix_test.go|exitCode=1",
	}

	signals := structuredFailureSignalsFromFailureOutput(output, errors.New("exit status 1"), failureClassShellCommand, "")

	targets := stringSliceFromValue(signals["targetRefs"])
	if !stringSliceContains(targets, "provider_matrix_test.go") {
		t.Fatalf("targetRefs = %#v, want provider_matrix_test.go", signals["targetRefs"])
	}
	lineRefs := mapSliceFromValue(signals["lineRefs"])
	if len(lineRefs) < 2 || lineRefs[0]["targetRef"] != "provider_matrix_test.go" || lineRefs[0]["line"] != 51 || lineRefs[0]["column"] != 32 {
		t.Fatalf("lineRefs = %#v, want parsed target line and column refs", signals["lineRefs"])
	}
	identifiers := stringSliceFromValue(signals["identifiers"])
	for _, want := range []string{"handleChatCompletions", "expectedVendor", "providerMatrixCase"} {
		if !stringSliceContains(identifiers, want) {
			t.Fatalf("identifiers = %#v, want %q", signals["identifiers"], want)
		}
	}
	messages := stringSliceFromValue(signals["messages"])
	foundDiagnostic := false
	for _, message := range messages {
		if strings.Contains(message, "handleChatCompletions") {
			foundDiagnostic = true
			break
		}
	}
	if !foundDiagnostic {
		t.Fatalf("messages = %#v, want bounded diagnostic line", signals["messages"])
	}
}

func TestMCPFailureTargetsPreferToolInputOverInternalDiagnostics(t *testing.T) {
	output := map[string]any{
		"operation": "mcp/tool_call",
		"serverId":  "mcp-semantic",
		"tool":      "lookup_references",
		"stage":     "mcp_tool_call_failed",
		"error":     "mcp tool lookup_references failed: column is beyond end of line",
		"input": map[string]any{
			"path": "src/symbol.go",
			"line": 1,
			"col":  13,
		},
		"stderr": strings.Join([]string{
			`provider: 2026/05/31 10:07:40 client.go:480: ignore notification`,
			`provider: 2026/05/31 10:07:41 client.go:442: internal stderr details`,
		}, "\n"),
		"runtimeDiagnostics": map[string]any{
			"operation":      "mcp/tool_call",
			"serverId":       "mcp-semantic",
			"tool":           "lookup_references",
			"targetRef":      "src/symbol.go",
			"failureTargets": []string{"src/symbol.go"},
		},
	}

	targets := failureTargetsForSignature(output)
	if len(targets) != 1 || targets[0] != "src/symbol.go" {
		t.Fatalf("failureTargetsForSignature = %#v, want only tool input target", targets)
	}

	signature := runtimeFailureSignature(failureClassMCPRuntime, &runmodel.Step{Kind: "mcp/tool_call", Output: output}, output, errors.New("column is beyond end of line"))
	if !strings.Contains(signature, "failureTargets=src/symbol.go") || strings.Contains(signature, "failureTargets=client.go") {
		t.Fatalf("signature = %q, want input target without internal diagnostic target", signature)
	}

	signals := structuredFailureSignalsFromFailureOutput(output, nil, failureClassMCPRuntime, signature)
	refs := stringSliceFromValue(signals["targetRefs"])
	if !stringSliceContains(refs, "src/symbol.go") || stringSliceContains(refs, "client.go") {
		t.Fatalf("structuredFailureSignals = %#v, want input target without internal diagnostic target", signals)
	}
}

func TestMCPFailureWithoutExplicitTargetDoesNotPromoteInternalDiagnostics(t *testing.T) {
	output := map[string]any{
		"operation": "mcp/tool_call",
		"serverId":  "mcp-semantic",
		"tool":      "initialize",
		"stage":     "mcp_tool_call_failed",
		"error":     "mcp tool initialize failed: missing dependency",
		"stderr": strings.Join([]string{
			`provider: 2026/05/31 12:10:23 client.go:480: ignore notification`,
			`provider: 2026/05/31 12:10:24 client.go:434: process exited`,
		}, "\n"),
		"runtimeDiagnostics": map[string]any{
			"operation": "mcp/tool_call",
			"serverId":  "mcp-semantic",
			"tool":      "initialize",
		},
		failureOutputKeySignature: "class=mcp_runtime_failed|kind=mcp/tool_call|stage=mcp_tool_call_failed|serverId=mcp-semantic|tool=initialize|operation=mcp/tool_call|failureTargets=client.go|error=missing dependency",
		"lastFailureDiagnostics": map[string]any{
			"stderrPreview":           `provider: 2026/05/31 12:10:23 client.go:480: ignore notification`,
			failureOutputKeySignature: "class=mcp_runtime_failed|kind=mcp/tool_call|stage=mcp_tool_call_failed|serverId=mcp-semantic|tool=initialize|operation=mcp/tool_call|failureTargets=client.go|error=missing dependency",
		},
	}

	if targets := failureTargetsForSignature(output); len(targets) != 0 {
		t.Fatalf("failureTargetsForSignature = %#v, want no promoted internal diagnostic target", targets)
	}

	signature := runtimeFailureSignature(failureClassMCPRuntime, &runmodel.Step{Kind: "mcp/tool_call", Output: output}, output, errors.New("missing dependency"))
	if strings.Contains(signature, "failureTargets=client.go") {
		t.Fatalf("signature = %q, must not promote internal diagnostic target", signature)
	}
	if !strings.Contains(signature, "serverId=mcp-semantic") || !strings.Contains(signature, "tool=initialize") {
		t.Fatalf("signature = %q, want external capability identity scope", signature)
	}

	signals := structuredFailureSignalsFromFailureOutput(output, nil, failureClassMCPRuntime, signature)
	if refs := stringSliceFromValue(signals["targetRefs"]); stringSliceContains(refs, "client.go") {
		t.Fatalf("structuredFailureSignals = %#v, must not promote internal diagnostic target", signals)
	}
	step := &runmodel.Step{
		Kind:   "mcp/tool_call",
		Output: output,
		Input: map[string]any{
			stepInputKeyRepairOfFailureSig: stringFromMap(output, failureOutputKeySignature),
		},
	}
	if refs := failureTargetRepairRefs(step); stringSliceContains(refs, "client.go") {
		t.Fatalf("failureTargetRepairRefs = %#v, must not promote internal diagnostic target", refs)
	}
	messages := stringSliceFromValue(signals["messages"])
	foundDiagnostic := false
	for _, message := range messages {
		if strings.Contains(message, "client.go") {
			foundDiagnostic = true
			break
		}
	}
	if !foundDiagnostic {
		t.Fatalf("messages = %#v, want diagnostic text retained as message", messages)
	}
}

func TestStructuredFailureSignalsIgnoresNestedFailedStepOutputInternalLogTargets(t *testing.T) {
	output := map[string]any{
		"stage":               "llm_context_integrity_failed",
		failureOutputKeyClass: failureClassRuntime,
		"failedStepOutput": map[string]any{
			"stage": "mcp_tool_call_failed",
			"stderr": strings.Join([]string{
				`provider: 2026/05/31 10:07:40 client.go:480: ignore notification`,
				`provider: 2026/05/31 10:07:41 client.go:442: internal stderr details`,
			}, "\n"),
			failureOutputKeySignature: "class=mcp_runtime_failed|kind=mcp/tool_call|failureTargets=src/symbol.go|error=column is beyond end of line",
			"runtimeDiagnostics": map[string]any{
				"targetRef": "src/symbol.go",
			},
		},
	}

	signals := structuredFailureSignalsFromFailureOutput(output, nil, failureClassRuntime, "")
	refs := stringSliceFromValue(signals["targetRefs"])
	if !stringSliceContains(refs, "src/symbol.go") {
		t.Fatalf("structuredFailureSignals = %#v, want explicit nested failed step target", signals)
	}
	if stringSliceContains(refs, "client.go") {
		t.Fatalf("structuredFailureSignals = %#v, must not promote nested stderr internal log target", signals)
	}
	step := &runmodel.Step{Output: output}
	targets := failureTargetRepairRefs(step)
	if !stringSliceContains(targets, "src/symbol.go") || stringSliceContains(targets, "client.go") {
		t.Fatalf("failureTargetRepairRefs = %#v, want explicit target without nested stderr noise", targets)
	}
}

func TestRepairActionPackageAnchorsPrimaryFailureAcrossWrapperFailure(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"failureClass":                failureClassLLMDecisionInvalid,
		"failureReason":               "invalid argument: executionGraph patch/apply must not depend on same-graph file/read",
		"failedStepOutput": map[string]any{
			"stage":               "llm_schema_invalid",
			"error":               "invalid argument: executionGraph patch/apply must not depend on same-graph file/read",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
		},
		"repairActionPackage": map[string]any{
			"currentFailure": map[string]any{
				"failedStepId":     "run/graph-001/011-build",
				"failedStepKind":   shellcommon.StepKindCommand,
				"failureClass":     failureClassShellCommand,
				"failureSignature": "class=shell_command_failed|kind=shell/command|failureTargets=pkg/lsp/server.go,main.go|exitCode=1",
				"reason":           "pkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
				"targetRef":        "pkg/lsp/server.go,main.go",
				"targetScope":      "failureTargets=pkg/lsp/server.go,main.go",
			},
			"lastFailureDiagnostics": map[string]any{
				"failureClass":     failureClassShellCommand,
				"failureSignature": "class=shell_command_failed|kind=shell/command|failureTargets=pkg/lsp/server.go,main.go|exitCode=1",
				"stderrPreview":    "pkg/lsp/server.go:10:24: undefined: LSPInstance",
			},
			"primaryDiagnosticRepair": map[string]any{
				"targetRefs": []string{"pkg/lsp/server.go", "main.go"},
				"lineRefs": []map[string]any{
					{"targetRef": "pkg/lsp/server.go", "line": 10, "column": 24},
					{"targetRef": "main.go", "line": 28, "column": 28},
				},
				"identifiers": []string{"LSPInstance", "NewServer"},
				"messages":    []string{"pkg/lsp/server.go:10:24: undefined: LSPInstance"},
			},
			"structuredFailureSignals": map[string]any{
				"targetRefs": []string{"pkg/lsp/server.go", "main.go"},
				"lineRefs": []map[string]any{
					{"targetRef": "pkg/lsp/server.go", "line": 10, "column": 24},
				},
			},
			"targetSnapshots": []map[string]any{{
				"targetRef":        "pkg/lsp/server.go",
				"exists":           true,
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash-server",
				"content":          "package lsp\n",
			}, {
				"targetRef":        "main.go",
				"exists":           true,
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash-main",
				"content":          "package main\n",
			}},
		},
	}

	pkg := repairActionPackageFromRepairInput(input, map[string]any{
		"stage":               "llm_schema_invalid",
		"error":               "invalid argument: executionGraph patch/apply must not depend on same-graph file/read",
		failureOutputKeyClass: failureClassLLMDecisionInvalid,
	})

	currentFailure := mapFromValue(pkg["currentFailure"])
	if currentFailure["failureClass"] != failureClassShellCommand ||
		currentFailure["failedStepId"] != "run/graph-001/011-build" ||
		!strings.Contains(stringFromMap(currentFailure, "reason"), "LSPInstance") {
		t.Fatalf("currentFailure = %+v, want original shell/build failure anchored", currentFailure)
	}
	signals := mapFromValue(pkg["structuredFailureSignals"])
	if refs := stringSliceFromValue(signals["targetRefs"]); !stringSliceContains(refs, "pkg/lsp/server.go") || stringSliceContains(refs, "client.go") {
		t.Fatalf("structuredFailureSignals = %+v, want primary target refs without wrapper noise", signals)
	}
	targets := mapSliceFromValue(pkg["targetSnapshots"])
	if len(targets) != 2 || targets[0]["targetRef"] != "pkg/lsp/server.go" || targets[1]["targetRef"] != "main.go" {
		t.Fatalf("targetSnapshots = %+v, want inherited current target snapshots", targets)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if required["mustPatchDirectlyFromProvidedSnapshot"] != true {
		t.Fatalf("requiredOutput = %+v, want direct snapshot patch contract", required)
	}
}

func TestRepairActionPackageAnchorsPrimaryFailureAcrossTargetAdmissionWrapperFailure(t *testing.T) {
	targetAdmissionFailure := `invalid argument: runtime repair patch/apply target "pkg/lsp/instance.go" is not related to failure target scope [pkg/lsp/server.go main.go]; patch a current failure target or return a real governance blocker`
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"failureClass":                failureClassRuntime,
		"failureReason":               targetAdmissionFailure,
		"failedStepOutput": map[string]any{
			"stage":                   "llm_execution_graph_plan",
			"error":                   targetAdmissionFailure,
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=" + targetAdmissionFailure,
		},
		"repairActionPackage": map[string]any{
			"currentFailure": map[string]any{
				"failedStepId":     "run/graph-001/009-verify-build",
				"failedStepKind":   shellcommon.StepKindCommand,
				"failureClass":     failureClassShellCommand,
				"failureSignature": "class=shell_command_failed|kind=shell/command|failureTargets=pkg/lsp/server.go,main.go|exitCode=1",
				"reason":           "pkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
				"targetRef":        "pkg/lsp/server.go,main.go",
				"targetScope":      "failureTargets=pkg/lsp/server.go,main.go",
			},
			"structuredFailureSignals": map[string]any{
				"targetRefs": []string{"pkg/lsp/server.go", "main.go"},
				"lineRefs": []map[string]any{
					{"targetRef": "pkg/lsp/server.go", "line": 10, "column": 24},
					{"targetRef": "main.go", "line": 28, "column": 28},
				},
			},
			"repairTargetSet": []string{"pkg/lsp/server.go", "main.go"},
			"targetSnapshots": []map[string]any{{
				"targetRef":        "pkg/lsp/server.go",
				"exists":           true,
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash-server",
				"content":          "package lsp\n",
			}, {
				"targetRef":        "main.go",
				"exists":           true,
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash-main",
				"content":          "package main\n",
			}},
		},
	}

	pkg := repairActionPackageFromRepairInput(input, map[string]any{
		"stage":                   "llm_execution_graph_plan",
		"error":                   targetAdmissionFailure,
		failureOutputKeyClass:     failureClassRuntime,
		failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=" + targetAdmissionFailure,
	})

	currentFailure := mapFromValue(pkg["currentFailure"])
	if currentFailure["failureClass"] != failureClassShellCommand ||
		currentFailure["failedStepId"] != "run/graph-001/009-verify-build" ||
		!strings.Contains(stringFromMap(currentFailure, "reason"), "LSPInstance") {
		t.Fatalf("currentFailure = %+v, want original shell/build failure anchored", currentFailure)
	}
	if refs := stringSliceFromValue(pkg["repairTargetSet"]); !stringSliceContains(refs, "pkg/lsp/server.go") || !stringSliceContains(refs, "main.go") {
		t.Fatalf("repairTargetSet = %+v, want original repair targets preserved", refs)
	}
	summary := mapFromValue(pkg["previousRejectedOutputSummary"])
	if !strings.Contains(stringFromMap(summary, "error"), "not related to failure target scope") {
		t.Fatalf("previousRejectedOutputSummary = %+v, want target admission wrapper retained as summary", summary)
	}
}

func TestRepairActionPackageAnchorsPrimaryFailureAcrossContinuableDecisionWrapperFailure(t *testing.T) {
	decisionFailure := "invalid argument: decision requests a continuable candidate choice"
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"failureClass":                "dod_coverage_missing",
		"failureReason":               decisionFailure,
		"failedStepOutput": map[string]any{
			"stage":                                "llm_decision_required",
			"error":                                decisionFailure,
			failureOutputKeyClass:                  "dod_coverage_missing",
			failureOutputKeySignature:              "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_decision_required|error=" + decisionFailure,
			"continuableDecisionConvertedToRepair": true,
			"convertedDecision": map[string]any{
				"question": "Please provide pkg/langserver/instance.go and pkg/handler/mcp.go.",
				"context": map[string]any{
					"missingFields":       []string{"pkg/langserver/instance.go content", "pkg/handler/mcp.go content"},
					"requiresHumanFields": []string{"pkg/langserver/instance.go content", "pkg/handler/mcp.go content"},
				},
			},
		},
		"readableResourceRepairContext": []map[string]any{{
			"targetRef":        "pkg/langserver/instance.go",
			"exists":           true,
			"contentComplete":  true,
			"safeForFileWrite": true,
			"contentHash":      "hash-instance",
			"content":          "package langserver\n",
		}, {
			"targetRef":        "pkg/handler/mcp.go",
			"exists":           true,
			"contentComplete":  true,
			"safeForFileWrite": true,
			"contentHash":      "hash-handler",
			"content":          "package handler\n",
		}},
		"repairActionPackage": map[string]any{
			"currentFailure": map[string]any{
				"failedStepId":     "run/graph-001/009-verify-build",
				"failedStepKind":   shellcommon.StepKindCommand,
				"failureClass":     failureClassShellCommand,
				"failureSignature": "class=shell_command_failed|kind=shell/command|failureTargets=pkg/lsp/server.go,main.go|exitCode=1",
				"reason":           "pkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
				"targetRef":        "pkg/lsp/server.go,main.go",
				"targetScope":      "failureTargets=pkg/lsp/server.go,main.go",
			},
			"structuredFailureSignals": map[string]any{
				"targetRefs": []string{"pkg/lsp/server.go", "main.go"},
				"lineRefs": []map[string]any{
					{"targetRef": "pkg/lsp/server.go", "line": 10, "column": 24},
					{"targetRef": "main.go", "line": 28, "column": 28},
				},
			},
			"repairTargetSet": []string{"pkg/lsp/server.go", "main.go"},
			"targetSnapshots": []map[string]any{{
				"targetRef":        "pkg/lsp/server.go",
				"exists":           true,
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash-server",
				"content":          "package lsp\n",
			}, {
				"targetRef":        "main.go",
				"exists":           true,
				"contentComplete":  true,
				"safeForFileWrite": true,
				"contentHash":      "hash-main",
				"content":          "package main\n",
			}},
		},
	}

	pkg := repairActionPackageFromRepairInput(input, map[string]any{
		"stage":                   "llm_decision_required",
		"error":                   decisionFailure,
		failureOutputKeyClass:     "dod_coverage_missing",
		failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_decision_required|error=" + decisionFailure,
	})

	currentFailure := mapFromValue(pkg["currentFailure"])
	if currentFailure["failureClass"] != failureClassShellCommand ||
		currentFailure["failedStepId"] != "run/graph-001/009-verify-build" ||
		!strings.Contains(stringFromMap(currentFailure, "reason"), "LSPInstance") {
		t.Fatalf("currentFailure = %+v, want original shell/build failure anchored", currentFailure)
	}
	if refs := stringSliceFromValue(pkg["repairTargetSet"]); !stringSliceContains(refs, "pkg/lsp/server.go") || !stringSliceContains(refs, "main.go") {
		t.Fatalf("repairTargetSet = %+v, want original repair targets preserved", refs)
	}
	targets := mapSliceFromValue(pkg["targetSnapshots"])
	targetRefs := make([]string, 0, len(targets))
	for _, target := range targets {
		targetRefs = append(targetRefs, stringFromMap(target, "targetRef"))
	}
	if !stringSliceContains(targetRefs, "pkg/lsp/server.go") || !stringSliceContains(targetRefs, "main.go") {
		t.Fatalf("targetSnapshots = %+v, want original target snapshots preserved", targets)
	}
	summary := mapFromValue(pkg["previousRejectedOutputSummary"])
	if !strings.Contains(stringFromMap(summary, "error"), "continuable candidate choice") {
		t.Fatalf("previousRejectedOutputSummary = %+v, want continuable decision wrapper retained as summary", summary)
	}
}

func TestRepairActionPackageAnchorsPrimaryFailureFromReplanContinuation(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"failureClass":                failureClassLLMDecisionInvalid,
		"failureReason":               "invalid argument: invalid llm decision json: no json object found",
		"failedStepOutput": map[string]any{
			"stage":               "llm_schema_invalid",
			"error":               "invalid argument: invalid llm decision json: no json object found",
			failureOutputKeyClass: failureClassLLMDecisionInvalid,
		},
		"repairActionPackage": map[string]any{
			"currentFailure": map[string]any{
				"failureClass": failureClassLLMDecisionInvalid,
				"stage":        "llm_schema_invalid",
				"reason":       "invalid argument: invalid llm decision json: no json object found",
			},
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
		"executionContext": map[string]any{
			"replanContinuation": map[string]any{
				stepInputKeyCurrentProgressDigest: map[string]any{
					"currentGap": "pkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
					"lastObservedFailure": map[string]any{
						"stepId":                    "run/graph-001/011-build",
						"kind":                      shellcommon.StepKindCommand,
						"failureClass":              failureClassShellCommand,
						"failureSignature":          "class=shell_command_failed|kind=shell/command|failureTargets=pkg/lsp/server.go,main.go|exitCode=1",
						"concreteFailureDiagnostic": "pkg/lsp/server.go:10:24: undefined: LSPInstance\nmain.go:28:28: undefined: NewServer",
						"evidenceValidity":          "actionable_failure",
					},
				},
			},
		},
	}

	pkg := repairActionPackageFromRepairInput(input, map[string]any{
		"stage":               "llm_schema_invalid",
		"error":               "invalid argument: invalid llm decision json: no json object found",
		failureOutputKeyClass: failureClassLLMDecisionInvalid,
	})

	currentFailure := mapFromValue(pkg["currentFailure"])
	if currentFailure["failureClass"] != failureClassShellCommand ||
		currentFailure["failedStepId"] != "run/graph-001/011-build" ||
		!strings.Contains(stringFromMap(currentFailure, "reason"), "LSPInstance") {
		t.Fatalf("currentFailure = %+v, want replan primary build failure", currentFailure)
	}
	signals := mapFromValue(pkg["structuredFailureSignals"])
	if refs := stringSliceFromValue(signals["targetRefs"]); !stringSliceContains(refs, "pkg/lsp/server.go") || !stringSliceContains(refs, "main.go") {
		t.Fatalf("structuredFailureSignals = %+v, want replan primary target refs", signals)
	}
	if summary := mapFromValue(pkg["previousRejectedOutputSummary"]); summary["stage"] != "llm_schema_invalid" {
		t.Fatalf("previousRejectedOutputSummary = %+v, want wrapper retained only as rejected summary", summary)
	}
}

func TestStructuredFailureSignalsDoNotPromoteRuntimeContractFieldsAsTargetRefs(t *testing.T) {
	output := map[string]any{
		"status":              "completed",
		"stage":               "llm_execution_graph_plan",
		"error":               "invalid argument: strategyReassessment.required=true requires state-changing patch output to declare failedSurface/rootCauseTarget/repairTargetDecision/forbiddenRepair/verificationPlan aligned with current failed surface",
		"reason":              "invalid argument: strategyReassessment.required=true requires state-changing patch output to declare failedSurface/rootCauseTarget/repairTargetDecision/forbiddenRepair/verificationPlan aligned with current failed surface",
		failureOutputKeyClass: failureClassRuntime,
	}

	signals := structuredFailureSignalsFromFailureOutput(output, nil, failureClassRuntime, "")

	targets := stringSliceFromValue(signals["targetRefs"])
	for _, forbidden := range []string{"strategyReassessment.required", "strategyreassessment.required"} {
		if stringSliceContains(targets, forbidden) {
			t.Fatalf("targetRefs = %#v, want runtime contract field %q filtered", targets, forbidden)
		}
	}
	identifiers := stringSliceFromValue(signals["identifiers"])
	if !stringSliceContains(identifiers, "strategyReassessment") || !stringSliceContains(identifiers, "rootCauseTarget") {
		t.Fatalf("identifiers = %#v, want contract field names retained only as identifiers", identifiers)
	}
}

func TestStructuredFailureSignalsDoNotPromoteSchemaFieldPathsAsTargetRefs(t *testing.T) {
	output := map[string]any{
		"stage":               "llm_schema_invalid",
		"error":               "invalid argument: invalid llm decision json: json: cannot unmarshal string into Go struct field DoDCoverage.dodAssessment.acceptanceDimensions.items of type models.DoDCoverageItem; concrete validation nodes must declare validationEvidence.mode and validationEvidence.observationType",
		failureOutputKeyClass: failureClassLLMDecisionInvalid,
	}

	signals := structuredFailureSignalsFromFailureOutput(output, nil, failureClassLLMDecisionInvalid, "")

	targets := stringSliceFromValue(signals["targetRefs"])
	for _, forbidden := range []string{
		"dodcoverage.dodassessment.acceptancedimensions.items",
		"DoDCoverage.dodAssessment.acceptanceDimensions.items",
		"validationevidence.mode",
		"validationEvidence.observationType",
	} {
		if stringSliceContains(targets, forbidden) {
			t.Fatalf("targetRefs = %#v, want schema field path %q filtered", targets, forbidden)
		}
	}
	identifiers := stringSliceFromValue(signals["identifiers"])
	if !stringSliceContains(identifiers, "DoDCoverage") || !stringSliceContains(identifiers, "DoDCoverageItem") {
		t.Fatalf("identifiers = %#v, want schema names retained diagnostically", identifiers)
	}
}

func TestPrimaryDiagnosticRepairRequiresConcreteTargetEvidence(t *testing.T) {
	signals := map[string]any{
		"identifiers": []string{
			"llm_decision_invalid",
			"workspace_execution",
			"llm_schema_invalid",
			"strategyReassessment",
		},
		"messages": []string{
			`invalid argument: invalid llm decision json: json: unknown field "strategyReassessment"`,
		},
	}

	if contract := primaryDiagnosticRepairContractFromSignals(signals); len(contract) != 0 {
		t.Fatalf("primaryDiagnosticRepair = %#v, want omitted without targetRefs or lineRefs", contract)
	}
}

func TestStructuredFailureSignalsFilterExecutionLabelsAndDurations(t *testing.T) {
	output := map[string]any{
		"stderr": strings.Join([]string{
			"# module-name [module-name.test]",
			"./actual_target.go:17:9: undefined: currentShape",
			"FAIL\tmodule-name\t0.03s",
		}, "\n"),
		failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=actual_target.go|exitCode=1",
	}

	signals := structuredFailureSignalsFromFailureOutput(output, errors.New("exit status 1"), failureClassShellCommand, "")

	targets := stringSliceFromValue(signals["targetRefs"])
	if !stringSliceContains(targets, "actual_target.go") {
		t.Fatalf("targetRefs = %#v, want actual_target.go", targets)
	}
	for _, forbidden := range []string{"module-name.test", "0.03s"} {
		if stringSliceContains(targets, forbidden) {
			t.Fatalf("targetRefs = %#v, want execution label/duration %q filtered", targets, forbidden)
		}
	}
}

func TestStructuredFailureSignalsKeepExternalReferencesOutOfTargetRefs(t *testing.T) {
	output := map[string]any{
		"stderr": strings.Join([]string{
			"missing metadata entry for external.example/unit/name",
			"./actual_target.txt:9:1: unresolved external reference",
		}, "\n"),
		failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=actual_target.txt|exitCode=1",
	}

	signals := structuredFailureSignalsFromFailureOutput(output, errors.New("exit status 1"), failureClassShellCommand, "")

	targets := stringSliceFromValue(signals["targetRefs"])
	if stringSliceContains(targets, "external.example/unit/name") {
		t.Fatalf("targetRefs = %#v, want external reference kept out of workspace targets", targets)
	}
	if !stringSliceContains(targets, "actual_target.txt") {
		t.Fatalf("targetRefs = %#v, want actual workspace-shaped target retained", targets)
	}
	externalRefs := stringSliceFromValue(signals["externalReferences"])
	if !stringSliceContains(externalRefs, "external.example/unit/name") {
		t.Fatalf("externalReferences = %#v, want external reference retained diagnostically", externalRefs)
	}
}

func TestRuntimeRepairActionPackageDoesNotUseRuntimeContractFieldsAsRepairTargetSet(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"runtimeRepairAttempt":        1,
		"maxRuntimeRepairAttempts":    10,
		"failedStepId":                "run/step-1",
		"failedStepKind":              llmWorkspaceExecutionStepKind,
		"failureClass":                failureClassRuntime,
		"failureReason":               "invalid argument: strategyReassessment.required=true requires failedSurface/rootCauseTarget/repairTargetDecision/forbiddenRepair/verificationPlan",
		"repairObjective":             repairObjectiveStateChangingOutput,
		"structuredFailureSignals": map[string]any{
			"targetRefs": []string{"strategyReassessment.required", "strategyreassessment.required", "validationEvidence.mode", "validationevidence.observ"},
			"identifiers": []string{
				"strategyReassessment",
				"failedSurface",
				"rootCauseTarget",
				"repairTargetDecision",
				"repairMode",
				"forbiddenRepair",
				"verificationPlan",
			},
		},
	}

	actionPackage := repairActionPackageFromRepairInput(input, map[string]any{
		"stage":               "llm_execution_graph_plan",
		"error":               "invalid argument: strategyReassessment.required=true requires failedSurface/rootCauseTarget/repairTargetDecision/forbiddenRepair/verificationPlan",
		failureOutputKeyClass: failureClassRuntime,
	})

	if targetSet := stringSliceFromValue(actionPackage["repairTargetSet"]); len(targetSet) != 0 {
		t.Fatalf("repairTargetSet = %#v, want runtime contract fields omitted", targetSet)
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustTargetRepairTargetSet"] == true {
		t.Fatalf("requiredOutput = %#v, want no repair target set requirement for runtime contract fields", required)
	}
	signals := mapFromValue(actionPackage["structuredFailureSignals"])
	targetRefs := stringSliceFromValue(signals["targetRefs"])
	if len(targetRefs) != 0 {
		t.Fatalf("structuredFailureSignals.targetRefs = %#v, want contract field refs filtered from package", targetRefs)
	}
}

func TestRuntimeRepairActionPackagePreservesTargetContextAcrossStrategyContractFailure(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"runtimeRepairAttempt":        4,
		"maxRuntimeRepairAttempts":    10,
		"targetRepairAttempts":        3,
		"failedStepId":                "run/repair-003",
		"failedStepKind":              llmWorkspaceExecutionStepKind,
		"failureClass":                failureClassRuntime,
		"failureReason":               "invalid argument: strategyReassessment.required=true requires state-changing patch output to declare failedSurface/rootCauseTarget/repairTargetDecision/repairMode/forbiddenRepair/verificationPlan aligned with current failed surface",
		"failureTargetScope":          "failureTargets=target.go",
		"repairObjective":             repairObjectiveStateChangingOutput,
		"strategyReassessment": map[string]any{
			"required":      true,
			"failedSurface": "failureTargets=target.go",
			"requiredDecisionFields": []string{
				"failedSurface",
				"rootCauseTarget",
				"repairTargetDecision",
				"repairMode",
				"forbiddenRepair",
				"verificationPlan",
			},
		},
		"repairActionPackage": map[string]any{
			"targetSnapshots": []map[string]any{{
				"targetRef":        "target.go",
				"exists":           true,
				"contentHash":      "hash-target",
				"content":          "package target\n",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
			"repairTargetSet": []string{"target.go"},
		},
		"structuredFailureSignals": map[string]any{
			"targetRefs": []string{"strategyReassessment.required"},
			"identifiers": []string{
				"strategyReassessment",
				"failedSurface",
				"rootCauseTarget",
				"repairTargetDecision",
				"repairMode",
				"forbiddenRepair",
				"verificationPlan",
			},
		},
	}

	actionPackage := repairActionPackageFromRepairInput(input, map[string]any{
		"stage":               "llm_execution_graph_plan",
		"error":               input["failureReason"],
		failureOutputKeyClass: failureClassRuntime,
	})

	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true || required["mustDeclareStrategyReassessmentFields"] != true {
		t.Fatalf("requiredOutput = %#v, want state-changing strategy reassessment contract preserved", required)
	}
	if targetSet := stringSliceFromValue(actionPackage["repairTargetSet"]); !stringSliceContains(targetSet, "target.go") {
		t.Fatalf("repairTargetSet = %#v, want inherited target.go preserved", targetSet)
	}
	if targetSet := stringSliceFromValue(actionPackage["repairTargetSet"]); stringSliceContains(targetSet, "strategyReassessment.required") {
		t.Fatalf("repairTargetSet = %#v, want contract marker excluded", targetSet)
	}
	snapshots := mapSliceFromValue(actionPackage["targetSnapshots"])
	if len(snapshots) != 1 || snapshots[0]["targetRef"] != "target.go" {
		t.Fatalf("targetSnapshots = %#v, want inherited target snapshot", snapshots)
	}
	reassessment := mapFromValue(actionPackage["strategyReassessment"])
	if reassessment["failedSurface"] != "failureTargets=target.go" {
		t.Fatalf("strategyReassessment = %#v, want inherited failed surface", reassessment)
	}
}

func TestRuntimeRepairActionPackageUsesEvidenceAcquisitionWithoutConcreteTarget(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"failedStepId":                "run/repair-001",
		"failedStepKind":              llmWorkspaceExecutionStepKind,
		"failureClass":                failureClassLLMDecisionInvalid,
		"failureReason":               "invalid argument: invalid llm decision json: no json object found",
	}

	actionPackage := repairActionPackageFromRepairInput(input, map[string]any{
		"stage":               "llm_schema_invalid",
		"error":               "invalid argument: invalid llm decision json: no json object found",
		failureOutputKeyClass: failureClassLLMDecisionInvalid,
	})

	if actionPackage["evidenceAcquisitionRequired"] != true {
		t.Fatalf("repairActionPackage = %#v, want evidence acquisition required", actionPackage)
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != false || required["mustProduceConcreteFailureEvidence"] != true {
		t.Fatalf("requiredOutput = %#v, want evidence acquisition output contract", required)
	}
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("acceptedKinds = %#v, want no patch/apply without concrete target evidence", required["acceptedKinds"])
	}
	if required["mustReturnValidJSON"] != true {
		t.Fatalf("requiredOutput = %#v, want schema recovery JSON contract preserved", required)
	}
}

func TestRuntimeRepairActionPackageRequiresPatchWithConcreteTargetSnapshot(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"failureTargetRepairContext": []map[string]any{{
			"targetRef":        "main_test.go",
			"exists":           true,
			"contentHash":      "hash-main-test",
			"contentComplete":  true,
			"safeForFileWrite": true,
			"content":          "package main\n",
		}},
		"structuredFailureSignals": map[string]any{
			"targetRefs": []string{"main_test.go"},
			"lineRefs":   []map[string]any{{"targetRef": "main_test.go", "line": 91}},
		},
	}

	actionPackage := repairActionPackageFromRepairInput(input, map[string]any{
		"status": "failed",
		"stderr": "./main_test.go:91:2: cannot assign to struct field in map",
	})

	if actionPackage["evidenceAcquisitionRequired"] == true {
		t.Fatalf("repairActionPackage = %#v, did not expect evidence acquisition with target snapshot", actionPackage)
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true || !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %#v, want patch/apply state-changing contract", required)
	}
	if targets := mapSliceFromValue(actionPackage["targetSnapshots"]); len(targets) != 1 || targets[0]["targetRef"] != "main_test.go" {
		t.Fatalf("targetSnapshots = %#v, want main_test.go snapshot", actionPackage["targetSnapshots"])
	}
}

func TestInitialStepInputReplanUsesEvidenceAcquisitionWithoutConcreteTarget(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-replan-no-target",
		WorkspaceID:     "ws-replan-no-target",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-predecessor",
				"reason":           "continue predecessor",
				"failureContext": map[string]any{
					failureOutputKeyClass: failureClassLLMDecisionInvalid,
					"failedStepKind":      llmWorkspaceExecutionStepKind,
					"failedStepOutput": map[string]any{
						"stage": "llm_schema_invalid",
						"error": "invalid argument: invalid llm decision json: no json object found",
					},
				},
			},
		},
	}

	input := initialStepInput(run)
	if input["stateChangingOutputRequired"] != nil {
		t.Fatalf("initial input = %#v, want no stateChangingOutputRequired without concrete target", input)
	}
	if input["repairObjective"] != repairObjectiveAcquireFailureEvidence || input["concreteFailureEvidenceRequired"] != true {
		t.Fatalf("initial input = %#v, want evidence acquisition objective", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != false || required["mustProduceConcreteFailureEvidence"] != true {
		t.Fatalf("requiredOutput = %#v, want evidence acquisition contract", required)
	}
}

func TestRuntimeRepairEmptyWorkspaceSurfaceEvidenceIncludesGoNoModuleDiagnostic(t *testing.T) {
	failedStep := &runmodel.Step{
		ID:    "run-empty-go-module/follow-up",
		RunID: "run-empty-go-module",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyGraphEvidence: []map[string]any{{
				"kind": "file/list",
				"output": map[string]any{
					"stage":     "file_list",
					"path":      ".",
					"truncated": false,
					"entries": []map[string]any{{
						"path": ".agents",
						"kind": "directory",
					}},
				},
			}, {
				"kind": "shell/command",
				"output": map[string]any{
					"stdout": "# ./...\npattern ./...: directory prefix . does not contain main module or its selected dependencies\nFAIL\t./... [setup failed]\nFAIL",
				},
			}},
		},
	}

	if !runtimeRepairHasConcreteImplementationTargetEvidence(map[string]any{}, failedStep) {
		t.Fatalf("runtimeRepairHasConcreteImplementationTargetEvidence = false, want Go no-module diagnostic to count as empty workspace surface evidence")
	}
}

func TestRuntimeRepairEmptyWorkspaceEvidenceEscalatesToStateChangingRepair(t *testing.T) {
	runID := common.RunID("run-empty-workspace-escalation")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-empty-workspace-escalation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/follow-up-graph-repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			stepInputKeyRuntimeRepairFollowUp: true,
			"repairActionPackage": map[string]any{
				"evidenceAcquisitionRequired": true,
				"requiredOutput": map[string]any{
					"mustProduceConcreteFailureEvidence": true,
				},
			},
			stepInputKeyGraphEvidence: []map[string]any{{
				"kind": "file/list",
				"output": map[string]any{
					"stage":     "file_list",
					"path":      ".",
					"truncated": false,
					"entries": []map[string]any{{
						"path": ".agents",
						"kind": "directory",
					}},
				},
			}, {
				"kind": "shell/command",
				"output": map[string]any{
					"stdout": "# ./...\npattern ./...: directory prefix . does not contain main module or its selected dependencies\nFAIL\t./... [setup failed]\nFAIL",
				},
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			"error":                   "invalid argument: dodAssessment is not satisfied",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment|error=invalid argument: dodassessment is not satisfied",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "The workspace is empty and still needs a state-changing project initialization.",
				"items": []map[string]any{{
					"item":         "生成最小可运行 Go 项目",
					"state":        "failed",
					"evidenceRefs": []string{"run-empty-workspace-escalation/graph-001/list-empty-workspace"},
				}},
				"goalCoverage": map[string]any{
					"satisfied": false,
					"items": []map[string]any{{
						"goalRequirementId": "goal-req-empty-workspace",
						"goal":              "从空 workspace 初始化一个最小 Go 项目",
						"state":             "failed",
						"reason":            "State-changing project initialization has not run.",
						"evidenceRefs":      []string{"run-empty-workspace-escalation/graph-001/list-empty-workspace"},
					}},
				},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingDoDItems": []map[string]any{{
					"item":  "生成最小可运行 Go 项目",
					"state": "failed",
				}},
				"missingGoalCoverage": []map[string]any{{
					"goalRequirementId": "goal-req-empty-workspace",
					"item":              "从空 workspace 初始化一个最小 Go 项目",
					"state":             "failed",
					"reason":            "State-changing project initialization has not run.",
				}},
			},
		},
	}

	input := (&RuntimeOrchestrator{steps: memory.NewStepRepository()}).runtimeRepairFollowUpStepInput(context.Background(), run, failedStep, 2, errors.New("invalid argument: dodAssessment is not satisfied"))

	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		boolValue(input["concreteFailureEvidenceRequired"]) {
		t.Fatalf("runtime repair input = %#v, want state-changing repair after empty workspace evidence", input)
	}
	if input["emptyWorkspaceEvidenceEscalatedToImplementation"] != true {
		t.Fatalf("emptyWorkspaceEvidenceEscalatedToImplementation = %#v, want true", input["emptyWorkspaceEvidenceEscalatedToImplementation"])
	}
}

func TestRuntimeRepairActionPackageIncludesStructuredFailureSignalsAndForbiddenReuse(t *testing.T) {
	runID := common.RunID("run-repair-grounding")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"status": "failed",
			"stderr": `./provider_matrix_test.go:51:32: undefined: handleChatCompletions
./provider_matrix_test.go:22:42: unknown field expectedVendor in struct literal of type providerMatrixCase`,
			"error":                   "file/edit oldText not found; patch diff contains redacted runtime markers",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=provider_matrix_test.go|exitCode=1",
		},
	}
	orch := &RuntimeOrchestrator{steps: memory.NewStepRepository()}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-grounding"}, failedStep, 1, errors.New("exit status 1"))

	actionPackage := mapFromValue(input["repairActionPackage"])
	if len(actionPackage) == 0 {
		t.Fatalf("repairActionPackage = %#v, want actionable package", input["repairActionPackage"])
	}
	lastFailure := mapFromValue(actionPackage["lastFailureDiagnostics"])
	if !strings.Contains(stringFromMap(lastFailure, "stderrPreview"), "provider_matrix_test.go:51:32") {
		t.Fatalf("lastFailureDiagnostics = %#v, want latest stderr preview preserved", lastFailure)
	}
	currentFailure := mapFromValue(actionPackage["currentFailure"])
	if !strings.Contains(stringFromMap(currentFailure, "stderrPreview"), "provider_matrix_test.go:51:32") {
		t.Fatalf("currentFailure = %#v, want actionable stderr preview preserved", currentFailure)
	}
	signals := mapFromValue(actionPackage["structuredFailureSignals"])
	if !stringSliceContains(stringSliceFromValue(signals["targetRefs"]), "provider_matrix_test.go") {
		t.Fatalf("repairActionPackage.structuredFailureSignals = %#v, want target ref", actionPackage["structuredFailureSignals"])
	}
	if !stringSliceContains(stringSliceFromValue(signals["identifiers"]), "handleChatCompletions") {
		t.Fatalf("repairActionPackage.structuredFailureSignals = %#v, want identifier", actionPackage["structuredFailureSignals"])
	}
	forbidden := stringSliceFromValue(actionPackage["forbiddenContentReuse"])
	for _, want := range []string{"missing_old_text", "redacted_runtime_marker_text"} {
		if !stringSliceContains(forbidden, want) {
			t.Fatalf("forbiddenContentReuse = %#v, want %q", actionPackage["forbiddenContentReuse"], want)
		}
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustRebuildFromCurrentEvidence"] != true {
		t.Fatalf("requiredOutput = %#v, want mustRebuildFromCurrentEvidence", required)
	}
}

func TestRuntimeRepairActionPackageOmitsRawRedactedFailureProducts(t *testing.T) {
	runID := common.RunID("run-repair-grounding-redacted")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			"status":                  "failed",
			"error":                   "invalid character '}' after array element",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			"rawContentPreview":       `{"diff":"@@ -1 +1 @@\n+[REDACTED]"}`,
			"repairRawContentPreview": `{"content":"[REDACTED]"}`,
			"streamEvents": []map[string]any{{
				"delta": `{"content":"[REDACTED]"}`,
			}},
		},
	}
	orch := &RuntimeOrchestrator{steps: memory.NewStepRepository()}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-grounding-redacted"}, failedStep, 1, common.ErrInvalidArgument)
	actionPackage := mapFromValue(input["repairActionPackage"])
	encoded, err := json.Marshal(actionPackage)
	if err != nil {
		t.Fatalf("marshal repairActionPackage: %v", err)
	}
	if strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("repairActionPackage exposed redacted marker: %s", encoded)
	}
	summary := mapFromValue(actionPackage["previousRejectedOutputSummary"])
	if summary["rawContentPreviewOmitted"] != true || summary["streamEventsOmitted"] != true {
		t.Fatalf("previousRejectedOutputSummary = %#v, want invalid raw products omitted", summary)
	}
	forbidden := stringSliceFromValue(actionPackage["forbiddenContentReuse"])
	for _, want := range []string{"raw_invalid_llm_decision_content", "redacted_runtime_marker_text"} {
		if !stringSliceContains(forbidden, want) {
			t.Fatalf("forbiddenContentReuse = %#v, want %q", actionPackage["forbiddenContentReuse"], want)
		}
	}
}

func TestRuntimeRepairActionPackageOmitsRedactedPartialOutputPreview(t *testing.T) {
	runID := common.RunID("run-repair-grounding-redacted-partial")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			"stage":                   "llm_execution_graph_plan",
			"status":                  "completed",
			"error":                   "invalid argument: patch diff contains redacted runtime markers",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
			"streamEvents": []map[string]any{{
				"delta": `{"diff":"@@ -1 +1 @@\n-[REDACTED]\n+safe\n"}`,
			}},
		},
	}
	orch := &RuntimeOrchestrator{steps: memory.NewStepRepository()}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-grounding-redacted-partial"}, failedStep, 1, common.ErrInvalidArgument)
	actionPackage := mapFromValue(input["repairActionPackage"])
	encoded, err := json.Marshal(actionPackage)
	if err != nil {
		t.Fatalf("marshal repairActionPackage: %v", err)
	}
	if strings.Contains(string(encoded), "[REDACTED]") || strings.Contains(string(encoded), "partialOutputPreview\"") {
		t.Fatalf("repairActionPackage exposed redacted partial output: %s", encoded)
	}
	summary := mapFromValue(actionPackage["previousRejectedOutputSummary"])
	if summary["partialOutputPreviewOmitted"] != true {
		t.Fatalf("previousRejectedOutputSummary = %#v, want partial preview omitted", summary)
	}
}

func TestRuntimeRepairInputCompactsDuplicatedHistoricalFieldsAroundActionPackage(t *testing.T) {
	runID := common.RunID("run-repair-grounding-compact")
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"largeOriginalInput":          strings.Repeat("x", 16*1024),
		},
		Output: map[string]any{
			"status":                  "failed",
			"stderr":                  "./target.txt:7:3: MissingThing",
			"error":                   "exit status 1",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=target.txt|exitCode=1",
			stepOutputKeyExecutionGraph: map[string]any{
				"nodes": []map[string]any{{
					"id":   "inspect",
					"kind": "file/read",
				}},
			},
		},
	}
	orch := &RuntimeOrchestrator{steps: memory.NewStepRepository()}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-repair-grounding-compact"}, failedStep, 1, errors.New("exit status 1"))

	if len(mapFromValue(input["repairActionPackage"])) == 0 {
		t.Fatalf("repairActionPackage = %#v, want package", input["repairActionPackage"])
	}
	for _, key := range []string{"failedStepInput", "failedStepOutput", "previousStepOutput", "failedExecutionGraph", stepOutputKeyExecutionGraph} {
		if _, ok := input[key]; ok {
			t.Fatalf("input[%s] still present after repair action package compaction", key)
		}
	}
	compaction := mapFromValue(input["repairInputCompaction"])
	if compaction["mode"] != "repair_action_package_primary" || !stringSliceContains(stringSliceFromValue(compaction["omittedFields"]), "failedStepOutput") {
		t.Fatalf("repairInputCompaction = %#v, want omitted field metadata", compaction)
	}
}

func TestRuntimeRepairActionPackageMarksRepeatedTargetRepairMode(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"targetRepairAttempts":        2,
		"failureTargetScope":          "target.txt",
		"repairObjective":             repairObjectiveStateChangingOutput,
		"failureTargetRepairContext": []map[string]any{{
			"targetRef":        "target.txt",
			"exists":           true,
			"contentHash":      "hash-1",
			"content":          "current content",
			"contentComplete":  true,
			"safeForFileWrite": true,
		}},
		"structuredFailureSignals": map[string]any{
			"targetRefs":  []string{"target.txt"},
			"identifiers": []string{"MissingThing"},
		},
	}

	actionPackage := repairActionPackageFromRepairInput(input, map[string]any{
		"error":                   "patch does not apply",
		failureOutputKeySignature: "class=patch_apply_failed|targetRef=target.txt",
	})

	repeated := mapFromValue(actionPackage["repeatedTargetRepairMode"])
	if repeated["enabled"] != true || repeated["mustPreferExactTargetSnapshot"] != true {
		t.Fatalf("repeatedTargetRepairMode = %#v, want strict repeated target mode", repeated)
	}
	reassessment := mapFromValue(actionPackage["strategyReassessment"])
	if reassessment["required"] != true || reassessment["reason"] != "same_target_scope_repeated_failure" {
		t.Fatalf("strategyReassessment = %#v, want repeated target reassessment", reassessment)
	}
	requiredFields := stringSliceFromValue(reassessment["requiredDecisionFields"])
	if !stringSliceContains(requiredFields, "rootCauseTarget") || !stringSliceContains(requiredFields, "repairTargetDecision") {
		t.Fatalf("requiredDecisionFields = %#v, want root cause and target decision fields", reassessment["requiredDecisionFields"])
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustUseCurrentTargetSnapshot"] != true || required["mustRebuildFromCurrentEvidence"] != true {
		t.Fatalf("requiredOutput = %#v, want repeated repair target constraints", required)
	}
	if required["mustDeclareStrategyReassessmentFields"] != true ||
		!stringSliceContains(stringSliceFromValue(required["strategyReassessmentRequiredFields"]), "rootCauseTarget") ||
		required["mustReassessRepeatedFailedSurface"] != true {
		t.Fatalf("requiredOutput = %#v, want explicit strategy reassessment output contract", required)
	}
	if rule := stringFromAny(required["strategyReassessmentMatchRule"]); !strings.Contains(rule, "inside the patch/apply input object") || strings.Contains(rule, "top-level decision fields") == false {
		t.Fatalf("strategyReassessmentMatchRule = %q, want explicit placement rule", rule)
	}
	diagnostics := mapFromValue(actionPackage["targetRelevanceDiagnostics"])
	if diagnostics["targetSnapshotCount"] != 1 || diagnostics["failureSignalIdentifierCount"] != 1 {
		t.Fatalf("targetRelevanceDiagnostics = %#v, want target and signal diagnostics", diagnostics)
	}
}

func TestRuntimeRepairActionPackageRefreshesStrategySurfaceFromConcreteTargets(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"runtimeRepairAttempt":        5,
		"repairObjective":             repairObjectiveStateChangingOutput,
		failureOutputKeyClass:         failureClassDoDCoverageMissing,
		failureOutputKeySignature:     "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
		"strategyReassessment": map[string]any{
			"required":      true,
			"failedSurface": "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
		},
		"repairTargetSet": []string{"config.yaml"},
		"targetSnapshots": []map[string]any{{
			"targetRef":        "config.yaml",
			"exists":           true,
			"contentHash":      "hash-config",
			"content":          "port: 8080\n",
			"contentComplete":  true,
			"safeForFileWrite": true,
		}},
	}

	actionPackage := repairActionPackageFromRepairInput(input, map[string]any{
		"status": "failed",
		"error":  "coverage still missing",
	})

	reassessment := mapFromValue(actionPackage["strategyReassessment"])
	if got := stringFromMap(reassessment, "failedSurface"); got != "targetRef=config.yaml" {
		t.Fatalf("strategyReassessment.failedSurface = %q, want concrete targetRef surface", got)
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustDeclareStrategyReassessmentFields"] != true {
		t.Fatalf("requiredOutput = %#v, want strategy reassessment contract", required)
	}
}

func TestRuntimeRepairActionPackageRequiresStrategyFieldsForDeclaredRelatedTargetSet(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"targetRepairAttempts":        2,
		"failureTargetScope":          "failureTargets=main.go",
		"repairObjective":             repairObjectiveStateChangingOutput,
		failureOutputKeyClass:         failureClassShellCommand,
		"failureTargetRepairContext": []map[string]any{{
			"targetRef":        "main.go",
			"exists":           true,
			"contentHash":      "hash-main",
			"content":          "current content",
			"contentComplete":  true,
			"safeForFileWrite": true,
		}},
		"structuredFailureSignals": map[string]any{
			"targetRefs": []string{"main.go", "related.lock"},
			"messages": []string{
				"main.go:10:2: missing related.lock entry",
			},
		},
	}

	actionPackage := repairActionPackageFromRepairInput(input, map[string]any{
		"status": "failed",
		"stderr": "main.go:10:2: missing related.lock entry",
	})

	targetSet := stringSliceFromValue(actionPackage["repairTargetSet"])
	if !stringSliceContains(targetSet, "main.go") || !stringSliceContains(targetSet, "related.lock") {
		t.Fatalf("repairTargetSet = %#v, want failed surface and declared related target", actionPackage["repairTargetSet"])
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustDeclareStrategyReassessmentFields"] != true ||
		required["mustTargetRepairTargetSet"] != true ||
		required["mustReassessRepeatedFailedSurface"] != true {
		t.Fatalf("requiredOutput = %#v, want target set plus explicit strategy reassessment contract", required)
	}
	fields := stringSliceFromValue(required["strategyReassessmentRequiredFields"])
	for _, want := range []string{"failedSurface", "rootCauseTarget", "repairTargetDecision", "repairMode", "forbiddenRepair", "verificationPlan"} {
		if !stringSliceContains(fields, want) {
			t.Fatalf("strategyReassessmentRequiredFields = %#v, want %q", fields, want)
		}
	}
}

func TestRuntimeRepairActionPackageRequiresDirectPatchFromCurrentSnapshot(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"failureTargetRepairContext": []map[string]any{{
			"targetRef":        "main.go",
			"exists":           true,
			"contentHash":      "hash-main",
			"content":          "package main\n",
			"contentComplete":  true,
			"safeForFileWrite": true,
		}},
		"structuredFailureSignals": map[string]any{
			"targetRefs": []string{"main.go"},
			"messages":   []string{"main.go failed validation"},
		},
	}

	actionPackage := repairActionPackageFromRepairInput(input, map[string]any{
		"status": "failed",
		"error":  "exit status 1",
	})

	required := mapFromValue(actionPackage["requiredOutput"])
	if required["mustUseCurrentTargetSnapshot"] != true ||
		required["mustPatchDirectlyFromProvidedSnapshot"] != true {
		t.Fatalf("requiredOutput = %#v, want direct current-snapshot patch contract", required)
	}
	if !strings.Contains(stringFromAny(required["sameGraphReadPatchRule"]), "do not put file/read before patch/apply") {
		t.Fatalf("sameGraphReadPatchRule = %q, want read-before-patch prohibition", required["sameGraphReadPatchRule"])
	}
	if !strings.Contains(stringFromAny(required["patchApplyFieldRule"]), "text_replace requires") {
		t.Fatalf("patchApplyFieldRule = %q, want patch field contract", required["patchApplyFieldRule"])
	}
	for _, want := range []string{"same_graph_file_read_before_patch", "incomplete_patch_apply_input"} {
		if !stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), want) {
			t.Fatalf("requiredOutput.forbiddenKinds = %#v, want %q", required["forbiddenKinds"], want)
		}
	}
}

func TestRuntimeRepairActionPackageGroundsCurrentPatchFailure(t *testing.T) {
	input := map[string]any{
		"stateChangingOutputRequired": true,
		"runtimeRepairAttempt":        3,
		"maxRuntimeRepairAttempts":    10,
		"targetRepairAttempts":        1,
		"failedStepId":                "run/graph-002/patch",
		"failedStepKind":              "patch/apply",
		"failureClass":                failureClassPatchApply,
		"failureSignature":            "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|targetRef=target.txt|error=context mismatch",
		"failureTargetScope":          "targetRef=target.txt",
		"patchRepairContext": []map[string]any{{
			"targetRef":        "target.txt",
			"exists":           true,
			"contentHash":      "sha256-current",
			"content":          "current content\n",
			"contentComplete":  true,
			"safeForFileWrite": true,
		}},
		"structuredFailureSignals": map[string]any{
			"targetRefs": []string{"target.txt"},
			"messages":   []string{"unified diff hunk context mismatch"},
		},
	}
	actionPackage := repairActionPackageFromRepairInput(input, map[string]any{
		"stage":                   "patch_apply",
		"targetRef":               "target.txt",
		"patchType":               "unified_diff",
		"error":                   "invalid argument: unified diff hunk context mismatch",
		failureOutputKeyClass:     failureClassPatchApply,
		failureOutputKeySignature: "class=patch_apply_failed|kind=patch/apply|stage=patch_apply|targetRef=target.txt|error=context mismatch",
		"runtimeDiagnostics": map[string]any{
			"kind":        failureClassPatchApply,
			"targetRef":   "target.txt",
			"patchType":   "unified_diff",
			"errorDetail": "unified_diff_hunk_context_mismatch",
		},
	})

	currentFailure := mapFromValue(actionPackage["currentFailure"])
	if currentFailure["failureClass"] != failureClassPatchApply ||
		currentFailure["failedStepId"] != "run/graph-002/patch" ||
		currentFailure["targetRef"] != "target.txt" {
		t.Fatalf("currentFailure = %#v, want current patch failure", currentFailure)
	}
	digest := mapFromValue(actionPackage["failedPatchDigest"])
	if digest["targetRef"] != "target.txt" || digest["stalePatchRejected"] != true {
		t.Fatalf("failedPatchDigest = %#v, want stale target digest", digest)
	}
	required := mapFromValue(actionPackage["requiredOutput"])
	if !strings.Contains(stringFromMap(required, "fileWriteExistingTargetRule"), "baseHash") ||
		!strings.Contains(stringFromMap(required, "fileWriteExistingTargetRule"), "contentHash") {
		t.Fatalf("requiredOutput.fileWriteExistingTargetRule = %#v, want explicit baseHash/contentHash rule", required["fileWriteExistingTargetRule"])
	}
	if actionPackage["requiredRepairMode"] != "patch_current_snapshot_only" {
		t.Fatalf("requiredRepairMode = %#v, want patch_current_snapshot_only", actionPackage["requiredRepairMode"])
	}
	forbidden := stringSliceFromValue(actionPackage["forbiddenActions"])
	for _, want := range []string{"read_only_graph_for_state_changing_repair", "modify_target_outside_repair_target_set", "reuse_stale_patch_hunk"} {
		if !stringSliceContains(forbidden, want) {
			t.Fatalf("forbiddenActions = %#v, want %q", forbidden, want)
		}
	}
	targetSet := stringSliceFromValue(actionPackage["repairTargetSet"])
	if len(targetSet) != 1 || targetSet[0] != "target.txt" {
		t.Fatalf("repairTargetSet = %#v, want target.txt", actionPackage["repairTargetSet"])
	}
	budget := mapFromValue(actionPackage["repairBudget"])
	if budget["budgetKind"] != "patch_apply_repair" || budget["attempt"] != int64(3) {
		t.Fatalf("repairBudget = %#v, want patch apply budget state", budget)
	}
}
