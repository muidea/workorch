package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func AppliedPatchTargetCount(items []readmodelcore.Patch) int {
	if len(items) == 0 {
		return 0
	}
	targets := map[string]struct{}{}
	for _, item := range items {
		if item.State != string(common.PatchStateApplied) {
			continue
		}
		target := strings.TrimSpace(item.TargetRef)
		if target == "" {
			target = strings.TrimSpace(item.ID)
		}
		if target == "" {
			continue
		}
		targets[target] = struct{}{}
	}
	return len(targets)
}
