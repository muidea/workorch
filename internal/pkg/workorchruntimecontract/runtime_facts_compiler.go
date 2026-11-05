package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

type runtimeCatalogCollectionKind uint8

const (
	runtimeFactsCatalogCollection runtimeCatalogCollectionKind = iota
	runtimeSemanticCatalogCollection
)

func compileRuntimeFactsCatalogsInSession(raw []RuntimeOperationAdmissibility, session *runtimeSemanticValidationSession) ([]RuntimeOperationAdmissibility, error) {
	return compileRuntimeCatalogCollectionInSession(raw, session, runtimeFactsCatalogCollection)
}

// One implementation owns collection validation and canonical ordering. Facts
// permits multiple prerequisites from one owner; a semantic manifest additionally
// requires a unique owner. These are private contract entry points, not an
// already-valid flag. Every membership/duplicate gate runs in original order.
// Catalog subtrees use exact-raw detached entries; the result is read-only.
func compileRuntimeCatalogCollectionInSession(raw []RuntimeOperationAdmissibility, session *runtimeSemanticValidationSession, kind runtimeCatalogCollectionKind) ([]RuntimeOperationAdmissibility, error) {
	seen := make(map[string]bool, len(raw))
	var owners map[string]bool
	if kind == runtimeSemanticCatalogCollection {
		owners = make(map[string]bool, len(raw))
	}
	values := make([]RuntimeOperationAdmissibility, 0, len(raw))
	failure := func(err error) error {
		if kind == runtimeSemanticCatalogCollection {
			return fmt.Errorf("runtime semantic owner catalogs: %w", err)
		}
		return err
	}
	for _, catalog := range raw {
		value, err := validateRuntimeAdmissibilityInSemanticSession(catalog, session)
		if err != nil {
			return nil, failure(err)
		}
		id := strings.TrimSpace(catalog.PrerequisiteID)
		if seen[id] {
			return nil, failure(fmt.Errorf("runtime operation admissibility duplicates prerequisite %q", id))
		}
		seen[id] = true
		if kind == runtimeSemanticCatalogCollection {
			if owners[value.OwnerModuleID] {
				return nil, fmt.Errorf("runtime semantic owner catalogs duplicate owner %q", value.OwnerModuleID)
			}
			owners[value.OwnerModuleID] = true
		}
		values = append(values, value)
	}
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].PrerequisiteID != values[j].PrerequisiteID {
			return values[i].PrerequisiteID < values[j].PrerequisiteID
		}
		return values[i].OwnerModuleID < values[j].OwnerModuleID
	})
	return nilIfEmpty(values), nil
}
