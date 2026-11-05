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
	if ledger.Manifest != nil && !committedRuntimeSemanticManifestDefinition(session.committedPreimage, *ledger.Manifest, seed, focuses) {
		if err := validateRuntimeSemanticManifestDefinitionInSession(*ledger.Manifest, seed, focuses, session); err != nil {
			return fmt.Errorf("runtime aggregate semantic active manifest: %w", err)
		}
	}
	for i, generation := range ledger.AllHistory() {
		if session.committedGeneration(generation) && runtimeRawEqual(seed, session.committedPreimage.Facts.DefinitionSeed) && runtimeRawEqual(focuses, session.committedPreimage.Facts.DefinitionFocuses) {
			continue
		}
		if v := generation.OwnerInputRevision; v != nil {
			requirements, err := CompileRuntimeValidationProofRequirements(seed.DefinitionRevision, seed.CoverageItems, focuses)
			if err != nil {
				return err
			}
			if err := ValidateRuntimeValidationProofRequirementCatalogApplicability(requirements, []RuntimeOperationAdmissibility{v.OwnerObservation.Catalog}); err != nil {
				return err
			}
		}
		if generation.Snapshot.Manifest == nil {
			continue
		}
		if err := validateRuntimeSemanticManifestDefinitionInSession(*generation.Snapshot.Manifest, seed, focuses, session); err != nil {
			return fmt.Errorf("runtime aggregate semantic history manifest %d: %w", i, err)
		}
	}
	return nil
}

// The immutable Definition and this exact manifest have already been joined
// by the owner. Ledger phase/request changes do not create a new proof task.
func committedRuntimeSemanticManifestDefinition(previous *RuntimeStateRecord, manifest RuntimeSemanticPlanManifest, seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus) bool {
	if previous == nil || !runtimeRawEqual(seed, previous.Facts.DefinitionSeed) || !runtimeRawEqual(focuses, previous.Facts.DefinitionFocuses) {
		return false
	}
	if previous.SemanticPlanning.Manifest != nil && runtimeRawEqual(*previous.SemanticPlanning.Manifest, manifest) {
		return true
	}
	for _, generation := range previous.SemanticPlanning.AllHistory() {
		if generation.Snapshot.Manifest != nil && runtimeRawEqual(*generation.Snapshot.Manifest, manifest) {
			return true
		}
	}
	return false
}
