package events

import (
	"errors"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestResolveRuntimeCapabilitiesUsesRequestedClass(t *testing.T) {
	tests := []struct {
		class runtimecontract.RuntimeCapabilityClass
		read  func(runtimecontract.RuntimeCapabilities) []string
	}{
		{class: runtimecontract.RuntimeCapabilityClassStateChange, read: func(value runtimecontract.RuntimeCapabilities) []string { return value.StateChangeKinds }},
		{class: runtimecontract.RuntimeCapabilityClassValidation, read: func(value runtimecontract.RuntimeCapabilities) []string { return value.ValidationKinds }},
		{class: runtimecontract.RuntimeCapabilityClassGovernance, read: func(value runtimecontract.RuntimeCapabilities) []string { return value.GovernanceKinds }},
	}
	for _, tt := range tests {
		result, err := ResolveRuntimeCapabilities(AcquireRuntimeCapabilitiesCommand{
			WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", PrerequisiteID: "capability-1",
			ModuleID: "module-1", CapabilityClass: tt.class, RequiredKinds: []string{"kind-1"},
		}, "module-1", "kind-1", "kind-2")
		if err != nil {
			t.Fatalf("class %s: %v", tt.class, err)
		}
		if values := tt.read(result.Capabilities); len(values) != 1 || values[0] != "kind-1" {
			t.Fatalf("class %s capabilities = %+v", tt.class, result.Capabilities)
		}
	}
}

func TestResolveRuntimeCapabilitiesRejectsUnsupportedKind(t *testing.T) {
	_, err := ResolveRuntimeCapabilities(AcquireRuntimeCapabilitiesCommand{
		WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", PrerequisiteID: "capability-1",
		ModuleID: "module-1", CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{"missing"},
	}, "module-1", "kind-1")
	if err == nil {
		t.Fatal("unsupported capability was accepted")
	}
	var unsupported *RuntimeCapabilityUnsupportedError
	if !errors.As(err, &unsupported) || unsupported.ModuleID != "module-1" || unsupported.CapabilityClass != runtimecontract.RuntimeCapabilityClassValidation ||
		len(unsupported.RequiredKinds) != 1 || unsupported.RequiredKinds[0] != "missing" || !strings.Contains(unsupported.Error(), "unsupported_kind") {
		t.Fatalf("unsupported capability error = %#v, want typed owner/class/kind failure", err)
	}
}
