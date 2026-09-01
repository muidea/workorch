package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// RuntimeCandidateDocumentDisposition is the LLM owner's pure classification
// of raw provider bytes before any candidate attempt is allocated. Shared
// placement does not authorize a downstream consumer to inspect or reclassify
// those bytes. Deciding whether to recover, terminate, or create an attempt
// remains the Orchestrator's responsibility.
type RuntimeCandidateDocumentDisposition string

const (
	RuntimeCandidateDocumentComplete   RuntimeCandidateDocumentDisposition = "complete_document"
	RuntimeCandidateDocumentIncomplete RuntimeCandidateDocumentDisposition = "incomplete_document"
	RuntimeCandidateDocumentEmpty      RuntimeCandidateDocumentDisposition = "empty_document"
	// RuntimeCandidateDocumentInvalid is a coherent non-JSON provider body. It
	// is a document-conformance observation, not a provider protocol failure;
	// only the Orchestrator may bind it to the bounded SM-3 attempt policy.
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
// candidate document. It is only a factual document-shape classification;
// callers do not gain retry authority from this error by itself.
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
// adjudicate it. A coherent non-JSON body is a document-conformance
// observation, not a provider-protocol failure; only a first JSON value cut
// off at EOF is eligible for whole-document recovery.
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
	// document defect for the SM-3 evaluator, never request recovery.
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
			operations = append(operations, RuntimeOperation{Kind: op.Kind, RequiredOperationRef: op.RequiredOperationRef, BindingRefs: op.BindingRefs, TargetRefs: op.TargetRefs, Parameters: params})
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
		return RuntimeCandidate{Governance: &GovernanceCandidate{ExternalDependency: ExternalDependency{
			Kind: value.ExternalDependency.Kind, SubjectRef: value.ExternalDependency.SubjectRef, Reason: value.ExternalDependency.Reason,
			RequestedAction: value.ExternalDependency.RequestedAction, RiskLevel: value.ExternalDependency.RiskLevel,
			AuthorizationContext: value.ExternalDependency.AuthorizationContext.Clone(),
		}}}, nil
	default:
		return RuntimeCandidate{}, fmt.Errorf("convergence state %s does not accept an LLM candidate", state)
	}
}

// EncodeRuntimeCandidatePayloadWithManifest is the pure canonical wire encoder
// used by deterministic fixtures. It deliberately accepts the sealed manifest
// rather than a derived action so a fixture cannot produce JSON that the active
// renderer/decoder contract would reject. Operation ID and dependency values
// on the input are replaced by the manifest-derived authority and never enter
// the wire document. Production never uses this helper to reinterpret provider
// output.
func EncodeRuntimeCandidatePayloadWithManifest(manifest RuntimeCandidateContractManifest, candidate RuntimeCandidate) ([]byte, error) {
	manifest = manifest.Normalized()
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
		return nil, fmt.Errorf("runtime candidate contract manifest is invalid")
	}
	canonicalCandidate := candidate
	if manifest.State == ConvergenceStateNeedStateChange || manifest.State == ConvergenceStateNeedValidation {
		var err error
		canonicalCandidate, err = DeriveRuntimeCandidateOperationAuthority(manifest, candidate)
		if err != nil {
			return nil, err
		}
	}
	if err := ValidateRuntimeCandidateAgainstManifest(manifest, canonicalCandidate); err != nil {
		return nil, err
	}
	var (
		payload any
		err     error
	)
	switch manifest.State {
	case ConvergenceStateNeedStateChange:
		if canonicalCandidate.StateChange == nil {
			return nil, fmt.Errorf("state-change candidate is missing its wire payload")
		}
		payload, err = runtimeCandidateProviderWirePayloadForCandidate(manifest, canonicalCandidate.StateChange.Operations, nil)
		if err != nil {
			return nil, err
		}
	case ConvergenceStateNeedValidation:
		if canonicalCandidate.Validation == nil {
			return nil, fmt.Errorf("validation candidate is missing its wire payload")
		}
		payload, err = runtimeCandidateProviderWirePayloadForCandidate(manifest, nil, canonicalCandidate.Validation.Operations)
		if err != nil {
			return nil, err
		}
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
			Reason: candidate.Governance.ExternalDependency.Reason, RequestedAction: candidate.Governance.ExternalDependency.RequestedAction,
			RiskLevel:            candidate.Governance.ExternalDependency.RiskLevel,
			AuthorizationContext: candidate.Governance.ExternalDependency.AuthorizationContext.Clone(),
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
	if StableRuntimeCandidateHash(decoded) != StableRuntimeCandidateHash(canonicalCandidate) {
		return nil, fmt.Errorf("canonical candidate payload changed candidate semantics")
	}
	return document, nil
}

// runtimeCandidateProviderWireOperation keeps parameters last in the canonical
// fixture and prompt example layout. JSON object order is not semantic, but
// this stable render prevents the schema map's alphabetical order from
// becoming accidental model guidance.
type runtimeCandidateProviderWireOperation struct {
	RequiredOperationRef string                      `json:"requiredOperationRef,omitempty"`
	Kind                 string                      `json:"kind,omitempty"`
	BindingRefs          []string                    `json:"bindingRefs,omitempty"`
	TargetRefs           []string                    `json:"targetRefs,omitempty"`
	Parameters           []runtimeCandidateParameter `json:"parameters,omitempty"`
}

type runtimeCandidateProviderWirePayload struct {
	Operations []runtimeCandidateProviderWireOperation `json:"operations"`
}

func runtimeCandidateProviderWirePayloadForCandidate(manifest RuntimeCandidateContractManifest, stateChanges []StateChangingOperation, validations []ValidationOperation) (runtimeCandidateProviderWirePayload, error) {
	wire, err := BuildRuntimeCandidateWireContract(manifest)
	if err != nil {
		return runtimeCandidateProviderWirePayload{}, err
	}
	operations := make([]RuntimeOperation, 0, len(stateChanges)+len(validations))
	for _, operation := range stateChanges {
		operations = append(operations, operation.Operation.Normalized())
	}
	for _, operation := range validations {
		operations = append(operations, operation.Operation.Normalized())
	}
	payload := runtimeCandidateProviderWirePayload{Operations: make([]runtimeCandidateProviderWireOperation, 0, len(operations))}
	for _, operation := range operations {
		var rule RuntimeCandidateWireOperationRule
		var found bool
		if operation.RequiredOperationRef != "" {
			rule, found = RuntimeCandidateWireRuleForRequiredOperation(wire, operation.RequiredOperationRef)
		} else {
			rule, found = RuntimeCandidateWireRuleForAuxiliaryKind(wire, operation.Kind)
		}
		if !found || rule.Kind != operation.Kind {
			return runtimeCandidateProviderWirePayload{}, fmt.Errorf("candidate operation has no exact provider wire rule")
		}
		value := runtimeCandidateProviderWireOperation{}
		if runtimeCandidateWireContains(rule.ProviderFields, "requiredOperationRef") {
			value.RequiredOperationRef = operation.RequiredOperationRef
		}
		if runtimeCandidateWireContains(rule.ProviderFields, "kind") {
			value.Kind = operation.Kind
		}
		if runtimeCandidateWireContains(rule.ProviderFields, "bindingRefs") {
			value.BindingRefs = append([]string(nil), operation.BindingRefs...)
		}
		if runtimeCandidateWireContains(rule.ProviderFields, "targetRefs") {
			value.TargetRefs = append([]string(nil), operation.TargetRefs...)
		}
		if runtimeCandidateWireContains(rule.ProviderFields, "parameters") {
			derived := make(map[string]struct{}, len(rule.DerivedParameters))
			for _, parameter := range rule.DerivedParameters {
				derived[parameter.Name] = struct{}{}
			}
			for _, parameter := range operation.Parameters {
				if _, fixed := derived[parameter.Name]; fixed {
					continue
				}
				value.Parameters = append(value.Parameters, runtimeCandidateWireParameterPayload(parameter))
			}
		}
		payload.Operations = append(payload.Operations, value)
	}
	return payload, nil
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
		value := runtimeCandidateOperation{Kind: operation.Kind, RequiredOperationRef: operation.RequiredOperationRef,
			BindingRefs: append([]string(nil), operation.BindingRefs...),
			TargetRefs:  append([]string(nil), operation.TargetRefs...), Parameters: make([]runtimeCandidateParameter, 0, len(operation.Parameters))}
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
	Kind                 string                      `json:"kind"`
	RequiredOperationRef string                      `json:"requiredOperationRef,omitempty"`
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
	Kind                 string                       `json:"kind"`
	SubjectRef           string                       `json:"subjectRef,omitempty"`
	Reason               string                       `json:"reason"`
	RequestedAction      string                       `json:"requestedAction,omitempty"`
	RiskLevel            string                       `json:"riskLevel,omitempty"`
	AuthorizationContext workorcheventpayload.Payload `json:"authorizationContext,omitempty"`
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
