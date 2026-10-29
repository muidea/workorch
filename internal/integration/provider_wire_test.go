package integration_test

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Fixtures consume both historical scalar Runtime inputs and section messages.
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
		for _, content := range msg.Content {
			if content.Type != "input_text" {
				return fmt.Errorf("invalid fixture content")
			}
			parts = append(parts, content.Text)
		}
	}
	*v = fixtureResponsesInput(strings.Join(parts, "\n\n"))
	return nil
}
