package activationports

import (
	"context"
	"fmt"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ensureProviderTargetReady(ctx context.Context, hub magicevent.Hub, target runtimecontract.ProviderModelTarget) error {
	return ensureProviderTargetReadyForOperation(ctx, hub, target, "chat_completions")
}

func EnsureProviderTargetReady(ctx context.Context, hub magicevent.Hub, target runtimecontract.ProviderModelTarget) error {
	return ensureProviderTargetReady(ctx, hub, target)
}

func ensureProviderTargetReadyForOperation(ctx context.Context, hub magicevent.Hub, target runtimecontract.ProviderModelTarget, operation string) error {
	target = target.Normalized()
	if hub == nil {
		return common.ErrInvalidArgument
	}
	if err := target.Validate(); err != nil {
		return err
	}
	ev := magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandResolveProviderModelTarget,
		modulecommon.ModuleID,
		llmevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		llmevents.ResolveProviderModelTargetCommand{Target: target, Operation: operation},
	)
	ev.BindLaneKey(events.EventHubLaneKeyFor(llmevents.TopicLLMCommandResolveProviderModelTarget, events.EventHubLaneScope{
		SubjectType: "llm_target",
		SubjectID:   target.ProviderID + "/" + target.ModelID + "/" + operation,
	}))
	result := hub.Send(ev)
	if result == nil {
		return common.ErrInvalidArgument
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return resultErr.Cause
		}
		return resultErr
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return resultErr
	}
	response, ok := data.(llmevents.ResolveProviderModelTargetResult)
	if !ok {
		return fmt.Errorf("%w: resolve provider target result type %T", common.ErrInvalidArgument, data)
	}
	if err := response.Profile.Target.Validate(); err != nil {
		return err
	}
	if !response.Profile.Target.Same(target) {
		return fmt.Errorf("%w: readiness response target does not match request target", common.ErrInvalidArgument)
	}
	wantOp := strings.TrimSpace(operation)
	if wantOp == "" {
		wantOp = "chat_completions"
	}
	gotOp := strings.TrimSpace(response.Profile.Operation)
	if gotOp == "" {
		return fmt.Errorf("%w: readiness response operation is missing", common.ErrInvalidArgument)
	}
	if gotOp != wantOp {
		return fmt.Errorf("%w: readiness response operation %q does not match request operation %q", common.ErrInvalidArgument, gotOp, wantOp)
	}
	if response.Profile.CapabilityRevision == "" || response.Profile.ConfigurationRevision == "" {
		return fmt.Errorf("%w: readiness profile revisions are incomplete", common.ErrInvalidArgument)
	}
	return nil
}
