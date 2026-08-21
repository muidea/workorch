package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// OpenRuntimeDefinitionClarification submits one already-sealed SM-3
// clarification dispatch to the Decision owner. Deriving and persisting the
// observation/dispatch remains an Orchestrator reducer responsibility; this
// adapter only performs the concrete typed owner exchange and validates the
// exact binding returned by Decision.
func OpenRuntimeDefinitionClarification(ctx context.Context, hub magicevent.Hub, command decisionevents.OpenRuntimeDefinitionClarificationRequestCommand) (decisionevents.OpenRuntimeDefinitionClarificationRequestResult, error) {
	command = command.Normalized()
	if hub == nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, common.ErrInvalidArgument
	}
	if err := command.Validate(); err != nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := hub.Send(magicevent.NewEventWithContext(
		decisionevents.TopicOpenRuntimeDefinitionClarification,
		ModuleID,
		decisionevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		command,
	))
	if result == nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(decisionevents.OpenRuntimeDefinitionClarificationRequestResult)
	if !ok {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, fmt.Errorf("Decision owner returned an invalid runtime Definition clarification result")
	}
	if response.TicketID != command.TicketID || response.WorkspaceID != command.WorkspaceID || response.RunID != command.RunID ||
		response.AuthoritySnapshotID != command.AuthoritySnapshotID || response.ClarificationDispatchID != command.ClarificationDispatchID ||
		response.ClarificationRequestBindingHash != command.BindingHash() ||
		(response.Disposition != decisionevents.DefinitionClarificationCreated && response.Disposition != decisionevents.DefinitionClarificationDuplicate) {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, common.ErrConflict
	}
	return response, nil
}

func (m *Biz) prepareRuntimeDefinitionClarificationRequest(ctx context.Context, aggregate runtime.RuntimeAggregate) (runtime.RuntimeAggregate, error) {
	if m == nil || m.runtimeController == nil || m.EventHub() == nil || aggregate.DefinitionClarification == nil || aggregate.DefinitionClarification.Dispatch == nil {
		return runtime.RuntimeAggregate{}, common.ErrInvalidArgument
	}
	state := aggregate.DefinitionClarification.Clone()
	if state.Request != nil {
		return aggregate, nil
	}
	dispatch := state.Dispatch.Clone()
	command, err := runtimeDefinitionClarificationCommand(dispatch)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	response, err := OpenRuntimeDefinitionClarification(ctx, m.EventHub(), command)
	if err != nil {
		return runtime.RuntimeAggregate{}, err
	}
	record := runtimecontract.RuntimeDefinitionClarificationRequestRecord{
		DispatchID: dispatch.DispatchID, TicketID: response.TicketID, RequestBindingHash: response.ClarificationRequestBindingHash,
	}
	updated, err := m.submitRuntimeInternalEvent(ctx, runtime.RuntimeInternalEvent{
		Kind:        runtime.RuntimeInternalEventDefinitionClarificationRequestObserved,
		Scope:       runtime.RuntimeInternalEventScope{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID},
		Transaction: &runtime.RuntimeInternalTransactionPayload{DefinitionClarificationRequest: &record},
	})
	return updated.Aggregate, err
}

func runtimeDefinitionClarificationCommand(dispatch runtimecontract.RuntimeDefinitionClarificationDispatch) (decisionevents.OpenRuntimeDefinitionClarificationRequestCommand, error) {
	dispatch = dispatch.Clone()
	if err := dispatch.Validate(); err != nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestCommand{}, err
	}
	gapKind, err := decisionDefinitionGapKind(dispatch.GapKind)
	if err != nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestCommand{}, err
	}
	if dispatch.SchemaVersion != runtimecontract.RuntimeDefinitionClarificationSchemaCurrent || dispatch.DispatchID == "" || dispatch.AuthoritySnapshotID == "" {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestCommand{}, common.ErrInvalidArgument
	}
	command := decisionevents.OpenRuntimeDefinitionClarificationRequestCommand{
		SchemaVersion: decisionevents.DefinitionClarificationSchemaVersionCurrent,
		WorkspaceID:   dispatch.WorkspaceID, RunID: dispatch.RunID,
		DefinitionRevision: dispatch.DefinitionRevision, StartupInputHash: dispatch.StartupInputHash,
		AuthoritySnapshotID: dispatch.AuthoritySnapshotID, ClarificationDispatchID: dispatch.DispatchID,
		FocusRef: dispatch.FocusRef, LogicalRequestID: dispatch.LogicalRequestID, SemanticDocumentHash: dispatch.SemanticDocumentHash,
		GapKind: gapKind, OffendingItemRefs: append([]string(nil), dispatch.OffendingItemRefs...),
		CanonicalGapSignatureVersion: dispatch.CanonicalGapSignatureVersion,
		CanonicalGapSignature:        dispatch.CanonicalGapSignature, CanonicalGapSignatureHash: dispatch.CanonicalGapSignatureHash,
		RequiredActionKind: decisionevents.DefinitionClarificationActionReviseAndRestart,
	}
	if decisionevents.CanonicalDefinitionClarificationSignatureHash("runtime-signature", command.CanonicalGapSignatureVersion, command.CanonicalGapSignature) != command.CanonicalGapSignatureHash {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestCommand{}, fmt.Errorf("runtime Definition clarification signature algorithm mismatch")
	}
	command.TicketID = decisionevents.StableRuntimeDefinitionClarificationTicketID(command)
	if err := command.Validate(); err != nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestCommand{}, err
	}
	return command, nil
}

func decisionDefinitionGapKind(kind runtimecontract.RuntimeDefinitionGapKind) (decisionevents.RuntimeDefinitionGapKind, error) {
	switch kind {
	case runtimecontract.RuntimeDefinitionGapImplicitOneOf:
		return decisionevents.RuntimeDefinitionGapImplicitOneOf, nil
	case runtimecontract.RuntimeDefinitionGapUndeclaredPrerequisite:
		return decisionevents.RuntimeDefinitionGapUndeclaredPrerequisite, nil
	case runtimecontract.RuntimeDefinitionGapSemanticAmbiguity:
		return decisionevents.RuntimeDefinitionGapSemanticAmbiguity, nil
	default:
		return "", common.ErrInvalidArgument
	}
}
