package workorchruntimecontract

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

type runtimeSemanticDefinitionSourceEntry struct {
	seed       RuntimeDefinitionSeed
	focuses    []RuntimeDefinitionFocus
	byCoverage map[common.CoverageWorkItemID]RuntimeDefinitionFocus
}

// This private read-only index is populated only after both full original
// gates succeed. It cannot be reused for a different raw seed/focus body.
func validateRuntimeSemanticDefinitionSourcesInSession(seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus, session *runtimeSemanticValidationSession) (map[common.CoverageWorkItemID]RuntimeDefinitionFocus, error) {
	for _, entry := range session.definitionSources {
		if runtimeRawEqual(entry.seed, seed) && runtimeRawEqual(entry.focuses, focuses) {
			return entry.byCoverage, nil
		}
	}
	if err := ValidateRuntimeDefinitionFocuses(seed, focuses); err != nil {
		return nil, err
	}
	index, err := runtimeSemanticBindingSourceFocuses(focuses)
	if err != nil {
		return nil, err
	}
	session.definitionSources = append(session.definitionSources, runtimeSemanticDefinitionSourceEntry{
		seed:       cloneRuntimeDefinitionSeedRaw(seed),
		focuses:    cloneRuntimeStateRawSlice(focuses, cloneRuntimeDefinitionFocusRaw),
		byCoverage: index,
	})
	return index, nil
}

// ValidateRuntimeSemanticLedgerDefinitionSources independently roots every
// current/history manifest in the supplied immutable Definition. No session,
// validation flag or successful source product crosses this pure call boundary.
func ValidateRuntimeSemanticLedgerDefinitionSources(ledger RuntimeSemanticPlanningLedger, seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus) error {
	return validateRuntimeSemanticLedgerDefinitionSourcesInSession(ledger, seed, focuses, newRuntimeSemanticValidationSession())
}

func validateRuntimeSemanticLedgerDefinitionSourcesInSession(ledger RuntimeSemanticPlanningLedger, seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus, session *runtimeSemanticValidationSession) error {
	if ledger.Manifest != nil {
		if err := validateRuntimeSemanticManifestDefinitionInSession(*ledger.Manifest, seed, focuses, session); err != nil {
			return fmt.Errorf("runtime aggregate semantic active manifest: %w", err)
		}
	}
	for i, generation := range ledger.AllHistory() {
		if generation.Snapshot.Manifest == nil {
			continue
		}
		if err := validateRuntimeSemanticManifestDefinitionInSession(*generation.Snapshot.Manifest, seed, focuses, session); err != nil {
			return fmt.Errorf("runtime aggregate semantic history manifest %d: %w", i, err)
		}
	}
	return nil
}
