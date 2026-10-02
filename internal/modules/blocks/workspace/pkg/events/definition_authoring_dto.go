package events

import (
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

// 创作回执使用 concrete Template DTO，不把持久模型中的 map 带入 EventHub。
type DefinitionDerivationRequest struct {
	RequestID            string
	Input                workspace.DefinitionAuthoringInput
	Template             WorkspaceTemplate
	Resources            []workspace.DefinitionAuthoringResourceSnapshot
	ResourceSnapshotHash string
	ProviderBindingHash  string
	PermitID             string
	Proposal             *workspace.DefinitionDerivedAcceptance
	FailureCode          string
}

type DefinitionAuthoringRecord struct {
	Input                 workspace.DefinitionAuthoringInput
	InputHash             string
	TemplateSnapshotHash  string
	TemplateSnapshot      WorkspaceTemplate
	DerivationRequests    []DefinitionDerivationRequest
	SpecificationRequests []workspace.DefinitionAuthoringSpecificationRequest
	SaveReceipts          []workspace.DefinitionAuthoringSaveReceipt
}

type DefinitionAuthoringConfirmedRevision struct {
	Record       DefinitionAuthoringRecord
	Confirmation workspace.DefinitionAuthoringConfirmation
}

func cloneAuthoringTemplate(t WorkspaceTemplate) WorkspaceTemplate {
	t.ParentTemplateIDs = append([]string(nil), t.ParentTemplateIDs...)
	t.EffectiveSources = append([]workspace.TemplateSource(nil), t.EffectiveSources...)
	t.DoDItems = append([]string(nil), t.DoDItems...)
	t.SoulBindings = append([]soulbinding.Binding(nil), t.SoulBindings...)
	t.ContextHints = t.ContextHints.Clone()
	t.EvidencePolicies = workorcheventpayload.CloneSlice(t.EvidencePolicies)
	return t
}

func authoringTemplateModel(t WorkspaceTemplate) (workspace.WorkspaceTemplate, error) {
	// 转为 map 前拒绝重复或空 binding；不能让后写覆盖掩盖原 DTO 的 cardinality。
	if err := soulbinding.Validate(t.SoulBindings); err != nil {
		return workspace.WorkspaceTemplate{}, err
	}
	return workspace.WorkspaceTemplate{
		ID: t.ID, ParentTemplateIDs: append([]string(nil), t.ParentTemplateIDs...),
		EffectiveSources: append([]workspace.TemplateSource(nil), t.EffectiveSources...),
		Name:             t.Name, Description: t.Description, GoalText: t.GoalText,
		DoDItems: append([]string(nil), t.DoDItems...), DefaultSoulID: t.DefaultSoulID,
		SoulBindings: soulbinding.StringMap(t.SoulBindings), ContextHints: t.ContextHints.Clone(),
		EvidencePolicies: workorcheventpayload.CloneSlice(t.EvidencePolicies),
	}, nil
}

func (r DefinitionDerivationRequest) Clone() DefinitionDerivationRequest {
	r.Input = r.Input.Clone()
	r.Template = cloneAuthoringTemplate(r.Template)
	r.Resources = workspace.CloneDefinitionAuthoringResources(r.Resources)
	if r.Proposal != nil {
		proposal := *r.Proposal
		if proposal.AdditionalItems != nil {
			proposal.AdditionalItems = append([]string{}, proposal.AdditionalItems...)
		}
		r.Proposal = &proposal
	}
	return r
}

// Model 只做无损、detached 表示转换，不授予已校验身份或 provider 调用许可。
func (r DefinitionDerivationRequest) Model() (workspace.DefinitionDerivationRequest, error) {
	template, err := authoringTemplateModel(r.Template)
	if err != nil {
		return workspace.DefinitionDerivationRequest{}, err
	}
	return workspace.DefinitionDerivationRequest{
		RequestID: r.RequestID, Input: r.Input, Template: template, Resources: r.Resources,
		ResourceSnapshotHash: r.ResourceSnapshotHash, ProviderBindingHash: r.ProviderBindingHash,
		PermitID: r.PermitID, Proposal: r.Proposal, FailureCode: r.FailureCode,
	}.Clone(), nil
}

func (r DefinitionDerivationRequest) Validate() error {
	model, err := r.Model()
	if err != nil {
		return err
	}
	return model.Validate()
}

func (r DefinitionAuthoringRecord) Clone() DefinitionAuthoringRecord {
	r.Input = r.Input.Clone()
	r.TemplateSnapshot = cloneAuthoringTemplate(r.TemplateSnapshot)
	r.DerivationRequests = append([]DefinitionDerivationRequest(nil), r.DerivationRequests...)
	for n := range r.DerivationRequests {
		r.DerivationRequests[n] = r.DerivationRequests[n].Clone()
	}
	r.SpecificationRequests = append([]workspace.DefinitionAuthoringSpecificationRequest(nil), r.SpecificationRequests...)
	for n := range r.SpecificationRequests {
		r.SpecificationRequests[n] = r.SpecificationRequests[n].Clone()
	}
	r.SaveReceipts = append([]workspace.DefinitionAuthoringSaveReceipt(nil), r.SaveReceipts...)
	return r
}

func (r DefinitionAuthoringRecord) Model() (workspace.DefinitionAuthoringRecord, error) {
	if len(r.DerivationRequests) > workspace.DefinitionDerivationRequestLimit || len(r.SpecificationRequests) > workspace.DefinitionAuthoringSpecificationRequestLimit || len(r.SaveReceipts) > workspace.DefinitionAuthoringSaveReceiptLimit {
		return workspace.DefinitionAuthoringRecord{}, common.ErrInvalidArgument
	}
	template, err := authoringTemplateModel(r.TemplateSnapshot)
	if err != nil {
		return workspace.DefinitionAuthoringRecord{}, err
	}
	ret := workspace.DefinitionAuthoringRecord{Input: r.Input.Clone(), InputHash: r.InputHash, TemplateSnapshotHash: r.TemplateSnapshotHash, TemplateSnapshot: template, SaveReceipts: append([]workspace.DefinitionAuthoringSaveReceipt(nil), r.SaveReceipts...)}
	if r.DerivationRequests != nil {
		ret.DerivationRequests = make([]workspace.DefinitionDerivationRequest, len(r.DerivationRequests))
	}
	for n, request := range r.DerivationRequests {
		ret.DerivationRequests[n], err = request.Model()
		if err != nil {
			return workspace.DefinitionAuthoringRecord{}, err
		}
	}
	if r.SpecificationRequests != nil {
		ret.SpecificationRequests = make([]workspace.DefinitionAuthoringSpecificationRequest, len(r.SpecificationRequests))
	}
	for n, request := range r.SpecificationRequests {
		ret.SpecificationRequests[n] = request.Clone()
	}
	return ret, nil
}

func (r DefinitionAuthoringRecord) Validate() error {
	model, err := r.Model()
	if err != nil {
		return err
	}
	return model.Validate()
}

func (r DefinitionAuthoringConfirmedRevision) Clone() DefinitionAuthoringConfirmedRevision {
	r.Record = r.Record.Clone()
	r.Confirmation = r.Confirmation.Clone()
	return r
}

func (r DefinitionAuthoringConfirmedRevision) Model() (workspace.DefinitionAuthoringConfirmedRevision, error) {
	record, err := r.Record.Model()
	if err != nil {
		return workspace.DefinitionAuthoringConfirmedRevision{}, err
	}
	return workspace.DefinitionAuthoringConfirmedRevision{Record: record, Confirmation: r.Confirmation.Clone()}, nil
}

func AuthoringHistoryModels(history []DefinitionAuthoringConfirmedRevision) ([]workspace.DefinitionAuthoringConfirmedRevision, error) {
	if len(history) > workspace.DefinitionDerivationRequestLimit {
		return nil, common.ErrInvalidArgument
	}
	if history == nil {
		return nil, nil
	}
	ret := make([]workspace.DefinitionAuthoringConfirmedRevision, len(history))
	for n, revision := range history {
		model, err := revision.Model()
		if err != nil {
			return nil, err
		}
		ret[n] = model
	}
	return ret, nil
}
