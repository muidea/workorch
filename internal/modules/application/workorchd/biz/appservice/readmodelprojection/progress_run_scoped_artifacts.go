package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func RunScopedArtifacts(snapshot readmodelcore.RunSnapshot) []readmodelcore.Artifact {
	if snapshot.Run.ID == "" {
		return snapshot.Artifacts
	}
	ret := make([]readmodelcore.Artifact, 0, len(snapshot.Artifacts))
	for _, item := range snapshot.Artifacts {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}
