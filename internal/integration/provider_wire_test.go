package integration_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// Fixtures consume provider scalar inputs and Runtime section messages.
// They preserve all message content and leave owner assertions unchanged.
type fixtureResponsesInput string

func (v *fixtureResponsesInput) UnmarshalJSON(data []byte) error {
	var text string
	if json.Unmarshal(data, &text) == nil {
		*v = fixtureResponsesInput(text)
		return nil
	}
	var messages []struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &messages); err != nil {
		return err
	}
	var parts []string
	for _, msg := range messages {
		if msg.Role != "developer" && msg.Role != "user" && msg.Role != "assistant" {
			return fmt.Errorf("invalid fixture message role")
		}
		var message strings.Builder
		for _, content := range msg.Content {
			if content.Type != "input_text" {
				return fmt.Errorf("invalid fixture content")
			}
			message.WriteString(content.Text)
		}
		parts = append(parts, message.String())
	}
	*v = fixtureResponsesInput(strings.Join(parts, "\n\n"))
	return nil
}

func TestFixtureResponsesInputPreservesSplitBlockBytes(t *testing.T) {
	var input fixtureResponsesInput
	raw := []byte(`[{"role":"developer","content":[{"type":"input_text","text":"contract"}]},{"role":"user","content":[{"type":"input_text","text":"{\"workContext\":{}"},{"type":"input_text","text":",\"planStep\":\"proof_batch\"}"}]}]`)
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	if string(input) != "contract\n\n"+`{"workContext":{},"planStep":"proof_batch"}` {
		t.Fatalf("block concatenation changed bytes: %q", input)
	}
}
