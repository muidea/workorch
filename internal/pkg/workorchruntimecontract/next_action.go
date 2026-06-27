package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const NextActionSchema = "workorch.next_action.v1"

const (
	NextActionTypeAssessGap             = "assess_gap"
	NextActionTypePlan                  = "plan"
	NextActionTypeAcquireTargetSnapshot = "acquire_target_snapshot"
	NextActionTypeStateChange           = "state_change"
	NextActionTypeValidate              = "validate"
	NextActionTypeAssessment            = "assessment"
	NextActionTypeRepair                = "repair"
	NextActionTypeRequestDecision       = "request_decision"
	NextActionTypeRequestApproval       = "request_approval"
	NextActionTypeComplete              = "complete"
	NextActionTypeFail                  = "fail"
)

const (
	ValidationRouteExistingEvidence = "validate_existing_evidence"
	ValidationRouteRuntimeAction    = "validate_runtime_action"
	ValidationRouteLLMGraph         = "validate_llm_graph"
	ValidationRouteManagedHarness   = "validate_managed_harness"
	ValidationRouteDeferred         = "validate_deferred"
	ValidationRouteRequestDecision  = "request_decision"
	ValidationRouteFail             = "fail"
)

type NextAction struct {
	ID                    common.NextActionID         `json:"id"`
	WorkspaceID           common.WorkspaceID          `json:"workspaceId"`
	RunID                 common.RunID                `json:"runId"`
	Type                  string                      `json:"type"`
	SubjectType           string                      `json:"subjectType,omitempty"`
	SubjectID             string                      `json:"subjectId,omitempty"`
	FocusCoverageItemIDs  []common.CoverageWorkItemID `json:"focusCoverageItemIds,omitempty"`
	RequiredOutput        map[string]any              `json:"requiredOutput,omitempty"`
	AllowedActions        []string                    `json:"allowedActions,omitempty"`
	ForbiddenActions      []string                    `json:"forbiddenActions,omitempty"`
	RequiredEvidenceKinds []string                    `json:"requiredEvidenceKinds,omitempty"`
	Stage                 string                      `json:"stage,omitempty"`
	StageMode             string                      `json:"stageMode,omitempty"`
	IdempotencyKey        string                      `json:"idempotencyKey"`
	Reason                string                      `json:"reason,omitempty"`
}

func (a NextAction) Normalized() NextAction {
	a.Type = normalizeNextActionType(a.Type)
	a.SubjectType = strings.TrimSpace(a.SubjectType)
	a.SubjectID = strings.TrimSpace(a.SubjectID)
	a.Stage = strings.TrimSpace(a.Stage)
	a.StageMode = strings.TrimSpace(a.StageMode)
	a.Reason = strings.TrimSpace(a.Reason)
	a.FocusCoverageItemIDs = compactCoverageWorkItemIDs(a.FocusCoverageItemIDs)
	a.AllowedActions = compactNextActionStrings(a.AllowedActions)
	a.ForbiddenActions = compactNextActionStrings(a.ForbiddenActions)
	a.RequiredEvidenceKinds = compactNextActionStrings(a.RequiredEvidenceKinds)
	a.IdempotencyKey = strings.TrimSpace(a.IdempotencyKey)
	if a.IdempotencyKey == "" {
		a.IdempotencyKey = StableNextActionIdempotencyKey(a)
	}
	if a.ID == "" {
		a.ID = StableNextActionID(a)
	}
	return a
}

func (a NextAction) HasScope() bool {
	return a.WorkspaceID != "" && a.RunID != ""
}

func StableNextActionID(action NextAction) common.NextActionID {
	key := StableNextActionIdempotencyKey(action)
	base := safeNextActionIDPart(action.Type)
	if base == "" {
		base = "next-action"
	}
	return common.NextActionID(base + "-" + shortNextActionDigest(key))
}

func StableNextActionIdempotencyKey(action NextAction) string {
	action.Type = normalizeNextActionType(action.Type)
	parts := []string{
		"next-action",
		string(action.WorkspaceID),
		string(action.RunID),
		action.Type,
		strings.TrimSpace(action.SubjectType),
		strings.TrimSpace(action.SubjectID),
		strings.TrimSpace(action.Stage),
		strings.TrimSpace(action.StageMode),
		strings.Join(coverageWorkItemIDsToStrings(compactCoverageWorkItemIDs(action.FocusCoverageItemIDs)), ","),
		strings.Join(compactNextActionStrings(action.RequiredEvidenceKinds), ","),
	}
	return strings.Join(parts, "|")
}

func normalizeNextActionType(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case NextActionTypeAssessGap,
		NextActionTypePlan,
		NextActionTypeAcquireTargetSnapshot,
		NextActionTypeStateChange,
		NextActionTypeValidate,
		NextActionTypeAssessment,
		NextActionTypeRepair,
		NextActionTypeRequestDecision,
		NextActionTypeRequestApproval,
		NextActionTypeComplete,
		NextActionTypeFail:
		return value
	default:
		return value
	}
}

func compactCoverageWorkItemIDs(values []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	if len(values) == 0 {
		return nil
	}
	seen := map[common.CoverageWorkItemID]bool{}
	ret := make([]common.CoverageWorkItemID, 0, len(values))
	for _, value := range values {
		value = common.CoverageWorkItemID(strings.TrimSpace(string(value)))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func coverageWorkItemIDsToStrings(values []common.CoverageWorkItemID) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			ret = append(ret, string(value))
		}
	}
	return ret
}

func compactNextActionStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func safeNextActionIDPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", "_", "-").Replace(value)
	value = strings.Trim(value, "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return value
}

func shortNextActionDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	digest := hex.EncodeToString(sum[:])
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}
