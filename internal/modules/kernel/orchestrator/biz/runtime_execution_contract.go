package orchestrator

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var shellAssignmentPattern = regexp.MustCompile(`(?m)([a-z_][a-z0-9_]*)=("[^"]*"|'[^']*'|[^ \t\r\n;]+)`)

func validateLLMExecutionOutputContract(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if run == nil || step == nil || output == nil || step.Kind != initialStepKind(run) {
		return nil
	}
	stage := runtimeStageFromStepInput(step.Input)
	if err := validateAssessmentOnlyOutputContract(step, output); err != nil {
		return err
	}
	if err := validateIncrementalArtifactOutputContract(run, step, output); err != nil {
		return err
	}
	normalizeManagedValidationOutput(step, output)
	if err := validateLLMExecutionGraphOutputContract(step, output); err != nil {
		return err
	}
	if err := validateLLMNextStepPatchTargetRelevance(step, output); err != nil {
		return err
	}
	if err := validateLLMRepairCoverageFocusBindingContract(step, output); err != nil {
		return err
	}
	if err := validatePrimaryDiagnosticPatchOutputContract(step, output); err != nil {
		return err
	}
	if err := validateLLMSoulGraphCapabilityContract(run, step, output); err != nil {
		return err
	}
	if err := validateLLMCapabilityUsagePolicyContract(run, step, output); err != nil {
		return err
	}
	if err := validateLLMFollowUpGraphStateChangingContract(run, step, output); err != nil {
		return err
	}
	if err := validateLLMCoverageBoundStateChangingIntent(run, step, output); err != nil {
		return err
	}
	if err := validateLLMRepairStrategyReassessmentContract(step, output); err != nil {
		return err
	}
	if err := validateScopedStateChangingRepairVerificationContract(step, output); err != nil {
		return err
	}
	if err := validateRuntimeRepairFailureDiagnosticsContract(step); err != nil {
		return err
	}
	if err := validateConcreteValidationEvidenceOutputContract(step, output); err != nil {
		return err
	}
	if boolValue(step.Input["stateChangingOutputRequired"]) {
		if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
			if outputModeIsIncrementalArtifact(step.Input) && artifactOutputPhaseFromInput(step.Input) == artifactOutputPhaseContext {
				return nil
			}
			if err := validateExecutionGraphShellSafety(graph, "state-changing execution graph"); err != nil {
				return err
			}
			if allowPatchRepairValidationEvidenceGraph(step, graph) {
				return nil
			}
			if allowDeclaredToolStateChangingRepairGraph(step, graph) {
				return nil
			}
			if allowAcceptedStateChangingEvidenceGraph(step, graph) {
				return nil
			}
			if err := validateStateChangingExecutionGraph(graph); err != nil {
				return err
			}
			return nil
		}
		if hasStateChangingLLMExecutionOutput(output) {
			return nil
		}
		if outputModeIsIncrementalArtifact(step.Input) &&
			(artifactOutputFieldPresent(output, "artifactPlan") ||
				artifactOutputFieldPresent(output, "artifactChunk") ||
				artifactOutputFieldPresent(output, "artifactChunkRepair")) {
			return nil
		}
		if assessment := doDAssessmentFromOutput(output); assessment != nil {
			if boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) && boolValue(assessment["satisfied"]) {
				return nil
			}
			return newRuntimeContractViolationError(contractViolationRepairNoStateChangingOutput, "state-changing llm repair step returned dodAssessment without executable state-changing output")
		}
		if spec := decisionBlockSpec(output); spec.Reason != "" {
			if !runtimeDecisionBlockSpecIsStructurallyValid(spec) {
				return newRuntimeContractViolationError(contractViolationRepairNoStateChangingOutput, "state-changing llm repair step returned incomplete decision blocker without executable state-changing output")
			}
			return nil
		}
		if spec := approvalBlockSpec(output); spec.Reason != "" {
			if !runtimeApprovalBlockSpecIsStructurallyValid(spec) {
				return newRuntimeContractViolationError(contractViolationRepairNoStateChangingOutput, "state-changing llm repair step returned incomplete approval blocker without executable state-changing output")
			}
			return nil
		}
		return newRuntimeContractViolationError(contractViolationRepairNoStateChangingOutput, "state-changing llm repair step returned no state-changing output")
	}
	if graphNodeID(step) == "" {
		return nil
	}
	if stage != runtimeStageExecution {
		return nil
	}
	if llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		return newRuntimeContractViolationError(contractViolationGraphNodeReturnedGraph, "execution-stage llm graph node returned executionGraph without concrete output")
	}
	if !graphNodeRequiresStateChangingOutput(step) {
		return nil
	}
	if hasStateChangingLLMExecutionOutput(output) || doDAssessmentFromOutput(output) != nil {
		return nil
	}
	if spec := decisionBlockSpec(output); spec.Reason != "" {
		return nil
	}
	if spec := approvalBlockSpec(output); spec.Reason != "" {
		return nil
	}
	return newRuntimeContractViolationError(contractViolationExecutionNoStateChangingOutput, "state-changing execution-stage llm graph node returned no state-changing output")
}

func runtimeDecisionBlockSpecIsStructurallyValid(spec runtimeDecisionBlockSpec) bool {
	return strings.TrimSpace(spec.Reason) != "" && strings.TrimSpace(spec.Question) != ""
}

func runtimeApprovalBlockSpecIsStructurallyValid(spec runtimeApprovalBlockSpec) bool {
	return strings.TrimSpace(spec.Reason) != "" && strings.TrimSpace(spec.Action) != ""
}

func validateAssessmentOnlyOutputContract(step *runModel.Step, output map[string]any) error {
	if step == nil || step.Input == nil || output == nil {
		return nil
	}
	if !boolValue(step.Input[stepInputKeyAssessmentOnly]) && !boolValue(step.Input["providerRetryAssessmentOnly"]) {
		return nil
	}
	fields := assessmentOnlyExecutableOutputFields(output)
	if len(fields) == 0 {
		return nil
	}
	return newRuntimeContractViolationError(
		contractViolationAssessmentOnlyExecutableOutput,
		fmt.Sprintf("assessment-only step returned executable or state-changing output fields: %s", strings.Join(fields, ", ")),
	)
}

func assessmentOnlyExecutableOutputFields(output map[string]any) []string {
	if output == nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, field := range assessmentOnlyExecutableFieldsInPayload(output, "") {
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
	}
	ret := stringStructKeys(seen)
	sort.Strings(ret)
	return ret
}

func assessmentOnlyExecutableFieldsInPayload(payload map[string]any, prefix string) []string {
	if payload == nil {
		return nil
	}
	ret := []string{}
	for _, key := range []string{
		stepOutputKeyExecutionGraph,
		"shellCommands",
		"patchRequests",
		"mcpToolCalls",
		stepOutputKeyMemoryWrites,
		stepOutputKeyMemoryRefs,
		"nextAction",
		stepOutputKeyNextStep,
		stepOutputKeyNextKind,
		stepOutputKeyNextInput,
		stepOutputKeyPatches,
		"artifactPlan",
		"artifactChunk",
		"artifactChunkRepair",
	} {
		if !assessmentOnlyOutputFieldPresent(payload, key) {
			continue
		}
		ret = append(ret, prefix+key)
	}
	for _, nestedKey := range []string{stepOutputKeyLLMDecision} {
		if nested := mapFromValue(payload[nestedKey]); len(nested) > 0 {
			ret = append(ret, assessmentOnlyExecutableFieldsInPayload(nested, prefix+nestedKey+".")...)
		}
	}
	return ret
}

func assessmentOnlyOutputFieldPresent(payload map[string]any, key string) bool {
	value, ok := payload[key]
	if !ok || value == nil {
		return false
	}
	if key == stepOutputKeyExecutionGraph {
		_, ok := runtimeGraphSpecFromValue(value)
		return ok
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case []map[string]any:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func validateConcreteValidationEvidenceOutputContract(step *runModel.Step, output map[string]any) error {
	if step == nil || step.Input == nil || output == nil || !inputRequiresConcreteValidationEvidence(step.Input) {
		return nil
	}
	if outputModeIsIncrementalArtifact(step.Input) && artifactOutputPhaseFromInput(step.Input) == artifactOutputPhaseContext {
		return nil
	}
	normalizeManagedValidationOutput(step, output)
	normalizeConcreteValidationScratchSetupOutput(step, output)
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		if !graphIsConcreteValidationEvidenceGraph(graph) {
			return newRuntimeContractViolationError(contractViolationValidationEvidenceRequired, "concrete validation evidence repair must return bounded shell/command or mcp/tool_call executionGraph or a satisfied dodAssessment with concrete evidenceRefs")
		}
		if err := validateConcreteValidationBehaviorEvidenceGraph(step, graph); err != nil {
			return err
		}
		return nil
	}
	if spec := decisionBlockSpec(output); spec.Reason != "" {
		return nil
	}
	if spec := approvalBlockSpec(output); spec.Reason != "" {
		return nil
	}
	if assessment := doDAssessmentFromOutput(output); assessment != nil {
		if boolValue(assessment["satisfied"]) {
			return nil
		}
		return newRuntimeContractViolationError(contractViolationValidationEvidenceRequired, "concrete validation evidence repair must return bounded shell/command or mcp/tool_call executionGraph; plain failed dodAssessment does not satisfy repairActionPackage.requiredOutput")
	}
	return newRuntimeContractViolationError(contractViolationValidationEvidenceRequired, "concrete validation evidence repair must return bounded shell/command or mcp/tool_call executionGraph or a satisfied dodAssessment with concrete evidenceRefs")
}

func normalizeConcreteValidationScratchSetupOutput(step *runModel.Step, output map[string]any) {
	if step == nil || output == nil {
		return
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		return
	}
	normalized, changed := normalizeConcreteValidationScratchSetupGraph(step, graph)
	if !changed {
		return
	}
	output[stepOutputKeyExecutionGraph] = normalized
}

func normalizeConcreteValidationScratchSetupGraph(step *runModel.Step, graph runtimemodel.BlockExecutionGraph) (runtimemodel.BlockExecutionGraph, bool) {
	scratch := validationScratchDirFromStep(step)
	if scratch == "" || len(graph.Nodes) == 0 {
		return graph, false
	}
	changed := false
	nodes := make([]runtimemodel.BlockExecutionNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		next, ok := normalizeConcreteValidationScratchSetupNode(node, scratch)
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

func normalizeConcreteValidationScratchSetupNode(node runtimemodel.BlockExecutionNode, validationScratchDir string) (runtimemodel.BlockExecutionNode, bool) {
	if canonicalRuntimeGraphNodeKind(node.Kind) != "shell/command" || strings.TrimSpace(validationScratchDir) == "" {
		return node, false
	}
	mode := nodeValidationEvidenceMode(node)
	if !validationEvidenceModeAllowsScratchWrites(mode) {
		return node, false
	}
	target, ok := shellCommandLineSingleMkdirTarget(node.Input)
	if !ok || strings.TrimSpace(target) == "" {
		return node, false
	}
	if shellCommandLineMentionsPath(target, validationScratchDir) {
		return node, false
	}
	evidence := mapFromValue(node.Input["validationEvidence"])
	intent := strings.ToLower(strings.Join([]string{
		node.ID,
		stringFromMap(evidence, "substituteFor"),
		stringFromMap(evidence, "substitute"),
		target,
	}, " "))
	if !strings.Contains(intent, "scratch") {
		return node, false
	}
	next := node
	next.Input = cloneStepPayload(node.Input)
	next.Input["validationScratchDir"] = validationScratchDir
	next.Input["validationScratchDirNormalized"] = true
	if normalizeShellMkdirInputToScratch(next.Input, validationScratchDir) {
		return next, true
	}
	return node, false
}

func shellCommandLineSingleMkdirTarget(input map[string]any) (string, bool) {
	if len(input) == 0 {
		return "", false
	}
	command := strings.TrimSpace(stringFromMap(input, "command"))
	args := stringSliceFromValue(input["args"])
	if command == "mkdir" {
		return shellMkdirArgsSingleTarget(args)
	}
	if command != "sh" && command != "bash" {
		return "", false
	}
	if len(args) != 2 || strings.TrimSpace(args[0]) != "-c" {
		return "", false
	}
	return shellMkdirScriptSingleTarget(args[1])
}

func shellMkdirScriptSingleTarget(script string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(script))
	return shellMkdirArgsSingleTarget(fields)
}

func shellMkdirArgsSingleTarget(args []string) (string, bool) {
	if len(args) < 2 {
		return "", false
	}
	if strings.TrimSpace(args[0]) != "-p" && strings.TrimSpace(args[0]) != "mkdir" {
		return "", false
	}
	start := 0
	if strings.TrimSpace(args[0]) == "mkdir" {
		start = 1
	}
	if start >= len(args) || strings.TrimSpace(args[start]) != "-p" {
		return "", false
	}
	if len(args) != start+2 {
		return "", false
	}
	return strings.Trim(strings.TrimSpace(args[start+1]), `"'`), true
}

func normalizeShellMkdirInputToScratch(input map[string]any, validationScratchDir string) bool {
	command := strings.TrimSpace(stringFromMap(input, "command"))
	args := stringSliceFromValue(input["args"])
	if command == "mkdir" {
		input["args"] = []string{"-p", validationScratchDir}
		return true
	}
	if (command == "sh" || command == "bash") && len(args) == 2 && strings.TrimSpace(args[0]) == "-c" {
		input["args"] = []string{"-c", "mkdir -p " + strconv.Quote(validationScratchDir)}
		return true
	}
	return false
}

func validateConcreteValidationBehaviorEvidenceGraph(step *runModel.Step, graph runtimemodel.BlockExecutionGraph) error {
	if step == nil || step.Input == nil {
		return nil
	}
	required := mapFromValue(mapFromValue(step.Input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustPreferExecutableBehaviorValidation"]) &&
		!boolValue(required["staticInspectionOnlyMayNotSatisfyRuntimeBehavior"]) &&
		!boolValue(required["mustDeclareValidationEvidenceMode"]) &&
		!boolValue(required["mustDeclareValidationEvidenceObservation"]) {
		return nil
	}
	if graphHasDeclaredConcreteValidationEvidenceNode(graph) {
		return validateConcreteValidationGraphShellSafety(step, graph)
	}
	return newRuntimeContractViolationError(contractViolationValidationEvidenceRequired, "concrete validation evidence repair returned executionGraph without a declared non-static validationEvidence contract; shell/command or mcp/tool_call nodes must declare validationEvidence.mode and validationEvidence.observationType, and controlled_substitute must declare validationEvidence.substituteFor or validationEvidence.substitute")
}

func validateConcreteValidationGraphShellSafety(step *runModel.Step, graph runtimemodel.BlockExecutionGraph) error {
	validationScratchDir := validationScratchDirFromStep(step)
	for _, node := range graph.Nodes {
		if canonicalRuntimeGraphNodeKind(node.Kind) != "shell/command" {
			continue
		}
		commandLine := runtimeGraphShellCommandLine(node.Input)
		if commandLine == "" {
			continue
		}
		if shellCommandLineContainsProcessTermination(commandLine) {
			return newRuntimeContractViolationError(contractViolationValidationEvidenceRequired, "concrete validation evidence graph must use non-destructive bounded validation; shell process termination is not allowed")
		}
		if shellCommandLineStartsDetachedProcess(commandLine) {
			return newRuntimeContractViolationError(contractViolationValidationEvidenceRequired, "concrete validation evidence graph must not start detached or background processes; use a bounded foreground command, timeout-controlled harness, or controlled substitute")
		}
		allowScratchWrites := validationEvidenceModeAllowsScratchWrites(nodeValidationEvidenceMode(node))
		if shellCommandLineContainsWorkspaceFileMutation(commandLine, allowScratchWrites, validationScratchDir) {
			if allowScratchWrites && validationScratchDir == "" {
				return newRuntimeContractViolationError(contractViolationValidationEvidenceRequired, "concrete validation evidence graph with controlled substitute writes requires runtime validationScratchDir")
			}
			return newRuntimeContractViolationError(contractViolationValidationEvidenceRequired, "concrete validation evidence graph must not mutate workspace files or write outside validationScratchDir through shell/command; use patch/apply for file state changes and keep validation shell commands read-only or scratch-bounded")
		}
	}
	return nil
}

func validationEvidenceModeAllowsScratchWrites(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "controlled_substitute", "dynamic_execution", "executable_contract", "runtime_behavior":
		return true
	default:
		return false
	}
}

func validateExecutionGraphShellSafety(graph runtimemodel.BlockExecutionGraph, scope string) error {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "execution graph"
	}
	for _, node := range graph.Nodes {
		if canonicalRuntimeGraphNodeKind(node.Kind) != "shell/command" {
			continue
		}
		commandLine := runtimeGraphShellCommandLine(node.Input)
		if commandLine == "" {
			continue
		}
		if shellCommandLineContainsProcessTermination(commandLine) {
			return fmt.Errorf("%w: %s must use non-destructive bounded shell commands; process termination is not allowed", common.ErrInvalidArgument, scope)
		}
		if shellCommandLineStartsDetachedProcess(commandLine) {
			return fmt.Errorf("%w: %s must not start detached or background processes; use a bounded foreground command, timeout-controlled harness, or controlled substitute", common.ErrInvalidArgument, scope)
		}
	}
	return nil
}

func runtimeGraphShellCommandLine(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	parts := []string{strings.TrimSpace(stringFromMap(input, "command"))}
	parts = append(parts, stringSliceFromValue(input["args"])...)
	return strings.TrimSpace(strings.Join(parts, " "))
}

func shellCommandLineContainsProcessTermination(commandLine string) bool {
	padded := " " + strings.ToLower(strings.TrimSpace(commandLine)) + " "
	for _, marker := range []string{
		" pkill ",
		" killall ",
		" xargs kill",
		" fuser -k",
		" kill -",
		" kill ",
	} {
		if strings.Contains(padded, marker) {
			return true
		}
	}
	return false
}

func shellCommandLineStartsDetachedProcess(commandLine string) bool {
	lower := strings.ToLower(strings.TrimSpace(commandLine))
	if lower == "" {
		return false
	}
	padded := " " + lower + " "
	if strings.Contains(padded, " nohup ") || strings.Contains(padded, " setsid ") {
		return true
	}
	return shellCommandLineContainsStandaloneAmpersand(lower)
}

func shellCommandLineContainsWorkspaceFileMutation(commandLine string, allowScratchWrites bool, validationScratchDir string) bool {
	lower := strings.ToLower(strings.TrimSpace(commandLine))
	if lower == "" {
		return false
	}
	if allowScratchWrites && strings.TrimSpace(validationScratchDir) != "" && shellCommandLineReferencesValidationScratch(lower, validationScratchDir) {
		return shellCommandLineContainsWorkspaceFileMutationOutsideValidationScratch(lower, validationScratchDir)
	}
	padded := " " + lower + " "
	for _, marker := range []string{
		" sed -i",
		" perl -pi",
		" truncate ",
		" touch ",
		" rm ",
		" rm -",
		" mv ",
		" cp ",
		" install ",
		" mkdir ",
		" rmdir ",
		" ln ",
		" chmod ",
		" chown ",
		" dd ",
		" tee ",
		" patch ",
		" git apply",
		" git checkout",
		" git restore",
	} {
		if strings.Contains(padded, marker) {
			return true
		}
	}
	return shellCommandLineContainsWriteRedirection(lower, false, "")
}

func shellCommandLineContainsWorkspaceFileMutationOutsideValidationScratch(commandLine string, validationScratchDir string) bool {
	scratchAliases := shellCommandLineValidationScratchAliases(commandLine, validationScratchDir)
	inScratchCWD := false
	for _, segment := range strings.FieldsFunc(commandLine, func(r rune) bool {
		return r == '\n' || r == ';'
	}) {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		segmentScratchScoped := inScratchCWD || shellCommandLineSegmentRunsInValidationScratch(segment, validationScratchDir, scratchAliases...)
		padded := " " + segment + " "
		for _, marker := range []string{
			" sed -i",
			" perl -pi",
			" truncate ",
			" touch ",
			" mv ",
			" install ",
			" ln ",
			" chmod ",
			" chown ",
			" dd ",
			" tee ",
			" patch ",
			" git apply",
			" git checkout",
			" git restore",
		} {
			if strings.Contains(padded, marker) {
				if !segmentScratchScoped && !shellCommandLineSegmentReferencesValidationScratch(segment, validationScratchDir, scratchAliases...) {
					return true
				}
				break
			}
		}
		for _, marker := range []string{" rm ", " rm -", " mkdir ", " rmdir ", " cp "} {
			if strings.Contains(padded, marker) && !segmentScratchScoped && !shellCommandLineSegmentReferencesValidationScratch(segment, validationScratchDir, scratchAliases...) {
				return true
			}
		}
		if !segmentScratchScoped && shellCommandLineContainsWriteRedirection(segment, true, validationScratchDir, scratchAliases...) {
			return true
		}
		if shellCommandLineSegmentChangesDirectory(segment) {
			inScratchCWD = shellCommandLineSegmentChangesDirectoryToValidationScratch(segment, validationScratchDir, scratchAliases...)
		}
	}
	return false
}

func shellCommandLineReferencesValidationScratch(commandLine string, validationScratchDir string) bool {
	return shellCommandLineSegmentReferencesValidationScratch(commandLine, validationScratchDir, shellCommandLineValidationScratchAliases(commandLine, validationScratchDir)...)
}

func shellCommandLineSegmentRunsInValidationScratch(segment string, validationScratchDir string, scratchAliases ...string) bool {
	segment = strings.ToLower(strings.TrimSpace(segment))
	if segment == "" || !shellCommandLineSegmentReferencesValidationScratch(segment, validationScratchDir, scratchAliases...) {
		return false
	}
	padded := " " + segment + " "
	return strings.Contains(padded, " cd ") || strings.Contains(segment, "(cd ")
}

func shellCommandLineSegmentChangesDirectory(segment string) bool {
	segment = strings.ToLower(strings.TrimSpace(segment))
	if segment == "" {
		return false
	}
	return strings.HasPrefix(segment, "cd ") || strings.HasPrefix(segment, "cd\t")
}

func shellCommandLineSegmentChangesDirectoryToValidationScratch(segment string, validationScratchDir string, scratchAliases ...string) bool {
	if !shellCommandLineSegmentChangesDirectory(segment) {
		return false
	}
	return shellCommandLineSegmentReferencesValidationScratch(segment, validationScratchDir, scratchAliases...)
}

func shellCommandLineValidationScratchAliases(commandLine string, validationScratchDir string) []string {
	commandLine = strings.ToLower(commandLine)
	validationScratchDir = strings.ToLower(strings.TrimSpace(normalizeValidationScratchDir(validationScratchDir)))
	aliases := map[string]struct{}{}
	for changed := true; changed; {
		changed = false
		for _, match := range shellAssignmentPattern.FindAllStringSubmatch(commandLine, -1) {
			if len(match) < 3 {
				continue
			}
			name := strings.TrimSpace(match[1])
			value := strings.Trim(strings.TrimSpace(match[2]), `"'`)
			if name == "" {
				continue
			}
			if _, exists := aliases[name]; exists {
				continue
			}
			if shellCommandLineSegmentReferencesValidationScratch(value, validationScratchDir, stringStructKeys(aliases)...) {
				aliases[name] = struct{}{}
				changed = true
			}
		}
	}
	return stringStructKeys(aliases)
}

func stringStructKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func shellCommandLineSegmentReferencesValidationScratch(segment string, validationScratchDir string, scratchAliases ...string) bool {
	segment = strings.ToLower(segment)
	validationScratchDir = strings.ToLower(strings.TrimSpace(normalizeValidationScratchDir(validationScratchDir)))
	if validationScratchDir != "" && shellCommandLineMentionsPath(segment, validationScratchDir) {
		return true
	}
	for _, alias := range scratchAliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias == "" {
			continue
		}
		if strings.Contains(segment, "$"+alias) || strings.Contains(segment, "${"+alias+"}") || strings.Contains(segment, "${"+alias) {
			return true
		}
	}
	return false
}

func shellCommandLineMentionsPath(segment string, path string) bool {
	segment = strings.ToLower(segment)
	path = strings.ToLower(strings.TrimSpace(normalizeValidationScratchDir(path)))
	if segment == "" || path == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(segment[start:], path)
		if idx < 0 {
			return false
		}
		idx += start
		beforeOK := idx == 0 || shellCommandLinePathBoundaryBefore(segment[idx-1])
		afterIdx := idx + len(path)
		afterOK := afterIdx >= len(segment) || segment[afterIdx] == '/' || shellCommandLinePathBoundaryAfter(segment[afterIdx])
		if beforeOK && afterOK {
			return true
		}
		start = idx + len(path)
	}
}

func shellCommandLinePathBoundaryBefore(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\'', '"', '`', '=', ':', '(', '{', '[', '<', '>':
		return true
	default:
		return false
	}
}

func shellCommandLinePathBoundaryAfter(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\'', '"', '`', ';', '&', '|', ')', '}', ']', '<', '>':
		return true
	default:
		return false
	}
}

func shellCommandLineContainsWriteRedirection(commandLine string, allowScratchTarget bool, validationScratchDir string, scratchAliases ...string) bool {
	for idx := 0; idx < len(commandLine); idx++ {
		if commandLine[idx] != '>' {
			continue
		}
		if shellCommandLineGreaterThanIsComparison(commandLine, idx) {
			continue
		}
		next := byte(0)
		if idx+1 < len(commandLine) {
			next = commandLine[idx+1]
		}
		if next == '&' {
			continue
		}
		targetStart := idx + 1
		if next == '>' {
			targetStart++
		}
		for targetStart < len(commandLine) && strings.ContainsRune(" \t\r\n", rune(commandLine[targetStart])) {
			targetStart++
		}
		targetEnd := targetStart
		for targetEnd < len(commandLine) && !strings.ContainsRune(" \t\r\n;|&(){}", rune(commandLine[targetEnd])) {
			targetEnd++
		}
		if targetEnd > targetStart {
			target := strings.Trim(commandLine[targetStart:targetEnd], `"'`)
			if target == "/dev/null" || (allowScratchTarget && shellCommandLineSegmentReferencesValidationScratch(target, validationScratchDir, scratchAliases...)) {
				continue
			}
		}
		return true
	}
	return false
}

func shellCommandLineGreaterThanIsComparison(commandLine string, idx int) bool {
	if idx < 0 || idx >= len(commandLine) || commandLine[idx] != '>' {
		return false
	}
	if idx+1 < len(commandLine) && commandLine[idx+1] == '=' {
		return true
	}
	if idx > 0 {
		switch commandLine[idx-1] {
		case '=', '-', '!':
			return true
		}
	}
	return false
}

func nodeValidationEvidenceMode(node runtimemodel.BlockExecutionNode) string {
	evidence := mapFromValue(node.Input["validationEvidence"])
	mode := strings.TrimSpace(stringFromMap(evidence, "mode"))
	if mode == "" {
		mode = strings.TrimSpace(stringFromMap(node.Input, "validationEvidenceMode"))
	}
	if mode == "" {
		mode = strings.TrimSpace(stringFromMap(node.Input, "evidenceMode"))
	}
	return mode
}

func shellCommandLineContainsStandaloneAmpersand(commandLine string) bool {
	for idx, value := range commandLine {
		if value != '&' {
			continue
		}
		prevBoundary := idx == 0 || strings.ContainsRune(" \t\r\n;(){}|", rune(commandLine[idx-1]))
		nextIdx := idx + len(string(value))
		nextBoundary := nextIdx >= len(commandLine) || strings.ContainsRune(" \t\r\n;(){}|", rune(commandLine[nextIdx]))
		if prevBoundary && nextBoundary {
			return true
		}
	}
	return false
}

func graphHasDeclaredConcreteValidationEvidenceNode(graph runtimemodel.BlockExecutionGraph) bool {
	for _, node := range graph.Nodes {
		switch strings.TrimSpace(node.Kind) {
		case "shell/command", "mcp/tool_call":
			if nodeValidationEvidenceContractIsConcrete(node) {
				return true
			}
		}
	}
	return false
}

func nodeValidationEvidenceContractIsConcrete(node runtimemodel.BlockExecutionNode) bool {
	evidence := mapFromValue(node.Input["validationEvidence"])
	mode := strings.TrimSpace(stringFromMap(evidence, "mode"))
	if mode := strings.TrimSpace(stringFromMap(node.Input, "validationEvidenceMode")); mode != "" {
		evidence["mode"] = mode
	}
	if mode == "" {
		mode = strings.TrimSpace(stringFromMap(node.Input, "evidenceMode"))
	}
	if !validationEvidenceModeIsConcrete(mode) {
		return false
	}
	observationType := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(evidence, "observationType"),
		stringFromMap(evidence, "observation"),
		stringFromMap(node.Input, "validationObservationType"),
	))
	if !validationEvidenceObservationTypeIsConcrete(observationType) {
		return false
	}
	if mode == "controlled_substitute" &&
		len(mapFromValue(evidence["substitute"])) == 0 &&
		strings.TrimSpace(stringFromMap(evidence, "substituteFor")) == "" {
		return false
	}
	return true
}

func validationEvidenceModeIsConcrete(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "runtime_behavior", "runtime_observation", "controlled_substitute", "controlled_substitute_observation", "executable_contract", "executable_contract_observation", "dynamic_execution", "dynamic_execution_observation":
		return true
	default:
		return false
	}
}

func validationEvidenceObservationTypeIsConcrete(observationType string) bool {
	switch strings.TrimSpace(observationType) {
	case "runtime_observation", "controlled_substitute_observation", "executable_contract_observation", "dynamic_execution_observation":
		return true
	default:
		return false
	}
}

func validateRuntimeStateChangingOutputBeforeSideEffects(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	if run == nil || step == nil || output == nil {
		return nil
	}
	if step.Kind == initialStepKind(run) {
		return validateLLMExecutionOutputContract(run, step, output)
	}
	if strings.TrimSpace(step.Kind) != "patch/apply" {
		return nil
	}
	return validatePatchStepCoverageTargetsBeforeRuntime(run, step, output)
}

func validateScopedStateChangingRepairVerificationContract(step *runModel.Step, output map[string]any) error {
	if step == nil || step.Input == nil || output == nil {
		return nil
	}
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	if runtimeStageFromStepInput(step.Input) != runtimeStageRepair {
		return nil
	}
	if !strategyFailedSurfaceRequiresBoundedVerification(
		strings.TrimSpace(stringFromMap(step.Input, "failureTargetScope")),
		mapFromValue(step.Input["strategyReassessment"]),
	) {
		return nil
	}
	if !strategyOutputGraphUsesUnboundedVerificationForStep(step, output) {
		return nil
	}
	return fmt.Errorf("%w: scoped state-changing runtime repair requires bounded verification; unbounded verification without explicit target scope is not allowed", common.ErrInvalidArgument)
}

func validateRuntimeRepairFailureDiagnosticsContract(step *runModel.Step) error {
	if step == nil || step.Input == nil {
		return nil
	}
	if !stateChangingRepairContractActive(step) {
		return nil
	}
	if strings.TrimSpace(stringFromMap(step.Input, "failedStepId")) == "" &&
		strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfStepID)) == "" {
		return nil
	}
	diagnostics := runtimeRepairFailureDiagnosticsFromInput(step.Input)
	if strings.TrimSpace(firstNonEmptyString(
		stringFromMap(diagnostics, "failureSignature"),
		stringFromMap(diagnostics, "failureClass"),
		stringFromMap(diagnostics, "error"),
	)) == "" {
		if runtimeRepairFailureDiagnosticsOptionalForCoverageContinuation(step.Input) {
			return nil
		}
		return fmt.Errorf("%w: state-changing runtime repair requires lastFailureDiagnostics or equivalent failedStepOutput diagnostics", common.ErrInvalidArgument)
	}
	return nil
}

func runtimeRepairFailureDiagnosticsOptionalForCoverageContinuation(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if !boolValue(input["coverageRepairRequired"]) && !boolValue(pkg["coverageRepairRequired"]) {
		return false
	}
	if runtimeRepairDiagnosticsHasSignal(mapFromValue(input["currentFailure"])) ||
		runtimeRepairDiagnosticsHasSignal(mapFromValue(pkg["currentFailure"])) ||
		runtimeRepairDiagnosticsHasSignal(mapFromValue(mapFromValue(input["currentProgressDigest"])["lastObservedFailure"])) ||
		runtimeRepairDiagnosticsHasSignal(mapFromValue(mapFromValue(input["currentExecutionProgress"])["lastObservedFailure"])) ||
		runtimeRepairDiagnosticsHasSignal(mapFromValue(mapFromValue(pkg["currentExecutionProgress"])["lastObservedFailure"])) {
		return false
	}
	if !runtimeRepairProgressHasNoCurrentFailure(input) {
		return false
	}
	return runtimeRepairInputHasUnresolvedCoverageTarget(input)
}

func runtimeRepairProgressHasNoCurrentFailure(input map[string]any) bool {
	for _, progress := range []map[string]any{
		mapFromValue(input["currentProgressDigest"]),
		mapFromValue(input["currentExecutionProgress"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["currentExecutionProgress"]),
	} {
		if len(progress) == 0 {
			continue
		}
		if strings.TrimSpace(stringFromMap(progress, "currentGap")) == "none_observed_in_recent_process_evidence" &&
			strings.TrimSpace(stringFromMap(progress, "nextRequiredAction")) == "continue_current_stage_contract" {
			return true
		}
	}
	return false
}

func runtimeRepairInputHasUnresolvedCoverageTarget(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if coverageTargetSetHasMissingEvidence(mapSliceFromValue(input["coverageTargetSet"])) {
		return true
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if coverageTargetSetHasMissingEvidence(mapSliceFromValue(pkg["coverageTargetSet"])) {
		return true
	}
	if coverageWorkItemContextHasMissingEvidence(mapFromValue(input["coverageWorkItemContext"])) {
		return true
	}
	return coverageWorkItemContextHasMissingEvidence(mapFromValue(pkg["coverageWorkItemContext"]))
}

func coverageTargetSetHasMissingEvidence(targets []map[string]any) bool {
	for _, target := range targets {
		if deriveCoverageWorkItemState(target).HasActionableEvidenceGap() {
			return true
		}
	}
	return false
}

func coverageWorkItemContextHasMissingEvidence(context map[string]any) bool {
	if len(context) == 0 {
		return false
	}
	if deriveCoverageWorkItemState(mapFromValue(context["currentFocus"])).HasActionableEvidenceGap() {
		return true
	}
	for _, item := range mapSliceFromValue(context["currentFocusItems"]) {
		if deriveCoverageWorkItemState(item).HasActionableEvidenceGap() {
			return true
		}
	}
	return false
}

func runtimeRepairFailureDiagnosticsFromInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	for _, candidate := range []map[string]any{
		mapFromValue(input["lastFailureDiagnostics"]),
		mapFromValue(input["failedStepOutput"]),
		mapFromValue(input["currentFailure"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["currentFailure"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["lastFailureDiagnostics"]),
		mapFromValue(mapFromValue(input["repairActionPackage"])["lastObservedFailure"]),
		mapFromValue(mapFromValue(input["currentProgressDigest"])["lastObservedFailure"]),
	} {
		if runtimeRepairDiagnosticsHasSignal(candidate) {
			return map[string]any{
				"failureSignature": firstNonEmptyString(
					stringFromMap(candidate, failureOutputKeySignature),
					stringFromMap(input, failureOutputKeySignature),
				),
				"failureClass": firstNonEmptyString(
					stringFromMap(candidate, failureOutputKeyClass),
					stringFromMap(input, failureOutputKeyClass),
				),
				"error": firstNonEmptyString(
					stringFromMap(candidate, "error"),
					stringFromMap(candidate, "reason"),
					stringFromMap(candidate, "failureReason"),
					stringFromMap(input, "failureReason"),
				),
			}
		}
	}
	return nil
}

func runtimeRepairDiagnosticsHasSignal(diagnostics map[string]any) bool {
	if len(diagnostics) == 0 {
		return false
	}
	return strings.TrimSpace(firstNonEmptyString(
		stringFromMap(diagnostics, failureOutputKeySignature),
		stringFromMap(diagnostics, failureOutputKeyClass),
		stringFromMap(diagnostics, "error"),
		stringFromMap(diagnostics, "reason"),
		stringFromMap(diagnostics, "failureReason"),
	)) != ""
}
