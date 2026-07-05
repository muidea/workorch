package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeStepEvidenceQualityUsesRuntimeOutputMetadata(t *testing.T) {
	run := &runmodel.Run{ID: "run-output-quality", WorkspaceID: "ws-output-quality"}
	step := &runmodel.Step{ID: "run-output-quality/block/verify", RunID: run.ID, Kind: shellcommon.StepKindCommand}
	output := map[string]any{
		"evidenceQuality": string(common.EvidenceQualityInconclusive),
		"evidenceReason":  "runtime adapter reported inconclusive evidence",
		"zeroWork":        true,
		"evidenceTags":    []any{"validation", "zero_work"},
	}
	quality := runtimeStepEvidenceQuality(run, step, common.ValidationResultStatePassed, output)
	if quality.Quality != common.EvidenceQualityInconclusive || !quality.ZeroWork || !strings.Contains(strings.Join(quality.Tags, ","), "zero_work") {
		t.Fatalf("quality = %+v, want inconclusive zero-work metadata", quality)
	}
}

func TestRuntimeStepEvidenceQualityRejectsFailedDeclaredMetric(t *testing.T) {
	run := &runmodel.Run{ID: "run-required-metric", WorkspaceID: "ws-required-metric"}
	step := &runmodel.Step{
		ID:    "run-required-metric/block/verify",
		RunID: run.ID,
		Kind:  shellcommon.StepKindCommand,
		Input: map[string]any{
			"validationEvidence": map[string]any{
				"requiredMetrics": []map[string]any{{
					"name":   "required_count",
					"source": "stdout_key_value",
					"op":     "nonzero",
				}},
			},
		},
		Output: map[string]any{
			"stdout": "required_count=0\n",
		},
	}
	quality := runtimeStepEvidenceQuality(run, step, common.ValidationResultStatePassed, step.Output)
	if quality.Quality != common.EvidenceQualityInconclusive ||
		!strings.Contains(quality.Reason, "required_count") ||
		!stringSliceContains(quality.Tags, "validation_criteria_failed") {
		t.Fatalf("quality = %+v, want inconclusive failed metric verdict", quality)
	}
}

func TestRuntimeStepEvidenceQualityAcceptsSatisfiedDeclaredMetric(t *testing.T) {
	run := &runmodel.Run{ID: "run-required-metric-pass", WorkspaceID: "ws-required-metric"}
	step := &runmodel.Step{
		ID:    "run-required-metric-pass/block/verify",
		RunID: run.ID,
		Kind:  shellcommon.StepKindCommand,
		Input: map[string]any{
			"validationEvidence": map[string]any{
				"requiredMetrics": []map[string]any{{
					"name":   "required_count",
					"source": "stdout_key_value",
					"op":     "gte",
					"value":  2,
				}},
			},
		},
		Output: map[string]any{
			"stdout": "required_count=2\n",
		},
	}
	quality := runtimeStepEvidenceQuality(run, step, common.ValidationResultStatePassed, step.Output)
	if quality.Quality != common.EvidenceQualityValid {
		t.Fatalf("quality = %+v, want valid satisfied metric verdict", quality)
	}
}

func TestRuntimeStepEvidenceQualityRejectsContradictedObservedClaims(t *testing.T) {
	run := &runmodel.Run{ID: "run-contradicted-claims", WorkspaceID: "ws-contradicted-claims"}
	step := &runmodel.Step{
		ID:    "run-contradicted-claims/block/verify",
		RunID: run.ID,
		Kind:  shellcommon.StepKindCommand,
		Input: map[string]any{
			"validationEvidence": map[string]any{
				"observedClaims": []string{"working tree is clean and has no uncommitted changes"},
			},
		},
		Output: map[string]any{
			"stdout": strings.Join([]string{
				"status summary:",
				"modified: config.example.yaml",
				"no changes added to commit",
			}, "\n"),
		},
	}

	quality := runtimeStepEvidenceQuality(run, step, common.ValidationResultStatePassed, step.Output)
	if quality.Quality != common.EvidenceQualityInvalid ||
		!strings.Contains(quality.Reason, "contradicted") ||
		!stringSliceContains(quality.Tags, "validation_observed_claims_contradicted") {
		t.Fatalf("quality = %+v, want invalid contradicted observedClaims", quality)
	}
}

func TestRuntimeStepEvidenceQualityDemotesControlledSubstituteDeliverablePatch(t *testing.T) {
	run := &runmodel.Run{ID: "run-controlled-substitute-deliverable", WorkspaceID: "ws-controlled-substitute"}
	step := &runmodel.Step{
		ID:    "run-controlled-substitute-deliverable/block/verify",
		RunID: run.ID,
		Kind:  shellcommon.StepKindCommand,
		Input: map[string]any{
			"command": "sh",
			"args": []string{
				"-c",
				`cp target.go "$SCRATCH/"; sed -i 's/bad/good/' "$SCRATCH/target.go"; go build ./...`,
			},
			"validationEvidence": map[string]any{
				"mode":            "controlled_substitute",
				"observationType": "controlled_substitute_observation",
				"substituteFor":   "workspace deliverable target.go",
				"observedClaims": []string{
					"project compiles after fixing the copied deliverable",
				},
			},
		},
		Output: map[string]any{
			"exitCode": 0,
			"stdout":   "BUILD_EXIT_CODE=0\n",
		},
	}

	quality := runtimeStepEvidenceQuality(run, step, common.ValidationResultStatePassed, step.Output)
	if quality.Quality != common.EvidenceQualityInconclusive ||
		!strings.Contains(quality.Reason, "controlled substitute") ||
		!stringSliceContains(quality.Tags, "controlled_substitute_deliverable_mutation") {
		t.Fatalf("quality = %+v, want inconclusive controlled substitute deliverable mutation", quality)
	}
}

func TestValidationEvidenceRecordRejectsPassedInconclusiveResult(t *testing.T) {
	run := &runmodel.Run{ID: "run-inconclusive-record", WorkspaceID: "ws-inconclusive-record"}
	validation := &runmodel.ValidationResult{
		ID:              "run-inconclusive-record/block/verify/validation/runtime-step",
		WorkspaceID:     run.WorkspaceID,
		RunID:           run.ID,
		StepID:          "run-inconclusive-record/block/verify",
		ValidationKind:  "runtime_step",
		TargetRef:       "cwi-001",
		State:           common.ValidationResultStatePassed,
		EvidenceQuality: common.EvidenceQualityInconclusive,
		EvidenceReason:  "controlled substitute did not validate current workspace state",
	}

	record := evidenceRecordForValidationResult(run, validation)
	if record == nil {
		t.Fatal("evidenceRecordForValidationResult returned nil")
	}
	if record.Status != "rejected" {
		t.Fatalf("record status = %q, want rejected", record.Status)
	}
}

func TestRuntimeStepEvidenceQualityAppliesEvidencePolicyCriteria(t *testing.T) {
	run := &runmodel.Run{ID: "run-policy-required-metric", WorkspaceID: "ws-policy-required-metric"}
	step := &runmodel.Step{
		ID:    "run-policy-required-metric/block/verify",
		RunID: run.ID,
		Kind:  shellcommon.StepKindCommand,
		Input: map[string]any{
			"evidencePolicies": []map[string]any{{
				"id":        "policy-required-metric",
				"appliesTo": shellcommon.StepKindCommand,
				"requiredMetrics": []map[string]any{{
					"name":   "policy_observed",
					"source": "stdout_key_value",
					"op":     "true",
				}},
			}},
		},
		Output: map[string]any{
			"stdout": "policy_observed=false\n",
		},
	}
	quality := runtimeStepEvidenceQuality(run, step, common.ValidationResultStatePassed, step.Output)
	if quality.Quality != common.EvidenceQualityInconclusive ||
		!strings.Contains(quality.Reason, "policy_observed") {
		t.Fatalf("quality = %+v, want policy criteria failure", quality)
	}
}

func TestDoDAssessmentEvidenceQualityRejectsInconclusiveStepEvidence(t *testing.T) {
	ctx := context.Background()
	validationRepo := memory.NewValidationResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{ValidationResults: validationRepo}}
	runID := common.RunID("run-dod-evidence-quality")
	stepID := common.StepID("run-dod-evidence-quality/block/verify")
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(stepID) + "/validation/runtime-step"),
		WorkspaceID:     "ws-dod-evidence-quality",
		RunID:           runID,
		StepID:          stepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStatePassed,
		Summary:         "runtime step completed",
		EvidenceQuality: common.EvidenceQualityInconclusive,
		EvidenceReason:  "runtime adapter reported inconclusive evidence",
		ZeroWork:        true,
		EvidenceTags:    []string{"validation", "zero_work"},
	}); err != nil {
		t.Fatalf("seed validation result: %v", err)
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, &runmodel.Run{ID: runID}, map[string]any{
		"items": []map[string]any{
			{
				"item":         "429 passthrough is covered",
				"state":        "satisfied",
				"evidenceRefs": []string{string(stepID)},
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "inconclusive") || !strings.Contains(err.Error(), string(stepID)) {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want inconclusive evidence rejection", err)
	}
	var diagnostic runtimeFailureDiagnosticError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %T, want runtime diagnostics", err)
	}
	diagnostics := diagnostic.RuntimeFailureDiagnostics()
	if stringFromMap(diagnostics, "kind") != "dod_evidence_quality_gap" {
		t.Fatalf("diagnostics = %+v, want dod_evidence_quality_gap", diagnostics)
	}
	missing := mapSliceFromValue(diagnostics["missingDoDItems"])
	if len(missing) != 1 || missing[0]["source"] != "evidenceQuality" || missing[0]["state"] != "failed" {
		t.Fatalf("missingDoDItems = %+v, want failed evidence quality gap", missing)
	}
}

func TestDoDAssessmentEvidenceKindRequirementRejectsReadOnlySideEffectEvidence(t *testing.T) {
	ctx := context.Background()
	validationRepo := memory.NewValidationResultRepository()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: validationRepo,
		},
	}
	runID := common.RunID("run-dod-evidence-kind")
	readStepID := common.StepID("run-dod-evidence-kind/block/read-usage")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    readStepID,
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed read step: %v", err)
	}
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(readStepID) + "/validation/runtime-step"),
		WorkspaceID:     "ws-dod-evidence-kind",
		RunID:           runID,
		StepID:          readStepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStatePassed,
		EvidenceQuality: common.EvidenceQualityValid,
	}); err != nil {
		t.Fatalf("seed validation result: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{
							{
								"id":                "persistence_or_side_effect",
								"scope":             "acceptanceDimension",
								"acceptedStepKinds": []string{shellcommon.StepKindCommand, "patch/apply", "mcp/tool_call"},
								"rejectedStepKinds": []string{"file/read", "file/list"},
								"description":       "Read-only evidence cannot prove side effects.",
							},
						},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"items": []map[string]any{{
			"item":         "side effect recorded",
			"state":        "satisfied",
			"evidenceRefs": []string{string(readStepID)},
		}},
		"acceptanceDimensions": []map[string]any{{
			"id":           "persistence_or_side_effect",
			"state":        "satisfied",
			"evidenceRefs": []string{string(readStepID)},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "persistence_or_side_effect") ||
		!strings.Contains(err.Error(), "file/read") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want read-only side-effect evidence rejection", err)
	}
	var diagnostic runtimeFailureDiagnosticError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %T, want runtime diagnostics", err)
	}
	missing := mapSliceFromValue(diagnostic.RuntimeFailureDiagnostics()["missingDoDItems"])
	if len(missing) != 1 || missing[0]["source"] != "evidenceKind" {
		t.Fatalf("missingDoDItems = %+v, want evidenceKind diagnostic", missing)
	}
}

func TestDoDAssessmentEvidenceKindRequirementDiagnosticsRequireStateChangingEvidence(t *testing.T) {
	ctx := context.Background()
	validationRepo := memory.NewValidationResultRepository()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: validationRepo,
		},
	}
	runID := common.RunID("run-dod-write-evidence-kind")
	readStepID := common.StepID("run-dod-write-evidence-kind/block/read-snapshot")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    readStepID,
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed read step: %v", err)
	}
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(readStepID) + "/validation/runtime-step"),
		WorkspaceID:     "ws-dod-write-evidence-kind",
		RunID:           runID,
		StepID:          readStepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStatePassed,
		EvidenceQuality: common.EvidenceQualityValid,
	}); err != nil {
		t.Fatalf("seed validation result: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                    "write_snapshot",
							"scope":                 "deliverable",
							"requireDirectEvidence": true,
							"acceptedStepKinds":     []string{gitcommon.StepKindWrite},
							"acceptedOperations":    []string{gitcommon.OperationCommit},
							"rejectedStepKinds":     []string{"file/read"},
						}},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"deliverableCoverage": map[string]any{
			"items": []map[string]any{{
				"id":           "write_snapshot",
				"state":        "satisfied",
				"evidenceRefs": []string{string(readStepID)},
			}},
		},
	})
	var diagnostic runtimeFailureDiagnosticError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %T, want runtime diagnostics", err)
	}
	diagnostics := diagnostic.RuntimeFailureDiagnostics()
	if !boolValue(diagnostics["stateChangingEvidenceNeeded"]) {
		t.Fatalf("diagnostics = %+v, want stateChangingEvidenceNeeded", diagnostics)
	}
	missing := mapSliceFromValue(diagnostics["missingDeliverableCoverage"])
	if len(missing) != 1 || !stringSliceContains(stringSliceFromValue(missing[0]["requiredEvidenceKinds"]), "state_change") {
		t.Fatalf("missingDeliverableCoverage = %+v, want state_change requiredEvidenceKinds", missing)
	}
	if !stringSliceContains(stringSliceFromValue(missing[0]["acceptedStepKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(missing[0]["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("missingDeliverableCoverage = %+v, want accepted write requirement fields", missing)
	}
}

func TestDoDAssessmentEvidenceKindRequirementAcceptsActionEvidence(t *testing.T) {
	ctx := context.Background()
	validationRepo := memory.NewValidationResultRepository()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: validationRepo,
		},
	}
	runID := common.RunID("run-dod-evidence-kind-valid")
	testStepID := common.StepID("run-dod-evidence-kind-valid/block/test")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    testStepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed shell step: %v", err)
	}
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(testStepID) + "/validation/runtime-step"),
		WorkspaceID:     "ws-dod-evidence-kind-valid",
		RunID:           runID,
		StepID:          testStepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStatePassed,
		EvidenceQuality: common.EvidenceQualityValid,
	}); err != nil {
		t.Fatalf("seed validation result: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{
							{
								"id":                "persistence_or_side_effect",
								"scope":             "acceptanceDimension",
								"acceptedStepKinds": []string{shellcommon.StepKindCommand, "patch/apply", "mcp/tool_call"},
								"rejectedStepKinds": []string{"file/read", "file/list"},
							},
						},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"items": []map[string]any{{
			"item":         "side effect recorded",
			"state":        "satisfied",
			"evidenceRefs": []string{string(testStepID)},
		}},
		"acceptanceDimensions": []map[string]any{{
			"id":           "persistence_or_side_effect",
			"state":        "satisfied",
			"evidenceRefs": []string{string(testStepID)},
		}},
	})
	if err != nil {
		t.Fatalf("validateDoDAssessmentEvidenceQuality: %v", err)
	}
}

func TestDoDAssessmentEvidenceKindRequirementRejectsPatchAndBuildForBehaviorClaim(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-behavior-evidence")
	claim := "streaming proxy forwarding works"
	patchStepID := common.StepID("run-dod-behavior-evidence/graph-001/001-apply")
	buildStepID := common.StepID("run-dod-behavior-evidence/graph-001/002-build")
	for _, step := range []*runmodel.Step{
		{
			ID:    patchStepID,
			RunID: runID,
			Kind:  "patch/apply",
			State: common.StepStateCompleted,
			Input: map[string]any{"coverageTargets": []string{claim}},
		},
		{
			ID:    buildStepID,
			RunID: runID,
			Kind:  shellcommon.StepKindCommand,
			State: common.StepStateCompleted,
			Input: map[string]any{
				"command":         "go build ./...",
				"coverageTargets": []string{claim},
				"validationEvidence": map[string]any{
					"mode":            "static_inspection",
					"observationType": "static_analysis",
				},
			},
			Output: map[string]any{"stdout": "build passed"},
		},
	} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                                "*",
							"scope":                             "dodItem",
							"requireDirectEvidence":             true,
							"requireClaimEvidence":              true,
							"requireExecutableBehaviorEvidence": true,
							"acceptedStepKinds":                 []string{shellcommon.StepKindCommand, "patch/apply", "mcp/tool_call"},
							"rejectedStepKinds":                 []string{"file/read", "file/list"},
						}},
					},
				},
			},
		},
	}

	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"items": []map[string]any{{
			"item":         claim,
			"state":        "satisfied",
			"evidenceRefs": []string{string(patchStepID), string(buildStepID)},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "runtime behavior") ||
		!strings.Contains(err.Error(), "implementation change") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want behavior evidence rejection", err)
	}
}

func TestDoDAssessmentEvidenceKindRequirementAcceptsTargetedTestForBehaviorClaim(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-behavior-test-evidence")
	claim := "streaming proxy forwarding works"
	testStepID := common.StepID("run-dod-behavior-test-evidence/graph-001/003-test")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    testStepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"command":         "go test ./... -run TestStreamingProxyForwarding",
			"coverageTargets": []string{claim},
		},
		Output: map[string]any{"stdout": "=== RUN   TestStreamingProxyForwarding\n--- PASS: TestStreamingProxyForwarding\nPASS"},
	}); err != nil {
		t.Fatalf("seed test step: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                                "*",
							"scope":                             "dodItem",
							"requireDirectEvidence":             true,
							"requireClaimEvidence":              true,
							"requireExecutableBehaviorEvidence": true,
							"acceptedStepKinds":                 []string{shellcommon.StepKindCommand, "patch/apply", "mcp/tool_call"},
							"rejectedStepKinds":                 []string{"file/read", "file/list"},
						}},
					},
				},
			},
		},
	}

	if err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"items": []map[string]any{{
			"item":         claim,
			"state":        "satisfied",
			"evidenceRefs": []string{string(testStepID)},
		}},
	}); err != nil {
		t.Fatalf("validateDoDAssessmentEvidenceQuality: %v", err)
	}
}

func TestDoDAssessmentEvidenceQualityRejectsArtifactChunkEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-artifact-chunk-evidence")
	chunkStepID := common.StepID(runID + "/artifact-continue-001")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    chunkStepID,
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"artifactContinuationContext": map[string]any{
				"state": string(common.ArtifactContinuationStateChunking),
			},
		},
		Output: map[string]any{
			"stage": "llm_artifact_chunk",
		},
	}); err != nil {
		t.Fatalf("seed chunk step: %v", err)
	}

	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, &runmodel.Run{ID: runID}, map[string]any{
		"items": []map[string]any{{
			"item":         "assembled artifact is delivered",
			"state":        "satisfied",
			"evidenceRefs": []string{string(chunkStepID)},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "artifact chunk evidence") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want artifact chunk evidence rejection", err)
	}
}

func TestDoDAssessmentEvidenceQualityAllowsArtifactApplyEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	patchRepo := memory.NewPatchRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			Patches: patchRepo,
		},
	}
	runID := common.RunID("run-dod-artifact-apply-evidence")
	stepID := common.StepID(runID + "/artifact-apply-001")
	patchID := common.PatchID(string(stepID) + "/patch/1")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  artifactApplyStepKind,
		State: common.StepStateCompleted,
		Output: map[string]any{
			"artifactContinuationApply": map[string]any{
				"targetRef": "deliverable/target.txt",
				"state":     string(common.ArtifactContinuationStateApplied),
			},
		},
	}); err != nil {
		t.Fatalf("seed apply step: %v", err)
	}
	if err := patchRepo.Create(ctx, &runmodel.Patch{
		ID:        patchID,
		RunID:     runID,
		StepID:    stepID,
		PatchType: "file_write",
		TargetRef: "deliverable/target.txt",
		State:     common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("seed patch: %v", err)
	}

	if err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, &runmodel.Run{ID: runID}, map[string]any{
		"items": []map[string]any{{
			"item":         "assembled artifact is delivered",
			"state":        "satisfied",
			"evidenceRefs": []string{string(patchID)},
		}},
	}); err != nil {
		t.Fatalf("validateDoDAssessmentEvidenceQuality: %v", err)
	}
}

func TestDoDAssessmentEvidenceKindRequirementAcceptsGitWriteDeliverableEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-git-deliverable")
	gitStepID := common.StepID("run-dod-git-deliverable/graph-001/003-git-commit")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    gitStepID,
		RunID: runID,
		Kind:  gitcommon.StepKindWrite,
		State: common.StepStateCompleted,
		Input: map[string]any{"operation": gitcommon.OperationCommit},
		Output: map[string]any{
			"operation": gitcommon.OperationCommit,
			"commit": map[string]any{
				"hash":    "1111111111111111111111111111111111111111",
				"subject": "implement goal",
			},
		},
	}); err != nil {
		t.Fatalf("seed git step: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                    "version_control_snapshot",
							"scope":                 "deliverable",
							"requireDirectEvidence": true,
							"acceptedStepKinds":     []string{gitcommon.StepKindWrite},
							"rejectedStepKinds":     []string{"file/read", shellcommon.StepKindCommand},
						}},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"deliverableCoverage": map[string]any{
			"items": []map[string]any{{
				"id":           "version_control_snapshot",
				"state":        "satisfied",
				"evidenceRefs": []string{string(gitStepID)},
			}},
		},
	})
	if err != nil {
		t.Fatalf("validateDoDAssessmentEvidenceQuality: %v", err)
	}
}

func TestStateChangingRepairContractAcceptsProfileRequiredWriteEvidenceGraph(t *testing.T) {
	run := &runmodel.Run{ID: "run-profile-write-evidence", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-profile-write-evidence/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{gitcommon.StepKindWrite},
					"acceptedOperations":           []string{gitcommon.OperationCommit},
				},
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{{
				"id":   "commit-snapshot",
				"kind": gitcommon.StepKindWrite,
				"input": map[string]any{
					"operation": gitcommon.OperationCommit,
					"message":   "record workspace snapshot",
				},
			}},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want accepted profile-required write evidence graph", err)
	}
}

func TestStateChangingRepairContractAcceptsProfileRequiredWriteEvidenceGraphFollowUp(t *testing.T) {
	run := &runmodel.Run{ID: "run-profile-write-evidence-follow-up", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-profile-write-evidence-follow-up/follow-up-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			"graphFollowUp":                   true,
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{gitcommon.StepKindWrite, shellcommon.StepKindCommand},
					"acceptedOperations":           []string{gitcommon.OperationCommit},
				},
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "stage-snapshot",
					"kind": gitcommon.StepKindWrite,
					"input": map[string]any{
						"operation": gitcommon.OperationAdd,
						"pathspecs": []string{"."},
					},
				},
				{
					"id":        "commit-snapshot",
					"kind":      gitcommon.StepKindWrite,
					"dependsOn": []string{"stage-snapshot"},
					"input": map[string]any{
						"operation": gitcommon.OperationCommit,
						"message":   "record workspace snapshot",
					},
				},
				{
					"id":        "verify-snapshot",
					"kind":      shellcommon.StepKindCommand,
					"dependsOn": []string{"commit-snapshot"},
					"input": map[string]any{
						"command":           "verify",
						"verificationScope": "snapshot evidence",
						"targetRef":         "workspace",
					},
				},
			},
		},
	}

	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want graph follow-up accepted for profile-required write evidence", err)
	}
}

func TestStateChangingRepairContractRejectsSupplementaryWriteOnlyGraphWhenPrimaryStateChangeRequired(t *testing.T) {
	run := &runmodel.Run{ID: "run-profile-write-evidence-mixed", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-profile-write-evidence-mixed/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode":        true,
					"mustIncludePrimaryStateChangingNode": true,
					"acceptedKinds":                       []string{"patch/apply", gitcommon.StepKindWrite},
					"primaryAcceptedKinds":                []string{"patch/apply", "shell/command", "mcp/tool_call"},
				},
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "init",
					"kind": gitcommon.StepKindWrite,
					"input": map[string]any{
						"operation": gitcommon.OperationInit,
					},
				},
				{
					"id":        "add",
					"kind":      gitcommon.StepKindWrite,
					"dependsOn": []string{"init"},
					"input": map[string]any{
						"operation": gitcommon.OperationAdd,
						"pathspecs": []string{"."},
					},
				},
				{
					"id":        "commit",
					"kind":      gitcommon.StepKindWrite,
					"dependsOn": []string{"add"},
					"input": map[string]any{
						"operation": gitcommon.OperationCommit,
						"message":   "record workspace snapshot",
					},
				},
			},
		},
	}

	err := validateLLMExecutionOutputContract(run, step, output)
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "without patch/apply or declared auditable state-changing operation") {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want primary state-changing contract rejection", err)
	}
}

func TestStateChangingRepairContractAcceptsSupportingWriteEvidenceGraphAfterValidationContract(t *testing.T) {
	run := &runmodel.Run{ID: "run-profile-write-evidence-supporting", InitialStepKind: llmWorkspaceExecutionStepKind}
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageVerification,
		"repairObjective":             repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired":   true,
		"stateChangingOutputRequired": true,
	}
	applied := ApplyConvergenceContract(input, ConvergenceContract{
		State:                       convergenceStateNeedStateChange,
		RuntimeStage:                runtimeStageRepair,
		RuntimeStageMode:            "strict_convergence",
		RepairObjective:             repairObjectiveStateChangingOutput,
		StateChangingOutputRequired: true,
		RepairActionPackage: map[string]any{
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []string{gitcommon.StepKindWrite},
				"acceptedOperations":           []string{gitcommon.OperationCommit},
			},
		},
	})
	step := &runmodel.Step{
		ID:    "run-profile-write-evidence-supporting/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: applied,
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "init",
					"kind": gitcommon.StepKindWrite,
					"input": map[string]any{
						"operation": gitcommon.OperationInit,
					},
				},
				{
					"id":        "add",
					"kind":      gitcommon.StepKindWrite,
					"dependsOn": []string{"init"},
					"input": map[string]any{
						"operation": gitcommon.OperationAdd,
						"pathspecs": []string{"."},
					},
				},
				{
					"id":        "commit",
					"kind":      gitcommon.StepKindWrite,
					"dependsOn": []string{"add"},
					"input": map[string]any{
						"operation": gitcommon.OperationCommit,
						"message":   "record workspace snapshot",
					},
				},
			},
		},
	}

	if inputRequiresConcreteValidationEvidence(step.Input) {
		t.Fatalf("applied input still requires concrete validation evidence: %+v", step.Input)
	}
	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want supporting write graph accepted", err)
	}
}

func TestStateChangingRepairContractOverridesStaleConcreteValidationEvidenceFlags(t *testing.T) {
	run := &runmodel.Run{ID: "run-profile-write-evidence-stale-validation", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    "run-profile-write-evidence-stale-validation/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired":   true,
			"evidenceRepairRequired":      true,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode":          true,
					"mustProduceConcreteValidationEvidence": true,
					"acceptedKinds":                         []string{gitcommon.StepKindWrite},
					"acceptedOperations":                    []string{gitcommon.OperationCommit},
				},
			},
		},
	}
	output := map[string]any{
		stepOutputKeyExecutionGraph: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "add",
					"kind": gitcommon.StepKindWrite,
					"input": map[string]any{
						"operation": gitcommon.OperationAdd,
						"pathspecs": []string{"."},
					},
				},
				{
					"id":        "commit",
					"kind":      gitcommon.StepKindWrite,
					"dependsOn": []string{"add"},
					"input": map[string]any{
						"operation": gitcommon.OperationCommit,
						"message":   "record workspace snapshot",
					},
				},
				{
					"id":        "verify",
					"kind":      shellcommon.StepKindCommand,
					"dependsOn": []string{"commit"},
					"input": map[string]any{
						"command": "true",
					},
				},
			},
		},
	}

	if inputRequiresConcreteValidationEvidence(step.Input) {
		t.Fatalf("stale validation flags still override accepted state-changing evidence contract: %+v", step.Input)
	}
	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		t.Fatalf("validateLLMExecutionOutputContract err = %v, want accepted state-changing evidence graph despite stale validation flags", err)
	}
}

func TestDoDAssessmentEvidenceKindRequirementRejectsWrongGitWriteOperation(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-git-deliverable-operation")
	addStepID := common.StepID("run-dod-git-deliverable-operation/graph-001/002-git-add")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     addStepID,
		RunID:  runID,
		Kind:   gitcommon.StepKindWrite,
		State:  common.StepStateCompleted,
		Input:  map[string]any{"operation": gitcommon.OperationAdd},
		Output: map[string]any{"operation": gitcommon.OperationAdd},
	}); err != nil {
		t.Fatalf("seed git step: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                 "version_control_snapshot",
							"scope":              "deliverable",
							"acceptedStepKinds":  []string{gitcommon.StepKindWrite},
							"acceptedOperations": []string{gitcommon.OperationCommit},
						}},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"deliverableCoverage": []map[string]any{{
			"id":           "version_control_snapshot",
			"state":        "satisfied",
			"evidenceRefs": []string{string(addStepID)},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "requires evidence operation: commit") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want commit operation requirement", err)
	}
}

func TestDoDAssessmentEvidenceKindRequirementRejectsZeroWorkAcceptedEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-git-deliverable-zero-work")
	commitStepID := common.StepID("run-dod-git-deliverable-zero-work/graph-001/002-git-commit")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    commitStepID,
		RunID: runID,
		Kind:  gitcommon.StepKindWrite,
		State: common.StepStateCompleted,
		Input: map[string]any{"operation": gitcommon.OperationCommit},
		Output: map[string]any{
			"operation": gitcommon.OperationCommit,
			"zeroWork":  true,
			"summary":   "git commit completed with no staged changes",
		},
	}); err != nil {
		t.Fatalf("seed git step: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                 "version_control_snapshot",
							"scope":              "deliverable",
							"acceptedStepKinds":  []string{gitcommon.StepKindWrite},
							"acceptedOperations": []string{gitcommon.OperationCommit},
						}},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"deliverableCoverage": []map[string]any{{
			"id":           "version_control_snapshot",
			"state":        "satisfied",
			"evidenceRefs": []string{string(commitStepID)},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "non-zero-work valid evidence") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want zero-work accepted evidence rejected", err)
	}
}

func TestDoDAssessmentEvidenceKindRequirementRejectsNegativeScalarShellEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: memory.NewValidationResultRepository(),
		},
	}
	runID := common.RunID("run-dod-evidence-negative-scalar")
	stepID := common.StepID("run-dod-evidence-negative-scalar/validate-decisions")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"command": "sh",
			"args":    []any{"-c", "jq '.decisions | length' snapshot.json"},
		},
		Output: map[string]any{
			"exitCode": 0,
			"stdout":   "0\n",
		},
	}); err != nil {
		t.Fatalf("seed shell step: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                "decision_validation",
							"scope":             "dod_item",
							"acceptedStepKinds": []string{shellcommon.StepKindCommand},
						}},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"items": []map[string]any{{
			"id":           "decision_validation",
			"item":         "decision validation completed",
			"state":        "satisfied",
			"evidenceRefs": []string{string(stepID)},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "non-zero-work valid evidence") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want negative scalar evidence rejected", err)
	}
}

func TestCoverageWorkItemStepUpdateRejectsNegativeScalarShellValidationEvidence(t *testing.T) {
	update := coverageWorkItemStepUpdate(&runmodel.Step{
		ID:    "run-coverage-negative-scalar/validate",
		RunID: "run-coverage-negative-scalar",
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"coverageWorkItemIds": []string{"cwi-001"},
		},
		Output: map[string]any{
			"exitCode": 0,
			"stdout":   "0",
		},
	})
	if stringSliceContains(update.EvidenceKinds, "validation") {
		t.Fatalf("update = %+v, want no validation evidence kind for negative scalar shell output", update)
	}
}

func TestDoDAssessmentEvidenceKindRequirementAllowsReadEvidenceWithActionEvidence(t *testing.T) {
	ctx := context.Background()
	validationRepo := memory.NewValidationResultRepository()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: validationRepo,
		},
	}
	runID := common.RunID("run-dod-evidence-kind-mixed")
	readStepID := common.StepID("run-dod-evidence-kind-mixed/block/read")
	actionStepID := common.StepID("run-dod-evidence-kind-mixed/block/action")
	for _, step := range []*runmodel.Step{
		{ID: readStepID, RunID: runID, Kind: "file/read", State: common.StepStateCompleted},
		{ID: actionStepID, RunID: runID, Kind: shellcommon.StepKindCommand, State: common.StepStateCompleted},
	} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
		if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
			ID:              common.ValidationResultID(string(step.ID) + "/validation/runtime-step"),
			WorkspaceID:     "ws-dod-evidence-kind-mixed",
			RunID:           runID,
			StepID:          step.ID,
			ValidationKind:  "runtime_step",
			State:           common.ValidationResultStatePassed,
			EvidenceQuality: common.EvidenceQualityValid,
		}); err != nil {
			t.Fatalf("seed validation result for %s: %v", step.ID, err)
		}
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{
							{
								"id":                "persistence_or_side_effect",
								"scope":             "acceptanceDimension",
								"acceptedStepKinds": []string{shellcommon.StepKindCommand, "patch/apply", "mcp/tool_call"},
								"rejectedStepKinds": []string{"file/read", "file/list"},
							},
						},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"acceptanceDimensions": []map[string]any{{
			"id":           "persistence_or_side_effect",
			"state":        "satisfied",
			"evidenceRefs": []string{string(readStepID), string(actionStepID)},
		}},
	})
	if err != nil {
		t.Fatalf("validateDoDAssessmentEvidenceQuality: %v", err)
	}
}

func TestDoDAssessmentEvidenceKindRequirementRejectsRejectedOnlyRequirement(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-evidence-kind-rejected-only")
	readStepID := common.StepID("run-dod-evidence-kind-rejected-only/block/read")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    readStepID,
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateCompleted,
	}); err != nil {
		t.Fatalf("seed read step: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{
							{
								"id":                "runtime_entrypoint",
								"scope":             "acceptanceDimension",
								"rejectedStepKinds": []string{"file/read"},
							},
						},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"acceptanceDimensions": []map[string]any{{
			"id":           "runtime_entrypoint",
			"state":        "satisfied",
			"evidenceRefs": []string{string(readStepID)},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "file/read") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want rejected-only requirement failure", err)
	}
}

func TestDoDAssessmentEvidenceKindRequirementRejectsUntraceableDirectEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-direct-evidence")
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                    "runtime_entrypoint",
							"scope":                 "acceptanceDimension",
							"requireDirectEvidence": true,
							"acceptedStepKinds":     []string{shellcommon.StepKindCommand},
						}},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"acceptanceDimensions": []map[string]any{{
			"id":           "runtime_entrypoint",
			"state":        "satisfied",
			"evidenceRefs": []string{"run-dod-direct-evidence/graph-001/999-unknown"},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "traceable direct evidenceRef") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want untraceable direct evidence rejection", err)
	}
}

func TestDoDAssessmentClaimEvidenceRequirementRejectsGenericValidationEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-claim-generic")
	stepID := common.StepID("run-dod-claim-generic/graph-001/001-go-test")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
		Input: map[string]any{"command": "go", "args": []string{"test", "-race", "./..."}},
		Output: map[string]any{
			"stdout": "ok  \texample.com/project\t(cached)",
		},
	}); err != nil {
		t.Fatalf("seed shell step: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                    "*",
							"scope":                 "dodItem",
							"requireDirectEvidence": true,
							"requireClaimEvidence":  true,
							"acceptedStepKinds":     []string{shellcommon.StepKindCommand, "patch/apply"},
						}},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"items": []map[string]any{{
			"item":         "streaming response emits incremental chunks",
			"state":        "satisfied",
			"evidenceRefs": []string{string(stepID)},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) || !strings.Contains(err.Error(), "coverageTargets") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want claim evidence alignment rejection", err)
	}
}

func TestDoDAssessmentClaimEvidenceRequirementAcceptsAlignedCoverageTarget(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-claim-aligned")
	stepID := common.StepID("run-dod-claim-aligned/graph-001/001-implement-stream")
	claim := "streaming response emits incremental chunks"
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "patch/apply",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"targetRef":       "internal/proxy/proxy.go",
			"coverageTargets": []string{claim},
		},
		Output: map[string]any{"status": "applied"},
	}); err != nil {
		t.Fatalf("seed patch step: %v", err)
	}
	run := &runmodel.Run{
		ID: runID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                    "*",
							"scope":                 "dodItem",
							"requireDirectEvidence": true,
							"requireClaimEvidence":  true,
							"acceptedStepKinds":     []string{shellcommon.StepKindCommand, "patch/apply"},
						}},
					},
				},
			},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, map[string]any{
		"items": []map[string]any{{
			"item":         claim,
			"state":        "satisfied",
			"evidenceRefs": []string{string(stepID)},
		}},
	})
	if err != nil {
		t.Fatalf("validateDoDAssessmentEvidenceQuality: %v", err)
	}
}

func TestCompletionGateNormalizesEvidenceRefsToCanonicalUsableRuntimeEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	validationRepo := memory.NewValidationResultRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: validationRepo,
		},
	}
	runID := common.RunID("run-dod-evidence-normalize")
	validStepID := common.StepID(runID + "/graph-001/003-verify-full-regression")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    validStepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
		Input: map[string]any{"graphNodeId": "verify_full_regression"},
	}); err != nil {
		t.Fatalf("seed valid step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/follow-up-graph-planning-001-verify_full_regression"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage": "llm_dod_assessment",
			"error": "coverage correction needed",
		},
	}); err != nil {
		t.Fatalf("seed follow-up assessment step: %v", err)
	}
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(validStepID) + "/validation/runtime-step"),
		WorkspaceID:     "ws-dod-evidence-normalize",
		RunID:           runID,
		StepID:          validStepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStatePassed,
		EvidenceQuality: common.EvidenceQualityValid,
	}); err != nil {
		t.Fatalf("seed valid validation: %v", err)
	}
	oldFailedStepID := common.StepID("run-old/graph-025/002-verify-targeted-streaming-fallback-tests")
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(oldFailedStepID) + "/validation/runtime-step"),
		WorkspaceID:     "ws-dod-evidence-normalize",
		RunID:           "run-old",
		StepID:          oldFailedStepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStateFailed,
		EvidenceQuality: common.EvidenceQualityInvalid,
		EvidenceReason:  "runtime step failed",
	}); err != nil {
		t.Fatalf("seed failed validation: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		DoDID:           "dod-dod-evidence-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "done",
			"items": []map[string]any{{
				"item":         "full regression",
				"state":        "satisfied",
				"evidenceRefs": []string{string(runID) + "/follow-up-graph-planning-001-verify_full_regression", string(runID) + "/verify_full_regression", string(oldFailedStepID)},
			}},
		},
	}

	normalized, err := normalizeCompletionGateDoDAssessmentEvidenceRefs(ctx, orch.steps, orch.evidence, run, output)
	if err != nil {
		t.Fatalf("normalizeCompletionGateDoDAssessmentEvidenceRefs: %v", err)
	}
	assessment := doDAssessmentFromOutput(normalized)
	items, _ := doDAssessmentItemMaps(assessment["items"])
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one normalized item", items)
	}
	if got := evidenceRefsFromValue(items[0]["evidenceRefs"]); !reflect.DeepEqual(got, []string{string(validStepID)}) {
		t.Fatalf("evidenceRefs = %+v, want canonical valid step only", got)
	}
	if err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, assessment); err != nil {
		t.Fatalf("validate normalized evidence: %v", err)
	}
}

func TestCompletionGateNormalizesSupersededFailureEvidenceToLatestSuccess(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	runID := common.RunID("run-dod-evidence-superseded")
	oldFailedStepID := common.StepID(runID + "/graph-006/002-rerun-targeted-goal-test")
	latestSuccessStepID := common.StepID(runID + "/graph-007/002-rerun-targeted-goal-test")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     oldFailedStepID,
		RunID:  runID,
		Kind:   shellcommon.StepKindCommand,
		State:  common.StepStateFailed,
		Output: map[string]any{"stderr": "old failure"},
	}); err != nil {
		t.Fatalf("seed old failed step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     latestSuccessStepID,
		RunID:  runID,
		Kind:   shellcommon.StepKindCommand,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stdout": "latest targeted validation passed"},
	}); err != nil {
		t.Fatalf("seed latest success step: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		DoDID:           "dod-dod-evidence-superseded",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "done",
			"items": []map[string]any{{
				"item":         "targeted goal validation",
				"state":        "satisfied",
				"evidenceRefs": []string{string(oldFailedStepID)},
			}},
		},
	}

	normalized, err := normalizeCompletionGateDoDAssessmentEvidenceRefs(ctx, orch.steps, orch.evidence, run, output)
	if err != nil {
		t.Fatalf("normalizeCompletionGateDoDAssessmentEvidenceRefs: %v", err)
	}
	assessment := doDAssessmentFromOutput(normalized)
	items, _ := doDAssessmentItemMaps(assessment["items"])
	if got := evidenceRefsFromValue(items[0]["evidenceRefs"]); !reflect.DeepEqual(got, []string{string(latestSuccessStepID)}) {
		t.Fatalf("evidenceRefs = %+v, want latest successful evidence", got)
	}
}

func TestCompletionGatePreservesGovernanceOwnerEvidenceRefs(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	run := &runmodel.Run{
		ID:              common.RunID("run-governance-owner-evidence-normalize"),
		DoDID:           "dod-governance-owner-evidence-normalize",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	output := map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "done",
			"items": []map[string]any{
				{
					"item":         "two Decision tickets are resolved",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-001-step-1-decision", "run-001-llm-after-decision-decision", "loose note"},
				},
				{
					"item":         "one Approval is approved",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-001-llm-after-decision-2-approval"},
				},
			},
		},
	}

	normalized, err := normalizeCompletionGateDoDAssessmentEvidenceRefs(ctx, orch.steps, orch.evidence, run, output)
	if err != nil {
		t.Fatalf("normalizeCompletionGateDoDAssessmentEvidenceRefs: %v", err)
	}
	assessment := doDAssessmentFromOutput(normalized)
	items, _ := doDAssessmentItemMaps(assessment["items"])
	if len(items) != 2 {
		t.Fatalf("items = %+v, want two normalized items", items)
	}
	if got := evidenceRefsFromValue(items[0]["evidenceRefs"]); !reflect.DeepEqual(got, []string{"run-001-step-1-decision", "run-001-llm-after-decision-decision"}) {
		t.Fatalf("decision evidenceRefs = %+v, want owner refs with loose note removed", got)
	}
	if got := evidenceRefsFromValue(items[1]["evidenceRefs"]); !reflect.DeepEqual(got, []string{"run-001-llm-after-decision-2-approval"}) {
		t.Fatalf("approval evidenceRefs = %+v, want owner approval ref", got)
	}
}

func TestDefinitionChangeAssessmentRejectsHistoricalOnlyEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	validationRepo := memory.NewValidationResultRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: validationRepo,
		},
	}
	runID := common.RunID("run-definition-change-current-evidence")
	currentValidationRef := string(runID) + "/graph-001/001-verify-latest/validation/runtime-step"
	currentStepID := common.StepID(runID + "/graph-001/001-verify-latest")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     currentStepID,
		RunID:  runID,
		Kind:   shellcommon.StepKindCommand,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stdout": "latest definition verified"},
	}); err != nil {
		t.Fatalf("seed current step: %v", err)
	}
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(currentValidationRef),
		WorkspaceID:     "ws-definition-change-current-evidence",
		RunID:           runID,
		StepID:          currentStepID,
		ValidationKind:  "runtime_step",
		State:           common.ValidationResultStatePassed,
		EvidenceQuality: common.EvidenceQualityValid,
	}); err != nil {
		t.Fatalf("seed current validation: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-definition-change-current-evidence",
		DoDID:           "dod-definition-change-current-evidence",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"definitionChangeImpact": map[string]any{"changed": true},
		},
	}
	assessment := map[string]any{
		"satisfied": true,
		"summary":   "done",
		"items": []map[string]any{{
			"item":         "new DoD item",
			"state":        "satisfied",
			"evidenceRefs": []string{"run-old/graph-001/001-verify-old"},
		}},
		"goalCoverage": map[string]any{
			"satisfied": true,
			"items": []map[string]any{{
				"item":         "new Goal requirement",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-old/graph-001/001-verify-old"},
			}},
		},
		"acceptanceDimensions": []map[string]any{{
			"id":           "functional_behavior",
			"state":        "satisfied",
			"evidenceRefs": []string{"run-old/graph-001/001-verify-old"},
		}},
		"deliverableCoverage": map[string]any{
			"items": []map[string]any{{
				"item":         "validation_report",
				"state":        "satisfied",
				"artifactRefs": []string{"run-old/artifact/result-summary"},
			}},
		},
	}
	err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, assessment)
	if !errors.Is(err, common.ErrInvalidArgument) ||
		!strings.Contains(err.Error(), "requires current run evidenceRefs") {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %v, want current-run evidence rejection", err)
	}
	var diagnostic runtimeFailureDiagnosticError
	if !errors.As(err, &diagnostic) {
		t.Fatalf("validateDoDAssessmentEvidenceQuality err = %T, want runtime diagnostics", err)
	}
	missing := mapSliceFromValue(diagnostic.RuntimeFailureDiagnostics()["missingDoDItems"])
	if len(missing) == 0 || missing[0]["source"] != "definitionChangeCurrentRunEvidence" {
		t.Fatalf("runtime diagnostics = %+v, want definition-change evidence gap", diagnostic.RuntimeFailureDiagnostics())
	}

	assessment["items"] = []map[string]any{{
		"item":         "new DoD item",
		"state":        "satisfied",
		"evidenceRefs": []string{"run-old/graph-001/001-verify-old", currentValidationRef},
	}}
	goalCoverage := mapFromValue(assessment["goalCoverage"])
	goalCoverage["items"] = []map[string]any{{
		"item":         "new Goal requirement",
		"state":        "satisfied",
		"evidenceRefs": []string{currentValidationRef},
	}}
	assessment["goalCoverage"] = goalCoverage
	assessment["acceptanceDimensions"] = []map[string]any{{
		"id":           "functional_behavior",
		"state":        "satisfied",
		"evidenceRefs": []string{currentValidationRef},
	}}
	deliverableCoverage := mapFromValue(assessment["deliverableCoverage"])
	deliverableCoverage["items"] = []map[string]any{{
		"item":         "validation_report",
		"state":        "satisfied",
		"evidenceRefs": []string{currentValidationRef},
	}}
	assessment["deliverableCoverage"] = deliverableCoverage
	if err := validateDoDAssessmentEvidenceQuality(ctx, orch.steps, orch.evidence, run, assessment); err != nil {
		t.Fatalf("validateDoDAssessmentEvidenceQuality with current evidence: %v", err)
	}
}
