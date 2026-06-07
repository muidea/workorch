package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func runScopedResults(snapshot readmodelcore.RunSnapshot) []readmodelcore.Result {
	if snapshot.Run.ID == "" {
		return snapshot.Results
	}
	ret := make([]readmodelcore.Result, 0, len(snapshot.Results))
	for _, item := range snapshot.Results {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}
