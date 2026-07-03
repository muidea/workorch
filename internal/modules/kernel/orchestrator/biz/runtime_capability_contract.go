package orchestrator

import (
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validateLLMSoulGraphCapabilityContract(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok || len(graph.Nodes) == 0 {
		return nil
	}
	if err := validateGraphDeclaredMCPServers(run, step, graph); err != nil {
		return err
	}
	soul := runtimeSoulForStep(run, step)
	if len(soul) == 0 {
		return nil
	}
	if err := validateGraphAllowedToolKinds(graph, stringSliceFromValue(soul["allowedToolKinds"])); err != nil {
		return err
	}
	if err := validateGraphAllowedMCPServers(graph, stringSliceFromValue(soul["allowedMCPServerRefs"])); err != nil {
		return err
	}
	return nil
}

func validateLLMCapabilityUsagePolicyContract(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if run == nil || step == nil || !ok || len(graph.Nodes) == 0 {
		return nil
	}
	policies := capabilityUsagePoliciesForStep(run, step)
	if len(policies) == 0 {
		return nil
	}
	for _, policy := range policies {
		requiredAfter := stringSliceFromValue(policy["requiredAfterKinds"])
		if len(requiredAfter) == 0 || !capabilityUsagePolicyAppliesToStage(policy, step) {
			continue
		}
		if !graphHasAnyNodeKind(graph, requiredAfter) {
			continue
		}
		if !capabilityUsagePolicyHasAvailableCapability(run, step, policy) {
			continue
		}
		if graphHasCapabilityPolicyFollowUp(graph, policy) || outputDeclaresCapabilityPolicySkip(output, policy) {
			continue
		}
		if capabilityUsagePolicyAllowsNonBlockingFallback(policy) {
			continue
		}
		return fmt.Errorf(
			"%w: executionGraph triggered capability usage policy %q after %q but returned no dependent matching mcp/tool_call or structured capabilityPolicySkips reason",
			common.ErrInvalidArgument,
			strings.TrimSpace(stringFromMap(policy, "id")),
			strings.Join(requiredAfter, ","),
		)
	}
	return nil
}

func capabilityUsagePoliciesForStep(run *runModel.Run, step *runModel.Step) []map[string]any {
	if run == nil {
		return nil
	}
	var ret []map[string]any
	seen := map[string]bool{}
	for _, source := range []map[string]any{
		mapFromValue(run.ExecutionContext["executionContract"]),
		mapFromValue(stepInputValue(step, "executionContract")),
		mapFromValue(mapFromValue(stepInputValue(step, "executionContext"))["executionContract"]),
		mapFromValue(mapFromValue(stepInputValue(step, stepInputKeyExecutionContextPackage))["executionContract"]),
	} {
		for _, policy := range mapSliceFromValue(source["capabilityUsagePolicies"]) {
			if len(policy) == 0 {
				continue
			}
			key := capabilityUsagePolicyIdentityKey(policy)
			if key != "" {
				if seen[key] {
					continue
				}
				seen[key] = true
			}
			ret = append(ret, policy)
		}
	}
	return ret
}

func capabilityUsagePolicyIdentityKey(policy map[string]any) string {
	if len(policy) == 0 {
		return ""
	}
	if id := strings.TrimSpace(stringFromMap(policy, "id")); id != "" {
		return "id:" + id
	}
	category := strings.TrimSpace(stringFromMap(policy, "appliesToCategory"))
	requiredAfter := strings.Join(compactUniqueStrings(stringSliceFromValue(policy["requiredAfterKinds"])), ",")
	preferredTools := strings.Join(compactUniqueStrings(stringSliceFromValue(policy["preferredTools"])), ",")
	if category == "" && requiredAfter == "" && preferredTools == "" {
		return ""
	}
	return "shape:" + category + "|" + requiredAfter + "|" + preferredTools
}

func capabilityUsagePolicyAllowsNonBlockingFallback(policy map[string]any) bool {
	if len(policy) == 0 {
		return false
	}
	mode := strings.TrimSpace(stringFromMap(policy, "usageMode"))
	fallback := strings.ToLower(strings.TrimSpace(stringFromMap(policy, "fallbackPolicy")))
	if mode != "prefer_when_triggered" {
		return false
	}
	return strings.Contains(fallback, "fallback") &&
		(strings.Contains(fallback, "non_blocking") ||
			strings.Contains(fallback, "non-blocking") ||
			strings.Contains(fallback, "without_blocking") ||
			strings.Contains(fallback, "without-blocking"))
}

func stepInputValue(step *runModel.Step, key string) any {
	if step == nil || step.Input == nil {
		return nil
	}
	return step.Input[key]
}

func capabilityUsagePolicyAppliesToStage(policy map[string]any, step *runModel.Step) bool {
	stages := stringSliceFromValue(policy["triggerStages"])
	if len(stages) == 0 {
		return true
	}
	stage := strings.TrimSpace(firstNonEmptyString(
		runtimeStageFromStepInput(step.Input),
		stringFromMap(step.Input, stepInputKeySoulStage),
	))
	if stage == "" {
		return true
	}
	return stringSliceContains(stages, stage)
}

func capabilityUsagePolicyHasAvailableCapability(run *runModel.Run, step *runModel.Step, policy map[string]any) bool {
	category := strings.TrimSpace(stringFromMap(policy, "appliesToCategory"))
	if category == "" {
		return false
	}
	preferred := stringSet(stringSliceFromValue(policy["preferredTools"]))
	for _, capability := range capabilityUsagePolicyCapabilityContexts(run, step) {
		if strings.TrimSpace(stringFromMap(capability, "category")) != category {
			continue
		}
		if !capabilityContextEnabled(capability) {
			continue
		}
		if len(preferred) == 0 || preferred[strings.TrimSpace(stringFromMap(capability, "name"))] {
			return true
		}
	}
	return false
}

func capabilityUsagePolicyCapabilityContexts(run *runModel.Run, step *runModel.Step) []map[string]any {
	var ret []map[string]any
	if run != nil {
		ret = append(ret, mapSliceFromValue(run.ExecutionContext["mcpCapabilities"])...)
	}
	for _, source := range []any{
		stepInputValue(step, "mcpCapabilities"),
		mapFromValue(stepInputValue(step, "executionContext"))["mcpCapabilities"],
		mapFromValue(stepInputValue(step, stepInputKeyExecutionContextPackage))["mcpCapabilities"],
		mapFromValue(mapFromValue(stepInputValue(step, stepInputKeyExecutionContextPackage))["runtimeCapabilities"])["mcpCapabilities"],
	} {
		ret = append(ret, mapSliceFromValue(source)...)
	}
	return ret
}

func capabilityContextEnabled(capability map[string]any) bool {
	status := strings.TrimSpace(stringFromMap(capability, "status"))
	if status != "" && status != "active" {
		return false
	}
	enabled, ok := capability["enabled"]
	if !ok {
		return true
	}
	return boolValue(enabled)
}

func graphHasCapabilityPolicyFollowUp(graph runtimemodel.BlockExecutionGraph, policy map[string]any) bool {
	preferred := stringSet(stringSliceFromValue(policy["preferredTools"]))
	nodes := graphNodesByID(graph)
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.Kind) != "mcp/tool_call" {
			continue
		}
		tool := graphNodeMCPToolName(node)
		if len(preferred) > 0 && !preferred[tool] {
			continue
		}
		if graphNodeHasAnyRequiredAfterAncestor(node, nodes, stringSliceFromValue(policy["requiredAfterKinds"]), map[string]bool{}) {
			return true
		}
	}
	return false
}

func graphNodeMCPToolName(node runtimemodel.BlockExecutionNode) string {
	if value := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(node.Input, "tool"),
		stringFromMap(node.Input, "toolName"),
		stringFromMap(node.Input, "name"),
	)); value != "" {
		return value
	}
	nested := mapFromValue(node.Input["input"])
	return strings.TrimSpace(firstNonEmptyString(
		stringFromMap(nested, "tool"),
		stringFromMap(nested, "toolName"),
		stringFromMap(nested, "name"),
	))
}

func graphNodeHasAnyRequiredAfterAncestor(node runtimemodel.BlockExecutionNode, nodes map[string]runtimemodel.BlockExecutionNode, requiredAfter []string, seen map[string]bool) bool {
	if len(requiredAfter) == 0 {
		return false
	}
	required := stringSet(requiredAfter)
	for _, depID := range node.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" || seen[depID] {
			continue
		}
		seen[depID] = true
		parent, ok := nodes[depID]
		if !ok {
			continue
		}
		if required[strings.TrimSpace(parent.Kind)] {
			return true
		}
		if graphNodeHasAnyRequiredAfterAncestor(parent, nodes, requiredAfter, seen) {
			return true
		}
	}
	return false
}

func graphHasAnyNodeKind(graph runtimemodel.BlockExecutionGraph, kinds []string) bool {
	if len(kinds) == 0 {
		return false
	}
	allowed := stringSet(kinds)
	for _, node := range graph.Nodes {
		if allowed[strings.TrimSpace(node.Kind)] {
			return true
		}
	}
	return false
}

func outputDeclaresCapabilityPolicySkip(output map[string]any, policy map[string]any) bool {
	if len(output) == 0 {
		return false
	}
	for _, item := range capabilityPolicySkipItems(output) {
		if !capabilityPolicySkipMatches(item, policy) {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(
			stringFromMap(item, "reason"),
			stringFromMap(item, "skipReason"),
			stringFromMap(item, "fallbackReason"),
		)) != "" {
			return true
		}
	}
	return false
}

func capabilityPolicySkipItems(output map[string]any) []map[string]any {
	var ret []map[string]any
	for _, key := range []string{"capabilityPolicySkips", "capabilityUsageSkips", "capabilityPolicyDecisions"} {
		ret = append(ret, mapSliceFromValue(output[key])...)
	}
	for _, key := range []string{"capabilityPolicySkip", "capabilityUsageSkip"} {
		if item := mapFromValue(output[key]); len(item) > 0 {
			ret = append(ret, item)
		}
	}
	return ret
}

func capabilityPolicySkipMatches(item map[string]any, policy map[string]any) bool {
	if len(item) == 0 || len(policy) == 0 {
		return false
	}
	policyID := strings.TrimSpace(stringFromMap(policy, "id"))
	if policyID != "" && strings.TrimSpace(firstNonEmptyString(
		stringFromMap(item, "policyId"),
		stringFromMap(item, "id"),
	)) == policyID {
		return true
	}
	category := strings.TrimSpace(stringFromMap(policy, "appliesToCategory"))
	return category != "" && strings.TrimSpace(firstNonEmptyString(
		stringFromMap(item, "category"),
		stringFromMap(item, "appliesToCategory"),
	)) == category
}

func runtimeSoulForStep(run *runModel.Run, step *runModel.Step) map[string]any {
	if run == nil || step == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	stage := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(step.Input, stepInputKeySoulStage),
		stringFromMap(step.Input, stepInputKeyRuntimeStage),
	))
	routing := mapFromValue(run.ExecutionContext["soulRouting"])
	bindings := mapFromValue(routing["bindings"])
	if stage != "" {
		if binding := mapFromValue(bindings[stage]); len(binding) > 0 {
			if soul := mapFromValue(binding["soul"]); len(soul) > 0 {
				return soul
			}
		}
	}
	return mapFromValue(run.ExecutionContext["soul"])
}

func validateGraphAllowedToolKinds(graph runtimemodel.BlockExecutionGraph, allowed []string) error {
	if len(allowed) == 0 || stringListAllows(allowed, "*") {
		return nil
	}
	allowedSet := stringSet(allowed)
	for _, node := range graph.Nodes {
		kind := strings.TrimSpace(node.Kind)
		if kind == "" || allowedSet[kind] {
			continue
		}
		return fmt.Errorf("%w: executionGraph node %q kind %q is not allowed by current soul allowedToolKinds", common.ErrInvalidArgument, strings.TrimSpace(node.ID), kind)
	}
	return nil
}

func validateGraphAllowedMCPServers(graph runtimemodel.BlockExecutionGraph, allowed []string) error {
	if len(allowed) == 0 || stringListAllows(allowed, "*") {
		return nil
	}
	allowedSet := stringSet(allowed)
	for _, node := range graph.Nodes {
		if !graphNodeRequiresMCPServerDeclaration(node) {
			continue
		}
		serverID := graphNodeMCPServerID(node)
		if serverID == "" || allowedSet[serverID] {
			continue
		}
		return fmt.Errorf("%w: executionGraph node %q MCP server %q is not allowed by current soul allowedMCPServerRefs", common.ErrInvalidArgument, strings.TrimSpace(node.ID), serverID)
	}
	return nil
}

func validateGraphDeclaredMCPServers(run *runModel.Run, step *runModel.Step, graph runtimemodel.BlockExecutionGraph) error {
	declared, present := declaredRuntimeMCPServerIDs(run, step)
	if !present {
		return nil
	}
	for _, node := range graph.Nodes {
		if !graphNodeRequiresMCPServerDeclaration(node) {
			continue
		}
		serverID := graphNodeMCPServerID(node)
		if serverID == "" || declared[serverID] {
			continue
		}
		return fmt.Errorf("%w: executionGraph node %q MCP server %q is not declared by current runtime MCP capabilities", common.ErrInvalidArgument, strings.TrimSpace(node.ID), serverID)
	}
	return nil
}

func declaredRuntimeMCPServerIDs(run *runModel.Run, step *runModel.Step) (map[string]bool, bool) {
	var sources []map[string]any
	if run != nil {
		sources = append(sources, run.ExecutionContext)
	}
	if step != nil && step.Input != nil {
		sources = append(sources,
			step.Input,
			mapFromValue(step.Input["executionContext"]),
			mapFromValue(step.Input[stepInputKeyExecutionContextPackage]),
		)
	}
	return declaredRuntimeMCPServerIDsFromSources(sources...)
}

func declaredRuntimeMCPServerIDsFromSources(sources ...map[string]any) (map[string]bool, bool) {
	ret := map[string]bool{}
	present := false
	addServer := func(server map[string]any) {
		if len(server) == 0 {
			return
		}
		present = true
		if id := strings.TrimSpace(firstNonEmptyString(stringFromMap(server, "id"), stringFromMap(server, "serverId"))); id != "" {
			ret[id] = true
		}
	}
	addCapability := func(capability map[string]any) {
		if len(capability) == 0 {
			return
		}
		present = true
		if id := strings.TrimSpace(stringFromMap(capability, "serverId")); id != "" {
			ret[id] = true
		}
	}
	addCapabilityID := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		present = true
		if idx := strings.Index(value, ":"); idx > 0 {
			ret[strings.TrimSpace(value[:idx])] = true
		}
	}
	var visit func(map[string]any)
	visit = func(source map[string]any) {
		if len(source) == 0 {
			return
		}
		for _, server := range mapSliceFromValue(source["mcpServers"]) {
			addServer(server)
		}
		for _, capability := range mapSliceFromValue(source["mcpCapabilities"]) {
			addCapability(capability)
		}
		for _, id := range stringSliceFromValue(source["capabilityIds"]) {
			addCapabilityID(id)
		}
		if selected := mapFromValue(source["selectedContext"]); len(selected) > 0 {
			visit(selected)
		}
		if runtimeCapabilities := mapFromValue(source["runtimeCapabilities"]); len(runtimeCapabilities) > 0 {
			visit(runtimeCapabilities)
		}
	}
	for _, source := range sources {
		visit(source)
	}
	return ret, present
}

func graphNodeRequiresMCPServerDeclaration(node runtimemodel.BlockExecutionNode) bool {
	switch strings.TrimSpace(node.Kind) {
	case "mcp/tool_call", "mcp/resource_read", "mcp/prompt_get":
		return true
	default:
		return false
	}
}

func graphNodeMCPServerID(node runtimemodel.BlockExecutionNode) string {
	if serverID := strings.TrimSpace(stringFromMap(node.Input, "serverId")); serverID != "" {
		return serverID
	}
	return strings.TrimSpace(stringFromMap(mapFromValue(node.Input["input"]), "serverId"))
}

func stringSet(values []string) map[string]bool {
	ret := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			ret[value] = true
		}
	}
	return ret
}

func stringListAllows(values []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}
