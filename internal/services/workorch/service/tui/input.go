package tui

import (
	"fmt"
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
	m.resetSlashSuggestionCursor()
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
	m.resetSlashSuggestionCursor()
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
	m.resetSlashSuggestionCursor()
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
	m.resetSlashSuggestionCursor()
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
	if m.inputPreviewShouldCollapse() {
		return m.renderCollapsedInputBuffer(prefix, width)
	}
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

func (m AppModel) inputPreviewShouldCollapse() bool {
	normalized := strings.ReplaceAll(m.input, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) <= 1 {
		return false
	}
	runeCount := utf8.RuneCountInString(normalized)
	return len(lines) > 6 || runeCount > 800
}

func (m AppModel) renderCollapsedInputBuffer(prefix string, width int) string {
	normalized := strings.ReplaceAll(m.input, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	runeCount := utf8.RuneCountInString(normalized)
	cursor := m.inputCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > runeCount {
		cursor = runeCount
	}
	summary := fmt.Sprintf("[folded input: %d lines, %d chars]", len(lines), runeCount)
	if cursor != runeCount {
		summary += fmt.Sprintf(" [cursor %d/%d]", cursor, runeCount)
	}
	head := firstNonEmptyLine(lines)
	if head == "" {
		head = "(blank)"
	}
	tail := lastNonEmptyLine(lines)
	if tail == "" {
		tail = "(blank)"
	}
	if cursor == runeCount {
		tail += "|"
	}
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	return strings.Join([]string{
		truncateDisplay(prefix+summary, width-2),
		truncateDisplay(indent+"head: "+head, width-2),
		truncateDisplay(indent+"tail: "+tail, width-2),
	}, "\n")
}

func firstNonEmptyLine(lines []string) string {
	for _, line := range lines {
		if value := strings.TrimSpace(line); value != "" {
			return value
		}
	}
	return ""
}

func lastNonEmptyLine(lines []string) string {
	for idx := len(lines) - 1; idx >= 0; idx-- {
		if value := strings.TrimSpace(lines[idx]); value != "" {
			return value
		}
	}
	return ""
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
	m.resetSlashSuggestionCursor()
}

func (m *AppModel) completeSlashCommand() {
	trimmedLeft := strings.TrimLeftFunc(m.input, unicode.IsSpace)
	if !strings.HasPrefix(trimmedLeft, "/") {
		return
	}
	matches := m.slashCommandSuggestions(m.input)
	if len(matches) > 0 {
		selected := m.clampedSlashSuggestionCursor(len(matches))
		insert := matches[selected].Insert
		if insert == "" {
			insert = matches[selected].Command
		}
		m.setInput(insert)
	}
}

func (m AppModel) renderSlashCommandSuggestions(width int) string {
	trimmedLeft := strings.TrimLeftFunc(m.input, unicode.IsSpace)
	if !strings.HasPrefix(trimmedLeft, "/") {
		return ""
	}
	matches := m.slashCommandSuggestions(m.input)
	if len(matches) == 0 {
		return ""
	}
	limit := min(4, len(matches))
	selected := m.clampedSlashSuggestionCursor(len(matches))
	start := 0
	if selected >= limit {
		start = selected - limit + 1
	}
	end := min(len(matches), start+limit)
	lines := make([]string, 0, limit+1)
	lines = append(lines, Dim.Render("Commands:"))
	for idx := start; idx < end; idx++ {
		suggestion := matches[idx]
		line := "  " + suggestion.Command
		if suggestion.Description != "" {
			line += "  " + Dim.Render(suggestion.Description)
		}
		if idx == selected {
			selected := "  " + suggestion.Command
			if suggestion.Description != "" {
				selected += "  " + suggestion.Description
			}
			line = Selection.Render(selected + "  ")
		}
		lines = append(lines, truncateDisplay(line, width-2))
	}
	return strings.Join(lines, "\n")
}

func (m *AppModel) resetSlashSuggestionCursor() {
	m.slashCursor = 0
}

func (m *AppModel) moveSlashSuggestionCursor(delta int) bool {
	matches := m.slashCommandSuggestions(m.input)
	if len(matches) == 0 {
		m.resetSlashSuggestionCursor()
		return false
	}
	m.slashCursor += delta
	if m.slashCursor < 0 {
		m.slashCursor = len(matches) - 1
	}
	if m.slashCursor >= len(matches) {
		m.slashCursor = 0
	}
	return true
}

func (m AppModel) slashCommandSuggestionActive() bool {
	return len(m.slashCommandSuggestions(m.input)) > 0
}

func (m AppModel) clampedSlashSuggestionCursor(length int) int {
	if length <= 0 || m.slashCursor < 0 {
		return 0
	}
	if m.slashCursor >= length {
		return length - 1
	}
	return m.slashCursor
}

type slashCommandSuggestion struct {
	Command     string
	Insert      string
	Description string
	Contextual  bool
}

func (m AppModel) slashCommandSuggestions(input string) []slashCommandSuggestion {
	return slashCommandSuggestionsForInput(input, m.contextualSlashCommandSuggestions())
}

func slashCommandMatches(input string) []string {
	suggestions := slashCommandSuggestionsForInput(input, nil)
	ret := make([]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		ret = append(ret, suggestion.Command)
	}
	return ret
}

func slashCommandSuggestionsForInput(input string, contextual []slashCommandSuggestion) []slashCommandSuggestion {
	trimmedLeft := strings.TrimLeftFunc(input, unicode.IsSpace)
	if !strings.HasPrefix(trimmedLeft, "/") {
		return nil
	}
	prefix := normalizeSlashCommandPrefix(trimmedLeft)
	commands := append([]slashCommandSuggestion{}, contextual...)
	commands = append(commands, baseSlashCommandSuggestions()...)
	seen := map[string]bool{}
	matches := make([]slashCommandSuggestion, 0, len(commands))
	for _, command := range commands {
		key := strings.TrimSpace(command.Command)
		if key == "" || seen[key] {
			continue
		}
		commandKey := strings.ToLower(strings.TrimPrefix(key, "/"))
		if prefix == "" || strings.HasPrefix(commandKey, prefix) {
			seen[key] = true
			if command.Insert == "" {
				command.Insert = key
			}
			command.Command = key
			matches = append(matches, command)
		}
	}
	return matches
}

func normalizeSlashCommandPrefix(input string) string {
	input = strings.TrimPrefix(input, "/")
	hasTrailingSpace := strings.HasSuffix(input, " ") || strings.HasSuffix(input, "\t")
	prefix := strings.ToLower(strings.Join(strings.Fields(input), " "))
	if hasTrailingSpace && prefix != "" {
		prefix += " "
	}
	return prefix
}

func baseSlashCommandSuggestions() []slashCommandSuggestion {
	return []slashCommandSuggestion{
		{Command: "/help", Description: "show help"},
		{Command: "/clear", Description: "clear current display"},
		{Command: "/config", Description: "show system configuration"},
		{Command: "/model", Description: "select workspace provider/model"},
		{Command: "/model show", Description: "show workspace model selection"},
		{Command: "/model clear", Description: "inherit system default model"},
		{Command: "/model reasoning", Insert: "/model reasoning ", Description: "set next-run reasoning effort"},
		{Command: "/workspace", Description: "select workspace"},
		{Command: "/workspace create", Insert: "/workspace create ", Description: "create workspace"},
		{Command: "/goal", Insert: "/goal ", Description: "submit a new goal"},
		{Command: "/dod", Insert: "/dod ", Description: "set DoD items"},
		{Command: "/status", Description: "show workspace and run status"},
		{Command: "/coverage", Description: "show DoD coverage progress"},
		{Command: "/steps", Description: "show recent step summary"},
		{Command: "/timeline", Description: "show runtime timeline"},
		{Command: "/patches", Description: "show patch evidence"},
		{Command: "/artifacts", Description: "show artifacts"},
		{Command: "/memory", Description: "show memory recall"},
		{Command: "/result", Description: "refresh result"},
		{Command: "/run resume", Description: "continue workspace execution"},
		{Command: "/run restart", Description: "start a fresh workspace activation"},
		{Command: "/exit", Description: "quit TUI"},
	}
}

func (m AppModel) contextualSlashCommandSuggestions() []slashCommandSuggestion {
	switch m.activeWorkspacePromptMode() {
	case workspacePromptModeDecision:
		return []slashCommandSuggestion{
			{Command: "/decision answer", Insert: "/decision answer ", Description: "answer current ticket", Contextual: true},
			{Command: "/decision terminate", Insert: "/decision terminate ", Description: "terminate current path", Contextual: true},
			{Command: "/decision show", Description: "show current ticket", Contextual: true},
			{Command: "/decision list", Description: "list open tickets", Contextual: true},
		}
	case workspacePromptModeApproval:
		return []slashCommandSuggestion{
			{Command: "/approval approve", Description: "approve current request", Contextual: true},
			{Command: "/approval reject", Description: "reject current request", Contextual: true},
			{Command: "/approval show", Description: "show current approval", Contextual: true},
			{Command: "/approval list", Description: "list approvals", Contextual: true},
		}
	case workspacePromptModeRunning:
		return []slashCommandSuggestion{
			{Command: "/run resume", Description: "continue workspace execution", Contextual: true},
			{Command: "/run restart", Description: "start a fresh workspace activation", Contextual: true},
			{Command: "/status", Description: "show workspace and run status", Contextual: true},
			{Command: "/coverage", Description: "show DoD coverage progress", Contextual: true},
			{Command: "/steps", Description: "show recent step summary", Contextual: true},
			{Command: "/timeline", Description: "show runtime timeline", Contextual: true},
			{Command: "/patches", Description: "show patch evidence", Contextual: true},
			{Command: "/artifacts", Description: "show artifacts", Contextual: true},
			{Command: "/result", Description: "refresh result", Contextual: true},
			{Command: "/memory", Description: "show memory recall", Contextual: true},
		}
	default:
		if m.activeWorkspaceID == "" {
			return []slashCommandSuggestion{
				{Command: "/workspace create", Insert: "/workspace create ", Description: "create workspace", Contextual: true},
				{Command: "/workspace", Description: "select workspace", Contextual: true},
			}
		}
		return nil
	}
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
		"resume": {}, "goon": {}, "proceed": {}, "keeprunning": {}, "resumerun": {}, "resumeexecution": {},
	}
	if _, ok := exact[normalized]; ok {
		return true
	}
	for _, prefix := range []string{"继续", "恢复", "接着"} {
		if strings.HasPrefix(normalized, prefix) && hasRunControlActionCue(strings.TrimPrefix(normalized, prefix)) {
			return true
		}
	}
	for _, prefix := range []string{"resume"} {
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
