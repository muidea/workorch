package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func RuntimeCapabilityFromContext(raw map[string]any) readmodelcore.RuntimeCapability {
	id := readModelStringValue(raw["id"])
	return readmodelcore.RuntimeCapability{
		ID:             id,
		ServerID:       readModelStringValue(raw["serverId"]),
		Kind:           readModelStringValue(raw["kind"]),
		Name:           readModelStringValue(raw["name"]),
		DisplayName:    firstNonEmptyTimelineString(readModelStringValue(raw["displayName"]), readModelStringValue(raw["name"]), id),
		Description:    readModelStringValue(raw["description"]),
		Category:       readModelStringValue(raw["category"]),
		Tags:           readModelStringSliceValue(raw["tags"]),
		RiskLevel:      readModelStringValue(raw["riskLevel"]),
		Enabled:        readModelBoolValue(raw["enabled"]),
		ApprovalPolicy: readModelStringValue(raw["approvalPolicy"]),
		Status:         readModelStringValue(raw["status"]),
	}
}
