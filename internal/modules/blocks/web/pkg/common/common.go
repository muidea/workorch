package common

import runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

const ModuleID = runtimemodel.RuntimeModuleWeb

const StepKindSearch = runtimemodel.StepKindWebSearch
const StepKindFetch = runtimemodel.StepKindWebFetch

// RuntimeCapabilityKinds is the Web owner's complete Runtime capability inventory.
func RuntimeCapabilityKinds() []string { return []string{StepKindSearch, StepKindFetch} }

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
