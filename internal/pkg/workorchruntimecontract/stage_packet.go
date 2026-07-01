package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const StagePacketSchema = "workorch.stage_packet.v1"

type StagePacketDefinition struct {
	Revision string        `json:"revision,omitempty"`
	GoalID   common.GoalID `json:"goalId,omitempty"`
	DoDID    common.DoDID  `json:"doDId,omitempty"`
}

type StagePacket struct {
	SchemaVersion         string                `json:"schemaVersion"`
	WorkspaceID           common.WorkspaceID    `json:"workspaceId"`
	RunID                 common.RunID          `json:"runId"`
	StepID                common.StepID         `json:"stepId,omitempty"`
	Definition            StagePacketDefinition `json:"definition,omitempty"`
	ActiveStage           string                `json:"activeStage,omitempty"`
	ActiveFocus           map[string]any        `json:"activeFocus,omitempty"`
	NextAction            *NextAction           `json:"nextAction,omitempty"`
	RequiredOutput        map[string]any        `json:"requiredOutput,omitempty"`
	AllowedActions        []string              `json:"allowedActions,omitempty"`
	ForbiddenActions      []string              `json:"forbiddenActions,omitempty"`
	RequiredEvidenceKinds []string              `json:"requiredEvidenceKinds,omitempty"`
	AcceptedEvidenceRefs  []string              `json:"acceptedEvidenceRefs,omitempty"`
	RejectedEvidenceRefs  []string              `json:"rejectedEvidenceRefs,omitempty"`
	CurrentFailure        map[string]any        `json:"currentFailure,omitempty"`
	TargetSnapshots       []map[string]any      `json:"targetSnapshots,omitempty"`
	IdempotencyKey        string                `json:"idempotencyKey"`
}

func (p StagePacket) Normalized() StagePacket {
	if strings.TrimSpace(p.SchemaVersion) == "" {
		p.SchemaVersion = StagePacketSchema
	}
	p.Definition.Revision = strings.TrimSpace(p.Definition.Revision)
	p.ActiveStage = strings.TrimSpace(p.ActiveStage)
	p.ActiveFocus = cloneStagePacketMap(p.ActiveFocus)
	p.RequiredOutput = cloneStagePacketMap(p.RequiredOutput)
	p.AllowedActions = compactNextActionStrings(p.AllowedActions)
	p.ForbiddenActions = compactNextActionStrings(p.ForbiddenActions)
	p.RequiredEvidenceKinds = compactNextActionStrings(p.RequiredEvidenceKinds)
	p.AcceptedEvidenceRefs = compactNextActionStrings(p.AcceptedEvidenceRefs)
	p.RejectedEvidenceRefs = compactNextActionStrings(p.RejectedEvidenceRefs)
	p.CurrentFailure = cloneStagePacketMap(p.CurrentFailure)
	p.TargetSnapshots = NormalizeStagePacketTargetSnapshots(p.TargetSnapshots)
	if p.NextAction != nil {
		next := p.NextAction.Normalized()
		if next.Type != "" {
			p.NextAction = &next
			if p.RequiredOutput == nil {
				p.RequiredOutput = cloneStagePacketMap(next.RequiredOutput)
			}
			if len(p.AllowedActions) == 0 {
				p.AllowedActions = append([]string(nil), next.AllowedActions...)
			}
			if len(p.ForbiddenActions) == 0 {
				p.ForbiddenActions = append([]string(nil), next.ForbiddenActions...)
			}
			if len(p.RequiredEvidenceKinds) == 0 {
				p.RequiredEvidenceKinds = append([]string(nil), next.RequiredEvidenceKinds...)
			}
			if p.ActiveStage == "" {
				p.ActiveStage = strings.TrimSpace(next.Stage)
			}
		} else {
			p.NextAction = nil
		}
	}
	p.IdempotencyKey = strings.TrimSpace(p.IdempotencyKey)
	if p.IdempotencyKey == "" {
		p.IdempotencyKey = StableStagePacketIdempotencyKey(p)
	}
	return p
}

func (p StagePacket) HasScope() bool {
	return p.WorkspaceID != "" && p.RunID != ""
}

func (p StagePacket) ToMap() map[string]any {
	p = p.Normalized()
	ret := map[string]any{
		"schemaVersion":  p.SchemaVersion,
		"workspaceId":    string(p.WorkspaceID),
		"runId":          string(p.RunID),
		"idempotencyKey": p.IdempotencyKey,
	}
	if p.StepID != "" {
		ret["stepId"] = string(p.StepID)
	}
	definition := map[string]any{}
	if p.Definition.Revision != "" {
		definition["revision"] = p.Definition.Revision
	}
	if p.Definition.GoalID != "" {
		definition["goalId"] = string(p.Definition.GoalID)
	}
	if p.Definition.DoDID != "" {
		definition["doDId"] = string(p.Definition.DoDID)
	}
	if len(definition) > 0 {
		ret["definition"] = definition
	}
	if p.ActiveStage != "" {
		ret["activeStage"] = p.ActiveStage
	}
	if len(p.ActiveFocus) > 0 {
		ret["activeFocus"] = cloneStagePacketMap(p.ActiveFocus)
	}
	if p.NextAction != nil {
		ret["nextAction"] = stagePacketNextActionMap(p.NextAction.Normalized())
	}
	if len(p.RequiredOutput) > 0 {
		ret["requiredOutput"] = cloneStagePacketMap(p.RequiredOutput)
	}
	if len(p.AllowedActions) > 0 {
		ret["allowedActions"] = append([]string(nil), p.AllowedActions...)
	}
	if len(p.ForbiddenActions) > 0 {
		ret["forbiddenActions"] = append([]string(nil), p.ForbiddenActions...)
	}
	if len(p.RequiredEvidenceKinds) > 0 {
		ret["requiredEvidenceKinds"] = append([]string(nil), p.RequiredEvidenceKinds...)
	}
	if len(p.AcceptedEvidenceRefs) > 0 {
		ret["acceptedEvidenceRefs"] = append([]string(nil), p.AcceptedEvidenceRefs...)
	}
	if len(p.RejectedEvidenceRefs) > 0 {
		ret["rejectedEvidenceRefs"] = append([]string(nil), p.RejectedEvidenceRefs...)
	}
	if len(p.CurrentFailure) > 0 {
		ret["currentFailure"] = cloneStagePacketMap(p.CurrentFailure)
	}
	if len(p.TargetSnapshots) > 0 {
		ret["targetSnapshots"] = cloneStagePacketMapSlice(p.TargetSnapshots)
	}
	return ret
}

func stagePacketNextActionMap(action NextAction) map[string]any {
	action = action.Normalized()
	if action.Type == "" {
		return nil
	}
	ret := map[string]any{
		"id":             string(action.ID),
		"workspaceId":    string(action.WorkspaceID),
		"runId":          string(action.RunID),
		"type":           action.Type,
		"idempotencyKey": action.IdempotencyKey,
	}
	if action.SubjectType != "" {
		ret["subjectType"] = action.SubjectType
	}
	if action.SubjectID != "" {
		ret["subjectId"] = action.SubjectID
	}
	if len(action.FocusCoverageItemIDs) > 0 {
		items := make([]string, 0, len(action.FocusCoverageItemIDs))
		for _, item := range action.FocusCoverageItemIDs {
			if item != "" {
				items = append(items, string(item))
			}
		}
		if len(items) > 0 {
			ret["focusCoverageItemIds"] = items
		}
	}
	if len(action.RequiredOutput) > 0 {
		ret["requiredOutput"] = cloneStagePacketMap(action.RequiredOutput)
	}
	if len(action.AllowedActions) > 0 {
		ret["allowedActions"] = append([]string(nil), action.AllowedActions...)
	}
	if len(action.ForbiddenActions) > 0 {
		ret["forbiddenActions"] = append([]string(nil), action.ForbiddenActions...)
	}
	if len(action.RequiredEvidenceKinds) > 0 {
		ret["requiredEvidenceKinds"] = append([]string(nil), action.RequiredEvidenceKinds...)
	}
	if action.Stage != "" {
		ret["stage"] = action.Stage
	}
	if action.StageMode != "" {
		ret["stageMode"] = action.StageMode
	}
	if action.Reason != "" {
		ret["reason"] = action.Reason
	}
	return ret
}

func StableStagePacketIdempotencyKey(packet StagePacket) string {
	nextActionKey := ""
	if packet.NextAction != nil {
		nextActionKey = packet.NextAction.Normalized().IdempotencyKey
	}
	parts := []string{
		"stage-packet",
		string(packet.WorkspaceID),
		string(packet.RunID),
		string(packet.StepID),
		strings.TrimSpace(packet.Definition.Revision),
		string(packet.Definition.GoalID),
		string(packet.Definition.DoDID),
		strings.TrimSpace(packet.ActiveStage),
		nextActionKey,
		shortStagePacketDigest(packet.ActiveFocus),
		shortStagePacketDigest(packet.RequiredOutput),
		strings.Join(compactNextActionStrings(packet.RequiredEvidenceKinds), ","),
		strings.Join(compactNextActionStrings(packet.AcceptedEvidenceRefs), ","),
		strings.Join(compactNextActionStrings(packet.RejectedEvidenceRefs), ","),
		shortStagePacketDigest(packet.CurrentFailure),
		shortStagePacketDigest(NormalizeStagePacketTargetSnapshots(packet.TargetSnapshots)),
	}
	return strings.Join(parts, "|")
}

func NormalizeStagePacketTargetSnapshots(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		next := normalizeStagePacketTargetSnapshot(item)
		if len(next) == 0 {
			continue
		}
		key := firstNonEmptyStagePacketString(next, "targetRef", "contentHash", "source", "contextSource", "error")
		if key == "" {
			key = shortStagePacketDigest(next)
		}
		if key != "" && seen[key] {
			continue
		}
		if key != "" {
			seen[key] = true
		}
		ret = append(ret, next)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizeStagePacketTargetSnapshot(item map[string]any) map[string]any {
	if len(item) == 0 {
		return nil
	}
	next := map[string]any{}
	for _, key := range []string{
		"targetRef",
		"exists",
		"contentHash",
		"contentComplete",
		"safeForFileWrite",
		"redacted",
		"truncated",
		"isBinary",
		"size",
		"source",
		"contextSource",
		"error",
	} {
		if value, ok := item[key]; ok && value != nil {
			next[key] = value
		}
	}
	if len(next) == 0 {
		return nil
	}
	integrity := stagePacketSnapshotIntegrity(next)
	next["integrityState"] = integrity
	next["safeForExactFileWrite"] = integrity == "full"
	if integrity != "full" {
		next["safeForFileWrite"] = false
	}
	return next
}

func stagePacketSnapshotIntegrity(item map[string]any) string {
	switch {
	case stagePacketBoolValue(item["redacted"]):
		return "redacted"
	case stagePacketBoolValue(item["truncated"]):
		return "truncated"
	case stagePacketBoolValue(item["contentComplete"]) &&
		stagePacketBoolValue(item["safeForFileWrite"]) &&
		!stagePacketBoolValue(item["isBinary"]):
		return "full"
	default:
		return "partial"
	}
}

func firstNonEmptyStagePacketString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stagePacketStringValue(item[key])); value != "" {
			return value
		}
	}
	return ""
}

func stagePacketStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func stagePacketBoolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}

func cloneStagePacketMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneStagePacketMapSlice(input []map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		if len(item) == 0 {
			continue
		}
		ret = append(ret, cloneStagePacketMap(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func shortStagePacketDigest(value any) string {
	if value == nil {
		return ""
	}
	payload, err := json.Marshal(value)
	if err != nil {
		payload = []byte(fmt.Sprintf("%#v", value))
	}
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}
