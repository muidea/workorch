package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeRejectedOperationShape is the exact, typed operation structure that
// remains rejected on the current focus. It is volatile candidate context, not
// stable-prefix configuration or a new authority source.
type RuntimeRejectedOperationShape struct {
	Hash             string
	Operation        RuntimeOperation
	EvidenceID       common.EvidenceRecordID
	FailureClass     FailureClass
	FailureCode      string
	FailureSignature string
	FailureMessage   string
}

func NormalizeRuntimeRejectedOperationShapes(values []RuntimeRejectedOperationShape) []RuntimeRejectedOperationShape {
	byHash := map[string]RuntimeRejectedOperationShape{}
	for _, value := range values {
		value.Operation = value.Operation.Normalized()
		value.Hash = StableRuntimeOperationShape(value.Operation)
		value.EvidenceID = common.EvidenceRecordID(strings.TrimSpace(string(value.EvidenceID)))
		value.FailureClass = FailureClass(strings.TrimSpace(string(value.FailureClass)))
		value.FailureCode = strings.TrimSpace(value.FailureCode)
		value.FailureSignature = strings.TrimSpace(value.FailureSignature)
		value.FailureMessage = strings.TrimSpace(value.FailureMessage)
		if value.Hash == "" || value.Operation.Kind == "" {
			continue
		}
		byHash[value.Hash] = value
	}
	hashes := make([]string, 0, len(byHash))
	for hash := range byHash {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	ret := make([]RuntimeRejectedOperationShape, 0, len(hashes))
	for _, hash := range hashes {
		ret = append(ret, byHash[hash])
	}
	return nilIfEmpty(ret)
}

// StableRuntimeOperationShape derives the domain-neutral identity of an operation.
// It is a pure contract helper: callers decide whether a matching shape is active
// or admissible in their own owner state machine.
func StableRuntimeOperationShape(operation RuntimeOperation) string {
	operation = operation.Normalized()
	h := sha256.New()
	writeHashString(h, operation.Kind)
	writeHashStrings(h, operation.DependsOn)
	writeHashStrings(h, operationShapeTargets(operation))
	// Hash only parameters that affect execution semantics. Display-only metadata
	// such as summary/boundedReason must not create a new shape.
	for _, name := range operationShapeSemanticParameterNames(operation.Kind) {
		parameter, ok := operationParameterByName(operation.Parameters, name)
		if !ok {
			continue
		}
		writeHashString(h, name)
		switch {
		case parameter.String != nil:
			writeHashString(h, *parameter.String)
		case parameter.Integer != nil:
			writeHashString(h, fmt.Sprintf("%d", *parameter.Integer))
		case parameter.Boolean != nil:
			writeHashString(h, fmt.Sprintf("%t", *parameter.Boolean))
		case len(parameter.StringList) > 0:
			writeHashStrings(h, parameter.StringList)
		}
	}
	// Dynamic MCP/prompt fields (input.*/arguments.*) are execution-semantic.
	for _, parameter := range operation.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if !strings.HasPrefix(name, "input.") && !strings.HasPrefix(name, "arguments.") {
			continue
		}
		writeHashString(h, name)
		switch {
		case parameter.String != nil:
			writeHashString(h, *parameter.String)
		case parameter.Integer != nil:
			writeHashString(h, fmt.Sprintf("%d", *parameter.Integer))
		case parameter.Boolean != nil:
			writeHashString(h, fmt.Sprintf("%t", *parameter.Boolean))
		case len(parameter.StringList) > 0:
			writeHashStrings(h, parameter.StringList)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// operationShapeNonSemanticParameterNames are contract parameters that do not change
// execution shape identity: display metadata, runtime-injected scratch, or target aliases
// already covered by operationShapeTargets.
func operationShapeNonSemanticParameterNames() map[string]struct{} {
	return map[string]struct{}{
		"summary":                      {},
		"boundedReason":                {},
		"validationScratchDir":         {},
		"validationScratchAutoCleanup": {},
		"targetRef":                    {},
		"targetRefs":                   {},
		"relatedFailureTargets":        {},
		"coverageTargets":              {},
		"verificationScope":            {},
		// path is folded into operationShapeTargets with targetRef.
		"path": {},
	}
}

func operationShapeSemanticParameterNames(kind string) []string {
	contract, known := RuntimeOperationContractForKind(kind)
	if !known {
		// Conservative fallback for unknown kinds: structural keys only.
		return []string{"command", "args", "operation", "tool", "serverId", "patchType", "oldText", "newText", "baseHash", "content", "diff", "uri", "query", "url", "stateChanging", "recursive"}
	}
	exclude := operationShapeNonSemanticParameterNames()
	ret := make([]string, 0, len(contract.Parameters))
	for _, parameter := range contract.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			continue
		}
		if _, skip := exclude[name]; skip {
			continue
		}
		ret = append(ret, name)
	}
	sort.Strings(ret)
	return ret
}

func operationParameterByName(parameters []OperationParameter, name string) (OperationParameter, bool) {
	name = strings.TrimSpace(name)
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Name) == name {
			return parameter, true
		}
	}
	return OperationParameter{}, false
}

func operationShapeTargets(operation RuntimeOperation) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(operation.TargetRefs)+2)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	for _, value := range operation.TargetRefs {
		add(value)
	}
	// Treat path and targetRef as the same structural target identity.
	add(operationStringParameter(operation.Parameters, "targetRef"))
	add(operationStringParameter(operation.Parameters, "path"))
	sort.Strings(ret)
	return ret
}
