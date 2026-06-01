package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestLLMContextSubsystemUsesStableCompactionRules(t *testing.T) {
	llmContext := newLLMContextSubsystem()
	input := llmContext.compactStepInput(map[string]any{
		"executionContext": map[string]any{"large": strings.Repeat("x", 256)},
		"repairActionPackage": map[string]any{
			"targetSnapshots": []map[string]any{{
				"targetRef":        "main.go",
				"content":          "package main\n",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
	})
	if _, ok := input["executionContext"]; ok {
		t.Fatalf("executionContext should be dropped from subsystem compact input")
	}
	targets := mapSliceFromValue(mapFromValue(input["repairActionPackage"])["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main.go" {
		t.Fatalf("targetSnapshots = %+v, want compact repair target snapshot", targets)
	}

	output := llmContext.compactStepOutput(map[string]any{
		"stderr": strings.Repeat("failure\n", 1024),
	})
	if stderr := mapFromValue(output["stderr"]); stderr["truncated"] != true {
		t.Fatalf("stderr = %+v, want compacted truncated evidence text", stderr)
	}
}

func TestCompactLLMContextDropsHeavyTransportFields(t *testing.T) {
	largeRepairContent := "package main\n" + strings.Repeat("func currentShape() {}\n", 300)
	input := compactStepInputForLLMContext(map[string]any{
		"executionContext":                  map[string]any{"skills": []any{map[string]any{"instruction": strings.Repeat("s", 256)}}},
		stepInputKeyExecutionContextPackage: map[string]any{"workspaceFileManifest": []map[string]any{{"path": ".git/object"}}},
		stepInputKeyWorkspaceFileManifest:   []map[string]any{{"path": ".git/object"}},
		"failedStepInput": map[string]any{
			"executionContext": map[string]any{"memories": []any{map[string]any{"content": strings.Repeat("m", 256)}}},
		},
		"failedStepOutput": map[string]any{"stderr": strings.Repeat("e", 256)},
		"previousStepOutput": map[string]any{
			stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{map[string]any{"id": "inspect", "kind": "file/list"}}},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{"mustIncludeStateChangingNode": true, "acceptedKinds": []string{"patch/apply"}},
			"targetSnapshots": []map[string]any{{
				"targetRef":        "main.go",
				"contentHash":      "hash-main",
				"contentComplete":  true,
				"safeForFileWrite": true,
				"redacted":         false,
				"truncated":        false,
				"content":          largeRepairContent,
			}},
		},
		"failedStepId":  "run-1/block/test",
		"failureReason": strings.Repeat("x", maxLLMContextStringBytes+16),
	})
	if _, ok := input["executionContext"]; ok {
		t.Fatalf("executionContext should be dropped from compact input")
	}
	if _, ok := input[stepInputKeyExecutionContextPackage]; ok {
		t.Fatalf("executionContextPackage should be dropped from compact input")
	}
	if _, ok := input[stepInputKeyWorkspaceFileManifest]; ok {
		t.Fatalf("workspaceFileManifest should be dropped from compact input")
	}
	for _, key := range []string{"failedStepInput", "failedStepOutput", "previousStepOutput"} {
		if _, ok := input[key]; ok {
			t.Fatalf("%s should be dropped from compact input", key)
		}
	}
	if got := fmt.Sprint(input["failureReason"]); len(got) > maxLLMContextStringBytes+3 {
		t.Fatalf("failureReason length = %d, want truncated", len(got))
	}
	actionPackage := mapFromValue(input["repairActionPackage"])
	if len(actionPackage) == 0 {
		t.Fatalf("repairActionPackage = %#v, want retained actionable package", input["repairActionPackage"])
	}
	if targets := mapSliceFromValue(actionPackage["targetSnapshots"]); len(targets) != 1 || targets[0]["targetRef"] != "main.go" {
		t.Fatalf("repairActionPackage.targetSnapshots = %#v, want compact target snapshot", actionPackage["targetSnapshots"])
	} else if got, want := stringFromMap(targets[0], "content"), strings.TrimSpace(largeRepairContent); got != want {
		t.Fatalf("repairActionPackage.targetSnapshots[0].content length = %d, want untruncated safe target length %d", len(got), len(want))
	}

	output := compactStepOutputForLLMContext(map[string]any{
		"stderr":       strings.Repeat("e", maxLLMContextStringBytes+10),
		"failureClass": failureClassShellCommand,
		"provider":     map[string]any{"id": "old-provider", "baseUrl": "http://127.0.0.1:8090/v1"},
		"contextBudget": map[string]any{
			"actualBytes": 1200,
			"providerId":  "old-provider",
			"model":       "old-model",
			"profile":     map[string]any{"maxInputTokens": 1},
		},
		"llmUsage": map[string]any{
			"inputTokens": 10,
			"providerId":  "old-provider",
			"provider":    "compatible",
			"model":       "old-model",
		},
		"nested": map[string]any{
			"executionContext":                  map[string]any{"large": "value"},
			stepInputKeyExecutionContextPackage: map[string]any{"large": "value"},
			"failedStepInput":                   map[string]any{"large": "value"},
		},
	})
	if output["failureClass"] != failureClassShellCommand {
		t.Fatalf("failureClass = %v, want preserved", output["failureClass"])
	}
	if output["provider"] != nil || output["providerId"] != nil {
		t.Fatalf("output = %+v, want provider binding stripped", output)
	}
	if budget := mapFromValue(output["contextBudget"]); budget["providerId"] != nil || budget["model"] != nil || budget["profile"] != nil {
		t.Fatalf("contextBudget = %+v, want provider binding stripped", budget)
	}
	if usage := mapFromValue(output["llmUsage"]); usage["providerId"] != nil || usage["provider"] != nil || usage["model"] != nil {
		t.Fatalf("llmUsage = %+v, want provider binding stripped", usage)
	}
	stderr := mapFromValue(output["stderr"])
	if len(stderr) == 0 ||
		stderr["truncated"] != true ||
		int64FromValue(stderr["bytes"]) <= maxLLMContextEvidenceTextBytes ||
		!strings.HasSuffix(fmt.Sprint(stderr["textPreview"]), "...") {
		t.Fatalf("stderr = %#v, want compact evidence text summary", output["stderr"])
	}
	nested := mapFromValue(output["nested"])
	if _, ok := nested[stepInputKeyExecutionContextPackage]; ok {
		t.Fatalf("nested executionContextPackage should be dropped")
	}
	if _, ok := nested["executionContext"]; ok {
		t.Fatalf("nested executionContext should be dropped")
	}
	if _, ok := nested["failedStepInput"]; ok {
		t.Fatalf("nested failedStepInput should be dropped")
	}
}

func TestCompactLLMContextOmitsArtifactChunkContent(t *testing.T) {
	chunkContent := "line 1\nline 2\nline 3\n"
	input := compactStepInputForLLMContext(map[string]any{
		"outputMode": outputModeIncrementalArtifact,
		"artifactContinuationContext": map[string]any{
			"id":                  "continuation-001",
			"runId":               "run-001",
			"artifactId":          "artifact-001",
			"targetRef":           "deliverable.txt",
			"operation":           "replace",
			"expectedSequence":    2,
			"previousChunkTail":   "line 1\n",
			"receivedSequences":   []int{1},
			"receivedChunkHashes": map[int]string{1: "sha256:previous"},
			"coverageTargets":     []string{"cwi-001"},
			"content":             chunkContent,
			"body":                chunkContent,
		},
	})
	continuation := mapFromValue(input["artifactContinuationContext"])
	if continuation["artifactId"] != "artifact-001" || continuation["expectedSequence"] != 2 {
		t.Fatalf("artifactContinuationContext = %+v, want compact current cursor", continuation)
	}
	if continuation["content"] != nil || continuation["body"] != nil || continuation["previousChunkTail"] != nil {
		t.Fatalf("artifactContinuationContext = %+v, want no raw body", continuation)
	}

	output := compactStepOutputForLLMContext(map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable.txt",
			"sequence":   2,
			"content":    chunkContent,
			"isFinal":    false,
			"chunkHash":  "sha256:chunk",
			"endsWith":   "line 3",
		},
		stepOutputKeyLLMDecision: map[string]any{
			"artifactChunk": map[string]any{
				"artifactId": "artifact-001",
				"targetRef":  "deliverable.txt",
				"sequence":   2,
				"content":    chunkContent,
				"isFinal":    false,
				"chunkHash":  "sha256:chunk",
			},
		},
	})
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal compact artifact output: %v", err)
	}
	if strings.Contains(string(encoded), chunkContent) || strings.Contains(string(encoded), `"content":"`) {
		t.Fatalf("compact artifact output retained chunk content: %s", encoded)
	}
	chunk := mapFromValue(output["artifactChunk"])
	if chunk["contentOmitted"] != true || chunk["contentBytes"] != len([]byte(strings.TrimSpace(chunkContent))) {
		t.Fatalf("artifactChunk = %+v, want omitted content metadata", chunk)
	}
	decisionChunk := mapFromValue(mapFromValue(output[stepOutputKeyLLMDecision])["artifactChunk"])
	if decisionChunk["contentOmitted"] != true {
		t.Fatalf("llmDecision.artifactChunk = %+v, want omitted content metadata", decisionChunk)
	}
}

func TestRuntimeContextLineageCompactionDropsRawFailureArtifacts(t *testing.T) {
	rawContent := "package main\nconst apiKey = \"[REDACTED]\"\n"
	continuation := compactReplanContinuationForRuntimeContext(map[string]any{
		"predecessorRunId": "run-before",
		"reason":           "decision_replan",
		"ticketContext": map[string]any{
			"failureClass":     failureClassRuntime,
			"failureSignature": "class=runtime_failed|kind=llm/workspace_execution|failureTargets=main.go",
			"failedStepInput": map[string]any{
				"executionContext": map[string]any{"large": strings.Repeat("x", 4096)},
			},
			"failureTargetRepairContext": []map[string]any{{
				"targetRef":        "main.go",
				"contentHash":      "hash-main",
				"contentComplete":  true,
				"safeForFileWrite": true,
				"content":          rawContent,
			}},
			"failedStepOutput": map[string]any{
				"stage":             "llm_schema_invalid",
				"rawContentPreview": `{"diff":"[REDACTED]"}`,
				"streamEvents": []map[string]any{{
					"delta": `{"content":"[REDACTED]"}`,
				}},
			},
		},
	})
	encoded, err := json.Marshal(continuation)
	if err != nil {
		t.Fatalf("marshal compact continuation: %v", err)
	}
	text := string(encoded)
	if strings.Contains(text, rawContent) || strings.Contains(text, "failedStepInput") || strings.Contains(text, "[REDACTED]") {
		t.Fatalf("compact continuation retained raw failure artifacts: %s", text)
	}
	ticketContext := mapFromValue(continuation["ticketContext"])
	targets := mapSliceFromValue(ticketContext["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "main.go" || targets[0]["content"] != nil {
		t.Fatalf("targetSnapshots = %#v, want metadata-only lineage target", ticketContext["targetSnapshots"])
	}
	failedOutput := mapFromValue(ticketContext["failedStepOutput"])
	if failedOutput["rawContentPreviewOmitted"] != true || failedOutput["streamEventsOmitted"] != true {
		t.Fatalf("failedStepOutput = %#v, want invalid LLM raw content omitted", failedOutput)
	}
}

func TestCompactStepOutputForLLMContextOmitsRedactedRejectedPatchFragments(t *testing.T) {
	output := compactStepOutputForLLMContext(map[string]any{
		"status":            "failed",
		"error":             "invalid argument: patch diff contains redacted runtime markers",
		"rawContentPreview": `{"patches":[{"targetRef":"main.go","diff":"@@ -1 +1 @@\n+token [REDACTED]"}]}`,
		"patchRequests": []map[string]any{{
			"targetRef": "main.go",
			"patchType": "file_write",
			"content":   "package main\nconst token = \"[REDACTED]\"\n",
		}},
		"llmDecision": map[string]any{
			stepOutputKeyExecutionGraph: map[string]any{
				"nodes": []map[string]any{{
					"id":   "patch",
					"kind": "patch/apply",
					"input": map[string]any{
						"targetRef": "main.go",
						"patchType": "unified_diff",
						"diff":      "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n+token [REDACTED]\n",
					},
				}},
			},
		},
	})

	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal compact output: %v", err)
	}
	if strings.Contains(string(encoded), "[REDACTED]") {
		t.Fatalf("compact output still exposes redacted marker: %s", encoded)
	}
	if omitted := mapFromValue(output["rawContentPreview"]); omitted["omitted"] != true || omitted["containedRuntimeRedactionMarker"] != true {
		t.Fatalf("rawContentPreview = %#v, want omitted redacted content summary", output["rawContentPreview"])
	}
	requests, _ := output["patchRequests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("patchRequests = %#v, want one compact request", output["patchRequests"])
	}
	request := mapFromValue(requests[0])
	if request["targetRef"] != "main.go" || mapFromValue(request["content"])["omitted"] != true {
		t.Fatalf("patchRequests[0] = %#v, want target preserved and content omitted", request)
	}
}
