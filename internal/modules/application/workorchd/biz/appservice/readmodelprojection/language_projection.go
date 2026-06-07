package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/localizationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func SnapshotLanguage(snapshot readmodelcore.RunSnapshot) string {
	if snapshot.Run.ExecutionContext == nil {
		return localizationcore.DefaultDialogLanguage()
	}
	if lang, ok := snapshot.Run.ExecutionContext["dialogLanguage"].(string); ok {
		if normalized := localizationcore.LanguageNameFromLocale(lang); normalized != "" {
			return normalized
		}
	}
	return localizationcore.DefaultDialogLanguage()
}

func LanguageFromExecutionContext(executionContext map[string]any) string {
	if executionContext == nil {
		return ""
	}
	if lang, ok := executionContext["dialogLanguage"].(string); ok {
		return localizationcore.LanguageNameFromLocale(lang)
	}
	return ""
}
