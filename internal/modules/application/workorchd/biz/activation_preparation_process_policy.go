package biz

import (
	"context"
	"os"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Observe only the original continuation's local, non-sensitive inputs. A
// missing loader is not a zero/default configuration observation. No global
// config fallback, provider access, managed write or Runtime builder is used.
func observePreparationProcessPolicy(ctx context.Context, original workspaceevents.CheckWorkspacePreparationCommand) (*workspace.PreparationProcessPolicyFacts, error) {
	if ctx == nil || original.Validate() != nil || original.ExpectedWorkspaceRevision == ^uint64(0) || configcore.LoaderFromContext(ctx) == nil {
		return nil, common.ErrConflict
	}
	read := func() (workspace.PreparationProcessPolicy, error) {
		if err := ctx.Err(); err != nil {
			return workspace.PreparationProcessPolicy{}, err
		}
		cfg, err := configcore.LoadFromContext(ctx)
		if err != nil {
			return workspace.PreparationProcessPolicy{}, err
		}
		return workspace.PreparationProcessPolicy{
			ConfiguredDialogLanguage: cfg.Server.DefaultDialogLanguage,
			LCAll:                    os.Getenv("LC_ALL"), LCMessages: os.Getenv("LC_MESSAGES"), Lang: os.Getenv("LANG"),
			MaxRuntimeRepairAttempts:    cfg.Runtime.MaxRuntimeRepairAttempts,
			MaxDocumentRecoveryAttempts: cfg.Runtime.LLMOutput.MaxDocumentRecoveryAttempts,
		}, nil
	}
	f := workspace.PreparationProcessPolicyFacts{Version: workspace.PreparationExecutionInputsVersion, WorkspaceID: original.WorkspaceID, WorkspaceRevision: original.ExpectedWorkspaceRevision + 1, StartedAt: time.Now().UTC()}
	var err error
	if f.Before, err = read(); err != nil {
		return nil, err
	}
	if f.After, err = read(); err != nil {
		return nil, err
	}
	f.CompletedAt = time.Now().UTC()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}
	return &f, nil
}
