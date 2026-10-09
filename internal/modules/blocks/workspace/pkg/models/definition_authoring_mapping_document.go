package workspace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Decoder 由 Workspace owner 管理，供 Application 预检和 owner 重解析共同使用。
// schema 或 transport-native 格式不能放宽此合同。
func DecodeDefinitionAuthoringMapping(payload []byte) (DefinitionAuthoringMapping, error) {
	if len(payload) == 0 || len(payload) > DefinitionAuthoringDiagnosticDocumentLimit || !utf8.Valid(payload) {
		return DefinitionAuthoringMapping{}, fmt.Errorf("authoring mapping requires bounded complete UTF-8 document")
	}
	if err := mappingStringEscapes(payload); err != nil {
		return DefinitionAuthoringMapping{}, err
	}
	scanner := json.NewDecoder(bytes.NewReader(payload))
	scanner.UseNumber()
	first, err := scanner.Token()
	if err != nil || first != json.Delim('{') {
		return DefinitionAuthoringMapping{}, fmt.Errorf("authoring mapping requires one JSON object")
	}
	if err := mappingScanObject(scanner, 1, "$"); err != nil {
		return DefinitionAuthoringMapping{}, err
	}
	// 已验证根对象完整闭合；其后只能存在 JSON 允许的空白，不接收第二个根值。
	for _, b := range payload[scanner.InputOffset():] {
		if b != ' ' && b != '\t' && b != '\r' && b != '\n' {
			return DefinitionAuthoringMapping{}, fmt.Errorf("authoring mapping contains trailing data")
		}
	}
	if err := authoringMappingShape.validate(payload, "$"); err != nil {
		return DefinitionAuthoringMapping{}, err
	}
	var wire struct {
		Mapping DefinitionAuthoringMapping `json:"mapping"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return DefinitionAuthoringMapping{}, err
	}
	return wire.Mapping, nil
}

type mappingShape struct {
	kind   byte
	fields map[string]mappingShape
	item   *mappingShape
}

func mappingObject(fields map[string]mappingShape) mappingShape {
	return mappingShape{kind: 'o', fields: fields}
}
func mappingArray(item mappingShape) mappingShape { return mappingShape{kind: 'a', item: &item} }

var mappingString = mappingShape{kind: 's'}
var mappingStrings = mappingArray(mappingString)
var authoringMappingShape = mappingObject(map[string]mappingShape{"mapping": mappingObject(map[string]mappingShape{
	"Protocol": mappingString, "InputHash": mappingString, "TemplateSnapshotHash": mappingString, "ResourceSnapshotHash": mappingString,
	"GoalRequirement": mappingObject(map[string]mappingShape{"DimensionRef": mappingString, "ResourceIDs": mappingStrings}),
	"Dimensions":      mappingArray(mappingObject(map[string]mappingShape{"Ref": mappingString, "ResourceIDs": mappingStrings})),
	"Requirements":    mappingArray(mappingObject(map[string]mappingShape{"Ref": mappingString, "DimensionRef": mappingString, "SourceItemRefs": mappingStrings, "ResourceIDs": mappingStrings, "Text": mappingString})),
	"Obligations":     mappingArray(mappingObject(map[string]mappingShape{"CandidateItemRef": mappingString, "RequirementRef": mappingString, "Verification": mappingObject(map[string]mappingShape{"Declaration": mappingString, "Status": mappingString, "ProfileID": mappingString, "EvidenceRequirementID": mappingString, "Scope": mappingString})})),
})})

func (s mappingShape) validate(raw json.RawMessage, path string) error {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) {
		return fmt.Errorf("%s: null_not_allowed", path)
	}
	switch s.kind {
	case 'o':
		if len(raw) == 0 || raw[0] != '{' {
			return fmt.Errorf("%s: expected_object", path)
		}
		seen := map[string]bool{}
		d := json.NewDecoder(bytes.NewReader(raw))
		_, _ = d.Token()
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name := key.(string)
			child, ok := s.fields[name]
			if !ok {
				return fmt.Errorf("%s.%s: unknown_field", path, name)
			}
			seen[name] = true
			var value json.RawMessage
			if err := d.Decode(&value); err != nil {
				return err
			}
			if err := child.validate(value, path+"."+name); err != nil {
				return err
			}
		}
		keys := []string{}
		for name := range s.fields {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		for _, name := range keys {
			if !seen[name] {
				return fmt.Errorf("%s.%s: required_field_missing", path, name)
			}
		}
	case 'a':
		if len(raw) == 0 || raw[0] != '[' {
			return fmt.Errorf("%s: expected_array", path)
		}
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return err
		}
		for i, value := range values {
			if err := s.item.validate(value, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case 's':
		if len(raw) == 0 || raw[0] != '"' {
			return fmt.Errorf("%s: expected_string", path)
		}
	default:
		return fmt.Errorf("%s: unsupported_shape", path)
	}
	return nil
}
func mappingScanObject(d *json.Decoder, depth int, path string) error {
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("%s: invalid_key", path)
		}
		canonical := strings.ToLower(key)
		if seen[canonical] {
			return fmt.Errorf("%s.%s: duplicate_field", path, key)
		}
		seen[canonical] = true
		if err := mappingScanValue(d, depth, path+"."+key); err != nil {
			return err
		}
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') {
		return fmt.Errorf("%s: incomplete_object", path)
	}
	return nil
}
func mappingScanValue(d *json.Decoder, depth int, path string) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if depth >= 32 {
		return fmt.Errorf("%s: nesting_limit", path)
	}
	switch delim {
	case '{':
		return mappingScanObject(d, depth+1, path)
	case '[':
		for i := 0; d.More(); i++ {
			if err := mappingScanValue(d, depth+1, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("%s: incomplete_array", path)
		}
		return nil
	default:
		return fmt.Errorf("%s: unexpected_delimiter", path)
	}
}
func mappingStringEscapes(payload []byte) error {
	inString := false
	for i := 0; i < len(payload); i++ {
		if payload[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || payload[i] != '\\' {
			continue
		}
		i++
		if i >= len(payload) || payload[i] != 'u' {
			continue
		}
		value, ok := mappingUnicodeEscape(payload, i)
		if !ok {
			return fmt.Errorf("authoring mapping byte %d: invalid_unicode_escape", i-1)
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return fmt.Errorf("authoring mapping byte %d: isolated_low_surrogate", i-5)
		}
		if value >= 0xd800 && value <= 0xdbff {
			if i+2 >= len(payload) || payload[i+1] != '\\' || payload[i+2] != 'u' {
				return fmt.Errorf("authoring mapping byte %d: isolated_high_surrogate", i-5)
			}
			low, ok := mappingUnicodeEscape(payload, i+2)
			if !ok || low < 0xdc00 || low > 0xdfff {
				return fmt.Errorf("authoring mapping byte %d: invalid_surrogate_pair", i-5)
			}
			i += 6
		}
	}
	return nil
}
func mappingUnicodeEscape(payload []byte, u int) (uint64, bool) {
	if u+4 >= len(payload) {
		return 0, false
	}
	for _, b := range payload[u+1 : u+5] {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F') {
			return 0, false
		}
	}
	v, err := strconv.ParseUint(string(payload[u+1:u+5]), 16, 16)
	return v, err == nil
}

// 稳定 schema／协议身份进入 provider 请求 C0 和命令 hash。
// schema 只用于传输加速，不是 compiler 权威。
const DefinitionAuthoringMappingSchema = `{"type":"object","additionalProperties":false,"required":["mapping"],"properties":{"mapping":{"type":"object","additionalProperties":false,"required":["Protocol","InputHash","TemplateSnapshotHash","ResourceSnapshotHash","GoalRequirement","Dimensions","Requirements","Obligations"],"properties":{"Protocol":{"type":"string","enum":["workorch.authoring.sealed-ref-mapping/v1"]},"InputHash":{"type":"string"},"TemplateSnapshotHash":{"type":"string"},"ResourceSnapshotHash":{"type":"string"},"GoalRequirement":{"type":"object","additionalProperties":false,"required":["DimensionRef","ResourceIDs"],"properties":{"DimensionRef":{"type":"string"},"ResourceIDs":{"type":"array","items":{"type":"string"}}}},"Dimensions":{"type":"array","minItems":1,"maxItems":64,"items":{"type":"object","additionalProperties":false,"required":["Ref","ResourceIDs"],"properties":{"Ref":{"type":"string"},"ResourceIDs":{"type":"array","items":{"type":"string"}}}}},"Requirements":{"type":"array","minItems":1,"maxItems":255,"items":{"type":"object","additionalProperties":false,"required":["Ref","DimensionRef","SourceItemRefs","ResourceIDs","Text"],"properties":{"Ref":{"type":"string"},"DimensionRef":{"type":"string"},"SourceItemRefs":{"type":"array","minItems":1,"maxItems":32,"items":{"type":"string"}},"ResourceIDs":{"type":"array","items":{"type":"string"}},"Text":{"type":"string"}}}},"Obligations":{"type":"array","maxItems":512,"items":{"type":"object","additionalProperties":false,"required":["CandidateItemRef","RequirementRef","Verification"],"properties":{"CandidateItemRef":{"type":"string"},"RequirementRef":{"type":"string"},"Verification":{"type":"object","additionalProperties":false,"required":["Declaration","Status","ProfileID","EvidenceRequirementID","Scope"],"properties":{"Declaration":{"type":"string"},"Status":{"type":"string","enum":["DECLARED","UNVERIFIED"]},"ProfileID":{"type":"string"},"EvidenceRequirementID":{"type":"string"},"Scope":{"type":"string"}}}}}}}}}}`
