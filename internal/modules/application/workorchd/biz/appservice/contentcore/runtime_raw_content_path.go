package contentcore

import (
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func SafeRuntimeRawContentPath(root string, fileName string) (string, error) {
	root = filepath.Clean(root)
	candidate := filepath.Join(root, filepath.Clean(fileName))
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.Contains(rel, string(filepath.Separator)) || strings.HasPrefix(rel, "..") {
		return "", common.ErrInvalidArgument
	}
	return candidate, nil
}
