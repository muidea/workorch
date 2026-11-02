package tui

import "time"

// Format the display only; owner timestamps and diagnostic text remain intact.
func localDateTime(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.Local().Format("2006-01-02 15:04:05 -07:00")
}

func activityTimeLabel(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.Local().Format("15:04")
}
