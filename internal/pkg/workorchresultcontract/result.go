// Package workorchresultcontract contains immutable, owner-neutral values for
// already-judged Result assessments. It defines no Result topic, command,
// persistence behavior, assessment policy, or Runtime state transition.
package workorchresultcontract

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type ResultSummary struct {
	GoalOutcome           string                       `json:"goalOutcome,omitempty"`
	DoDItems              []DoDAssessmentItem          `json:"doDItems,omitempty"`
	ImplementationSummary string                       `json:"implementationSummary,omitempty"`
	VerificationSummary   string                       `json:"verificationSummary,omitempty"`
	EvidenceRefs          []string                     `json:"evidenceRefs,omitempty"`
	ArtifactRefs          []common.ArtifactID          `json:"artifactRefs,omitempty"`
	KnownLimitations      []string                     `json:"knownLimitations,omitempty"`
	ResidualRisks         []string                     `json:"residualRisks,omitempty"`
	NextActions           []string                     `json:"nextActions,omitempty"`
	Diagnostics           workorcheventpayload.Payload `json:"diagnostics,omitzero"`
}

type DoDAssessment struct {
	Satisfied            bool                `json:"satisfied"`
	Items                []DoDAssessmentItem `json:"items,omitempty"`
	GoalCoverage         *DoDCoverage        `json:"goalCoverage,omitempty"`
	AcceptanceDimensions *DoDCoverage        `json:"acceptanceDimensions,omitempty"`
	DeliverableCoverage  *DoDCoverage        `json:"deliverableCoverage,omitempty"`
	Summary              string              `json:"summary,omitempty"`
}

type DoDAssessmentItem struct {
	Item          string   `json:"item"`
	State         string   `json:"state"`
	EvidenceRefs  []string `json:"evidenceRefs,omitempty"`
	FailureReason string   `json:"failureReason,omitempty"`
	Confidence    float64  `json:"confidence,omitempty"`
	SourceNote    string   `json:"sourceNote,omitempty"`
}

type DoDCoverage struct {
	Satisfied bool              `json:"satisfied"`
	Items     []DoDCoverageItem `json:"items,omitempty"`
	Summary   string            `json:"summary,omitempty"`
}

type DoDCoverageItem struct {
	ID           string              `json:"id,omitempty"`
	Deliverable  string              `json:"deliverable,omitempty"`
	Name         string              `json:"name,omitempty"`
	Path         string              `json:"path,omitempty"`
	Goal         string              `json:"goal,omitempty"`
	GoalItem     string              `json:"goalItem,omitempty"`
	Item         string              `json:"item,omitempty"`
	Requirement  string              `json:"requirement,omitempty"`
	State        string              `json:"state"`
	EvidenceRefs []string            `json:"evidenceRefs,omitempty"`
	ArtifactRefs []common.ArtifactID `json:"artifactRefs,omitempty"`
}

func (item ResultSummary) Clone() ResultSummary {
	item.DoDItems = append([]DoDAssessmentItem(nil), item.DoDItems...)
	for idx := range item.DoDItems {
		item.DoDItems[idx].EvidenceRefs = append([]string(nil), item.DoDItems[idx].EvidenceRefs...)
	}
	item.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	item.ArtifactRefs = append([]common.ArtifactID(nil), item.ArtifactRefs...)
	item.KnownLimitations = append([]string(nil), item.KnownLimitations...)
	item.ResidualRisks = append([]string(nil), item.ResidualRisks...)
	item.NextActions = append([]string(nil), item.NextActions...)
	item.Diagnostics = item.Diagnostics.Clone()
	return item
}
