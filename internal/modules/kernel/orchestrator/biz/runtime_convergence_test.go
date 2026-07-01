package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMain(m *testing.M) {
	configDir, err := os.MkdirTemp("", "workorch-convergence-test-config-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(configDir)
	if err := writeConvergenceTestConfig(configDir); err != nil {
		panic(err)
	}
	_ = workorchconfig.Close()
	_ = os.Setenv(workorchconfig.EnvConfigDir, configDir)
	code := m.Run()
	_ = workorchconfig.Close()
	os.Exit(code)
}

func writeConvergenceTestConfig(configDir string) error {
	return os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(`
endpointName = "workorch"

[runtime.git]
defaultTimeout = "30s"

[runtime.llmOutput]
maxPatchContentBytes = 12288
maxGraphBytes = 32768
maxGraphFileWrites = 3
schemaInvalidLargePreviewThreshold = 8192
maxArtifactChunkBytes = 8192
previousChunkTailBytes = 300
maxChunkRetryAttempts = 3
maxArtifactPlanAttempts = 2
maxArtifactReplanAttempts = 2
`), 0o600)
}

func TestBuildConvergenceContractNeedStateChangeWithTargetEvidence(t *testing.T) {
	step := &runmodel.Step{
		ID: "run-convergence/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"exists":           true,
					"contentHash":      "hash-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
	}

	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": step.Input}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedStateChange)
	}
	if !contract.StateChangingOutputRequired {
		t.Fatalf("stateChangingOutputRequired = false, want true")
	}
	if len(contract.TargetEvidence) == 0 {
		t.Fatalf("target evidence missing")
	}
}

func TestBuildConvergenceContractUsesProfileRequiredStateChangingEvidenceForCoverageTarget(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-evidence",
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
	step := &runmodel.Step{
		ID: "run-convergence-profile-evidence/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveConcreteValidationEvidence,
			"coverageTargetSet": []map[string]any{{
				"kind":                "deliverable",
				"item":                "version_control_snapshot",
				"coverageTargetToken": "version_control_snapshot",
				"targetText":          "version_control_snapshot",
				"state":               "failed",
				"source":              "deliverableCoverage",
			}},
			"repairActionPackage": map[string]any{
				"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
			},
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": step.Input}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedStateChange)
	}
	if !contract.StateChangingOutputRequired || contract.RepairObjective != repairObjectiveStateChangingOutput {
		t.Fatalf("contract = %+v, want state-changing repair objective", contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("requiredOutput = %+v, want git/write commit accepted state-changing evidence", required)
	}
	if strings.TrimSpace(stringFromMap(required, "supportingEvidenceNodeRule")) == "" {
		t.Fatalf("requiredOutput = %+v, want supporting evidence node rule", required)
	}
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, should not introduce unrelated state-changing kinds", required["acceptedKinds"])
	}
	if targets := mapSliceFromValue(contract.RepairActionPackage["coverageTargetSet"]); len(targets) != 1 || targets[0]["item"] != "version_control_snapshot" {
		t.Fatalf("coverageTargetSet = %+v, want version_control_snapshot target", targets)
	}
}

func TestBuildConvergenceContractClearsStaleEvidenceAcquisitionForProfileStateChange(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-work-item",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                 "snapshot-output",
							"scope":              "deliverable",
							"acceptedStepKinds":  []string{gitcommon.StepKindWrite},
							"acceptedOperations": []string{gitcommon.OperationCommit},
						}},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID: "run-convergence-profile-work-item/repair-004",
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeStageMode:      "strict_convergence",
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"concreteFailureEvidenceRequired": true,
			coverageWorkItemContextKey: map[string]any{
				"currentFocus": map[string]any{
					"id":                    "cwi-snapshot",
					"status":                "failed",
					"source":                "deliverable",
					"sourceScope":           "deliverable",
					"sourceRequirementId":   "snapshot-output",
					"targetText":            "snapshot-output",
					"requiredEvidenceKinds": []string{"validation", "state_change"},
					"missingEvidenceKinds":  []string{"validation", "state_change"},
				},
			},
			"repairActionPackage": map[string]any{
				"evidenceAcquisitionRequired": true,
				"requiredOutput": map[string]any{
					"mustProduceConcreteFailureEvidence": true,
					"stateChangingForbidden":             true,
					"acceptedKinds":                      []string{"file/read", gitcommon.StepKindRead},
				},
			},
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": step.Input}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	if !contract.StateChangingOutputRequired || contract.RepairObjective != repairObjectiveStateChangingOutput {
		t.Fatalf("contract = %+v, want state-changing objective", contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if boolValue(contract.RepairActionPackage["evidenceAcquisitionRequired"]) ||
		boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		boolValue(required["stateChangingForbidden"]) {
		t.Fatalf("repairActionPackage.requiredOutput = %+v package=%+v, want stale evidence-acquisition flags cleared", required, contract.RepairActionPackage)
	}
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("requiredOutput = %+v, want profile git/write state-changing route", required)
	}
}

func TestBuildConvergenceContractDoesNotReopenPrimaryStateChangeWhenOnlyValidationMissing(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-evidence-mixed-targets",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{
							{
								"id":                "primary-output",
								"scope":             "deliverable",
								"evidenceRole":      "primary_state_change",
								"acceptedStepKinds": []string{"patch/apply"},
							},
							{
								"id":                 "snapshot-output",
								"scope":              "deliverable",
								"evidenceRole":       "supplemental_evidence",
								"acceptedStepKinds":  []string{gitcommon.StepKindWrite},
								"acceptedOperations": []string{gitcommon.OperationCommit},
							},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID: "run-convergence-profile-evidence-mixed-targets/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageRepair,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveStateChangingOutput,
			"coverageTargetSet": []map[string]any{
				{
					"kind":                  "deliverable",
					"source":                "deliverable",
					"sourceScope":           "deliverable",
					"sourceRequirementId":   "primary-output",
					"sourcePath":            "executionContract.profileContracts.neutral-profile.deliverableExpectations[0]",
					"targetText":            "primary-output",
					"state":                 "failed",
					"evidenceRole":          "primary_state_change",
					"requiredEvidenceKinds": []string{"validation", "state_change"},
					"missingEvidenceKinds":  []string{"validation"},
					"acceptedEvidenceRefs":  []string{"run/graph/001-primary-output"},
				},
				{
					"kind":                  "deliverable",
					"source":                "deliverable",
					"sourceScope":           "deliverable",
					"sourceRequirementId":   "snapshot-output",
					"sourcePath":            "executionContract.profileContracts.neutral-profile.deliverableExpectations[1]",
					"targetText":            "snapshot-output",
					"state":                 "open",
					"evidenceRole":          "supplemental_evidence",
					"requiredEvidenceKinds": []string{"validation", "state_change"},
					"missingEvidenceKinds":  []string{"validation", "state_change"},
				},
			},
			"coverageWorkItemContext": map[string]any{
				"currentFocus": map[string]any{
					"id":                    "cwi-snapshot",
					"source":                "deliverable",
					"sourceScope":           "deliverable",
					"sourceRequirementId":   "snapshot-output",
					"targetText":            "snapshot-output",
					"status":                "open",
					"evidenceRole":          "supplemental_evidence",
					"requiredEvidenceKinds": []string{"validation", "state_change"},
					"missingEvidenceKinds":  []string{"validation", "state_change"},
				},
			},
			"repairActionPackage": map[string]any{
				"coverageFocus": map[string]any{
					"id":                    "stale-primary-focus",
					"sourceRequirementId":   "primary-output",
					"targetText":            "primary-output",
					"state":                 "failed",
					"evidenceRole":          "primary_state_change",
					"requiredEvidenceKinds": []string{"validation", "state_change"},
					"missingEvidenceKinds":  []string{"validation"},
					"acceptedEvidenceRefs":  []string{"run/graph/001-primary-output"},
				},
			},
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": step.Input}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedStateChange)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if required["mustIncludePrimaryStateChangingNode"] == true {
		t.Fatalf("requiredOutput = %+v, should not reopen primary state-change when the primary target only misses validation", required)
	}
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("acceptedKinds = %+v, should not include unrelated primary patch kind", required["acceptedKinds"])
	}
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("requiredOutput = %+v, want supplemental git/write commit contract", required)
	}
}

func TestBuildConvergenceContractPreservesStrategyReassessmentForProfileStateChange(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-strategy-reassessment",
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
	step := &runmodel.Step{
		ID: "run-convergence-profile-strategy-reassessment/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveConcreteValidationEvidence,
			"coverageTargetSet": []map[string]any{{
				"kind":                "deliverable",
				"item":                "version_control_snapshot",
				"coverageTargetToken": "version_control_snapshot",
				"targetText":          "version_control_snapshot",
				"state":               "failed",
				"source":              "deliverableCoverage",
			}},
			"repairActionPackage": map[string]any{
				"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
				"strategyReassessment": map[string]any{
					"required": true,
					"requiredDecisionFields": []string{
						"failedSurface",
						"rootCauseTarget",
						"repairTargetDecision",
						"repairMode",
						"forbiddenRepair",
						"verificationPlan",
					},
				},
			},
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": step.Input}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedStateChange)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !boolValue(required["mustDeclareStrategyReassessmentFields"]) {
		t.Fatalf("requiredOutput = %+v, want strategy reassessment declaration", required)
	}
	if modes := stringSliceFromValue(required["strategyReassessmentRepairModes"]); !stringSliceContains(modes, "continue_same_target_repair") ||
		!stringSliceContains(modes, "shift_root_cause_target") {
		t.Fatalf("strategyReassessmentRepairModes = %+v, want enum modes", required["strategyReassessmentRepairModes"])
	}
	if fields := stringSliceFromValue(required["strategyReassessmentRequiredFields"]); !stringSliceContains(fields, "repairMode") {
		t.Fatalf("strategyReassessmentRequiredFields = %+v, want repairMode", fields)
	}
}

func TestBuildConvergenceContractDefersPrimaryStateChangeWithoutTargetWhenProfileWriteEvidenceIsSupplementary(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-evidence-mixed",
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
	step := &runmodel.Step{
		ID: "run-convergence-profile-evidence-mixed/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveConcreteValidationEvidence,
			"coverageTargetSet": []map[string]any{
				{
					"kind":                "deliverable",
					"item":                "source_change",
					"coverageTargetToken": "source_change",
					"targetText":          "source_change",
					"state":               "failed",
					"source":              "deliverableCoverage",
				},
				{
					"kind":                "deliverable",
					"item":                "version_control_snapshot",
					"coverageTargetToken": "version_control_snapshot",
					"targetText":          "version_control_snapshot",
					"state":               "failed",
					"source":              "deliverableCoverage",
				},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
			},
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": step.Input}, nil)

	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedEvidence)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) ||
		boolValue(required["mustIncludePrimaryStateChangingNode"]) {
		t.Fatalf("requiredOutput = %+v, want primary state-changing deferred until target evidence exists", required)
	}
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want evidence acquisition kinds only", required["acceptedKinds"])
	}
	if len(stringSliceFromValue(required["acceptedOperations"])) != 0 {
		t.Fatalf("requiredOutput.acceptedOperations = %+v, want no operation narrowing before target evidence exists", required["acceptedOperations"])
	}
	if !boolValue(required["profileEvidenceRequirementDeferred"]) {
		t.Fatalf("requiredOutput = %+v, want profile evidence deferred marker", required)
	}
}

func TestBuildConvergenceContractAllowsPrimaryDeliverableStateChangeWithoutTargetEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-primary-deliverable-new-workspace",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                 "version_control_snapshot",
							"scope":              "deliverable",
							"evidenceRole":       "supplemental_evidence",
							"acceptedStepKinds":  []string{gitcommon.StepKindWrite},
							"acceptedOperations": []string{gitcommon.OperationCommit},
						}},
					},
				},
			},
		},
	}
	sourceChange := map[string]any{
		"id":                    "cwi-001",
		"sourceScope":           "deliverable",
		"sourceRequirementId":   "source_change",
		"targetText":            "source_change",
		"evidenceRole":          "primary_state_change",
		"status":                "open",
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"missingEvidenceKinds":  []string{"state_change", "validation"},
	}
	versionSnapshot := map[string]any{
		"id":                    "cwi-002",
		"sourceScope":           "deliverable",
		"sourceRequirementId":   "version_control_snapshot",
		"targetText":            "version_control_snapshot",
		"evidenceRole":          "supplemental_evidence",
		"status":                "open",
		"requiredEvidenceKinds": []string{"validation"},
		"missingEvidenceKinds":  []string{"validation"},
	}
	step := &runmodel.Step{
		ID: "run-convergence-primary-deliverable-new-workspace/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveConcreteValidationEvidence,
			"coverageWorkItemContext": map[string]any{
				"currentFocus":      sourceChange,
				"currentFocusItems": []map[string]any{sourceChange, versionSnapshot},
			},
			"coverageTargetSet": []map[string]any{
				{
					"kind":                "deliverable",
					"item":                "source_change",
					"coverageTargetToken": "source_change",
					"targetText":          "source_change",
					"state":               "failed",
					"source":              "deliverableCoverage",
				},
				{
					"kind":                "deliverable",
					"item":                "version_control_snapshot",
					"coverageTargetToken": "version_control_snapshot",
					"targetText":          "version_control_snapshot",
					"state":               "failed",
					"source":              "deliverableCoverage",
				},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
			},
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": step.Input}, nil)

	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedEvidence, contract)
	}
	if !contract.ConcreteFailureEvidenceRequired || contract.RepairObjective != repairObjectiveAcquireFailureEvidence || contract.StateChangingOutputRequired {
		t.Fatalf("contract = %+v, want concrete evidence acquisition before state-changing repair", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if required["mustProduceConcreteFailureEvidence"] != true ||
		boolValue(required["mustIncludeStateChangingNode"]) ||
		!boolValue(required["profileEvidenceRequirementDeferred"]) {
		t.Fatalf("requiredOutput = %+v, want deferred profile state-change until concrete target evidence exists", required)
	}
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want patch/apply forbidden before target evidence", required["acceptedKinds"])
	}
}

func TestBuildConvergenceContractValidationEscalationNeedsTargetEvidenceBeforePatch(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-validation-escalation-no-target",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	failedStep := &runmodel.Step{
		ID:    "run-convergence-validation-escalation-no-target/validation",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{"repairObjective": repairObjectiveConcreteValidationEvidence},
		Output: map[string]any{
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "usage.csv append behavior is not satisfied",
				"items": []map[string]any{{
					"item":  "usage.csv append behavior",
					"state": "failed",
				}},
			},
		},
	}
	step := &runmodel.Step{
		ID: "run-convergence-validation-escalation-no-target/repair",
		Input: map[string]any{
			"validationRepairEscalatedToImplementation": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":       "dod",
				"targetText": "usage.csv append behavior",
				"state":      "failed",
			}},
			"coverageWorkItemContext": map[string]any{
				"currentFocus": map[string]any{
					"id":                    "cwi-usage",
					"sourceScope":           "dod_item",
					"sourceRequirementId":   "dod-usage",
					"targetText":            "usage.csv append behavior",
					"status":                "failed",
					"requiredEvidenceKinds": []string{"validation"},
					"missingEvidenceKinds":  []string{"validation"},
				},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
			},
		},
	}

	contract := BuildConvergenceContract(run, step, failedStep, map[string]any{"input": step.Input}, nil)

	if contract.State != convergenceStateNeedEvidence ||
		contract.RepairObjective != repairObjectiveAcquireFailureEvidence ||
		!contract.ConcreteFailureEvidenceRequired {
		t.Fatalf("contract = %+v, want evidence acquisition before state-changing patch", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) ||
		!boolValue(required["mustProduceConcreteFailureEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want concrete target evidence acquisition", required)
	}
}

func TestBuildConvergenceContractProfileStateChangePreservesCurrentTargetSnapshot(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-current-target",
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
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		stepInputKeyRuntimeRepairFollowUp: true,
		"repairObjective":                 repairObjectiveConcreteValidationEvidence,
		"runtimeRepairAttempt":            5,
		failureOutputKeyClass:             failureClassRuntime,
		failureOutputKeySignature:         "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=strategy_reassessment_wrapper",
		"strategyReassessment": map[string]any{
			"required":      true,
			"failedSurface": "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=strategy_reassessment_wrapper",
		},
		"coverageTargetSet": []map[string]any{
			{
				"kind":                "deliverable",
				"item":                "source_change",
				"coverageTargetToken": "source_change",
				"targetText":          "source_change",
				"state":               "failed",
				"source":              "deliverableCoverage",
			},
			{
				"kind":                "deliverable",
				"item":                "version_control_snapshot",
				"coverageTargetToken": "version_control_snapshot",
				"targetText":          "version_control_snapshot",
				"state":               "failed",
				"source":              "deliverableCoverage",
			},
		},
		"failureTargetRepairContext": []map[string]any{{
			"targetRef":        "target.txt",
			"exists":           true,
			"content":          "old\n",
			"contentHash":      "hash-target",
			"contentComplete":  true,
			"safeForFileWrite": true,
		}},
		"repairActionPackage": map[string]any{
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	step := &runmodel.Step{
		ID:    "run-convergence-profile-current-target/repair-005",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: input,
		Output: map[string]any{
			"stage": "llm_execution_graph_plan",
			"error": "invalid argument: strategyReassessment.required=true requires state-changing patch output",
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": input}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedStateChange)
	}
	pkg := mapFromValue(contract.RepairActionPackage)
	targets := mapSliceFromValue(pkg["targetSnapshots"])
	if len(targets) != 1 || targets[0]["targetRef"] != "target.txt" || targets[0]["contentHash"] != "hash-target" {
		t.Fatalf("targetSnapshots = %+v, want current failure target snapshot", targets)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustUseCurrentTargetSnapshot"]) ||
		!boolValue(required["mustPatchDirectlyFromProvidedSnapshot"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want current-snapshot direct patch contract", required)
	}
	reassessment := mapFromValue(pkg["strategyReassessment"])
	if got := stringFromMap(reassessment, "failedSurface"); got != "targetRef=target.txt" {
		t.Fatalf("strategyReassessment.failedSurface = %q, want concrete target surface", got)
	}
}

func TestBuildConvergenceContractUsesUnfocusedCoverageWorkItemStateChange(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-unfocused-cwi-state-change",
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
	step := &runmodel.Step{
		ID: "run-convergence-unfocused-cwi-state-change/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:     runtimeStageVerification,
			stepInputKeyRuntimeStageMode: "strict_convergence",
			"repairObjective":            repairObjectiveConcreteValidationEvidence,
			"coverageTargetSet": []map[string]any{{
				"kind":                "deliverable",
				"item":                "version_control_snapshot",
				"coverageTargetToken": "version_control_snapshot",
				"targetText":          "version_control_snapshot",
				"state":               "failed",
			}},
			coverageWorkItemContextKey: map[string]any{
				"currentFocus": map[string]any{
					"id":                   "cwi-001",
					"sourceScope":          "goal_leaf",
					"missingEvidenceKinds": []string{"validation"},
					"status":               "open",
				},
				"unresolvedItems": []map[string]any{{
					"id":                   "cwi-021",
					"sourceScope":          "deliverable",
					"sourceRequirementId":  "version_control_snapshot",
					"targetText":           "version_control_snapshot",
					"missingEvidenceKinds": []string{"validation", "state_change"},
					"status":               "open",
				}},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
			},
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": step.Input}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("requiredOutput = %+v, want git/write state-changing evidence requirement", required)
	}
	if context := mapFromValue(contract.RepairActionPackage[coverageWorkItemContextKey]); len(mapSliceFromValue(context["unresolvedItems"])) != 1 {
		t.Fatalf("coverageWorkItemContext = %+v, want unresolved CWI preserved", context)
	}
}

func TestStateChangingRepairActionPackageForConvergencePersistsStrategyReassessmentContract(t *testing.T) {
	reassessment := map[string]any{
		"required": true,
		"requiredDecisionFields": []string{
			"failedSurface",
			"rootCauseTarget",
			"repairTargetDecision",
			"repairMode",
			"forbiddenRepair",
			"verificationPlan",
		},
	}
	input := map[string]any{
		"repairActionPackage": map[string]any{
			"strategyReassessment": reassessment,
			"requiredOutput":       stateChangingRepairRequiredOutput(),
			"targetSnapshots": []map[string]any{{
				"targetRef":        "target.txt",
				"content":          "before\n",
				"contentHash":      "hash-target",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
	}

	pkg := stateChangingRepairActionPackageForConvergence(input, nil)
	gotRequired := mapFromValue(pkg["requiredOutput"])
	if !boolValue(gotRequired["mustDeclareStrategyReassessmentFields"]) {
		t.Fatalf("requiredOutput = %+v, want strategy reassessment declaration persisted", gotRequired)
	}
	if !stringSliceContains(stringSliceFromValue(gotRequired["strategyReassessmentRepairModes"]), "continue_same_target_repair") ||
		!stringSliceContains(stringSliceFromValue(gotRequired["strategyReassessmentRepairModes"]), "shift_root_cause_target") {
		t.Fatalf("strategyReassessmentRepairModes = %+v, want repair mode enum contract", gotRequired["strategyReassessmentRepairModes"])
	}
	if fields := stringSliceFromValue(gotRequired["strategyReassessmentRequiredFields"]); !stringSliceContains(fields, "repairMode") {
		t.Fatalf("strategyReassessmentRequiredFields = %+v, want repairMode required", fields)
	}
}

func TestBuildConvergenceContractKeepsProfileWriteEvidenceAfterGitWriteFailure(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-git-write-failure",
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
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired":  true,
		"coverageTargetSet": []map[string]any{{
			"kind":                "deliverable",
			"item":                "version_control_snapshot",
			"sourceRequirementId": "version_control_snapshot",
			"coverageTargetToken": "version_control_snapshot",
			"targetText":          "version_control_snapshot",
			"state":               "failed",
		}},
		"repairActionPackage": map[string]any{
			"coverageRepairRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":                "deliverable",
				"item":                "version_control_snapshot",
				"sourceRequirementId": "version_control_snapshot",
				"coverageTargetToken": "version_control_snapshot",
				"targetText":          "version_control_snapshot",
				"state":               "failed",
			}},
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-git-write-failure/graph-001/003-stage-all",
		Kind:   gitcommon.StepKindWrite,
		Input:  map[string]any{"operation": gitcommon.OperationAdd, "pathspecs": []string{"."}},
		Output: map[string]any{"operation": gitcommon.OperationAdd, "error": "invalid argument: broad pathspec is not allowed"},
		State:  common.StepStateFailed,
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("requiredOutput = %+v, want git/write commit contract preserved after git/write failure", required)
	}
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "shell/command") {
		t.Fatalf("requiredOutput = %+v, must not downgrade profile write evidence to shell/command", required)
	}
}

func TestBuildConvergenceContractBlocksProfileEvidenceWhenAcceptedKindUnavailable(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-evidence-unavailable-kind",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                 "profile_snapshot",
							"scope":              "deliverable",
							"acceptedStepKinds":  []string{"artifact/write"},
							"acceptedOperations": []string{"seal"},
						}},
					},
				},
			},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"coverageTargetSet": []map[string]any{{
			"kind":                "deliverable",
			"item":                "profile_snapshot",
			"coverageTargetToken": "profile_snapshot",
			"targetText":          "profile_snapshot",
			"state":               "failed",
			"source":              "deliverableCoverage",
		}},
		"repairActionPackage": map[string]any{
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-profile-evidence-unavailable-kind/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid argument: satisfied deliverable references insufficient evidence"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedGovernance {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedGovernance, contract)
	}
	gap := mapFromValue(contract.RepairActionPackage["runtimeCapabilityGap"])
	if !stringSliceContains(stringSliceFromValue(gap["missingKinds"]), "artifact/write") {
		t.Fatalf("runtimeCapabilityGap = %+v, want missing artifact/write", gap)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "shell/command") ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "artifact/write") {
		t.Fatalf("requiredOutput = %+v, want no substitute executable accepted kinds", required)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied["governanceRequired"] != true || stringFromMap(applied, "runtimeStage") != runtimeStageDecision {
		t.Fatalf("applied input = %+v, want governance decision stage", applied)
	}
}

func TestBuildConvergenceContractEscalatesValidationFailureWithDiagnosticsToStateChange(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageVerification,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		stepInputKeyRuntimeRepairFollowUp: true,
		"repairObjective":                 repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired":       true,
		"repairActionPackage": map[string]any{
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
			"lastFailureDiagnostics": map[string]any{
				"failureTargets": []string{"target_test.txt"},
				"stdoutPreview":  "target_test.txt: expected active=true",
				"exitCode":       1,
			},
			"repairTargetSet": []string{"target.txt", "target_test.txt"},
			"targetSnapshots": []map[string]any{{
				"targetRef":        "target.txt",
				"exists":           true,
				"contentHash":      "hash-target",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                   "cwi-001",
				"status":               "open",
				"sourceScope":          "dod_item",
				"sourceRequirementId":  "dod-1",
				"targetText":           "runtime validation must pass",
				"missingEvidenceKinds": []string{"validation"},
			},
		},
	}
	step := &runmodel.Step{
		ID:     "run-convergence/repair-validation",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  input,
		Output: map[string]any{"error": "previous validation failed"},
	}

	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	if !contract.StateChangingOutputRequired || contract.RepairObjective != repairObjectiveStateChangingOutput {
		t.Fatalf("contract = %+v, want state-changing repair objective", contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!boolValue(required["mustReferenceCoverageWorkItem"]) {
		t.Fatalf("requiredOutput = %+v, want state-changing coverage repair contract", required)
	}
}

func TestBuildConvergenceContractProfileRequiredEvidencePreemptsAssessmentOnlyCorrection(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-evidence-assessment-only",
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
	input := map[string]any{
		stepInputKeyAssessmentOnly: true,
		"repairObjective":          "complete_dod_goal_coverage_assessment",
		"repairGuidance": []string{
			"Return only completed or failed dodAssessment, or a real governance blocker.",
			"Do not return executionGraph, shellCommands, patchRequests, mcpToolCalls, memoryWrites, or other executable work.",
		},
		"missingDeliverableCoverage": []map[string]any{{
			"id":    "version_control_snapshot",
			"kind":  "deliverableExpectation",
			"state": "missing",
		}},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-profile-evidence-assessment-only/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid argument: dodAssessment.deliverableCoverage missing required deliverables: version_control_snapshot"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("requiredOutput = %+v, want profile-required git/write commit", required)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied[stepInputKeyAssessmentOnly] != nil || applied["repairObjective"] != repairObjectiveStateChangingOutput || applied["stateChangingOutputRequired"] != true {
		t.Fatalf("applied input = %+v, want state-changing repair not assessment-only", applied)
	}
	guidance := strings.Join(stringListFromValue(applied["repairGuidance"]), "\n")
	for _, forbidden := range []string{"Return only completed or failed dodAssessment", "Do not return executionGraph"} {
		if strings.Contains(guidance, forbidden) {
			t.Fatalf("repairGuidance contains stale assessment-only instruction %q: %s", forbidden, guidance)
		}
	}
	if !strings.Contains(guidance, "dodAssessment alone cannot satisfy") {
		t.Fatalf("repairGuidance = %s, want executable state-changing guidance", guidance)
	}
}

func TestBuildConvergenceContractProfilePrimaryTargetDoesNotDependOnCurrentFocus(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-primary-target",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                "primary_output",
							"scope":             "deliverable",
							"evidenceRole":      "primary_state_change",
							"acceptedStepKinds": []string{"patch/apply"},
							"requireDirect":     true,
						}},
					},
				},
			},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageRepair,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveAcquireFailureEvidence,
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-ready-goal",
				"sourceScope":           "goal_leaf",
				"sourceRequirementId":   "goal-ready",
				"targetText":            "already ready goal",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
			},
		},
		"coverageTargetSet": []map[string]any{{
			"kind":                  "deliverable",
			"source":                "deliverableCoverage",
			"sourceScope":           "deliverable",
			"sourceRequirementId":   "primary_output",
			"item":                  "primary_output",
			"targetText":            "primary_output",
			"coverageTargetToken":   "primary_output",
			"evidenceRole":          "primary_state_change",
			"requiredEvidenceKinds": []string{"validation", "state_change"},
			"missingEvidenceKinds":  []string{"state_change"},
		}},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-profile-primary-target/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid argument: profile primary state-changing evidence missing"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedEvidence, contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !boolValue(required["stateChangingForbidden"]) ||
		!boolValue(required["mustProduceConcreteFailureEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want concrete evidence acquisition before profile primary state change", required)
	}
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want patch/apply deferred until target evidence exists", required["acceptedKinds"])
	}
}

func TestApplyConvergenceContractNeedGovernanceClearsExecutableRepairFollowUp(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		"runtimeRepairFollowUp":           true,
		stepInputKeyProviderRetryFollowUp: true,
		"repairObjective":                 repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired":     true,
		"boundedValidationRequired":       true,
		"repairActionPackage": map[string]any{
			"currentFailure": map[string]any{
				"failedStepId": "run-governance/graph-001/action",
				"reason":       "operator decision required",
			},
			"coverageTargetSet": []map[string]any{{
				"sourceRequirementId": "goal-leaf-001",
				"targetText":          "goal leaf",
			}},
		},
	}
	contract := ConvergenceContract{
		State:            convergenceStateNeedGovernance,
		RuntimeStage:     runtimeStageDecision,
		RuntimeStageMode: "structured_governance",
		GovernanceReason: "operator decision required",
	}

	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	pkg := mapFromValue(applied["repairActionPackage"])

	for _, key := range []string{
		"runtimeRepairFollowUp",
		stepInputKeyProviderRetryFollowUp,
		"repairObjective",
		"stateChangingOutputRequired",
		"concreteFailureEvidenceRequired",
		"boundedValidationRequired",
		"evidenceRepairRequired",
	} {
		if _, ok := applied[key]; ok {
			t.Fatalf("applied[%q] still present in governance input: %+v", key, applied)
		}
	}
	if len(mapFromValue(pkg["requiredOutput"])) > 0 {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want stripped for governance", pkg["requiredOutput"])
	}
	if len(mapFromValue(pkg["currentFailure"])) == 0 {
		t.Fatalf("repairActionPackage = %+v, want current failure context preserved", pkg)
	}
	if applied["governanceRequired"] != true || applied["governanceReason"] != "operator decision required" {
		t.Fatalf("governance fields = required:%v reason:%v", applied["governanceRequired"], applied["governanceReason"])
	}
}

func TestRuntimeRepairFollowUpProfileEvidenceKindFailurePreemptsAssessmentOnly(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-profile-evidence-kind-gap",
		WorkspaceID:     "ws-profile-evidence-kind-gap",
		InitialStepKind: llmWorkspaceExecutionStepKind,
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
	failed := &runmodel.Step{
		ID:    "run-profile-evidence-kind-gap/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly: true,
			"repairObjective":          "complete_dod_goal_coverage_assessment",
		},
		Output: map[string]any{
			"error": "invalid argument: satisfied dodAssessment deliverable item \"version_control_snapshot\" references insufficient evidence \"run/graph/001-read\"; requires evidence from step kinds: git/write",
			"runtimeDiagnostics": map[string]any{
				"kind":  "dod_evidence_quality_gap",
				"scope": "deliverable",
				"missingDeliverableCoverage": []map[string]any{{
					"kind":         "leaf",
					"item":         "version_control_snapshot",
					"state":        "failed",
					"source":       "evidenceKind",
					"evidenceRefs": []string{"run/graph/001-read"},
					"reason":       "requires evidence from step kinds: git/write",
				}},
			},
		},
	}
	orch := &RuntimeOrchestrator{}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, nil)
	contract := mapFromValue(input["convergenceContract"])
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])

	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want profile-required state-changing evidence repair", contract)
	}
	if input[stepInputKeyAssessmentOnly] != nil || input["stateChangingOutputRequired"] != true || input["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("repair input = %+v, want executable state-changing repair", input)
	}
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("requiredOutput = %+v, want profile accepted kind/operation", required)
	}
}

func TestBuildConvergenceContractProfileEvidenceKindFailureIgnoresReadyCoverageNoise(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-profile-evidence-kind-gap-with-ready-noise",
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
	input := map[string]any{
		stepInputKeyAssessmentOnly: true,
		"repairObjective":          "complete_dod_goal_coverage_assessment",
		"missingDeliverableCoverage": []map[string]any{{
			"item":         "version_control_snapshot",
			"kind":         "deliverable",
			"source":       "deliverableCoverage",
			"state":        "failed",
			"evidenceRefs": []string{"run/graph/001-read"},
		}},
		"coverageTargetSet": []map[string]any{{
			"item":                "version_control_snapshot",
			"kind":                "deliverable",
			"source":              "deliverableCoverage",
			"state":               "failed",
			"targetText":          "version_control_snapshot",
			"coverageTargetToken": "version_control_snapshot",
		}, {
			"kind":                "goal",
			"source":              "goal",
			"sourceScope":         "goal_leaf",
			"sourceRequirementId": "goal-ready",
			"state":               "evidence_ready",
			"targetText":          "already ready goal",
			"coverageTargetToken": "goal-ready",
		}},
		"repairActionPackage": map[string]any{
			"repairObjective": repairObjectiveStateChangingOutput,
			"coverageTargetSet": []map[string]any{{
				"item":                "version_control_snapshot",
				"kind":                "deliverable",
				"source":              "deliverableCoverage",
				"state":               "failed",
				"targetText":          "version_control_snapshot",
				"coverageTargetToken": "version_control_snapshot",
			}, {
				"kind":                "goal",
				"source":              "goal",
				"sourceScope":         "goal_leaf",
				"sourceRequirementId": "goal-ready",
				"state":               "evidence_ready",
				"targetText":          "already ready goal",
				"coverageTargetToken": "goal-ready",
			}},
		},
	}
	failed := &runmodel.Step{
		ID:     "run-profile-evidence-kind-gap-with-ready-noise/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid argument: dodAssessment is not satisfied"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want profile-required state-changing evidence repair despite ready coverage noise", contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) {
		t.Fatalf("requiredOutput = %+v, want profile-required accepted kind", required)
	}
}

func TestBuildConvergenceContractUsesInputProfileEvidenceContractWhenRunContextMissing(t *testing.T) {
	run := &runmodel.Run{ID: "run-profile-evidence-input-contract"}
	input := map[string]any{
		stepInputKeyAssessmentOnly: true,
		"repairObjective":          "complete_dod_goal_coverage_assessment",
		"executionContextPackage": map[string]any{
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
		"missingDeliverableCoverage": []map[string]any{{
			"item":         "version_control_snapshot",
			"kind":         "deliverable",
			"source":       "deliverableCoverage",
			"state":        "failed",
			"evidenceRefs": []string{"run/graph/001-read"},
		}},
	}
	failed := &runmodel.Step{
		ID:     "run-profile-evidence-input-contract/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid argument: dodAssessment is not satisfied"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want input profile contract to drive state-changing evidence repair", contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("requiredOutput = %+v, want input profile accepted kind/operation", required)
	}
}

func TestBuildConvergenceContractMatchesAnnotatedCoverageTargetScope(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-evidence-annotated-target",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                    "version_control_snapshot",
							"scope":                 "deliverable",
							"acceptedStepKinds":     []string{gitcommon.StepKindWrite},
							"acceptedOperations":    []string{gitcommon.OperationCommit},
							"requireDirectEvidence": true,
						}},
					},
				},
			},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageRepair,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"convergenceContract": map[string]any{
			"targetEvidence": map[string]any{
				"coverageTargetSet": []map[string]any{{
					"kind":                "dod",
					"item":                "version_control_snapshot（deliverableCoverage）",
					"coverageTargetToken": "version_control_snapshot（deliverableCoverage）",
					"targetText":          "version_control_snapshot（deliverableCoverage）",
					"state":               "failed",
					"source":              "dodAssessment",
				}},
				"repairTargetSet": []string{"usage.csv"},
				"targetSnapshots": []map[string]any{{
					"targetRef":        "usage.csv",
					"exists":           true,
					"contentHash":      "hash-usage",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-profile-evidence-annotated-target/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid argument: dodAssessment is not satisfied"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("requiredOutput = %+v, want profile-required git/write commit", required)
	}
	requirements := mapSliceFromValue(contract.RepairActionPackage["evidenceKindRequirements"])
	if len(requirements) != 1 || requirements[0]["id"] != "version_control_snapshot" {
		t.Fatalf("evidenceKindRequirements = %+v, want annotated target matched to profile requirement", requirements)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	appliedRequired := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
	if !stringSliceContains(stringSliceFromValue(appliedRequired["acceptedKinds"]), gitcommon.StepKindWrite) {
		t.Fatalf("applied repairActionPackage.requiredOutput = %+v, want git/write preserved", appliedRequired)
	}
}

func TestBuildConvergenceContractDoesNotCrossMatchAnnotatedCoverageTargetScope(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-evidence-annotated-target-scope-mismatch",
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
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"coverageTargetSet": []map[string]any{{
			"kind":                "dod",
			"item":                "version_control_snapshot（goalCoverage）",
			"coverageTargetToken": "version_control_snapshot（goalCoverage）",
			"targetText":          "version_control_snapshot（goalCoverage）",
			"state":               "failed",
			"source":              "dodAssessment",
		}},
		"repairActionPackage": map[string]any{
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-profile-evidence-annotated-target-scope-mismatch/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid argument: dodAssessment is not satisfied"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State == convergenceStateNeedStateChange {
		t.Fatalf("contract = %+v, did not expect deliverable evidence requirement to match goalCoverage annotation", contract)
	}
}

func TestBuildConvergenceContractDropsStaleProfileStateChangeWhenCurrentGoalCoverageNeedsValidation(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-stale-deliverable-target-current-goal-gap",
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
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"evidenceRepairRequired":     true,
		"missingGoalCoverage": []map[string]any{{
			"goalRequirementId": "goal-req-usage",
			"raw":               "usage must be validated",
			"state":             "failed",
		}},
		"currentExecutionProgress": map[string]any{
			"latestActionEvidence": []map[string]any{{
				"kind":   gitcommon.StepKindWrite,
				"stepId": "run-convergence-stale-deliverable-target-current-goal-gap/graph-001/003-commit",
				"outputSummary": map[string]any{
					"stage":   "git_write",
					"summary": "git commit completed",
					"stdout":  "[master abc123] commit\n 1 file changed, 1 insertion(+)",
				},
			}},
		},
		"repairActionPackage": map[string]any{
			"coverageRepairRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":                "dod",
				"item":                "version_control_snapshot（deliverableCoverage）",
				"coverageTargetToken": "version_control_snapshot（deliverableCoverage）",
				"targetText":          "version_control_snapshot（deliverableCoverage）",
				"state":               "failed",
			}},
			"evidenceKindRequirements": []map[string]any{{
				"scope":              "deliverable",
				"id":                 "version_control_snapshot",
				"acceptedStepKinds":  []string{gitcommon.StepKindWrite},
				"acceptedOperations": []string{gitcommon.OperationCommit},
			}},
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []string{gitcommon.StepKindWrite},
				"acceptedOperations":           []string{gitcommon.OperationCommit},
			},
		},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-stale-deliverable-target-current-goal-gap/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid argument: concrete validation evidence repair must return bounded shell/command or mcp/tool_call executionGraph"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedValidation {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedValidation, contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) || stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) {
		t.Fatalf("requiredOutput = %+v, want concrete validation contract without stale git/write requirement", required)
	}
	targets := mapSliceFromValue(contract.RepairActionPackage["coverageTargetSet"])
	if len(targets) != 1 || targets[0]["goalRequirementId"] != "goal-req-usage" {
		t.Fatalf("coverageTargetSet = %+v, want current goal gap only", targets)
	}
}

func TestBuildConvergenceContractDefersPrimaryProfileStateChangeWithoutTargetEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-primary-state-change-without-target",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []any{map[string]any{
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
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"coverageRepairRequired":     true,
		"coverageTargetSet": []map[string]any{
			{
				"kind":                "deliverable",
				"item":                "version_control_snapshot",
				"coverageTargetToken": "version_control_snapshot",
				"targetText":          "version_control_snapshot",
				"state":               "failed",
			},
			{
				"kind":                "dod",
				"item":                "behavior must be verified",
				"coverageTargetToken": "behavior must be verified",
				"targetText":          "behavior must be verified",
				"state":               "failed",
			},
			{
				"kind":                "goal",
				"item":                "goal still lacks accepted evidence",
				"coverageTargetToken": "goal still lacks accepted evidence",
				"targetText":          "goal still lacks accepted evidence",
				"state":               "failed",
			},
		},
		"repairActionPackage": map[string]any{
			"coverageRepairRequired": true,
			"coverageTargetSet": []map[string]any{
				{
					"kind":                "deliverable",
					"item":                "version_control_snapshot",
					"coverageTargetToken": "version_control_snapshot",
					"targetText":          "version_control_snapshot",
					"state":               "failed",
				},
				{
					"kind":                "dod",
					"item":                "behavior must be verified",
					"coverageTargetToken": "behavior must be verified",
					"targetText":          "behavior must be verified",
					"state":               "failed",
				},
				{
					"kind":                "goal",
					"item":                "goal still lacks accepted evidence",
					"coverageTargetToken": "goal still lacks accepted evidence",
					"targetText":          "goal still lacks accepted evidence",
					"state":               "failed",
				},
			},
			"currentExecutionProgress": map[string]any{
				"latestActionEvidence": []map[string]any{{
					"kind":  "mcp/tool_call",
					"state": "completed",
				}},
			},
		},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-profile-primary-state-change-without-target/assessment",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{failureOutputKeyClass: failureClassDoDCoverageMissing, "stage": "llm_dod_assessment", "error": "invalid argument: dodAssessment is not satisfied"},
		State:  common.StepStateFailed,
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedEvidence, contract)
	}
	if !contract.ConcreteFailureEvidenceRequired || contract.RepairObjective != repairObjectiveAcquireFailureEvidence {
		t.Fatalf("contract = %+v, want bounded evidence acquisition", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) ||
		boolValue(required["mustIncludePrimaryStateChangingNode"]) ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want no state-changing artifact before concrete target evidence", required)
	}
	if !boolValue(required["profileEvidenceRequirementDeferred"]) {
		t.Fatalf("requiredOutput = %+v, want profile evidence requirement deferred marker", required)
	}
}

func TestBuildConvergenceContractRestoresProfileRequiredStateChangingEvidenceWhenEvidenceAcquisitionWasStale(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-evidence-stale-need-evidence",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                    "version_control_snapshot",
							"scope":                 "deliverable",
							"acceptedStepKinds":     []string{gitcommon.StepKindWrite},
							"acceptedOperations":    []string{gitcommon.OperationCommit},
							"requireDirectEvidence": true,
						}},
					},
				},
			},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		"coverageRepairRequired":          true,
		"coverageTargetSet": []map[string]any{{
			"kind":                "deliverable",
			"item":                "version_control_snapshot",
			"coverageTargetToken": "version_control_snapshot",
			"targetText":          "version_control_snapshot",
			"state":               "missing",
		}},
		"repairActionPackage": map[string]any{
			"coverageRepairRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":                "deliverable",
				"item":                "version_control_snapshot",
				"coverageTargetToken": "version_control_snapshot",
				"targetText":          "version_control_snapshot",
				"state":               "missing",
			}},
			"requiredOutput": evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
		},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-profile-evidence-stale-need-evidence/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid llm decision json: no json object found"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) {
		t.Fatalf("requiredOutput = %+v, want git/write commit state-changing evidence", required)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if boolValue(applied["concreteFailureEvidenceRequired"]) || stringFromMap(applied, "repairObjective") != repairObjectiveStateChangingOutput {
		t.Fatalf("applied input = %+v, want stale evidence-acquisition flags replaced by state-changing repair", applied)
	}
}

func TestBuildConvergenceContractAllowsPrimaryStateChangeFromCoverageTargetsWithoutSnapshot(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-primary-state-change-greenfield",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                    "source_change",
							"scope":                 "deliverable",
							"evidenceRole":          "primary_state_change",
							"acceptedStepKinds":     []string{"patch/apply"},
							"requireDirectEvidence": true,
						}},
					},
				},
			},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		"coverageRepairRequired":          true,
		coverageFocusKey: map[string]any{
			"id":                  "focus-cwi-source-change",
			"coverageWorkItemIds": []string{"cwi-source-change"},
			"evidenceRole":        "primary_state_change",
			"targetRefs":          []string{"source_change"},
			"state":               "active",
		},
		"coverageTargetSet": []map[string]any{{
			"kind":                "deliverable",
			"item":                "source_change",
			"coverageTargetToken": "source_change",
			"targetText":          "source_change",
			"evidenceRole":        "primary_state_change",
			"state":               "failed",
		}},
		"repairActionPackage": map[string]any{
			"coverageRepairRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":                "deliverable",
				"item":                "source_change",
				"coverageTargetToken": "source_change",
				"targetText":          "source_change",
				"evidenceRole":        "primary_state_change",
				"state":               "failed",
			}},
			"requiredOutput": evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
		},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-primary-state-change-greenfield/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid llm decision json: no json object found"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	pkg := mapFromValue(contract.RepairActionPackage)
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		boolValue(required["stateChangingForbidden"]) {
		t.Fatalf("requiredOutput = %+v, want patch/apply state-changing output allowed", required)
	}
	if !boolValue(pkg["primaryStateChangeWithoutTargetEvidence"]) ||
		!boolValue(pkg["primaryStateChangeCoverageScoped"]) {
		t.Fatalf("repairActionPackage = %+v, want coverage-scoped primary state change without target evidence", pkg)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if boolValue(applied["concreteFailureEvidenceRequired"]) || stringFromMap(applied, "repairObjective") != repairObjectiveStateChangingOutput {
		t.Fatalf("applied input = %+v, want state-changing repair instead of evidence acquisition", applied)
	}
}

func TestBuildConvergenceContractDerivesProfileEvidenceTargetFromCoverageWorkItemContext(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-evidence-cwi-target",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                 "profile_snapshot",
							"scope":              "deliverable",
							"acceptedStepKinds":  []string{gitcommon.StepKindWrite},
							"acceptedOperations": []string{gitcommon.OperationCommit},
						}},
					},
				},
			},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-profile-snapshot",
				"source":                "deliverable",
				"sourceScope":           "deliverable",
				"sourceRequirementId":   "profile_snapshot",
				"sourcePath":            "executionContract.profileContracts.neutral-profile.deliverableExpectations[0]",
				"targetText":            "profile_snapshot",
				"status":                "open",
				"evidenceRole":          "supplemental_evidence",
				"requiredEvidenceKinds": []string{"validation", "state_change"},
				"missingEvidenceKinds":  []string{"validation", "state_change"},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
		},
	}
	failed := &runmodel.Step{
		ID:     "run-convergence-profile-evidence-cwi-target/repair-001",
		Kind:   llmWorkspaceExecutionStepKind,
		Input:  cloneStepPayload(input),
		Output: map[string]any{"error": "invalid llm decision json: no json object found"},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange || contract.RepairObjective != repairObjectiveStateChangingOutput {
		t.Fatalf("convergenceContract = %+v, want profile state-changing evidence from CWI target", contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), gitcommon.StepKindWrite) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedOperations"]), gitcommon.OperationCommit) ||
		boolValue(required["profileEvidenceRequirementDeferred"]) {
		t.Fatalf("requiredOutput = %+v, want direct profile-required git/write commit contract", required)
	}
	targets := mapSliceFromValue(contract.RepairActionPackage["coverageTargetSet"])
	if len(targets) != 1 || stringFromMap(targets[0], "sourceRequirementId") != "profile_snapshot" {
		t.Fatalf("coverageTargetSet = %+v, want target derived from coverageWorkItemContext", targets)
	}
}

func TestApplyConvergenceContractClearsStaleValidationFlagsForProfileStateChange(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired":  true,
		"evidenceRepairRequired":     true,
		stepInputKeyAssessmentOnly:   true,
		"assessmentOnlyReason":       "previous assessment correction",
		"repairGuidance": []string{
			"Return only completed or failed dodAssessment, or a real governance blocker.",
			"Do not return executionGraph, shellCommands, patchRequests, mcpToolCalls, memoryWrites, or other executable work.",
			"Preserve existing coverage.",
		},
	}
	contract := ConvergenceContract{
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
	}

	applied := ApplyConvergenceContract(input, contract)

	if stringFromMap(applied, "repairObjective") != repairObjectiveStateChangingOutput || !boolValue(applied["stateChangingOutputRequired"]) {
		t.Fatalf("applied = %+v, want state-changing contract", applied)
	}
	for _, key := range []string{"boundedValidationRequired", "evidenceRepairRequired", stepInputKeyAssessmentOnly, "assessmentOnlyReason"} {
		if _, ok := applied[key]; ok {
			t.Fatalf("applied[%s] still present in state-changing contract: %+v", key, applied)
		}
	}
	guidance := strings.Join(stringListFromValue(applied["repairGuidance"]), "\n")
	for _, forbidden := range []string{"Return only completed or failed dodAssessment", "Do not return executionGraph"} {
		if strings.Contains(guidance, forbidden) {
			t.Fatalf("repairGuidance contains stale assessment-only instruction %q: %s", forbidden, guidance)
		}
	}
	if !strings.Contains(guidance, "dodAssessment alone cannot satisfy") {
		t.Fatalf("repairGuidance = %s, want explicit executable state-changing guidance", guidance)
	}
}

func TestApplyConvergenceContractNeedStateChangeRestoresRequiredOutput(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage: runtimeStageRepair,
		"repairActionPackage": map[string]any{
			"currentFailure": map[string]any{"reason": "state-changing repair still needed"},
		},
	}
	contract := ConvergenceContract{
		State:                       convergenceStateNeedStateChange,
		RuntimeStage:                runtimeStageRepair,
		RuntimeStageMode:            "strict_convergence",
		RepairObjective:             repairObjectiveStateChangingOutput,
		StateChangingOutputRequired: true,
		RepairActionPackage: map[string]any{
			"currentFailure": map[string]any{"reason": "state-changing repair still needed"},
		},
	}

	applied := ApplyConvergenceContract(input, contract)

	pkg := mapFromValue(applied["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("requiredOutput = %+v, want state-changing output contract", required)
	}
	if !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput.acceptedKinds = %+v, want state-changing patch/apply kind", required["acceptedKinds"])
	}
	if stringFromMap(applied, "repairObjective") != repairObjectiveStateChangingOutput || !boolValue(applied["stateChangingOutputRequired"]) {
		t.Fatalf("applied = %+v, want state-changing repair objective", applied)
	}
}

func TestApplyConvergenceContractNeedStateChangeClearsStaleEvidenceAcquisitionPackage(t *testing.T) {
	input := map[string]any{
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
		},
		"repairActionPackage": map[string]any{
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"evidenceAcquisitionRequired":     true,
			"concreteFailureEvidenceRequired": true,
			"evidenceAcquisitionReason":       "old diagnostic lineage",
			"requiredOutput":                  evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
		},
	}
	contract := ConvergenceContract{
		State:                       convergenceStateNeedStateChange,
		RuntimeStage:                runtimeStageRepair,
		RuntimeStageMode:            "strict_convergence",
		RepairObjective:             repairObjectiveStateChangingOutput,
		StateChangingOutputRequired: true,
		RepairActionPackage: map[string]any{
			"repairObjective":             repairObjectiveAcquireFailureEvidence,
			"evidenceAcquisitionRequired": true,
			"requiredOutput":              evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
		},
	}

	applied := ApplyConvergenceContract(input, contract)

	pkg := mapFromValue(applied["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	stageRequired := mapFromValue(mapFromValue(applied[stepInputKeyStagePacket])["requiredOutput"])
	if stringFromMap(pkg, "repairObjective") != repairObjectiveStateChangingOutput ||
		boolValue(pkg["evidenceAcquisitionRequired"]) ||
		boolValue(pkg["concreteFailureEvidenceRequired"]) {
		t.Fatalf("repairActionPackage = %+v, want current state-changing package without stale evidence acquisition fields", pkg)
	}
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		boolValue(required["mustProduceConcreteFailureEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want state-changing output only", required)
	}
	if !boolValue(mapFromValue(applied["requiredOutput"])["mustIncludeStateChangingNode"]) ||
		!boolValue(stageRequired["mustIncludeStateChangingNode"]) {
		t.Fatalf("requiredOutput top=%+v stage=%+v, want synchronized state-changing required output", applied["requiredOutput"], stageRequired)
	}
}

func TestBuildConvergenceContractNeedStateChangeRestoresRequiredOutput(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		stepInputKeyRuntimeStageMode:  "strict_convergence",
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"targetSnapshots": []map[string]any{{
				"targetRef":        "target.txt",
				"exists":           true,
				"contentHash":      "hash-target",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) || !boolValue(required["mustTargetFailureScope"]) {
		t.Fatalf("requiredOutput = %+v, want state-changing target-scoped output contract", required)
	}
}

func TestBuildConvergenceContractKeepsConvertedReadableDecisionAssessmentOnlyWithoutCoverageGap(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-readable-decision-assessment-only",
		DoDID:           "dod-readable-decision-assessment-only",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID: "run-readable-decision-assessment-only/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"readableResourceRepairContext": []map[string]any{{
				"targetRef": "current.txt",
				"content":   "current content",
			}},
		},
		Output: map[string]any{
			"continuableDecisionConvertedToRepair": true,
			"convertedDecisionGate":                "convergence_contract",
			"convertedDecision": map[string]any{
				"question": "Please provide current.txt.",
			},
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": step.Input}, nil)

	if contract.State == convergenceStateNeedStateChange {
		t.Fatalf("contract = %+v, want readable resource repair without coverage gap not forced into state change", contract)
	}
}

func TestBuildConvergenceContractKeepsGoalCoverageGapAsValidationWhenDirectEvidenceCanBeCommand(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-goal-coverage-validation",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                    "*",
							"scope":                 "goalCoverage",
							"acceptedStepKinds":     []string{"shell/command", "patch/apply", "mcp/tool_call"},
							"requireDirectEvidence": true,
						}},
					},
				},
			},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"coverageTargetSet": []map[string]any{{
			"kind":                "goal",
			"goalRequirementId":   "goal-req-usage",
			"coverageTargetToken": "goal-req-usage",
			"targetText":          "usage field must be parsed",
			"state":               "failed",
		}},
		"repairActionPackage": map[string]any{
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence-goal-coverage-validation/follow-up",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: input,
		Output: map[string]any{
			"stage": "llm_dod_assessment",
			"error": "invalid argument: dodAssessment does not cover all current Goal requirements",
		},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedValidation {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedValidation, contract)
	}
	if contract.StateChangingOutputRequired {
		t.Fatalf("StateChangingOutputRequired = true, want false")
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if !boolValue(required["mustProduceConcreteValidationEvidence"]) ||
		boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "shell/command") ||
		stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "shell/command") {
		t.Fatalf("requiredOutput = %+v, want concrete validation evidence contract", required)
	}
}

func TestBuildConvergenceContractKeepsAssessmentOnlyOverRejectedContinuation(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-assessment-only",
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
	input := map[string]any{
		stepInputKeyAssessmentOnly: true,
		"repairObjective":          "complete_dod_goal_coverage_assessment",
		"missingGoalCoverage": []map[string]any{{
			"goalRequirementId": "goal-req-001",
			"raw":               "current goal leaf",
			"evidenceCandidates": []map[string]any{{
				"stepId":          "run-convergence-assessment-only/graph-001/001-verify",
				"kind":            "shell/command",
				"state":           "completed",
				"evidenceQuality": "direct",
			}},
		}},
		"rejectedResultContinuation": map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
			},
			"coverageGaps": map[string]any{
				"missingDeliverableCoverage": []map[string]any{{
					"kind":  "deliverable",
					"item":  "version_control_snapshot",
					"state": "failed",
				}},
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence-assessment-only/follow-up",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: input,
		Output: map[string]any{
			"stage":               "llm_dod_assessment",
			failureOutputKeyClass: failureClassDoDCoverageMissing,
			"error":               "invalid argument: dodAssessment does not cover all current Goal requirements; missing: current goal leaf",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "coverage correction needed",
				"goalCoverage": map[string]any{
					"satisfied": true,
					"items": []map[string]any{{
						"goalRequirementId": "goal-req-999",
						"item":              "older goal leaf",
						"state":             "satisfied",
						"evidenceRefs":      []string{"run-convergence-assessment-only/graph-001/001-verify"},
					}},
				},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingGoalCoverage": []map[string]any{{
					"goalRequirementId": "goal-req-001",
					"raw":               "current goal leaf",
				}},
			},
		},
	}

	contract := BuildConvergenceContract(run, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State != convergenceStateNeedAssessment {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedAssessment, contract)
	}

	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if boolValue(applied["stateChangingOutputRequired"]) {
		t.Fatalf("stateChangingOutputRequired = true, want assessment-only contract: %+v", applied)
	}
	if pkg := mapFromValue(applied["repairActionPackage"]); len(pkg) > 0 {
		required := mapFromValue(pkg["requiredOutput"])
		if boolValue(required["mustIncludeStateChangingNode"]) {
			t.Fatalf("repairActionPackage.requiredOutput = %+v, want no state-changing requirement", required)
		}
	}
}

func TestApplyConvergenceContractNeedEvidenceRemovesStateChangingRequirement(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"outputMode":                  outputModeIncrementalArtifact,
		"outputModeSelector": map[string]any{
			"mode": outputModeIncrementalArtifact,
		},
		"artifactOutputContract": map[string]any{
			"mode":  outputModeIncrementalArtifact,
			"phase": artifactOutputPhasePlan,
		},
		"artifactContinuationContext": map[string]any{
			"id":         "artifact-continuation-001",
			"artifactId": "artifact-001",
		},
		"artifactContinuationId": "artifact-continuation-001",
		"repairActionPackage": map[string]any{
			"requiredOutput": evidenceAcquisitionRequiredOutput(map[string]any{
				"mustReferenceCoverageWorkItem": true,
				"mustDeclareCoverageTargets":    true,
				"coverageTargetMatchRule":       "validation evidence must map to coverageTargetSet",
			}),
			"evidenceAcquisitionRequired": true,
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": input}, nil)
	output := ApplyConvergenceContract(input, contract)

	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedEvidence)
	}
	if boolValue(output["stateChangingOutputRequired"]) {
		t.Fatalf("stateChangingOutputRequired = true, want removed for evidence acquisition")
	}
	if !boolValue(output["concreteFailureEvidenceRequired"]) {
		t.Fatalf("concreteFailureEvidenceRequired = false, want true")
	}
	for _, key := range []string{"outputMode", "outputModeSelector", "artifactOutputContract", "artifactContinuationContext", "artifactContinuationId"} {
		if _, ok := output[key]; ok {
			t.Fatalf("output[%s] still present in need_evidence contract: %+v", key, output[key])
		}
	}
	required := mapFromValue(mapFromValue(output["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustProduceConcreteFailureEvidence"]) {
		t.Fatalf("requiredOutput = %#v, want concrete evidence acquisition", required)
	}
	if !boolValue(required["mustReferenceCoverageWorkItem"]) ||
		!boolValue(required["mustDeclareCoverageTargets"]) ||
		stringFromMap(required, "coverageTargetMatchRule") == "" {
		t.Fatalf("requiredOutput = %#v, want coverage focus contract preserved", required)
	}
}

func TestApplyConvergenceContractNeedValidationClearsStaleStateChangeAndEvidencePackage(t *testing.T) {
	input := map[string]any{
		stepInputKeyStagePacket: map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
		},
		"repairActionPackage": map[string]any{
			"repairObjective":                 repairObjectiveAcquireFailureEvidence,
			"stateChangingOutputRequired":     true,
			"evidenceAcquisitionRequired":     true,
			"concreteFailureEvidenceRequired": true,
			"requiredOutput":                  stateChangingRepairRequiredOutput(),
		},
	}
	contract := ConvergenceContract{
		State:            convergenceStateNeedValidation,
		RuntimeStage:     runtimeStageVerification,
		RuntimeStageMode: "strict_convergence",
		RepairObjective:  repairObjectiveConcreteValidationEvidence,
		RepairActionPackage: map[string]any{
			"repairObjective":             repairObjectiveAcquireFailureEvidence,
			"stateChangingOutputRequired": true,
			"evidenceAcquisitionRequired": true,
			"requiredOutput":              stateChangingRepairRequiredOutput(),
		},
	}

	applied := ApplyConvergenceContract(input, contract)

	pkg := mapFromValue(applied["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	stageRequired := mapFromValue(mapFromValue(applied[stepInputKeyStagePacket])["requiredOutput"])
	if stringFromMap(pkg, "repairObjective") != repairObjectiveConcreteValidationEvidence ||
		boolValue(pkg["stateChangingOutputRequired"]) ||
		boolValue(pkg["evidenceAcquisitionRequired"]) ||
		boolValue(pkg["concreteFailureEvidenceRequired"]) {
		t.Fatalf("repairActionPackage = %+v, want current validation package without stale state-change/evidence fields", pkg)
	}
	if !boolValue(required["mustProduceConcreteValidationEvidence"]) ||
		boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("requiredOutput = %+v, want concrete validation output only", required)
	}
	if !boolValue(mapFromValue(applied["requiredOutput"])["mustProduceConcreteValidationEvidence"]) ||
		!boolValue(stageRequired["mustProduceConcreteValidationEvidence"]) {
		t.Fatalf("requiredOutput top=%+v stage=%+v, want synchronized validation required output", applied["requiredOutput"], stageRequired)
	}
}

func TestApplyConvergenceContractNeedAssessmentStripsExecutableContract(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"boundedValidationRequired":   true,
		"evidenceRepairRequired":      true,
		"repairGuidance":              []string{"produce patch/apply before verification"},
		"outputMode":                  outputModeIncrementalArtifact,
		"outputModeSelector": map[string]any{
			"mode": outputModeIncrementalArtifact,
		},
		"artifactOutputContract": map[string]any{
			"mode":  outputModeIncrementalArtifact,
			"phase": artifactOutputPhasePlan,
		},
		"artifactContinuationContext": map[string]any{
			"id":         "artifact-continuation-001",
			"artifactId": "artifact-001",
		},
		"artifactContinuationId": "artifact-continuation-001",
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustReturnArtifactPlan": true,
				"outputMode":             outputModeIncrementalArtifact,
			},
			"coverageTargetSet": []map[string]any{{
				"targetText": "coverage-gap",
			}},
			"currentFailure": map[string]any{
				"reason": "coverage gap remains",
			},
		},
	}
	contract := ConvergenceContract{
		State:            convergenceStateNeedAssessment,
		RuntimeStage:     runtimeStageVerification,
		RuntimeStageMode: "strict_convergence",
	}

	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)

	if applied[stepInputKeyRuntimeStage] != runtimeStageVerification || applied[stepInputKeyAssessmentOnly] != true {
		t.Fatalf("applied = %+v, want verification assessment-only contract", applied)
	}
	if applied["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("repairObjective = %+v, want assessment-only coverage correction objective", applied["repairObjective"])
	}
	for _, key := range []string{
		"stateChangingOutputRequired",
		"boundedValidationRequired",
		"evidenceRepairRequired",
		"outputMode",
		"outputModeSelector",
		"artifactOutputContract",
		"artifactContinuationContext",
		"artifactContinuationId",
	} {
		if _, ok := applied[key]; ok {
			t.Fatalf("applied[%s] still present in need_assessment contract: %+v", key, applied[key])
		}
	}
	pkg := mapFromValue(applied["repairActionPackage"])
	if len(pkg) == 0 {
		t.Fatalf("repairActionPackage removed entirely, want contextual coverage/currentFailure retained")
	}
	if pkg["requiredOutput"] != nil {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want stripped for assessment-only contract", pkg["requiredOutput"])
	}
	if len(mapSliceFromValue(pkg["coverageTargetSet"])) == 0 || len(mapFromValue(pkg["currentFailure"])) == 0 {
		t.Fatalf("repairActionPackage = %+v, want contextual fields retained", pkg)
	}
}

func TestBuildConvergenceContractAssessmentOnlyFailedGapSwitchesToValidation(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		stepInputKeyAssessmentOnly:   true,
		"repairObjective":            "complete_dod_goal_coverage_assessment",
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-001",
				"sourceScope":           "dod_item",
				"sourceRequirementId":   "dod-item-001",
				"targetText":            "remaining validation claim",
				"status":                "failed",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"rejectedEvidenceRefs":  []string{"run/graph/001-verify"},
			},
			"currentFocusItems": []map[string]any{{
				"id":                    "cwi-001",
				"sourceScope":           "dod_item",
				"sourceRequirementId":   "dod-item-001",
				"targetText":            "remaining validation claim",
				"status":                "failed",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"rejectedEvidenceRefs":  []string{"run/graph/001-verify"},
			}},
			"summary": map[string]any{"total": 1, "failed": 1},
		},
		"repairActionPackage": map[string]any{
			"currentFailure": map[string]any{"failureClass": failureClassDoDCoverageMissing},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence/repair-001",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: input,
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"items": []map[string]any{{
					"item":         "remaining validation claim",
					"state":        "failed",
					"evidenceRefs": []string{"run/graph/001-verify"},
				}},
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedValidation {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedValidation, contract)
	}
	if contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repairObjective = %q, want %q", contract.RepairObjective, repairObjectiveConcreteValidationEvidence)
	}
	targets := mapSliceFromValue(contract.RepairActionPackage["coverageTargetSet"])
	if len(targets) != 1 || stringFromMap(targets[0], "targetText") != "remaining validation claim" {
		t.Fatalf("coverageTargetSet = %+v, want failed work item target", targets)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied[stepInputKeyAssessmentOnly] != nil ||
		applied[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		applied["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("applied = %+v, want concrete validation repair without assessment-only", applied)
	}
	required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustProduceConcreteValidationEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want concrete validation evidence contract", required)
	}
	if !boolValue(required["mustDeclareCoverageTargets"]) ||
		stringFromMap(required, "coverageTargetMatchRule") == "" {
		t.Fatalf("requiredOutput = %+v, want coverage target contract preserved", required)
	}
	if stringFromMap(required, validationScratchDirKey) == "" ||
		stringFromMap(mapFromValue(applied["repairActionPackage"]), validationScratchDirKey) != stringFromMap(required, validationScratchDirKey) ||
		stringFromMap(applied, validationScratchDirKey) != stringFromMap(required, validationScratchDirKey) {
		t.Fatalf("scratch contract not mirrored applied=%q pkg=%q required=%q", stringFromMap(applied, validationScratchDirKey), stringFromMap(mapFromValue(applied["repairActionPackage"]), validationScratchDirKey), stringFromMap(required, validationScratchDirKey))
	}
}

func TestBuildConvergenceContractCoverageFocusWithAcceptedEvidenceNeedsAssessment(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired":  true,
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                            "cwi-ready",
				"sourceScope":                   "dod_item",
				"sourceRequirementId":           "dod-item-ready",
				"targetText":                    "neutral claim coverage",
				"status":                        "failed",
				"requiredEvidenceKinds":         []string{"validation"},
				"missingEvidenceKinds":          []string{},
				"acceptedEvidenceRefs":          []string{"run/graph/001-validate"},
				"acceptedEvidenceByKind":        map[string][]string{"validation": {"run/graph/001-validate"}},
				"acceptedEvidenceBindingByKind": map[string][]string{"validation": {"run/graph/001-validate"}},
				"nextAllowedConvergenceStates":  []string{convergenceStateNeedValidation},
				"currentGap":                    "required evidence kinds present; coverage assessment required",
			},
			"currentFocusItems": []map[string]any{{
				"id":                            "cwi-ready",
				"sourceScope":                   "dod_item",
				"sourceRequirementId":           "dod-item-ready",
				"targetText":                    "neutral claim coverage",
				"status":                        "failed",
				"requiredEvidenceKinds":         []string{"validation"},
				"missingEvidenceKinds":          []string{},
				"acceptedEvidenceRefs":          []string{"run/graph/001-validate"},
				"acceptedEvidenceByKind":        map[string][]string{"validation": {"run/graph/001-validate"}},
				"acceptedEvidenceBindingByKind": map[string][]string{"validation": {"run/graph/001-validate"}},
				"nextAllowedConvergenceStates":  []string{convergenceStateNeedValidation},
			}},
			"summary": map[string]any{"total": 1, "failed": 1},
		},
		"repairActionPackage": map[string]any{
			"coverageRepairRequired": true,
			"currentFailure":         map[string]any{"failureClass": failureClassDoDCoverageMissing},
			"requiredOutput":         concreteValidationEvidenceRequiredOutput(nil),
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State != convergenceStateNeedAssessment {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedAssessment, contract)
	}
	if contract.RepairObjective != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("repairObjective = %q, want assessment objective", contract.RepairObjective)
	}

	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		applied[stepInputKeyAssessmentOnly] != true ||
		applied["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("applied = %+v, want verification assessment-only contract", applied)
	}
	for _, key := range []string{"boundedValidationRequired", "stateChangingOutputRequired", "evidenceRepairRequired"} {
		if _, ok := applied[key]; ok {
			t.Fatalf("applied[%s] still present in assessment-ready contract: %+v", key, applied[key])
		}
	}
	required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
	if len(required) > 0 {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want executable validation contract stripped", required)
	}
}

func TestBuildConvergenceContractNextActionAssessmentPreemptsStaleEvidenceAcquisition(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		gapBaselineKey: map[string]any{
			"schemaVersion":    gapBaselineSchema,
			"baselineRevision": "sha256:assessment-ready",
			"nextAction":       map[string]any{"kind": "assessment", "reason": "coverage work item evidence is ready for assessment"},
		},
		coverageWorkItemContextKey: map[string]any{
			"definitionRevision":   "rev-assessment-ready",
			"coverageBaselineHash": "sha256:assessment-ready",
			"currentFocus": map[string]any{
				"id":                    "cwi-ready",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceRefs": []string{"run/graph/001-validate"},
			},
			"currentFocusItems": []map[string]any{{
				"id":                    "cwi-ready",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceRefs": []string{"run/graph/001-validate"},
			}},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustAcquireConcreteFailureEvidence": true,
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State != convergenceStateNeedAssessment {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedAssessment, contract)
	}
	if contract.ConcreteFailureEvidenceRequired || contract.StateChangingOutputRequired || contract.RepairObjective != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("contract = %+v, want assessment to preempt stale evidence acquisition", contract)
	}

	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		applied[stepInputKeyAssessmentOnly] != true ||
		applied["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("applied = %+v, want assessment-only contract", applied)
	}
	for _, key := range []string{"concreteFailureEvidenceRequired", "stateChangingOutputRequired", "boundedValidationRequired", "evidenceRepairRequired"} {
		if _, ok := applied[key]; ok {
			t.Fatalf("applied[%s] still present in assessment next-action contract: %+v", key, applied[key])
		}
	}
	if required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"]); len(required) > 0 {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want stale executable output stripped", required)
	}
}

func TestBuildConvergenceContractSkipsAssessmentForDeterministicAcceptedBinding(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		gapBaselineKey: map[string]any{
			"schemaVersion":    gapBaselineSchema,
			"baselineRevision": "sha256:deterministic-ready",
			"nextAction":       map[string]any{"kind": "assessment", "reason": "stale assessment authority"},
		},
		coverageWorkItemContextKey: map[string]any{
			"definitionRevision":   "rev-deterministic-ready",
			"coverageBaselineHash": "sha256:deterministic-ready",
			"currentFocus": map[string]any{
				"id":                    "cwi-deterministic-ready",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceBindingRefs": []string{"run/graph/001-validate"},
				"acceptedEvidenceRefs":        []string{"run/graph/001-validate"},
			},
			"currentFocusItems": []map[string]any{{
				"id":                    "cwi-deterministic-ready",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceBindingRefs": []string{"run/graph/001-validate"},
				"acceptedEvidenceRefs":        []string{"run/graph/001-validate"},
			}},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustAcquireConcreteFailureEvidence": true,
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State == convergenceStateNeedAssessment {
		t.Fatalf("contract state = %q, want deterministic ledger close to avoid LLM assessment; contract=%+v", contract.State, contract)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied[stepInputKeyAssessmentOnly] != nil || applied["repairObjective"] == "complete_dod_goal_coverage_assessment" {
		t.Fatalf("applied = %+v, want no assessment-only contract for deterministic accepted binding", applied)
	}
}

func TestBuildConvergenceContractValidationFocusPreemptsStaleEvidenceAcquisition(t *testing.T) {
	focus := map[string]any{
		"id":                    "cwi-validation",
		"status":                "open",
		"sourceScope":           "dod_item",
		"sourceRequirementId":   "dod-validation",
		"targetText":            "neutral behavior validation remains missing",
		"requiredEvidenceKinds": []string{"validation"},
		"missingEvidenceKinds":  []string{"validation"},
		"acceptedEvidenceRefs":  []string{"run/graph/001-state-change"},
		"acceptedEvidenceByKind": map[string]any{
			"state_change": []string{"run/graph/001-state-change"},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus":  focus,
			"nextAction":    map[string]any{"kind": "acquire_evidence", "reason": "stale diagnostic lineage"},
		},
		gapBaselineKey: map[string]any{
			"schemaVersion": gapBaselineSchema,
			"currentFocus":  focus,
			"nextAction":    map[string]any{"kind": "acquire_evidence", "reason": "stale diagnostic lineage"},
		},
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": focus,
			"currentFocusItems": []map[string]any{
				focus,
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": evidenceAcquisitionRequiredOutput(nil),
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedValidation || contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("convergenceContract = %+v, want validation focus to preempt stale evidence acquisition", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true || boolValue(required["mustProduceConcreteFailureEvidence"]) {
		t.Fatalf("requiredOutput = %+v, want concrete validation evidence contract", required)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if got := stringFromMap(mapFromValue(mapFromValue(applied[convergenceAuthorityKey])["nextAction"]), "kind"); got != "validate" {
		t.Fatalf("convergenceAuthority.nextAction = %q, want validate; applied=%+v", got, applied)
	}
	if got := stringFromMap(mapFromValue(mapFromValue(applied[gapBaselineKey])["nextAction"]), "kind"); got != "validate" {
		t.Fatalf("gapBaseline.nextAction = %q, want validate; applied=%+v", got, applied)
	}
	applied[stepInputKeyStagePacket] = map[string]any{
		"schemaVersion":  "workorch.stage_packet.v1",
		"requiredOutput": mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"]),
	}
	if err := validateConvergenceDispatchInput(applied); err != nil {
		t.Fatalf("validateConvergenceDispatchInput() error = %v; applied=%+v", err, applied)
	}
}

func TestInitialStepInputAuthorityDeterministicCompletePreemptsStaleReplanStateChange(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-replan-authority-assessment",
		WorkspaceID:     "ws-replan-authority-assessment",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		State:           common.RunStateRunning,
		ExecutionContext: map[string]any{
			"definitionRevision": "rev-replan-authority-assessment",
			"dod": map[string]any{
				"id": "dod-authority",
				"items": []map[string]any{{
					"id":                    "dod-ready",
					"text":                  "neutral completed coverage needs final assessment",
					"requiredEvidenceKinds": []string{"validation"},
				}},
			},
			coverageWorkItemsKey: []map[string]any{{
				"definitionRevision":   "rev-replan-authority-assessment",
				"source":               "dod",
				"sourceScope":          "dod_item",
				"sourceRequirementId":  "dod-ready",
				"sourcePath":           "dod.items[0]",
				"targetText":           "neutral completed coverage needs final assessment",
				"status":               "open",
				"acceptedEvidenceRefs": []string{"evidence-validation-ready"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"evidence-validation-ready"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation": {"evidence-validation-ready"},
				},
			}},
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-stale-state-change",
				"reason":           "historical repair exhausted",
				"repairActionPackage": map[string]any{
					"requiredOutput": stateChangingRepairRequiredOutput(),
					"targetSnapshots": []map[string]any{{
						"targetRef":        "stale-target.txt",
						"exists":           true,
						"contentHash":      "hash-stale",
						"contentComplete":  true,
						"safeForFileWrite": true,
					}},
				},
				"stateChangingOutputRequired": true,
				"repairObjective":             repairObjectiveStateChangingOutput,
			},
		},
	}

	input := initialStepInput(run)

	if got := stringFromMap(mapFromValue(mapFromValue(input[convergenceAuthorityKey])["nextAction"]), "kind"); got != "complete" {
		t.Fatalf("convergenceAuthority.nextAction = %q, want complete; input=%+v", got, input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if state := stringFromMap(contract, "state"); state != "" {
		t.Fatalf("convergenceContract.state = %q, want no completion state", state)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageVerification {
		t.Fatalf("input = %+v, want verification runtime stage", input)
	}
	for _, key := range []string{"replanStateChangingContinuation", "stateChangingOutputRequired", "concreteFailureEvidenceRequired", stepInputKeyAssessmentOnly, "repairObjective"} {
		if _, ok := input[key]; ok {
			t.Fatalf("input[%s] = %+v, want stale dynamic contract stripped", key, input[key])
		}
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		t.Fatalf("repairActionPackage = %+v, want stale state-changing package stripped", pkg)
	}
}

func TestCoverageAssessmentIncompleteRecomputePreemptsStaleAssessmentAuthorityWithEvidenceGap(t *testing.T) {
	focus := map[string]any{
		"id":                   "cwi-state-change",
		"status":               "open",
		"sourceScope":          "deliverable",
		"sourceRequirementId":  "source_change",
		"targetText":           "neutral state-changing deliverable",
		"missingEvidenceKinds": []string{"state_change"},
		"requiredEvidenceKinds": []string{
			"state_change",
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            "complete_dod_goal_coverage_assessment",
		stepInputKeyAssessmentOnly:   true,
		"contractViolationRetry":     true,
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus":  focus,
			"nextAction":    map[string]any{"kind": "assessment"},
		},
		gapBaselineKey: map[string]any{
			"schemaVersion": gapBaselineSchema,
			"currentFocus":  focus,
			"nextAction":    map[string]any{"kind": "assessment"},
		},
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": focus,
			"unresolvedItems": []map[string]any{
				focus,
			},
		},
		"coverageTargetSet": []map[string]any{{
			"kind":                 "deliverable",
			"sourceScope":          "deliverable",
			"sourceRequirementId":  "source_change",
			"targetText":           "neutral state-changing deliverable",
			"status":               "open",
			"missingEvidenceKinds": []string{"state_change"},
			"requiredEvidenceKinds": []string{
				"state_change",
			},
		}},
		stepInputKeyContractViolationRecovery: map[string]any{
			"violatedRule":   contractViolationRuleCoverageAssessmentIncomplete,
			"recoverability": "recompute_authority",
			"correctionMode": contractViolationCorrectionModeForcedNarrow,
			"correctionContract": map[string]any{
				"requiredOutputShape": "traceable_coverage_assessment",
				"runtimeRecomputedAuthority": map[string]any{
					"owner":    "runtime",
					"required": true,
				},
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State == convergenceStateNeedAssessment {
		t.Fatalf("contract state = %q, want runtime recompute to avoid repeated assessment-only; contract=%+v", contract.State, contract)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied[stepInputKeyAssessmentOnly] != nil {
		t.Fatalf("%s = %v, want removed when recompute selects non-assessment action", stepInputKeyAssessmentOnly, applied[stepInputKeyAssessmentOnly])
	}
	if got := stringFromMap(mapFromValue(mapFromValue(applied[convergenceAuthorityKey])["nextAction"]), "kind"); got == "assessment" || got == "" {
		t.Fatalf("convergenceAuthority.nextAction = %q, want recomputed non-assessment action; applied=%+v", got, applied)
	}
	if got := stringFromMap(mapFromValue(mapFromValue(applied[gapBaselineKey])["nextAction"]), "kind"); got == "assessment" || got == "" {
		t.Fatalf("gapBaseline.nextAction = %q, want recomputed non-assessment action; applied=%+v", got, applied)
	}
}

func TestCoverageAssessmentIncompleteRecomputeSkipsAssessmentForDeterministicBinding(t *testing.T) {
	focus := map[string]any{
		"id":                    "cwi-deterministic-ready",
		"status":                "evidence_ready",
		"sourceScope":           "dod_item",
		"sourceRequirementId":   "dod-ready",
		"targetText":            "neutral validated requirement",
		"missingEvidenceKinds":  []string{},
		"requiredEvidenceKinds": []string{"validation"},
		"acceptedEvidenceByKind": map[string][]string{
			"validation": {"run/graph/001-validate"},
		},
		"acceptedEvidenceBindingByKind": map[string][]string{
			"validation": {"run/graph/001-validate"},
		},
		"acceptedEvidenceBindingRefs": []string{"run/graph/001-validate"},
		"acceptedEvidenceRefs":        []string{"run/graph/001-validate"},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            "complete_dod_goal_coverage_assessment",
		stepInputKeyAssessmentOnly:   true,
		"contractViolationRetry":     true,
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus":  focus,
			"nextAction":    map[string]any{"kind": "assessment"},
		},
		gapBaselineKey: map[string]any{
			"schemaVersion": gapBaselineSchema,
			"currentFocus":  focus,
			"nextAction":    map[string]any{"kind": "assessment"},
		},
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": focus,
			"unresolvedItems": []map[string]any{
				focus,
			},
		},
		stepInputKeyContractViolationRecovery: map[string]any{
			"violatedRule":   contractViolationRuleCoverageAssessmentIncomplete,
			"recoverability": "recompute_authority",
			"correctionMode": contractViolationCorrectionModeForcedNarrow,
			"correctionContract": map[string]any{
				"requiredOutputShape": "traceable_coverage_assessment",
				"runtimeRecomputedAuthority": map[string]any{
					"owner":    "runtime",
					"required": true,
				},
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State == convergenceStateNeedAssessment {
		t.Fatalf("contract state = %q, want deterministic ledger recompute to avoid repeated assessment-only; contract=%+v", contract.State, contract)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied[stepInputKeyAssessmentOnly] != nil {
		t.Fatalf("%s = %v, want removed when deterministic ledger owns current focus", stepInputKeyAssessmentOnly, applied[stepInputKeyAssessmentOnly])
	}
}

func TestCoverageAssessmentIncompleteStateChangeRecomputeSyncsEvidenceNextAction(t *testing.T) {
	focus := map[string]any{
		"id":                   "cwi-state-change",
		"status":               "open",
		"sourceScope":          "deliverable",
		"sourceRequirementId":  "source_change",
		"targetText":           "neutral state-changing deliverable",
		"missingEvidenceKinds": []string{"state_change"},
		"requiredEvidenceKinds": []string{
			"validation",
			"state_change",
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		stepInputKeyRuntimeStageMode:  "strict_convergence",
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"currentFocus":  focus,
			"nextAction":    map[string]any{"kind": "state_change", "reason": "state change evidence is missing"},
		},
		gapBaselineKey: map[string]any{
			"schemaVersion": gapBaselineSchema,
			"currentFocus":  focus,
			"nextAction":    map[string]any{"kind": "state_change", "reason": "state change evidence is missing"},
		},
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": focus,
			"unresolvedItems": []map[string]any{
				focus,
			},
		},
		stepInputKeyContractViolationRecovery: map[string]any{
			"violatedRule":   contractViolationRuleCoverageAssessmentIncomplete,
			"recoverability": "recompute_authority",
			"correctionContract": map[string]any{
				"requiredOutputShape": "bounded_failure_evidence",
				"runtimeRecomputedAuthority": map[string]any{
					"owner":    "runtime",
					"required": true,
				},
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("contract state = %q, want %q before state-changing dispatch is actionable; contract=%+v", contract.State, convergenceStateNeedEvidence, contract)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if got := stringFromMap(mapFromValue(mapFromValue(applied[convergenceAuthorityKey])["nextAction"]), "kind"); got != "acquire_evidence" {
		t.Fatalf("convergenceAuthority.nextAction = %q, want acquire_evidence; applied=%+v", got, applied)
	}
	if got := stringFromMap(mapFromValue(mapFromValue(applied[gapBaselineKey])["nextAction"]), "kind"); got != "acquire_evidence" {
		t.Fatalf("gapBaseline.nextAction = %q, want acquire_evidence; applied=%+v", got, applied)
	}
	applied[stepInputKeyStagePacket] = map[string]any{"schemaVersion": "workorch.stage_packet.v1"}
	if err := validateConvergenceDispatchInput(applied); err != nil {
		t.Fatalf("validateConvergenceDispatchInput() error = %v; applied=%+v", err, applied)
	}
}

func TestBuildConvergenceContractAuthorityPreemptsReplanWithoutInputPollution(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"schemaVersion": convergenceAuthoritySchema,
			"nextAction":    map[string]any{"kind": "assessment"},
		},
		coverageWorkItemContextKey: map[string]any{
			"currentFocusItems": []map[string]any{{
				"id":                    "cwi-authority-assessment",
				"source":                "dod",
				"sourcePath":            "dod.items[0]",
				"targetText":            "ready coverage",
				"status":                "open",
				"requiredEvidenceKinds": []string{"validation"},
				"acceptedEvidenceByKind": map[string]any{
					"validation": []string{"evidence-validation"},
				},
				"acceptedEvidenceRefs": []string{"evidence-validation"},
			}},
		},
		"replanContinuation": map[string]any{
			"predecessorRunId": "run-stale",
			"reason":           "stale repair lineage",
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":        "stale-target",
					"contentHash":      "hash-stale",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
		},
	}
	run := &runmodel.Run{
		ID:               "run-authority-preempts-replan",
		WorkspaceID:      "ws-authority-preempts-replan",
		ExecutionContext: input,
	}

	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": input}, nil)

	if contract.State != convergenceStateNeedAssessment {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedAssessment)
	}
	for _, key := range []string{"stateChangingOutputRequired", "repairObjective", "repairActionPackage"} {
		if _, ok := input[key]; ok {
			t.Fatalf("input[%s] = %+v, want authority next-action to avoid stale replan pollution", key, input[key])
		}
	}
}

func TestCoverageWorkItemEvidenceReadyAssessmentGapIsNotActionableEvidenceGap(t *testing.T) {
	item := map[string]any{
		"id":                    "cwi-ready",
		"status":                "evidence_ready",
		"requiredEvidenceKinds": []string{"validation", "state_change"},
		"missingEvidenceKinds":  []string{},
		"acceptedEvidenceRefs":  []string{"run/graph/001-patch", "run/graph/002-validate"},
		"acceptedEvidenceByKind": map[string]any{
			"state_change": []string{"run/graph/001-patch"},
			"validation":   []string{"run/graph/002-validate"},
		},
		"acceptedEvidenceBindingByKind": map[string]any{
			"state_change": []string{"run/graph/001-patch"},
			"validation":   []string{"run/graph/002-validate"},
		},
		"currentGap": "required evidence kinds present; coverage assessment required",
	}

	if coverageWorkItemHasOpenGap(item) {
		t.Fatalf("coverageWorkItemHasOpenGap(%+v) = true, want false for evidence_ready assessment-only gap", item)
	}
}

func TestConvergenceCoverageFocusRequiresValidationIgnoresStaleNextAllowedForEvidenceReady(t *testing.T) {
	input := map[string]any{
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-ready",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceRefs":  []string{"run/graph/001-validate"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"nextAllowedConvergenceStates": []string{convergenceStateNeedValidation},
			},
		},
	}

	if convergenceCoverageFocusRequiresValidation(input) {
		t.Fatalf("convergenceCoverageFocusRequiresValidation = true, want false for evidence_ready focus")
	}
}

func TestCoverageConvergenceFullChainActionMatrix(t *testing.T) {
	baseItem := func(id string, values map[string]any) map[string]any {
		item := map[string]any{
			"id":                  id,
			"sourceScope":         "dod_item",
			"sourceRequirementId": id + "-requirement",
			"targetText":          id + " neutral target",
			"status":              "open",
		}
		for key, value := range values {
			item[key] = value
		}
		return item
	}
	baseInput := func(item map[string]any, pkg map[string]any) map[string]any {
		if len(pkg) == 0 {
			pkg = map[string]any{}
		}
		pkg[coverageWorkItemContextKey] = map[string]any{
			"currentFocus":      cloneStepPayload(item),
			"currentFocusItems": []map[string]any{cloneStepPayload(item)},
			"summary":           map[string]any{"total": 1, stringFromMap(item, "status"): 1},
		}
		return map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageRepair,
			stepInputKeyRuntimeStageMode:      "strict_convergence",
			stepInputKeyRuntimeRepairFollowUp: true,
			"repairActionPackage":             pkg,
			coverageWorkItemContextKey:        pkg[coverageWorkItemContextKey],
		}
	}

	tests := []struct {
		name         string
		input        map[string]any
		wantState    string
		wantComplete bool
		check        func(t *testing.T, input map[string]any, contract ConvergenceContract, applied map[string]any)
	}{
		{
			name: "missing validation requires validation graph",
			input: baseInput(baseItem("cwi-validation", map[string]any{
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{"validation"},
			}), map[string]any{}),
			wantState: convergenceStateNeedValidation,
			check: func(t *testing.T, input map[string]any, contract ConvergenceContract, applied map[string]any) {
				t.Helper()
				if applied[stepInputKeyRuntimeStage] != runtimeStageVerification || applied["repairObjective"] != repairObjectiveConcreteValidationEvidence {
					t.Fatalf("applied = %+v, want verification concrete validation contract", applied)
				}
				required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
				if !boolValue(required["mustProduceConcreteValidationEvidence"]) || boolValue(required["mustIncludeStateChangingNode"]) {
					t.Fatalf("requiredOutput = %+v, want validation-only graph contract", required)
				}
				if !boolValue(required["mustReferenceCoverageWorkItem"]) {
					t.Fatalf("requiredOutput = %+v, want coverage work item binding", required)
				}
			},
		},
		{
			name: "missing state change with target snapshot requires state changing output",
			input: baseInput(baseItem("cwi-state-change", map[string]any{
				"requiredEvidenceKinds": []string{"state_change", "validation"},
				"missingEvidenceKinds":  []string{"state_change"},
			}), map[string]any{
				"targetSnapshots": []map[string]any{{
					"targetRef":        "neutral-target.txt",
					"exists":           true,
					"contentHash":      "hash-neutral-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			}),
			wantState: convergenceStateNeedStateChange,
			check: func(t *testing.T, input map[string]any, contract ConvergenceContract, applied map[string]any) {
				t.Helper()
				if applied[stepInputKeyRuntimeStage] != runtimeStageRepair || applied["repairObjective"] != repairObjectiveStateChangingOutput || applied["stateChangingOutputRequired"] != true {
					t.Fatalf("applied = %+v, want state-changing repair contract", applied)
				}
				required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
				if !boolValue(required["mustIncludeStateChangingNode"]) || !boolValue(required["mustReferenceCoverageWorkItem"]) {
					t.Fatalf("requiredOutput = %+v, want state-changing coverage work item contract", required)
				}
			},
		},
		{
			name: "evidence ready with accepted binding completes deterministically",
			input: baseInput(baseItem("cwi-ready", map[string]any{
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceRefs":  []string{"run/graph/validate-001"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/validate-001"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation": {"run/graph/validate-001"},
				},
			}), map[string]any{
				"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
			}),
			wantComplete: true,
			check: func(t *testing.T, input map[string]any, contract ConvergenceContract, applied map[string]any) {
				t.Helper()
				if !contract.DeterministicComplete || contract.State != "" || contract.RuntimeStageMode != "deterministic_ledger" {
					t.Fatalf("contract = %+v, want deterministic completion without convergence state", contract)
				}
				if state := stringFromMap(mapFromValue(applied["convergenceContract"]), "state"); state != "" {
					t.Fatalf("applied convergenceContract.state = %q, want no completion state", state)
				}
			},
		},
		{
			name: "satisfied item does not request repair",
			input: baseInput(baseItem("cwi-satisfied", map[string]any{
				"status":                "satisfied",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceRefs":  []string{"run/graph/validate-001"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/validate-001"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation": {"run/graph/validate-001"},
				},
			}), map[string]any{}),
			wantState: convergenceStateBlocked,
			check: func(t *testing.T, input map[string]any, contract ConvergenceContract, applied map[string]any) {
				t.Helper()
				if _, ok := applied["convergenceContract"]; ok {
					t.Fatalf("applied = %+v, want no convergence repair contract for satisfied item", applied)
				}
			},
		},
		{
			name: "structured decision evidence gap requires governance",
			input: baseInput(baseItem("cwi-decision", map[string]any{
				"requiredEvidenceKinds": []string{"decision"},
				"missingEvidenceKinds":  []string{"decision"},
				"currentGap":            "structured owner evidence missing",
			}), map[string]any{}),
			wantState: convergenceStateNeedGovernance,
			check: func(t *testing.T, input map[string]any, contract ConvergenceContract, applied map[string]any) {
				t.Helper()
				if applied[stepInputKeyRuntimeStage] != runtimeStageDecision || applied["governanceRequired"] != true {
					t.Fatalf("applied = %+v, want structured governance contract", applied)
				}
			},
		},
		{
			name: "structured approval evidence gap requires governance",
			input: baseInput(baseItem("cwi-approval", map[string]any{
				"requiredEvidenceKinds": []string{"approval"},
				"missingEvidenceKinds":  []string{"approval"},
				"currentGap":            "structured owner evidence missing",
			}), map[string]any{}),
			wantState: convergenceStateNeedGovernance,
			check: func(t *testing.T, input map[string]any, contract ConvergenceContract, applied map[string]any) {
				t.Helper()
				if applied[stepInputKeyRuntimeStage] != runtimeStageDecision || applied["governanceRequired"] != true {
					t.Fatalf("applied = %+v, want structured governance contract", applied)
				}
			},
		},
		{
			name: "legacy decision text does not infer governance",
			input: baseInput(baseItem("cwi-legacy-text", map[string]any{
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{"validation"},
				"currentGap":            "legacy text mentions decision but structured evidence kind is validation",
			}), map[string]any{}),
			wantState: convergenceStateNeedValidation,
			check: func(t *testing.T, input map[string]any, contract ConvergenceContract, applied map[string]any) {
				t.Helper()
				if applied[stepInputKeyRuntimeStage] != runtimeStageVerification || applied["governanceRequired"] == true {
					t.Fatalf("applied = %+v, want validation contract without governance inferred from text", applied)
				}
				required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
				if !boolValue(required["mustProduceConcreteValidationEvidence"]) {
					t.Fatalf("requiredOutput = %+v, want validation evidence contract", required)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := cloneStepPayload(tt.input)
			contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
			if contract.State != tt.wantState || contract.DeterministicComplete != tt.wantComplete {
				t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, tt.wantState, contract)
			}
			applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
			tt.check(t, input, contract, applied)
		})
	}
}

func TestBuildConvergenceContractCurrentFocusAssessmentPreemptsHistoricalCoverageGaps(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		stepInputKeyRuntimeStageMode:  "strict_convergence",
		"runtimeRepairFollowUp":       true,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"boundedValidationRequired":   true,
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-ready",
				"sourceScope":           "goal_leaf",
				"sourceRequirementId":   "goal-req-ready",
				"targetText":            "focused requirement",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceRefs":         []string{"run/graph/001-validate"},
				"nextAllowedConvergenceStates": []string{convergenceStateNeedAssessment},
			},
			"currentFocusItems": []map[string]any{{
				"id":                    "cwi-ready",
				"sourceScope":           "goal_leaf",
				"sourceRequirementId":   "goal-req-ready",
				"targetText":            "focused requirement",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceRefs":         []string{"run/graph/001-validate"},
				"nextAllowedConvergenceStates": []string{convergenceStateNeedAssessment},
			}},
			"ledgerSummary": []map[string]any{{
				"id":                    "cwi-historical",
				"sourceScope":           "dod_item",
				"sourceRequirementId":   "dod-item-historical",
				"targetText":            "historical requirement",
				"status":                "open",
				"missingEvidenceKinds":  []string{"validation"},
				"requiredEvidenceKinds": []string{"validation"},
			}},
			"summary": map[string]any{"total": 2, "evidence_ready": 1, "open": 1},
		},
		"missingDoDItems": []map[string]any{{
			"item":       "historical requirement",
			"targetText": "historical requirement",
			"state":      "missing",
		}},
		"coverageTargetSet": []map[string]any{{
			"kind":                "dod",
			"sourceRequirementId": "dod-item-historical",
			"coverageTargetToken": "dod-item-historical",
			"targetText":          "historical requirement",
			"state":               "failed",
		}, {
			"kind":                "goal",
			"sourceRequirementId": "goal-req-ready",
			"coverageTargetToken": "goal-req-ready",
			"targetText":          "focused requirement",
			"state":               "evidence_ready",
		}},
		"repairActionPackage": map[string]any{
			"coverageRepairRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":                "dod",
				"sourceRequirementId": "dod-item-historical",
				"coverageTargetToken": "dod-item-historical",
				"targetText":          "historical requirement",
				"state":               "failed",
			}, {
				"kind":                "goal",
				"sourceRequirementId": "goal-req-ready",
				"coverageTargetToken": "goal-req-ready",
				"targetText":          "focused requirement",
				"state":               "evidence_ready",
			}},
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"mustDeclareCoverageTargets":   true,
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence/repair-001",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: cloneStepPayload(input),
		Output: map[string]any{
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=repair",
		},
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State != "" || !contract.DeterministicComplete || contract.RuntimeStageMode != "deterministic_ledger" {
		t.Fatalf("contract = %+v, want deterministic completion without convergence state for current focus accepted binding", contract)
	}
}

func TestBuildConvergenceContractExplicitDeliverableGapPreemptsAssessmentReadyFocus(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		stepInputKeyAssessmentOnly:   true,
		"repairObjective":            "complete_dod_goal_coverage_assessment",
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-ready",
				"sourceScope":           "goal_leaf",
				"sourceRequirementId":   "goal-req-ready",
				"targetText":            "already evidenced goal",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceRefs": []string{"run/graph/001-validate"},
			},
			"currentFocusItems": []map[string]any{{
				"id":                    "cwi-ready",
				"sourceScope":           "goal_leaf",
				"sourceRequirementId":   "goal-req-ready",
				"targetText":            "already evidenced goal",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation": {"run/graph/001-validate"},
				},
				"acceptedEvidenceRefs": []string{"run/graph/001-validate"},
			}},
			"summary": map[string]any{"total": 30, "evidence_ready": 30},
		},
		"missingDeliverableCoverage": []map[string]any{{
			"id":         "coverage_map",
			"item":       "coverage_map",
			"targetText": "coverage_map",
			"state":      "failed",
		}},
		"repairActionPackage": map[string]any{
			"coverageRepairRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":                "deliverable",
				"source":              "deliverableCoverage",
				"sourceRequirementId": "coverage_map",
				"targetText":          "coverage_map",
				"coverageTargetToken": "coverage_map",
			}},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence/assessment-001",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: cloneStepPayload(input),
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingDeliverableCoverage": []map[string]any{{
					"id":    "coverage_map",
					"item":  "coverage_map",
					"state": "failed",
				}},
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State != convergenceStateNeedValidation {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedValidation, contract)
	}
	if contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repairObjective = %q, want %q", contract.RepairObjective, repairObjectiveConcreteValidationEvidence)
	}
	targets := mapSliceFromValue(contract.RepairActionPackage["coverageTargetSet"])
	if len(targets) == 0 || stringFromMap(targets[0], "coverageTargetToken") != "coverage_map" {
		t.Fatalf("coverageTargetSet = %+v, want explicit coverage_map target", targets)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied[stepInputKeyAssessmentOnly] != nil || applied["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("applied = %+v, want validation repair without assessment-only", applied)
	}
}

func TestBuildConvergenceContractIgnoresExplicitDeliverableGapClosedByCoverageWorkItem(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		stepInputKeyAssessmentOnly:   true,
		"repairObjective":            "complete_dod_goal_coverage_assessment",
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-source-change",
				"sourceScope":           "deliverable",
				"sourceRequirementId":   "source_change",
				"targetText":            "source_change",
				"status":                "evidence_ready",
				"requiredEvidenceKinds": []string{"validation", "state_change"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation":   {"run/graph/001-validate"},
					"state_change": {"run/graph/001-patch"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation":   {"run/graph/001-validate"},
					"state_change": {"run/graph/001-patch"},
				},
				"acceptedEvidenceRefs": []string{"run/graph/001-patch", "run/graph/001-validate"},
			},
			"ledgerSummary": []map[string]any{{
				"id":                   "cwi-source-change",
				"sourceScope":          "deliverable",
				"sourceRequirementId":  "source_change",
				"status":               "evidence_ready",
				"missingEvidenceKinds": nil,
				"acceptedEvidenceRefs": []string{"run/graph/001-patch", "run/graph/001-validate"},
				"acceptedEvidenceByKind": map[string][]string{
					"validation":   {"run/graph/001-validate"},
					"state_change": {"run/graph/001-patch"},
				},
				"acceptedEvidenceBindingByKind": map[string][]string{
					"validation":   {"run/graph/001-validate"},
					"state_change": {"run/graph/001-patch"},
				},
				"omittedFromCurrentFocus": false,
			}},
			"summary": map[string]any{"total": 1, "evidence_ready": 1},
		},
		"missingDeliverableCoverage": []map[string]any{{
			"id":         "source_change",
			"item":       "source_change",
			"kind":       "deliverable",
			"source":     "deliverableCoverage",
			"targetText": "source_change",
			"state":      "missing",
		}},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence/assessment-closed-gap",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: cloneStepPayload(input),
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingDeliverableCoverage": []map[string]any{{
					"id":     "source_change",
					"item":   "source_change",
					"kind":   "deliverable",
					"source": "deliverableCoverage",
					"state":  "missing",
				}},
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State != "" || !contract.DeterministicComplete || contract.RuntimeStageMode != "deterministic_ledger" {
		t.Fatalf("contract = %+v, want deterministic completion without convergence state when CWI already closed the explicit gap", contract)
	}
	if explicitCoverageGapShouldPreemptAssessment(input, failed) {
		t.Fatalf("explicit coverage gap should not preempt assessment when CWI already closed it")
	}
	if open := filterCoverageGapsOpenByWorkItems(input, mapSliceFromValue(input["missingDeliverableCoverage"])); len(open) != 0 {
		t.Fatalf("open coverage gaps = %+v, want stale gap filtered", open)
	}
}

func TestBuildConvergenceContractPromotesConcreteValidationTargetEvidenceToStateChange(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:    runtimeStageVerification,
		"repairObjective":           repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired": true,
		"coverageTargetSet": []map[string]any{{
			"kind":                "dod",
			"coverageTargetToken": "dod-streaming",
			"targetText":          "streaming behavior remains unresolved",
			"state":               "failed",
		}},
		"convergenceContract": map[string]any{
			"state":           convergenceStateNeedValidation,
			"runtimeStage":    runtimeStageVerification,
			"repairObjective": repairObjectiveConcreteValidationEvidence,
			"targetEvidence": map[string]any{
				"repairTargetSet": []string{"proxy/proxy.go", "proxy/proxy_test.go"},
				"coverageTargetSet": []map[string]any{{
					"kind":                "dod",
					"coverageTargetToken": "dod-streaming",
					"targetText":          "streaming behavior remains unresolved",
					"state":               "failed",
				}},
				"targetSnapshots": []map[string]any{{
					"targetRef":        "proxy/proxy.go",
					"exists":           true,
					"content":          "package proxy\n",
					"contentHash":      "hash-proxy",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence/follow-up",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: input,
		Output: map[string]any{
			"stage": "llm_dod_assessment",
			"error": "invalid argument: dodAssessment is not satisfied",
		},
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	if !contract.StateChangingOutputRequired {
		t.Fatalf("StateChangingOutputRequired = false, want true")
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		stringSliceContains(stringSliceFromValue(required["forbiddenKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want state-changing patch contract", required)
	}
	if targets := mapSliceFromValue(contract.RepairActionPackage["targetSnapshots"]); len(targets) != 1 || targets[0]["targetRef"] != "proxy/proxy.go" {
		t.Fatalf("targetSnapshots = %+v, want hydrated target snapshot", targets)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied[stepInputKeyRuntimeStage] != runtimeStageRepair ||
		applied["repairObjective"] != repairObjectiveStateChangingOutput ||
		applied["stateChangingOutputRequired"] != true {
		t.Fatalf("applied input = %+v, want state-changing repair", applied)
	}
}

func TestBuildConvergenceContractBlockedByDefault(t *testing.T) {
	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": map[string]any{}}, nil)
	if contract.State != convergenceStateBlocked {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateBlocked)
	}
}

func TestShouldCreateDecisionTicketBlocksMechanicalStateChangingDecision(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		stepInputKeyRuntimeStageMode:  "strict_convergence",
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"targetSnapshots": []map[string]any{{
				"targetRef":        "target.txt",
				"exists":           true,
				"contentHash":      "hash-target",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
	}
	step := &runmodel.Step{ID: "run-convergence/repair-001", Input: input}
	spec := runtimeDecisionBlockSpec{
		Reason:       "need target repair choice",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Should the repair update this target?",
	}
	output := decisionOutputFromSpec(spec)
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": input}, map[string]any{"llmOutput": output})

	if ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = true, want false for mechanically repairable state-changing contract")
	}
}

func TestShouldCreateDecisionTicketBlocksProfileRequiredStateChangingEvidenceDecision(t *testing.T) {
	contract := ConvergenceContract{
		State:                       convergenceStateNeedStateChange,
		RuntimeStage:                runtimeStageRepair,
		RuntimeStageMode:            "strict_convergence",
		RepairObjective:             repairObjectiveStateChangingOutput,
		StateChangingOutputRequired: true,
		RepairActionPackage: map[string]any{
			"coverageTargetSet": []map[string]any{{
				"kind": "deliverable",
				"item": "version_control_snapshot",
			}},
			"evidenceKindRequirements": []map[string]any{{
				"scope":             "deliverable",
				"id":                "version_control_snapshot",
				"acceptedStepKinds": []string{gitcommon.StepKindWrite},
			}},
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
				"acceptedKinds":                []string{gitcommon.StepKindWrite},
				"acceptedOperations":           []string{gitcommon.OperationCommit},
			},
		},
	}
	output := decisionOutputFromSpec(runtimeDecisionBlockSpec{
		Reason:       "need confirmation before version-control snapshot",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Should the runtime create the declared snapshot?",
	})

	if ShouldCreateDecisionTicket(map[string]any{}, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = true, want false for profile-declared state-changing evidence contract")
	}
}

func TestShouldCreateDecisionTicketKeepsExternalGovernanceDecision(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"targetSnapshots": []map[string]any{{
				"targetRef":   "target.txt",
				"exists":      true,
				"contentHash": "hash-target",
			}},
		},
	}
	step := &runmodel.Step{ID: "run-convergence/repair-001", Input: input}
	spec := runtimeDecisionBlockSpec{
		Reason:       "provider config required",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Please configure the provider.",
		Context: map[string]any{
			decisionContextKeyScope:      decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency: decisionContextDependencyLLMProvider,
		},
	}
	output := decisionOutputFromSpec(spec)
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": input}, map[string]any{"llmOutput": output})

	if !ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = false, want true for governance dependency")
	}
}

func TestShouldCreateDecisionTicketBlocksDeterministicLedgerNextAction(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": "validate"},
			"currentFocus": map[string]any{
				"id":                   "cwi-validation",
				"missingEvidenceKinds": []string{"validation"},
			},
		},
	}
	output := decisionOutputFromSpec(runtimeDecisionBlockSpec{
		Reason:       "need decision even though validation can continue",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Should validation continue?",
	})
	contract := ConvergenceContract{State: convergenceStateBlocked}

	if ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = true, want false while ledger has deterministic validation next action")
	}
}

func TestShouldCreateDecisionTicketBlocksDeterministicNextActionEvenWithUserChoiceContext(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
			"currentFocus": map[string]any{
				"id":                   "cwi-state-change",
				"missingEvidenceKinds": []string{"state_change"},
			},
		},
	}
	output := decisionOutputFromSpec(runtimeDecisionBlockSpec{
		Reason:       "user choice requested while state change remains deterministic",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Choose a state change path.",
		Context: map[string]any{
			"requiresUserChoice": true,
		},
	})
	contract := ConvergenceContract{State: convergenceStateBlocked}

	if ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = true, want false when deterministic next action still exists")
	}
}

func TestShouldCreateDecisionTicketKeepsUserChoiceWhenNoDeterministicNextAction(t *testing.T) {
	output := decisionOutputFromSpec(runtimeDecisionBlockSpec{
		Reason:       "user choice required",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Choose one available option.",
		Context: map[string]any{
			"requiresUserChoice": true,
		},
	})
	contract := ConvergenceContract{State: convergenceStateBlocked}

	if !ShouldCreateDecisionTicket(map[string]any{}, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = false, want true for explicit user choice without deterministic next action")
	}
}

func TestShouldCreateDecisionTicketBlocksMechanicalConcreteValidationDecision(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired":  true,
		"evidenceRepairRequired":     true,
		"coverageTargetSet": []map[string]any{{
			"kind":       "dod",
			"targetText": "close validation coverage",
		}},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
			},
		},
	}
	step := &runmodel.Step{ID: "run-convergence/validation-001", Input: input}
	spec := runtimeDecisionBlockSpec{
		Reason:       "need stage choice",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Should the runtime switch to an execution stage?",
	}
	output := decisionOutputFromSpec(spec)
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": input}, map[string]any{"llmOutput": output})

	if ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = true, want false for mechanically continuable validation evidence contract")
	}
}

func TestBuildConvergenceContractDoesNotPromoteDiagnosticExpectedContractOverNarrowValidation(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		stepInputKeyRuntimeStageMode:  "strict_convergence",
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"contractViolationRetry":      true,
		stepInputKeyContractViolationRecovery: map[string]any{
			"violatedRule":   contractViolationRuleConcreteValidationMissing,
			"correctionMode": contractViolationCorrectionModeForcedNarrow,
			"correctionContract": map[string]any{
				"requiredOutputShape": "bounded_validation_evidence",
				"mode":                contractViolationCorrectionModeForcedNarrow,
				"allowedOutputs":      []string{"executionGraph_with_declared_validationEvidence"},
				"forbiddenOutputs":    []string{"decision_request_when_runtime_can_continue"},
			},
			"expectedContract": map[string]any{
				"stateChangingOutputRequired": true,
				"repairObjective":             repairObjectiveStateChangingOutput,
				"convergenceContract": map[string]any{
					"state":                       convergenceStateNeedStateChange,
					"stateChangingOutputRequired": true,
					"repairObjective":             repairObjectiveStateChangingOutput,
				},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	step := &runmodel.Step{ID: "run-convergence/validation-retry", Input: input, Output: map[string]any{
		failureOutputKeyClass:     failureClassRuntime,
		failureOutputKeySignature: "class=runtime_failed|rule=concrete_validation_contract_missing",
	}}

	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": input}, nil)
	if contract.State != convergenceStateNeedValidation {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedValidation, contract)
	}
	if contract.StateChangingOutputRequired {
		t.Fatalf("contract = %+v, want diagnostic expectedContract not to require state change", contract)
	}

	output := decisionOutputFromSpec(runtimeDecisionBlockSpec{
		Reason:       "diagnostic expected contract appears contradictory",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Please resolve the stale expected contract contradiction.",
		Context: map[string]any{
			"requiresHumanFields": []string{"resolution"},
			"knownValues": map[string]any{
				"requiredStateChanging": true,
			},
			"runtimeCanContinue": false,
		},
	})
	if ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = true, want false for stale diagnostic expectedContract blocker")
	}
}

func TestShouldCreateDecisionTicketKeepsConcreteValidationRuntimeDependency(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:    runtimeStageVerification,
		"repairObjective":           repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []string{"shell/command"},
			},
		},
	}
	step := &runmodel.Step{ID: "run-convergence/validation-001", Input: input}
	spec := runtimeDecisionBlockSpec{
		Reason:       "runtime config required",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Please configure the runtime.",
		Context: map[string]any{
			decisionContextKeyScope:      decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency: decisionContextDependencyContextGate,
		},
	}
	output := decisionOutputFromSpec(spec)
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": input}, map[string]any{"llmOutput": output})

	if !ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = false, want true for runtime dependency")
	}
}

func TestShouldCreateDecisionTicketBlocksContinuableRuntimeContractViolationDecision(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:    runtimeStageVerification,
		"repairObjective":           repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
			},
		},
	}
	output := decisionOutputFromSpec(runtimeDecisionBlockSpec{
		Reason:       "contract violation recovery exhausted",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Choose a recovery action.",
		Context: map[string]any{
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyRuntimeContract,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
			"contractViolation": map[string]any{
				"violatedRule": contractViolationRuleValidationEvidenceMetadataMissing,
			},
		},
	})
	contract := ConvergenceContract{
		State:           convergenceStateNeedValidation,
		RuntimeStage:    runtimeStageVerification,
		RepairObjective: repairObjectiveConcreteValidationEvidence,
		RepairActionPackage: map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteValidationEvidence": true,
				"acceptedKinds":                         []string{"shell/command", "mcp/tool_call"},
			},
		},
	}

	if ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = true, want false for continuable runtime contract violation")
	}
}

func TestShouldCreateDecisionTicketBlocksMechanicalEvidenceAcquisitionDecision(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"mustProduceConcreteFailureEvidence": true,
				"acceptedKinds": []string{
					"file/list",
					"file/read",
					"shell/command",
					"mcp/tool_call",
				},
			},
		},
	}
	step := &runmodel.Step{ID: "run-convergence/evidence-001", Input: input}
	spec := runtimeDecisionBlockSpec{
		Reason:       "need stage choice",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Should the runtime switch stage?",
	}
	output := decisionOutputFromSpec(spec)
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": input}, map[string]any{"llmOutput": output})

	if ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = true, want false for mechanically continuable evidence acquisition contract")
	}
}

func TestShouldCreateDecisionTicketKeepsEvidenceAcquisitionRuntimeDependency(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": evidenceAcquisitionRequiredOutput(nil),
		},
	}
	step := &runmodel.Step{ID: "run-convergence/evidence-001", Input: input}
	spec := runtimeDecisionBlockSpec{
		Reason:       "runtime config required",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Please configure the runtime.",
		Context: map[string]any{
			decisionContextKeyScope:      decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency: decisionContextDependencyContextGate,
		},
	}
	output := decisionOutputFromSpec(spec)
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": input}, map[string]any{"llmOutput": output})

	if !ShouldCreateDecisionTicket(input, output, contract) {
		t.Fatalf("ShouldCreateDecisionTicket = false, want true for runtime dependency")
	}
}

func TestBuildConvergenceContractNeedGovernanceForRuntimeDependencyDecision(t *testing.T) {
	step := &runmodel.Step{
		ID: "run-governance/repair-001",
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
		Output: map[string]any{
			stepOutputKeyBlockDecisionContext: map[string]any{
				decisionContextKeyScope:      decisionContextScopeWorkorchRuntime,
				decisionContextKeyDependency: decisionContextDependencyContextGate,
				decisionContextKeyReason:     "runtime context gate requires missing protected snapshot",
			},
		},
	}
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": step.Input}, nil)
	if contract.State != convergenceStateNeedGovernance {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedGovernance)
	}
	if contract.RuntimeStage != runtimeStageDecision {
		t.Fatalf("runtimeStage = %q, want %q", contract.RuntimeStage, runtimeStageDecision)
	}
}

func TestBuildConvergenceContractUsesLatestActionEvidenceAsConcreteTarget(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"currentExecutionProgress": map[string]any{
			"failedStepId":     "run-convergence/follow-up-verify",
			"failureClass":     failureClassRuntime,
			"failureReason":    "validation follow-up llm step returned read-only executionGraph without patch output",
			"failureStage":     "llm_execution_graph_plan",
			"coverageGapCount": 0,
			"latestActionEvidence": []map[string]any{{
				"stepId": "run-convergence/graph-003/002-verify",
				"kind":   "shell/command",
				"state":  string(common.StepStateCompleted),
				"outputSummary": map[string]any{
					"status":        "completed",
					"command":       "sh",
					"exitCode":      0,
					"stdoutPreview": "# target [target.test]\n./main.go:5:2: \"bytes\" imported and not used\nFAIL",
				},
			}},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"currentExecutionProgress": map[string]any{
				"latestActionEvidence": []map[string]any{{
					"kind": "shell/command",
					"outputSummary": map[string]any{
						"stdoutPreview": "./main.go:5:2: \"bytes\" imported and not used",
					},
				}},
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence/follow-up-verify",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: input,
		Output: map[string]any{
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
			"error":                   "validation follow-up llm step returned read-only executionGraph without patch output",
		},
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": input}, nil)
	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedStateChange)
	}
	if len(contract.TargetEvidence) == 0 {
		t.Fatalf("targetEvidence missing from contract: %+v", contract)
	}
	signals := mapFromValue(contract.TargetEvidence["structuredFailureSignals"])
	if !stringSliceContains(stringSliceFromValue(signals["targetRefs"]), "main.go") {
		t.Fatalf("targetEvidence.structuredFailureSignals = %+v, want latest action target main.go", signals)
	}

	applied := ApplyConvergenceContract(map[string]any{}, contract)
	required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want patch/apply state-changing contract", required)
	}
}

func TestBuildConvergenceContractPreservesValidationFocusAfterInvalidStateChangingRepair(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeRepairFollowUp: true,
		stepInputKeyRuntimeStage:          runtimeStageRepair,
		"repairObjective":                 repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired":     true,
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                           "cwi-validation",
				"status":                       "failed",
				"sourceRequirementId":          "req-validation",
				"sourceScope":                  "dod_item",
				"targetText":                   "path compatibility must be validated",
				"requiredEvidenceKinds":        []string{"validation"},
				"missingEvidenceKinds":         []string{"validation"},
				"nextAllowedConvergenceStates": []string{convergenceStateNeedValidation},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"failureTargetRepairContext": []map[string]any{{
				"targetRef": "target.txt",
				"exists":    true,
			}},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence/repair-invalid-patch",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: input,
		Output: map[string]any{
			"stage":               "llm_execution_graph_plan",
			failureOutputKeyClass: failureClassRuntime,
			"error":               "invalid argument: executionGraph patch/apply input invalid: invalid argument: patch describes a non-substantive state-change placeholder",
		},
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": input}, nil)
	if contract.State != convergenceStateNeedValidation {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedValidation)
	}
	if contract.RuntimeStage != runtimeStageVerification || contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("contract = %+v, want concrete validation evidence contract", contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true || required["mustIncludeStateChangingNode"] == true {
		t.Fatalf("requiredOutput = %+v, want validation-only repair contract", required)
	}
}

func TestBuildConvergenceContractRequiresEvidenceWhenStateChangeOnlyHasRuntimeFieldTargets(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"coverageTargetSet": []map[string]any{{
			"kind":       "dod",
			"targetText": "unresolved requirement",
			"state":      "failed",
		}},
		"structuredFailureSignals": map[string]any{
			"targetRefs": []string{"validationEvidence.mode", "validationevidence.observ"},
			"messages": []string{
				"concrete validation nodes must declare validationEvidence.mode and validationEvidence.observationType",
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"repairTargetSet": []string{
				"validationEvidence.mode",
				"validationevidence.observ",
			},
			"targetSnapshots": []map[string]any{{
				"targetRef": "validationEvidence.mode",
				"exists":    false,
				"error":     "no such file or directory",
			}},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-convergence/repair-invalid-targets",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: input,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan",
			"error":                   "concrete validation nodes must declare validationEvidence.mode and validationEvidence.observationType",
		},
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedEvidence, contract)
	}
	if contract.StateChangingOutputRequired {
		t.Fatalf("contract.StateChangingOutputRequired = true, want false")
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if boolValue(applied["stateChangingOutputRequired"]) {
		t.Fatalf("stateChangingOutputRequired still set in applied input: %+v", applied)
	}
	required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "file/read") ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want bounded evidence acquisition without patch/apply", required)
	}
	pkg := mapFromValue(applied["repairActionPackage"])
	if len(stringSliceFromValue(pkg["repairTargetSet"])) > 0 || len(mapSliceFromValue(pkg["targetSnapshots"])) > 0 {
		t.Fatalf("repairActionPackage = %+v, want invalid runtime-field target evidence removed", pkg)
	}
}

func TestRefreshConvergenceCoverageTargetsPreservesPrerequisiteRootCauseSnapshots(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"coverageTargetSet": []map[string]any{{
			"kind":       "deliverable",
			"targetText": "validation_report",
			"state":      "open",
		}},
		"missingDeliverableCoverage": []map[string]any{{
			"kind":       "deliverable",
			"item":       "validation_report",
			"targetText": "validation_report",
			"state":      "open",
		}},
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"rootCauseCandidateContext": []map[string]any{{
				"targetRef":        "dependency.manifest",
				"source":           "prerequisite_manifest_candidate",
				"exists":           true,
				"contentHash":      "sha256-current",
				"contentComplete":  true,
				"safeForFileWrite": true,
				"content":          "dependency = stable\n",
			}},
		},
	}

	refreshConvergenceCoverageTargets(input)

	pkg := mapFromValue(input["repairActionPackage"])
	snapshots := mapSliceFromValue(pkg["targetSnapshots"])
	if len(snapshots) != 1 || stringFromMap(snapshots[0], "targetRef") != "dependency.manifest" {
		t.Fatalf("targetSnapshots = %+v, want promoted prerequisite root-cause snapshot", snapshots)
	}
	if targets := stringSliceFromValue(pkg["repairTargetSet"]); !stringSliceContains(targets, "dependency.manifest") {
		t.Fatalf("repairTargetSet = %+v, want prerequisite target retained", targets)
	}
	required := mapFromValue(pkg["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true || required["toolStateChangingInputContract"] == nil {
		t.Fatalf("requiredOutput = %+v, want state-changing contract retained", required)
	}
}

func TestDecisionResolutionInputAppliesConvergenceContract(t *testing.T) {
	ticket := &decisionmodel.Ticket{
		ID:           "ticket-resolved-continue",
		StepID:       "run-resolved-continue/repair-001",
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     "Should we continue the target repair?",
		Reason:       "need target repair choice",
		Context: map[string]any{
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":        "target.txt",
					"exists":           true,
					"contentHash":      "hash-target",
					"contentComplete":  true,
					"safeForFileWrite": true,
				}},
			},
			"stateChangingOutputRequired": true,
		},
	}
	resolution := &decisionmodel.Resolution{
		ID:   "resolution-resolved-continue",
		Type: decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload: map[string]any{
			"answer":         "continue",
			"recoveryAction": "continue_same_target_repair",
			"decisionReason": "continue",
		},
	}

	input := decisionResolutionInput(ticket, resolution)

	if got := stringFromMap(input, stepInputKeyRuntimeStage); got != runtimeStageRepair {
		t.Fatalf("runtimeStage = %q, want %q", got, runtimeStageRepair)
	}
	if !boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("stateChangingOutputRequired = false, want true")
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
}

func TestInitialStepInputTreatsReplanRepairPackageAsDiagnosticLineage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-replan-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-predecessor",
				"repairActionPackage": map[string]any{
					"requiredOutput": stateChangingRepairRequiredOutput(),
					"targetSnapshots": []map[string]any{{
						"targetRef":        "target.txt",
						"exists":           true,
						"contentHash":      "hash-target",
						"contentComplete":  true,
						"safeForFileWrite": true,
					}},
				},
				"stateChangingOutputRequired": true,
				"repairObjective":             repairObjectiveStateChangingOutput,
			},
		},
	}

	input := initialStepInput(run)

	if got := stringFromMap(input, stepInputKeyRuntimeStage); got != runtimeStageRepair {
		t.Fatalf("runtimeStage = %q, want %q", got, runtimeStageRepair)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedEvidence ||
		input["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		input["concreteFailureEvidenceRequired"] != true {
		t.Fatalf("input = %+v, want stale replan repair package to require current evidence", input)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if targets := mapSliceFromValue(pkg["targetSnapshots"]); len(targets) > 0 {
		t.Fatalf("repairActionPackage.targetSnapshots = %+v, want stale replan target ignored", targets)
	}
	if required := mapFromValue(pkg["requiredOutput"]); boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("repairActionPackage.requiredOutput = %+v, want evidence acquisition contract", required)
	}
}

func TestBuildConvergenceContractStartsPrimaryDeliverableStateChangeFromCoverageFocus(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-primary-deliverable-cwi",
		ExecutionContext: map[string]any{
			"goal": map[string]any{
				"id":   "goal-primary-deliverable-cwi",
				"text": "deliver observable behavior",
			},
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true, "evidenceRole": "primary_state_change"},
							{"id": "validation_report", "required": true, "evidenceRole": "supplemental_evidence"},
						},
					},
				},
			},
		},
	}
	newCoverageSubsystem().ensure(run)
	input := map[string]any{
		coverageWorkItemContextKey:        run.ExecutionContext[coverageWorkItemContextKey],
		stepInputKeyRuntimeStage:          runtimeStageVerification,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		stepInputKeyRuntimeRepairFollowUp: true,
		"repairObjective":                 repairObjectiveConcreteValidationEvidence,
	}

	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": input}, nil)

	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("contract = %+v, want concrete target evidence before primary deliverable state-changing contract", contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if !boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		boolValue(required["mustIncludeStateChangingNode"]) ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want evidence acquisition before primary patch-capable output", required)
	}
	targets := mapSliceFromValue(mapFromValue(contract.RepairActionPackage)["coverageTargetSet"])
	if len(targets) == 0 || stringFromMap(targets[0], "sourceRequirementId") != "source_change" {
		t.Fatalf("coverageTargetSet = %+v, want source_change focus target", targets)
	}
}

func TestBuildConvergenceContractDefersSupplementalDeliverableStateChangeWithoutTargetEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-supplemental-deliverable-cwi",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "evidence_ledger", "required": true, "evidenceRole": "supplemental_evidence"},
						},
					},
				},
			},
		},
	}
	newCoverageSubsystem().ensure(run)
	input := map[string]any{
		coverageWorkItemContextKey:        run.ExecutionContext[coverageWorkItemContextKey],
		stepInputKeyRuntimeStage:          runtimeStageVerification,
		stepInputKeyRuntimeStageMode:      "strict_convergence",
		stepInputKeyRuntimeRepairFollowUp: true,
		"repairObjective":                 repairObjectiveConcreteValidationEvidence,
	}

	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": input}, nil)

	if contract.State == convergenceStateNeedStateChange {
		t.Fatalf("contract = %+v, supplemental evidence item must not start primary state change without target evidence", contract)
	}
	required := mapFromValue(mapFromValue(contract.RepairActionPackage)["requiredOutput"])
	if boolValue(required["mustIncludePrimaryStateChangingNode"]) {
		t.Fatalf("requiredOutput = %+v, want no primary state-changing node for supplemental evidence focus", required)
	}
	focus := mapFromValue(mapFromValue(contract.RepairActionPackage)[coverageWorkItemContextKey])["currentFocus"]
	if role := stringFromMap(mapFromValue(focus), "evidenceRole"); role != "supplemental_evidence" {
		t.Fatalf("currentFocus.evidenceRole = %q, want supplemental_evidence", role)
	}
}

func TestRunExecutionInputCarriesNeutralRuntimeContractSignals(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-contract-signal",
		WorkspaceID: "ws-contract-signal",
		GoalID:      "goal-contract-signal",
		DoDID:       "dod-contract-signal",
		ExecutionContext: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"requestedOutputMode":         outputModeIncrementalArtifact,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode": true,
					"estimatedPatchContentBytes":   defaultMaxPatchContentBytes + 1,
				},
			},
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}

	input := initialStepInput(run)

	if !boolValue(input["stateChangingOutputRequired"]) ||
		stringFromMap(input, "repairObjective") != repairObjectiveStateChangingOutput ||
		stringFromMap(input, "requestedOutputMode") != outputModeIncrementalArtifact {
		t.Fatalf("input contract signals = %+v", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		intFromAny(required["estimatedPatchContentBytes"]) <= defaultMaxPatchContentBytes {
		t.Fatalf("repairActionPackage.requiredOutput = %+v", required)
	}
	if stringFromMap(mapFromValue(input["executionContract"]), "stateChangePolicy") != "must_produce_state_change" {
		t.Fatalf("executionContract = %+v", input["executionContract"])
	}
}

func TestEnforceStateChangingRequiredOutputPreservesNeutralOutputBudgetSignals(t *testing.T) {
	pkg := map[string]any{
		"requiredOutput": map[string]any{
			"mustIncludeStateChangingNode":       true,
			"estimatedPatchContentBytes":         defaultMaxPatchContentBytes + 1,
			"estimatedGraphBytes":                defaultMaxGraphBytes + 1,
			"expectedStateChangingArtifactCount": defaultMaxGraphFileWrites + 1,
		},
	}

	required := enforceStateChangingRequiredOutputForConvergence(pkg)

	if intFromAny(required["estimatedPatchContentBytes"]) != defaultMaxPatchContentBytes+1 ||
		intFromAny(required["estimatedGraphBytes"]) != defaultMaxGraphBytes+1 ||
		intFromAny(required["expectedStateChangingArtifactCount"]) != defaultMaxGraphFileWrites+1 {
		t.Fatalf("requiredOutput = %+v, want output budget signals preserved", required)
	}
}

func TestInitialStepOutputModeSelectionRestoresBudgetSignalsWithProfileStateChange(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-contract-profile-signal",
		WorkspaceID: "ws-contract-profile-signal",
		GoalID:      "goal-contract-profile-signal",
		DoDID:       "dod-contract-profile-signal",
		ExecutionContext: map[string]any{
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"requestedOutputMode":         outputModeIncrementalArtifact,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode":       true,
					"estimatedPatchContentBytes":         defaultMaxPatchContentBytes + 1,
					"expectedStateChangingArtifactCount": 1,
				},
			},
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
				"profileContracts": map[string]any{
					"neutral": map[string]any{
						"deliverableExpectations": []map[string]any{
							{
								"id":          "source_change",
								"required":    true,
								"description": "A neutral deliverable must be changed through runtime-managed artifact output.",
							},
						},
					},
				},
			},
		},
	}

	input := initialStepInput(run)
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}
	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, run, input, nil, 0)
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])

	if intFromAny(required["estimatedPatchContentBytes"]) <= defaultMaxPatchContentBytes ||
		intFromAny(required["expectedStateChangingArtifactCount"]) != 1 {
		t.Fatalf("requiredOutput = %+v, want output budget signals preserved with profile state change", required)
	}
}

func TestConvergencePreservesRequestedIncrementalArtifactStateChangeWhenCoverageNeedsValidation(t *testing.T) {
	run := &runmodel.Run{
		ID:          "run-convergence-incremental-artifact",
		WorkspaceID: "ws-convergence-incremental-artifact",
		ExecutionContext: map[string]any{
			"requestedOutputMode":         outputModeIncrementalArtifact,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"mustIncludeStateChangingNode":       true,
					"estimatedPatchContentBytes":         defaultMaxPatchContentBytes + 1,
					"expectedStateChangingArtifactCount": 1,
				},
			},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageRepair,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"requestedOutputMode":        outputModeIncrementalArtifact,
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-incremental-artifact",
				"status":                "open",
				"source":                "goal",
				"sourceScope":           "goal_leaf",
				"sourceRequirementId":   "goal-req-incremental-artifact",
				"targetText":            "produce a neutral incremental artifact deliverable",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{"validation"},
			},
		},
	}
	orch := &RuntimeOrchestrator{policy: NormalizeRuntimePolicy(RuntimePolicy{})}
	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, run, input, nil, 0)

	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": input}, nil)
	if contract.State != convergenceStateNeedStateChange {
		t.Fatalf("contract state = %q, want %q; contract=%+v", contract.State, convergenceStateNeedStateChange, contract)
	}
	input = ApplyConvergenceContract(input, contract)
	applyLLMOutputModeSelection(context.Background(), orch.policy.LLMOutput, orch.evidence.ArtifactContinuations, run, input, nil, 0)

	if outputModeFromInput(input) != outputModeIncrementalArtifact {
		t.Fatalf("outputMode = %v, want incrementalArtifact; input=%+v", input["outputMode"], input)
	}
	if input["stateChangingOutputRequired"] != true ||
		stringFromMap(input, "repairObjective") != repairObjectiveStateChangingOutput {
		t.Fatalf("state-changing contract = %+v, want preserved artifact state-change contract", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["outputMode"] != outputModeIncrementalArtifact ||
		required["mustReturnArtifactPlan"] != true ||
		intFromAny(required["estimatedPatchContentBytes"]) <= defaultMaxPatchContentBytes ||
		intFromAny(required["expectedStateChangingArtifactCount"]) != 1 {
		t.Fatalf("requiredOutput = %+v, want preserved incremental artifact output contract", required)
	}
	if contract := mapFromValue(input["artifactOutputContract"]); contract["phase"] != artifactOutputPhasePlan {
		t.Fatalf("artifactOutputContract = %+v, want artifact_plan", contract)
	}
}

func TestBuildConvergenceContractNeedValidationWhenPatchAlreadySatisfied(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"currentTargetAlreadySatisfiesFailedPatch": []map[string]any{{
				"targetRef": "target.txt",
				"state":     "satisfied",
			}},
			"targetSnapshots": []map[string]any{{
				"targetRef":   "target.txt",
				"exists":      true,
				"contentHash": "hash-target",
			}},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": input}, nil)

	if contract.State != convergenceStateNeedValidation {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedValidation)
	}
}

func TestBuildConvergenceContractNeedValidationCarriesScratchContract(t *testing.T) {
	input := map[string]any{
		"workspaceId":                 "ws-convergence-scratch",
		"runId":                       "run-convergence-scratch",
		"failedStepId":                "run-convergence-scratch/repair-001",
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"currentTargetAlreadySatisfiesFailedPatch": []map[string]any{{
				"targetRef": "target.txt",
				"state":     "satisfied",
			}},
			"targetSnapshots": []map[string]any{{
				"targetRef":   "target.txt",
				"exists":      true,
				"contentHash": "hash-target",
			}},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)
	if contract.State != convergenceStateNeedValidation {
		t.Fatalf("contract state = %q, want %q", contract.State, convergenceStateNeedValidation)
	}
	contractRequired := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	contractScratch := stringFromMap(contractRequired, validationScratchDirKey)
	if contractScratch == "" {
		t.Fatalf("contract requiredOutput = %+v, want validationScratchDir", contractRequired)
	}

	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	pkg := mapFromValue(applied["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	if stringFromMap(applied, validationScratchDirKey) != contractScratch ||
		stringFromMap(pkg, validationScratchDirKey) != contractScratch ||
		stringFromMap(required, validationScratchDirKey) != contractScratch {
		t.Fatalf("scratch contract not mirrored applied=%q pkg=%q required=%q want %q", stringFromMap(applied, validationScratchDirKey), stringFromMap(pkg, validationScratchDirKey), stringFromMap(required, validationScratchDirKey), contractScratch)
	}
	if !strings.Contains(stringFromMap(required, "controlledSubstituteScratchRule"), validationScratchDirKey) {
		t.Fatalf("requiredOutput = %+v, want scratch rule", required)
	}
	if applied["stateChangingOutputRequired"] != nil || applied["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("applied input = %+v, want validation repair contract", applied)
	}
}

func TestRuntimeRepairFollowUpSchemaInvalidKeepsStateChangingContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-schema-invalid",
		WorkspaceID:     "ws-schema-invalid",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-schema-invalid/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			stepInputKeySoulStage:         soulStageExecution,
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":   "target.txt",
					"exists":      true,
					"contentHash": "hash-target",
				}},
			},
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|stage=llm_schema_invalid",
			"error":                   "invalid llm decision json",
		},
	}
	orch := &RuntimeOrchestrator{}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, nil)
	contract := mapFromValue(input["convergenceContract"])

	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change for schema invalid repair", contract)
	}
}

func TestRuntimeRepairFollowUpSchemaInvalidWithRejectedContinuationKeepsStateChangingContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-schema-invalid-rejected-continuation",
		WorkspaceID:     "ws-schema-invalid-rejected-continuation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"rejectedResultContinuation": map[string]any{
				"coverageGaps": map[string]any{
					"missingGoalCoverage": []map[string]any{{
						"goalRequirementId": "goal-req-1",
						"raw":               "coverage gap",
					}},
				},
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-schema-invalid-rejected-continuation/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			stepInputKeySoulStage:         soulStageExecution,
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"coverageTargetSet": []map[string]any{{
				"targetText": "source_change",
				"kind":       "deliverable",
				"state":      "failed",
			}},
			coverageWorkItemContextKey: map[string]any{
				"currentFocus": map[string]any{
					"id":                  "cwi-source-change",
					"sourceScope":         "deliverable",
					"sourceRequirementId": "source_change",
					"evidenceRole":        "primary_state_change",
					"missingEvidenceKinds": []string{
						"validation",
						"state_change",
					},
					"status": "open",
				},
			},
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"coverageTargetSet": []map[string]any{{
					"targetText": "source_change",
					"kind":       "deliverable",
					"state":      "failed",
				}},
			},
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|stage=llm_schema_invalid",
			"error":                   "invalid llm decision json",
		},
	}
	orch := &RuntimeOrchestrator{}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, nil)
	contract := mapFromValue(input["convergenceContract"])

	if stringFromMap(contract, "state") != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want validation repair instead of targetless state-changing repair", contract)
	}
	if input["stateChangingOutputRequired"] != nil || input["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repair input = %+v, want rejected continuation to avoid targetless state-changing repair", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustProduceConcreteValidationEvidence"]) ||
		boolValue(required["mustIncludeStateChangingNode"]) ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want non-state-changing validation repair contract", required)
	}
}

func TestRuntimeRepairFollowUpProviderNormalizedInvalidKeepsStateChangingContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-provider-normalized-invalid",
		WorkspaceID:     "ws-provider-normalized-invalid",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-provider-normalized-invalid/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			"stateChangingOutputRequired": true,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"repairActionPackage": map[string]any{
				"requiredOutput": stateChangingRepairRequiredOutput(),
				"targetSnapshots": []map[string]any{{
					"targetRef":   "target.txt",
					"exists":      true,
					"contentHash": "hash-target",
				}},
			},
		},
		Output: map[string]any{
			"stage":                   "llm_decision_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|stage=llm_decision_invalid",
			"error":                   "provider returned normalized output that does not satisfy the decision contract",
		},
	}
	orch := &RuntimeOrchestrator{}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, nil)
	contract := mapFromValue(input["convergenceContract"])

	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change for provider-normalized invalid output", contract)
	}
	if input["stateChangingOutputRequired"] != true || input["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("repair input = %+v, want provider-normalized invalid to stay on state-changing repair contract", input)
	}
}

func TestRuntimeRepairFollowUpAssessmentSchemaInvalidKeepsAssessmentContract(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-assessment-schema-invalid-contract",
		WorkspaceID:     "ws-assessment-schema-invalid-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	failed := &runmodel.Step{
		ID:    "run-assessment-schema-invalid-contract/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:          runtimeStageVerification,
			stepInputKeyRuntimeRepairFollowUp: true,
			"repairObjective":                 "produce_concrete_validation_evidence",
			"boundedValidationRequired":       true,
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-1",
				"raw":               "coverage gap",
			}},
			"missingDeliverableCoverage": []map[string]any{{
				"id":    "validation_report",
				"state": "missing",
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid|error=invalid argument: invalid llm decision json: no json object found",
			"error":                   "invalid argument: invalid llm decision json: no json object found",
			"rawContentPreview":       "{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"goalCoverage\":{\"items\":[]},\"deliverableCoverage\":{\"items\":[]}}}",
		},
	}
	orch := &RuntimeOrchestrator{}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, errors.New("invalid llm decision json"))
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation for concrete validation schema invalid repair", contract)
	}
	if input[stepInputKeyAssessmentOnly] != nil ||
		input["repairObjective"] != repairObjectiveConcreteValidationEvidence ||
		input["boundedValidationRequired"] != true ||
		input["schemaInvalidRepairMode"] != "single_valid_json_decision" {
		t.Fatalf("repair input = %+v, want concrete validation schema invalid contract", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustProduceConcreteValidationEvidence"]) || !boolValue(required["mustReturnValidJSON"]) {
		t.Fatalf("requiredOutput = %+v, want valid-json concrete validation contract", required)
	}
}

func TestRuntimeRepairFollowUpToolFailureWithConcreteTargetNeedsStateChange(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-tool-failure-contract",
		WorkspaceID:     "ws-tool-failure-contract",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-tool-failure-contract/graph-001/002-call-tool",
		RunID: run.ID,
		Kind:  "mcp/tool_call",
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageExecution,
			stepInputKeyGraphNodeID:     "call_tool",
			stepInputKeyGraphRootStepID: string(run.ID) + "/step-1",
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "target.txt:7:1: target contract mismatch",
			failureOutputKeyClass:     failureClassMCPRuntime,
			failureOutputKeySignature: "class=mcp_runtime_failed|kind=mcp/tool_call|failureTargets=target.txt|node=call_tool",
		},
	}
	orch := &RuntimeOrchestrator{steps: memory.NewStepRepository()}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, nil)
	contract := mapFromValue(input["convergenceContract"])

	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change for concrete target tool failure", contract)
	}
	if input["stateChangingOutputRequired"] != true || input["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("repair input = %+v, want concrete target tool failure to require state-changing repair", input)
	}
}

func TestRuntimeRepairFollowUpToolLocatorInvalidNeedsEvidence(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-tool-locator-invalid",
		WorkspaceID:     "ws-tool-locator-invalid",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"stateChangePolicy": "must_produce_state_change",
			},
		},
	}
	failed := &runmodel.Step{
		ID:    "run-tool-locator-invalid/graph-001/002-call-tool",
		RunID: run.ID,
		Kind:  "mcp/tool_call",
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageExecution,
			stepInputKeyGraphNodeID:     "call_tool",
			stepInputKeyGraphRootStepID: string(run.ID) + "/step-1",
		},
		Output: map[string]any{
			"status":              "failed",
			"operation":           "mcp/tool_call",
			"serverId":            "semantic-capability",
			"tool":                "definition.lookup",
			"error":               "mcp tool definition.lookup failed: column is beyond end of line",
			failureOutputKeyClass: failureClassMCPRuntime,
			failureOutputKeySignature: "class=mcp_runtime_failed|kind=mcp/tool_call|stage=mcp_tool_call_failed|" +
				"serverId=semantic-capability|tool=definition.lookup|failureTargets=target.txt|error=column is beyond end of line",
			"input": map[string]any{
				"path": "target.txt",
				"line": 10,
				"col":  200,
			},
		},
	}
	orch := &RuntimeOrchestrator{steps: memory.NewStepRepository()}

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, failed, 1, errors.New("column is beyond end of line"))
	contract := mapFromValue(input["convergenceContract"])
	pkg := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])

	if stringFromMap(contract, "state") != convergenceStateNeedEvidence {
		t.Fatalf("convergenceContract = %+v, want need_evidence for invalid capability locator", contract)
	}
	if input["stateChangingOutputRequired"] != nil || input["repairObjective"] != repairObjectiveAcquireFailureEvidence {
		t.Fatalf("repair input = %+v, want evidence acquisition without state-changing output", input)
	}
	if required["mustProduceConcreteFailureEvidence"] != true || !stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "mcp/tool_call") {
		t.Fatalf("requiredOutput = %+v, want bounded capability evidence acquisition", required)
	}
	if boolValue(pkg["capabilityInputLocatorInvalid"]) != true {
		t.Fatalf("repairActionPackage = %+v, want locator invalid marker", pkg)
	}
}

func TestAssessmentOnlyContractCorrectionStepInputAppliesConvergenceContract(t *testing.T) {
	step := &runmodel.Step{
		ID:   "run-assessment-correction/step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyAssessmentOnly: true,
			"repairObjective":          "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req",
				"raw":               "coverage gap",
				"state":             "failed",
			}},
		},
	}

	input := assessmentOnlyContractCorrectionStepInput(step, map[string]any{
		"stage": "llm_dod_assessment",
		"error": "missing goal coverage",
	}, nil)

	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedAssessment {
		t.Fatalf("convergenceContract = %+v, want need_assessment for assessment-only correction", contract)
	}
	if input[stepInputKeyRuntimeStage] != runtimeStageVerification || input[stepInputKeyAssessmentOnly] != true {
		t.Fatalf("input = %+v, want verification assessment-only correction input", input)
	}
}

func TestBuildConvergenceContractReadsRejectedResultContinuationFromExecutionContext(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-rejected-result-continuation",
		WorkspaceID:     "ws-rejected-result-continuation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"rejectedResultContinuation": map[string]any{
				"predecessorRunId": "run-previous",
				"summary":          "coverage gaps remain",
				"requiredEvidence": []string{"main.go"},
			},
		},
	}
	step := &runmodel.Step{
		ID:    "run-rejected-result-continuation/repair-001",
		RunID: run.ID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
		},
	}

	contract := BuildConvergenceContract(run, step, step, map[string]any{"input": map[string]any{"executionContext": run.ExecutionContext}}, nil)
	if contract.State != convergenceStateNeedEvidence {
		t.Fatalf("convergenceContract = %+v, want need_evidence from rejected result continuation", contract)
	}
}

func TestBuildConvergenceContractRejectedResultSkipsHistoricalReplanPackage(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-rejected-skips-replan-package",
		WorkspaceID:     "ws-rejected-skips-replan-package",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"rejectedResultContinuation": map[string]any{
				"predecessorRunId": "run-latest",
				"requiredEvidence": []string{"collect current evidence"},
			},
			"replanContinuation": map[string]any{
				"historicalOnly": true,
				"repairActionPackage": map[string]any{
					"repairTargetSet": []string{"old_test.go"},
					"requiredOutput": map[string]any{
						"mustIncludeStateChangingNode": true,
					},
				},
			},
		},
	}
	input := map[string]any{
		"executionContext": map[string]any{
			"rejectedResultContinuation": run.ExecutionContext["rejectedResultContinuation"],
			"replanContinuation":         run.ExecutionContext["replanContinuation"],
		},
	}

	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": input}, nil)
	if contract.State != convergenceStateNeedEvidence || contract.StateChangingOutputRequired {
		t.Fatalf("convergenceContract = %+v, want rejected-result evidence acquisition", contract)
	}
	if targets := stringSliceFromValue(contract.RepairActionPackage["repairTargetSet"]); len(targets) > 0 {
		t.Fatalf("repairActionPackage = %+v, want historical replan target suppressed", contract.RepairActionPackage)
	}
}

func TestBuildConvergenceContractRejectedResultWithCoverageGapsNeedsValidation(t *testing.T) {
	run := &runmodel.Run{
		ID:              "run-rejected-coverage-validation",
		WorkspaceID:     "ws-rejected-coverage-validation",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"rejectedResultContinuation": map[string]any{
				"predecessorRunId": "run-prev",
				"unsatisfiedDoDItems": []map[string]any{{
					"item":  "verify external contract",
					"state": "skipped",
				}},
				"coverageGaps": map[string]any{
					"missingGoalCoverage": []map[string]any{{
						"goalRequirementId": "goal-req-1",
						"item":              "goal coverage remains open",
						"state":             "skipped",
					}},
				},
			},
		},
	}

	input := map[string]any{"executionContext": run.ExecutionContext}
	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": input}, nil)
	if contract.State != convergenceStateNeedValidation || contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("convergenceContract = %+v, want need_validation from rejected result coverage gaps", contract)
	}
	applied := ApplyConvergenceContract(input, contract)
	required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "shell/command") {
		t.Fatalf("requiredOutput = %+v, want validation evidence contract", required)
	}
	if applied[stepInputKeyRuntimeStage] != runtimeStageVerification || applied[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("applied stages = runtime:%v soul:%v, want verification/validation", applied[stepInputKeyRuntimeStage], applied[stepInputKeySoulStage])
	}
	targets := mapSliceFromValue(applied["coverageTargetSet"])
	if len(targets) != 2 {
		t.Fatalf("applied = %+v, coverageTargetSet = %+v, want DoD + Goal targets inherited from rejected result", applied, targets)
	}
}

func TestBuildConvergenceContractValidationAssessmentGapStaysValidationWhenTargetMissing(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:    runtimeStageVerification,
		"repairObjective":           repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired": true,
		"evidenceRepairRequired":    true,
		"missingDoDItems": []map[string]any{{
			"item":  "runtime behavior remains unproven",
			"state": "failed",
		}},
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-validation-gap",
				"status":                "open",
				"sourceScope":           "dod_item",
				"sourceRequirementId":   "dod-validation-gap",
				"targetText":            "runtime behavior remains unproven",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{"validation"},
			},
		},
		"repairActionPackage": map[string]any{
			"coverageRepairRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":       "dod",
				"targetText": "runtime behavior remains unproven",
				"state":      "failed",
			}},
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	failed := &runmodel.Step{
		ID:    "run-validation-gap/repair-001",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: cloneStepPayload(input),
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "coverage gap remains after concrete validation attempt",
				"items": []map[string]any{{
					"item":        "runtime behavior remains unproven",
					"state":       "failed",
					"reason":      "no current evidence closes the target",
					"nextActions": []string{"acquire bounded target evidence"},
				}},
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedValidation || contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("convergenceContract = %+v, want need_validation after failed concrete validation assessment", contract)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	required := mapFromValue(mapFromValue(applied["repairActionPackage"])["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "shell/command") {
		t.Fatalf("requiredOutput = %+v, want bounded validation evidence contract", required)
	}
}

func TestBuildConvergenceContractCoverageStateChangeAlreadySatisfiedNeedsValidation(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageRepair,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveStateChangingOutput,
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-state-change",
				"status":                "open",
				"sourceScope":           "goal_leaf",
				"sourceRequirementId":   "goal-req-state-change",
				"targetText":            "neutral target state change is complete but not validated",
				"requiredEvidenceKinds": []string{"state_change", "validation"},
				"missingEvidenceKinds":  []string{"state_change", "validation"},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"currentTargetAlreadySatisfiesFailedPatch": []map[string]any{{
				"targetRef": "target.txt",
				"state":     "satisfied",
			}},
			"targetSnapshots": []map[string]any{{
				"targetRef":        "target.txt",
				"exists":           true,
				"contentHash":      "hash-target",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedValidation || contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("convergenceContract = %+v, want validation after already-satisfied state change", contract)
	}
	if contract.StateChangingOutputRequired {
		t.Fatalf("convergenceContract = %+v, should not require another state-changing graph", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("requiredOutput = %+v, want validation-only contract without patch/apply", required)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied["stateChangingOutputRequired"] != nil || applied["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("applied input = %+v, want validation follow-up not repeated state change", applied)
	}
}

func TestBuildConvergenceContractAlreadySatisfiedOldTargetAllowsNewValidationFailureStateChange(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageRepair,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"currentTargetAlreadySatisfiesFailedPatch": []map[string]any{{
			"targetRef":         "old-target.txt",
			"patchType":         "text_replace",
			"safeForValidation": true,
			"mechanicalSignal":  true,
		}},
		"stateChangeAlreadySatisfiedByCurrentTarget": true,
		"lastFailureDiagnostics": map[string]any{
			"failureTargets": []string{"new-target.txt"},
			"kind":           "validation_failed",
		},
		"failureTargetRepairContext": []map[string]any{{
			"targetRef":        "new-target.txt",
			"exists":           true,
			"contentHash":      "hash-new-target",
			"contentComplete":  true,
			"safeForFileWrite": true,
		}},
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-new-target",
				"status":                "open",
				"sourceScope":           "goal_leaf",
				"sourceRequirementId":   "goal-req-new-target",
				"targetText":            "neutral target still has a concrete validation failure",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{"validation"},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"currentTargetAlreadySatisfiesFailedPatch": []map[string]any{{
				"targetRef":         "old-target.txt",
				"patchType":         "text_replace",
				"safeForValidation": true,
				"mechanicalSignal":  true,
			}},
			"stateChangeAlreadySatisfiedByCurrentTarget": true,
			"targetSnapshots": []map[string]any{{
				"targetRef":        "new-target.txt",
				"exists":           true,
				"contentHash":      "hash-new-target",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange || contract.RepairObjective != repairObjectiveStateChangingOutput {
		t.Fatalf("convergenceContract = %+v, want state-changing repair for new validation failure target", contract)
	}
	if !contract.StateChangingOutputRequired {
		t.Fatalf("convergenceContract = %+v, want state-changing output required", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want state-changing patch contract", required)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied["stateChangingOutputRequired"] != true || applied["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("applied input = %+v, want state-changing follow-up for new target", applied)
	}
}

func TestBuildConvergenceContractAlreadySatisfiedProfileStateChangeDoesNotOverrideValidation(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-already-satisfied",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                 "version_control_snapshot",
							"scope":              "deliverable",
							"evidenceRole":       "supplemental_evidence",
							"acceptedStepKinds":  []string{gitcommon.StepKindWrite},
							"acceptedOperations": []string{gitcommon.OperationCommit},
						}},
					},
				},
			},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		stepInputKeyRuntimeStageMode:  "strict_convergence",
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"currentTargetAlreadySatisfiesFailedPatch": []map[string]any{{
			"targetRef":         "target.txt",
			"patchType":         "text_replace",
			"safeForValidation": true,
			"mechanicalSignal":  true,
		}},
		"stateChangeAlreadySatisfiedByCurrentTarget": true,
		"coverageTargetSet": []map[string]any{{
			"kind":                   "deliverable",
			"source":                 "deliverable",
			"sourceRequirementId":    "version_control_snapshot",
			"coverageTargetToken":    "version_control_snapshot",
			"targetText":             "version_control_snapshot",
			"requiredEvidenceKinds":  []string{"state_change", "validation"},
			"missingEvidenceKinds":   []string{"validation"},
			"acceptedEvidenceByKind": map[string]any{"state_change": []string{"step-state-change"}},
			"acceptedEvidenceBindingByKind": map[string]any{
				"state_change": []string{"step-state-change"},
			},
			"state": "in_progress",
		}},
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-validation",
				"status":                "open",
				"sourceScope":           "goal_leaf",
				"sourceRequirementId":   "goal-req-validation",
				"targetText":            "neutral goal validation remains missing",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{"validation"},
			},
			"currentFocusItems": []map[string]any{{
				"id":                    "cwi-state-change-profile",
				"status":                "open",
				"sourceScope":           "deliverable",
				"sourceRequirementId":   "version_control_snapshot",
				"targetText":            "version_control_snapshot",
				"requiredEvidenceKinds": []string{"state_change", "validation"},
				"missingEvidenceKinds":  []string{"validation"},
				"acceptedEvidenceByKind": map[string]any{
					"state_change": []string{"step-state-change"},
				},
				"acceptedEvidenceBindingByKind": map[string]any{
					"state_change": []string{"step-state-change"},
				},
				"evidenceRole": "supplemental_evidence",
			}},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": stateChangingRepairRequiredOutput(),
			"currentTargetAlreadySatisfiesFailedPatch": []map[string]any{{
				"targetRef":         "target.txt",
				"patchType":         "text_replace",
				"safeForValidation": true,
				"mechanicalSignal":  true,
			}},
			"stateChangeAlreadySatisfiedByCurrentTarget": true,
			"targetSnapshots": []map[string]any{{
				"targetRef":        "target.txt",
				"exists":           true,
				"contentHash":      "hash-target",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
		},
	}

	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedValidation || contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("convergenceContract = %+v, want need_validation despite profile state-change evidence requirement", contract)
	}
	if contract.StateChangingOutputRequired {
		t.Fatalf("convergenceContract = %+v, should not require another state-changing graph", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("requiredOutput = %+v, want validation-only contract", required)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied["stateChangingOutputRequired"] != nil || applied["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("applied input = %+v, want validation follow-up not repeated state change", applied)
	}
}

func TestBuildConvergenceContractAcceptedProfileStateChangeNeedsValidationOnly(t *testing.T) {
	run := &runmodel.Run{
		ID: "run-convergence-profile-accepted-state-change",
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"neutral-profile": map[string]any{
						"evidenceRequirements": []map[string]any{{
							"id":                 "version_control_snapshot",
							"scope":              "deliverable",
							"evidenceRole":       "supplemental_evidence",
							"acceptedStepKinds":  []string{gitcommon.StepKindWrite},
							"acceptedOperations": []string{gitcommon.OperationCommit},
						}},
					},
				},
			},
		},
	}
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageRepair,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"coverageTargetSet": []map[string]any{{
			"kind":                   "deliverable",
			"source":                 "deliverable",
			"sourceRequirementId":    "version_control_snapshot",
			"coverageTargetToken":    "version_control_snapshot",
			"targetText":             "version_control_snapshot",
			"requiredEvidenceKinds":  []string{"state_change", "validation"},
			"missingEvidenceKinds":   []string{"validation"},
			"acceptedEvidenceByKind": map[string]any{"state_change": []string{"step-state-change"}},
			"acceptedEvidenceRefs":   []string{"step-state-change"},
			"state":                  "in_progress",
		}},
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                     "cwi-profile-validation",
				"status":                 "open",
				"sourceScope":            "deliverable",
				"sourceRequirementId":    "version_control_snapshot",
				"targetText":             "version_control_snapshot",
				"requiredEvidenceKinds":  []string{"state_change", "validation"},
				"missingEvidenceKinds":   []string{"validation"},
				"acceptedEvidenceByKind": map[string]any{"state_change": []string{"step-state-change"}},
				"acceptedEvidenceRefs":   []string{"step-state-change"},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}

	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedValidation || contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("convergenceContract = %+v, want need_validation for accepted profile state-change evidence", contract)
	}
	if contract.StateChangingOutputRequired {
		t.Fatalf("convergenceContract = %+v, should not require another state-changing graph", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true || boolValue(required["mustIncludeStateChangingNode"]) {
		t.Fatalf("requiredOutput = %+v, want validation evidence contract", required)
	}
}

func TestBuildConvergenceContractFailedAssessmentWithValidationFocusStaysValidation(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageRepair,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired":  true,
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                           "cwi-validation",
				"status":                       "open",
				"sourceScope":                  "deliverable",
				"sourceRequirementId":          "source_change",
				"targetText":                   "source_change",
				"requiredEvidenceKinds":        []string{"state_change", "validation"},
				"missingEvidenceKinds":         []string{"validation"},
				"acceptedEvidenceByKind":       map[string]any{"state_change": []string{"step-state-change"}},
				"acceptedEvidenceRefs":         []string{"step-state-change"},
				"nextAllowedConvergenceStates": []string{convergenceStateNeedValidation},
			},
		},
		"coverageTargetSet": []map[string]any{{
			"kind":                   "deliverable",
			"source":                 "deliverable",
			"sourceRequirementId":    "source_change",
			"coverageTargetToken":    "source_change",
			"targetText":             "source_change",
			"requiredEvidenceKinds":  []string{"state_change", "validation"},
			"missingEvidenceKinds":   []string{"validation"},
			"acceptedEvidenceByKind": map[string]any{"state_change": []string{"step-state-change"}},
			"acceptedEvidenceRefs":   []string{"step-state-change"},
			"state":                  "in_progress",
		}},
		"repairActionPackage": map[string]any{
			"coverageRepairRequired": true,
			"coverageTargetSet": []map[string]any{{
				"kind":                   "deliverable",
				"sourceRequirementId":    "source_change",
				"targetText":             "source_change",
				"requiredEvidenceKinds":  []string{"state_change", "validation"},
				"missingEvidenceKinds":   []string{"validation"},
				"acceptedEvidenceByKind": map[string]any{"state_change": []string{"step-state-change"}},
			}},
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	failed := &runmodel.Step{
		ID:    "run-validation-focus/repair-001",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: cloneStepPayload(input),
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "validation evidence still missing",
			},
		},
		State: common.StepStateFailed,
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedValidation || contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("convergenceContract = %+v, want need_validation for focused validation gap", contract)
	}
	if contract.ConcreteFailureEvidenceRequired || contract.StateChangingOutputRequired {
		t.Fatalf("convergenceContract = %+v, should not switch to evidence acquisition or state change", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "file/read") {
		t.Fatalf("requiredOutput = %+v, want bounded validation evidence contract", required)
	}
}

func TestBuildConvergenceContractValidationReadOnlyContractLoopNeedsEvidenceWhenTargetMissing(t *testing.T) {
	input := map[string]any{
		stepInputKeyRuntimeStage:     runtimeStageVerification,
		stepInputKeyRuntimeStageMode: "strict_convergence",
		"repairObjective":            repairObjectiveConcreteValidationEvidence,
		"boundedValidationRequired":  true,
		"coverageWorkItemContext": map[string]any{
			"currentFocus": map[string]any{
				"id":                    "cwi-validation",
				"status":                "open",
				"sourceScope":           "dod_item",
				"sourceRequirementId":   "dod-validation",
				"targetText":            "neutral validation evidence remains missing",
				"requiredEvidenceKinds": []string{"validation"},
				"missingEvidenceKinds":  []string{"validation"},
			},
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
		},
	}
	failed := &runmodel.Step{
		ID:    "run-validation-read-only-loop/repair-001",
		Kind:  llmWorkspaceExecutionStepKind,
		Input: cloneStepPayload(input),
		Output: map[string]any{
			"stage":               "llm_execution_graph_plan",
			failureOutputKeyClass: failureClassRuntime,
			"error":               "invalid argument: validation follow-up llm step returned read-only executionGraph without concrete validation evidence",
		},
		State: common.StepStateFailed,
	}

	contract := BuildConvergenceContract(nil, nil, failed, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedValidation || contract.RepairObjective != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("convergenceContract = %+v, want validation contract after validation contract loop", contract)
	}
	if contract.ConcreteFailureEvidenceRequired || contract.StateChangingOutputRequired {
		t.Fatalf("convergenceContract = %+v, should not switch to evidence acquisition or state-changing output", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if required["mustProduceConcreteValidationEvidence"] != true ||
		boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "shell/command") {
		t.Fatalf("requiredOutput = %+v, want bounded validation evidence contract", required)
	}
	applied := ApplyConvergenceContract(cloneStepPayload(input), contract)
	if applied["repairObjective"] != repairObjectiveConcreteValidationEvidence ||
		applied["stateChangingOutputRequired"] != nil ||
		applied["boundedValidationRequired"] != true {
		t.Fatalf("applied input = %+v, want validation repair", applied)
	}
}

func TestBuildConvergenceContractStateChangeRequiresTargetSnapshotAcquisition(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
		},
		"repairActionPackage": map[string]any{
			"requiresCurrentTargetSnapshotForStateChange": true,
			"repairTargetSet": []string{
				"target.txt",
			},
			"requiredOutput": map[string]any{
				"mustUseCurrentTargetSnapshot": true,
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange ||
		contract.RuntimeStageMode != "target_snapshot_acquisition" ||
		contract.RepairObjective != repairObjectiveStateChangingOutput ||
		!contract.StateChangingOutputRequired ||
		stringFromMap(contract.Prerequisite, "kind") != "acquire_target_snapshot" {
		t.Fatalf("convergenceContract = %+v, want state_change with target snapshot prerequisite", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if required["mustAcquireCurrentTargetSnapshot"] != true ||
		required["mustUseReadOnlySnapshotAcquisition"] != true ||
		required["mustNotMutateWorkspace"] != true {
		t.Fatalf("requiredOutput = %+v, want read-only target snapshot acquisition contract", required)
	}
	if stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want no state-changing accepted kind during snapshot acquisition", required)
	}
}

func TestBuildConvergenceContractStateChangeDoesNotAcquireSnapshotForAbstractCoverageRefs(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
		},
		coverageWorkItemContextKey: map[string]any{
			"currentFocus": map[string]any{
				"id":                   "cwi-085",
				"coverageWorkItemIds":  []string{"cwi-085"},
				"missingEvidenceKinds": []string{"validation", "state_change"},
				"targetRefs":           []string{"cwi-085", "deliverable:deliverable:source_change", "source_change"},
				"structuredTargetRefs": []string{"cwi-085", "deliverable:deliverable:source_change", "source_change"},
			},
		},
		"repairActionPackage": map[string]any{
			"requiresCurrentTargetSnapshotForStateChange": true,
			"repairTargetSet": []string{
				"cwi-085",
				"deliverable:deliverable:source_change",
				"source_change",
				"goal.leaves[4]",
			},
			"requiredOutput": map[string]any{
				"mustUseCurrentTargetSnapshot": true,
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange ||
		contract.RuntimeStageMode == "target_snapshot_acquisition" ||
		stringFromMap(contract.Prerequisite, "kind") == "acquire_target_snapshot" {
		t.Fatalf("convergenceContract = %+v, want direct state_change without abstract target snapshot prerequisite", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) ||
		boolValue(required["stateChangingForbidden"]) ||
		boolValue(required["mustAcquireCurrentTargetSnapshot"]) {
		t.Fatalf("requiredOutput = %+v, want state-changing output allowed without abstract snapshot acquisition", required)
	}
}

func TestBuildConvergenceContractStateChangeUsesExistingTargetSnapshot(t *testing.T) {
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{
			"nextAction": map[string]any{"kind": "state_change"},
		},
		"repairActionPackage": map[string]any{
			"requiresCurrentTargetSnapshotForStateChange": true,
			"targetSnapshots": []map[string]any{{
				"targetRef":        "target.txt",
				"exists":           true,
				"contentHash":      "hash-target",
				"contentComplete":  true,
				"safeForFileWrite": true,
			}},
			"requiredOutput": map[string]any{
				"mustUseCurrentTargetSnapshot": true,
			},
		},
	}

	contract := BuildConvergenceContract(nil, nil, nil, map[string]any{"input": cloneStepPayload(input)}, nil)

	if contract.State != convergenceStateNeedStateChange ||
		contract.RepairObjective != repairObjectiveStateChangingOutput ||
		!contract.StateChangingOutputRequired {
		t.Fatalf("convergenceContract = %+v, want state_change with existing snapshot", contract)
	}
	required := mapFromValue(contract.RepairActionPackage["requiredOutput"])
	if required["mustAcquireCurrentTargetSnapshot"] == true {
		t.Fatalf("requiredOutput = %+v, want no acquisition requirement when snapshot exists", required)
	}
	if !boolValue(required["mustUseCurrentTargetSnapshot"]) {
		t.Fatalf("requiredOutput = %+v, want direct snapshot patch contract", required)
	}
}

func TestValidateConvergenceDispatchRejectsStaleEvidencePackageForStateChange(t *testing.T) {
	required := map[string]any{
		"mustIncludeStateChangingNode": true,
		"acceptedKinds":                []string{"patch/apply"},
	}
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeStateChange}},
		"convergenceContract": map[string]any{
			"state":                       convergenceStateNeedStateChange,
			"stateChangingOutputRequired": true,
		},
		"stateChangingOutputRequired": true,
		"requiredOutput":              cloneStepPayload(required),
		stepInputKeyStagePacket:       map[string]any{"requiredOutput": cloneStepPayload(required)},
		"repairActionPackage": map[string]any{
			"repairObjective":             repairObjectiveAcquireFailureEvidence,
			"evidenceAcquisitionRequired": true,
			"requiredOutput":              cloneStepPayload(required),
		},
	}

	if err := validateConvergenceDispatchInput(input); err == nil {
		t.Fatal("validateConvergenceDispatchInput succeeded, want stale evidence package rejection")
	}
}

func TestValidateConvergenceDispatchRejectsStaleStateChangePackageForValidation(t *testing.T) {
	required := concreteValidationEvidenceRequiredOutput(nil)
	input := map[string]any{
		convergenceAuthorityKey: map[string]any{"nextAction": map[string]any{"kind": runtimemodel.NextActionTypeValidate}},
		"convergenceContract": map[string]any{
			"state": convergenceStateNeedValidation,
		},
		"boundedValidationRequired": true,
		"requiredOutput":            cloneStepPayload(required),
		stepInputKeyStagePacket:     map[string]any{"requiredOutput": cloneStepPayload(required)},
		"repairActionPackage": map[string]any{
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"requiredOutput":              cloneStepPayload(required),
		},
	}

	if err := validateConvergenceDispatchInput(input); err == nil {
		t.Fatal("validateConvergenceDispatchInput succeeded, want stale state-change package rejection")
	}
}
