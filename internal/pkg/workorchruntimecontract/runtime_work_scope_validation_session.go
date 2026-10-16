package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"slices"
)

type runtimeWorkScopeValidationEntry struct {
	raw     RuntimeImplementationWorkScopeSources
	request RuntimeImplementationWorkScopeRequest
}

type runtimeWorkDefinitionValidationEntry struct {
	seed     RuntimeDefinitionSeed
	focuses  []RuntimeDefinitionFocus
	profiles []RuntimeProfileContract
}

func validateRuntimeWorkDefinitionSourcesInSession(seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus, profiles []RuntimeProfileContract, session *runtimeSemanticValidationSession) error {
	if session != nil {
		for _, entry := range session.workDefinitions {
			if reflect.DeepEqual(entry.seed, seed) && reflect.DeepEqual(entry.focuses, focuses) && reflect.DeepEqual(entry.profiles, profiles) {
				return nil
			}
		}
	}
	if err := seed.Validate(); err != nil {
		return err
	}
	if err := ValidateRuntimeDefinitionFocuses(seed, focuses); err != nil {
		return err
	}
	if err := ValidateRuntimeDefinitionImplementationWorkPolicies(focuses, profiles); err != nil {
		return err
	}
	if session != nil {
		session.workDefinitions = append(session.workDefinitions, runtimeWorkDefinitionValidationEntry{
			seed:     cloneRuntimeDefinitionSeedRaw(seed),
			focuses:  cloneRuntimeStateRawSlice(focuses, cloneRuntimeDefinitionFocusRaw),
			profiles: cloneRuntimeStateRawSlice(profiles, cloneRuntimeProfileContractRaw),
		})
	}
	return nil
}

// Complete source equality, not request ID or seed hash, is the lookup fence.
// Only a successful complete independent source compiler can populate this
// local list; its own exact raw Definition gates share the same local session.
func validateRuntimeWorkScopeInSession(r RuntimeImplementationWorkScopeRequest, sources RuntimeImplementationWorkScopeSources, session *runtimeSemanticValidationSession) error {
	if err := r.Validate(); err != nil {
		return err
	}
	for _, entry := range session.workScopes {
		if reflect.DeepEqual(entry.raw, sources) {
			if !reflect.DeepEqual(r, entry.request) {
				return fmt.Errorf("implementation work scope request does not match current sealed sources")
			}
			return nil
		}
	}
	expected, err := compileRuntimeWorkScopeRequestInSession(sources.WorkspaceID, sources.RunID, sources.AuthoritySnapshotID,
		sources.Seed, sources.Focuses, sources.Profiles, sources.CoverageWorkItemID, sources.ManifestBinding, session)
	if err != nil {
		return err
	}
	raw := sources
	raw.Seed = cloneRuntimeDefinitionSeedRaw(sources.Seed)
	raw.Focuses = cloneRuntimeStateRawSlice(sources.Focuses, cloneRuntimeDefinitionFocusRaw)
	raw.Profiles = cloneRuntimeStateRawSlice(sources.Profiles, cloneRuntimeProfileContractRaw)
	raw.ManifestBinding.Manifest.TargetRefs = slices.Clone(sources.ManifestBinding.Manifest.TargetRefs)
	raw.ManifestBinding.Observation.Dependencies = slices.Clone(sources.ManifestBinding.Observation.Dependencies)
	session.workScopes = append(session.workScopes, runtimeWorkScopeValidationEntry{raw: raw, request: expected.Clone()})
	if !reflect.DeepEqual(r, expected) {
		return fmt.Errorf("implementation work scope request does not match current sealed sources")
	}
	return nil
}
