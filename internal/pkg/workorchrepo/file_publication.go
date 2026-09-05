package workorchrepo

import (
	"fmt"
	"os"

	"github.com/muidea/workorch/internal/pkg/repository/filecommit"
)

type repositoryFileCommitFunc func(string, []byte, os.FileMode) (filecommit.Outcome, error)

func publishRepositoryFile(path string, data []byte, perm os.FileMode) error {
	return publishRepositoryFileWithCommit(path, data, perm, filecommit.Publish)
}

func publishRepositoryFileWithCommit(path string, data []byte, perm os.FileMode, commit repositoryFileCommitFunc) error {
	if commit == nil {
		return fmt.Errorf("workspace repository file commit is nil")
	}
	outcome, err := commit(path, data, perm)
	if err != nil {
		if outcome.Published {
			// The canonical path no longer contains the caller's old view. Keep
			// the publication marker in the error chain and require owner logic
			// to re-read its revision/head before any retry.
			return fmt.Errorf("workspace repository canonical file %s was published; reconcile before retry: %w", path, err)
		}
		return err
	}
	if !outcome.Published || !outcome.DurabilityAcknowledged {
		return fmt.Errorf("workspace repository file commit returned an incomplete success outcome for %s", path)
	}
	return nil
}
