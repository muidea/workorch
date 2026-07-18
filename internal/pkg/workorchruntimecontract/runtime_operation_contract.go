package workorchruntimecontract

import (
	"fmt"
	"strings"
)

type RuntimeOperationParameterType string

const (
	RuntimeOperationParameterTypeString     RuntimeOperationParameterType = "string"
	RuntimeOperationParameterTypeInteger    RuntimeOperationParameterType = "integer"
	RuntimeOperationParameterTypeBoolean    RuntimeOperationParameterType = "boolean"
	RuntimeOperationParameterTypeStringList RuntimeOperationParameterType = "string_list"
)

type RuntimeOperationParameterContract struct {
	Name        string
	Type        RuntimeOperationParameterType
	Required    bool
	Description string
}

type RuntimeOperationContract struct {
	Kind                   string
	Parameters             []RuntimeOperationParameterContract
	RequireOneOfParameters []string
	RequireTargetRefs      bool
	ForbiddenParameters    []string
	Rules                  []string
}

func RuntimeOperationContractForKind(kind string) (RuntimeOperationContract, bool) {
	switch CanonicalRuntimeBlockKind(kind) {
	case StepKindFileList:
		return RuntimeOperationContract{
			Kind: StepKindFileList,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "path", Type: RuntimeOperationParameterTypeString, Description: "workspace-relative directory path; defaults to workspace root"},
				{Name: "recursive", Type: RuntimeOperationParameterTypeBoolean, Description: "recursively enumerate descendants"},
				{Name: "maxEntries", Type: RuntimeOperationParameterTypeInteger, Description: "bounded maximum entry count"},
			},
			ForbiddenParameters: []string{"input", "output", "payload"},
			Rules:               []string{"maxEntries must be between 1 and 500 when provided"},
		}, true
	case StepKindFileRead:
		return RuntimeOperationContract{
			Kind: StepKindFileRead,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "path", Type: RuntimeOperationParameterTypeString, Required: true, Description: "workspace-relative file path"},
				{Name: "optional", Type: RuntimeOperationParameterTypeBoolean, Description: "return exists=false when the target is absent"},
				{Name: "maxBytes", Type: RuntimeOperationParameterTypeInteger, Description: "bounded maximum returned content bytes"},
				{Name: "startLine", Type: RuntimeOperationParameterTypeInteger, Description: "optional 1-based inclusive start line"},
				{Name: "endLine", Type: RuntimeOperationParameterTypeInteger, Description: "optional 1-based inclusive end line"},
			},
			ForbiddenParameters: []string{"input", "output", "payload"},
			Rules:               []string{"path must be non-empty", "maxBytes must be between 1 and 65536 when provided", "line bounds must be positive and ordered when provided"},
		}, true
	case StepKindFileWrite:
		return RuntimeOperationContract{
			Kind: StepKindFileWrite,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "path", Type: RuntimeOperationParameterTypeString, Description: "workspace-relative file path"},
				{Name: "targetRef", Type: RuntimeOperationParameterTypeString, Description: "workspace-relative target path"},
				{Name: "content", Type: RuntimeOperationParameterTypeString, Required: true, Description: "complete file content; an empty string is allowed"},
			},
			ForbiddenParameters: []string{"input", "output", "payload"},
			Rules:               []string{"target must be identified by path, targetRef, or operation targetRefs when provided"},
		}, true
	case StepKindFileEdit:
		return RuntimeOperationContract{
			Kind: StepKindFileEdit,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "path", Type: RuntimeOperationParameterTypeString, Required: true, Description: "workspace-relative file path"},
				{Name: "oldText", Type: RuntimeOperationParameterTypeString, Required: true, Description: "exact current text to replace"},
				{Name: "newText", Type: RuntimeOperationParameterTypeString, Required: true, Description: "replacement text; an empty string is allowed"},
				{Name: "baseHash", Type: RuntimeOperationParameterTypeString, Description: "optional current content hash"},
				{Name: "occurrence", Type: RuntimeOperationParameterTypeInteger, Description: "1-based match index when oldText is not unique"},
			},
			ForbiddenParameters: []string{"filePath", "fileContent", "targetRef", "targetRefs"},
			Rules: []string{
				"oldText must be non-empty and different from newText",
				"targetRefs must contain the same target as path",
			},
		}, true
	case RuntimeBlockKindPatchApply:
		return RuntimeOperationContract{
			Kind: RuntimeBlockKindPatchApply,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "targetRef", Type: RuntimeOperationParameterTypeString, Description: "preferred workspace-relative target path"},
				{Name: "path", Type: RuntimeOperationParameterTypeString, Description: "workspace-relative target path alias"},
				{Name: "patchType", Type: RuntimeOperationParameterTypeString, Required: true, Description: "file_write, text_replace, or unified_diff"},
				{Name: "content", Type: RuntimeOperationParameterTypeString, Description: "complete file_write content; an empty string is allowed"},
				{Name: "diff", Type: RuntimeOperationParameterTypeString, Description: "unified_diff content"},
				{Name: "oldText", Type: RuntimeOperationParameterTypeString, Description: "exact text_replace source text"},
				{Name: "newText", Type: RuntimeOperationParameterTypeString, Description: "text_replace replacement; an empty string is allowed"},
				{Name: "baseHash", Type: RuntimeOperationParameterTypeString, Description: "optional current content hash"},
				{Name: "occurrence", Type: RuntimeOperationParameterTypeInteger, Description: "1-based match index when oldText is not unique"},
				{Name: "coverageTargets", Type: RuntimeOperationParameterTypeStringList, Description: "coverage targets addressed by this state change"},
				{Name: "summary", Type: RuntimeOperationParameterTypeString, Description: "bounded change summary"},
			},
			RequireOneOfParameters: []string{"targetRef", "path"},
			RequireTargetRefs:      true,
			ForbiddenParameters:    []string{"filePath", "fileContent", "targetRefs"},
			Rules: []string{
				"file_write must provide content",
				"text_replace must provide non-empty oldText and a distinct newText",
				"unified_diff must provide non-empty diff",
				"exactly one canonical target must be resolved from targetRef or path",
				"targetRefs must contain the resolved target",
			},
		}, true
	case RuntimeBlockKindPatchPreview:
		contract, _ := RuntimeOperationContractForKind(RuntimeBlockKindPatchApply)
		contract.Kind = RuntimeBlockKindPatchPreview
		return contract, true
	case StepKindShellCommand, RuntimeBlockKindManagedValidationSession:
		return RuntimeOperationContract{
			Kind: CanonicalRuntimeBlockKind(kind),
			Parameters: []RuntimeOperationParameterContract{
				{Name: "command", Type: RuntimeOperationParameterTypeString, Required: true, Description: "executable or shell binary"},
				{Name: "args", Type: RuntimeOperationParameterTypeStringList, Description: "ordered command arguments"},
				{Name: "timeoutMillis", Type: RuntimeOperationParameterTypeInteger, Description: "bounded execution timeout in milliseconds"},
				{Name: "stateChanging", Type: RuntimeOperationParameterTypeBoolean, Description: "declares that the command is expected to change workspace state"},
				{Name: "targetRef", Type: RuntimeOperationParameterTypeString, Description: "primary workspace target"},
				{Name: "targetRefs", Type: RuntimeOperationParameterTypeStringList, Description: "workspace targets related to the command"},
				{Name: "relatedFailureTargets", Type: RuntimeOperationParameterTypeStringList, Description: "failure targets addressed by bounded verification"},
				{Name: "verificationScope", Type: RuntimeOperationParameterTypeString, Description: "bounded verification scope"},
				{Name: "boundedReason", Type: RuntimeOperationParameterTypeString, Description: "reason the command is bounded"},
				{Name: "validationScratchDir", Type: RuntimeOperationParameterTypeString, Description: "step-scoped validation scratch directory"},
				{Name: "validationScratchAutoCleanup", Type: RuntimeOperationParameterTypeBoolean, Description: "whether validation scratch is removed after execution"},
			},
			ForbiddenParameters: []string{"input", "output", "payload"},
			Rules: []string{
				"command must be non-empty",
				"timeoutMillis must be between 1000 and 600000 when provided",
				"stateChanging commands must identify at least one target through targetRef or targetRefs",
			},
		}, true
	case StepKindGitRead, StepKindGitWrite:
		return RuntimeOperationContract{
			Kind: CanonicalRuntimeBlockKind(kind),
			Parameters: []RuntimeOperationParameterContract{
				{Name: "operation", Type: RuntimeOperationParameterTypeString, Required: true, Description: "git sub-operation"},
				{Name: "timeoutMillis", Type: RuntimeOperationParameterTypeInteger, Description: "bounded execution timeout in milliseconds"},
				{Name: "pathspecs", Type: RuntimeOperationParameterTypeStringList, Description: "workspace-relative git pathspecs"},
				{Name: "ref", Type: RuntimeOperationParameterTypeString, Description: "git reference"},
				{Name: "baseRef", Type: RuntimeOperationParameterTypeString, Description: "base git reference"},
				{Name: "limit", Type: RuntimeOperationParameterTypeInteger, Description: "bounded log result limit"},
				{Name: "message", Type: RuntimeOperationParameterTypeString, Description: "commit message"},
				{Name: "expectedHead", Type: RuntimeOperationParameterTypeString, Description: "optional optimistic HEAD value"},
			},
			ForbiddenParameters: []string{"command", "args", "input", "payload"},
			Rules: []string{
				"git/read accepts status, diff, log, show, or branch",
				"git/write accepts init, add, or commit",
				"add requires pathspecs and commit requires message",
			},
		}, true
	case StepKindMCPToolCall, StepKindMCPResourceRead, StepKindMCPPromptGet:
		return RuntimeOperationContract{
			Kind: CanonicalRuntimeBlockKind(kind),
			Parameters: []RuntimeOperationParameterContract{
				{Name: "serverId", Type: RuntimeOperationParameterTypeString, Required: true, Description: "configured MCP server identity"},
				{Name: "tool", Type: RuntimeOperationParameterTypeString, Description: "tool name for mcp/tool_call"},
				{Name: "uri", Type: RuntimeOperationParameterTypeString, Description: "resource URI for mcp/resource_read"},
				{Name: "resourceUri", Type: RuntimeOperationParameterTypeString, Description: "resource URI alias"},
				{Name: "prompt", Type: RuntimeOperationParameterTypeString, Description: "prompt name for mcp/prompt_get"},
				{Name: "name", Type: RuntimeOperationParameterTypeString, Description: "prompt name alias"},
				{Name: "summary", Type: RuntimeOperationParameterTypeString, Description: "bounded operation summary"},
				{Name: "stream", Type: RuntimeOperationParameterTypeBoolean, Description: "requests streaming external protocol handling"},
			},
			ForbiddenParameters: []string{"input", "arguments", "payload"},
			Rules: []string{
				"tool calls require tool and may use scalar input.<name> fields",
				"resource reads require uri or resourceUri",
				"prompt gets require prompt or name and may use scalar arguments.<name> fields",
			},
		}, true
	case StepKindSSHCommand:
		return RuntimeOperationContract{
			Kind: StepKindSSHCommand,
			Parameters: []RuntimeOperationParameterContract{
				{Name: "host", Type: RuntimeOperationParameterTypeString, Required: true, Description: "remote host"},
				{Name: "port", Type: RuntimeOperationParameterTypeInteger, Description: "remote SSH port"},
				{Name: "username", Type: RuntimeOperationParameterTypeString, Required: true, Description: "remote username"},
				{Name: "password", Type: RuntimeOperationParameterTypeString, Description: "password credential"},
				{Name: "privateKey", Type: RuntimeOperationParameterTypeString, Description: "inline private key credential"},
				{Name: "privateKeyPath", Type: RuntimeOperationParameterTypeString, Description: "workspace-relative or absolute private key path"},
				{Name: "command", Type: RuntimeOperationParameterTypeString, Required: true, Description: "bounded remote command"},
				{Name: "timeoutMillis", Type: RuntimeOperationParameterTypeInteger, Description: "bounded timeout in milliseconds"},
			},
			ForbiddenParameters: []string{"args", "input", "payload"},
			Rules:               []string{"port must be in 1..65535", "one password/privateKey/privateKeyPath credential is required", "timeoutMillis must be between 1000 and 300000"},
		}, true
	case StepKindWebSearch, StepKindWebFetch:
		return RuntimeOperationContract{
			Kind: CanonicalRuntimeBlockKind(kind),
			Parameters: []RuntimeOperationParameterContract{
				{Name: "providerId", Type: RuntimeOperationParameterTypeString, Description: "configured provider identity"},
				{Name: "query", Type: RuntimeOperationParameterTypeString, Description: "search query"},
				{Name: "locale", Type: RuntimeOperationParameterTypeString, Description: "search locale"},
				{Name: "recency", Type: RuntimeOperationParameterTypeString, Description: "search recency filter"},
				{Name: "domains", Type: RuntimeOperationParameterTypeStringList, Description: "included domains"},
				{Name: "excludeDomains", Type: RuntimeOperationParameterTypeStringList, Description: "excluded domains"},
				{Name: "maxResults", Type: RuntimeOperationParameterTypeInteger, Description: "bounded search result count"},
				{Name: "safeSearch", Type: RuntimeOperationParameterTypeBoolean, Description: "safe search policy"},
				{Name: "approvalId", Type: RuntimeOperationParameterTypeString, Description: "approval identity when policy requires it"},
				{Name: "url", Type: RuntimeOperationParameterTypeString, Description: "fetch URL"},
				{Name: "maxBytes", Type: RuntimeOperationParameterTypeInteger, Description: "bounded fetch bytes"},
				{Name: "extractMode", Type: RuntimeOperationParameterTypeString, Description: "provider extraction mode"},
			},
			ForbiddenParameters: []string{"input", "payload", "result"},
			Rules:               []string{"web/search requires query", "web/fetch requires url", "maxResults and maxBytes must be positive when provided"},
		}, true
	default:
		return RuntimeOperationContract{}, false
	}
}

func ValidateRuntimeOperationContract(operation RuntimeOperation) error {
	operation = operation.Normalized()
	contract, known := RuntimeOperationContractForKind(operation.Kind)
	if !known {
		return nil
	}
	allowed := make(map[string]RuntimeOperationParameterContract, len(contract.Parameters))
	for _, parameter := range contract.Parameters {
		allowed[parameter.Name] = parameter
	}
	values := make(map[string]OperationParameter, len(operation.Parameters))
	for _, parameter := range operation.Parameters {
		definition, ok := allowed[parameter.Name]
		if !ok && isMCPDynamicOperationParameter(operation.Kind, parameter.Name) {
			values[parameter.Name] = parameter
			continue
		}
		if !ok {
			return fmt.Errorf("%s", invalidRuntimeOperationParameterError(operation.Kind, parameter.Name, contract))
		}
		if !operationParameterMatchesType(parameter, definition.Type) {
			return fmt.Errorf("kind %q parameter %q requires %s", operation.Kind, parameter.Name, definition.Type)
		}
		values[parameter.Name] = parameter
	}
	for _, parameter := range contract.Parameters {
		if parameter.Required {
			if _, ok := values[parameter.Name]; !ok {
				return fmt.Errorf("kind %q requires parameter %q", operation.Kind, parameter.Name)
			}
		}
	}
	switch operation.Kind {
	case StepKindFileList:
		if limit, ok := parameterInteger(values["maxEntries"]); ok && (limit <= 0 || limit > 500) {
			return fmt.Errorf("kind %q maxEntries must be between 1 and 500", operation.Kind)
		}
	case StepKindFileRead:
		if parameterString(values["path"]) == "" {
			return fmt.Errorf("kind %q requires path", operation.Kind)
		}
		if limit, ok := parameterInteger(values["maxBytes"]); ok && (limit <= 0 || limit > 65536) {
			return fmt.Errorf("kind %q maxBytes must be between 1 and 65536", operation.Kind)
		}
		start, hasStart := parameterInteger(values["startLine"])
		end, hasEnd := parameterInteger(values["endLine"])
		if (hasStart && start <= 0) || (hasEnd && end <= 0) || (hasStart && hasEnd && end < start) {
			return fmt.Errorf("kind %q requires positive ordered line bounds", operation.Kind)
		}
	case StepKindFileWrite:
		path := parameterString(values["path"])
		targetRef := parameterString(values["targetRef"])
		if path != "" && targetRef != "" && path != targetRef {
			return fmt.Errorf("kind %q path and targetRef must identify the same target", operation.Kind)
		}
		target := targetRef
		if target == "" {
			target = path
		}
		if target == "" && len(operation.TargetRefs) == 1 {
			target = operation.TargetRefs[0]
		}
		if target == "" {
			return fmt.Errorf("kind %q requires one target via path, targetRef, or targetRefs", operation.Kind)
		}
		if len(operation.TargetRefs) > 0 && !containsString(operation.TargetRefs, target) {
			return fmt.Errorf("kind %q targetRefs must contain target %q", operation.Kind, target)
		}
	case StepKindFileEdit:
		path := parameterString(values["path"])
		oldText := parameterRawString(values["oldText"])
		newText := parameterRawString(values["newText"])
		if path == "" || oldText == "" || oldText == newText {
			return fmt.Errorf("kind %q requires non-empty path and oldText with a distinct newText", operation.Kind)
		}
		if occurrence, ok := parameterInteger(values["occurrence"]); ok && occurrence <= 0 {
			return fmt.Errorf("kind %q occurrence must be positive", operation.Kind)
		}
		if !containsString(operation.TargetRefs, path) {
			return fmt.Errorf("kind %q targetRefs must contain path %q", operation.Kind, path)
		}
	case RuntimeBlockKindPatchApply, RuntimeBlockKindPatchPreview:
		targetRef := parameterString(values["targetRef"])
		path := parameterString(values["path"])
		if targetRef == "" {
			targetRef = path
		} else if path != "" && path != targetRef {
			return fmt.Errorf("kind %q targetRef and path must identify the same target", operation.Kind)
		}
		patchType := parameterString(values["patchType"])
		if targetRef == "" || !containsString(operation.TargetRefs, targetRef) {
			return fmt.Errorf("kind %q targetRefs must contain targetRef %q", operation.Kind, targetRef)
		}
		switch patchType {
		case "file_write":
			if _, ok := values["content"]; !ok {
				return fmt.Errorf("kind %q file_write requires content", operation.Kind)
			}
		case "text_replace":
			oldText := parameterRawString(values["oldText"])
			newText, newTextPresent := values["newText"]
			if oldText == "" || !newTextPresent || oldText == parameterRawString(newText) {
				return fmt.Errorf("kind %q text_replace requires non-empty oldText and a distinct newText", operation.Kind)
			}
		case "unified_diff":
			if parameterRawString(values["diff"]) == "" {
				return fmt.Errorf("kind %q unified_diff requires diff", operation.Kind)
			}
		default:
			return fmt.Errorf("kind %q patchType %q is unsupported", operation.Kind, patchType)
		}
		if occurrence, ok := parameterInteger(values["occurrence"]); ok && occurrence <= 0 {
			return fmt.Errorf("kind %q occurrence must be positive", operation.Kind)
		}
	case StepKindShellCommand, RuntimeBlockKindManagedValidationSession:
		if parameterString(values["command"]) == "" {
			return fmt.Errorf("kind %q requires a non-empty command", operation.Kind)
		}
		if timeout, ok := parameterInteger(values["timeoutMillis"]); ok && (timeout < 1000 || timeout > 600000) {
			return fmt.Errorf("kind %q timeoutMillis must be between 1000 and 600000", operation.Kind)
		}
		stateChanging := false
		if parameter, ok := values["stateChanging"]; ok && parameter.Boolean != nil {
			stateChanging = *parameter.Boolean
		}
		if stateChanging && parameterString(values["targetRef"]) == "" && len(values["targetRefs"].StringList) == 0 && len(operation.TargetRefs) == 0 {
			return fmt.Errorf("kind %q state-changing command requires target refs", operation.Kind)
		}
	case StepKindGitRead, StepKindGitWrite:
		subOperation := strings.ToLower(parameterString(values["operation"]))
		if timeout, ok := parameterInteger(values["timeoutMillis"]); ok && (timeout < 1000 || timeout > 600000) {
			return fmt.Errorf("kind %q timeoutMillis must be between 1000 and 600000", operation.Kind)
		}
		if limit, ok := parameterInteger(values["limit"]); ok && limit <= 0 {
			return fmt.Errorf("kind %q limit must be positive", operation.Kind)
		}
		if operation.Kind == StepKindGitRead {
			switch subOperation {
			case "status", "diff", "log", "show", "branch":
			default:
				return fmt.Errorf("kind %q does not accept git operation %q", operation.Kind, subOperation)
			}
		} else {
			switch subOperation {
			case "init":
			case "add":
				if len(values["pathspecs"].StringList) == 0 {
					return fmt.Errorf("kind %q add requires pathspecs", operation.Kind)
				}
			case "commit":
				if parameterString(values["message"]) == "" {
					return fmt.Errorf("kind %q commit requires message", operation.Kind)
				}
				if len(values["pathspecs"].StringList) > 0 {
					return fmt.Errorf("kind %q commit does not accept pathspecs", operation.Kind)
				}
			default:
				return fmt.Errorf("kind %q does not accept git operation %q", operation.Kind, subOperation)
			}
		}
	case StepKindMCPToolCall, StepKindMCPResourceRead, StepKindMCPPromptGet:
		if parameterString(values["serverId"]) == "" {
			return fmt.Errorf("kind %q requires serverId", operation.Kind)
		}
		switch operation.Kind {
		case StepKindMCPToolCall:
			if parameterString(values["tool"]) == "" {
				return fmt.Errorf("kind %q requires tool", operation.Kind)
			}
		case StepKindMCPResourceRead:
			if parameterString(values["uri"]) == "" && parameterString(values["resourceUri"]) == "" {
				return fmt.Errorf("kind %q requires uri", operation.Kind)
			}
		case StepKindMCPPromptGet:
			if parameterString(values["prompt"]) == "" && parameterString(values["name"]) == "" {
				return fmt.Errorf("kind %q requires prompt", operation.Kind)
			}
		}
	case StepKindSSHCommand:
		port := int64(22)
		if value, ok := parameterInteger(values["port"]); ok {
			port = value
		}
		if port <= 0 || port > 65535 || parameterString(values["host"]) == "" || parameterString(values["username"]) == "" || parameterString(values["command"]) == "" {
			return fmt.Errorf("kind %q requires host, username, command and a valid port", operation.Kind)
		}
		if parameterString(values["password"]) == "" && parameterString(values["privateKey"]) == "" && parameterString(values["privateKeyPath"]) == "" {
			return fmt.Errorf("kind %q requires an SSH credential", operation.Kind)
		}
		if timeout, ok := parameterInteger(values["timeoutMillis"]); ok && (timeout < 1000 || timeout > 300000) {
			return fmt.Errorf("kind %q timeoutMillis must be between 1000 and 300000", operation.Kind)
		}
	case StepKindWebSearch:
		if parameterString(values["query"]) == "" {
			return fmt.Errorf("kind %q requires query", operation.Kind)
		}
		if limit, ok := parameterInteger(values["maxResults"]); ok && limit <= 0 {
			return fmt.Errorf("kind %q maxResults must be positive", operation.Kind)
		}
	case StepKindWebFetch:
		if parameterString(values["url"]) == "" {
			return fmt.Errorf("kind %q requires url", operation.Kind)
		}
		if limit, ok := parameterInteger(values["maxBytes"]); ok && limit <= 0 {
			return fmt.Errorf("kind %q maxBytes must be positive", operation.Kind)
		}
	}
	return nil
}

// RuntimeOperationRequiresCurrentTargetSnapshot reports whether an operation's
// technical contract depends on exact current target content. Repair planning
// uses it to reacquire stale targets after a conflict; it does not encode
// business or profile semantics.
func RuntimeOperationRequiresCurrentTargetSnapshot(operation RuntimeOperation) bool {
	operation = operation.Normalized()
	switch operation.Kind {
	case StepKindFileEdit:
		return true
	case RuntimeBlockKindPatchApply:
		patchType := operationStringParameter(operation.Parameters, "patchType")
		return patchType == "text_replace" || patchType == "unified_diff"
	default:
		return false
	}
}

func invalidRuntimeOperationParameterError(kind string, name string, contract RuntimeOperationContract) string {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	allowedNames := make([]string, 0, len(contract.Parameters))
	for _, parameter := range contract.Parameters {
		allowedNames = append(allowedNames, parameter.Name)
	}
	message := fmt.Sprintf("kind %q does not accept parameter %q", kind, name)
	switch kind {
	case StepKindMCPToolCall:
		suggestion := "input." + name
		if strings.HasPrefix(name, "input.") {
			suggestion = "a declared tool field or a different input.<name> key"
		}
		message += fmt.Sprintf("; for mcp/tool_call use fixed parameters %s or dynamic input.<name> fields (example: %q)", strings.Join(allowedNames, ", "), suggestion)
	case StepKindMCPPromptGet:
		suggestion := "arguments." + name
		if strings.HasPrefix(name, "arguments.") {
			suggestion = "a declared prompt field or a different arguments.<name> key"
		}
		message += fmt.Sprintf("; for mcp/prompt_get use fixed parameters %s or dynamic arguments.<name> fields (example: %q)", strings.Join(allowedNames, ", "), suggestion)
	case StepKindMCPResourceRead:
		message += fmt.Sprintf("; for mcp/resource_read use fixed parameters %s", strings.Join(allowedNames, ", "))
	default:
		if len(allowedNames) > 0 {
			message += fmt.Sprintf("; allowed parameters: %s", strings.Join(allowedNames, ", "))
		}
	}
	if len(contract.ForbiddenParameters) > 0 {
		message += fmt.Sprintf("; forbidden parameters: %s", strings.Join(contract.ForbiddenParameters, ", "))
	}
	return message
}

func isMCPDynamicOperationParameter(kind string, name string) bool {
	switch kind {
	case StepKindMCPToolCall:
		return strings.HasPrefix(name, "input.") && strings.TrimSpace(strings.TrimPrefix(name, "input.")) != ""
	case StepKindMCPPromptGet:
		return strings.HasPrefix(name, "arguments.") && strings.TrimSpace(strings.TrimPrefix(name, "arguments.")) != ""
	default:
		return false
	}
}

func operationParameterMatchesType(parameter OperationParameter, expected RuntimeOperationParameterType) bool {
	switch expected {
	case RuntimeOperationParameterTypeString:
		return parameter.String != nil && parameter.Integer == nil && parameter.Boolean == nil && len(parameter.StringList) == 0
	case RuntimeOperationParameterTypeInteger:
		return parameter.String == nil && parameter.Integer != nil && parameter.Boolean == nil && len(parameter.StringList) == 0
	case RuntimeOperationParameterTypeBoolean:
		return parameter.String == nil && parameter.Integer == nil && parameter.Boolean != nil && len(parameter.StringList) == 0
	case RuntimeOperationParameterTypeStringList:
		return parameter.String == nil && parameter.Integer == nil && parameter.Boolean == nil && len(parameter.StringList) > 0
	default:
		return false
	}
}

func parameterString(parameter OperationParameter) string {
	if parameter.String == nil {
		return ""
	}
	return strings.TrimSpace(*parameter.String)
}

func parameterRawString(parameter OperationParameter) string {
	if parameter.String == nil {
		return ""
	}
	return *parameter.String
}

func parameterInteger(parameter OperationParameter) (int64, bool) {
	if parameter.Integer == nil {
		return 0, false
	}
	return *parameter.Integer, true
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
