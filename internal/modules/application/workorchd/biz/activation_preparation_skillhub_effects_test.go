package biz

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

type preparationStatusRunnerFunc func(context.Context, string, bool) (contextregistryports.SkillHubPreparationOutput, error)

func (f preparationStatusRunnerFunc) ObserveStatus(ctx context.Context, dir string, global bool) (contextregistryports.SkillHubPreparationOutput, error) {
	return f(ctx, dir, global)
}

func TestPreparationSkillHubErrorRetainsBytesOrExplicitUnsaved(t *testing.T) {
	for _, raw := range []string{"原文\r\n\xff", strings.Repeat("x", workspace.PreparationSkillHubErrorLimit+1)} {
		var f workspace.PreparationSkillHubStatusFacts
		preservePreparationSkillHubError(&f, errors.New(raw))
		if f.ErrorBytes != uint64(len(raw)) || f.ErrorSHA256 != fileobservation.Digest([]byte(raw)) {
			t.Fatal("error identity changed")
		}
		if len(raw) <= workspace.PreparationSkillHubErrorLimit {
			if string(f.ErrorText) != raw || f.ErrorNotSavedReason != "" {
				t.Fatal("raw error changed")
			}
		} else if len(f.ErrorText) != 0 || f.ErrorNotSavedReason != "error_exceeds_storage_limit" {
			t.Fatal("error silently truncated")
		}
	}
}
