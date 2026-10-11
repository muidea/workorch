package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeValidationInputTargetsMaxCount bounds non-selectable protection
// observations, not owner inventory or candidate context choices. Observations
// must retain the complete bounded input set across proof
// batches; applying the 32-choice limit would lose protection or reject later
// conjunctive additions. This limit grants no additional operation targets.
const RuntimeValidationInputTargetsMaxCount = 512

func runtimeValidationInputProtectionCatalogs(catalogs []RuntimeOperationAdmissibility, operations []RuntimeCandidateOperationManifest) []RuntimeOperationAdmissibility {
	var ret []RuntimeOperationAdmissibility
	for _, catalog := range catalogs {
		if runtimeValidationInputProtectionCatalogReferenced(catalog, operations) {
			ret = append(ret, catalog)
		}
	}
	return NormalizeRuntimeOperationAdmissibilities(ret)
}

func runtimeValidationInputProtectionCatalogReferenced(catalog RuntimeOperationAdmissibility, operations []RuntimeCandidateOperationManifest) bool {
	for _, surface := range catalog.ValidationSurfaces {
		for _, operation := range operations {
			if len(surface.InputTargetRefs) != 0 && surface.InputTargetOwnerModuleID == operation.OwnerModuleID {
				return true
			}
		}
	}
	return false
}

func validateRuntimeManifestInputProtection(manifest RuntimeCandidateContractManifest) error {
	if len(manifest.MutationInputCatalogs) != 0 && manifest.State != ConvergenceStateNeedStateChange {
		return fmt.Errorf("mutation input protection requires a state-change manifest")
	}
	for _, catalog := range manifest.MutationInputCatalogs {
		if !runtimeValidationInputProtectionCatalogReferenced(catalog, manifest.Operations) {
			return fmt.Errorf("mutation input protection contains an unreferenced catalog")
		}
		if err := catalog.Validate(); err != nil {
			return fmt.Errorf("mutation input catalog: %w", err)
		}
	}
	for _, operation := range manifest.Operations {
		facts := RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: manifest.MutationInputCatalogs, InputRepair: operation.InputRepair}
		required := RequiredOutputContract{RepairPurpose: manifest.RepairPurpose, RepairClosureID: manifest.RepairClosureID}
		want := runtimeValidationInputMutationExclusions(operation.OwnerModuleID, facts, required, manifest.Bindings)
		if !sameStrings(want, operation.Target.ExcludedTargetRefs) {
			return fmt.Errorf("mutation exclusions do not match exact owner input preimages")
		}
	}
	return nil
}

func validateRuntimeValidationInputTargets(surface RuntimeValidationSurface) error {
	if (surface.InputTargetOwnerModuleID == "") != (len(surface.InputTargetRefs) == 0) {
		return fmt.Errorf("validation input targets require resource owner and targets together")
	}
	if len(surface.InputTargetRefs) == 0 {
		return nil
	}
	knownOwner := false
	for _, descriptor := range runtimeOperationDescriptorInventory {
		knownOwner = knownOwner || descriptor.OwnerModuleID == surface.InputTargetOwnerModuleID
	}
	if !knownOwner || len(surface.InputTargetRefs) > RuntimeValidationInputTargetsMaxCount {
		return fmt.Errorf("validation input targets have unknown resource owner or exceed inventory bound")
	}
	seen := make(map[string]bool, len(surface.InputTargetRefs))
	for _, target := range surface.InputTargetRefs {
		if target == "" || strings.TrimSpace(target) != target || len(target) > 2048 || seen[target] ||
			strings.IndexFunc(target, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return fmt.Errorf("validation input targets contain an invalid or duplicate identity")
		}
		seen[target] = true
	}
	return nil
}

// Validation-input protection is a pure compilation of owner facts. It does
// not inspect files, infer an input defect, or select a Runtime transition.
func runtimeValidationInputMutationExclusions(owner string, facts RuntimeCandidateManifestOwnerFacts, required RequiredOutputContract, bindings []CoverageEvidenceBinding) []string {
	var targets []string
	for _, catalog := range facts.OperationAdmissibilities {
		for _, surface := range catalog.ValidationSurfaces {
			if surface.InputTargetOwnerModuleID == owner {
				targets = append(targets, surface.InputTargetRefs...)
			}
		}
	}
	targets = normalizeStrings(targets)
	// Only the existing exact, criterion-preserving transaction can open one
	// input target. A normal source-change candidate cannot borrow a diagnostic
	// or another requirement's repair identity. Final seal also checks the
	// receiving owner's before/after ReplayProof.
	repair := facts.InputRepair
	if repair != nil && required.RepairPurpose == RuntimeDispatchRepairPurposeConfirmedValidation &&
		required.RepairClosureID != "" && len(bindings) == 1 && repair.MatchesBinding(bindings[0]) {
		targets = runtimeTargetsWithoutExcluded(targets, []string{repair.TargetRef})
	}
	return targets
}

func runtimeTargetsWithoutExcluded(targets, excluded []string) []string {
	ret := make([]string, 0, len(targets))
	for _, target := range targets {
		if !containsString(excluded, target) {
			ret = append(ret, target)
		}
	}
	return nilIfEmpty(ret)
}

func writeRuntimeExcludedTargetsHash(h hashStringWriter, targets []string) {
	if len(targets) != 0 {
		writeHashString(h, "runtime-target-mutation-exclusions/v1")
		writeHashStrings(h, targets)
	}
}

func writeRuntimeValidationInputTargetsHash(h hashStringWriter, surface RuntimeValidationSurface) {
	if len(surface.InputTargetRefs) != 0 || surface.InputTargetOwnerModuleID != "" {
		writeHashString(h, "runtime-validation-input-targets/v1")
		writeHashString(h, surface.InputTargetOwnerModuleID)
		writeHashStrings(h, surface.InputTargetRefs)
	}
}
