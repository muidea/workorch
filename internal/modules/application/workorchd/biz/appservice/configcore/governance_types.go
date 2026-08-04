package configcore

import "github.com/muidea/workorch/internal/pkg/workorchconfig"

type GovernanceConfiguration struct {
	AutoTicket   GovernanceAutoTicketConfiguration     `json:"autoTicket"`
	AutoApproval GovernanceAutoApprovalConfiguration   `json:"autoApproval"`
	Budget       workorchconfig.GovernanceBudgetConfig `json:"budget"`
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
