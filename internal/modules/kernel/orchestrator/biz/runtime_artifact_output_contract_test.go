package orchestrator

import (
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateIncrementalArtifactPlanOutputContractAcceptsPlanOnly(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-plan-ok", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-plan-ok/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_plan",
			},
		},
	}
	output := map[string]any{
		"artifactPlan": map[string]any{
			"artifacts": []map[string]any{{
				"artifactId":      "artifact-001",
				"targetRef":       "deliverable/main.go",
				"operation":       "replace",
				"coverageTargets": []string{"goal-1", "dod-2"},
				"estimatedChunks": 2,
				"estimatedBytes":  4096,
			}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v", err)
	}
}

func TestValidateIncrementalArtifactPlanOutputContractRejectsExecutionGraph(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-plan-graph", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-plan-graph/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_plan",
			},
		},
	}
	output := map[string]any{
		"artifactPlan": map[string]any{
			"artifacts": []map[string]any{{
				"artifactId":      "artifact-001",
				"targetRef":       "deliverable/main.go",
				"operation":       "replace",
				"coverageTargets": []string{"goal-1"},
				"estimatedChunks": 1,
				"estimatedBytes":  512,
			}},
		},
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{"id": "apply", "kind": "patch/apply"}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "must not include executionGraph") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want executionGraph rejection", err)
	}
}

func TestValidateIncrementalArtifactOutputContractRejectsResponseOutputMode(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-output-mode-response", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-output-mode-response/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_plan",
			},
		},
	}
	output := map[string]any{
		"outputMode": outputModeIncrementalArtifact,
		"artifactPlan": map[string]any{
			"artifacts": []map[string]any{{
				"artifactId":      "artifact-001",
				"targetRef":       "deliverable/main.go",
				"operation":       "replace",
				"coverageTargets": []string{"goal-1"},
				"estimatedChunks": 1,
				"estimatedBytes":  512,
			}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "must not include outputMode") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want outputMode rejection", err)
	}
}

func TestValidateIncrementalArtifactOutputContractRejectsNestedResponseOutputMode(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-nested-output-mode-response", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-nested-output-mode-response/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_plan",
			},
		},
	}
	output := map[string]any{
		"artifactPlan": map[string]any{
			"artifacts": []map[string]any{{
				"artifactId":      "artifact-001",
				"targetRef":       "deliverable/main.go",
				"operation":       "replace",
				"coverageTargets": []string{"goal-1"},
				"estimatedChunks": 1,
				"estimatedBytes":  512,
			}},
		},
		"llmDecision": map[string]any{
			"completionState": "continue",
			"outputMode":      outputModeIncrementalArtifact,
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "must not include llmDecision.outputMode") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want nested outputMode rejection", err)
	}
}

func TestValidateIncrementalArtifactContextOutputContractAcceptsBoundedToolGraph(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-context-ok", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-context-ok/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_context",
			},
			"artifactContinuationContext": map[string]any{
				"id":               "cont-001",
				"artifactId":       "artifact-001",
				"targetRef":        "deliverable/main.go",
				"expectedSequence": 2,
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":    "read-current",
				"kind":  "file/read",
				"input": map[string]any{"targetRef": "deliverable/main.go"},
			}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v", err)
	}
}

func TestValidateIncrementalArtifactContextOutputContractRejectsNonAllowlistedGraphKind(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-context-non-allowlisted", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-context-non-allowlisted/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_context",
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{"id": "custom", "kind": "custom/read"}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "forbidden node kind") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want non-allowlisted graph rejection", err)
	}
}

func TestValidateIncrementalArtifactContextOutputContractRejectsOversizedGraph(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-context-oversized", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-context-oversized/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_context",
			},
		},
	}
	nodes := make([]map[string]any, 0, artifactContextMaxGraphNodes+1)
	for idx := 0; idx <= artifactContextMaxGraphNodes; idx++ {
		nodes = append(nodes, map[string]any{"id": "read", "kind": "file/read"})
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{"nodes": nodes},
		"llmDecision":               map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "exceeds maxGraphNodes") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want max graph nodes rejection", err)
	}
}

func TestValidateIncrementalArtifactContextOutputContractRejectsArtifactChunk(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-context-mixed", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-context-mixed/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_context",
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{"id": "read-current", "kind": "file/read"}},
		},
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   1,
			"content":    "package main\n",
			"isFinal":    false,
			"chunkHash":  "sha256:df1d036cbbf3df46e2045071e082245ece204c7f53ecf0a4e022bff9bb228f47",
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "must not include artifactChunk") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want artifactChunk rejection", err)
	}
}

func TestValidateIncrementalArtifactContextOutputContractRejectsPatchApplyGraph(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-context-patch", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-context-patch/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_context",
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{"id": "write", "kind": "patch/apply"}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "forbidden node kind") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want forbidden graph rejection", err)
	}
}

func TestValidateIncrementalArtifactPlanOutputContractRejectsMissingCoverageTargets(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-plan-coverage", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-plan-coverage/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_plan",
			},
		},
	}
	output := map[string]any{
		"artifactPlan": map[string]any{
			"artifacts": []map[string]any{{
				"artifactId":      "artifact-001",
				"targetRef":       "deliverable/main.go",
				"operation":       "replace",
				"estimatedChunks": 1,
				"estimatedBytes":  512,
			}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "coverageTargets is required") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want coverageTargets rejection", err)
	}
}

func TestValidateIncrementalArtifactPlanOutputContractAcceptsCoverageTargetContext(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-plan-coverage-context", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-plan-coverage-context/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_plan",
			},
			"coverageTargetSet": []map[string]any{{
				"coverageTargetToken": "goal-allowed",
				"targetText":          "Goal allowed",
			}},
		},
	}
	output := map[string]any{
		"artifactPlan": map[string]any{
			"artifacts": []map[string]any{{
				"artifactId":      "artifact-001",
				"targetRef":       "deliverable/main.go",
				"operation":       "replace",
				"coverageTargets": []string{"goal-allowed"},
				"estimatedChunks": 1,
				"estimatedBytes":  512,
			}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v", err)
	}
}

func TestValidateIncrementalArtifactPlanOutputContractRejectsDuplicateArtifactID(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-plan-duplicate", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-plan-duplicate/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_plan",
			},
		},
	}
	output := map[string]any{
		"artifactPlan": map[string]any{
			"artifacts": []map[string]any{
				{
					"artifactId":      "artifact-001",
					"targetRef":       "deliverable/main.go",
					"operation":       "replace",
					"coverageTargets": []string{"goal-1"},
					"estimatedChunks": 1,
					"estimatedBytes":  512,
				},
				{
					"artifactId":      "artifact-001",
					"targetRef":       "deliverable/second.go",
					"operation":       "replace",
					"coverageTargets": []string{"goal-1"},
					"estimatedChunks": 1,
					"estimatedBytes":  512,
				},
			},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "artifactId is duplicated") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want duplicate artifactId rejection", err)
	}
}

func TestValidateIncrementalArtifactPlanOutputContractRejectsTargetOutsideBoundary(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-plan-target-boundary", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-plan-target-boundary/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_plan",
			},
		},
	}
	output := map[string]any{
		"artifactPlan": map[string]any{
			"artifacts": []map[string]any{{
				"artifactId":      "artifact-001",
				"targetRef":       "../secret.txt",
				"operation":       "replace",
				"coverageTargets": []string{"goal-1"},
				"estimatedChunks": 1,
				"estimatedBytes":  512,
			}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "outside deliverable boundary") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want target boundary rejection", err)
	}
}

func TestValidateIncrementalArtifactPlanOutputContractRejectsUnknownCoverageTarget(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-plan-coverage-mismatch", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-plan-coverage-mismatch/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_plan",
			},
			"coverageTargetSet": []map[string]any{{
				"coverageTargetToken": "goal-allowed",
				"targetText":          "Goal allowed",
			}},
		},
	}
	output := map[string]any{
		"artifactPlan": map[string]any{
			"artifacts": []map[string]any{{
				"artifactId":      "artifact-001",
				"targetRef":       "deliverable/main.go",
				"operation":       "replace",
				"coverageTargets": []string{"goal-missing"},
				"estimatedChunks": 1,
				"estimatedBytes":  512,
			}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "unknown target") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want coverage mismatch rejection", err)
	}
}

func TestValidateIncrementalArtifactChunkOutputContractAcceptsMatchingChunk(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-ok", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-ok/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase":                 "artifact_chunk",
				"maxArtifactChunkBytes": 64,
			},
			"artifactContinuationContext": map[string]any{
				"id":               "cont-001",
				"state":            string(common.ArtifactContinuationStateChunking),
				"artifactId":       "artifact-001",
				"targetRef":        "deliverable/main.go",
				"expectedSequence": 2,
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   2,
			"content":    "package main\n",
			"isFinal":    false,
			"chunkHash":  "sha256:df1d036cbbf3df46e2045071e082245ece204c7f53ecf0a4e022bff9bb228f47",
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v", err)
	}
}

func TestValidateIncrementalArtifactContextOutputBypassesConcreteEvidenceGate(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-context-gate", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-context-gate/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"concreteFailureEvidenceRequired": true,
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"stateChangingOutputRequired":     true,
			"outputMode":                      outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase":         artifactOutputPhaseContext,
				"maxGraphNodes": 8,
			},
			"artifactContinuationContext": map[string]any{
				"id":               "cont-context-001",
				"state":            string(common.ArtifactContinuationStateChunking),
				"artifactId":       "artifact-001",
				"targetRef":        "deliverable/main.go",
				"expectedSequence": 1,
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": evidenceAcquisitionRequiredOutput(map[string]any{
					"outputMode": outputModeIncrementalArtifact,
				}),
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "read-artifact-context",
				"kind": "file/read",
				"input": map[string]any{
					"targetRef": "deliverable/main.go",
					"optional":  true,
				},
			}},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want artifact context graph accepted", err)
	}
}

func TestValidateIncrementalArtifactChunkOutputContractRejectsSequenceMismatch(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-seq", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-seq/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase":                 "artifact_chunk",
				"maxArtifactChunkBytes": 64,
			},
			"artifactContinuationContext": map[string]any{
				"id":               "cont-001",
				"state":            string(common.ArtifactContinuationStateChunking),
				"artifactId":       "artifact-001",
				"targetRef":        "deliverable/main.go",
				"expectedSequence": 3,
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   2,
			"content":    "package main\n",
			"isFinal":    false,
			"chunkHash":  "sha256:df1d036cbbf3df46e2045071e082245ece204c7f53ecf0a4e022bff9bb228f47",
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "expectedSequence") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want expectedSequence rejection", err)
	}
}

func TestValidateIncrementalArtifactChunkOutputContractRejectsHashMismatch(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-hash", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-hash/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase":                 "artifact_chunk",
				"maxArtifactChunkBytes": 64,
			},
			"artifactContinuationContext": map[string]any{
				"id":               "cont-001",
				"state":            string(common.ArtifactContinuationStateChunking),
				"artifactId":       "artifact-001",
				"targetRef":        "deliverable/main.go",
				"expectedSequence": 2,
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   2,
			"content":    "package main\n",
			"isFinal":    false,
			"chunkHash":  "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "chunkHash does not match content") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want chunk hash rejection", err)
	}
}

func TestValidateIncrementalArtifactChunkOutputContractAcceptsAlreadyReceivedSequenceWithSameHash(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-duplicate", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-duplicate/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase":                 "artifact_chunk",
				"maxArtifactChunkBytes": 64,
			},
			"artifactContinuationContext": map[string]any{
				"id":                "cont-001",
				"state":             string(common.ArtifactContinuationStateChunking),
				"artifactId":        "artifact-001",
				"targetRef":         "deliverable/main.go",
				"expectedSequence":  2,
				"receivedSequences": []int{1, 2},
				"receivedChunkHashes": map[int]string{
					2: "sha256:df1d036cbbf3df46e2045071e082245ece204c7f53ecf0a4e022bff9bb228f47",
				},
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   2,
			"content":    "package main\n",
			"isFinal":    false,
			"chunkHash":  "sha256:df1d036cbbf3df46e2045071e082245ece204c7f53ecf0a4e022bff9bb228f47",
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v", err)
	}
}

func TestValidateIncrementalArtifactChunkOutputContractRejectsAlreadyReceivedSequenceWithDifferentHash(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-duplicate-hash", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-duplicate-hash/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase":                 "artifact_chunk",
				"maxArtifactChunkBytes": 64,
			},
			"artifactContinuationContext": map[string]any{
				"id":                "cont-001",
				"state":             string(common.ArtifactContinuationStateChunking),
				"artifactId":        "artifact-001",
				"targetRef":         "deliverable/main.go",
				"expectedSequence":  2,
				"receivedSequences": []int{1, 2},
				"receivedChunkHashes": map[string]any{
					"2": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
				},
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   2,
			"content":    "package main\n",
			"isFinal":    false,
			"chunkHash":  "sha256:df1d036cbbf3df46e2045071e082245ece204c7f53ecf0a4e022bff9bb228f47",
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "different chunkHash") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want duplicate hash mismatch rejection", err)
	}
}

func TestValidateIncrementalArtifactChunkOutputContractRejectsFullPreviousTailRepeat(t *testing.T) {
	content := "package main\n"
	run := &runmodel.Run{ID: "run-artifact-chunk-tail-repeat", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-tail-repeat/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase":                 "artifact_chunk",
				"maxArtifactChunkBytes": 64,
			},
			"artifactContinuationContext": map[string]any{
				"id":                "cont-001",
				"state":             string(common.ArtifactContinuationStateChunking),
				"artifactId":        "artifact-001",
				"targetRef":         "deliverable/main.go",
				"expectedSequence":  2,
				"previousChunkTail": content,
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   2,
			"content":    content,
			"isFinal":    false,
			"chunkHash":  "sha256:" + sha256HexBytes([]byte(content)),
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "repeats previousChunkTail") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want repeated tail rejection", err)
	}
}

func TestValidateIncrementalArtifactChunkOutputContractWarnsPartialPreviousTailOverlap(t *testing.T) {
	content := "defghi"
	run := &runmodel.Run{ID: "run-artifact-chunk-tail-overlap", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-tail-overlap/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase":                 "artifact_chunk",
				"maxArtifactChunkBytes": 64,
			},
			"artifactContinuationContext": map[string]any{
				"id":                "cont-001",
				"state":             string(common.ArtifactContinuationStateChunking),
				"artifactId":        "artifact-001",
				"targetRef":         "deliverable/main.go",
				"expectedSequence":  2,
				"previousChunkTail": "abcdef",
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   2,
			"content":    content,
			"isFinal":    false,
			"chunkHash":  "sha256:" + sha256HexBytes([]byte(content)),
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v", err)
	}
	if warnings := stringSliceFromValue(output["artifactChunkWarnings"]); !stringSliceContains(warnings, "partial_previous_chunk_tail_overlap") {
		t.Fatalf("artifactChunkWarnings = %+v, want partial overlap warning", warnings)
	}
}

func TestValidateIncrementalArtifactChunkOutputContractWarnsTailMarkerMismatch(t *testing.T) {
	content := "next-content"
	run := &runmodel.Run{ID: "run-artifact-chunk-marker-warning", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-marker-warning/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase":                 "artifact_chunk",
				"maxArtifactChunkBytes": 64,
			},
			"artifactContinuationContext": map[string]any{
				"id":                "cont-001",
				"state":             string(common.ArtifactContinuationStateChunking),
				"artifactId":        "artifact-001",
				"targetRef":         "deliverable/main.go",
				"expectedSequence":  2,
				"previousChunkTail": "previous-tail",
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId":  "artifact-001",
			"targetRef":   "deliverable/main.go",
			"sequence":    2,
			"content":     content,
			"isFinal":     false,
			"chunkHash":   "sha256:" + sha256HexBytes([]byte(content)),
			"startsAfter": "other-tail",
			"endsWith":    "missing-suffix",
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v", err)
	}
	warnings := stringSliceFromValue(output["artifactChunkWarnings"])
	if !stringSliceContains(warnings, "startsAfter_does_not_match_previousChunkTail") ||
		!stringSliceContains(warnings, "endsWith_does_not_match_content_suffix") {
		t.Fatalf("artifactChunkWarnings = %+v, want marker mismatch warnings", warnings)
	}
}

func TestValidateIncrementalArtifactChunkOutputContractRejectsDoDAssessment(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-dod", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-dod/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase":                 "artifact_chunk",
				"maxArtifactChunkBytes": 64,
			},
			"artifactContinuationContext": map[string]any{
				"id":               "cont-001",
				"state":            string(common.ArtifactContinuationStateChunking),
				"artifactId":       "artifact-001",
				"targetRef":        "deliverable/main.go",
				"expectedSequence": 1,
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   1,
			"content":    "package main\n",
			"isFinal":    true,
			"chunkHash":  "sha256:df1d036cbbf3df46e2045071e082245ece204c7f53ecf0a4e022bff9bb228f47",
		},
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "done",
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "must not include dodAssessment") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want dodAssessment rejection", err)
	}
}

func TestValidateIncrementalArtifactChunkOutputContractRejectsChunkRepair(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-repair-mixed", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-repair-mixed/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_chunk",
			},
		},
	}
	output := map[string]any{
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   1,
			"content":    "package main\n",
			"isFinal":    true,
			"chunkHash":  "sha256:df1d036cbbf3df46e2045071e082245ece204c7f53ecf0a4e022bff9bb228f47",
		},
		"artifactChunkRepair": map[string]any{
			"artifactId":    "artifact-001",
			"targetRef":     "deliverable/main.go",
			"sequence":      1,
			"failureReason": "mixed response",
			"nextAction":    "retry_current_sequence",
			"constraints":   []string{"retry current sequence"},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "must not include artifactChunkRepair") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want artifactChunkRepair rejection", err)
	}
}

func TestValidateIncrementalArtifactChunkRepairOutputContractAcceptsRetryAction(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-repair-ok", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-repair-ok/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_chunk_repair",
			},
			"artifactContinuationContext": map[string]any{
				"id":               "cont-001",
				"state":            string(common.ArtifactContinuationStateChunking),
				"artifactId":       "artifact-001",
				"targetRef":        "deliverable/main.go",
				"expectedSequence": 2,
			},
		},
	}
	output := map[string]any{
		"artifactChunkRepair": map[string]any{
			"artifactId":    "artifact-001",
			"targetRef":     "deliverable/main.go",
			"sequence":      2,
			"failureReason": "chunk hash mismatch",
			"nextAction":    "retry_current_sequence",
			"constraints":   []string{"preserve targetRef", "return only sequence 2"},
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v", err)
	}
}

func TestValidateIncrementalArtifactChunkRepairOutputContractAcceptsBlockedAction(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-repair-blocked", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-repair-blocked/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_chunk_repair",
			},
			"artifactContinuationContext": map[string]any{
				"id":               "cont-001",
				"state":            string(common.ArtifactContinuationStateChunking),
				"artifactId":       "artifact-001",
				"targetRef":        "deliverable/main.go",
				"expectedSequence": 2,
			},
		},
	}
	output := map[string]any{
		"artifactChunkRepair": map[string]any{
			"artifactId":    "artifact-001",
			"targetRef":     "deliverable/main.go",
			"sequence":      2,
			"failureReason": "missing required source evidence",
			"nextAction":    "blocker",
			"constraints":   []string{"request missing evidence"},
		},
		"completionState": "blocked",
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v", err)
	}
}

func TestValidateIncrementalArtifactChunkRepairOutputContractRejectsChunkPayload(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-repair-payload", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-repair-payload/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_chunk_repair",
			},
		},
	}
	output := map[string]any{
		"artifactChunkRepair": map[string]any{
			"artifactId":    "artifact-001",
			"targetRef":     "deliverable/main.go",
			"sequence":      2,
			"failureReason": "retry with full body",
			"nextAction":    "retry_current_sequence",
			"constraints":   []string{"retry current sequence"},
			"content":       "large body must be returned as artifactChunk in a later response",
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "must not include payload field content") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want payload rejection", err)
	}
}

func TestValidateIncrementalArtifactChunkRepairOutputContractRejectsArtifactChunk(t *testing.T) {
	run := &runmodel.Run{ID: "run-artifact-chunk-repair-chunk", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-artifact-chunk-repair-chunk/llm-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeIncrementalArtifact,
			"artifactOutputContract": map[string]any{
				"phase": "artifact_chunk_repair",
			},
		},
	}
	output := map[string]any{
		"artifactChunkRepair": map[string]any{
			"artifactId":    "artifact-001",
			"targetRef":     "deliverable/main.go",
			"sequence":      2,
			"failureReason": "retry with chunk",
			"nextAction":    "retry_current_sequence",
			"constraints":   []string{"retry current sequence"},
		},
		"artifactChunk": map[string]any{
			"artifactId": "artifact-001",
			"targetRef":  "deliverable/main.go",
			"sequence":   2,
			"content":    "package main\n",
			"isFinal":    true,
			"chunkHash":  "sha256:df1d036cbbf3df46e2045071e082245ece204c7f53ecf0a4e022bff9bb228f47",
		},
		"llmDecision": map[string]any{"completionState": "continue"},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if err == nil || !strings.Contains(err.Error(), "must not include artifactChunk") {
		t.Fatalf("validateLLMExecutionOutputContract() error = %v, want artifactChunk rejection", err)
	}
}
