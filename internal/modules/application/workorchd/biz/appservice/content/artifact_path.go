package content

import (
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func SafeArtifactPath(root string, relativePath string) (string, error) {
	root = filepath.Clean(root)
	candidate := filepath.Join(root, filepath.Clean(relativePath))
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", common.ErrInvalidArgument
	}
	return candidate, nil
}
