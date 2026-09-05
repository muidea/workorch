package workorchrepo

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/repository/filecommit"
)

func TestPublishRepositoryFilePreservesPostPublicationReconciliationSignal(t *testing.T) {
	wantErr := errors.New("parent directory sync failed")
	err := publishRepositoryFileWithCommit("state.json", []byte("new"), 0o644, func(path string, data []byte, perm os.FileMode) (filecommit.Outcome, error) {
		if path != "state.json" || string(data) != "new" || perm != 0o644 {
			t.Fatalf("commit input = path:%q data:%q perm:%o", path, data, perm)
		}
		return filecommit.Outcome{Published: true}, errors.Join(filecommit.ErrPublishedButDurabilityUnacknowledged, wantErr)
	})
	if !errors.Is(err, wantErr) || !errors.Is(err, filecommit.ErrPublishedButDurabilityUnacknowledged) {
		t.Fatalf("publish error = %v, want post-publication causes preserved", err)
	}
	if !strings.Contains(err.Error(), "reconcile before retry") {
		t.Fatalf("publish error = %q, want explicit re-read/reconciliation instruction", err)
	}
}

func TestPublishRepositoryFileDoesNotMarkPrePublicationFailure(t *testing.T) {
	wantErr := errors.New("temporary write failed")
	err := publishRepositoryFileWithCommit("state.json", []byte("new"), 0o644, func(string, []byte, os.FileMode) (filecommit.Outcome, error) {
		return filecommit.Outcome{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("publish error = %v, want %v", err, wantErr)
	}
	if strings.Contains(err.Error(), "reconcile before retry") || errors.Is(err, filecommit.ErrPublishedButDurabilityUnacknowledged) {
		t.Fatalf("pre-publication error was marked as published: %v", err)
	}
}
