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
	if m.ContractRef != RuntimeProfileDeliverableEvidencePurpose ||
		!runtimeSourceIdentity(m.HistoricalProfileID, 128) || !runtimeSourceIdentity(m.HistoricalDeliverableID, 128) ||
		!validHistoricalProfileSourceHash(m.HistoricalProfileHash) ||
		m.HistoricalItemRef != RuntimeProfileDeliverablePurposeRef(m.HistoricalProfileID, m.HistoricalDeliverableID) ||
		strings.TrimSpace(m.HistoricalText) == "" || len(m.HistoricalText) > 4096 || !utf8.ValidString(m.HistoricalText) || strings.ContainsRune(m.HistoricalText, 0) {
		return fmt.Errorf("Profile evidence purpose requires exact bounded snapshot identity and complete original deliverable text")
	}
	return nil
}

// ValidateRuntimeHistoricalProfileEvidencePurposeSources authenticates the source
// against its original confirmed Profile. Current applicability is a separate
// check: a later declaration is not the preimage of a historical source hash.
func ValidateRuntimeHistoricalProfileEvidencePurposeSources(inventory *RuntimeSourceObligationInventory, original []RuntimeProfileContract) error {
	if inventory == nil {
		return nil
	}
	for _, m := range inventory.EvidencePurposeMappings {
		if m.ContractRef != RuntimeProfileDeliverableEvidencePurpose {
			continue
		}
		if err := validateRuntimeProfileEvidencePurposeSource(m, original); err != nil {
			return err
		}
		for _, p := range original {
			if p.ID == m.HistoricalProfileID {
				if m.HistoricalProfileHash != StableRuntimeProfileContractsHash([]RuntimeProfileContract{p}) {
					return fmt.Errorf("Profile evidence purpose differs from original Profile hash")
				}
				for _, d := range p.DeliverableExpectations {
					if d.ID == m.HistoricalDeliverableID && d.Description != m.HistoricalText {
						return fmt.Errorf("Profile evidence purpose differs from complete original deliverable text")
					}
				}
			}
		}
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
			// Historical text/hash authenticate the original source inventory, not the
			// full current Profile. A policy or descriptive update cannot retire
			// this business source; current existence/requiredness is still checked.
			// Coverage source IDs have no Profile namespace. Reject another
			// declaration of this ID rather than selecting a first match.
			matches++
			if p.ID != m.HistoricalProfileID || !d.Required {
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
		if m.ContractRef == RuntimeProfileDeliverableEvidencePurpose {
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

func validHistoricalProfileSourceHash(value string) bool {
	const prefix = "runtime-profile-contracts:candidate-prefix-"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	raw := strings.TrimPrefix(value, prefix)
	bytes, err := hex.DecodeString(raw)
	return err == nil && len(bytes) == 10 && hex.EncodeToString(bytes) == raw
}
