package specifications

import (
	"strings"
	"testing"
)

func TestValidateDocumentShape(t *testing.T) {
	for name, payload := range map[string]string{
		"object": `{}`,
		"nested": `{"items":[{"id":"a"},{"id":"b"}],"review":{"pass":true}}`,
		"space":  " \n{\"text\":\"中文\"}\t",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDocumentShape([]byte(payload)); err != nil {
				t.Fatal(err)
			}
		})
	}
	for name, payload := range map[string]string{
		"empty":                 "",
		"null":                  `null`,
		"array":                 `[]`,
		"fenced":                "```json\n{}\n```",
		"case duplicate":        `{"Status":"FAIL","status":"PASS"}`,
		"nested case duplicate": `{"review":{"Kind":"traceability","kind":"regression"}}`,
		"invalid utf8":          "{\"text\":\"\xff\"}",
		"duplicate":             `{"id":"a","id":"b"}`,
		"escaped duplicate":     "{\"id\":\"a\",\"\\u0069d\":\"b\"}",
		"nested duplicate":      `{"items":[{"id":"a","id":"b"}]}`,
		"trailing":              `{} {}`,
		"trailing text":         `{} done`,
		"incomplete":            `{"items":[`,
		"deep":                  `{"items":` + strings.Repeat("[", maxDocumentDepth) + `0` + strings.Repeat("]", maxDocumentDepth) + `}`,
		"oversized":             `{"text":"` + strings.Repeat("a", maxDocumentBytes) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDocumentShape([]byte(payload)); err == nil {
				t.Fatal("ambiguous or unbounded document accepted")
			}
		})
	}
}
