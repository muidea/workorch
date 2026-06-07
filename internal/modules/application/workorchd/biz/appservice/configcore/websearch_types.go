package configcore

type WebSearchConfiguration struct {
	DefaultProviderID string                           `json:"defaultProviderId"`
	Providers         []WebSearchProviderConfiguration `json:"providers"`
	ProviderTemplates []WebSearchProviderTemplate      `json:"providerTemplates"`
}

type WebSearchProviderConfiguration struct {
	ID               string            `json:"id"`
	Provider         string            `json:"provider"`
	SearchEndpoint   string            `json:"searchEndpoint,omitempty"`
	FetchEndpoint    string            `json:"fetchEndpoint,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Timeout          string            `json:"timeout,omitempty"`
	MaxResults       int               `json:"maxResults,omitempty"`
	MaxFetchBytes    int64             `json:"maxFetchBytes,omitempty"`
	MaxRedirects     int               `json:"maxRedirects,omitempty"`
	AllowDomains     []string          `json:"allowDomains,omitempty"`
	DenyDomains      []string          `json:"denyDomains,omitempty"`
	Disabled         bool              `json:"disabled"`
	Ready            bool              `json:"ready"`
	Reason           string            `json:"reason,omitempty"`
	Default          bool              `json:"default"`
	APIKeyConfigured bool              `json:"apiKeyConfigured"`
}

type WebSearchProviderTemplate struct {
	TemplateID     string `json:"templateId"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	ID             string `json:"id"`
	Provider       string `json:"provider"`
	Timeout        string `json:"timeout,omitempty"`
	MaxResults     int    `json:"maxResults,omitempty"`
	MaxFetchBytes  int64  `json:"maxFetchBytes,omitempty"`
	MaxRedirects   int    `json:"maxRedirects,omitempty"`
	SearchEndpoint string `json:"searchEndpoint,omitempty"`
	FetchEndpoint  string `json:"fetchEndpoint,omitempty"`
}
