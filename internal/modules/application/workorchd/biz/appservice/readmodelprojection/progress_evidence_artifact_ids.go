package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func IDsFromResults(items []readmodelcore.Result) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
		ret = append(ret, item.ArtifactRefs...)
	}
	return compactUniqueStrings(ret)
}

func IDsFromArtifacts(items []readmodelcore.Artifact) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
	}
	return compactUniqueStrings(ret)
}
