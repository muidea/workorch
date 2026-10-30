package workorchruntimecontract

import (
	"fmt"
	"sort"
)

// RequiredRuntimeEvidencePurposes is a detached declaration catalog. Empty
// AcceptanceItemRef means that a semantic selection is still required, never a
// default consumer or evidence of completed work.
func RequiredRuntimeEvidencePurposes(profiles []RuntimeProfileContract) ([]RuntimeEvidencePurposeMapping, error) {
	var result []RuntimeEvidencePurposeMapping
	for _, p := range profiles {
		for _, d := range p.DeliverableExpectations {
			if d.RequiredEvidencePurposeContractRef == "" {
				continue
			}
			if !d.Required || d.RequiredEvidencePurposeContractRef != RuntimeProfileDeliverableEvidencePurposeV2 {
				return nil, fmt.Errorf("required_evidence_purpose_invalid: unsupported required deliverable purpose")
			}
			m := RuntimeEvidencePurposeMapping{ContractRef: d.RequiredEvidencePurposeContractRef,
				HistoricalItemRef: RuntimeProfileDeliverablePurposeRef(p.ID, d.ID), HistoricalProfileID: p.ID,
				HistoricalDeliverableID: d.ID, HistoricalProfileHash: StableRuntimeProfileContractsHash([]RuntimeProfileContract{p}), HistoricalText: d.Description}
			if err := validateRuntimeProfileEvidencePurposeSource(m, profiles); err != nil {
				return nil, err
			}
			result = append(result, m)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].HistoricalItemRef < result[j].HistoricalItemRef })
	return result, nil
}

func validateRequiredRuntimeEvidencePurposes(mappings []RuntimeEvidencePurposeMapping, profiles []RuntimeProfileContract) error {
	required, err := RequiredRuntimeEvidencePurposes(profiles)
	if err != nil {
		return err
	}
	for _, r := range required {
		matches := 0
		for _, m := range mappings {
			if m.HistoricalItemRef != r.HistoricalItemRef {
				continue
			}
			matches++
			expected := r
			expected.AcceptanceItemRef = m.AcceptanceItemRef
			if m != expected || m.AcceptanceItemRef == "" {
				return fmt.Errorf("required_evidence_purpose_invalid: %s differs from original Profile", r.HistoricalItemRef)
			}
		}
		if matches != 1 {
			return fmt.Errorf("required_evidence_purpose_missing: %s requires exactly one original acceptance source (got %d)", r.HistoricalItemRef, matches)
		}
	}
	return nil
}
