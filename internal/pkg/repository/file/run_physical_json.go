package file

import (
	"bytes"
	"encoding/json"
	"sort"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

//go:generate go run ../../../../scripts/generate-run-physical-json.go

// marshalRunRuntimeWrite keeps the physical representation identical to
// json.Marshal(items). Only the isolated state encoded and hashed in this CAS
// is inserted directly; other Runs retain their complete original values.
// There are no persistent references, semantic caches or cross-call buffers.
func marshalRunRuntimeWrite(items map[common.RunID]run.Run, written *run.Run) ([]byte, error) {
	e := runPhysicalJSONEncoder{frozen: written}
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)
	e.text("{")
	for i, key := range keys {
		if i != 0 {
			e.text(",")
		}
		e.value(key)
		e.text(":")
		if common.RunID(key) == written.ID {
			e.run(written)
		} else {
			other := items[common.RunID(key)]
			e.run(&other)
		}
	}
	e.text("}")
	if e.err != nil {
		return nil, e.err
	}
	return e.out.Bytes(), nil
}

type runPhysicalJSONEncoder struct {
	out    bytes.Buffer
	err    error
	frozen *run.Run
}

func (e *runPhysicalJSONEncoder) text(s string) {
	if e.err == nil {
		e.out.WriteString(s)
	}
}
func (e *runPhysicalJSONEncoder) value(v any) {
	if e.err != nil {
		return
	}
	body, err := json.Marshal(v)
	if err != nil {
		e.err = err
		return
	}
	e.out.Write(body)
}
func (e *runPhysicalJSONEncoder) state(v *run.Run) {
	if e.err != nil {
		return
	}
	if v == e.frozen {
		v.RuntimeStateHash, e.err = run.AppendRuntimeStateWrite(&e.out, v)
		return
	}
	e.value(v.RuntimeState)
}
