package workspace

import (
	"fmt"
	"reflect"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

// 每次 owner load 都从原 document 重编译及重判 semantic error；失败摘要/自报 hash 不是权威。
func validateAuthoringMappingOutcomes(record workspacemodel.DefinitionAuthoringRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	for _, r := range record.SpecificationRequests {
		if r.Proposal == nil && r.FailureCode != "invalid_proposal" {
			continue
		}
		if r.FailureCode == "invalid_proposal" {
			diagnostic := r.Diagnostic
			if diagnostic == nil || diagnostic.ProviderCode != "" || diagnostic.ProviderCause != nil || diagnostic.ProviderDocument == nil || diagnostic.ProviderWire == nil || !diagnostic.ProviderDocument.Observed || !diagnostic.ProviderDocument.Complete || !diagnostic.ProviderDocument.Saved || !reflect.DeepEqual(diagnostic.Payload, diagnostic.ProviderDocument.Content) {
				return fmt.Errorf("invalid proposal lacks complete original parser document")
			}
			_, parseErr := workspacemodel.DecodeDefinitionAuthoringMapping(diagnostic.ProviderDocument.Content)
			if parseErr != nil {
				if r.CompletionIdentity != "" {
					return fmt.Errorf("parser failure claims semantic mapping completion identity")
				}
				expected := workspacemodel.NewDefinitionAuthoringDiagnostic(parseErr.Error(), diagnostic.ProviderDocument.Content)
				document, wire := diagnostic.ProviderDocument.Clone(), diagnostic.ProviderWire.Clone()
				expected.ProviderDocument, expected.ProviderWire = &document, &wire
				if !reflect.DeepEqual(expected, *diagnostic) {
					return fmt.Errorf("parser failure original error differs from owner re-evaluation")
				}
				continue
			}
			if r.CompletionIdentity == "" {
				return fmt.Errorf("decoded current mapping failure lacks semantic completion identity")
			}
		}
		var d *workspacemodel.DefinitionDerivationRequest
		for i := range record.DerivationRequests {
			if record.DerivationRequests[i].PermitID == r.DerivationPermitID {
				d = &record.DerivationRequests[i]
				break
			}
		}
		if d == nil {
			return fmt.Errorf("mapping outcome has no sealed derivation")
		}
		if r.Proposal != nil {
			if err := validateAuthoringVerificationAgainstSealedTemplate(workspacemodel.WorkspaceTemplateSnapshotHash(&d.Template), *d, *r.Proposal); err != nil {
				return err
			}
			continue
		}
		m, a, err := workspacemodel.ReparseMappingFailure(r)
		if err != nil {
			return err
		}
		s, semanticErr := workspacemodel.CompileDefinitionAuthoringMapping(*d, m)
		if semanticErr == nil {
			semanticErr = validateAuthoringVerificationAgainstSealedTemplate(workspacemodel.WorkspaceTemplateSnapshotHash(&d.Template), *d, s)
		}
		if semanticErr == nil {
			return fmt.Errorf("mapping failure is not a known invalid proposal")
		}
		expected := workspacemodel.MappingFailureDiagnostic(semanticErr, a)
		if !reflect.DeepEqual(expected, *r.Diagnostic) {
			return fmt.Errorf("mapping failure original error differs from owner re-evaluation")
		}
	}
	return nil
}
func validateWorkspaceAuthoringMappingOutcomes(w *workspacemodel.Workspace) error {
	if w == nil {
		return fmt.Errorf("mapping owner workspace missing")
	}
	if w.DefinitionAuthoring != nil {
		if err := validateAuthoringMappingOutcomes(*w.DefinitionAuthoring); err != nil {
			return err
		}
	}
	for _, h := range w.DefinitionAuthoringHistory {
		if err := validateAuthoringMappingOutcomes(h.Record); err != nil {
			return err
		}
	}
	return nil
}
