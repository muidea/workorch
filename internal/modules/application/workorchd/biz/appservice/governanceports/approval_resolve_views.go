package governanceports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/governancecore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/governancesemantics"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ResolveApprovalView(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, approvalID common.ApprovalID, action string, scope common.WorkspaceID) (workorchprojectioncontract.Approval, error) {
	return ResolveApprovalViewWithCommand(ctx, hub, traces, approvalID, appmodelcore.ResolveApprovalCommand{Action: action}, scope)
}

func ResolveApprovalViewWithCommand(ctx context.Context, hub magicevent.Hub, traces governancecore.AutoGovernanceTraceReader, approvalID common.ApprovalID, cmd appmodelcore.ResolveApprovalCommand, scope common.WorkspaceID) (workorchprojectioncontract.Approval, error) {
	semantic := approvalSemanticFromCommand(cmd, "")
	action, err := governancecore.NormalizeApprovalBatchAction(semantic.Action)
	if err != nil {
		return workorchprojectioncontract.Approval{}, err
	}
	existing, err := ReadApprovalForResolution(ctx, hub, approvalID, scope)
	if err != nil {
		return workorchprojectioncontract.Approval{}, err
	}
	if err := admitApprovalMutation(ctx, hub, existing, action, scope); err != nil {
		return workorchprojectioncontract.Approval{}, err
	}
	resolutionContext := governancecore.WithApprovalSemanticPayload(cmd.Payload, semantic, approvalSemanticSource(cmd.Payload))
	item, err := ResolveApprovalItemWithContext(ctx, hub, existing, action, resolutionContext)
	if err != nil {
		return workorchprojectioncontract.Approval{}, err
	}
	return ApprovalViewForItem(ctx, traces, item, scope)
}

func ReadApprovalForResolution(ctx context.Context, hub magicevent.Hub, approvalID common.ApprovalID, scope common.WorkspaceID) (*approvalmodel.Approval, error) {
	if approvalID == "" {
		return nil, common.ErrInvalidArgument
	}
	if scope != "" {
		item, err := ReadApproval(contextWithGovernanceScope(ctx, scope), hub, approvalID)
		if err != nil {
			return nil, err
		}
		if err := governancecore.EnsureApprovalWorkspaceScope(scope, item); err != nil {
			return nil, err
		}
		return item, nil
	}
	candidates, err := ApprovalsForQuery(ctx, hub, "", "")
	if err != nil {
		return nil, err
	}
	var matches []*approvalmodel.Approval
	var pendingMatches []*approvalmodel.Approval
	for _, item := range candidates {
		if item == nil || item.ID != approvalID {
			continue
		}
		matches = append(matches, item)
		if item.State == common.ApprovalStatePending {
			pendingMatches = append(pendingMatches, item)
		}
	}
	switch {
	case len(pendingMatches) == 1:
		return pendingMatches[0], nil
	case len(pendingMatches) > 1:
		return nil, common.ErrConflict
	case len(matches) == 1:
		return matches[0], nil
	case len(matches) > 1:
		return nil, common.ErrConflict
	default:
		return nil, common.ErrNotFound
	}
}

func resolveApprovalItemByAction(ctx context.Context, hub magicevent.Hub, item *approvalmodel.Approval, action string) (*approvalmodel.Approval, error) {
	return ResolveApprovalItemWithContext(ctx, hub, item, action, workorcheventpayload.Payload{})
}

func approvalSemanticFromCommand(cmd appmodelcore.ResolveApprovalCommand, fallbackReason string) governancesemantics.ApprovalNormalization {
	rawInput := approvalPayloadString(cmd.Payload, "rawInput", "answer", "value", "input", "text", "message")
	if strings.TrimSpace(rawInput) == "" {
		rawInput = cmd.Action
	}
	return governancesemantics.NormalizeApprovalAction(cmd.Action, rawInput, fallbackReason)
}

func approvalSemanticSource(payload workorcheventpayload.Payload) string {
	source := approvalPayloadString(payload, "source", "operator", "entry", "client", "channel")
	if strings.TrimSpace(source) == "" {
		return "unknown"
	}
	return source
}

func approvalPayloadString(payload workorcheventpayload.Payload, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload.String(key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
