package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// StableWorkspaceTemplateContentHash fingerprints the complete owner content,
// not an application projection or an admitted Workspace startup snapshot.
func StableWorkspaceTemplateContentHash(item *WorkspaceTemplate) string {
	if item == nil {
		return ""
	}
	data, err := json.Marshal(item)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(data)
	return "workspace-template-content-sha256:" + hex.EncodeToString(hash[:])
}
