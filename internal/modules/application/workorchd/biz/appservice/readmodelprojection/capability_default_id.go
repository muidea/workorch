package readmodelprojection

import (
	"net/url"
	"strings"
)

func defaultRuntimeCapabilityID(serverID string, kind string, name string) string {
	return strings.TrimSpace(serverID) + ":" + strings.TrimSpace(kind) + ":" + url.PathEscape(strings.TrimSpace(name))
}
