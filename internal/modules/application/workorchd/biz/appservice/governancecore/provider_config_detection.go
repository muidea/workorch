package governancecore

func ContainsInlineLLMProviderConfig(payload map[string]any) bool {
	if len(payload) == 0 {
		return false
	}
	hits := map[string]bool{}
	VisitProviderConfigValue(payload, hits)
	if hits["apiKey"] || hits["secret"] {
		return true
	}
	configHits := 0
	for _, key := range []string{"provider", "baseURL", "model"} {
		if hits[key] {
			configHits++
		}
	}
	return configHits >= 2
}

func VisitProviderConfigValue(value any, hits map[string]bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			RecordProviderConfigKey(key, hits)
			VisitProviderConfigValue(nested, hits)
		}
	case []any:
		for _, nested := range typed {
			VisitProviderConfigValue(nested, hits)
		}
	case []map[string]any:
		for _, nested := range typed {
			VisitProviderConfigValue(nested, hits)
		}
	case string:
		RecordProviderConfigText(typed, hits)
	}
}
