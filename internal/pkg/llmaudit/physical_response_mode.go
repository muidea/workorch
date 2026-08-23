package llmaudit

import (
	"fmt"
	"strings"
)

// PhysicalResponseMode records how one physical provider response was
// transported. It is audit framing only; it does not describe model behavior
// or select a Runtime transition.
type PhysicalResponseMode string

const (
	PhysicalResponseModeNonStreaming PhysicalResponseMode = "non_streaming"
	PhysicalResponseModeStreaming    PhysicalResponseMode = "streaming"
)

func (m PhysicalResponseMode) Normalized() PhysicalResponseMode {
	return PhysicalResponseMode(strings.TrimSpace(string(m)))
}

func (m PhysicalResponseMode) Validate() error {
	switch m.Normalized() {
	case PhysicalResponseModeNonStreaming, PhysicalResponseModeStreaming:
		return nil
	default:
		return fmt.Errorf("invalid physical response mode %q", m)
	}
}

// Streaming is a pure derivation from the bounded physical response mode.
func (m PhysicalResponseMode) Streaming() bool {
	return m.Normalized() == PhysicalResponseModeStreaming
}
