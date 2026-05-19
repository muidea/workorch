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
			"targetRefs": []string{"strategyReassessment.required", "strategyreassessment.required"},
			"identifiers": []string{
				"strategyReassessment",
				"failedSurface",
				"rootCauseTarget",
				"repairTargetDecision",
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
		required["mustDistinguishRootCauseTarget"] != true {
		t.Fatalf("requiredOutput = %#v, want explicit strategy reassessment output contract", required)
	}
	diagnostics := mapFromValue(actionPackage["targetRelevanceDiagnostics"])
	if diagnostics["targetSnapshotCount"] != 1 || diagnostics["failureSignalIdentifierCount"] != 1 {
		t.Fatalf("targetRelevanceDiagnostics = %#v, want target and signal diagnostics", diagnostics)
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
		required["mustDistinguishRootCauseTarget"] != true {
		t.Fatalf("requiredOutput = %#v, want target set plus explicit strategy reassessment contract", required)
	}
	fields := stringSliceFromValue(required["strategyReassessmentRequiredFields"])
	for _, want := range []string{"failedSurface", "rootCauseTarget", "repairTargetDecision", "forbiddenRepair", "verificationPlan"} {
		if !stringSliceContains(fields, want) {
			t.Fatalf("strategyReassessmentRequiredFields = %#v, want %q", fields, want)
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
