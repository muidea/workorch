package workorchruntimecontract

import (
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestResolveRuntimeCapabilityAcquisitionUsesRequestedClass(t *testing.T) {
	tests := []struct {
		class RuntimeCapabilityClass
		read  func(RuntimeCapabilities) []string
	}{
		{class: RuntimeCapabilityClassStateChange, read: func(value RuntimeCapabilities) []string { return value.StateChangeKinds }},
		{class: RuntimeCapabilityClassValidation, read: func(value RuntimeCapabilities) []string { return value.ValidationKinds }},
		{class: RuntimeCapabilityClassGovernance, read: func(value RuntimeCapabilities) []string { return value.GovernanceKinds }},
	}
	for _, tt := range tests {
		result, err := ResolveRuntimeCapabilityAcquisition(RuntimeCapabilityAcquisitionRequest{
			WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", PrerequisiteID: "capability-1",
			OwnerModuleID: "module-1", CapabilityClass: tt.class, RequiredKinds: []string{"kind-1"},
		}, "module-1", "kind-1", "kind-2")
		if err != nil {
			t.Fatalf("class %s: %v", tt.class, err)
		}
		if values := tt.read(result.Capabilities); len(values) != 1 || values[0] != "kind-1" {
			t.Fatalf("class %s capabilities = %+v", tt.class, result.Capabilities)
		}
		request := RuntimeCapabilityAcquisitionRequest{
			WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", PrerequisiteID: "capability-1",
			OwnerModuleID: "module-1", CapabilityClass: tt.class, RequiredKinds: []string{"kind-1"},
		}
		if err := result.ValidateForRequest(request); err != nil {
			t.Fatalf("class %s scoped acquisition validation: %v", tt.class, err)
		}
	}
}

func TestRuntimeCapabilityAcquisitionRejectsScopeAndObservationDrift(t *testing.T) {
	request := RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", PrerequisiteID: "capability-1",
		OwnerModuleID: "module-1", CapabilityClass: RuntimeCapabilityClassValidation, RequiredKinds: []string{"kind-1"},
	}
	result, err := ResolveRuntimeCapabilityAcquisition(request, "module-1", "kind-1")
	if err != nil {
		t.Fatal(err)
	}
	wrongScope := result
	wrongScope.WorkspaceID = "ws-2"
	if err := wrongScope.ValidateForRequest(request); err == nil {
		t.Fatal("cross-workspace capability acquisition unexpectedly validated")
	}
	wrongObservation := result
	wrongObservation.Observation.PrerequisiteID = "capability-2"
	if err := wrongObservation.ValidateForRequest(request); err == nil {
		t.Fatal("mismatched capability observation unexpectedly validated")
	}
}

func TestResolveRuntimeCapabilityAcquisitionRejectsUnsupportedKind(t *testing.T) {
	_, err := ResolveRuntimeCapabilityAcquisition(RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1", PrerequisiteID: "capability-1",
		OwnerModuleID: "module-1", CapabilityClass: RuntimeCapabilityClassValidation, RequiredKinds: []string{"missing"},
	}, "module-1", "kind-1")
	if err == nil {
		t.Fatal("unsupported capability was accepted")
	}
	var unsupported *RuntimeCapabilityUnsupportedError
	if !errors.As(err, &unsupported) || unsupported.OwnerModuleID != "module-1" || unsupported.CapabilityClass != RuntimeCapabilityClassValidation ||
		len(unsupported.RequiredKinds) != 1 || unsupported.RequiredKinds[0] != "missing" || !strings.Contains(unsupported.Error(), "unsupported_kind") {
		t.Fatalf("unsupported capability error = %#v, want typed owner/class/kind failure", err)
	}
}

func TestRuntimeCapabilityCatalogUnavailableErrorIsTypedAndInvalidArgument(t *testing.T) {
	err := &RuntimeCapabilityCatalogUnavailableError{
		OwnerModuleID: "blocks/mcp", SubjectRef: "mcp_capability_catalog", Reason: "owner_unavailable",
	}
	var typed *RuntimeCapabilityCatalogUnavailableError
	if !errors.As(err, &typed) || typed.OwnerModuleID != "blocks/mcp" || typed.SubjectRef != "mcp_capability_catalog" || typed.Reason != "owner_unavailable" {
		t.Fatalf("typed catalog error = %#v", err)
	}
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("catalog error = %v, want invalid argument unwrap", err)
	}
}
