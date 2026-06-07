package governancecore

import "strings"

func RecordProviderConfigKey(key string, hits map[string]bool) {
	normalized := NormalizeProviderConfigText(key)
	switch normalized {
	case "provider":
		hits["provider"] = true
	case "baseurl", "baseuri", "endpoint", "apiurl":
		hits["baseURL"] = true
	case "model", "modelid":
		hits["model"] = true
	case "apikey", "token", "secret":
		hits["apiKey"] = true
	}
}

func RecordProviderConfigText(value string, hits map[string]bool) {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "sk-") || strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") || strings.Contains(lower, "api key") {
		hits["apiKey"] = true
	}
	for _, marker := range []string{"provider", "baseurl", "base_url", "base url", "endpoint", "model"} {
		if strings.Contains(lower, marker) {
			RecordProviderConfigKey(marker, hits)
		}
	}
}

func NormalizeProviderConfigText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
	return replacer.Replace(value)
}
