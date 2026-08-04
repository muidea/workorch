package workorchprojectioncontract

type RuntimeCapability struct {
	ID             string   `json:"id"`
	ServerID       string   `json:"serverId,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Name           string   `json:"name,omitempty"`
	DisplayName    string   `json:"displayName,omitempty"`
	Description    string   `json:"description,omitempty"`
	Category       string   `json:"category,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	RiskLevel      string   `json:"riskLevel,omitempty"`
	Enabled        bool     `json:"enabled"`
	ApprovalPolicy string   `json:"approvalPolicy,omitempty"`
	Status         string   `json:"status,omitempty"`
	UseCount       int      `json:"useCount,omitempty"`
	EvidenceRefs   []string `json:"evidenceRefs,omitempty"`
}
