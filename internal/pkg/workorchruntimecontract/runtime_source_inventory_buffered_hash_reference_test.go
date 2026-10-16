package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Original full unbuffered length-prefix/field/cardinality encoding.
func referenceSourceInventoryUnbufferedHash(i RuntimeSourceObligationInventory) string {
	h := sha256.New()
	writeHashString(h, i.SchemaVersion)
	writeHashString(h, i.CandidateHash)
	writeHashString(h, i.SpecificationHash)
	writeHashString(h, i.DefinitionRevision)
	writeHashString(h, i.StartupInputHash)
	writeHashString(h, fmt.Sprintf("%d", len(i.Sources)))
	for _, source := range i.Sources {
		writeHashString(h, source.CandidateItemRef)
		writeHashString(h, source.ItemRef)
		writeHashString(h, source.SourceScope)
		writeHashString(h, source.SourceID)
		writeHashString(h, source.Text)
	}
	writeHashString(h, fmt.Sprintf("%d", len(i.Resources)))
	for _, resource := range i.Resources {
		writeHashString(h, resource.ID)
		writeHashString(h, resource.Ref)
		writeHashString(h, resource.SnapshotHash)
		writeHashString(h, fmt.Sprintf("%t", resource.Required))
		writeHashString(h, fmt.Sprintf("%t", resource.Available))
	}
	writeHashString(h, fmt.Sprintf("%d", len(i.Requirements)))
	for _, requirement := range i.Requirements {
		writeHashString(h, requirement.ID)
		writeHashString(h, requirement.DimensionID)
		writeHashString(h, fmt.Sprintf("%t", requirement.Required))
		writeHashString(h, requirement.SourceKind)
		writeHashString(h, requirement.Text)
		writeHashStrings(h, requirement.SourceItemRefs)
		writeHashStrings(h, requirement.ResourceIDs)
	}
	writeHashString(h, fmt.Sprintf("%d", len(i.Obligations)))
	for _, obligation := range i.Obligations {
		writeHashString(h, obligation.ID)
		writeHashString(h, obligation.RequirementID)
		writeHashString(h, obligation.Text)
		writeHashStrings(h, obligation.DoDItemRefs)
	}
	writeHashString(h, fmt.Sprintf("%d", len(i.Verifications)))
	for _, verification := range i.Verifications {
		writeHashString(h, verification.ID)
		writeHashString(h, verification.ObligationID)
		writeHashString(h, verification.Declaration)
		writeHashString(h, verification.ProfileID)
		writeHashString(h, verification.EvidenceRequirementID)
		writeHashString(h, verification.Scope)
	}
	if len(i.ExecutionProofMappings) != 0 {
		writeHashString(h, RuntimeCrossSourceExecutionProofV1)
		writeHashString(h, fmt.Sprint(len(i.ExecutionProofMappings)))
		for _, mapping := range i.ExecutionProofMappings {
			writeHashString(h, mapping.ContractRef)
			writeHashString(h, mapping.ConsumerItemRef)
			writeHashString(h, mapping.RuleRef)
			writeHashString(h, mapping.SourceItemRef)
			if mapping.ConsumerGoalText != "" {
				writeHashString(h, RuntimeGoalSourceExecutionProofV2)
				writeHashString(h, mapping.ConsumerGoalText)
			}
		}
	}
	if len(i.EvidencePurposeMappings) != 0 {
		writeHashString(h, RuntimeCrossCWIEvidencePurposeV1)
		writeHashString(h, fmt.Sprint(len(i.EvidencePurposeMappings)))
		for _, mapping := range i.EvidencePurposeMappings {
			writeHashString(h, mapping.ContractRef)
			writeHashString(h, mapping.HistoricalItemRef)
			writeHashString(h, mapping.AcceptanceItemRef)
			if runtimeEvidencePurposeHasProfileFields(mapping) {
				writeHashString(h, "profile-deliverable-source/v2")
				writeHashString(h, mapping.HistoricalProfileID)
				writeHashString(h, mapping.HistoricalDeliverableID)
				writeHashString(h, mapping.HistoricalProfileHash)
				writeHashString(h, mapping.HistoricalText)
			}
		}
	}
	return "runtime-source-obligation-inventory-sha256:" + hex.EncodeToString(h.Sum(nil))
}
