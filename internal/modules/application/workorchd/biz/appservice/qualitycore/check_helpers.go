package qualitycore

import "strings"

func (c *Check) hasError(code string) bool {
	for _, issue := range c.Issues {
		if issue.Code == code && issue.Severity == "error" {
			return true
		}
	}
	return false
}

func (c *Check) addIssue(code string, severity string, target string, message string, suggestion string) {
	c.Issues = append(c.Issues, Issue{
		Code:       code,
		Severity:   severity,
		Target:     target,
		Message:    message,
		Suggestion: suggestion,
	})
	if suggestion != "" {
		c.addSuggestion(suggestion)
	}
}

func (c *Check) addMissingConstraint(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range c.MissingConstraints {
		if existing == value {
			return
		}
	}
	c.MissingConstraints = append(c.MissingConstraints, value)
}

func (c *Check) addSuggestion(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range c.Suggestions {
		if existing == value {
			return
		}
	}
	c.Suggestions = append(c.Suggestions, value)
}
