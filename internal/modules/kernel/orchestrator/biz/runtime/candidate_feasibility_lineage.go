package runtime

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The SM-3 failed exit retains only catalog preimages actually used by its
// probe facts, within the same owner-private CAS as the disposition. Current
// catalogs remain the only input to admission; this ledger is reverse proof.
func retainFailedCandidateFeasibilityCatalogs(aggregate *RuntimeAggregate, idx int) error {
	attempt := &aggregate.CandidateAttempts[idx]
	var catalogs []runtimecontract.RuntimeOperationAdmissibility
	seen := map[string]bool{}
	for _, fact := range attempt.Feasibilities {
		if seen[fact.AdmissibilitySnapshotID] {
			continue
		}
		found := false
		for _, catalog := range aggregate.Facts.Resources.OperationAdmissibilities {
			if catalog.SnapshotID == fact.AdmissibilitySnapshotID && catalog.AcquisitionBinding == fact.AcquisitionBinding {
				catalogs = append(catalogs, catalog.Clone())
				seen[catalog.SnapshotID] = true
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("candidate feasibility has no current owner catalog to retain")
		}
	}
	attempt.FeasibilityCatalogs = catalogs
	return nil
}

func candidateManifestBindsFeasibilityCatalog(manifest runtimecontract.RuntimeCandidateContractManifest, kind, snapshotID string) bool {
	for _, operation := range manifest.Operations {
		if operation.Kind != kind {
			continue
		}
		for _, ref := range operation.AdmissibilitySnapshotRefs {
			if ref == snapshotID {
				return true
			}
		}
	}
	return false
}
