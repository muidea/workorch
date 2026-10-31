// Package testkit 提供显式当前协议的 synthetic 创作样本，不是生产兼容解析器。
package testkit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

// Mapping 根据已封存派生输入构造当前 synthetic wire；模板、文本和来源仍由生产 compiler 反证。
func Mapping(d workspace.DefinitionDerivationRequest, verification workspace.DefinitionAuthoringVerificationMapping) (workspace.DefinitionAuthoringMapping, error) {
	if d.Proposal == nil {
		return workspace.DefinitionAuthoringMapping{}, fmt.Errorf("synthetic mapping requires sealed derivation")
	}
	items, err := d.Proposal.CandidateItems(d.Input, &d.Template)
	if err != nil {
		return workspace.DefinitionAuthoringMapping{}, err
	}
	m := workspace.DefinitionAuthoringMapping{Protocol: workspace.DefinitionAuthoringMappingProtocol, InputHash: d.Input.InputHash(), TemplateSnapshotHash: workspace.WorkspaceTemplateSnapshotHash(&d.Template), ResourceSnapshotHash: d.ResourceSnapshotHash, GoalRequirement: workspace.DefinitionAuthoringGoalMapping{DimensionRef: "all", ResourceIDs: []string{}}, Dimensions: []workspace.DefinitionAuthoringDimensionMapping{{Ref: "all", ResourceIDs: []string{}}}, Requirements: []workspace.DefinitionAuthoringRequirementMapping{}, Obligations: []workspace.DefinitionAuthoringObligationMapping{}}
	for _, item := range items {
		ref := item.CandidateItemRef
		m.Requirements = append(m.Requirements, workspace.DefinitionAuthoringRequirementMapping{Ref: ref, DimensionRef: "all", SourceItemRefs: []string{ref}, ResourceIDs: []string{}, Text: item.Text})
		m.Obligations = append(m.Obligations, workspace.DefinitionAuthoringObligationMapping{CandidateItemRef: ref, RequirementRef: ref, Verification: verification})
	}
	return m, nil
}
func Audit(m workspace.DefinitionAuthoringMapping) (workspace.DefinitionAuthoringSuccessByteAudit, error) {
	payload, err := json.Marshal(struct {
		Mapping workspace.DefinitionAuthoringMapping `json:"mapping"`
	}{m})
	if err != nil {
		return workspace.DefinitionAuthoringSuccessByteAudit{}, err
	}
	payload = OwnerBoundFixtureDocument(payload)
	sum := sha256.Sum256(payload)
	o := workspace.DefinitionProviderObservation{Content: payload, ObservedBytes: uint64(len(payload)), SHA256: hex.EncodeToString(sum[:]), Observed: true, Complete: true, Saved: true}
	return workspace.DefinitionAuthoringSuccessByteAudit{Document: o, Wire: o.Clone()}, nil
}
func Proposal(d workspace.DefinitionDerivationRequest, v workspace.DefinitionAuthoringVerificationMapping, consumers ...string) (workspace.DefinitionAuthoringMapping, workspace.DefinitionAuthoringSuccessByteAudit, workspace.GoalSpecification, error) {
	m, err := Mapping(d, v)
	if err != nil {
		return m, workspace.DefinitionAuthoringSuccessByteAudit{}, workspace.GoalSpecification{}, err
	}
	if len(consumers) > 0 {
		m, err = SelectProfileAcceptance(d, m, consumers[0])
		if err != nil {
			return m, workspace.DefinitionAuthoringSuccessByteAudit{}, workspace.GoalSpecification{}, err
		}
	}
	a, err := Audit(m)
	if err != nil {
		return m, a, workspace.GoalSpecification{}, err
	}
	s, err := workspace.CompileDefinitionAuthoringMapping(d, m)
	return m, a, s, err
}

// SelectProfileAcceptance is an explicit synthetic provider choice supplied by
// the scenario, never a production default or a first-candidate heuristic.
func SelectProfileAcceptance(d workspace.DefinitionDerivationRequest, m workspace.DefinitionAuthoringMapping, consumer string) (workspace.DefinitionAuthoringMapping, error) {
	profiles, err := workspace.AuthoringProfileContracts(d)
	if err != nil {
		return m, err
	}
	required, err := runtimecontract.RequiredRuntimeEvidencePurposes(profiles)
	if err != nil {
		return m, err
	}
	for _, r := range required {
		m.ProfileAcceptanceSelections = append(m.ProfileAcceptanceSelections, workspace.DefinitionProfileAcceptanceSelection{SourceRef: r.HistoricalItemRef, AcceptanceItemRef: consumer})
	}
	return m, nil
}
