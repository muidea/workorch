package tui

import (
	"os"
	"strings"
	"unicode"

	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
	glamourstyles "github.com/charmbracelet/glamour/styles"
)

const (
	markdownRendererEnv      = "WORKORCH_TUI_MARKDOWN"
	markdownRendererStyleEnv = "WORKORCH_TUI_MARKDOWN_STYLE"
)

func useGlamourMarkdownRenderer() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv(markdownRendererEnv)), "plain")
}

func renderGlamourRichText(value string, width int, maxLines int) (string, bool) {
	if width <= 0 || maxLines <= 0 {
		return "", true
	}
	options := []glamour.TermRendererOption{
		glamour.WithPreservedNewLines(),
		glamour.WithWordWrap(max(20, width)),
	}
	if styleName := glamourStyleName(); styleName != "" {
		if styleName == "dark" || styleName == "light" {
			options = append(options, glamour.WithStyles(glamourWorkorchStyle(styleName)))
		} else {
			options = append(options, glamour.WithStandardStyle(styleName))
		}
	}
	renderer, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return "", false
	}
	rendered, err := renderer.Render(value)
	if err != nil {
		return "", false
	}
	rendered = strings.TrimRight(strings.ReplaceAll(rendered, "\r\n", "\n"), "\n")
	if rendered == "" {
		return "", true
	}
	lines := compactGlamourLines(value, strings.Split(rendered, "\n"))
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n"), true
}

func glamourStyleName() string {
	if style := strings.TrimSpace(os.Getenv(markdownRendererStyleEnv)); style != "" {
		return style
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("WORKORCH_THEME")), "light") {
		return "light"
	}
	return "dark"
}

func glamourWorkorchStyle(name string) glamouransi.StyleConfig {
	base := glamourstyles.DarkStyleConfig
	if strings.EqualFold(name, "light") {
		base = glamourstyles.LightStyleConfig
	}
	base.Document.BlockPrefix = ""
	base.Document.BlockSuffix = ""
	base.Document.Margin = uintPtr(0)
	base.Paragraph.BlockPrefix = ""
	base.Paragraph.BlockSuffix = ""
	base.BlockQuote.BlockPrefix = ""
	base.BlockQuote.BlockSuffix = ""
	base.Heading.BlockPrefix = ""
	base.Heading.BlockSuffix = ""
	base.Heading.Margin = uintPtr(0)
	base.H1.Prefix = ""
	base.H1.Suffix = ""
	base.H2.Prefix = ""
	base.H3.Prefix = ""
	base.H4.Prefix = ""
	base.H5.Prefix = ""
	base.H6.Prefix = ""
	base.Table.Margin = uintPtr(0)
	return base
}

func uintPtr(v uint) *uint { return &v }

func compactGlamourLines(source string, lines []string) []string {
	headings := markdownHeadingSet(source)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(out) == 0 {
				continue
			}
			prev := strings.TrimSpace(stripANSICodes(out[len(out)-1]))
			if prev == "" || headings[prev] {
				continue
			}
			out = append(out, "")
			continue
		}
		out = append(out, trimRightDisplaySpace(line))
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func markdownHeadingSet(source string) map[string]bool {
	headings := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
		if _, title, ok := markdownHeading(strings.TrimSpace(line)); ok {
			headings[title] = true
		}
	}
	return headings
}

func stripANSICodes(value string) string {
	var out strings.Builder
	inEscape := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if inEscape {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				inEscape = false
			}
			continue
		}
		if ch == 0x1b {
			inEscape = true
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func trimRightDisplaySpace(value string) string {
	suffix := ""
	value = strings.TrimRightFunc(value, unicode.IsSpace)
	for {
		start := strings.LastIndex(value, "\x1b")
		if start < 0 || start == len(value)-1 {
			break
		}
		seq := value[start:]
		if !isANSISequence(seq) {
			break
		}
		suffix = seq + suffix
		value = value[:start]
		value = strings.TrimRightFunc(value, unicode.IsSpace)
	}
	return strings.TrimRightFunc(value, unicode.IsSpace) + suffix
}

func isANSISequence(value string) bool {
	if len(value) < 3 || value[0] != 0x1b || value[1] != '[' {
		return false
	}
	last := value[len(value)-1]
	return (last >= 'A' && last <= 'Z') || (last >= 'a' && last <= 'z')
}
