package common

const ModuleID = "blocks/web"

const StepKindSearch = "web/search"
const StepKindFetch = "web/fetch"

const ProviderHTTP = "http"

const (
	ErrorProviderUnavailable = "provider_unavailable"
	ErrorInvalidRequest      = "invalid_request"
	ErrorPolicyDenied        = "policy_denied"
	ErrorApprovalRequired    = "approval_required"
	ErrorTimeout             = "timeout"
	ErrorBadGatewayResponse  = "bad_gateway_response"
	ErrorContentTooLarge     = "content_too_large"
)
