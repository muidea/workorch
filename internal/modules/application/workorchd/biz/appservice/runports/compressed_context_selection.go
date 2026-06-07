package runports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CompressedContextForLoadedRun(ctx context.Context, deps CompressedContextDependencies, item *runmodel.Run) (map[string]any, error) {
	if item == nil || item.ID == "" {
		return nil, common.ErrInvalidArgument
	}
	defaultLanguage := strings.TrimSpace(deps.DefaultLanguage)
	if defaultLanguage == "" {
		defaultLanguage = localizationcore.DefaultDialogLanguageFromContext(ctx)
	}
	language := runtimecore.RunDialogLanguage(item, defaultLanguage)
	compressed := runtimecore.CreatedRunCompressedContext(item, language)
	if compressed != nil {
		return compressed, nil
	}
	compressed, err := BlockedRunResumeCompressedContext(ctx, deps.Tickets, deps.Resolutions, deps.Approvals, item, language)
	if err != nil {
		return nil, err
	}
	if compressed != nil {
		return compressed, nil
	}
	if deps.RecoverySnapshot == nil {
		return nil, common.ErrInvalidArgument
	}
	snapshot, err := deps.RecoverySnapshot(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	return runtimecore.CompressedContextMap(readmodelprojection.CompressedContextFromLocalizedRunSnapshot(snapshot)), nil
}
