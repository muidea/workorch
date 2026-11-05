package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAppendRuntimeStateWriteExactBytesAndFailureRollback(t *testing.T) {
	for _, state := range []*rc.RuntimeStateRecord{nil, {}, {ProcessedEventIDs: []string{}}, {ProcessedEventIDs: []string{" <&\n中文\u2028\xff> "}}} {
		item := &Run{RuntimeState: state}
		want, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		out.WriteString("original-prefix:")
		identity, err := AppendRuntimeStateWrite(&out, item)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out.Bytes(), append([]byte("original-prefix:"), want...)) {
			t.Fatal("physical JSON bytes changed")
		}
		oracle, err := RuntimeStateWriteHash(item)
		if err != nil || identity != oracle {
			t.Fatal("physical identity differs", err)
		}
	}
	for _, arm := range []string{"wrong-fence", "unencodable", "nil-state-fence", "nil-state-hash", "nil-item"} {
		t.Run(arm, func(t *testing.T) {
			item := &Run{RuntimeState: &rc.RuntimeStateRecord{}}
			wantErr := ErrRuntimeStateIntegrity
			switch arm {
			case "wrong-fence":
				item.RuntimeLifecycleFence = &rc.RuntimeOpaqueLifecycleFence{RuntimeStateHash: "wrong"}
			case "unencodable":
				item.RuntimeState.Facts.Activation.Memories = []rc.RuntimeMemoryContext{{VectorScore: math.Inf(1)}}
			case "nil-state-fence":
				item.RuntimeState = nil
				item.RuntimeLifecycleFence = &rc.RuntimeOpaqueLifecycleFence{}
			case "nil-state-hash":
				item.RuntimeState = nil
				item.RuntimeStateHash = "not-empty"
			case "nil-item":
				item = nil
				wantErr = common.ErrInvalidArgument
			}
			var out bytes.Buffer
			out.WriteString("original-prefix:")
			if digest, err := AppendRuntimeStateWrite(&out, item); digest != "" || !errors.Is(err, wantErr) {
				t.Fatalf("identity=%q err=%v", digest, err)
			}
			if out.String() != "original-prefix:" {
				t.Fatal("failed encoding retained partial state")
			}
		})
	}
	if _, err := AppendRuntimeStateWrite(nil, &Run{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatal(err)
	}
}
