package activationcore

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
)

func DialogLanguage(requested string, fallback string) string {
	if normalized := localizationcore.LanguageNameFromLocale(requested); normalized != "" {
		return normalized
	}
	return strings.TrimSpace(fallback)
}
