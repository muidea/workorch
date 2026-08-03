package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCapabilityAcquisitionRequest is the owner-neutral value consumed by
// capability-owner EventHub contracts. It is not itself an EventHub command.
type RuntimeCapabilityAcquisitionRequest struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	PrerequisiteID      string
	OwnerModuleID       string
	CapabilityClass     RuntimeCapabilityClass
	RequiredKinds       []string
}

// RuntimeCapabilityAcquisition is a pure owner capability observation. Each
// capability owner exposes it through its own concrete EventHub result.
type RuntimeCapabilityAcquisition struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	Capabilities        RuntimeCapabilities
	Observation         RuntimePrerequisiteObservation
}

// RuntimeCapabilityUnsupportedError reports that the sealed owner/class/kind
// tuple cannot be served. It is a factual prerequisite error, not a Runtime
// action, retry decision, or governance request.
type RuntimeCapabilityUnsupportedError struct {
	OwnerModuleID   string
	CapabilityClass RuntimeCapabilityClass
	RequiredKinds   []string
	Reason          string
}

func (e *RuntimeCapabilityUnsupportedError) Error() string {
	if e == nil {
		return "runtime capability unavailable"
	}
	ownerModuleID := strings.TrimSpace(e.OwnerModuleID)
	reason := strings.TrimSpace(e.Reason)
	kinds := append([]string(nil), e.RequiredKinds...)
	for idx := range kinds {
		kinds[idx] = strings.TrimSpace(kinds[idx])
	}
	sort.Strings(kinds)
	if reason == "" {
		reason = "unsupported"
	}
	return fmt.Sprintf("runtime capability %s for owner %s is %s: %s", e.CapabilityClass, ownerModuleID, reason, strings.Join(kinds, ","))
}

// ResolveRuntimeCapabilityAcquisition validates a concrete owner request
// against that owner's immutable capability catalog. It performs no I/O and
// makes no Runtime transition or disposition decision.
func ResolveRuntimeCapabilityAcquisition(request RuntimeCapabilityAcquisitionRequest, ownerModuleID string, supportedKinds ...string) (RuntimeCapabilityAcquisition, error) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	if request.WorkspaceID == "" || request.RunID == "" || request.AuthoritySnapshotID == "" || request.PrerequisiteID == "" || strings.TrimSpace(request.OwnerModuleID) != ownerModuleID || len(request.RequiredKinds) == 0 {
		return RuntimeCapabilityAcquisition{}, fmt.Errorf("invalid runtime capability prerequisite request")
	}
	supported := make(map[string]struct{}, len(supportedKinds))
	for _, kind := range supportedKinds {
		kind = strings.TrimSpace(kind)
		if kind != "" {
			supported[kind] = struct{}{}
		}
	}
	resolved := make([]string, 0, len(request.RequiredKinds))
	seen := map[string]struct{}{}
	for _, kind := range request.RequiredKinds {
		kind = strings.TrimSpace(kind)
		if _, ok := supported[kind]; !ok {
			return RuntimeCapabilityAcquisition{}, &RuntimeCapabilityUnsupportedError{
				OwnerModuleID: ownerModuleID, CapabilityClass: request.CapabilityClass, RequiredKinds: []string{kind}, Reason: "unsupported_kind",
			}
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		resolved = append(resolved, kind)
	}
	capabilities := RuntimeCapabilities{}
	switch request.CapabilityClass {
	case RuntimeCapabilityClassStateChange:
		capabilities.StateChangeKinds = resolved
	case RuntimeCapabilityClassValidation:
		capabilities.ValidationKinds = resolved
	case RuntimeCapabilityClassGovernance:
		capabilities.GovernanceKinds = resolved
	default:
		return RuntimeCapabilityAcquisition{}, fmt.Errorf("invalid runtime capability class %q", request.CapabilityClass)
	}
	observation := runtimeCapabilityAcquisitionObservation(request, ownerModuleID, resolved)
	return RuntimeCapabilityAcquisition{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.AuthoritySnapshotID,
		Capabilities: capabilities, Observation: observation,
	}, nil
}

// ValidateForRequest verifies the complete scoped capability fact. The
// prerequisite identity is canonical in Observation and is not repeated on
// the acquisition envelope.
func (a RuntimeCapabilityAcquisition) ValidateForRequest(request RuntimeCapabilityAcquisitionRequest) error {
	if a.WorkspaceID != request.WorkspaceID || a.RunID != request.RunID ||
		strings.TrimSpace(a.AuthoritySnapshotID) != strings.TrimSpace(request.AuthoritySnapshotID) {
		return fmt.Errorf("runtime capability acquisition scope does not match request")
	}
	observation := a.Observation.Normalized()
	if observation.PrerequisiteID != strings.TrimSpace(request.PrerequisiteID) ||
		observation.Kind != PrerequisiteKindCapability ||
		observation.OwnerModuleID != strings.TrimSpace(request.OwnerModuleID) {
		return fmt.Errorf("runtime capability acquisition observation does not match request")
	}
	if err := observation.Validate(); err != nil {
		return err
	}
	want := normalizeStrings(request.RequiredKinds)
	stateChange := normalizeStrings(a.Capabilities.StateChangeKinds)
	validation := normalizeStrings(a.Capabilities.ValidationKinds)
	governance := normalizeStrings(a.Capabilities.GovernanceKinds)
	var got []string
	switch request.CapabilityClass {
	case RuntimeCapabilityClassStateChange:
		got = stateChange
		if len(validation) != 0 || len(governance) != 0 {
			return fmt.Errorf("runtime capability acquisition returned capabilities outside requested class")
		}
	case RuntimeCapabilityClassValidation:
		got = validation
		if len(stateChange) != 0 || len(governance) != 0 {
			return fmt.Errorf("runtime capability acquisition returned capabilities outside requested class")
		}
	case RuntimeCapabilityClassGovernance:
		got = governance
		if len(stateChange) != 0 || len(validation) != 0 {
			return fmt.Errorf("runtime capability acquisition returned capabilities outside requested class")
		}
	default:
		return fmt.Errorf("invalid runtime capability class %q", request.CapabilityClass)
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		return fmt.Errorf("runtime capability acquisition does not match required kinds")
	}
	return nil
}

func runtimeCapabilityAcquisitionObservation(request RuntimeCapabilityAcquisitionRequest, ownerModuleID string, kinds []string) RuntimePrerequisiteObservation {
	kinds = append([]string(nil), kinds...)
	sort.Strings(kinds)
	material := strings.Join([]string{ownerModuleID, string(request.CapabilityClass), strings.Join(kinds, ",")}, "|")
	digest := sha256.Sum256([]byte(material))
	revision := "capability-catalog-" + hex.EncodeToString(digest[:])[:20]
	observation := RuntimePrerequisiteObservation{
		PrerequisiteID: request.PrerequisiteID,
		Kind:           PrerequisiteKindCapability,
		OwnerModuleID:  ownerModuleID,
		ContentHash:    revision,
		Dependencies: []RuntimeFreshnessDependency{{
			Domain: RuntimeResourceOwnerCatalog, OwnerModuleID: ownerModuleID,
			SubjectRef: "runtime_capabilities", Revision: revision,
		}},
	}
	observation.ObservationID = StableRuntimePrerequisiteObservationID(observation)
	return observation
}
