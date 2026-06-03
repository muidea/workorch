package models

import "time"

type ProviderConfig struct {
	ID                string
	Provider          string
	SearchEndpoint    string
	FetchEndpoint     string
	APIKey            string
	Headers           map[string]string
	Timeout           time.Duration
	MaxResults        int
	MaxFetchBytes     int64
	MaxRedirects      int
	AllowDomains      []string
	DenyDomains       []string
	Disabled          bool
	Unavailable       bool
	UnavailableReason string
}
