package governanceports

import (
	"context"
	"strings"
	"unicode"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/governancesemantics"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func ResolveDecisionView(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, ticketID common.TicketID, cmd appmodelcore.ResolveDecisionCommand, scope common.WorkspaceID) (appmodelcore.Decision, error) {
	semantic := governancesemantics.NormalizeDecisionResolution(cmd.ResolutionType, cmd.ResolutionPayload)
	if !decisioncommon.IsSupportedResolutionType(semantic.ResolutionType) {
		return appmodelcore.Decision{}, common.ErrInvalidArgument
	}
	resolutionType := semantic.ResolutionType
	payload := governancecore.WithDecisionSemanticPayload(cmd.ResolutionPayload, semantic)
	ticket, err := ReadDecisionTicketForResolution(ctx, hub, ticketID, scope)
	if err != nil {
		return appmodelcore.Decision{}, err
	}
	if decisionResolutionMissingRequiredInfo(semantic, cmd.ResolutionPayload) {
		return appmodelcore.Decision{}, common.ErrInvalidArgument
	}
	item, resolution, err := resolveDecisionTicket(ctx, hub, ticket, resolutionType, payload, scope)
	if err != nil {
		return appmodelcore.Decision{}, err
	}
	autoGovernance, err := governancecore.LatestAutoGovernanceSummary(ctx, traces, item.WorkspaceID, item.RunID, "ticket", string(item.ID))
	if err != nil {
		return appmodelcore.Decision{}, err
	}
	return readmodelprojection.MapDecision(item, resolution, autoGovernance), nil
}

func decisionResolutionMissingRequiredInfo(semantic governancesemantics.DecisionNormalization, payload workorcheventpayload.Payload) bool {
	if strings.TrimSpace(semantic.ResolutionType) != decisioncommon.ResolutionTypeProvideMissingInfo {
		return false
	}
	switch strings.TrimSpace(semantic.Intent) {
	case governancesemantics.IntentRequestReplan,
		governancesemantics.IntentRequestTerminate,
		governancesemantics.IntentRejectExecution:
		return false
	}
	rawInput := strings.TrimSpace(semantic.RawInput)
	if rawInput == "" && !decisionResolutionPayloadHasExplicitRawInput(payload) {
		return false
	}
	if decisionResolutionSlashCommandOnly(rawInput) {
		return true
	}
	for _, r := range rawInput {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func decisionResolutionSlashCommandOnly(rawInput string) bool {
	fields := strings.Fields(strings.TrimSpace(rawInput))
	if len(fields) == 0 {
		return false
	}
	first := strings.ToLower(fields[0])
	if !strings.HasPrefix(first, "/") {
		return false
	}
	command := strings.TrimPrefix(first, "/")
	if len(fields) == 1 {
		switch command {
		case "", "decision", "approval", "run", "goal", "dod", "status", "coverage", "timeline", "memory", "config", "help", "exit":
			return true
		default:
			return false
		}
	}
	if len(fields) != 2 {
		return false
	}
	subcommand := strings.ToLower(fields[1])
	switch command {
	case "decision":
		switch subcommand {
		case "answer", "resolve", "replan", "terminate", "reject", "show", "list":
			return true
		}
	case "approval":
		switch subcommand {
		case "approve", "reject", "show", "list":
			return true
		}
	case "run":
		switch subcommand {
		case "resume", "restart":
			return true
		}
	case "goal", "dod", "workspace":
		return true
	}
	return false
}

func decisionResolutionPayloadHasExplicitRawInput(payload workorcheventpayload.Payload) bool {
	for _, key := range []string{"answer", "value", "input", "note", "text", "message"} {
		if _, ok := payload.Field(key); ok {
			return true
		}
	}
	return false
}

func ReadDecisionTicketForResolution(ctx context.Context, hub magicevent.Hub, ticketID common.TicketID, scope common.WorkspaceID) (*decisionmodel.Ticket, error) {
	if ticketID == "" {
		return nil, common.ErrInvalidArgument
	}
	if scope != "" {
		return ReadDecisionTicket(common.ContextWithRuntimeWorkspaceID(ctx, scope), hub, ticketID)
	}
	candidates, err := DecisionTicketsForQuery(ctx, hub, "", "")
	if err != nil {
		return nil, err
	}
	var matches []*decisionmodel.Ticket
	var openMatches []*decisionmodel.Ticket
	for _, item := range candidates {
		if item == nil || item.ID != ticketID {
			continue
		}
		matches = append(matches, item)
		if item.State == common.DecisionTicketStateOpen {
			openMatches = append(openMatches, item)
		}
	}
	switch {
	case len(openMatches) == 1:
		return openMatches[0], nil
	case len(openMatches) > 1:
		return nil, common.ErrConflict
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return nil, common.ErrConflict
	default:
		return nil, common.ErrNotFound
	}
}

func resolveDecisionTicket(ctx context.Context, hub magicevent.Hub, ticket *decisionmodel.Ticket, resolutionType string, payload workorcheventpayload.Payload, scope common.WorkspaceID) (*decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	normalizedPayload, normalizedType, err := prepareDecisionResolution(ticket, resolutionType, payload, scope)
	if err != nil {
		return nil, nil, err
	}
	return ResolveDecision(ctx, hub, ticket, normalizedType, common.RedactSensitivePayload(normalizedPayload))
}

func prepareDecisionResolution(ticket *decisionmodel.Ticket, resolutionType string, payload workorcheventpayload.Payload, scope common.WorkspaceID) (workorcheventpayload.Payload, string, error) {
	if err := governancecore.EnsureDecisionTicketWorkspaceScope(scope, ticket); err != nil {
		return workorcheventpayload.Payload{}, "", err
	}
	// State-check is delegated to the kernel: kernel.EnsureResolvedTicketAndReturn
	// performs an orphan-resolution recovery (accepts matching payload on an
	// already-resolved ticket idempotently) and returns ErrConflict for state
	// mismatches. Re-checking state here would block the idempotent path before
	// the kernel can do its orphan check. The batch path's pre-validation loop
	// no longer short-circuits on state=resolved, so the per-ticket kernel call
	// is the single source of truth for conflict detection.
	normalizedPayload, normalizedType := governancecore.NormalizeDecisionResolutionPayloadForTicket(ticket, payload, resolutionType)
	if err := governancecore.ValidateDecisionResolutionTypeForTicket(ticket, normalizedType); err != nil {
		return workorcheventpayload.Payload{}, "", err
	}
	if err := governancecore.ValidateDecisionResolutionPayloadForTicket(ticket, normalizedPayload); err != nil {
		return workorcheventpayload.Payload{}, "", err
	}
	return normalizedPayload, normalizedType, nil
}
