package biz

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// 交付错误没有可信 provider outcome，保留原 reserve 为未决；不能补造 provider_failed。
func (b *Biz) completeAuthoringDocument(ctx context.Context, permit string, request llmevents.CompleteProviderCommand) (llmevents.CompleteProviderDocumentResult, error) {
	command := llmevents.CompleteProviderDocumentCommand{RequestIdentity: permit, Target: request.Target, Request: request.Request, ExpectedConfigurationRevision: request.ExpectedConfigurationRevision}.Clone()
	if err := command.Validate(); err != nil {
		return llmevents.CompleteProviderDocumentResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return llmevents.CompleteProviderDocumentResult{}, err
	}
	ev := magicevent.NewEventWithContext(llmevents.TopicLLMCommandCompleteProviderDocument, ModuleID, llmevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicLLMCommandCompleteProviderDocument, eventcontract.LaneScope{WorkspaceID: command.Request.WorkspaceID, SubjectType: "provider_document"}))
	result := b.SendEvent(ev)
	if result == nil {
		return llmevents.CompleteProviderDocumentResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return llmevents.CompleteProviderDocumentResult{}, err
	}
	value, resultErr := result.Get()
	if resultErr != nil {
		return llmevents.CompleteProviderDocumentResult{}, resultErr
	}
	response, ok := value.(llmevents.CompleteProviderDocumentResult)
	if !ok || response.ValidateFor(command) != nil {
		return llmevents.CompleteProviderDocumentResult{}, common.ErrConflict
	}
	return response.Clone(), nil
}

func authoringProviderDiagnostic(response llmevents.CompleteProviderDocumentResult) *workspace.DefinitionAuthoringDiagnostic {
	var cause []byte
	code := ""
	if response.Failure != nil {
		cause, code = response.Failure.Cause, response.Failure.Code
	}
	if response.Incomplete != nil {
		cause, code = response.Incomplete.Cause, response.Incomplete.Code
	}
	if response.Document != nil && !response.Document.Content.Saved {
		cause, code = []byte("complete provider document exceeds supported diagnostic transport storage boundary"), "document_not_saved"
	}
	if code == "" {
		return nil
	}
	diagnostic := workspace.NewDefinitionAuthoringDiagnostic(string(cause), nil)
	document := authoringProviderObservation(response.ObservedDocument)
	wire := authoringProviderObservation(response.Wire)
	diagnostic.ProviderCode, diagnostic.ProviderDocument, diagnostic.ProviderWire = code, &document, &wire
	if response.TransportCause != nil {
		cause := authoringProviderObservation(*response.TransportCause)
		diagnostic.ProviderCause = &cause
	}
	return &diagnostic
}

func authoringParserDiagnostic(parseErr error, payload []byte, response llmevents.CompleteProviderDocumentResult) workspace.DefinitionAuthoringDiagnostic {
	diagnostic := workspace.NewDefinitionAuthoringDiagnostic(parseErr.Error(), payload)
	document := authoringProviderObservation(response.ObservedDocument)
	wire := authoringProviderObservation(response.Wire)
	diagnostic.ProviderDocument, diagnostic.ProviderWire = &document, &wire
	return diagnostic
}

func authoringProviderObservation(observation llmmodels.ProviderObservedContent) workspace.DefinitionProviderObservation {
	copy := observation.Clone()
	return workspace.DefinitionProviderObservation{Content: copy.Content, ObservedBytes: copy.ObservedBytes, SHA256: copy.SHA256, Observed: copy.Observed, Complete: copy.Complete, Saved: copy.Saved, Reason: copy.Reason}
}

func authoringCompletePayload(response llmevents.CompleteProviderDocumentResult) ([]byte, error) {
	if response.Document == nil || !response.Document.Content.Saved {
		return nil, fmt.Errorf("provider document is unavailable for strict parsing")
	}
	return append([]byte{}, response.Document.Content.Content...), nil
}
