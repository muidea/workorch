package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
)

// MappingCompletionIdentity 绑定原 reservation permit、typed mapping 和完整文档/wire
// 观察身份。它不是 canonical hash，也不授予原文完整性或准入权。
func MappingCompletionIdentity(permit string, mapping DefinitionAuthoringMapping, audit DefinitionAuthoringSuccessByteAudit) string {
	metadata := audit.Metadata()
	payload, err := json.Marshal(struct {
		Protocol string
		Permit   string
		Mapping  DefinitionAuthoringMapping
		Audit    DefinitionAuthoringSuccessByteAudit
	}{DefinitionAuthoringMappingProtocol, permit, mapping, metadata})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return "authoring-mapping-completion-sha256:" + hex.EncodeToString(sum[:])
}

// ReparseMappingFailure 从 owner 原文重新证明失败 completion 的身份，不信持久自报 hash。
func ReparseMappingFailure(r DefinitionAuthoringSpecificationRequest, derivations ...DefinitionDerivationRequest) (DefinitionAuthoringMapping, DefinitionAuthoringSuccessByteAudit, error) {
	d := r.Diagnostic
	if r.Phase != DefinitionSpecificationProposal || r.FailureCode != "invalid_proposal" || r.Proposal != nil || r.Review != nil || r.Mapping != nil || r.SuccessAudit != nil || d == nil || d.Validate() != nil || d.ProviderCode != "" || d.ProviderDocument == nil || d.ProviderWire == nil {
		return DefinitionAuthoringMapping{}, DefinitionAuthoringSuccessByteAudit{}, fmt.Errorf("mapping failure lacks exact owner diagnostic")
	}
	a := DefinitionAuthoringSuccessByteAudit{Document: d.ProviderDocument.Clone(), Wire: d.ProviderWire.Clone()}
	if a.Validate() != nil || !reflect.DeepEqual(a.Document.Content, d.Payload) {
		return DefinitionAuthoringMapping{}, a, fmt.Errorf("mapping failure document differs from payload")
	}
	var derivation DefinitionDerivationRequest
	if len(derivations) == 1 {
		derivation = derivations[0]
	}
	if r.WireContractRef != "" && (derivation.WireContractRef != r.WireContractRef || derivation.PermitID != r.DerivationPermitID) {
		return DefinitionAuthoringMapping{}, a, fmt.Errorf("mapping failure missing exact wire derivation")
	}
	m, err := DecodeAuthoringMappingFor(derivation, a.Document.Content)
	if err != nil || r.CompletionIdentity != MappingCompletionIdentity(r.PermitID, m, a) {
		return m, a, fmt.Errorf("mapping failure completion identity differs from raw reparse")
	}
	return m, a, nil
}

// MappingFailureDiagnostic 保全 owner semantic rejection 的原错误与 document/wire。
func MappingFailureDiagnostic(err error, audit DefinitionAuthoringSuccessByteAudit) DefinitionAuthoringDiagnostic {
	diagnostic := NewDefinitionAuthoringDiagnostic(err.Error(), audit.Document.Content)
	document, wire := audit.Document.Clone(), audit.Wire.Clone()
	diagnostic.ProviderDocument, diagnostic.ProviderWire = &document, &wire
	return diagnostic
}

func (r DefinitionAuthoringRecord) CompleteMappingFailure(permit string, mapping DefinitionAuthoringMapping, audit DefinitionAuthoringSuccessByteAudit, diagnostic DefinitionAuthoringDiagnostic) (DefinitionAuthoringRecord, error) {
	if audit.Validate() != nil || diagnostic.Validate() != nil {
		return DefinitionAuthoringRecord{}, fmt.Errorf("mapping failure observation invalid")
	}
	var d DefinitionDerivationRequest
	for _, req := range r.SpecificationRequests {
		if req.PermitID == permit {
			var err error
			d, err = r.derivationForSpecification(req.DerivationPermitID)
			if err != nil {
				return DefinitionAuthoringRecord{}, err
			}
			break
		}
	}
	decoded, err := DecodeAuthoringMappingFor(d, audit.Document.Content)
	if err != nil || !reflect.DeepEqual(decoded, mapping) || diagnostic.ProviderDocument == nil || diagnostic.ProviderWire == nil || !reflect.DeepEqual(*diagnostic.ProviderDocument, audit.Document) || !reflect.DeepEqual(*diagnostic.ProviderWire, audit.Wire) {
		return DefinitionAuthoringRecord{}, fmt.Errorf("mapping failure raw identity mismatch")
	}
	return r.completeSpecification(permit, nil, nil, "invalid_proposal", []*DefinitionAuthoringDiagnostic{&diagnostic}, nil, nil, MappingCompletionIdentity(permit, mapping, audit))
}
