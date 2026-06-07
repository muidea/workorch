package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MergeWorkspaceSnapshotPatches(existing []readmodelcore.Patch, latest []readmodelcore.Patch) []readmodelcore.Patch {
	if len(existing) == 0 {
		return latest
	}
	if len(latest) == 0 {
		return existing
	}
	seen := make(map[string]int, len(existing)+len(latest))
	out := make([]readmodelcore.Patch, 0, len(existing)+len(latest))
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
