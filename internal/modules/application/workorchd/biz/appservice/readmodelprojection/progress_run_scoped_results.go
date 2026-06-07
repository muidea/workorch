package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func RunScopedResults(snapshot readmodelcore.RunSnapshot) []readmodelcore.Result {
	return runScopedResults(snapshot)
}
