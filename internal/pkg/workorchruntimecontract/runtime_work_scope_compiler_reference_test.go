package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	"reflect"
)

// Frozen complete independent scope compiler and request source gate.
func referenceWorkScopeCompiler(workspaceID common.WorkspaceID, runID common.RunID, authoritySnapshotID string,
	seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus, profiles []RuntimeProfileContract, coverageID common.CoverageWorkItemID, manifest RuntimeWorkspaceManifestBinding,
) (RuntimeImplementationWorkScopeRequest, error) {
	if err := manifest.Validate(); err != nil {
		return RuntimeImplementationWorkScopeRequest{}, err
	}
	if err := seed.Validate(); err != nil {
		return RuntimeImplementationWorkScopeRequest{}, err
	}
	if err := ValidateRuntimeDefinitionFocuses(seed, focuses); err != nil {
		return RuntimeImplementationWorkScopeRequest{}, err
	}
	if err := ValidateRuntimeDefinitionImplementationWorkPolicies(focuses, profiles); err != nil {
		return RuntimeImplementationWorkScopeRequest{}, err
	}
	var selected *RuntimeImplementationWorkPolicy
	for _, focus := range focuses {
		if len(focus.CoverageWorkItemIDs) != 1 || focus.CoverageWorkItemIDs[0] != coverageID {
			continue
		}
		if selected != nil {
			return RuntimeImplementationWorkScopeRequest{}, fmt.Errorf("implementation work scope requires one exact CWI declaration")
		}
		selected = focus.ImplementationWorkPolicy
	}
	if selected == nil {
		return RuntimeImplementationWorkScopeRequest{}, fmt.Errorf("implementation work scope has no policy for CWI %q", coverageID)
	}
	request := RuntimeImplementationWorkScopeRequest{
		WorkspaceID: workspaceID, RunID: runID, AuthoritySnapshotID: authoritySnapshotID,
		DefinitionSeedHash: seed.Hash, CoverageWorkItemID: coverageID, Policy: selected.Normalized(), ManifestBinding: manifest.Clone(),
	}
	// Observation time is audit metadata, not a new scope/revision identity.
	request.ManifestBinding.Observation.ObservedAt = ""
	request.RequestID = implementationWorkScopeRequestID(request)
	if err := request.Validate(); err != nil {
		return RuntimeImplementationWorkScopeRequest{}, err
	}
	return request, nil
}

func referenceWorkScopeAgainstSources(r RuntimeImplementationWorkScopeRequest, s RuntimeImplementationWorkScopeSources) error {
	if err := r.Validate(); err != nil {
		return err
	}
	expected, err := referenceWorkScopeCompiler(s.WorkspaceID, s.RunID, s.AuthoritySnapshotID, s.Seed, s.Focuses, s.Profiles, s.CoverageWorkItemID, s.ManifestBinding)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(r, expected) {
		return fmt.Errorf("implementation work scope request does not match current sealed sources")
	}
	return nil
}
