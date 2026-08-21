package mcp

import (
	"fmt"
	"strings"

	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// validateMCPOperationInputSchema enforces the bounded scalar subset that can
// cross RuntimeOperation. The full Registry schema remains sealed by revision;
// unsupported schema shapes fail closed instead of being silently ignored.
func validateMCPOperationInputSchema(capability *mcpmodel.Capability, spec operationSpec) error {
	if capability == nil {
		return common.ErrInvalidArgument
	}
	var values []runtimecontract.OperationParameter
	switch spec.Operation {
	case runtimecontract.StepKindMCPToolCall:
		values = spec.Input
	case runtimecontract.StepKindMCPPromptGet:
		values = spec.Arguments
	case runtimecontract.StepKindMCPResourceRead:
		return nil
	default:
		return common.ErrInvalidArgument
	}
	schema := capability.Schema
	if len(schema.Fields) == 0 {
		if len(values) == 0 {
			return nil
		}
		return fmt.Errorf("%w: MCP capability has no input schema for supplied arguments", common.ErrInvalidArgument)
	}
	if err := validateMCPScalarSchemaVocabulary(schema); err != nil {
		return err
	}
	schemaType, declared := schema.String("type")
	if !declared || strings.TrimSpace(schemaType) != "object" {
		return fmt.Errorf("%w: MCP capability input schema must be an object", common.ErrInvalidArgument)
	}
	properties, hasProperties := schema.Object("properties")
	if rawProperties, declared := schema.Field("properties"); declared && rawProperties.Kind != workorcheventpayload.ValueObject {
		return fmt.Errorf("%w: MCP capability properties must be an object", common.ErrInvalidArgument)
	} else if !hasProperties {
		properties = workorcheventpayload.Payload{}
	}
	required, hasRequired := schema.StringSlice("required")
	if _, declared := schema.Field("required"); declared && !hasRequired {
		return fmt.Errorf("%w: MCP capability required input list is invalid", common.ErrInvalidArgument)
	}
	if len(required) > 64 {
		return fmt.Errorf("%w: MCP capability required input list exceeds 64 entries", common.ErrInvalidArgument)
	}
	seenRequired := make(map[string]struct{}, len(required))
	for index := range required {
		name := strings.TrimSpace(required[index])
		if name == "" || len(name) > 128 || name != required[index] {
			return fmt.Errorf("%w: MCP capability required input has an invalid bounded name", common.ErrInvalidArgument)
		}
		if _, duplicate := seenRequired[name]; duplicate {
			return fmt.Errorf("%w: MCP capability required input %q is duplicated", common.ErrInvalidArgument, name)
		}
		seenRequired[name] = struct{}{}
	}
	additionalAllowed := true
	if additional, declared := schema.Field("additionalProperties"); declared {
		if additional.Kind != workorcheventpayload.ValueBool {
			return fmt.Errorf("%w: MCP capability additionalProperties must be boolean", common.ErrInvalidArgument)
		}
		additionalAllowed = additional.Bool
	}
	provided := make(map[string]runtimecontract.OperationParameter, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			return common.ErrInvalidArgument
		}
		if _, duplicate := provided[name]; duplicate {
			return fmt.Errorf("%w: MCP input %q is duplicated", common.ErrInvalidArgument, name)
		}
		provided[name] = value
		property, known := properties.Field(name)
		if !known {
			if !additionalAllowed {
				return fmt.Errorf("%w: MCP input %q is outside the sealed capability schema", common.ErrInvalidArgument, name)
			}
			continue
		}
		if property.Kind != workorcheventpayload.ValueObject {
			return fmt.Errorf("%w: MCP input schema for %q is invalid", common.ErrInvalidArgument, name)
		}
		propertySchema := workorcheventpayload.Payload{Fields: property.Object}
		propertyType, ok := propertySchema.String("type")
		if !ok || !mcpOperationParameterMatchesSchemaType(value, strings.TrimSpace(propertyType)) {
			return fmt.Errorf("%w: MCP input %q does not match the sealed scalar schema", common.ErrInvalidArgument, name)
		}
	}
	for _, name := range required {
		if _, found := provided[strings.TrimSpace(name)]; !found {
			return fmt.Errorf("%w: MCP input %q is required by the sealed capability schema", common.ErrInvalidArgument, name)
		}
	}
	return nil
}

func validateMCPScalarSchemaVocabulary(schema workorcheventpayload.Payload) error {
	allowedRoot := map[string]struct{}{
		"$id": {}, "$schema": {}, "additionalProperties": {}, "description": {}, "properties": {}, "required": {}, "title": {}, "type": {},
	}
	if len(schema.Fields) > 72 {
		return fmt.Errorf("%w: MCP capability input schema exceeds the bounded scalar vocabulary", common.ErrInvalidArgument)
	}
	for _, field := range schema.Fields {
		if _, allowed := allowedRoot[field.Name]; !allowed {
			return fmt.Errorf("%w: MCP capability input schema keyword %q is unsupported", common.ErrInvalidArgument, field.Name)
		}
	}
	for _, name := range []string{"$id", "$schema", "description", "title"} {
		if value, declared := schema.Field(name); declared && value.Kind != workorcheventpayload.ValueString {
			return fmt.Errorf("%w: MCP capability input schema metadata %q must be a string", common.ErrInvalidArgument, name)
		}
	}
	properties, declared := schema.Object("properties")
	if !declared {
		return nil
	}
	if len(properties.Fields) > 64 {
		return fmt.Errorf("%w: MCP capability input schema exceeds 64 scalar properties", common.ErrInvalidArgument)
	}
	for _, property := range properties.Fields {
		if name := strings.TrimSpace(property.Name); name == "" || len(name) > 128 || name != property.Name {
			return fmt.Errorf("%w: MCP capability input schema has an invalid bounded property name", common.ErrInvalidArgument)
		}
		if property.Value.Kind != workorcheventpayload.ValueObject {
			return fmt.Errorf("%w: MCP capability input schema property %q must be an object", common.ErrInvalidArgument, property.Name)
		}
		definition := workorcheventpayload.Payload{Fields: property.Value.Object}
		propertyType := ""
		for _, field := range definition.Fields {
			switch field.Name {
			case "type":
				if field.Value.Kind != workorcheventpayload.ValueString {
					return fmt.Errorf("%w: MCP capability input schema property %q type must be a string", common.ErrInvalidArgument, property.Name)
				}
				propertyType = strings.TrimSpace(field.Value.String)
			case "description", "title":
				if field.Value.Kind != workorcheventpayload.ValueString {
					return fmt.Errorf("%w: MCP capability input schema property %q metadata must be a string", common.ErrInvalidArgument, property.Name)
				}
			default:
				return fmt.Errorf("%w: MCP capability input schema property %q keyword %q is unsupported", common.ErrInvalidArgument, property.Name, field.Name)
			}
		}
		switch propertyType {
		case "string", "integer", "number", "boolean":
		default:
			return fmt.Errorf("%w: MCP capability input schema property %q has unsupported scalar type %q", common.ErrInvalidArgument, property.Name, propertyType)
		}
	}
	return nil
}

func mcpOperationParameterMatchesSchemaType(value runtimecontract.OperationParameter, schemaType string) bool {
	switch schemaType {
	case "string":
		return value.String != nil && value.Integer == nil && value.Boolean == nil
	case "integer", "number":
		return value.Integer != nil && value.String == nil && value.Boolean == nil
	case "boolean":
		return value.Boolean != nil && value.String == nil && value.Integer == nil
	default:
		return false
	}
}
