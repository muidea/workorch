package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func RunScopedArtifacts(snapshot readmodelcore.RunSnapshot) []projectioncontract.Artifact {
	if snapshot.Run.ID == "" {
		return snapshot.Artifacts
	}
	ret := make([]projectioncontract.Artifact, 0, len(snapshot.Artifacts))
	for _, item := range snapshot.Artifacts {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}
