package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const currentAuthoritySnapshotSchema = "workorch.current_authority_snapshot.v1"

type convergenceStateMachineSpec struct {
	NextAction    string
	ContractState string
}

var convergenceStateMachineSpecs = []convergenceStateMachineSpec{
	{NextAction: runtimemodel.NextActionTypeAcquireTargetSnapshot, ContractState: convergenceStateNeedEvidence},
	{NextAction: runtimemodel.NextActionTypeStateChange, ContractState: convergenceStateNeedStateChange},
	{NextAction: runtimemodel.NextActionTypeValidate, ContractState: convergenceStateNeedValidation},
	{NextAction: runtimemodel.NextActionTypeAssessment, ContractState: convergenceStateNeedAssessment},
	{NextAction: runtimemodel.NextActionTypeRepair, ContractState: convergenceStateNeedEvidence},
	{NextAction: runtimemodel.NextActionTypeRequestDecision, ContractState: convergenceStateNeedGovernance},
	{NextAction: runtimemodel.NextActionTypeRequestApproval, ContractState: convergenceStateNeedGovernance},
}

func convergenceContractStateForNextAction(nextAction string) (string, bool) {
	canonical := canonicalDispatchNextActionKind(nextAction)
	for _, spec := range convergenceStateMachineSpecs {
		if spec.NextAction == canonical {
			return spec.ContractState, true
		}
	}
	return "", false
}

func validateRequiredOutputStateMachineShape(input map[string]any, label string, required map[string]any, state string, nextAction string) error {
	if len(required) == 0 {
		return nil
	}
	state = strings.TrimSpace(state)
	nextAction = canonicalDispatchNextActionKind(nextAction)
	routes := requiredOutputSatisfiableRouteKinds(required)
	hasStateChangingRoute := requiredOutputRouteHasStateChangingKind(routes)
	switch {
	case nextAction == runtimemodel.NextActionTypeAcquireTargetSnapshot:
		if !requiredOutputIsReadOnlyTargetSnapshotPrerequisite(required) {
			appendAuthorityDiagnostic(input, "required_output_mismatch_next_action", "error", "internal_authority_mismatch", label, "acquire_target_snapshot requires read-only target snapshot acquisition requiredOutput")
			return convergenceDispatchMisalignmentError("required_output_mismatch_next_action: %s acquire_target_snapshot requires read-only requiredOutput", label)
		}
	case state == convergenceStateNeedEvidence || nextAction == runtimemodel.NextActionTypeRepair:
		if hasStateChangingRoute || requiredOutputRequiresStateChanging(required) {
			appendAuthorityDiagnostic(input, "required_output_mismatch_next_action", "error", "internal_authority_mismatch", label, "need_evidence must not expose workspace-mutating output")
			return convergenceDispatchMisalignmentError("required_output_mismatch_next_action: %s need_evidence exposes state-changing output", label)
		}
	case state == convergenceStateNeedStateChange || nextAction == runtimemodel.NextActionTypeStateChange:
		if targetSnapshotPrerequisiteActive(input) && requiredOutputIsReadOnlyTargetSnapshotPrerequisite(required) {
			return nil
		}
		if boolValue(required["stateChangingForbidden"]) {
			return nil
		}
		if !requiredOutputRequiresStateChanging(required) || !hasStateChangingRoute {
			appendAuthorityDiagnostic(input, "required_output_mismatch_next_action", "error", "internal_authority_mismatch", label, "need_state_change requires an allowed state-changing output route")
			return convergenceDispatchMisalignmentError("required_output_mismatch_next_action: %s need_state_change lacks state-changing output route", label)
		}
	case state == convergenceStateNeedValidation || nextAction == runtimemodel.NextActionTypeValidate:
		if requiredOutputRequiresStateChanging(required) || hasStateChangingRoute || requiredOutputRequiresConcreteFailureEvidence(required) {
			appendAuthorityDiagnostic(input, "required_output_mismatch_next_action", "error", "internal_authority_mismatch", label, "need_validation requires validation-only requiredOutput")
			return convergenceDispatchMisalignmentError("required_output_mismatch_next_action: %s need_validation is not validation-only", label)
		}
	case state == convergenceStateNeedAssessment || nextAction == runtimemodel.NextActionTypeAssessment:
		if requiredOutputRequiresStateChanging(required) || hasStateChangingRoute || requiredOutputRequiresConcreteFailureEvidence(required) {
			appendAuthorityDiagnostic(input, "required_output_mismatch_next_action", "error", "internal_authority_mismatch", label, "need_assessment requires assessment-only requiredOutput")
			return convergenceDispatchMisalignmentError("required_output_mismatch_next_action: %s need_assessment is not assessment-only", label)
		}
	}
	return nil
}

func sealCurrentAuthoritySnapshotLineage(input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	lineage := currentAuthoritySnapshotLineage(input)
	if len(lineage) == 0 {
		return input
	}
	id := strings.TrimSpace(stringFromMap(lineage, "id"))
	if id == "" {
		return input
	}
	input["currentAuthoritySnapshot"] = lineage
	input["authoritySnapshotId"] = id
	for _, key := range []string{"convergenceContract", "repairActionPackage", currentActionContractKey, stepInputKeyStagePacket} {
		value := mapFromValue(input[key])
		if len(value) == 0 {
			continue
		}
		value["authoritySnapshotId"] = id
		input[key] = value
	}
	packet := mapFromValue(input[stepInputKeyStagePacket])
	if next := mapFromValue(packet["nextAction"]); len(next) > 0 {
		next["authoritySnapshotId"] = id
		packet["nextAction"] = next
		packet["authoritySnapshotId"] = id
		input[stepInputKeyStagePacket] = packet
	}
	contract := mapFromValue(input[currentActionContractKey])
	if len(contract) > 0 {
		contract["authoritySnapshot"] = lineage
		input[currentActionContractKey] = contract
	}
	return input
}

func currentAuthoritySnapshotLineage(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	nextAction := strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind"))
	if nextAction == "" {
		nextAction = strings.TrimSpace(convergenceNextActionKind(nil, input))
	}
	contract := mapFromValue(input["convergenceContract"])
	state := strings.TrimSpace(stringFromMap(contract, "state"))
	if nextAction == "" && state == "" {
		return nil
	}
	required := activeRequiredOutputForCurrentAction(input)
	focus := dispatchAuthorityCurrentFocus(input)
	source := map[string]any{
		"authority":      currentAuthoritySourceForLineage(input),
		"contractState":  state,
		"nextActionKind": canonicalDispatchNextActionKind(nextAction),
		"requiredOutput": required,
		"focus":          focus,
	}
	id := "auth-" + shortAuthoritySnapshotDigest(stableCoverageJSON(source))
	lineage := map[string]any{
		"schemaVersion":      currentAuthoritySnapshotSchema,
		"id":                 id,
		"nextActionKind":     canonicalDispatchNextActionKind(nextAction),
		"convergenceState":   state,
		"requiredOutputHash": shortAuthoritySnapshotDigest(stableCoverageJSON(required)),
	}
	if focusID := strings.TrimSpace(stringFromMap(focus, "id")); focusID != "" {
		lineage["focusId"] = focusID
	}
	if ids := dispatchFocusWorkItemIDs(focus); len(ids) > 0 {
		lineage["coverageWorkItemIds"] = ids
	}
	if authority := currentAuthoritySourceForLineage(input); len(authority) > 0 {
		if revision := strings.TrimSpace(stringFromMap(authority, "definitionRevision")); revision != "" {
			lineage["definitionRevision"] = revision
		}
		if revision := strings.TrimSpace(stringFromMap(authority, "baselineRevision")); revision != "" {
			lineage["baselineRevision"] = revision
		}
	}
	return lineage
}

func currentAuthoritySourceForLineage(input map[string]any) map[string]any {
	for _, source := range []map[string]any{
		convergenceAuthorityMapFromValue(input[convergenceAuthorityKey]),
		gapBaselineMapFromValue(input[gapBaselineKey]),
		convergenceAuthorityMapFromValue(mapFromValue(input["executionContext"])[convergenceAuthorityKey]),
		gapBaselineMapFromValue(mapFromValue(input["executionContext"])[gapBaselineKey]),
	} {
		if len(source) > 0 {
			return cloneStepPayload(source)
		}
	}
	return nil
}

func shortAuthoritySnapshotDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	digest := hex.EncodeToString(sum[:])
	if len(digest) > 16 {
		return digest[:16]
	}
	return digest
}

func validateCurrentAuthoritySnapshotLineage(input map[string]any) error {
	if len(input) == 0 {
		return nil
	}
	lineage := mapFromValue(input["currentAuthoritySnapshot"])
	if len(lineage) == 0 {
		return nil
	}
	id := strings.TrimSpace(stringFromMap(lineage, "id"))
	if id == "" {
		return convergenceDispatchMisalignmentError("current authority snapshot lineage is missing id")
	}
	for label, value := range map[string]map[string]any{
		"convergenceContract":    mapFromValue(input["convergenceContract"]),
		"repairActionPackage":    mapFromValue(input["repairActionPackage"]),
		"currentActionContract":  mapFromValue(input[currentActionContractKey]),
		"stagePacket":            mapFromValue(input[stepInputKeyStagePacket]),
		"stagePacket.nextAction": mapFromValue(mapFromValue(input[stepInputKeyStagePacket])["nextAction"]),
	} {
		if len(value) == 0 {
			continue
		}
		actual := strings.TrimSpace(stringFromMap(value, "authoritySnapshotId"))
		if actual == "" || actual == id {
			continue
		}
		appendAuthorityDiagnostic(input, "authority_snapshot_lineage_mismatch", "error", "internal_authority_mismatch", label, "derived dispatch field was not rendered from the current authority snapshot")
		return convergenceDispatchMisalignmentError("authority snapshot lineage mismatch: %s=%s expected=%s", label, actual, id)
	}
	return nil
}
