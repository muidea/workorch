package orchestrator

import (
	"net"
	"strconv"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	managedValidationSessionNodeKind = "validation/managed_session"
	managedValidationTimeoutMillis   = 60000
)

func normalizeManagedValidationOutput(step *runModel.Step, output map[string]any) {
	if step == nil || output == nil {
		return
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		return
	}
	normalized, changed := normalizeManagedValidationGraph(step, graph)
	if !changed {
		return
	}
	output[stepOutputKeyExecutionGraph] = normalized
}

func normalizeManagedValidationGraph(step *runModel.Step, graph runtimemodel.BlockExecutionGraph) (runtimemodel.BlockExecutionGraph, bool) {
	if len(graph.Nodes) == 0 {
		return graph, false
	}
	changed := false
	nodes := make([]runtimemodel.BlockExecutionNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		next, ok := normalizeManagedValidationNode(step, node)
		if ok {
			changed = true
		}
		nodes = append(nodes, next)
	}
	if !changed {
		return graph, false
	}
	return runtimemodel.BlockExecutionGraph{Nodes: nodes}, true
}

func normalizeManagedValidationNode(step *runModel.Step, node runtimemodel.BlockExecutionNode) (runtimemodel.BlockExecutionNode, bool) {
	if strings.TrimSpace(node.Kind) != managedValidationSessionNodeKind {
		return node, false
	}
	next := node
	next.Kind = "shell/command"
	next.Input = cloneStepPayload(node.Input)
	if next.Input == nil {
		next.Input = map[string]any{}
	}
	applyManagedValidationObservationInput(next.Input)
	managed := mapFromValue(next.Input["managedValidation"])
	if managed == nil {
		managed = map[string]any{}
	}
	managed["kind"] = managedValidationSessionNodeKind
	managed["originalNodeKind"] = managedValidationSessionNodeKind
	managed["runtimeManaged"] = true
	managed["autoCleanup"] = true
	managed["cleanupPolicy"] = "process_group_on_timeout_or_completion"
	managed["executionKind"] = "bounded_foreground_command"
	if scratch := validationScratchDirFromStep(step); scratch != "" {
		managed[validationScratchDirKey] = scratch
		if strings.TrimSpace(stringFromMap(next.Input, validationScratchDirKey)) == "" {
			next.Input[validationScratchDirKey] = scratch
		}
	}
	if port, ok := managedValidationPort(next.Input, managed); ok {
		managed["port"] = port
		managed["allocatedPort"] = port
		next.Input["managedValidationPort"] = port
		applyManagedValidationPortPlaceholders(next.Input, port)
	}
	next.Input["managedValidation"] = managed
	if next.Input["timeoutMillis"] == nil {
		next.Input["timeoutMillis"] = managedValidationTimeoutMillis
	}
	return next, true
}

func applyManagedValidationObservationInput(input map[string]any) {
	if len(input) == 0 {
		return
	}
	observation := mapFromValue(input["observation"])
	if len(observation) == 0 {
		observation = mapFromValue(input["observe"])
	}
	if len(observation) == 0 {
		return
	}
	for _, key := range []string{"command", "args", "timeoutMillis", "targetRefs", "validationEvidence"} {
		if input[key] == nil && observation[key] != nil {
			input[key] = observation[key]
		}
	}
}

func managedValidationPort(input map[string]any, managed map[string]any) (int, bool) {
	for _, value := range []any{
		input["managedValidationPort"],
		input["port"],
		managed["port"],
		managed["allocatedPort"],
	} {
		if port, ok := managedValidationInt(value); ok && port > 0 {
			return port, true
		}
	}
	if !boolValue(input["allocatePort"]) && !boolValue(managed["allocatePort"]) && strings.TrimSpace(stringFromMap(managed, "portPolicy")) != "runtime_allocated" {
		return 0, false
	}
	port, ok := allocateManagedValidationPort()
	return port, ok
}

func allocateManagedValidationPort() (int, bool) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, false
	}
	defer listener.Close()
	parts := strings.Split(listener.Addr().String(), ":")
	if len(parts) == 0 {
		return 0, false
	}
	port, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || port <= 0 {
		return 0, false
	}
	return port, true
}

func managedValidationInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), typed > 0
	case string:
		port, err := strconv.Atoi(strings.TrimSpace(typed))
		return port, err == nil
	default:
		return 0, false
	}
}

func applyManagedValidationPortPlaceholders(input map[string]any, port int) {
	if len(input) == 0 || port <= 0 {
		return
	}
	portText := strconv.Itoa(port)
	for _, key := range []string{"command"} {
		if value := strings.TrimSpace(stringFromMap(input, key)); value != "" {
			input[key] = replaceManagedValidationPortPlaceholders(value, portText)
		}
	}
	args := stringSliceFromValue(input["args"])
	if len(args) > 0 {
		for idx, value := range args {
			args[idx] = replaceManagedValidationPortPlaceholders(value, portText)
		}
		input["args"] = args
	}
}

func replaceManagedValidationPortPlaceholders(value string, portText string) string {
	for _, placeholder := range []string{
		"{{managedValidation.port}}",
		"{{managedValidation.allocatedPort}}",
		"${WORKORCH_MANAGED_VALIDATION_PORT}",
		"$WORKORCH_MANAGED_VALIDATION_PORT",
	} {
		value = strings.ReplaceAll(value, placeholder, portText)
	}
	return value
}
