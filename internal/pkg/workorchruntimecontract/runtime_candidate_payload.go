package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCandidateDocumentDisposition classifies the provider response before
// any candidate attempt is allocated.  It is deliberately a pure payload
// boundary type: deciding whether to continue, retry transport or create an
// attempt remains the Orchestrator's responsibility.
type RuntimeCandidateDocumentDisposition string

const (
	RuntimeCandidateDocumentComplete   RuntimeCandidateDocumentDisposition = "complete_document"
	RuntimeCandidateDocumentIncomplete RuntimeCandidateDocumentDisposition = "incomplete_document"
	RuntimeCandidateDocumentEmpty      RuntimeCandidateDocumentDisposition = "empty_document"
	// RuntimeCandidateDocumentInvalid is a non-JSON provider body. It is a
	// provider-output protocol outcome, not a semantic candidate rejection.
	RuntimeCandidateDocumentInvalid RuntimeCandidateDocumentDisposition = "invalid_document"
)

// RuntimeCandidatePayloadIncompleteError means the JSON decoder reached the
// end of the provider stream while decoding the first candidate document.  It
// is distinct from a complete-but-invalid document (including trailing JSON).
// Callers must not use error strings or provider usage to infer this state.
type RuntimeCandidatePayloadIncompleteError struct {
	Cause error
}

func (e *RuntimeCandidatePayloadIncompleteError) Error() string {
	if e == nil || e.Cause == nil {
		return "runtime candidate document is incomplete"
	}
	return "runtime candidate document is incomplete: " + e.Cause.Error()
}

func (e *RuntimeCandidatePayloadIncompleteError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// RuntimeCandidatePayloadEmptyError is a successful provider response with no
// candidate document. It is a provider/session outcome, never a schema retry.
type RuntimeCandidatePayloadEmptyError struct{}

func (*RuntimeCandidatePayloadEmptyError) Error() string {
	return "runtime candidate document is empty"
}

// RuntimeCandidateDocumentDispositionForError exposes the structural decoder
// classification without letting callers parse implementation-specific text.
func RuntimeCandidateDocumentDispositionForError(err error) RuntimeCandidateDocumentDisposition {
	if err == nil {
		return RuntimeCandidateDocumentComplete
	}
	var empty *RuntimeCandidatePayloadEmptyError
	if errors.As(err, &empty) {
		return RuntimeCandidateDocumentEmpty
	}
	var incomplete *RuntimeCandidatePayloadIncompleteError
	if errors.As(err, &incomplete) {
		return RuntimeCandidateDocumentIncomplete
	}
	return RuntimeCandidateDocumentComplete
}

// InspectRuntimeCandidateDocumentFraming classifies provider bytes without
// decoding a Runtime candidate. A syntactically complete JSON value remains a
// complete document even when its schema is invalid: SM-3 must persist and
// adjudicate it. A non-JSON body is a provider protocol outcome; only a first
// JSON value cut off at EOF is eligible for output continuation.
func InspectRuntimeCandidateDocumentFraming(payload []byte) RuntimeCandidateDocumentDisposition {
	if len(bytes.TrimSpace(payload)) == 0 {
		return RuntimeCandidateDocumentEmpty
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var value json.RawMessage
	if err := decoder.Decode(&value); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return RuntimeCandidateDocumentIncomplete
		}
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			return RuntimeCandidateDocumentInvalid
		}
		return RuntimeCandidateDocumentComplete
	}
	// Once the first value is complete, trailing bytes are a completed
	// document defect for the SM-3 evaluator, never an output continuation.
	return RuntimeCandidateDocumentComplete
}

// NormalizeRuntimeCandidatePayloadDocument removes only one complete, outer
// Markdown json fence from a provider document. It is a wire normalization,
// not a permissive decoder: prose before or after the fence, a different
// language tag, a missing closing fence, an empty fenced body, multiple JSON
// values, unknown fields, and every schema violation remain for the strict
// decoder to reject. The raw provider bytes must remain the persisted audit
// artifact; callers use the returned bytes solely for candidate decoding.
func NormalizeRuntimeCandidatePayloadDocument(payload []byte) ([]byte, bool) {
	raw := bytes.TrimSpace(payload)
	openingEnd := bytes.IndexByte(raw, '\n')
	if openingEnd <= 0 || !bytes.Equal(bytes.TrimSpace(raw[:openingEnd]), []byte("```json")) {
		return payload, false
	}
	remainder := raw[openingEnd+1:]
	closingStart := bytes.LastIndex(remainder, []byte("\n```"))
	if closingStart < 0 || !bytes.Equal(bytes.TrimSpace(remainder[closingStart+1:]), []byte("```")) {
		return payload, false
	}
	document := bytes.TrimSpace(remainder[:closingStart])
	if len(document) == 0 || bytes.Contains(document, []byte("\n```")) {
		return payload, false
	}
	return append([]byte(nil), document...), true
}

// DecodeRuntimeCandidatePayload is the owner-neutral, strict DTO decoder for
// a completed provider candidate document. It performs no state transition or
// admission decision; Orchestrator remains the sole authority for both.
func DecodeRuntimeCandidatePayload(state ConvergenceState, payload []byte) (RuntimeCandidate, error) {
	switch state {
	case ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation:
		var value runtimeCandidateOperationPayload
		if err := strictDecodeRuntimeCandidatePayload(payload, &value); err != nil {
			return RuntimeCandidate{}, err
		}
		operations := make([]RuntimeOperation, 0, len(value.Operations))
		for _, op := range value.Operations {
			params := make([]OperationParameter, 0, len(op.Parameters))
			for _, param := range op.Parameters {
				params = append(params, OperationParameter{Name: param.Name, String: param.String, Integer: param.Integer, Boolean: param.Boolean, StringList: param.StringList})
			}
			operations = append(operations, RuntimeOperation{ID: op.ID, Kind: op.Kind, RequiredOperationRef: op.RequiredOperationRef, DependsOn: op.DependsOn, BindingRefs: op.BindingRefs, TargetRefs: op.TargetRefs, Parameters: params})
		}
		if state == ConvergenceStateNeedStateChange {
			ret := make([]StateChangingOperation, 0, len(operations))
			for _, op := range operations {
				ret = append(ret, StateChangingOperation{Operation: op})
			}
			return RuntimeCandidate{StateChange: &StateChangeCandidate{Operations: ret}}, nil
		}
		ret := make([]ValidationOperation, 0, len(operations))
		for _, op := range operations {
			ret = append(ret, ValidationOperation{Operation: op})
		}
		return RuntimeCandidate{Validation: &ValidationCandidate{Operations: ret}}, nil
	case ConvergenceStateNeedAssessment:
		var value runtimeCandidateAssessmentPayload
		if err := strictDecodeRuntimeCandidatePayload(payload, &value); err != nil {
			return RuntimeCandidate{}, err
		}
		items := make([]RuntimeAssessmentItemFact, 0, len(value.Items))
		for _, item := range value.Items {
			items = append(items, RuntimeAssessmentItemFact{CoverageWorkItemID: item.CoverageWorkItemID, Satisfied: item.Satisfied, Reason: item.Reason, EvidenceRefs: append([]string(nil), item.EvidenceRefs...)})
		}
		var gap *RuntimeAssessmentGap
		if value.Gap != nil {
			gap = &RuntimeAssessmentGap{CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), value.Gap.CoverageWorkItemIDs...), MissingEvidenceKinds: append([]string(nil), value.Gap.MissingEvidenceKinds...), TargetRefs: append([]string(nil), value.Gap.TargetRefs...), EvidenceRefs: append([]string(nil), value.Gap.EvidenceRefs...), Reason: value.Gap.Reason, FailureClass: FailureClass(value.Gap.FailureClass), FailureCode: value.Gap.FailureCode, FailureSignature: value.Gap.FailureSignature}
		}
		return RuntimeCandidate{Assessment: &AssessmentCandidate{FocusID: value.FocusID, SliceID: value.SliceID, CoverageWorkItemIDs: value.CoverageWorkItemIDs, Disposition: AssessmentDisposition(value.Disposition), Items: items, Gap: gap, Summary: value.Summary, EvidenceRefs: value.EvidenceRefs, ArtifactRefs: value.ArtifactRefs}}, nil
	case ConvergenceStateNeedGovernance:
		var value runtimeCandidateGovernancePayload
		if err := strictDecodeRuntimeCandidatePayload(payload, &value); err != nil {
			return RuntimeCandidate{}, err
		}
		return RuntimeCandidate{Governance: &GovernanceCandidate{ExternalDependency: ExternalDependency{Kind: value.ExternalDependency.Kind, SubjectRef: value.ExternalDependency.SubjectRef, Reason: value.ExternalDependency.Reason}}}, nil
	default:
		return RuntimeCandidate{}, fmt.Errorf("convergence state %s does not accept an LLM candidate", state)
	}
}

// EncodeRuntimeCandidatePayloadWithManifest is the pure canonical wire encoder
// used by deterministic fixtures. It deliberately accepts the sealed manifest
// rather than a derived action so a fixture cannot produce JSON that the active
// renderer/decoder contract would reject. Production never uses this helper to
// reinterpret provider output.
func EncodeRuntimeCandidatePayloadWithManifest(manifest RuntimeCandidateContractManifest, candidate RuntimeCandidate) ([]byte, error) {
	manifest = manifest.Normalized()
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
		return nil, fmt.Errorf("runtime candidate contract manifest is invalid")
	}
	if err := ValidateRuntimeCandidateAgainstManifest(manifest, candidate); err != nil {
		return nil, err
	}
	var payload any
	switch manifest.State {
	case ConvergenceStateNeedStateChange:
		if candidate.StateChange == nil {
			return nil, fmt.Errorf("state-change candidate is missing its wire payload")
		}
		payload = runtimeCandidateOperationPayloadForCandidate(candidate.StateChange.Operations, nil)
	case ConvergenceStateNeedValidation:
		if candidate.Validation == nil {
			return nil, fmt.Errorf("validation candidate is missing its wire payload")
		}
		payload = runtimeCandidateOperationPayloadForCandidate(nil, candidate.Validation.Operations)
	case ConvergenceStateNeedAssessment:
		if candidate.Assessment == nil {
			return nil, fmt.Errorf("assessment candidate is missing its wire payload")
		}
		payload = runtimeCandidateAssessmentPayloadForCandidate(*candidate.Assessment)
	case ConvergenceStateNeedGovernance:
		if candidate.Governance == nil {
			return nil, fmt.Errorf("governance candidate is missing its wire payload")
		}
		payload = runtimeCandidateGovernancePayload{ExternalDependency: runtimeCandidateExternalDependency{
			Kind: candidate.Governance.ExternalDependency.Kind, SubjectRef: candidate.Governance.ExternalDependency.SubjectRef,
			Reason: candidate.Governance.ExternalDependency.Reason,
		}}
	default:
		return nil, fmt.Errorf("convergence state %s does not accept an LLM candidate", manifest.State)
	}
	document, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	decoded, err := DecodeRuntimeCandidatePayloadWithManifest(manifest, document)
	if err != nil {
		return nil, err
	}
	if StableRuntimeCandidateHash(decoded) != StableRuntimeCandidateHash(candidate) {
		return nil, fmt.Errorf("canonical candidate payload changed candidate semantics")
	}
	return document, nil
}

func runtimeCandidateOperationPayloadForCandidate(stateChanges []StateChangingOperation, validations []ValidationOperation) runtimeCandidateOperationPayload {
	operations := make([]RuntimeOperation, 0, len(stateChanges)+len(validations))
	for _, operation := range stateChanges {
		operations = append(operations, operation.Operation)
	}
	for _, operation := range validations {
		operations = append(operations, operation.Operation)
	}
	payload := runtimeCandidateOperationPayload{Operations: make([]runtimeCandidateOperation, 0, len(operations))}
	for _, operation := range operations {
		value := runtimeCandidateOperation{ID: operation.ID, Kind: operation.Kind, RequiredOperationRef: operation.RequiredOperationRef,
			DependsOn: append([]string(nil), operation.DependsOn...), BindingRefs: append([]string(nil), operation.BindingRefs...),
			TargetRefs: append([]string(nil), operation.TargetRefs...), Parameters: make([]runtimeCandidateParameter, 0, len(operation.Parameters))}
		for _, parameter := range operation.Parameters {
			value.Parameters = append(value.Parameters, runtimeCandidateParameter{Name: parameter.Name, String: parameter.String, Integer: parameter.Integer, Boolean: parameter.Boolean, StringList: append([]string(nil), parameter.StringList...)})
		}
		payload.Operations = append(payload.Operations, value)
	}
	return payload
}

func runtimeCandidateAssessmentPayloadForCandidate(assessment AssessmentCandidate) runtimeCandidateAssessmentPayload {
	value := runtimeCandidateAssessmentPayload{FocusID: assessment.FocusID, SliceID: assessment.SliceID,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), assessment.CoverageWorkItemIDs...), Disposition: string(assessment.Disposition),
		Summary: assessment.Summary, EvidenceRefs: append([]string(nil), assessment.EvidenceRefs...), ArtifactRefs: append([]string(nil), assessment.ArtifactRefs...)}
	for _, item := range assessment.Items {
		value.Items = append(value.Items, runtimeCandidateAssessmentItem{CoverageWorkItemID: item.CoverageWorkItemID, Satisfied: item.Satisfied, Reason: item.Reason, EvidenceRefs: append([]string(nil), item.EvidenceRefs...)})
	}
	if assessment.Gap != nil {
		gap := assessment.Gap
		value.Gap = &runtimeCandidateAssessmentGap{CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), gap.CoverageWorkItemIDs...), MissingEvidenceKinds: append([]string(nil), gap.MissingEvidenceKinds...), TargetRefs: append([]string(nil), gap.TargetRefs...), EvidenceRefs: append([]string(nil), gap.EvidenceRefs...), Reason: gap.Reason, FailureClass: string(gap.FailureClass), FailureCode: gap.FailureCode, FailureSignature: gap.FailureSignature}
	}
	return value
}

func strictDecodeRuntimeCandidatePayload(payload []byte, target any) error {
	if len(bytes.TrimSpace(payload)) == 0 {
		return &RuntimeCandidatePayloadEmptyError{}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return &RuntimeCandidatePayloadIncompleteError{Cause: err}
		}
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("candidate payload contains multiple JSON values")
		}
		return err
	}
	return nil
}

type runtimeCandidateOperationPayload struct {
	Operations []runtimeCandidateOperation `json:"operations"`
}
type runtimeCandidateOperation struct {
	ID                   string                      `json:"id"`
	Kind                 string                      `json:"kind"`
	RequiredOperationRef string                      `json:"requiredOperationRef,omitempty"`
	DependsOn            []string                    `json:"dependsOn,omitempty"`
	BindingRefs          []string                    `json:"bindingRefs,omitempty"`
	TargetRefs           []string                    `json:"targetRefs,omitempty"`
	Parameters           []runtimeCandidateParameter `json:"parameters,omitempty"`
}
type runtimeCandidateParameter struct {
	Name       string   `json:"name"`
	String     *string  `json:"string,omitempty"`
	Integer    *int64   `json:"integer,omitempty"`
	Boolean    *bool    `json:"boolean,omitempty"`
	StringList []string `json:"stringList,omitempty"`
}
type runtimeCandidateGovernancePayload struct {
	ExternalDependency runtimeCandidateExternalDependency `json:"externalDependency"`
}
type runtimeCandidateExternalDependency struct {
	Kind       string `json:"kind"`
	SubjectRef string `json:"subjectRef,omitempty"`
	Reason     string `json:"reason"`
}
type runtimeCandidateAssessmentPayload struct {
	FocusID             string                           `json:"focusId"`
	SliceID             string                           `json:"sliceId,omitempty"`
	CoverageWorkItemIDs []common.CoverageWorkItemID      `json:"coverageWorkItemIds"`
	Disposition         string                           `json:"disposition,omitempty"`
	Items               []runtimeCandidateAssessmentItem `json:"items,omitempty"`
	Gap                 *runtimeCandidateAssessmentGap   `json:"gap,omitempty"`
	Summary             string                           `json:"summary"`
	EvidenceRefs        []string                         `json:"evidenceRefs,omitempty"`
	ArtifactRefs        []string                         `json:"artifactRefs,omitempty"`
}
type runtimeCandidateAssessmentItem struct {
	CoverageWorkItemID common.CoverageWorkItemID `json:"coverageWorkItemId"`
	Satisfied          bool                      `json:"satisfied"`
	Reason             string                    `json:"reason,omitempty"`
	EvidenceRefs       []string                  `json:"evidenceRefs,omitempty"`
}
type runtimeCandidateAssessmentGap struct {
	CoverageWorkItemIDs  []common.CoverageWorkItemID `json:"coverageWorkItemIds"`
	MissingEvidenceKinds []string                    `json:"missingEvidenceKinds,omitempty"`
	TargetRefs           []string                    `json:"targetRefs,omitempty"`
	EvidenceRefs         []string                    `json:"evidenceRefs,omitempty"`
	Reason               string                      `json:"reason,omitempty"`
	FailureClass         string                      `json:"failureClass,omitempty"`
	FailureCode          string                      `json:"failureCode,omitempty"`
	FailureSignature     string                      `json:"failureSignature,omitempty"`
}
