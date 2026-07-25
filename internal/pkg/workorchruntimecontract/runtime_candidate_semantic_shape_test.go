package workorchruntimecontract

import (
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestStableRuntimeCandidateSemanticShapeIgnoresOperationIDsAndWireOrder(t *testing.T) {
	first := semanticShapeStateChangeCandidate("prepare-v1", "write-v1")
	second := semanticShapeStateChangeCandidate("bootstrap-op", "implementation-op")
	second.StateChange.Operations[0], second.StateChange.Operations[1] = second.StateChange.Operations[1], second.StateChange.Operations[0]
	for idx := range second.StateChange.Operations {
		parameters := second.StateChange.Operations[idx].Operation.Parameters
		slices.Reverse(parameters)
		second.StateChange.Operations[idx].Operation.Parameters = parameters
	}
	if StableRuntimeCandidateHash(first) == StableRuntimeCandidateHash(second) {
		t.Fatal("exact proposal identity must continue to distinguish operation IDs and wire order")
	}
	if StableRuntimeCandidateSemanticShape(first) != StableRuntimeCandidateSemanticShape(second) {
		t.Fatalf("semantic shape changed for ID/wire-order-only correction: first=%s second=%s", StableRuntimeCandidateSemanticShape(first), StableRuntimeCandidateSemanticShape(second))
	}
}

func TestStableRuntimeCandidateSemanticShapeIncludesCoverageTargetVariantAndDependency(t *testing.T) {
	base := semanticShapeStateChangeCandidate("prepare-v1", "write-v1")
	changedCoverage := semanticShapeStateChangeCandidate("prepare-v2", "write-v2")
	changedCoverage.StateChange.Operations[1].Operation.CoverageWorkItemIDs = []common.CoverageWorkItemID{"coverage-other"}
	changedTarget := semanticShapeStateChangeCandidate("prepare-v3", "write-v3")
	changedTarget.StateChange.Operations[1].Operation.TargetRefs = []string{"other.go"}
	changedTarget.StateChange.Operations[1].Operation.Parameters[0].String = semanticShapeStringPointer("other.go")
	changedVariant := semanticShapeStateChangeCandidate("prepare-v4", "write-v4")
	changedVariant.StateChange.Operations[1].Operation.RequiredOperationRef = "different-variant"
	changedDependency := semanticShapeStateChangeCandidate("prepare-v5", "write-v5")
	changedDependency.StateChange.Operations[1].Operation.DependsOn = nil

	baseShape := StableRuntimeCandidateSemanticShape(base)
	for name, candidate := range map[string]RuntimeCandidate{
		"coverage":   changedCoverage,
		"target":     changedTarget,
		"variant":    changedVariant,
		"dependency": changedDependency,
	} {
		if baseShape == StableRuntimeCandidateSemanticShape(candidate) {
			t.Fatalf("%s change did not alter candidate semantic shape", name)
		}
	}
}

func FuzzStableRuntimeCandidateSemanticShapeIgnoresIDAndWireOrder(f *testing.F) {
	f.Add("prepare-v1", "write-v1", "prepare-v2", "write-v2")
	f.Fuzz(func(t *testing.T, firstPrepare, firstWrite, secondPrepare, secondWrite string) {
		first := semanticShapeStateChangeCandidate("prepare-"+firstPrepare, "write-"+firstWrite)
		second := semanticShapeStateChangeCandidate("prepare-"+secondPrepare, "write-"+secondWrite)
		second.StateChange.Operations[0], second.StateChange.Operations[1] = second.StateChange.Operations[1], second.StateChange.Operations[0]
		for idx := range second.StateChange.Operations {
			parameters := second.StateChange.Operations[idx].Operation.Parameters
			slices.Reverse(parameters)
			second.StateChange.Operations[idx].Operation.Parameters = parameters
		}
		if StableRuntimeCandidateSemanticShape(first) != StableRuntimeCandidateSemanticShape(second) {
			t.Fatalf("semantic shape drifted for renamed/reordered graph: first=%s second=%s", StableRuntimeCandidateSemanticShape(first), StableRuntimeCandidateSemanticShape(second))
		}
	})
}

func semanticShapeStateChangeCandidate(prepareID, writeID string) RuntimeCandidate {
	preparePath, writePath := "bootstrap.go", "main.go"
	prepareContent, writeContent := "package bootstrap", "package main"
	patchType := "create"
	return RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: []StateChangingOperation{
		{Operation: RuntimeOperation{
			ID: prepareID, Kind: RuntimeBlockKindPatchApply, RequiredOperationRef: "bootstrap-variant",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"coverage-bootstrap"}, TargetRefs: []string{preparePath},
			Parameters: []OperationParameter{{Name: "path", String: &preparePath}, {Name: "patchType", String: &patchType}, {Name: "content", String: &prepareContent}},
		}},
		{Operation: RuntimeOperation{
			ID: writeID, Kind: RuntimeBlockKindPatchApply, RequiredOperationRef: "implementation-variant", DependsOn: []string{prepareID},
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"coverage-implementation"}, TargetRefs: []string{writePath},
			Parameters: []OperationParameter{{Name: "path", String: &writePath}, {Name: "patchType", String: &patchType}, {Name: "content", String: &writeContent}},
		}},
	}}}
}

func semanticShapeStringPointer(value string) *string {
	return &value
}
