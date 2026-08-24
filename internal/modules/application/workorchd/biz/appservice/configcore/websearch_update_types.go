package configcore

type UpdateWebSearchConfigurationCommand struct {
	DefaultProviderID *string                                 `json:"defaultProviderId,omitempty"`
	Providers         *[]UpdateWebSearchProviderConfiguration `json:"providers,omitempty"`
}

type UpdateWebSearchProviderConfiguration struct {
	ID             string            `json:"id"`
	Provider       *string           `json:"provider,omitempty"`
	SearchEndpoint *string           `json:"searchEndpoint,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	APIKey         *string           `json:"apiKey,omitempty"`
	Timeout        *string           `json:"timeout,omitempty"`
	MaxResults     *int              `json:"maxResults,omitempty"`
	MaxFetchBytes  *int64            `json:"maxFetchBytes,omitempty"`
	MaxRedirects   *int              `json:"maxRedirects,omitempty"`
	AllowDomains   []string          `json:"allowDomains,omitempty"`
	DenyDomains    []string          `json:"denyDomains,omitempty"`
	Disabled       *bool             `json:"disabled,omitempty"`
}
