package workorchmanagedconfig

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

// These maps are local JSON syntax trees only, never EventHub authority or
// caller-supplied verdicts. Duplicate keys, trailing values and excessive depth
// are explicit failures. UseNumber avoids rounding distinct large integers.
func parsePreparationObject(raw []byte) (map[string]any, error) {
	if len(raw) > fileobservation.ResourceMaxBytes || !utf8.Valid(raw) || !validPreparationEscapes(raw) {
		return nil, common.ErrInvalidArgument
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	value, err := readPreparationJSON(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, common.ErrInvalidArgument
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return object, nil
}

// encoding/json replaces unpaired UTF-16 surrogate escapes with U+FFFD.
// Reject that lossy interpretation instead of treating distinct source bytes
// as the expected string. Escaped backslashes are skipped as a pair.
func validPreparationEscapes(raw []byte) bool {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		value, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
		if value >= 0xd800 && value <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

type preparationNumber string

func readPreparationJSON(d *json.Decoder, depth int) (any, error) {
	if depth > 32 {
		return nil, common.ErrInvalidArgument
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	if number, ok := token.(json.Number); ok {
		return preparationJSONNumber(string(number))
	}
	if delim, ok := token.(json.Delim); ok {
		switch delim {
		case '{':
			object := make(map[string]any)
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return nil, err
				}
				name, ok := key.(string)
				if !ok {
					return nil, common.ErrInvalidArgument
				}
				if _, found := object[name]; found {
					return nil, common.ErrInvalidArgument
				}
				value, err := readPreparationJSON(d, depth+1)
				if err != nil {
					return nil, err
				}
				object[name] = value
			}
			close, err := d.Token()
			if err != nil || close != json.Delim('}') {
				return nil, common.ErrInvalidArgument
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for d.More() {
				value, err := readPreparationJSON(d, depth+1)
				if err != nil {
					return nil, err
				}
				array = append(array, value)
			}
			close, err := d.Token()
			if err != nil || close != json.Delim(']') {
				return nil, common.ErrInvalidArgument
			}
			return array, nil
		}
		return nil, common.ErrInvalidArgument
	}
	return token, nil
}

func preparationJSONNumber(raw string) (preparationNumber, error) {
	negative := strings.HasPrefix(raw, "-")
	raw = strings.TrimPrefix(raw, "-")
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == 'e' || r == 'E' })
	exponent := int64(0)
	if len(parts) == 2 {
		var err error
		exponent, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || exponent < -(1<<30) || exponent > 1<<30 {
			return "", common.ErrInvalidArgument
		}
	}
	digits := parts[0]
	if dot := strings.IndexByte(digits, '.'); dot >= 0 {
		exponent -= int64(len(digits) - dot - 1)
		digits = digits[:dot] + digits[dot+1:]
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return "0", nil
	}
	trimmed := strings.TrimRight(digits, "0")
	exponent += int64(len(digits) - len(trimmed))
	sign := ""
	if negative {
		sign = "-"
	}
	return preparationNumber(sign + trimmed + "e" + strconv.FormatInt(exponent, 10)), nil
}

func equalPreparationJSON(actual, expected any) bool { return reflect.DeepEqual(actual, expected) }

func hasPreparationDefaults(actual, defaults map[string]any) bool {
	for key, fallback := range defaults {
		current, found := actual[key]
		if !found {
			return false
		}
		want, wantObject := fallback.(map[string]any)
		got, gotObject := current.(map[string]any)
		if wantObject && gotObject && !hasPreparationDefaults(got, want) {
			return false
		}
	}
	return true
}
