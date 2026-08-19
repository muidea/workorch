package events

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

type CLIStringEntry struct {
	Key   string
	Value string
}

type CLIMCPServerMutation struct {
	ID                    string
	Name                  string
	Transport             string
	Endpoint              string
	Command               string
	Args                  []string
	EnvironmentRefs       []CLIStringEntry
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	AllowedPrompts        []string
	DefaultApprovalPolicy string
	SessionPolicy         string
	TimeoutMillis         int
}

type CLIMCPServer CLIMCPServerMutation

type CLIMCPCapability struct {
	ID                           string
	ServerID                     string
	Kind                         string
	Name                         string
	DisplayName                  string
	Description                  string
	Category                     string
	Tags                         []string
	UsageHint                    string
	InputSummary                 string
	OutputSummary                string
	StageHints                   []string
	RiskLevel                    string
	EffectScope                  string
	IdempotencyClass             string
	ValidationProofApplicability string
	Schema                       workorcheventpayload.Payload
	URI                          string
	Enabled                      bool
	ApprovalPolicy               string
	Source                       string
	SourceRef                    string
	Revision                     uint64
}
