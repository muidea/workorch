package legacy

import (
	"context"
	"fmt"
	"path"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/legacy/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
)

type MinimalRuntimeAdapter struct{}

func NewMinimalRuntimeAdapter() *MinimalRuntimeAdapter { return &MinimalRuntimeAdapter{} }

func (a *MinimalRuntimeAdapter) ExecuteStep(ctx context.Context, runID common.RunID, stepID common.StepID) (map[string]any, error) {
	_ = ctx
	if strings.Contains(string(runID), modulecommon.RuntimeFailureRunToken) {
		return nil, fmt.Errorf("legacy runtime execution failed for %s", runID)
	}
	if strings.Contains(string(runID), modulecommon.MultiStepRunToken) {
		if path.Base(string(stepID)) == "step-1" {
			return map[string]any{
				"runId":    string(runID),
				"stepId":   string(stepID),
				"status":   "completed",
				"stage":    "prepare",
				"nextStep": fmt.Sprintf("%s/step-2", runID),
				"nextKind": "legacy/minimal",
			}, nil
		}
		return map[string]any{
			"runId":  string(runID),
			"stepId": string(stepID),
			"status": "completed",
			"stage":  "finalize",
		}, nil
	}
	if strings.Contains(string(runID), modulecommon.ConditionalBranchRunToken) {
		if path.Base(string(stepID)) == "step-1" {
			branch := "fast-track"
			nextStep := fmt.Sprintf("%s/step-fast", runID)
			if strings.Contains(string(runID), "branch-slow") {
				branch = "review-path"
				nextStep = fmt.Sprintf("%s/step-review", runID)
			}
			return map[string]any{
				"runId":    string(runID),
				"stepId":   string(stepID),
				"status":   "completed",
				"stage":    "route",
				"branch":   branch,
				"nextStep": nextStep,
				"nextKind": "legacy/minimal",
			}, nil
		}
		return map[string]any{
			"runId":  string(runID),
			"stepId": string(stepID),
			"status": "completed",
			"stage":  "finalize",
		}, nil
	}
	if strings.Contains(string(runID), modulecommon.MultiDecisionGateRunToken) {
		switch path.Base(string(stepID)) {
		case "step-1":
			return map[string]any{
				"runId":                 string(runID),
				"stepId":                string(stepID),
				"status":                "completed",
				"stage":                 "await-gate-1",
				"blockDecisionReason":   "workorch runtime gate 1",
				"blockDecisionTicketId": fmt.Sprintf("ticket-%s-gate-1", runID),
				"nextStep":              fmt.Sprintf("%s/step-2", runID),
				"nextKind":              "legacy/minimal",
			}, nil
		case "step-2":
			return map[string]any{
				"runId":                 string(runID),
				"stepId":                string(stepID),
				"status":                "completed",
				"stage":                 "await-gate-2",
				"blockDecisionReason":   "workorch runtime gate 2",
				"blockDecisionTicketId": fmt.Sprintf("ticket-%s-gate-2", runID),
				"nextStep":              fmt.Sprintf("%s/step-3", runID),
				"nextKind":              "legacy/minimal",
			}, nil
		default:
			return map[string]any{
				"runId":  string(runID),
				"stepId": string(stepID),
				"status": "completed",
				"stage":  "finalize",
			}, nil
		}
	}
	if strings.Contains(string(runID), modulecommon.MixedDecisionApprovalRunToken) {
		switch path.Base(string(stepID)) {
		case "step-1":
			return map[string]any{
				"runId":                 string(runID),
				"stepId":                string(stepID),
				"status":                "completed",
				"stage":                 "await-decision",
				"blockDecisionReason":   "workorch mixed decision gate",
				"blockDecisionTicketId": fmt.Sprintf("ticket-%s-mixed-decision", runID),
				"nextStep":              fmt.Sprintf("%s/step-2", runID),
				"nextKind":              "legacy/minimal",
			}, nil
		case "step-2":
			return map[string]any{
				"runId":               string(runID),
				"stepId":              string(stepID),
				"status":              "completed",
				"stage":               "await-approval",
				"blockApprovalReason": "workorch mixed approval gate",
				"blockApprovalId":     fmt.Sprintf("approval-%s-mixed-approval", runID),
				"nextStep":            fmt.Sprintf("%s/step-3", runID),
				"nextKind":            "legacy/minimal",
			}, nil
		default:
			return map[string]any{
				"runId":  string(runID),
				"stepId": string(stepID),
				"status": "completed",
				"stage":  "finalize",
			}, nil
		}
	}
	if strings.Contains(string(runID), modulecommon.RuntimeDecisionBlockRunToken) {
		if path.Base(string(stepID)) == "step-1" {
			return map[string]any{
				"runId":                 string(runID),
				"stepId":                string(stepID),
				"status":                "completed",
				"stage":                 "await-decision",
				"blockDecisionReason":   "workorch runtime decision gate",
				"blockDecisionTicketId": fmt.Sprintf("ticket-%s-runtime-decision", runID),
				"nextStep":              fmt.Sprintf("%s/step-2", runID),
				"nextKind":              "legacy/minimal",
			}, nil
		}
		return map[string]any{
			"runId":  string(runID),
			"stepId": string(stepID),
			"status": "completed",
			"stage":  "finalize",
		}, nil
	}
	if strings.Contains(string(runID), modulecommon.RuntimeApprovalBlockRunToken) {
		if path.Base(string(stepID)) == "step-1" {
			return map[string]any{
				"runId":               string(runID),
				"stepId":              string(stepID),
				"status":              "completed",
				"stage":               "await-approval",
				"blockApprovalReason": "workorch runtime approval gate",
				"blockApprovalId":     fmt.Sprintf("approval-%s-runtime-approval", runID),
				"nextStep":            fmt.Sprintf("%s/step-2", runID),
				"nextKind":            "legacy/minimal",
			}, nil
		}
		return map[string]any{
			"runId":  string(runID),
			"stepId": string(stepID),
			"status": "completed",
			"stage":  "finalize",
		}, nil
	}
	return map[string]any{
		"runId":  string(runID),
		"stepId": string(stepID),
		"status": "completed",
	}, nil
}
