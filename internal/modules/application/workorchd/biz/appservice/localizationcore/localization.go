package localizationcore

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/pkg/dialoglanguage"
)

func DetectSystemLocale() string {
	return dialoglanguage.FromEnvironment()
}

func LanguageNameFromLocale(locale string) string {
	return dialoglanguage.NormalizeTag(locale)
}

func PrefersChinese(language string) bool {
	return dialoglanguage.PrefersChinese(language)
}

func DefaultDialogLanguage() string {
	return DetectSystemLocale()
}

func DefaultDialogLanguageFromContext(ctx context.Context) string {
	if cfg, err := configcore.LoadFromContext(ctx); err == nil {
		if lang := dialoglanguage.NormalizeTag(cfg.Server.DefaultDialogLanguage); lang != "" {
			return lang
		}
	}
	return DefaultDialogLanguage()
}
