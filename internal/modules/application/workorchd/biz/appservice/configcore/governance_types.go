package configcore

type GovernanceConfiguration struct {
	AutoTicket   GovernanceAutoTicketConfiguration   `json:"autoTicket"`
	AutoApproval GovernanceAutoApprovalConfiguration `json:"autoApproval"`
	Budget       GovernanceBudgetConfiguration       `json:"budget"`
}

type GovernanceAutoTicketConfiguration struct {
	Enabled       bool    `json:"enabled"`
	MinConfidence float64 `json:"minConfidence"`
	MaxPerRun     int     `json:"maxPerRun"`
}

type GovernanceAutoApprovalConfiguration struct {
	Enabled             bool     `json:"enabled"`
	MinConfidence       float64  `json:"minConfidence"`
	MaxPerRun           int      `json:"maxPerRun"`
	AllowReadOnly       bool     `json:"allowReadOnly"`
	AllowWorkspaceWrite bool     `json:"allowWorkspaceWrite"`
	RequireHumanFor     []string `json:"requireHumanFor,omitempty"`
}

type GovernanceBudgetConfiguration struct {
	MaxConsecutiveFailures int `json:"maxConsecutiveFailures"`
	MaxRepeatedSamePrompt  int `json:"maxRepeatedSamePrompt"`
}
