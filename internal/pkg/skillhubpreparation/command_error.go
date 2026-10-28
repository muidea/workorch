package skillhubpreparation

import (
	"bytes"
	"encoding/json"
	"io"
)

// ProjectNotFound is a negative command result, never an empty inventory or
// proof that an earlier service request did not save state. Preserve raw bytes.
func ProjectNotFound(raw []byte) bool {
	if !ValidStatusDocument(raw) {
		return false
	}
	var document struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"command_error"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(&document) == nil && d.Decode(new(json.RawMessage)) == io.EOF && document.Error != nil && document.Error.Code == "PROJECT_NOT_FOUND" && document.Error.Message != ""
}
