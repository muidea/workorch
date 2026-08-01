package common

import (
	"fmt"
	"strings"

	basecommon "github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = "blocks/decision"

// RuntimeCapabilityKinds is the Decision owner's complete Runtime capability inventory.
func RuntimeCapabilityKinds() []string {
	return []string{runtimecontract.RuntimeBlockKindDecisionRequest}
}

const (
	ResolutionTypeReplan             = "replan"
	ResolutionTypeRejectCurrentPath  = "reject_current_path"
	ResolutionTypeProvideMissingInfo = "provide_missing_info"
)

func IsSupportedResolutionType(value string) bool {
	switch value {
	case ResolutionTypeReplan,
		ResolutionTypeRejectCurrentPath,
		ResolutionTypeProvideMissingInfo:
		return true
	default:
		return false
	}
}

// ResolutionActionConflictError is the actionable, ticket-scoped result for a
// supported resolution intent that is not allowed by the ticket's persisted
// recovery binding. It deliberately unwraps to ErrConflict so transport
// adapters can map it to HTTP 409 without parsing Error().
type ResolutionActionConflictError struct {
	TicketID       basecommon.TicketID
	RequestedType  string
	AllowedActions []string
	Remediation    string
}

func NewResolutionActionConflict(ticketID basecommon.TicketID, requestedType string, allowedActions []string) *ResolutionActionConflictError {
	allowed := make([]string, 0, len(allowedActions))
	for _, action := range allowedActions {
		if action = strings.TrimSpace(action); action != "" {
			allowed = append(allowed, action)
		}
	}
	remediation := "choose one of the ticket's allowed recovery actions"
	if len(allowed) == 0 {
		remediation = "refresh the ticket because it has no allowed recovery actions"
	}
	return &ResolutionActionConflictError{
		TicketID:       ticketID,
		RequestedType:  strings.TrimSpace(requestedType),
		AllowedActions: allowed,
		Remediation:    remediation,
	}
}

func (e *ResolutionActionConflictError) Error() string {
	if e == nil {
		return basecommon.ErrConflict.Error()
	}
	return fmt.Sprintf("%s: resolution type %q is not allowed for ticket %s; %s", basecommon.ErrConflict, e.RequestedType, e.TicketID, e.Remediation)
}

func (e *ResolutionActionConflictError) Unwrap() error {
	return basecommon.ErrConflict
}
