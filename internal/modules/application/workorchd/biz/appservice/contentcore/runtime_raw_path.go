package contentcore

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimepath"
)

func RuntimeRawItemRoot(collectionRoot string, runID common.RunID, itemPath string) (string, error) {
	collectionRoot = filepath.Clean(strings.TrimSpace(collectionRoot))
	itemPath = strings.Trim(strings.TrimSpace(itemPath), "/")
	if collectionRoot == "" || collectionRoot == "." || itemPath == "" {
		return "", common.ErrInvalidArgument
	}
	direct := filepath.Join(collectionRoot, filepath.FromSlash(itemPath))
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	sequenced := runtimepath.SequenceScopedRoot(collectionRoot, runtimepath.SequenceScopedPath(collectionRoot, string(runID), itemPath))
	if _, err := os.Stat(sequenced); err == nil {
		return sequenced, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return "", common.ErrNotFound
}
