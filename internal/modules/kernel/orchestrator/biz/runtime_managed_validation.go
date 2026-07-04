package orchestrator

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	managedValidationSessionNodeKind = runtimemodel.RuntimeBlockKindManagedValidationSession
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
	for _, key := range []string{"command", "args", "timeoutMillis", "coverageWorkItemIds", "structuredTargetRefs", "targetRefs", "observedClaims", "validationEvidence"} {
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

func attachManagedValidationResourceOwnership(input map[string]any, request runtimemodel.StepExecutionRequest, now time.Time) bool {
	if len(input) == 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	createdAt := now.UTC().Format(time.RFC3339Nano)
	resources := make([]map[string]any, 0, 2)
	managedInput := inputHasManagedValidation(input)
	if managedInput {
		resources = append(resources, map[string]any{
			"id":            managedValidationResourceID(request.WorkspaceID, request.RunID, request.StepID, "process_group"),
			"workspaceId":   string(request.WorkspaceID),
			"runId":         string(request.RunID),
			"stepId":        string(request.StepID),
			"kind":          "process_group",
			"status":        "pending",
			"cleanupPolicy": "process_group_on_timeout_or_completion",
			"createdAt":     createdAt,
			"source":        "runtime_dispatch",
		})
	}
	if scratch := strings.TrimSpace(stringFromMap(input, validationScratchDirKey)); scratch != "" {
		cleanupPolicy := "remove_step_scoped_scratch_on_completion"
		if validationScratchCleanupRetainedUntilRunTerminal(input) {
			cleanupPolicy = "retain_graph_scoped_scratch_until_run_terminal"
		}
		resources = append(resources, map[string]any{
			"id":            managedValidationResourceID(request.WorkspaceID, request.RunID, request.StepID, "scratch_dir"),
			"workspaceId":   string(request.WorkspaceID),
			"runId":         string(request.RunID),
			"stepId":        string(request.StepID),
			"kind":          "scratch_dir",
			"path":          scratch,
			"status":        "pending",
			"cleanupPolicy": cleanupPolicy,
			"createdAt":     createdAt,
			"source":        "runtime_dispatch",
		})
	}
	if len(resources) == 0 {
		return false
	}
	input["validationResources"] = resources
	refs := make([]string, 0, len(resources))
	for _, resource := range resources {
		if id := strings.TrimSpace(stringFromMap(resource, "id")); id != "" {
			refs = append(refs, id)
		}
	}
	input["validationResourceOwnership"] = map[string]any{
		"persisted":         true,
		"ownedResourceRefs": refs,
		"cleanupPolicy":     "runtime_lifecycle",
	}
	if managedInput {
		managed := mapFromValue(input["managedValidation"])
		if managed == nil {
			managed = map[string]any{}
		}
		managed["resourceOwnershipPersisted"] = true
		managed["ownedResourceRefs"] = refs
		input["managedValidation"] = managed
	}
	return true
}

func inputHasManagedValidation(input map[string]any) bool {
	managed := mapFromValue(input["managedValidation"])
	return boolValue(managed["runtimeManaged"]) ||
		strings.TrimSpace(stringFromMap(managed, "kind")) == managedValidationSessionNodeKind ||
		strings.TrimSpace(stringFromMap(managed, "originalNodeKind")) == managedValidationSessionNodeKind
}

func managedValidationResourceID(workspaceID common.WorkspaceID, runID common.RunID, stepID common.StepID, kind string) string {
	parts := []string{
		string(workspaceID),
		string(runID),
		string(stepID),
		strings.TrimSpace(kind),
	}
	return strings.Join(parts, ":")
}

func finalizeManagedValidationResourcesForStep(step *runModel.Step, finalStatus string, finalizedBy string) bool {
	if step == nil || len(step.Input) == 0 {
		return false
	}
	resources := mapSliceFromValue(step.Input["validationResources"])
	if len(resources) == 0 {
		return false
	}
	finalStatus = strings.TrimSpace(finalStatus)
	if finalStatus == "" {
		finalStatus = "released"
	}
	finalizedBy = strings.TrimSpace(finalizedBy)
	if finalizedBy == "" {
		finalizedBy = "runtime_orchestrator"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	changed := false
	for idx := range resources {
		resource := resources[idx]
		status := strings.TrimSpace(stringFromMap(resource, "status"))
		if managedValidationResourceStatusTerminal(status) {
			continue
		}
		resource["status"] = finalStatus
		resource["finalizedAt"] = now
		resource["finalizedBy"] = finalizedBy
		if strings.TrimSpace(stringFromMap(resource, "kind")) == "scratch_dir" {
			if validationScratchResourceRetainedUntilRunTerminal(resource) && !managedValidationFinalizedByRun(finalizedBy) {
				resource["status"] = "retained"
				resource["retainedUntil"] = "run_terminal"
				resource["cleanupStatus"] = "retained_until_run_terminal"
				resources[idx] = resource
				changed = true
				continue
			}
			if cleanupStatus, cleanupErr := cleanupOwnedValidationScratch(resource); cleanupStatus != "" {
				resource["cleanupStatus"] = cleanupStatus
				if cleanupErr != "" {
					resource["cleanupError"] = cleanupErr
					resource["status"] = "cleanup_failed"
				}
			}
		}
		resources[idx] = resource
		changed = true
	}
	if !changed {
		return false
	}
	step.Input["validationResources"] = resources
	ownership := mapFromValue(step.Input["validationResourceOwnership"])
	if len(ownership) > 0 {
		ownership["finalized"] = true
		ownership["finalizedAt"] = now
		ownership["finalizedBy"] = finalizedBy
		step.Input["validationResourceOwnership"] = ownership
	}
	managed := mapFromValue(step.Input["managedValidation"])
	if len(managed) > 0 {
		managed["resourcesFinalized"] = true
		managed["resourcesFinalizedAt"] = now
		managed["resourcesFinalizedBy"] = finalizedBy
		step.Input["managedValidation"] = managed
	}
	return true
}

func validationScratchCleanupRetainedUntilRunTerminal(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(input, "validationScratchScope")) == "execution_graph" {
		return true
	}
	if strings.TrimSpace(stringFromMap(input, "validationScratchCleanupPolicy")) == "retain_until_run_terminal" {
		return true
	}
	if value, ok := input["validationScratchAutoCleanup"]; ok && !boolValue(value) && strings.TrimSpace(stringFromMap(input, stepInputKeyGraphRootStepID)) != "" {
		return true
	}
	return false
}

func validationScratchResourceRetainedUntilRunTerminal(resource map[string]any) bool {
	switch strings.TrimSpace(stringFromMap(resource, "cleanupPolicy")) {
	case "retain_graph_scoped_scratch_until_run_terminal", "retain_until_run_terminal":
		return true
	default:
		return false
	}
}

func managedValidationFinalizedByRun(finalizedBy string) bool {
	finalizedBy = strings.TrimSpace(finalizedBy)
	return finalizedBy == "run_terminal" || strings.HasPrefix(finalizedBy, "run_")
}

func managedValidationResourceStatusTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case "released", "skipped", "cleanup_failed", "cleanup_skipped", "removed":
		return true
	default:
		return false
	}
}

func (o *RuntimeOrchestrator) finalizeManagedValidationResourcesForRun(ctx context.Context, run *runModel.Run, finalStatus string, finalizedBy string) error {
	if o == nil || o.steps == nil || run == nil || run.ID == "" {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if !finalizeManagedValidationResourcesForStep(step, finalStatus, finalizedBy) {
			continue
		}
		if err := o.steps.Update(ctx, step); err != nil {
			return err
		}
	}
	return nil
}

func cleanupOwnedValidationScratch(resource map[string]any) (string, string) {
	path := strings.TrimSpace(stringFromMap(resource, "path"))
	if path == "" {
		return "", ""
	}
	root := normalizeValidationScratchDir(filepath.Join(os.TempDir(), "workorch-validation-scratch"))
	if !validationScratchDirUnderRoot(path, root) {
		return "skipped_outside_runtime_scratch_root", ""
	}
	if err := os.RemoveAll(path); err != nil {
		return "failed", err.Error()
	}
	return "removed", ""
}
