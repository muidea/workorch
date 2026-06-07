package workspacecore

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

func SafeRemovalPath(repositoryDataDir string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if abs == string(filepath.Separator) {
		return "", common.ErrInvalidArgument
	}
	home, _ := os.UserHomeDir()
	wd, _ := os.Getwd()
	protected := []string{home, wd}
	if strings.TrimSpace(repositoryDataDir) != "" {
		protected = append(protected, repositoryDataDir)
	}
	for _, item := range protected {
		if item == "" {
			continue
		}
		itemAbs, err := filepath.Abs(item)
		if err != nil {
			return "", err
		}
		if abs == filepath.Clean(itemAbs) {
			return "", common.ErrInvalidArgument
		}
	}
	return abs, nil
}
