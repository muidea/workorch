package specifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// 当前 DTO 的封闭形状，不解释 provider JSON Schema，也不授予业务身份或准入权。
type documentShape struct {
	kind   byte
	fields map[string]documentShape
	item   *documentShape
}

var (
	stringShape  = documentShape{kind: 's'}
	boolShape    = documentShape{kind: 'b'}
	uintShape    = documentShape{kind: 'u'}
	stringsShape = arrayShape(stringShape)
	reviewShape  = objectShape(map[string]documentShape{
		"Kind": stringShape, "Status": stringShape, "SpecificationHash": stringShape,
		"Reason": stringShape, "EvidenceRefs": stringsShape, "RequirementIDs": stringsShape,
	})
	proposalShape = objectShape(map[string]documentShape{"specification": objectShape(map[string]documentShape{
		"Version": uintShape, "GoalItemRef": stringShape, "GoalTextHash": stringShape,
		"Resources": arrayShape(objectShape(map[string]documentShape{
			"ID": stringShape, "Ref": stringShape, "SnapshotHash": stringShape, "Required": boolShape, "Available": boolShape,
		})),
		"Dimensions": arrayShape(objectShape(map[string]documentShape{
			"ID": stringShape, "Required": boolShape, "Activated": boolShape, "ResourceIDs": stringsShape,
		})),
		"Requirements": arrayShape(objectShape(map[string]documentShape{
			"ID": stringShape, "DimensionID": stringShape, "Required": boolShape, "SourceKind": stringShape,
			"SourceItemRefs": stringsShape, "ResourceIDs": stringsShape, "Text": stringShape,
		})),
		"Obligations": arrayShape(objectShape(map[string]documentShape{
			"ID": stringShape, "RequirementID": stringShape, "DoDItemRefs": stringsShape, "Text": stringShape,
		})),
		"Verifications": arrayShape(objectShape(map[string]documentShape{
			"ID": stringShape, "ObligationID": stringShape, "Declaration": stringShape, "Status": stringShape,
			"ProfileID": stringShape, "EvidenceRequirementID": stringShape, "Scope": stringShape,
		})),
		"Reviews": arrayShape(reviewShape),
	})})
	derivationShape = objectShape(map[string]documentShape{
		"AuthoringInputHash": stringShape, "TemplateSnapshotHash": stringShape, "AdditionalItems": stringsShape,
	})
)

func objectShape(fields map[string]documentShape) documentShape {
	return documentShape{kind: 'o', fields: fields}
}

func arrayShape(item documentShape) documentShape {
	return documentShape{kind: 'a', item: &item}
}

func validateCurrentDocument(payload []byte, shape documentShape) error {
	if err := validateDocumentShape(payload); err != nil {
		return err
	}
	return shape.validate(payload, "$")
}

func (s documentShape) validate(raw json.RawMessage, path string) error {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) {
		return fmt.Errorf("%s: null_not_allowed", path)
	}
	switch s.kind {
	case 'o':
		if len(raw) == 0 || raw[0] != '{' {
			return fmt.Errorf("%s: expected_object", path)
		}
		fields := make(map[string]struct{}, len(s.fields))
		// 按 wire 顺序定位错误，避免 map 遍历导致诊断不确定。
		decoder := json.NewDecoder(bytes.NewReader(raw))
		_, _ = decoder.Token()
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return fmt.Errorf("%s: invalid_field: %w", path, err)
			}
			name := key.(string)
			fields[name] = struct{}{}
			child, ok := s.fields[name]
			if !ok {
				return fmt.Errorf("%s.%s: unknown_field", path, name)
			}
			var value json.RawMessage
			if err := decoder.Decode(&value); err != nil {
				return fmt.Errorf("%s.%s: invalid_value: %w", path, name, err)
			}
			if err := child.validate(value, path+"."+name); err != nil {
				return err
			}
		}
		if len(fields) != len(s.fields) {
			// 缺字段诊断按固定字典序选择，不依赖进程 map seed。
			missing := ""
			for name := range s.fields {
				if _, exists := fields[name]; !exists && (missing == "" || name < missing) {
					missing = name
				}
			}
			return fmt.Errorf("%s.%s: required_field_missing", path, missing)
		}
	case 'a':
		if len(raw) == 0 || raw[0] != '[' {
			return fmt.Errorf("%s: expected_array", path)
		}
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return fmt.Errorf("%s: invalid_array: %w", path, err)
		}
		for i, item := range items {
			if err := s.item.validate(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case 's':
		if len(raw) == 0 || raw[0] != '"' {
			return fmt.Errorf("%s: expected_string", path)
		}
	case 'b':
		if !bytes.Equal(raw, []byte("true")) && !bytes.Equal(raw, []byte("false")) {
			return fmt.Errorf("%s: expected_boolean", path)
		}
	case 'u':
		if _, err := strconv.ParseUint(string(raw), 10, 32); err != nil {
			return fmt.Errorf("%s: expected_uint32", path)
		}
	default:
		return fmt.Errorf("%s: unsupported_document_shape", path)
	}
	return nil
}

// encoding/json 会将孤立 UTF-16 surrogate 替换为 U+FFFD；必须在解码前拒绝。
func validateStringEscapes(payload []byte) error {
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
		value, ok := unicodeEscape(payload, i)
		if !ok {
			return fmt.Errorf("specification document byte %d: invalid_unicode_escape", i-1)
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return fmt.Errorf("specification document byte %d: isolated_low_surrogate", i-5)
		}
		if value >= 0xd800 && value <= 0xdbff {
			if i+2 >= len(payload) || payload[i+1] != '\\' || payload[i+2] != 'u' {
				return fmt.Errorf("specification document byte %d: isolated_high_surrogate", i-5)
			}
			low, ok := unicodeEscape(payload, i+2)
			if !ok || low < 0xdc00 || low > 0xdfff {
				return fmt.Errorf("specification document byte %d: invalid_surrogate_pair", i-5)
			}
			i += 6
		}
	}
	return nil
}

func unicodeEscape(payload []byte, u int) (uint64, bool) {
	if u+4 >= len(payload) {
		return 0, false
	}
	for _, b := range payload[u+1 : u+5] {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F') {
			return 0, false
		}
	}
	value, err := strconv.ParseUint(string(payload[u+1:u+5]), 16, 16)
	return value, err == nil
}
