// Package workorchexecutioncontract contains the protocol-neutral
// classification and wire view of a Workspace-owner execution admission.
package workorchexecutioncontract

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	AdmissionDisplayAccepted = "accepted"
	AdmissionDisplayRejected = "rejected"
)

// AdmissionPresentation is the only classification of an immutable owner
// admission used by HTTP and read projections. Consumers display this fact;
// they must not independently reinterpret owner outcomes.
type AdmissionPresentation struct {
	Accepted       bool
	HTTPStatusCode int
	Display        string
	Diagnostic     string
}

func PresentAdmission(admission runtimecontract.RuntimeExecutionIntentAdmission) (AdmissionPresentation, error) {
	admission = admission.Normalized()
	if err := admission.Validate(); err != nil {
		return AdmissionPresentation{}, fmt.Errorf("invalid workspace execution admission: %w", err)
	}
	accepted := false
	switch admission.Outcome {
	case runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		runtimecontract.RuntimeExecutionIntentReplacementPending:
		accepted = true
	case runtimecontract.RuntimeExecutionIntentRestartRequired,
		runtimecontract.RuntimeExecutionIntentFreshActivationRequired,
		runtimecontract.RuntimeExecutionIntentDefinitionChanged,
		runtimecontract.RuntimeExecutionIntentGovernanceRequired:
	default:
		return AdmissionPresentation{}, fmt.Errorf("unsupported workspace execution admission outcome %q", admission.Outcome)
	}
	if accepted {
		return AdmissionPresentation{
			Accepted:       true,
			HTTPStatusCode: http.StatusAccepted,
			Display:        AdmissionDisplayAccepted,
			Diagnostic:     runtimecontract.RuntimeExecutionIntentReasonSummary(admission.Reason),
		}, nil
	}
	return AdmissionPresentation{
		HTTPStatusCode: http.StatusConflict,
		Display:        AdmissionDisplayRejected,
		Diagnostic:     runtimecontract.RuntimeExecutionIntentReasonSummary(admission.Reason),
	}, nil
}

// WorkspaceExecutionAdmissionResponse preserves the exact owner-authored
// identity, outcome and reason while adding one derived transport/display
// classification. RequestReason remains separate from owner Reason.
type WorkspaceExecutionAdmissionResponse struct {
	Service       string                                        `json:"service"`
	Backend       string                                        `json:"backend,omitempty"`
	DataRoot      string                                        `json:"dataRoot,omitempty"`
	Action        string                                        `json:"action,omitempty"`
	Accepted      bool                                          `json:"accepted"`
	State         string                                        `json:"state"`
	WorkspaceID   common.WorkspaceID                            `json:"workspaceId"`
	RunID         common.RunID                                  `json:"runId,omitempty"`
	RequestID     string                                        `json:"requestId"`
	Intent        runtimecontract.RuntimeExecutionIntentKind    `json:"intent"`
	Outcome       runtimecontract.RuntimeExecutionIntentOutcome `json:"outcome"`
	Reason        runtimecontract.RuntimeExecutionIntentReason  `json:"reason"`
	RequestReason string                                        `json:"requestReason,omitempty"`
	Source        string                                        `json:"source,omitempty"`
	Diagnostic    string                                        `json:"diagnostic,omitempty"`
	Error         string                                        `json:"error,omitempty"`
}

func NewWorkspaceExecutionAdmissionResponse(service, backend, dataRoot, action, requestReason, source string, admission runtimecontract.RuntimeExecutionIntentAdmission) (WorkspaceExecutionAdmissionResponse, int, error) {
	admission = admission.Normalized()
	presentation, err := PresentAdmission(admission)
	if err != nil {
		return WorkspaceExecutionAdmissionResponse{}, 0, err
	}
	response := WorkspaceExecutionAdmissionResponse{
		Service:       strings.TrimSpace(service),
		Backend:       strings.TrimSpace(backend),
		DataRoot:      strings.TrimSpace(dataRoot),
		Action:        strings.TrimSpace(action),
		Accepted:      presentation.Accepted,
		State:         presentation.Display,
		WorkspaceID:   admission.WorkspaceID,
		RunID:         admission.RunID,
		RequestID:     admission.RequestID,
		Intent:        admission.Kind,
		Outcome:       admission.Outcome,
		Reason:        admission.Reason,
		RequestReason: strings.TrimSpace(requestReason),
		Source:        strings.TrimSpace(source),
		Diagnostic:    presentation.Diagnostic,
	}
	if !presentation.Accepted {
		response.Error = presentation.Diagnostic
		if response.Error == "" {
			response.Error = string(admission.Reason)
		}
	}
	return response, presentation.HTTPStatusCode, nil
}

func (r WorkspaceExecutionAdmissionResponse) Admission() (runtimecontract.RuntimeExecutionIntentAdmission, error) {
	admission := runtimecontract.RuntimeExecutionIntentAdmission{
		WorkspaceID: r.WorkspaceID,
		RunID:       r.RunID,
		RequestID:   r.RequestID,
		Kind:        r.Intent,
		Outcome:     r.Outcome,
		Reason:      r.Reason,
	}.Normalized()
	presentation, err := PresentAdmission(admission)
	if err != nil {
		return runtimecontract.RuntimeExecutionIntentAdmission{}, err
	}
	if r.Accepted != presentation.Accepted || strings.TrimSpace(r.State) != presentation.Display {
		return runtimecontract.RuntimeExecutionIntentAdmission{}, fmt.Errorf("workspace execution admission classification does not match owner outcome")
	}
	return admission, nil
}
