package tui

import (
	"strings"
	"testing"
)

func TestAuthoringGuidanceNeverRecommendsRetiredWrites(t *testing.T) {
	m := NewAppModel(newFake(t), "", "")
	for name, text := range map[string]string{
		"empty":   m.emptyResultText(),
		"help":    stripANSI(m.renderHelp(240)),
		"compact": stripANSI(m.renderHelp(100)),
		"overlay": stripANSI(m.renderHelpOverlay(160, 60)),
	} {
		for _, retired := range []string{"/goal", "/dod", "/workspace create", "creates a new goal"} {
			if strings.Contains(text, retired) {
				t.Errorf("%s 仍推荐退休写操作 %q: %s", name, retired, text)
			}
		}
	}
	for _, suggestions := range [][]slashCommandSuggestion{baseSlashCommandSuggestions(), m.contextualSlashCommandSuggestions(), m.slashCommandSuggestions("/")} {
		for _, suggestion := range suggestions {
			switch suggestion.Command {
			case "/goal", "/dod", "/workspace create":
				t.Errorf("仍补全退休写操作: %+v", suggestion)
			}
		}
	}
}
