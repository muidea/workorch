package contracts

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type CreateServerCommand struct {
	ServerID              common.MCPServerID
	Name                  string
	Transport             string
	Endpoint              string
	Command               string
	Args                  []string
	Env                   map[string]string
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	AllowedPrompts        []string
	DefaultApprovalPolicy string
	SessionPolicy         string
	TimeoutMillis         int
}

type UpdateServerCommand = CreateServerCommand

type CapabilityQuery struct {
	ServerID  common.MCPServerID
	Kind      string
	Category  string
	Stage     string
	RiskLevel string
	Enabled   *bool
	Query     string
}

type CreateCapabilityCommand struct {
	ID             common.MCPCapabilityID
	ServerID       common.MCPServerID
	Kind           string
	Name           string
	DisplayName    string
	Description    string
	Category       string
	Tags           []string
	UsageHint      string
	InputSummary   string
	OutputSummary  string
	StageHints     []string
	RiskLevel      string
	Schema         workorcheventpayload.Payload
	URI            string
	Enabled        bool
	ApprovalPolicy string
	Status         string
}

type UpdateCapabilityCommand = CreateCapabilityCommand
