package workorcheventpayload

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func canonicalDeepCopyReference(payload Payload) string {
	var builder strings.Builder
	var fields func([]Field)
	var value func(Value)
	fields = func(input []Field) {
		input = cloneFields(input)
		sort.Slice(input, func(i, j int) bool { return input[i].Name < input[j].Name })
		for _, field := range input {
			// 标量编码沿用既有实现；递归排序独立保留原深拷贝路径。
			builder.WriteString(strconv.Quote(field.Name))
			builder.WriteByte(':')
			value(field.Value)
			builder.WriteByte(';')
		}
	}
	value = func(input Value) {
		switch input.Kind {
		case ValueObject:
			builder.WriteString(string(input.Kind) + "={")
			fields(input.Object)
			builder.WriteByte('}')
		case ValueArray:
			builder.WriteString(string(input.Kind) + "=[")
			for _, item := range input.Array {
				value(item)
				builder.WriteByte(',')
			}
			builder.WriteByte(']')
		default:
			appendCanonicalValue(&builder, input)
		}
	}
	fields(payload.Fields)
	return builder.String()
}

func canonicalNestedFixture(depth int) Payload {
	payload := Payload{Fields: []Field{StringField("z", "中文\n原文"), BoolField("a", true)}}
	for i := 0; i < depth; i++ {
		payload = Payload{Fields: []Field{
			{Name: "z", Value: Value{Kind: ValueArray, Array: []Value{{Kind: ValueObject, Object: payload.Fields}, {Kind: ValueInt, Int: -7}}}},
			StringField("a", "锚点"),
		}}
	}
	return payload
}

func TestCanonicalStringPreservesDeepCopyReference(t *testing.T) {
	for _, payload := range []Payload{
		{}, canonicalNestedFixture(12),
		{Fields: []Field{
			{Name: "null", Value: Value{Kind: ValueNull}},
			{Name: "uint", Value: Value{Kind: ValueUint, Uint: 42}},
			{Name: "float", Value: Value{Kind: ValueFloat, Float: 1.25}},
			{Name: "array", Value: Value{Kind: ValueArray}},
		}},
		{Fields: []Field{StringField("same", "first"), StringField("same", "second"), {Name: "empty", Value: Value{Kind: ValueObject}}}},
	} {
		before := payload.Clone()
		want := canonicalDeepCopyReference(payload)
		if got := payload.CanonicalString(); got != want {
			t.Fatalf("canonical 编码改变: got=%q want=%q", got, want)
		}
		if !reflect.DeepEqual(before, payload) {
			t.Fatal("canonical 编码修改来源")
		}
		var workers sync.WaitGroup
		for i := 0; i < 4; i++ {
			workers.Go(func() {
				if payload.CanonicalString() != want {
					t.Error("并发只读 canonical 结果改变")
				}
			})
		}
		workers.Wait()
		if !payload.Equal(before) {
			t.Fatal("本层排序副本改变嵌套比较结果")
		}
		if len(payload.Fields) > 0 {
			changed := payload.Clone()
			changed.Fields[0].Value = Value{Kind: ValueString, String: "后续原文"}
			if changed.CanonicalString() != canonicalDeepCopyReference(changed) || changed.Equal(payload) {
				t.Fatal("前次编码掩盖后续来源变化")
			}
		}
	}
}

func FuzzCanonicalStringPreservesReference(f *testing.F) {
	f.Add("中文\n原文", "z", "a", uint8(3))
	f.Add("\x00\xff", "same", "same", uint8(0))
	f.Fuzz(func(t *testing.T, text, first, second string, depth uint8) {
		payload := canonicalNestedFixture(int(depth % 16))
		payload.Fields = append(payload.Fields, StringField(first, text), StringField(second, text))
		before := payload.Clone()
		if payload.CanonicalString() != canonicalDeepCopyReference(payload) || !reflect.DeepEqual(before, payload) {
			t.Fatal("canonical 编码与独立 reference 不一致或修改来源")
		}
	})
}

func BenchmarkCanonicalNestedPayload(b *testing.B) {
	payload := canonicalNestedFixture(12)
	for _, mode := range []string{"deep_copy_reference", "canonical"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if mode == "deep_copy_reference" {
					_ = canonicalDeepCopyReference(payload)
				} else {
					_ = payload.CanonicalString()
				}
			}
		})
	}
}
