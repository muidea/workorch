package mcp

import (
	"fmt"
	"strings"

	blockcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func parseOperationSpec(request RuntimeOperationRequest) (operationSpec, error) {
	spec := operationSpec{
		Operation:  strings.TrimSpace(request.Kind),
		ServerID:   strings.TrimSpace(request.Data.ServerID),
		Tool:       strings.TrimSpace(request.Data.Tool),
		URI:        strings.TrimSpace(request.Data.URI),
		Prompt:     strings.TrimSpace(request.Data.Prompt),
		Input:      append([]runtimecontract.OperationParameter(nil), request.Data.Input...),
		Arguments:  append([]runtimecontract.OperationParameter(nil), request.Data.Arguments...),
		Summary:    strings.TrimSpace(request.Data.Summary),
		Stream:     request.Data.Stream,
		TargetRefs: append([]string(nil), request.TargetRefs...), TargetRole: strings.TrimSpace(request.TargetRole),
	}
	if spec.Operation == "" {
		spec.Operation = blockcommon.StepKindToolCall
	}
	if spec.ServerID == "" {
		return spec, fmt.Errorf("%w: serverId is required", common.ErrInvalidArgument)
	}
	switch spec.Operation {
	case blockcommon.StepKindToolCall:
		if spec.Tool == "" {
			return spec, fmt.Errorf("%w: tool is required", common.ErrInvalidArgument)
		}
	case blockcommon.StepKindResourceRead:
		if spec.URI == "" {
			return spec, fmt.Errorf("%w: uri is required", common.ErrInvalidArgument)
		}
	case blockcommon.StepKindPromptGet:
		if spec.Prompt == "" {
			return spec, fmt.Errorf("%w: prompt is required", common.ErrInvalidArgument)
		}
	default:
		return spec, fmt.Errorf("%w: unsupported mcp operation %q", common.ErrInvalidArgument, spec.Operation)
	}
	return spec, nil
}

func validateServerPolicy(server *mcpmodel.Server, spec operationSpec) error {
	if server == nil || server.ID == "" {
		return common.ErrNotFound
	}
	if !server.Enabled {
		return fmt.Errorf("%w: mcp server %s is disabled", common.ErrInvalidArgument, spec.ServerID)
	}
	switch spec.Operation {
	case blockcommon.StepKindToolCall:
		if containsTrimmed(server.AllowedTools, spec.Tool) {
			return nil
		}
		return fmt.Errorf("%w: tool %s is not allowed for mcp server %s", common.ErrInvalidArgument, spec.Tool, spec.ServerID)
	case blockcommon.StepKindResourceRead:
		if containsTrimmed(server.AllowedResources, spec.URI) {
			return nil
		}
		return fmt.Errorf("%w: resource %s is not allowed for mcp server %s", common.ErrInvalidArgument, spec.URI, spec.ServerID)
	case blockcommon.StepKindPromptGet:
		if containsTrimmed(server.AllowedPrompts, spec.Prompt) {
			return nil
		}
		return fmt.Errorf("%w: prompt %s is not allowed for mcp server %s", common.ErrInvalidArgument, spec.Prompt, spec.ServerID)
	default:
		return common.ErrInvalidArgument
	}
}

func containsTrimmed(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
