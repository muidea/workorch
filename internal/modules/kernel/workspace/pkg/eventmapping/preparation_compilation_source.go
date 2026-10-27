package eventmapping

import (
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
)

func compilationAuthoring(r workspace.DefinitionAuthoringRecord) workspaceevents.PreparationCompilationAuthoring {
	r = r.Clone()
	dto := DefinitionAuthoringRecordDTO(r)
	dto.PreparationDiagnostics = nil
	// Ordinary DTOs omit raw success/failure material. Private compilation must
	// retain the entire original ledger, not silently use that public projection.
	dto.SpecificationRequests = r.SpecificationRequests
	for i, request := range r.DerivationRequests {
		dto.DerivationRequests[i].Diagnostic = request.Diagnostic
	}
	return workspaceevents.PreparationCompilationAuthoring{Record: dto, Diagnostics: r.PreparationDiagnostics}
}

func PreparationCompilationSource(w *workspace.Workspace) *workspaceevents.PreparationCompilationSource {
	if w == nil {
		return nil
	}
	s := &workspaceevents.PreparationCompilationSource{Metadata: *WorkspaceDTO(w), Directories: workspace.ClonePreparationDirectoryEvidence(w.PreparationDirectoryEvidence), Resources: workspace.ClonePreparationResourceEvidence(w.PreparationResourceEvidence), Configs: workspace.ClonePreparationManagedConfigEvidence(w.PreparationManagedConfigEvidence), Souls: workspace.ClonePreparationSoulEvidence(w.PreparationSoulEvidence), Skills: workspace.ClonePreparationSkillRegistryEvidence(w.PreparationSkillRegistryEvidence), Providers: workspace.ClonePreparationProviderCacheEvidence(w.PreparationProviderCacheEvidence), Runs: workspace.ClonePreparationRunHistoryEvidence(w.PreparationRunHistoryEvidence), Memories: workspace.ClonePreparationMemoryInventoryEvidence(w.PreparationMemoryInventoryEvidence), MCP: workspace.ClonePreparationMCPRegistryEvidence(w.PreparationMCPRegistryEvidence), Effects: workspace.ClonePreparationSkillHubEffects(w.PreparationSkillHubEffects)}
	if w.SoulBindings == nil {
		s.Metadata.SoulBindings = nil
	}
	if w.DefinitionAuthoring != nil {
		v := compilationAuthoring(*w.DefinitionAuthoring)
		s.Authoring = &v
	}
	if w.DefinitionAuthoringConfirmation != nil {
		v := w.DefinitionAuthoringConfirmation.Clone()
		s.Confirmation = &v
	}
	if w.StartupPreparation != nil {
		v := w.StartupPreparation.Clone()
		s.Record = &v
	}
	if w.DefinitionAuthoringHistory != nil {
		s.History = make([]workspaceevents.PreparationCompilationHistory, len(w.DefinitionAuthoringHistory))
	}
	for i, h := range w.DefinitionAuthoringHistory {
		s.History[i] = workspaceevents.PreparationCompilationHistory{Authoring: compilationAuthoring(h.Record), Confirmation: h.Confirmation.Clone()}
	}
	if w.PreparationSealEvidence != nil {
		s.Seals = make([]workspaceevents.PreparationCompilationSeal, len(w.PreparationSealEvidence))
	}
	for i, source := range w.PreparationSealEvidence {
		e := source.Clone()
		v := workspaceevents.PreparationCompilationSeal{Version: e.Version, Ref: e.Ref, RequestID: e.RequestID, PayloadHash: e.PayloadHash, Binding: e.Binding, StartedAt: e.StartedAt, CompletedAt: e.CompletedAt, Before: e.Before, After: e.After, ContentHash: e.ContentHash}
		if x := e.ExecutionInputs; x != nil {
			c := x.Configuration
			v.Inputs = &workspaceevents.PreparationCompilationExecutionInputs{Version: x.Version, Process: x.Process, Template: *TemplateDTO(&x.Template), Configuration: workspaceevents.PreparationCompilationConfiguration{Directory: c.Directory, InternalDirectory: c.InternalDirectory, TemplateID: c.TemplateID, ProfileID: c.ProfileID, DefaultLLMSelection: c.DefaultLLMSelection, DefaultSoulID: c.DefaultSoulID, SoulBindings: soulbinding.FromSoulIDMap(c.SoulBindings), ContextHints: c.ContextHints, EvidencePolicies: c.EvidencePolicies}}
			if c.SoulBindings == nil {
				v.Inputs.Configuration.SoulBindings = nil
			}
		}
		s.Seals[i] = v
	}
	return s
}
