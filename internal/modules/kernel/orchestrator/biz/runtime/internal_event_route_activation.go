package runtime

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// applyActivationInternalEvent reduces the one-time activation family. The
// exported dispatcher has already classified and validated the envelope.
func (c *RuntimeController) applyActivationInternalEvent(ctx context.Context, event RuntimeInternalEvent) (RuntimeInternalEventResult, error) {
	result := RuntimeInternalEventResult{}
	switch event.Kind {
	case RuntimeInternalEventActivationObserved:
		if event.Activation.ActivationFacts == nil || event.Activation.ActivationDefinition == nil ||
			event.Activation.ActivationFacts.WorkspaceID != event.Scope.WorkspaceID ||
			event.Activation.ActivationFacts.RunID != event.Scope.RunID {
			return result, common.ErrInvalidArgument
		}
		aggregate, err := c.initialize(ctx, *event.Activation.ActivationFacts, event.Activation.ActivationDefinition.Clone(), append([]runtimecontract.RuntimeRequirement(nil), event.Activation.ActivationRequirements...))
		result.Aggregate = aggregate
		return result, err
	default:
		return unsupportedRuntimeInternalEvent(event.Kind)
	}
}
