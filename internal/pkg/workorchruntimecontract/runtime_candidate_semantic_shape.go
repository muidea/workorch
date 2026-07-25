package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// StableRuntimeCandidateSemanticShape returns an ID- and wire-order-independent
// semantic fingerprint for a complete candidate. It is intentionally narrower
// than StableRuntimeCandidateHash: the latter remains the exact persisted
// proposal/dispatch identity, whereas this helper is used only to decide
// whether a correction repeated the same rejected candidate shape.
//
// It is a pure contract helper. It neither admits a candidate nor reads runtime
// state. Operation IDs are deliberately excluded; dependency edges are instead
// represented by the semantic base shapes of their referenced operations.
func StableRuntimeCandidateSemanticShape(candidate RuntimeCandidate) string {
	h := sha256.New()
	writeHashString(h, "runtime-candidate-semantic-shape-v1")
	switch {
	case candidate.StateChange != nil:
		writeHashString(h, "state_change")
		writeRuntimeCandidateSemanticOperations(h, stateChangingRuntimeOperations(candidate.StateChange.Operations))
	case candidate.Validation != nil:
		writeHashString(h, "validation")
		writeRuntimeCandidateSemanticOperations(h, validationRuntimeOperations(candidate.Validation.Operations))
	case candidate.Assessment != nil:
		writeHashString(h, "assessment")
		value := candidate.Assessment.Normalized()
		writeHashString(h, value.FocusID)
		writeHashString(h, value.SliceID)
		coverage := make([]string, 0, len(value.CoverageWorkItemIDs))
		for _, id := range value.CoverageWorkItemIDs {
			coverage = append(coverage, string(id))
		}
		writeHashStrings(h, coverage)
		writeHashString(h, string(value.Disposition))
		writeHashStrings(h, value.EvidenceRefs)
		writeHashStrings(h, value.ArtifactRefs)
	case candidate.Governance != nil:
		writeHashString(h, "governance")
		dependency := candidate.Governance.ExternalDependency
		writeHashString(h, strings.TrimSpace(dependency.Kind))
		writeHashString(h, strings.TrimSpace(dependency.SubjectRef))
	default:
		writeHashString(h, "empty")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeRuntimeCandidateSemanticOperations(h hashStringWriter, operations []RuntimeOperation) {
	normalized := make([]RuntimeOperation, 0, len(operations))
	byID := make(map[string]RuntimeOperation, len(operations))
	for _, value := range operations {
		operation := value.Normalized()
		normalized = append(normalized, operation)
		if operation.ID != "" {
			byID[operation.ID] = operation
		}
	}
	baseByID := make(map[string]string, len(byID))
	for id, operation := range byID {
		baseByID[id] = stableRuntimeOperationSemanticBaseShape(operation)
	}
	nodes := make([]string, 0, len(normalized))
	for _, operation := range normalized {
		base := stableRuntimeOperationSemanticBaseShape(operation)
		dependencies := make([]string, 0, len(operation.DependsOn))
		for _, dependencyID := range operation.DependsOn {
			if dependency, found := baseByID[dependencyID]; found {
				dependencies = append(dependencies, dependency)
				continue
			}
			// Validation separately rejects an absent dependency. Keeping only a
			// typed missing-edge marker here avoids letting arbitrary operation IDs
			// create a new semantic shape before that validation occurs.
			dependencies = append(dependencies, "missing_dependency")
		}
		sort.Strings(dependencies)
		node := sha256.New()
		writeHashString(node, "runtime-candidate-semantic-node-v1")
		writeHashString(node, base)
		writeHashStrings(node, dependencies)
		nodes = append(nodes, hex.EncodeToString(node.Sum(nil)))
	}
	sort.Strings(nodes)
	writeHashString(h, fmt.Sprintf("%d", len(nodes)))
	writeHashStrings(h, nodes)
}

func stableRuntimeOperationSemanticBaseShape(operation RuntimeOperation) string {
	operation = operation.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-operation-semantic-base-v1")
	writeHashString(h, CanonicalRuntimeBlockKind(operation.Kind))
	writeHashString(h, operation.RequiredOperationRef)
	coverage := make([]string, 0, len(operation.CoverageWorkItemIDs))
	for _, id := range operation.CoverageWorkItemIDs {
		coverage = append(coverage, string(id))
	}
	writeHashStrings(h, coverage)
	writeHashStrings(h, operationShapeTargets(operation))
	if descriptor, known := RuntimeOperationDescriptorForKind(operation.Kind); known {
		writeHashString(h, string(descriptor.DefaultEffectScope))
	} else {
		writeHashString(h, string(RuntimeEffectUnknown))
	}
	for _, name := range operationShapeSemanticParameterNames(operation.Kind) {
		parameter, ok := operationParameterByName(operation.Parameters, name)
		if !ok {
			continue
		}
		writeRuntimeOperationSemanticParameter(h, name, parameter)
	}
	for _, parameter := range operation.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if !strings.HasPrefix(name, "input.") && !strings.HasPrefix(name, "arguments.") {
			continue
		}
		writeRuntimeOperationSemanticParameter(h, name, parameter)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeRuntimeOperationSemanticParameter(h hashStringWriter, name string, parameter OperationParameter) {
	writeHashString(h, strings.TrimSpace(name))
	switch {
	case parameter.String != nil:
		writeHashString(h, "string")
		writeHashString(h, *parameter.String)
	case parameter.Integer != nil:
		writeHashString(h, "integer")
		writeHashString(h, fmt.Sprintf("%d", *parameter.Integer))
	case parameter.Boolean != nil:
		writeHashString(h, "boolean")
		writeHashString(h, fmt.Sprintf("%t", *parameter.Boolean))
	case len(parameter.StringList) > 0:
		writeHashString(h, "string_list")
		writeHashStrings(h, parameter.StringList)
	default:
		writeHashString(h, "empty")
	}
}
