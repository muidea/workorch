package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func IDsFromPatches(items []projectioncontract.Patch) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func IDsFromValidations(items []projectioncontract.ValidationResult) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID, item.EvidenceRef)
	}
	return compactUniqueStrings(ret)
}
