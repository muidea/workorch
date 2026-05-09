package doditems

import (
	"regexp"
	"strings"
	"unicode"
)

type listContext struct {
	indent int
	text   string
}

var (
	headingPattern     = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	orderedListPattern = regexp.MustCompile(`^\d+[\.)、]\s+`)
	ordinalPattern     = regexp.MustCompile(`^\d+(?:\.\d+)*(?:[\.)、])?\s+`)
)

// Normalize converts free-form DoD text into executable acceptance items.
// It preserves Markdown section and parent-list context while dropping
// structural-only lines such as headings or checklist group labels.
func Normalize(items []string) []string {
	lines := splitInput(items)
	headings := make([]string, 0, 6)
	listContexts := make([]listContext, 0, 8)
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(lines))

	for _, line := range lines {
		indent := leadingIndent(line)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isHorizontalRule(trimmed) {
			continue
		}

		if level, text, ok := parseHeading(trimmed); ok {
			text = stripOrdinal(cleanInlineMarkdown(text))
			if text == "" {
				continue
			}
			for len(headings) < level {
				headings = append(headings, "")
			}
			headings[level-1] = text
			headings = headings[:level]
			listContexts = listContexts[:0]
			continue
		}

		content, isListItem := stripListMarker(trimmed)
		content = cleanInlineMarkdown(content)
		if content == "" {
			continue
		}

		listContexts = trimListContexts(listContexts, indent, isSameLevelContinuation(isListItem, content))
		if isListItem && isParentOnly(content) {
			listContexts = append(listContexts, listContext{indent: indent, text: trimTrailingColon(content)})
			continue
		}

		full := withContext(activeContext(headings, listContexts), content)
		if full == "" {
			continue
		}
		if _, ok := seen[full]; ok {
			continue
		}
		seen[full] = struct{}{}
		ret = append(ret, full)
	}

	return ret
}

func splitInput(items []string) []string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ReplaceAll(item, "\r\n", "\n")
		item = strings.ReplaceAll(item, "\r", "\n")
		for _, line := range strings.Split(item, "\n") {
			if shouldSplitInline(line) {
				for _, part := range strings.FieldsFunc(line, func(r rune) bool {
					return r == ';' || r == '；'
				}) {
					lines = append(lines, part)
				}
				continue
			}
			lines = append(lines, line)
		}
	}
	return lines
}

func shouldSplitInline(line string) bool {
	if !strings.ContainsAny(line, ";；") {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if headingPattern.MatchString(trimmed) {
		return false
	}
	if _, ok := stripListMarker(trimmed); ok {
		return false
	}
	return true
}

func parseHeading(line string) (int, string, bool) {
	match := headingPattern.FindStringSubmatch(line)
	if len(match) != 3 {
		return 0, "", false
	}
	return len(match[1]), strings.TrimSpace(match[2]), true
}

func stripListMarker(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") || strings.HasPrefix(line, "• ") {
		return stripCheckbox(strings.TrimSpace(line[2:])), true
	}
	if orderedListPattern.MatchString(line) {
		return stripCheckbox(strings.TrimSpace(orderedListPattern.ReplaceAllString(line, ""))), true
	}
	if strings.HasPrefix(line, "[ ]") || strings.HasPrefix(strings.ToLower(line), "[x]") {
		return stripCheckbox(line), true
	}
	return line, false
}

func stripCheckbox(line string) string {
	line = strings.TrimSpace(line)
	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(line, "[ ]"):
		return strings.TrimSpace(line[3:])
	case strings.HasPrefix(lower, "[x]"):
		return strings.TrimSpace(line[3:])
	default:
		return line
	}
}

func cleanInlineMarkdown(text string) string {
	text = strings.TrimSpace(text)
	replacer := strings.NewReplacer("**", "", "__", "", "`", "")
	text = replacer.Replace(text)
	text = strings.TrimSpace(text)
	text = strings.Trim(text, " \t")
	return strings.Join(strings.Fields(text), " ")
}

func stripOrdinal(text string) string {
	return strings.TrimSpace(ordinalPattern.ReplaceAllString(text, ""))
}

func isParentOnly(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	return strings.HasSuffix(text, ":") || strings.HasSuffix(text, "：")
}

func trimTrailingColon(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimRightFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || r == ':' || r == '：'
	})
	return strings.TrimSpace(text)
}

func leadingIndent(line string) int {
	indent := 0
	for _, r := range line {
		switch r {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent
		}
	}
	return indent
}

func trimListContexts(contexts []listContext, indent int, keepSameLevel bool) []listContext {
	for len(contexts) > 0 {
		lastIndent := contexts[len(contexts)-1].indent
		if lastIndent > indent || (lastIndent == indent && !keepSameLevel) {
			contexts = contexts[:len(contexts)-1]
			continue
		}
		break
	}
	return contexts
}

func isSameLevelContinuation(isListItem bool, text string) bool {
	return isListItem && !isParentOnly(text) && !hasInlineLabel(text)
}

func hasInlineLabel(text string) bool {
	for idx, r := range text {
		if r != ':' && r != '：' {
			continue
		}
		if idx == 0 || idx == len(text)-1 {
			return false
		}
		label := strings.TrimSpace(text[:idx])
		return label != "" && len([]rune(label)) <= 40
	}
	return false
}

func activeContext(headings []string, listContexts []listContext) []string {
	ret := make([]string, 0, len(headings)+len(listContexts))
	for _, heading := range headings {
		if heading != "" {
			ret = append(ret, heading)
		}
	}
	for _, context := range listContexts {
		if context.text != "" {
			ret = append(ret, context.text)
		}
	}
	return ret
}

func withContext(context []string, item string) string {
	item = strings.TrimSpace(item)
	if item == "" {
		return ""
	}
	if len(context) == 0 {
		return item
	}
	prefix := strings.Join(context, " / ")
	if strings.HasPrefix(item, prefix) {
		return item
	}
	return prefix + "： " + item
}

func isHorizontalRule(line string) bool {
	if len(line) < 3 {
		return false
	}
	for _, r := range line {
		if r != '-' && r != '_' && r != '*' {
			return false
		}
	}
	return true
}
