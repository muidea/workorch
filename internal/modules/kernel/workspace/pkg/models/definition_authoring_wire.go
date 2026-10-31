package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"unicode/utf8"
)

// The owner binds machine identities; the model returns only the current semantic body.
const DefinitionAuthoringOwnerBound = "workorch.authoring.owner-bound"

const authoringDeriveSemanticSchema = `{"type":"object","additionalProperties":false,"required":["AdditionalItems"],"properties":{"AdditionalItems":{"type":"array","maxItems":128,"items":{"type":"string"}}}}`
const authoringReviewSemanticSchema = `{"type":"object","additionalProperties":false,"required":["review"],"properties":{"review":{"type":"object","additionalProperties":false,"required":["Status","Reason","EvidenceRefs","RequirementIDs"],"properties":{"Status":{"type":"string","enum":["PASS","FAIL","UNKNOWN"]},"Reason":{"type":"string"},"EvidenceRefs":{"type":"array","items":{"type":"string"}},"RequirementIDs":{"type":"array","items":{"type":"string"}}}}}}`

func AuthoringWireSchema(ref, phase string) (string, error) {
	if !OwnerBoundAuthoringWire(ref) {
		return "", fmt.Errorf("unsupported owner-bound authoring wire")
	}
	switch phase {
	case "derive":
		return authoringDeriveSemanticSchema, nil
	case "proposal":
		// Detached schema transformation is a definition, not a provider document repair.
		var schema map[string]any
		if err := json.Unmarshal([]byte(DefinitionAuthoringMappingSchema), &schema); err != nil {
			return "", err
		}
		mapping := schema["properties"].(map[string]any)["mapping"].(map[string]any)
		fields := mapping["properties"].(map[string]any)

		fields["ProfileAcceptanceSelections"] = map[string]any{"type": "array", "maxItems": 512, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"SourceRef", "AcceptanceItemRef"}, "properties": map[string]any{"SourceRef": map[string]any{"type": "string"}, "AcceptanceItemRef": map[string]any{"type": "string"}}}}

		for _, k := range []string{"Protocol", "InputHash", "TemplateSnapshotHash", "ResourceSnapshotHash"} {
			delete(fields, k)
		}
		required := []any{}
		for _, v := range mapping["required"].([]any) {
			if _, ok := fields[v.(string)]; ok {
				required = append(required, v)
			}
		}
		mapping["required"] = required
		data, err := json.Marshal(schema)
		return string(data), err
	default:
		if GoalSpecificationReviewKind(phase) {
			return authoringReviewSemanticSchema, nil
		}
		return "", fmt.Errorf("unsupported authoring wire phase")
	}
}
func AuthoringWireHash(ref, phase string) string {
	if ref == "" {
		return ""
	}
	schema, err := AuthoringWireSchema(ref, phase)
	if err != nil {
		return ""
	}
	h := sha256.Sum256([]byte(ref + "\x00" + phase + "\x00" + schema))
	return "authoring-wire-sha256:" + hex.EncodeToString(h[:])
}
func validateAuthoringWire(ref, hash, phase string) error {

	if hash == "" || hash != AuthoringWireHash(ref, phase) {
		return fmt.Errorf("authoring wire contract identity drift")
	}
	return nil
}

// Provider bytes are never rewritten. The owner constructs a separate typed
// canonical value from its exact reservation and the closed semantic body.
func DecodeAuthoringMappingFor(d DefinitionDerivationRequest, payload []byte) (DefinitionAuthoringMapping, error) {

	if validateAuthoringWire(d.WireContractRef, d.WireContractHash, "derive") != nil {
		return DefinitionAuthoringMapping{}, fmt.Errorf("authoring wire derivation drift")
	}
	m, err := decodeDefinitionAuthoringMappingShape(payload, authoringSemanticMappingShape())
	if err != nil {
		return m, err
	}
	m.Protocol = DefinitionAuthoringMappingProtocol
	m.InputHash = d.Input.InputHash()
	m.TemplateSnapshotHash = WorkspaceTemplateSnapshotHash(&d.Template)
	m.ResourceSnapshotHash = d.ResourceSnapshotHash
	return m, nil
}
func decodeAuthoringSemantic(payload []byte, shape mappingShape, out any) error {
	if len(payload) == 0 || len(payload) > DefinitionAuthoringDiagnosticDocumentLimit || !utf8.Valid(payload) {
		return fmt.Errorf("authoring requires bounded complete UTF-8 document")
	}
	if err := mappingStringEscapes(payload); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(payload))
	d.UseNumber()
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return fmt.Errorf("authoring requires one JSON object")
	}
	if err = mappingScanObject(d, 1, "$"); err != nil {
		return err
	}
	if len(bytes.TrimSpace(payload[d.InputOffset():])) != 0 {
		return fmt.Errorf("authoring contains trailing data")
	}
	if err = shape.validate(payload, "$"); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}
func DecodeOwnerBoundDerivation(d DefinitionDerivationRequest, payload []byte) (DefinitionDerivedAcceptance, error) {
	if !OwnerBoundAuthoringWire(d.WireContractRef) || validateAuthoringWire(d.WireContractRef, d.WireContractHash, "derive") != nil {
		return DefinitionDerivedAcceptance{}, fmt.Errorf("derivation wire identity invalid")
	}
	var body struct{ AdditionalItems []string }
	if err := decodeAuthoringSemantic(payload, mappingObject(map[string]mappingShape{"AdditionalItems": mappingStrings}), &body); err != nil {
		return DefinitionDerivedAcceptance{}, err
	}
	p := DefinitionDerivedAcceptance{AuthoringInputHash: d.Input.InputHash(), TemplateSnapshotHash: WorkspaceTemplateSnapshotHash(&d.Template), AdditionalItems: body.AdditionalItems}
	return p, p.ValidateFor(d.Input, &d.Template)
}
func DecodeOwnerBoundReview(r DefinitionAuthoringSpecificationRequest, payload []byte) (GoalSpecificationCompletenessReview, error) {
	if !OwnerBoundAuthoringWire(r.WireContractRef) || validateAuthoringWire(r.WireContractRef, r.WireContractHash, r.ReviewKind) != nil {
		return GoalSpecificationCompletenessReview{}, fmt.Errorf("review wire identity invalid")
	}
	var body struct {
		Review struct {
			Status         string
			Reason         string
			EvidenceRefs   []string
			RequirementIDs []string
		} `json:"review"`
	}
	shape := mappingObject(map[string]mappingShape{"review": mappingObject(map[string]mappingShape{"Status": mappingString, "Reason": mappingString, "EvidenceRefs": mappingStrings, "RequirementIDs": mappingStrings})})
	if err := decodeAuthoringSemantic(payload, shape, &body); err != nil {
		return GoalSpecificationCompletenessReview{}, err
	}
	return GoalSpecificationCompletenessReview{Kind: r.ReviewKind, SpecificationHash: r.ProposalHash, Status: body.Review.Status, Reason: body.Review.Reason, EvidenceRefs: append([]string(nil), body.Review.EvidenceRefs...), RequirementIDs: append([]string(nil), body.Review.RequirementIDs...)}, nil
}
func validateOwnerBoundDerivationAudit(r DefinitionDerivationRequest, view bool) error {

	if r.Proposal == nil {
		if r.SuccessAudit != nil {
			return fmt.Errorf("non-success derivation carries audit")
		}
		return nil
	}
	if r.SuccessAudit == nil {
		return fmt.Errorf("owner-bound derivation lacks original document")
	}
	if view {
		return r.SuccessAudit.ValidateMetadata()
	}
	if err := r.SuccessAudit.Validate(); err != nil {
		return err
	}
	p, err := DecodeOwnerBoundDerivation(r, r.SuccessAudit.Document.Content)
	if err != nil || !reflect.DeepEqual(p, *r.Proposal) {
		return fmt.Errorf("derivation differs from original semantic document")
	}
	return nil
}

// OwnerBoundAuthoringWire identifies immutable supported semantic-only contracts.
func OwnerBoundAuthoringWire(ref string) bool {
	return ref == DefinitionAuthoringOwnerBound
}

func authoringSemanticMappingShape() mappingShape {
	fields := map[string]mappingShape{}
	for k, v := range authoringMappingShape.fields["mapping"].fields {
		fields[k] = v
	}

	shape := mappingArray(mappingObject(map[string]mappingShape{"SourceRef": mappingString, "AcceptanceItemRef": mappingString}))
	shape.optional = true
	fields["ProfileAcceptanceSelections"] = shape

	for _, k := range []string{"Protocol", "InputHash", "TemplateSnapshotHash", "ResourceSnapshotHash"} {
		delete(fields, k)
	}

	return mappingObject(map[string]mappingShape{"mapping": mappingObject(fields)})
}
