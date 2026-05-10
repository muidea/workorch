package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

func (m *AppModel) insertInput(value string) {
	if value == "" {
		return
	}
	runes := []rune(m.input)
	m.clampInputCursor(len(runes))
	inserted := []rune(value)
	next := make([]rune, 0, len(runes)+len(inserted))
	next = append(next, runes[:m.inputCursor]...)
	next = append(next, inserted...)
	next = append(next, runes[m.inputCursor:]...)
	m.input = string(next)
	m.inputCursor += len(inserted)
	m.historyCursor = -1
}

func (m *AppModel) deleteBeforeCursor() {
	runes := []rune(m.input)
	m.clampInputCursor(len(runes))
	if m.inputCursor == 0 {
		return
	}
	next := make([]rune, 0, len(runes)-1)
	next = append(next, runes[:m.inputCursor-1]...)
	next = append(next, runes[m.inputCursor:]...)
	m.input = string(next)
	m.inputCursor--
}

func (m *AppModel) deleteAtCursor() {
	runes := []rune(m.input)
	m.clampInputCursor(len(runes))
	if m.inputCursor >= len(runes) {
		return
	}
	next := make([]rune, 0, len(runes)-1)
	next = append(next, runes[:m.inputCursor]...)
	next = append(next, runes[m.inputCursor+1:]...)
	m.input = string(next)
}

func (m *AppModel) moveInputCursor(delta int) {
	runes := []rune(m.input)
	m.inputCursor += delta
	m.clampInputCursor(len(runes))
}

func (m *AppModel) clearInput() {
	m.input = ""
	m.inputCursor = 0
	m.historyCursor = -1
}

func (m *AppModel) clampInputCursor(length int) {
	if m.inputCursor < 0 {
		m.inputCursor = 0
	}
	if m.inputCursor > length {
		m.inputCursor = length
	}
}

func (m AppModel) renderEditableInput() string {
	runes := []rune(m.input)
	cursor := m.inputCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	return string(runes[:cursor]) + "|" + string(runes[cursor:])
}

func (m AppModel) renderInputBuffer(prompt string, width int) string {
	prefix := prompt + " > "
	editable := strings.ReplaceAll(m.renderEditableInput(), "\r\n", "\n")
	lines := strings.Split(editable, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	limit := 4
	if len(lines) > limit {
		lines = append([]string{"..."}, lines[len(lines)-limit+1:]...)
	}
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	out := make([]string, 0, len(lines))
	for idx, line := range lines {
		linePrefix := indent
		if idx == 0 {
			linePrefix = prefix
		}
		out = append(out, truncateDisplay(linePrefix+line, width-2))
	}
	return strings.Join(out, "\n")
}

func (m *AppModel) addInputHistory(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if len(m.inputHistory) > 0 && m.inputHistory[len(m.inputHistory)-1] == value {
		m.historyCursor = -1
		return
	}
	m.inputHistory = append(m.inputHistory, value)
	if len(m.inputHistory) > maxInputHistory {
		m.inputHistory = append([]string(nil), m.inputHistory[len(m.inputHistory)-maxInputHistory:]...)
	}
	m.historyCursor = -1
}

func (m *AppModel) previousInputHistory() {
	if len(m.inputHistory) == 0 {
		return
	}
	if m.historyCursor < 0 {
		m.historyCursor = len(m.inputHistory) - 1
	} else if m.historyCursor > 0 {
		m.historyCursor--
	}
	m.setInput(m.inputHistory[m.historyCursor])
}

func (m *AppModel) nextInputHistory() {
	if len(m.inputHistory) == 0 || m.historyCursor < 0 {
		return
	}
	if m.historyCursor >= len(m.inputHistory)-1 {
		m.historyCursor = -1
		m.setInput("")
		return
	}
	m.historyCursor++
	m.setInput(m.inputHistory[m.historyCursor])
}

func (m *AppModel) setInput(value string) {
	m.input = value
	m.inputCursor = len([]rune(value))
}

func (m *AppModel) completeSlashCommand() {
	trimmed := strings.TrimSpace(m.input)
	if !strings.HasPrefix(trimmed, "/") {
		return
	}
	matches := slashCommandMatches(trimmed)
	if len(matches) > 0 {
		m.setInput(matches[0])
	}
}

func (m AppModel) renderSlashCommandSuggestions(width int) string {
	trimmed := strings.TrimSpace(m.input)
	if !strings.HasPrefix(trimmed, "/") {
		return ""
	}
	matches := slashCommandMatches(trimmed)
	if len(matches) == 0 {
		return ""
	}
	limit := min(4, len(matches))
	lines := make([]string, 0, limit+1)
	lines = append(lines, Dim.Render("Commands:"))
	for idx, command := range matches[:limit] {
		line := "  " + command
		if idx == 0 {
			line = Selection.Render("  " + command + "  ")
		}
		lines = append(lines, truncateDisplay(line, width-2))
	}
	return strings.Join(lines, "\n")
}

func slashCommandMatches(input string) []string {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}
	prefix := strings.ToLower(strings.TrimPrefix(trimmed, "/"))
	commands := []string{"/help", "/clear", "/workspace", "/workspace create", "/goal", "/dod", "/memory", "/result", "/continue", "/exit"}
	matches := make([]string, 0, len(commands))
	for _, command := range commands {
		if strings.HasPrefix(strings.TrimPrefix(command, "/"), prefix) {
			matches = append(matches, command)
		}
	}
	return matches
}

func isRunControlIntent(input string) bool {
	normalized := normalizeRunControlInput(input)
	if normalized == "" || utf8.RuneCountInString(normalized) > 24 {
		return false
	}
	exact := map[string]struct{}{
		"继续": {}, "继续处理": {}, "继续执行": {}, "继续推进": {}, "继续收口": {}, "继续修复": {}, "继续运行": {}, "继续工作": {},
		"恢复": {}, "恢复执行": {}, "恢复运行": {}, "恢复处理": {},
		"接着处理": {}, "接着执行": {}, "接着推进": {}, "接着修复": {},
		"continue": {}, "resume": {}, "goon": {}, "proceed": {}, "keeprunning": {}, "continuerun": {}, "resumerun": {}, "resumeexecution": {}, "continueexecution": {},
	}
	if _, ok := exact[normalized]; ok {
		return true
	}
	for _, prefix := range []string{"继续", "恢复", "接着"} {
		if strings.HasPrefix(normalized, prefix) && hasRunControlActionCue(strings.TrimPrefix(normalized, prefix)) {
			return true
		}
	}
	for _, prefix := range []string{"continue", "resume"} {
		if strings.HasPrefix(normalized, prefix) && hasASCIIRunControlCue(strings.TrimPrefix(normalized, prefix)) {
			return true
		}
	}
	return false
}

func normalizeRunControlInput(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var b strings.Builder
	for _, r := range input {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r):
			b.WriteRune(r)
		case r >= '\u4e00' && r <= '\u9fff':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasRunControlActionCue(value string) bool {
	if value == "" {
		return true
	}
	cues := []string{"处理", "执行", "推进", "收口", "修复", "运行", "工作", "做", "跑", "走", "弄", "剩余", "后续", "当前", "任务"}
	for _, cue := range cues {
		if strings.Contains(value, cue) {
			return true
		}
	}
	return false
}

func hasASCIIRunControlCue(value string) bool {
	if value == "" {
		return true
	}
	cues := []string{"run", "execution", "process", "work", "task", "fix", "repair", "remaining", "current", "next"}
	for _, cue := range cues {
		if strings.Contains(value, cue) {
			return true
		}
	}
	return false
}
