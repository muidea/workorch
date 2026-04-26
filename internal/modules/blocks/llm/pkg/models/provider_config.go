package models

import "time"

type ProviderConfig struct {
	Provider          string
	BaseURL           string
	APIKey            string
	Model             string
	Timeout           time.Duration
	Disabled          bool
	Unavailable       bool
	UnavailableReason string
}

type RedactedProviderConfig struct {
	Provider string `json:"provider,omitempty"`
	BaseURL  string `json:"baseUrl,omitempty"`
	Model    string `json:"model,omitempty"`
	Timeout  string `json:"timeout,omitempty"`
	Disabled bool   `json:"disabled"`
	Ready    bool   `json:"ready"`
	Reason   string `json:"reason,omitempty"`
}
