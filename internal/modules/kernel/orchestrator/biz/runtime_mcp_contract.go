package orchestrator

import (
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validateMCPToolCallStepInputBeforeRuntime(run *runModel.Run, step *runModel.Step, input map[string]any) error {
	if step == nil || strings.TrimSpace(step.Kind) != runtimemodel.StepKindMCPToolCall {
		return nil
	}
	serverID := strings.TrimSpace(stringFromMap(input, "serverId"))
	tool := strings.TrimSpace(stringFromMap(input, "tool"))
	if serverID == "" || tool == "" {
		return nil
	}
	schema := mcpToolInputSchemaForStep(run, step, serverID, tool)
	if len(schema) == 0 {
		return nil
	}
	toolInput := mapFromValue(input["input"])
	if toolInput == nil {
		toolInput = map[string]any{}
	}
	if err := validateRuntimeMCPSchemaValue(toolInput, schema, "input"); err != nil {
		return fmt.Errorf("%w: mcp/tool_call input does not satisfy capability schema for %s/%s: %v", common.ErrInvalidArgument, serverID, tool, err)
	}
	return nil
}

func mcpToolInputSchemaForStep(run *runModel.Run, step *runModel.Step, serverID string, tool string) map[string]any {
	serverID = strings.TrimSpace(serverID)
	tool = strings.TrimSpace(tool)
	if serverID == "" || tool == "" {
		return nil
	}
	for _, capability := range capabilityUsagePolicyCapabilityContexts(run, step) {
		if !capabilityContextEnabled(capability) {
			continue
		}
		if strings.TrimSpace(stringFromMap(capability, "kind")) != "tool" {
			continue
		}
		if strings.TrimSpace(stringFromMap(capability, "serverId")) != serverID ||
			strings.TrimSpace(stringFromMap(capability, "name")) != tool {
			continue
		}
		if schema := mapFromValue(capability["schema"]); len(schema) > 0 {
			return schema
		}
	}
	return nil
}

func validateRuntimeMCPSchemaValue(value any, schema map[string]any, path string) error {
	if len(schema) == 0 {
		return nil
	}
	if requirements := mapSliceFromValue(schema["allOf"]); len(requirements) > 0 {
		base := cloneStepPayload(schema)
		delete(base, "allOf")
		if err := validateRuntimeMCPSchemaValue(value, base, path); err != nil {
			return err
		}
		for _, requirement := range requirements {
			if err := validateRuntimeMCPSchemaValue(value, requirement, path); err != nil {
				return err
			}
		}
		return nil
	}
	if alternatives := mapSliceFromValue(schema["anyOf"]); len(alternatives) > 0 {
		base := cloneStepPayload(schema)
		delete(base, "anyOf")
		delete(base, "oneOf")
		if err := validateRuntimeMCPSchemaValue(value, base, path); err != nil {
			return err
		}
		return validateRuntimeMCPSchemaAlternatives(value, alternatives, path, "anyOf")
	}
	if alternatives := mapSliceFromValue(schema["oneOf"]); len(alternatives) > 0 {
		base := cloneStepPayload(schema)
		delete(base, "anyOf")
		delete(base, "oneOf")
		if err := validateRuntimeMCPSchemaValue(value, base, path); err != nil {
			return err
		}
		return validateRuntimeMCPSchemaAlternatives(value, alternatives, path, "oneOf")
	}
	schemaType := strings.TrimSpace(stringFromMap(schema, "type"))
	if schemaType != "" {
		if err := validateRuntimeMCPSchemaType(value, schemaType, path); err != nil {
			return err
		}
	}
	if schemaType == "object" || len(mapFromValue(schema["properties"])) > 0 || len(stringSliceFromValue(schema["required"])) > 0 {
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be object", path)
		}
		for _, field := range stringSliceFromValue(schema["required"]) {
			if _, ok := object[field]; !ok {
				return fmt.Errorf("%s.%s is required", path, field)
			}
		}
		properties := mapFromValue(schema["properties"])
		for key, raw := range object {
			propertySchema := mapFromValue(properties[key])
			if len(propertySchema) == 0 {
				if schema["additionalProperties"] == false {
					return fmt.Errorf("%s.%s is not declared by capability schema", path, key)
				}
				continue
			}
			if err := validateRuntimeMCPSchemaValue(raw, propertySchema, path+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRuntimeMCPSchemaAlternatives(value any, alternatives []map[string]any, path string, label string) error {
	var lastErr error
	for _, alternative := range alternatives {
		if err := validateRuntimeMCPSchemaValue(value, alternative, path); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no alternatives declared")
	}
	return fmt.Errorf("%s does not match %s capability schema: %v", path, label, lastErr)
}

func validateRuntimeMCPSchemaType(value any, schemaType string, path string) error {
	switch schemaType {
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%s must be object", path)
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be string", path)
		}
	case "number":
		switch value.(type) {
		case int, int64, int32, float64, float32:
		default:
			return fmt.Errorf("%s must be number", path)
		}
	case "integer":
		switch value.(type) {
		case int, int64, int32, float64, float32:
		default:
			return fmt.Errorf("%s must be integer", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be boolean", path)
		}
	case "array":
		switch value.(type) {
		case []any, []string:
		default:
			return fmt.Errorf("%s must be array", path)
		}
	}
	return nil
}
