package file

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSkillListRejectsKeyIdentityMismatchWithoutRewritingOrPartialRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills.json")
	raw := []byte(`{"a-valid":{"ID":"a-valid"},"z-storage-key":{"ID":"different-record-id"}}`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		items, err := NewSkillRepository(path).List(context.Background())
		if !errors.Is(err, common.ErrConflict) || items != nil {
			t.Fatal("corrupt map became complete or partial catalog")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("read repaired or rewrote original bytes")
	}
}
