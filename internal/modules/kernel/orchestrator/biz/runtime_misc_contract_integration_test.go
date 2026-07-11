package orchestrator

import (
	"errors"
	"strings"
	"testing"

	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeFailureSignatureSeparatesRepeatedFailuresByTargetObject(t *testing.T) {
	runID := common.RunID("run-target-object-signature")
	stepFor := func(id string, stderr string) *runmodel.Step {
		step := &runmodel.Step{
			ID:    common.StepID(string(runID) + "/" + id),
			RunID: runID,
			Kind:  shellcommon.StepKindCommand,
			State: common.StepStateFailed,
			Input: map[string]any{stepInputKeyGraphNodeID: "run_tests"},
			Output: map[string]any{
				"status":   "failed",
				"command":  "go",
				"args":     []string{"test", "./..."},
				"exitCode": 1,
				"stderr":   stderr,
				"error":    "exit status 1",
			},
		}
		classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("exit status 1"))
		attachRuntimeFailureClassification(step.Output, classification)
		return step
	}

	configFailure := stepFor("config", "config/settings.yaml:10:2: missing required value")
	serverFailure := stepFor("policy", "policies/routing.rules:1:1: invalid rule expression")
	configAgain := stepFor("config-again", "./config/settings.yaml:99:1: different line, same target object")

	configSig := stringFromMap(configFailure.Output, failureOutputKeySignature)
	serverSig := stringFromMap(serverFailure.Output, failureOutputKeySignature)
	configAgainSig := stringFromMap(configAgain.Output, failureOutputKeySignature)
	if !strings.Contains(configSig, "failureTargets=config/settings.yaml") || !strings.Contains(serverSig, "failureTargets=policies/routing.rules") {
		t.Fatalf("signatures = config:%q policy:%q, want target-object failure signatures", configSig, serverSig)
	}
	if configSig == serverSig {
		t.Fatalf("signature = %q, want different signatures for different target objects", configSig)
	}
	if configSig != configAgainSig {
		t.Fatalf("same target object signatures differ: %q vs %q", configSig, configAgainSig)
	}
}

func duplicateShellFailureStep(runID common.RunID, suffix string) *runmodel.Step {
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/" + suffix),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{stepInputKeyGraphNodeID: "verify"},
		Output: map[string]any{
			"status":   "failed",
			"command":  "go",
			"args":     []string{"test", "./..."},
			"exitCode": 1,
			"stderr":   "same compiler error",
			"error":    "exit status 1",
		},
	}
	classification := classifyRuntimeFailure(step, step.Output, "runtime step failed", errors.New("exit status 1"))
	attachRuntimeFailureClassification(step.Output, classification)
	return step
}

func hasMemoryItem(items []*memorymodel.MemoryItem, runID common.RunID, memoryType string, summary string) bool {
	for _, item := range items {
		if item != nil &&
			item.RunID == runID &&
			item.SourceType == "run" &&
			item.SourceRef == string(runID) &&
			item.MemoryType == memoryType &&
			item.Summary == summary {
			return true
		}
	}
	return false
}

func hasSkillFeedbackMemory(items []*memorymodel.MemoryItem, runID common.RunID, skillID string) bool {
	for _, item := range items {
		if item != nil &&
			item.RunID == runID &&
			item.SourceType == "run" &&
			item.MemoryType == memorycommon.MemoryTypeSkillFeedback &&
			strings.Contains(item.Summary, skillID) {
			return true
		}
	}
	return false
}

func hasCompletionRunSummaryMemory(items []*memorymodel.MemoryItem, runID common.RunID, sourceRef common.StepID, summary string) bool {
	for _, item := range items {
		if item != nil &&
			item.RunID == runID &&
			item.SourceType == "run" &&
			item.SourceRef == string(sourceRef) &&
			item.MemoryType == memorycommon.MemoryTypeRunSummary &&
			item.Summary == summary &&
			strings.Contains(item.Content, "DoD items:") {
			return true
		}
	}
	return false
}

func hasRunSummaryContent(items []*memorymodel.MemoryItem, runID common.RunID, parts ...string) bool {
	for _, item := range items {
		if item == nil || item.RunID != runID || item.MemoryType != memorycommon.MemoryTypeRunSummary {
			continue
		}
		missing := false
		for _, part := range parts {
			if !strings.Contains(item.Content, part) {
				missing = true
				break
			}
		}
		if !missing {
			return true
		}
	}
	return false
}

func memoryContextContains(items []map[string]any, id string) bool {
	for _, item := range items {
		if item != nil && item["id"] == id {
			return true
		}
	}
	return false
}

func memoryIDsContain(items []common.MemoryID, id common.MemoryID) bool {
	for _, item := range items {
		if item == id {
			return true
		}
	}
	return false
}

func memoryItemsContainRun(items []*memorymodel.MemoryItem, runID common.RunID) bool {
	for _, item := range items {
		if item != nil && item.RunID == runID {
			return true
		}
	}
	return false
}

func runtimeMemoryWriteRequestsMust(t *testing.T, output map[string]any) []runtimeMemoryWriteRequest {
	t.Helper()
	requests, err := runtimeMemoryWriteRequests(output)
	if err != nil {
		t.Fatalf("runtimeMemoryWriteRequests: %v", err)
	}
	return requests
}

func memoryWriteRequestContains(items []runtimeMemoryWriteRequest, memoryType string, contentPart string) bool {
	for _, item := range items {
		if item.MemoryType == memoryType && strings.Contains(item.Content, contentPart) {
			return true
		}
	}
	return false
}

func TestCoverageRepairTokenMatchesNeutralPrefixesAndPartialTargets(t *testing.T) {
	if !coverageRepairTokenMatches("goal:goal-123", "goal-123") {
		t.Fatalf("expected goal: prefix to match bare goal id")
	}
	if !coverageRepairTokenMatches("功能准则 (Functional)： 基础转发", "功能准则 (Functional)： 基础转发：能够正确代理请求并获得预期回复。") {
		t.Fatalf("expected bounded partial coverage target to match full target text")
	}
	if coverageRepairTokenMatches("test", "technical test target") {
		t.Fatalf("short generic token should not match by substring")
	}
}

func TestRuntimeRepairTargetAdmissionRejectsRuntimeAndAbsoluteTargets(t *testing.T) {
	rejected := []string{
		"/home/user/workspace/main.go",
		"home/user/workorch/.workorch-workspaces/ws-1/main.go",
		".workorch-data/internal/run.json",
		"llmexecutiondecision.plan",
		"validationEvidence.mode",
		"validationevidence.observationType",
		"runs/run-1/request.json",
		"../escape.go",
	}
	for _, target := range rejected {
		if runtimeRepairTargetRefAdmissible(target) {
			t.Fatalf("runtimeRepairTargetRefAdmissible(%q) = true, want false", target)
		}
	}
	for _, target := range []string{"src/main.go", "gateway/config.yaml"} {
		if !runtimeRepairTargetRefAdmissible(target) {
			t.Fatalf("runtimeRepairTargetRefAdmissible(%q) = false, want true", target)
		}
	}
	signals := structuredFailureSignalsFromFailureOutput(map[string]any{
		"stderr": "no files in /home/user/workorch/.workorch-workspaces/ws-1",
	}, nil, failureClassShellCommand, "class=shell_command_failed|kind=shell/command|exitCode=1")
	if structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
		t.Fatalf("signals = %+v, want local absolute workspace path treated as diagnostic-only", signals)
	}
	signals = structuredFailureSignalsFromFailureOutput(map[string]any{
		"stdout": "gateway: ELF executable, interpreter /lib64/ld-linux-x86-64.so.2, not stripped",
	}, nil, failureClassShellCommand, "")
	if structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
		t.Fatalf("signals = %+v, want incidental command-output path treated as diagnostic-only", signals)
	}
	signals = structuredFailureSignalsFromFailureOutput(map[string]any{
		"stderr": "main.go:12: undefined: Gateway",
	}, nil, failureClassShellCommand, "")
	if !structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
		t.Fatalf("signals = %+v, want explicit line diagnostic treated as target evidence", signals)
	}
}

func TestProcessEvidenceDigestTreatsDoDCoverageMissingAsActionableGap(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-coverage/verify",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			"error":                   "invalid argument: dodAssessment is not satisfied",
		},
	}
	timeline := processEvidenceTimelineFromRunSteps(&runmodel.Run{ID: "run-coverage", WorkspaceID: "ws-coverage"}, []*runmodel.Step{step}, "")
	digest := currentProgressDigestFromProcessEvidenceTimeline(timeline)
	failure := mapFromValue(digest["lastObservedFailure"])
	if stringFromMap(failure, "evidenceValidity") != "actionable_gap" ||
		!strings.Contains(stringFromMap(digest, "currentGap"), "dodAssessment") {
		t.Fatalf("digest = %+v, want latest DoD coverage failure as actionable gap", digest)
	}
}
