package events

import (
	"time"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Private detached compiler input, not a Workspace facade or validation token.
// Map-backed persistence representations never cross EventHub. All original
// diagnostics remain complete; this type must never be included in HTTP DTOs.
type PreparationCompilationSource struct {
	Metadata     Workspace
	Authoring    *PreparationCompilationAuthoring
	Confirmation *workspace.DefinitionAuthoringConfirmation
	History      []PreparationCompilationHistory
	Record       *workspace.WorkspacePreparationRecord
	Directories  []workspace.WorkspacePreparationDirectoryEvidence
	Resources    []workspace.WorkspacePreparationResourceEvidence
	Configs      []workspace.WorkspacePreparationManagedConfigEvidence
	Souls        []workspace.WorkspacePreparationSoulEvidence
	Skills       []workspace.WorkspacePreparationSkillRegistryEvidence
	Providers    []workspace.WorkspacePreparationProviderCacheEvidence
	Runs         []workspace.WorkspacePreparationRunHistoryEvidence
	Memories     []workspace.WorkspacePreparationMemoryInventoryEvidence
	MCP          []workspace.WorkspacePreparationMCPRegistryEvidence
	Effects      []workspace.WorkspacePreparationSkillHubEffects
	Seals        []PreparationCompilationSeal
}

type PreparationCompilationAuthoring struct {
	Record DefinitionAuthoringRecord
	// The ordinary authoring DTO deliberately projects these as metadata.
	// This private compiler input instead retains the full original bytes.
	Diagnostics []workspace.DefinitionAuthoringPreparationDiagnostic
}

type PreparationCompilationHistory struct {
	Authoring    PreparationCompilationAuthoring
	Confirmation workspace.DefinitionAuthoringConfirmation
}

type PreparationCompilationSeal struct {
	Version, Ref, RequestID, PayloadHash string
	Binding                              workspace.WorkspacePreparationBinding
	StartedAt, CompletedAt               time.Time
	Before, After                        workspace.PreparationSealSnapshot
	ContentHash                          string
	Inputs                               *PreparationCompilationExecutionInputs
}

type PreparationCompilationExecutionInputs struct {
	Version       string
	Process       workspace.PreparationProcessPolicyFacts
	Template      WorkspaceTemplate
	Configuration PreparationCompilationConfiguration
}

type PreparationCompilationConfiguration struct {
	Directory, InternalDirectory, TemplateID, ProfileID string
	DefaultLLMSelection                                 *runtimecontract.ProviderInferenceSelection
	DefaultSoulID                                       common.SoulID
	SoulBindings                                        []soulbinding.Binding
	ContextHints                                        workorcheventpayload.Payload
	EvidencePolicies                                    []workorcheventpayload.Payload
}

func (s PreparationCompilationAuthoring) model() (workspace.DefinitionAuthoringRecord, error) {
	if len(s.Record.PreparationDiagnostics) != 0 || len(s.Diagnostics) > workspace.DefinitionAuthoringPreparationDiagnosticLimit {
		return workspace.DefinitionAuthoringRecord{}, common.ErrConflict
	}
	r, err := s.Record.Model()
	if err != nil {
		return r, err
	}
	if s.Diagnostics != nil {
		r.PreparationDiagnostics = make([]workspace.DefinitionAuthoringPreparationDiagnostic, len(s.Diagnostics))
	}
	for i, d := range s.Diagnostics {
		r.PreparationDiagnostics[i] = d.Clone()
	}
	return r, nil
}

// Model performs only explicit reversible representation conversion and clone.
// Callers must independently validate the full archives before compilation.
func (s *PreparationCompilationSource) Model() (*workspace.Workspace, error) {
	if s == nil || soulbinding.Validate(s.Metadata.SoulBindings) != nil {
		return nil, common.ErrConflict
	}
	m := s.Metadata
	w := &workspace.Workspace{ID: m.ID, Revision: m.Revision, Title: m.Title, Description: m.Description, DeliverableDirectory: m.DeliverableDirectory, InternalDirectory: m.InternalDirectory, State: m.State, ActiveRunID: m.ActiveRunID, TemplateID: m.TemplateID, TemplateBindingHash: m.TemplateBindingHash, ProfileID: m.ProfileID, DefaultSoulID: m.DefaultSoulID, SoulBindings: soulbinding.SoulIDMap(m.SoulBindings), ContextHints: m.ContextHints.Clone(), EvidencePolicies: workorcheventpayload.CloneSlice(m.EvidencePolicies)}
	if m.DefaultLLMSelection != nil {
		v := *m.DefaultLLMSelection
		w.DefaultLLMSelection = &v
	}
	if m.StartupInput != nil {
		v := m.StartupInput.Clone()
		w.StartupInput = &v
	}
	if m.ActivationAdmission != nil {
		v := *m.ActivationAdmission
		w.ActivationAdmission = &v
	}
	if m.DefinitionAdmissionCandidates != nil {
		w.DefinitionAdmissionCandidates = make([]workspace.DefinitionAdmissionCandidate, len(m.DefinitionAdmissionCandidates))
	}
	for i, c := range m.DefinitionAdmissionCandidates {
		w.DefinitionAdmissionCandidates[i] = c.Clone()
	}
	if s.Authoring != nil {
		v, err := s.Authoring.model()
		if err != nil {
			return nil, err
		}
		w.DefinitionAuthoring = &v
	}
	if s.Confirmation != nil {
		v := s.Confirmation.Clone()
		w.DefinitionAuthoringConfirmation = &v
	}
	if s.History != nil {
		w.DefinitionAuthoringHistory = make([]workspace.DefinitionAuthoringConfirmedRevision, len(s.History))
	}
	for i, h := range s.History {
		r, err := h.Authoring.model()
		if err != nil {
			return nil, err
		}
		w.DefinitionAuthoringHistory[i] = workspace.DefinitionAuthoringConfirmedRevision{Record: r, Confirmation: h.Confirmation.Clone()}
	}
	if s.Record != nil {
		v := s.Record.Clone()
		w.StartupPreparation = &v
	}
	w.PreparationDirectoryEvidence = workspace.ClonePreparationDirectoryEvidence(s.Directories)
	w.PreparationResourceEvidence = workspace.ClonePreparationResourceEvidence(s.Resources)
	w.PreparationManagedConfigEvidence = workspace.ClonePreparationManagedConfigEvidence(s.Configs)
	w.PreparationSoulEvidence = workspace.ClonePreparationSoulEvidence(s.Souls)
	w.PreparationSkillRegistryEvidence = workspace.ClonePreparationSkillRegistryEvidence(s.Skills)
	w.PreparationProviderCacheEvidence = workspace.ClonePreparationProviderCacheEvidence(s.Providers)
	w.PreparationRunHistoryEvidence = workspace.ClonePreparationRunHistoryEvidence(s.Runs)
	w.PreparationMemoryInventoryEvidence = workspace.ClonePreparationMemoryInventoryEvidence(s.Memories)
	w.PreparationMCPRegistryEvidence = workspace.ClonePreparationMCPRegistryEvidence(s.MCP)
	w.PreparationSkillHubEffects = workspace.ClonePreparationSkillHubEffects(s.Effects)
	if s.Seals != nil {
		w.PreparationSealEvidence = make([]workspace.WorkspacePreparationSealEvidence, len(s.Seals))
	}
	for i, e := range s.Seals {
		v := workspace.WorkspacePreparationSealEvidence{Version: e.Version, Ref: e.Ref, RequestID: e.RequestID, PayloadHash: e.PayloadHash, Binding: e.Binding, StartedAt: e.StartedAt, CompletedAt: e.CompletedAt, Before: e.Before.Clone(), After: e.After.Clone(), ContentHash: e.ContentHash}
		if e.Inputs != nil {
			x := e.Inputs
			c := x.Configuration
			if soulbinding.Validate(c.SoulBindings) != nil {
				return nil, common.ErrConflict
			}
			t, err := authoringTemplateModel(x.Template)
			if err != nil {
				return nil, err
			}
			inputs := workspace.PreparationExecutionInputs{Version: x.Version, Process: x.Process, Template: t, Configuration: workspace.WorkspaceConfigurationSnapshot{Directory: c.Directory, InternalDirectory: c.InternalDirectory, TemplateID: c.TemplateID, ProfileID: c.ProfileID, DefaultLLMSelection: c.DefaultLLMSelection, DefaultSoulID: c.DefaultSoulID, SoulBindings: soulbinding.SoulIDMap(c.SoulBindings), ContextHints: c.ContextHints, EvidencePolicies: c.EvidencePolicies}}
			cloned := inputs.Clone()
			v.ExecutionInputs = &cloned
		}
		w.PreparationSealEvidence[i] = v
	}
	return w, nil
}
