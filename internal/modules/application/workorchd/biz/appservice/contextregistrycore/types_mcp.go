package contextregistrycore

type MCPCapabilityQuery struct {
	ServerID  string
	Kind      string
	Category  string
	Stage     string
	RiskLevel string
	Enabled   *bool
	Query     string
}
