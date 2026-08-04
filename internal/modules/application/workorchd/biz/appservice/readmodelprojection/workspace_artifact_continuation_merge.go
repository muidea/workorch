package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func MergeWorkspaceSnapshotArtifactContinuations(existing []projectioncontract.ArtifactContinuation, latest []projectioncontract.ArtifactContinuation) []projectioncontract.ArtifactContinuation {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]projectioncontract.ArtifactContinuation, 0, len(existing)+len(latest))
	for _, item := range existing {
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	for _, item := range latest {
		if idx, ok := seen[item.ID]; ok {
			out[idx] = item
			continue
		}
		seen[item.ID] = len(out)
		out = append(out, item)
	}
	return out
}
