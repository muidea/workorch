// Package grounding contains deterministic comparisons for owner target and
// prerequisite facts.  It never owns a Runtime aggregate, chooses a
// transition, consumes a budget, or performs I/O.
package grounding

import (
	"sort"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CatalogFreshnessGap is a detached comparison result.  The Runtime owner
// maps it to its local audit/invalidation record; this package does not decide
// whether a gap should be refreshed or terminalized.
type CatalogFreshnessGap struct {
	PrerequisiteID        string
	ManifestSnapshotID    string
	CurrentSnapshotID     string
	ManifestObservationID string
	CurrentObservationID  string
}

// TargetRefsContained reports exact normalized target membership.  Empty
// required refs are vacuously contained; empty available refs cannot authorize
// a non-empty target set.
func TargetRefsContained(required, available []string) bool {
	set := make(map[string]struct{}, len(available))
	for _, value := range normalizeStrings(available) {
		set[value] = struct{}{}
	}
	for _, value := range normalizeStrings(required) {
		if _, found := set[value]; !found {
			return false
		}
	}
	return true
}

// CatalogCoversFocusTargets determines whether an owner catalog can cover the
// current focus target set.  It only reads owner-published target policy and
// selectors; it never infers an operation or replacement action.
func CatalogCoversFocusTargets(catalog runtimecontract.RuntimeOperationAdmissibility, focus runtimecontract.CoverageFocus, requiredTargets []string) bool {
	catalog = catalog.Normalized()
	focus = focus.Normalized()
	if TargetRefsContained(requiredTargets, catalog.TargetRefs) {
		return true
	}
	variantsByKind := make(map[string][]string)
	for _, variant := range focus.AcceptedOperationVariants {
		kind := runtimecontract.CanonicalRuntimeBlockKind(variant.Kind)
		if kind == "" {
			continue
		}
		variantsByKind[kind] = append(variantsByKind[kind], variant.VariantID)
	}
	policy := runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope
	if len(focus.ProspectiveTargetRefs) != 0 {
		policy = runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget
	}
	for _, rawKind := range focus.AcceptedStepKinds {
		kind := runtimecontract.CanonicalRuntimeBlockKind(rawKind)
		if kind == "" || !stringIncluded(catalog.OperationKinds, kind) {
			continue
		}
		variantIDs := append([]string(nil), variantsByKind[kind]...)
		if len(variantIDs) == 0 {
			for _, variant := range catalog.OperationVariants {
				if variant.Kind == kind {
					variantIDs = append(variantIDs, variant.VariantID)
				}
			}
			if len(variantIDs) == 0 {
				variantIDs = []string{""}
			}
		}
		for _, variantID := range variantIDs {
			allowed, selectors, _, _, err := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(catalog, kind, variantID, policy)
			if err == nil && allowed && len(selectors) != 0 {
				return true
			}
		}
	}
	return false
}

// CurrentOperationAdmissibility returns the valid current catalog for the
// same owner/prerequisite identity.  A malformed matching fact is rejected
// instead of being treated as fresh merely because its IDs match.
func CurrentOperationAdmissibility(facts runtimecontract.RuntimeFacts, wanted runtimecontract.RuntimeOperationAdmissibility) (runtimecontract.RuntimeOperationAdmissibility, bool) {
	wanted = wanted.Normalized()
	for _, candidate := range facts.Resources.OperationAdmissibilities {
		candidate = candidate.Normalized()
		if candidate.PrerequisiteID == wanted.PrerequisiteID && candidate.OwnerModuleID == wanted.OwnerModuleID {
			if err := candidate.Validate(); err != nil {
				return runtimecontract.RuntimeOperationAdmissibility{}, false
			}
			return candidate, true
		}
	}
	return runtimecontract.RuntimeOperationAdmissibility{}, false
}

// CurrentCatalogObservation returns the exact current observation for a
// catalog.  Matching only a prerequisite ID is insufficient after an owner
// resource change, so content hash and owner identity are required as well.
func CurrentCatalogObservation(observations []runtimecontract.RuntimePrerequisiteObservation, catalog runtimecontract.RuntimeOperationAdmissibility) (runtimecontract.RuntimePrerequisiteObservation, bool) {
	catalog = catalog.Normalized()
	for _, observation := range observations {
		observation = observation.Normalized()
		if observation.PrerequisiteID == catalog.PrerequisiteID &&
			observation.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility &&
			observation.OwnerModuleID == catalog.OwnerModuleID &&
			observation.ContentHash == catalog.SnapshotID {
			if err := observation.Validate(); err == nil {
				return observation, true
			}
		}
	}
	return runtimecontract.RuntimePrerequisiteObservation{}, false
}

// CompareCatalogFreshness compares all manifest catalogs against current
// owner facts and observations.  Output ordering is stable by prerequisite ID
// so callers can persist deterministic invalidation lineage.
func CompareCatalogFreshness(
	manifestCatalogs []runtimecontract.RuntimeOperationAdmissibility,
	currentFacts runtimecontract.RuntimeFacts,
) []CatalogFreshnessGap {
	currentObservations := currentFacts.Resources.PrerequisiteObservations
	gaps := make([]CatalogFreshnessGap, 0)
	for _, rawCatalog := range manifestCatalogs {
		catalog := rawCatalog.Normalized()
		currentCatalog, catalogFound := CurrentOperationAdmissibility(currentFacts, catalog)
		currentObservation, observationFound := CurrentCatalogObservation(currentObservations, catalog)
		gap := CatalogFreshnessGap{
			PrerequisiteID:        catalog.PrerequisiteID,
			ManifestSnapshotID:    catalog.SnapshotID,
			ManifestObservationID: "",
		}
		// Keep the manifest observation identity only when the matching current
		// record is valid. A malformed record proves a gap but is not lineage.
		for _, rawObservation := range currentObservations {
			observation := rawObservation.Normalized()
			if observation.PrerequisiteID == catalog.PrerequisiteID &&
				observation.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility &&
				observation.OwnerModuleID == catalog.OwnerModuleID {
				if err := observation.Validate(); err == nil {
					gap.ManifestObservationID = observation.ObservationID
				}
				break
			}
		}
		if catalogFound {
			gap.CurrentSnapshotID = currentCatalog.SnapshotID
		}
		if observationFound {
			gap.CurrentObservationID = currentObservation.ObservationID
		}
		if !catalogFound || currentCatalog.SnapshotID != catalog.SnapshotID ||
			runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(currentCatalog) != catalog.SnapshotID ||
			!observationFound || currentObservation.ContentHash != catalog.SnapshotID ||
			currentObservation.PrerequisiteID != catalog.PrerequisiteID || currentObservation.OwnerModuleID != catalog.OwnerModuleID {
			gaps = append(gaps, gap)
		}
	}
	sort.SliceStable(gaps, func(left, right int) bool { return gaps[left].PrerequisiteID < gaps[right].PrerequisiteID })
	return gaps
}

func normalizeStrings(values []string) []string {
	ret := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func stringIncluded(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	if wanted == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}
