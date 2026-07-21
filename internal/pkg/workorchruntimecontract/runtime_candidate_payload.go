package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/muidea/workorch/internal/pkg/common"
)

// DecodeRuntimeCandidatePayload is the owner-neutral, strict DTO decoder for
// a completed provider candidate document. It performs no state transition or
// admission decision; Orchestrator remains the sole authority for both.
func DecodeRuntimeCandidatePayload(action RuntimeAction, payload []byte) (RuntimeCandidate, error) {
	switch action {
	case RuntimeActionProduceStateChange, RuntimeActionProduceValidation:
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
			operations = append(operations, RuntimeOperation{ID: op.ID, Kind: op.Kind, RequiredOperationRef: op.RequiredOperationRef, DependsOn: op.DependsOn, CoverageWorkItemIDs: op.CoverageWorkItemIDs, TargetRefs: op.TargetRefs, Parameters: params})
		}
		if action == RuntimeActionProduceStateChange {
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
	case RuntimeActionProduceAssessment:
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
	case RuntimeActionRequestGovernance:
		var value runtimeCandidateGovernancePayload
		if err := strictDecodeRuntimeCandidatePayload(payload, &value); err != nil {
			return RuntimeCandidate{}, err
		}
		return RuntimeCandidate{Governance: &GovernanceCandidate{ExternalDependency: ExternalDependency{Kind: value.ExternalDependency.Kind, SubjectRef: value.ExternalDependency.SubjectRef, Reason: value.ExternalDependency.Reason}}}, nil
	default:
		return RuntimeCandidate{}, fmt.Errorf("action %s does not accept an LLM candidate", action)
	}
}

func strictDecodeRuntimeCandidatePayload(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
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
	CoverageWorkItemIDs  []common.CoverageWorkItemID `json:"coverageWorkItemIds,omitempty"`
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
