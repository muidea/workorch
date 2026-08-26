package workorchdeletioncontract

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestScopeValidateRequiresCurrentExactRequestHash(t *testing.T) {
	scope := Scope{WorkspaceID: " ws-1 ", DeletionID: " deletion-1 ", DeletionGeneration: 1, Mode: ModePurge, QuiescenceID: " quiescence-1 "}
	scope.RequestHash = StableRequestHash(scope)
	if err := scope.Validate(); err != nil {
		t.Fatalf("validate scope: %v", err)
	}
	changed := scope
	changed.Mode = ModeRetain
	if err := changed.Validate(); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed mode error = %v, want conflict", err)
	}
	changed = scope
	changed.DeletionGeneration = 2
	changed.RequestHash = StableRequestHash(changed)
	if err := changed.Validate(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("generation error = %v, want invalid argument", err)
	}
}
