package orchestrator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyWorkorchRuntimeCoreFlowScriptViolationFixtures(t *testing.T) {
	cases := []struct {
		name                 string
		steps                []map[string]any
		results              []map[string]any
		tickets              []map[string]any
		runState             string
		workspaceState       string
		workspaceActiveRunID string
		wantErr              string
		wantWarning          string
	}{
		{
			name: "unhandled invalid patch graph fails",
			steps: []map[string]any{
				invalidPatchGraphScriptStep("run-script-unhandled/step-1"),
			},
			wantErr: "unhandled violation: invalid executionGraph patch/apply",
		},
		{
			name: "handled invalid patch graph passes with warning",
			steps: []map[string]any{
				invalidPatchGraphScriptStep("run-script-handled/step-1"),
				{
					"id":    "run-script-handled/repair-001-of-step-1",
					"runId": "run-script-handled",
					"input": map[string]any{
						"runtimeRepairFollowUp": true,
						"repairOfStepId":        "run-script-handled/step-1",
						"recoveryAction":        "runtime_repair",
						"recoveryRootStepId":    "run-script-handled/step-1",
						"recoveryLineageType":   "runtime_repair",
						"recoveryBudgetScope":   map[string]any{"rootStepId": "run-script-handled/step-1"},
					},
				},
			},
			wantWarning: "warning: handled violation by runtime repair",
		},
		{
			name: "planning patch graph gated by llm node fails",
			steps: []map[string]any{
				gatedPatchGraphScriptStep("run-script-gated-patch/step-1"),
			},
			wantErr: "planning executionGraph state-changing node depends on llm/workspace_execution before state change",
		},
		{
			name: "provider retry of state changing repair loses contract fails",
			steps: []map[string]any{
				providerRetryRepairLostStateChangingScriptStep("run-script-provider-retry-repair-contract/provider-retry-001-of-repair-001-of-planning-001"),
			},
			wantErr: "provider retry of state-changing repair lost stateChangingOutputRequired",
		},
		{
			name:     "superseded repair exhausted rejected policy without result fails",
			runState: "superseded",
			steps: []map[string]any{{
				"id":    "run-script-repair-exhausted-no-result/repair-003-of-planning-001",
				"runId": "run-script-repair-exhausted-no-result",
				"input": map[string]any{
					"runtimeRepairFollowUp": true,
					"recoveryAction":        "runtime_repair",
					"recoveryRootStepId":    "run-script-repair-exhausted-no-result/step-1",
					"recoveryLineageType":   "runtime_repair",
					"recoveryBudgetScope":   map[string]any{"rootStepId": "run-script-repair-exhausted-no-result/step-1"},
				},
				"state": "failed",
			}},
			tickets: []map[string]any{{
				"id":     "run-script-repair-exhausted-no-result-repair-exhausted-decision",
				"runId":  "run-script-repair-exhausted-no-result",
				"stepId": "run-script-repair-exhausted-no-result/repair-003-of-planning-001",
				"state":  "resolved",
				"context": map[string]any{
					"recoveryPolicy": "runtime_repair_exhausted_requires_strategy",
					"resultPolicy":   "rejected",
				},
			}},
			wantErr: "run has rejected resultPolicy decision without rejected result",
		},
		{
			name: "legacy block graph node id fails",
			steps: []map[string]any{
				{
					"id":    "run-script-legacy-block/block/read-file",
					"runId": "run-script-legacy-block",
					"input": map[string]any{
						"graphNodeId":  "read-file",
						"graphSeq":     1,
						"graphNodeSeq": 1,
					},
				},
			},
			wantErr: "graph node step id is not graph-scoped",
		},
		{
			name: "missing graph sequence fails",
			steps: []map[string]any{
				{
					"id":    "run-script-missing-graph-seq/graph-001/001-read-file",
					"runId": "run-script-missing-graph-seq",
					"input": map[string]any{
						"graphNodeId":  "read-file",
						"graphNodeSeq": 1,
					},
				},
			},
			wantErr: "graph node step missing graphSeq",
		},
		{
			name: "provider retry graph context is recovery step",
			steps: []map[string]any{
				{
					"id":    "run-script-provider-retry-context/provider-retry-001-of-graph-001-002-implement",
					"runId": "run-script-provider-retry-context",
					"input": map[string]any{
						"providerRetryFollowUp":   true,
						"providerRetryAttempt":    1,
						"providerRetryRootStepId": "run-script-provider-retry-context/graph-001/002-implement",
						"recoveryAction":          "provider_retry",
						"recoveryRootStepId":      "run-script-provider-retry-context/graph-001/002-implement",
						"recoveryLineageType":     "provider_retry",
						"recoveryBudgetScope":     map[string]any{"rootStepId": "run-script-provider-retry-context/graph-001/002-implement"},
						"graphNodeId":             "implement",
					},
				},
			},
		},
		{
			name: "repeated missing file reads without patch fails",
			steps: []map[string]any{
				missingFileReadScriptStep("run-script-missing-files/graph-001/001-read-main", "main.go"),
				missingFileReadScriptStep("run-script-missing-files/graph-001/002-read-mod", "go.mod"),
				{
					"id":    "run-script-missing-files/repair-001-of-planning-001",
					"runId": "run-script-missing-files",
					"input": map[string]any{
						"runtimeRepairFollowUp": true,
						"repairOfStepId":        "run-script-missing-files/graph-001/001-read-main",
						"recoveryAction":        "runtime_repair",
						"recoveryRootStepId":    "run-script-missing-files/step-1",
						"recoveryLineageType":   "runtime_repair",
						"recoveryBudgetScope":   map[string]any{"rootStepId": "run-script-missing-files/step-1"},
					},
					"state": "failed",
					"output": map[string]any{
						"stage":        "llm_provider_timeout",
						"failureClass": "provider_timeout",
						"error":        "context deadline exceeded",
					},
				},
			},
			wantErr: "running DoD run repeats missing file reads without patch/result/ticket",
		},
		{
			name:     "workspace active run points to terminal run fails",
			runState: "superseded",
			steps: []map[string]any{{
				"id":    "run-script-terminal-active/step-1",
				"runId": "run-script-terminal-active",
				"input": map[string]any{},
			}},
			workspaceState:       "running",
			workspaceActiveRunID: "run-script-terminal-active",
			wantErr:              "workspace active run points to terminal run",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runID := scriptFixtureRunID(tc.steps)
			dataRoot := t.TempDir()
			writeRuntimeVerifyFixture(t, dataRoot, "ws-script-fixture", runID, tc.runState, tc.workspaceState, tc.workspaceActiveRunID, tc.steps, tc.results, tc.tickets)

			output, err := runRuntimeVerifyScript(t, dataRoot, "ws-script-fixture", runID)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(output, tc.wantErr) {
					t.Fatalf("verify output = %s err=%v, want error containing %q", output, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("verify output = %s err=%v, want success", output, err)
			}
			if tc.wantWarning != "" && !strings.Contains(output, tc.wantWarning) {
				t.Fatalf("verify output = %s, want warning %q", output, tc.wantWarning)
			}
		})
	}
}

func TestVerifyWorkorchRuntimeSoftwareDevProfileScriptFixtures(t *testing.T) {
	cases := []struct {
		name     string
		run      map[string]any
		results  []map[string]any
		wantErr  string
		wantSkip bool
		wantPass bool
	}{
		{
			name:     "generic profile skips",
			run:      profileScriptRun("run-profile-generic", "completed", nil),
			wantSkip: true,
		},
		{
			name:     "software dev accepted coverage passes",
			run:      profileScriptRun("run-profile-software-dev-pass", "completed", softwareDevProfileContractFixture()),
			results:  []map[string]any{softwareDevAcceptedResultFixture("run-profile-software-dev-pass", true)},
			wantPass: true,
		},
		{
			name:    "software dev missing deliverable coverage fails",
			run:     profileScriptRun("run-profile-software-dev-fail", "completed", softwareDevProfileContractFixture()),
			results: []map[string]any{softwareDevAcceptedResultFixture("run-profile-software-dev-fail", false)},
			wantErr: "accepted software-dev result missing deliverableCoverage ids: user_deliverable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dataRoot := t.TempDir()
			runID := stringFromMap(tc.run, "id")
			writeProfileVerifyFixture(t, dataRoot, "ws-profile-fixture", runID, tc.run, tc.results)

			output, err := runRuntimeProfileSoftwareDevVerifyScript(t, dataRoot, "ws-profile-fixture", runID)
			switch {
			case tc.wantErr != "":
				if err == nil || !strings.Contains(output, tc.wantErr) {
					t.Fatalf("verify output = %s err=%v, want error containing %q", output, err, tc.wantErr)
				}
			case tc.wantSkip:
				if err != nil || !strings.Contains(output, "skipped for non-software-dev run") {
					t.Fatalf("verify output = %s err=%v, want skip", output, err)
				}
			case tc.wantPass:
				if err != nil || !strings.Contains(output, "software-dev profile verification passed") {
					t.Fatalf("verify output = %s err=%v, want pass", output, err)
				}
			}
		})
	}
}

func invalidPatchGraphScriptStep(stepID string) map[string]any {
	runID := strings.Split(stepID, "/")[0]
	return map[string]any{
		"id":    stepID,
		"runId": runID,
		"input": map[string]any{
			"stateChangingOutputRequired": true,
		},
		"output": map[string]any{
			"executionGraph": map[string]any{
				"nodes": []map[string]any{
					{
						"id":   "write-main",
						"kind": "patch/apply",
						"input": map[string]any{
							"patchType": "file_write",
							"content":   "package main\n",
						},
					},
				},
			},
		},
	}
}

func profileScriptRun(runID string, state string, profile map[string]any) map[string]any {
	run := map[string]any{
		"id":          runID,
		"workspaceId": "ws-profile-fixture",
		"state":       state,
	}
	if profile != nil {
		run["executionContext"] = map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": profile,
				},
			},
		}
	}
	return run
}

func softwareDevProfileContractFixture() map[string]any {
	return map[string]any{
		"acceptanceDimensions": []map[string]any{{
			"id":       "functional_behavior",
			"required": true,
		}},
		"deliverableExpectations": []map[string]any{{
			"id":       "user_deliverable",
			"required": true,
		}},
		"evidenceRequirements": []map[string]any{{
			"id":       "runtime_execution",
			"required": true,
		}},
	}
}

func softwareDevAcceptedResultFixture(runID string, includeDeliverable bool) map[string]any {
	assessment := map[string]any{
		"satisfied": true,
		"summary":   "accepted",
		"items": []map[string]any{{
			"item":         "required outcome",
			"state":        "satisfied",
			"evidenceRefs": []string{runID + "/validation/runtime-step"},
		}},
		"goalCoverage": map[string]any{
			"satisfied": true,
			"items": []map[string]any{{
				"goal":         "goal covered",
				"state":        "satisfied",
				"evidenceRefs": []string{runID + "/validation/runtime-step"},
			}},
		},
		"acceptanceDimensions": map[string]any{
			"satisfied": true,
			"items": []map[string]any{{
				"id":           "functional_behavior",
				"state":        "satisfied",
				"evidenceRefs": []string{runID + "/validation/runtime-step"},
			}},
		},
	}
	if includeDeliverable {
		assessment["deliverableCoverage"] = map[string]any{
			"satisfied": true,
			"items": []map[string]any{{
				"id":           "user_deliverable",
				"state":        "satisfied",
				"artifactRefs": []string{runID + "/artifact/workspace-change-diff"},
			}},
		}
	}
	return map[string]any{
		"id":            runID + "/result/dod-assessment",
		"runId":         runID,
		"workspaceId":   "ws-profile-fixture",
		"state":         "accepted",
		"doDAssessment": assessment,
	}
}

func gatedPatchGraphScriptStep(stepID string) map[string]any {
	runID := strings.Split(stepID, "/")[0]
	return map[string]any{
		"id":    stepID,
		"runId": runID,
		"input": map[string]any{
			"runtimeStage": "planning",
		},
		"output": map[string]any{
			"executionGraph": map[string]any{
				"nodes": []map[string]any{
					{
						"id":   "analysis",
						"kind": "llm/workspace_execution",
					},
					{
						"id":        "write-main",
						"kind":      "patch/apply",
						"dependsOn": []string{"analysis"},
						"input": map[string]any{
							"patchType": "file_write",
							"targetRef": "main.go",
							"content":   "package main\n",
						},
					},
				},
			},
		},
	}
}

func providerRetryRepairLostStateChangingScriptStep(stepID string) map[string]any {
	runID := strings.Split(stepID, "/")[0]
	return map[string]any{
		"id":    stepID,
		"runId": runID,
		"state": "completed",
		"input": map[string]any{
			"providerRetryFollowUp":   true,
			"providerRetryAttempt":    1,
			"providerRetryRootStepId": runID + "/repair-001-of-planning-001",
			"recoveryAction":          "provider_retry",
			"recoveryRootStepId":      runID + "/repair-001-of-planning-001",
			"recoveryLineageType":     "provider_retry",
			"recoveryBudgetScope":     map[string]any{"rootStepId": runID + "/repair-001-of-planning-001"},
			"repairOfRootStepId":      runID + "/step-1",
			"repairOfStepId":          runID + "/step-1",
			"repairOfFailureClass":    "runtime_failed",
			"repairOfFailureSignature": "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|" +
				"error=invalid argument: planning executiongraph state-changing node depends on llm/workspace_execution before state change",
		},
		"output": map[string]any{
			"stage": "llm_execution_graph_plan",
			"executionGraph": map[string]any{
				"nodes": []map[string]any{{
					"id":   "implement",
					"kind": "llm/workspace_execution",
				}},
			},
		},
	}
}

func missingFileReadScriptStep(stepID string, target string) map[string]any {
	runID := strings.Split(stepID, "/")[0]
	return map[string]any{
		"id":    stepID,
		"runId": runID,
		"kind":  "file/read",
		"state": "failed",
		"input": map[string]any{
			"graphNodeId":  strings.TrimSuffix(strings.TrimPrefix(filepath.Base(stepID), "001-"), "-file"),
			"graphSeq":     1,
			"graphNodeSeq": 1,
		},
		"output": map[string]any{
			"stage":            "file_read_invalid_path",
			"status":           "failed",
			"failureClass":     "runtime_failed",
			"failureSignature": "class=runtime_failed|kind=file/read|stage=file_read_invalid_path|error=lstat /workspace/" + target + ": no such file or directory",
			"error":            "lstat /workspace/" + target + ": no such file or directory",
		},
	}
}

func scriptFixtureRunID(steps []map[string]any) string {
	for _, step := range steps {
		id := strings.TrimSpace(stringFromMap(step, "id"))
		if id == "" {
			continue
		}
		if idx := strings.Index(id, "/"); idx > 0 {
			return id[:idx]
		}
	}
	return "run-script-fixture"
}

func writeRuntimeVerifyFixture(t *testing.T, dataRoot, workspaceID, runID, runState, workspaceState, workspaceActiveRunID string, steps, results, tickets []map[string]any) {
	t.Helper()
	runDir := filepath.Join(dataRoot, "workspaces", workspaceID, "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if steps == nil {
		steps = []map[string]any{}
	}
	if results == nil {
		results = []map[string]any{}
	}
	if tickets == nil {
		tickets = []map[string]any{}
	}
	if runState == "" {
		runState = "running"
	}
	activeRunID := runID
	if workspaceState == "" && workspaceActiveRunID == "" && runState == "completed" {
		workspaceState = "completed"
		activeRunID = ""
	} else if workspaceState == "" && workspaceActiveRunID == "" && (runState == "failed" || runState == "cancelled" || runState == "superseded") {
		workspaceState = "failed"
		activeRunID = ""
	} else {
		if workspaceState == "" {
			workspaceState = "running"
		}
		if workspaceActiveRunID != "" {
			activeRunID = workspaceActiveRunID
		}
	}
	writeJSONFile(t, filepath.Join(dataRoot, "workspaces", workspaceID, "workspace.json"), []map[string]any{{
		"id":          workspaceID,
		"state":       workspaceState,
		"activeRunId": activeRunID,
	}})
	writeJSONFile(t, filepath.Join(runDir, "run.json"), []map[string]any{{
		"id":          runID,
		"workspaceId": workspaceID,
		"state":       runState,
		"doDId":       "dod-script-fixture",
	}})
	writeJSONFile(t, filepath.Join(runDir, "step.json"), steps)
	writeJSONFile(t, filepath.Join(runDir, "result.json"), results)
	writeJSONFile(t, filepath.Join(runDir, "ticket.json"), tickets)
}

func writeProfileVerifyFixture(t *testing.T, dataRoot, workspaceID, runID string, run map[string]any, results []map[string]any) {
	t.Helper()
	runDir := filepath.Join(dataRoot, "workspaces", workspaceID, "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir profile fixture: %v", err)
	}
	if results == nil {
		results = []map[string]any{}
	}
	writeJSONFile(t, filepath.Join(dataRoot, "workspaces", workspaceID, "workspace.json"), []map[string]any{{
		"id":          workspaceID,
		"state":       "completed",
		"activeRunId": "",
	}})
	writeJSONFile(t, filepath.Join(runDir, "run.json"), []map[string]any{run})
	writeJSONFile(t, filepath.Join(runDir, "result.json"), results)
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runRuntimeVerifyScript(t *testing.T, dataRoot, workspaceID, runID string) (string, error) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "verify-workorch-runtime-core-flow.sh"), workspaceID, runID)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "WORKORCH_RUNTIME_VERIFY_DATA_ROOT="+dataRoot)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runRuntimeProfileSoftwareDevVerifyScript(t *testing.T, dataRoot, workspaceID, runID string) (string, error) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "verify-workorch-runtime-profile-software-dev.sh"), workspaceID, runID)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "WORKORCH_RUNTIME_VERIFY_DATA_ROOT="+dataRoot)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
