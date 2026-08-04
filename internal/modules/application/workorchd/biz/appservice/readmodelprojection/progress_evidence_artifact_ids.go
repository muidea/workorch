package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func IDsFromResults(items []projectioncontract.Result) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
		ret = append(ret, item.ArtifactRefs...)
	}
	return compactUniqueStrings(ret)
}

func IDsFromArtifacts(items []projectioncontract.Artifact) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
	}
	return compactUniqueStrings(ret)
}
