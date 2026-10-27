package biz

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestPreparationProcessPolicyPreservesRawInputsAndActivationLanguage(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LC_MESSAGES", "en_US.UTF-8")
	t.Setenv("LANG", "C")
	for _, language := range []string{"  中文  ", "en", ""} {
		cfg := workorchconfig.Config{}
		cfg.Server.DefaultDialogLanguage = language
		cfg.Runtime.MaxRuntimeRepairAttempts = 7
		cfg.Runtime.LLMOutput.MaxDocumentRecoveryAttempts = 2
		reads := 0
		ctx := configcore.ContextWithLoader(context.Background(), func() (workorchconfig.Config, error) { reads++; return cfg, nil })
		c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: "ws", RequestID: "original", ExpectedWorkspaceRevision: 3}
		facts, err := observePreparationProcessPolicy(ctx, c)
		if err != nil || facts.Validate() != nil || reads != 2 || facts.WorkspaceRevision != 4 || facts.Before.ConfiguredDialogLanguage != language || facts.Before.LCAll != "zh_CN.UTF-8" || facts.Before.MaxRuntimeRepairAttempts != 7 || facts.Before.MaxDocumentRecoveryAttempts != 2 {
			t.Fatal("incomplete original observation", facts, err, reads)
		}
		if facts.Before.DialogLanguage() != localizationcore.DefaultDialogLanguageFromContext(ctx) {
			t.Fatal("preparation language differs from activation")
		}
		saved := *facts
		cfg.Server.DefaultDialogLanguage = "new configuration"
		cfg.Runtime.MaxRuntimeRepairAttempts++
		if !reflect.DeepEqual(saved, *facts) {
			t.Fatal("archive aliases mutable configuration")
		}
	}
}

func TestPreparationProcessPolicyMissingCancelledDriftingOrInvalidFailsClosed(t *testing.T) {
	c := workspaceevents.CheckWorkspacePreparationCommand{WorkspaceID: "ws", RequestID: "original", ExpectedWorkspaceRevision: 3}
	if _, err := observePreparationProcessPolicy(context.Background(), c); err == nil {
		t.Fatal("missing loader became a default observation")
	}
	for _, scenario := range []string{"cancelled", "error", "drift", "locale_drift", "invalid_utf8", "invalid_limit"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("LC_ALL", "en")
			reads := 0
			cfg := workorchconfig.Config{}
			cfg.Runtime.MaxRuntimeRepairAttempts = 7
			cfg.Runtime.LLMOutput.MaxDocumentRecoveryAttempts = 2
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = configcore.ContextWithLoader(ctx, func() (workorchconfig.Config, error) {
				reads++
				if scenario == "error" {
					return cfg, errors.New("unavailable")
				}
				if scenario == "drift" {
					cfg.Runtime.MaxRuntimeRepairAttempts++
				}
				if scenario == "locale_drift" && reads == 2 {
					t.Setenv("LC_ALL", "zh")
				}
				if scenario == "invalid_utf8" {
					cfg.Server.DefaultDialogLanguage = "\xff"
				}
				if scenario == "invalid_limit" {
					cfg.Runtime.MaxRuntimeRepairAttempts = 0
				}
				return cfg, nil
			})
			if scenario == "cancelled" {
				cancel()
			}
			if _, err := observePreparationProcessPolicy(ctx, c); err == nil {
				t.Fatal("unobserved or changed input accepted")
			}
			if scenario == "cancelled" && reads != 0 {
				t.Fatal("read after cancellation")
			}
		})
	}
}
