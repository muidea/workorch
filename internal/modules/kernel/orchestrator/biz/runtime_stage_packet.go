package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const stepInputKeyStagePacket = "stagePacket"

func attachStagePacketToStepInput(run *runModel.Run, stepID common.StepID, input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	packet := buildStagePacketFromStepInput(run, stepID, input)
	if !packet.HasScope() {
		return input
	}
	input[stepInputKeyStagePacket] = packet.ToMap()
	return input
}

func buildStagePacketFromStepInput(run *runModel.Run, stepID common.StepID, input map[string]any) runtimemodel.StagePacket {
	if run == nil {
		return runtimemodel.StagePacket{}
	}
	packet := runtimemodel.StagePacket{
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      stepID,
		Definition: runtimemodel.StagePacketDefinition{
			Revision: stagePacketDefinitionRevision(run, input),
			GoalID:   run.GoalID,
			DoDID:    run.DoDID,
		},
		ActiveStage:           stagePacketActiveStage(input),
		ActiveFocus:           stagePacketActiveFocus(input),
		RequiredOutput:        stagePacketRequiredOutput(input),
		AllowedActions:        stringSliceFromValue(input["allowedActions"]),
		ForbiddenActions:      stringSliceFromValue(input["forbiddenActions"]),
		RequiredEvidenceKinds: stringSliceFromValue(input["requiredEvidenceKinds"]),
	}
	return packet.Normalized()
}

func stagePacketDefinitionRevision(run *runModel.Run, input map[string]any) string {
	for _, value := range []string{
		strings.TrimSpace(stringFromMap(input, "definitionRevision")),
		strings.TrimSpace(stringFromMap(mapFromValue(input["executionContext"]), "definitionRevision")),
		strings.TrimSpace(stringFromMap(coverageWorkItemContextFromInput(input), "definitionRevision")),
		strings.TrimSpace(stringFromMap(mapFromValue(input[gapBaselineKey]), "definitionRevision")),
	} {
		if value != "" {
			return value
		}
	}
	if run != nil {
		for _, value := range []string{
			strings.TrimSpace(run.DefinitionRevision),
			strings.TrimSpace(stringFromMap(run.ExecutionContext, "definitionRevision")),
			strings.TrimSpace(stringFromMap(mapFromValue(run.ExecutionContext[coverageWorkItemContextKey]), "definitionRevision")),
			strings.TrimSpace(stringFromMap(mapFromValue(run.ExecutionContext[gapBaselineKey]), "definitionRevision")),
		} {
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func stagePacketActiveStage(input map[string]any) string {
	for _, value := range []string{
		strings.TrimSpace(stringFromMap(input, stepInputKeyRuntimeStage)),
		strings.TrimSpace(stringFromMap(input, stepInputKeySoulStage)),
		strings.TrimSpace(stringFromMap(mapFromValue(input["executionContext"]), stepInputKeyRuntimeStage)),
	} {
		if value != "" {
			return value
		}
	}
	return ""
}

func stagePacketActiveFocus(input map[string]any) map[string]any {
	for _, source := range []map[string]any{
		coverageWorkItemContextFromInput(input),
		mapFromValue(input["repairActionPackage"]),
		mapFromValue(input["acceptance"]),
		mapFromValue(input[coverageFocusKey]),
		mapFromValue(mapFromValue(input["executionContext"])[coverageFocusKey]),
	} {
		if focus := mapFromValue(source["currentFocus"]); len(focus) > 0 {
			return cloneStepPayload(focus)
		}
		for _, key := range []string{"currentFocusItems", "coverageTargetSet", "repairTargetSet"} {
			for _, item := range mapSliceFromValue(source[key]) {
				if len(item) == 0 {
					continue
				}
				focus := cloneStepPayload(item)
				if stringFromMap(focus, "id") == "" {
					if id := firstNonEmptyString(stringFromMap(focus, "coverageWorkItemId"), stringFromMap(focus, "coverageTargetToken")); id != "" {
						focus["id"] = id
					}
				}
				return focus
			}
		}
		if len(source) > 0 && strings.TrimSpace(stringFromMap(source, "id")) != "" {
			return cloneStepPayload(source)
		}
	}
	return nil
}

func stagePacketRequiredOutput(input map[string]any) map[string]any {
	for _, source := range []map[string]any{
		mapFromValue(input["repairActionPackage"]),
		mapFromValue(input),
		mapFromValue(mapFromValue(input["executionContext"])["repairActionPackage"]),
	} {
		if required := mapFromValue(source["requiredOutput"]); len(required) > 0 {
			return cloneStepPayload(required)
		}
	}
	return nil
}
