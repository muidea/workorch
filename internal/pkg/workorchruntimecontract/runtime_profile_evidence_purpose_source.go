package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// The reserved ref is not an original DoD ref. It names exactly one required
// deliverable in the confirmed Profile snapshot, never a runtime-selected CWI.
func RuntimeProfileDeliverablePurposeRef(profileID, deliverableID string) string {
	return "profile/" + profileID + "/deliverable/" + deliverableID
}

func runtimeEvidencePurposeHasProfileFields(m RuntimeEvidencePurposeMapping) bool {
	return m.HistoricalProfileID != "" || m.HistoricalDeliverableID != "" || m.HistoricalProfileHash != "" || m.HistoricalText != ""
}

func ValidateRuntimeProfileEvidencePurposeMappingDeclaration(m RuntimeEvidencePurposeMapping) error {
	if m.ContractRef != RuntimeProfileDeliverableEvidencePurposeV2 ||
		!runtimeSourceIdentity(m.HistoricalProfileID, 128) || !runtimeSourceIdentity(m.HistoricalDeliverableID, 128) ||
		!runtimeSourceIdentity(m.HistoricalProfileHash, 256) ||
		m.HistoricalItemRef != RuntimeProfileDeliverablePurposeRef(m.HistoricalProfileID, m.HistoricalDeliverableID) ||
		strings.TrimSpace(m.HistoricalText) == "" || len(m.HistoricalText) > 4096 || !utf8.ValidString(m.HistoricalText) || strings.ContainsRune(m.HistoricalText, 0) {
		return fmt.Errorf("Profile evidence purpose requires exact bounded snapshot identity and complete original deliverable text")
	}
	return nil
}

func validateRuntimeProfileEvidencePurposeSource(m RuntimeEvidencePurposeMapping, profiles []RuntimeProfileContract) error {
	if err := ValidateRuntimeProfileEvidencePurposeMappingDeclaration(m); err != nil {
		return err
	}
	matches, profileMatches := 0, 0
	for _, p := range profiles {
		if p.ID == m.HistoricalProfileID {
			profileMatches++
		}
		for _, d := range p.DeliverableExpectations {
			if d.ID != m.HistoricalDeliverableID {
				continue
			}
			// Coverage source IDs have no Profile namespace. Reject another
			// declaration of this ID rather than selecting a first match.
			matches++
			if p.ID != m.HistoricalProfileID || !d.Required || d.Description != m.HistoricalText ||
				m.HistoricalProfileHash != StableRuntimeProfileContractsHash([]RuntimeProfileContract{p}) {
				return fmt.Errorf("Profile evidence purpose source differs from confirmed required deliverable snapshot")
			}
		}
	}
	if matches != 1 || profileMatches != 1 {
		return fmt.Errorf("Profile evidence purpose source is missing or ambiguous")
	}
	return nil
}

func runtimeProfileEvidencePurposeSource(m RuntimeEvidencePurposeMapping) RuntimeDefinitionSource {
	return RuntimeDefinitionSource{CandidateItemRef: m.HistoricalItemRef, ItemRef: m.HistoricalItemRef,
		SourceScope: "deliverable", SourceID: m.HistoricalDeliverableID, Text: m.HistoricalText}
}

// Detached explicit source projection; it does not extend the original DoD
// graph or claim the Profile declaration is an execution/current proof.
func RuntimeEvidencePurposeSources(i RuntimeSourceObligationInventory) []RuntimeDefinitionSource {
	ret := slices.Clone(i.Sources)
	for _, m := range i.EvidencePurposeMappings {
		if m.ContractRef == RuntimeProfileDeliverableEvidencePurposeV2 {
			ret = append(ret, runtimeProfileEvidencePurposeSource(m))
		}
	}
	return ret
}

func runtimeProfileEvidencePurposeCheck(i RuntimeSourceObligationInventory, m RuntimeEvidencePurposeMapping, item RuntimeCoverageItem) RuntimeSourceObligationCheck {
	digest := sha256.Sum256([]byte(m.HistoricalItemRef))
	id := hex.EncodeToString(digest[:])
	check := RuntimeSourceObligationCheck{InventoryHash: i.Hash, ObligationID: "profile-obligation-" + id,
		RequirementID: "profile-requirement-" + id, VerificationID: "profile-verification-" + id,
		Criterion: m.HistoricalText, Declaration: m.HistoricalText}
	check.CheckRef = StableRuntimeSourceObligationCheckRef(i.DefinitionRevision, item.ID, check)
	return check
}
