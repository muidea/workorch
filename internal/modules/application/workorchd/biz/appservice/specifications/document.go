package specifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const maxDocumentBytes = 256 * 1024
const maxDocumentDepth = 32

// validateDocumentShape rejects ambiguous provider documents before decoding
// them into an authoring proposal. It never repairs or extracts fenced JSON.
func validateDocumentShape(payload []byte) error {
	if len(payload) == 0 || len(payload) > maxDocumentBytes {
		return fmt.Errorf("specification document size must be in 1..%d", maxDocumentBytes)
	}
	if !utf8.Valid(payload) {
		return fmt.Errorf("specification document requires valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return fmt.Errorf("specification document requires one JSON object")
	}
	if err := readObject(decoder, 1); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("specification document contains trailing data")
	}
	return nil
}

func readObject(decoder *json.Decoder, depth int) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return fmt.Errorf("specification document object key is invalid")
		}
		// encoding/json 对结构体字段名不区分大小写，必须拒绝这类覆盖歧义。
		key = strings.ToLower(key)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("specification document contains duplicate field")
		}
		seen[key] = struct{}{}
		if err := readValue(decoder, depth); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return fmt.Errorf("specification document object is incomplete")
	}
	return nil
}

func readValue(decoder *json.Decoder, depth int) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, container := token.(json.Delim)
	if !container {
		return nil
	}
	if depth >= maxDocumentDepth {
		return fmt.Errorf("specification document nesting exceeds limit")
	}
	switch delim {
	case '{':
		return readObject(decoder, depth+1)
	case '[':
		for decoder.More() {
			if err := readValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("specification document array is incomplete")
		}
		return nil
	default:
		return fmt.Errorf("specification document has unexpected delimiter")
	}
}
