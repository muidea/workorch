package workorchruntimecontract

// This binary supports the coordinated historical-progress/current-acceptance
// consumer. Availability does not grant proof or replace bootstrap/seed source,
// profile, dependency-closure and original confirmation admission.
func ValidateRuntimeEvidencePurposeMappingAvailability(inventory *RuntimeSourceObligationInventory) error {
	if inventory != nil && len(inventory.EvidencePurposeMappings) != 0 {
		return inventory.Validate()
	}
	return nil
}
