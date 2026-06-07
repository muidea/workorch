package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func IDsFromPatches(items []readmodelcore.Patch) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func IDsFromValidations(items []readmodelcore.ValidationResult) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID, item.EvidenceRef)
	}
	return compactUniqueStrings(ret)
}
