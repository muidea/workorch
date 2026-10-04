package llm

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) handleProviderDocument(ev magicevent.Event, result magicevent.Result) {
	command, ok := ev.Data().(llmevents.CompleteProviderDocumentCommand)
	if !ok || command.Validate() != nil {
		setLLMMagicError(result, common.ErrInvalidArgument)
		return
	}
	command = command.Clone()
	requestHash, err := command.RequestHash()
	if err != nil {
		setLLMMagicError(result, err)
		return
	}
	request, err := command.Clone().Request.ToModel()
	if err != nil {
		setLLMMagicError(result, err)
		return
	}
	request.CaptureDocumentObservation = true
	started := time.Now().UTC()
	completion, metadata, callErr := m.completeProvider(ev.Context(), command.Target, request, command.ExpectedConfigurationRevision)
	audit := providerCommandAudit(request, metadata, completion.RequestBytes, completion.ResponseBytes, started, time.Now().UTC(), callErr)
	applyProviderUsageToAudit(audit, completion.Usage)
	status, failureClass := providerCommandAuditOutcome(callErr, metadata.Submitted)
	auditErr := m.publishLLMUsageRecorded(ev.Context(), audit, completion.Usage, status, failureClass)
	response := llmevents.CompleteProviderDocumentResult{
		RequestIdentity: command.RequestIdentity, RequestHash: requestHash, Target: metadata.Target,
		ConfigurationRevision: command.ExpectedConfigurationRevision, Usage: cloneProviderUsage(completion.Usage),
		ProviderSubmitted: completion.ProviderSubmitted, PhysicalResponseMode: completion.PhysicalResponseMode,
		Wire:             llmmodels.ProviderObservedContent{Reason: "response_body_not_observed"},
		ObservedDocument: llmmodels.ProviderObservedContent{Reason: "document_not_observed"},
	}
	if completion.WireObservation != nil {
		response.Wire = completion.WireObservation.Clone()
	}
	if completion.DocumentObservation != nil {
		response.ObservedDocument = completion.DocumentObservation.Clone()
	}
	if callErr == nil {
		if completion.DocumentObservation == nil || !completion.DocumentObservation.Complete {
			setLLMMagicError(result, fmt.Errorf("%w: provider omitted complete document observation", common.ErrConflict))
			return
		}
		response.Document = &llmevents.ProviderCompleteDocument{Content: response.ObservedDocument.Clone()}
	} else {
		code, incomplete, known := providerDocumentFailureCode(callErr)
		if !known || !completion.ProviderSubmitted {
			setLLMMagicError(result, callErr)
			return
		}
		var transport *ProviderTransportError
		if errors.As(callErr, &transport) {
			cause := transport.diagnosticCause
			if cause == nil {
				cause = transport.cause
			}
			if cause != nil {
				observed := llmmodels.NewProviderObservedContent([]byte(cause.Error()), true, "")
				response.TransportCause = &observed
			}
		}
		if incomplete {
			response.Incomplete = &llmevents.ProviderIncompleteDocument{Code: code, Cause: []byte(callErr.Error())}
		} else {
			response.Failure = &llmevents.ProviderDocumentFailure{Code: code, Cause: []byte(callErr.Error())}
		}
	}
	if auditErr != nil {
		// 审计展示不决定调用事实；失败分类不携带原始审计错误。
		response.AuditPublicationFailureClass = "audit_publication_failed"
	}
	if err := response.ValidateFor(command); err != nil {
		setLLMMagicError(result, err)
		return
	}
	result.Set(response.Clone(), nil)
}

func providerDocumentFailureCode(err error) (code string, incomplete, known bool) {
	var outputLimit *ProviderOutputLimitError
	if errors.As(err, &outputLimit) {
		return "output_limit", true, true
	}
	var protocol *ProviderProtocolError
	if errors.As(err, &protocol) {
		return protocol.Code, protocol.Code == "stream_ended_before_completion", true
	}
	var status *ProviderHTTPStatusError
	if errors.As(err, &status) {
		return "http_" + strconv.Itoa(status.StatusCode), false, true
	}
	var transport *ProviderTransportError
	if errors.As(err, &transport) {
		return transport.Code, false, true
	}
	return "", false, false
}
