package contextregistrycore

type MCPServer struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Transport             string            `json:"transport"`
	Endpoint              string            `json:"endpoint,omitempty"`
	Command               string            `json:"command,omitempty"`
	Args                  []string          `json:"args,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	Enabled               bool              `json:"enabled"`
	AllowedTools          []string          `json:"allowedTools,omitempty"`
	AllowedResources      []string          `json:"allowedResources,omitempty"`
	AllowedPrompts        []string          `json:"allowedPrompts,omitempty"`
	DefaultApprovalPolicy string            `json:"defaultApprovalPolicy,omitempty"`
	SessionPolicy         string            `json:"sessionPolicy,omitempty"`
	TimeoutMillis         int               `json:"timeoutMillis,omitempty"`
}

type MCPTool struct {
	ServerID string `json:"serverId"`
	Name     string `json:"name"`
}

type MCPResource struct {
	ServerID string `json:"serverId"`
	URI      string `json:"uri"`
}

type MCPPrompt struct {
	ServerID string `json:"serverId"`
	Name     string `json:"name"`
}

type MCPCapability struct {
	ID               string         `json:"id"`
	ServerID         string         `json:"serverId"`
	Kind             string         `json:"kind"`
	Name             string         `json:"name"`
	DisplayName      string         `json:"displayName"`
	Description      string         `json:"description,omitempty"`
	Category         string         `json:"category,omitempty"`
	Tags             []string       `json:"tags,omitempty"`
	UsageHint        string         `json:"usageHint,omitempty"`
	InputSummary     string         `json:"inputSummary,omitempty"`
	OutputSummary    string         `json:"outputSummary,omitempty"`
	StageHints       []string       `json:"stageHints,omitempty"`
	RiskLevel        string         `json:"riskLevel,omitempty"`
	EffectScope      string         `json:"effectScope,omitempty"`
	IdempotencyClass string         `json:"idempotencyClass,omitempty"`
	Schema           map[string]any `json:"schema,omitempty"`
	URI              string         `json:"uri,omitempty"`
	Enabled          bool           `json:"enabled"`
	ApprovalPolicy   string         `json:"approvalPolicy,omitempty"`
	Status           string         `json:"status,omitempty"`
}

type MCPCapabilityQuery struct {
	ServerID  string
	Kind      string
	Category  string
	Stage     string
	RiskLevel string
	Enabled   *bool
	Query     string
}
