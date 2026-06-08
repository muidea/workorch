package workorchclient

import (
	"net/url"
	"strings"
)

func escapePath(value string) string {
	return url.PathEscape(strings.TrimSpace(value))
}

func joinQuery(path string, values url.Values) string {
	encoded := values.Encode()
	if encoded == "" {
		return path
	}
	return path + "?" + encoded
}
