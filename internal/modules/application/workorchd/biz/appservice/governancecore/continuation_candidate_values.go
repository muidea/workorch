package governancecore

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func PreferredContinuationCandidate(value workorcheventpayload.Value) string {
	switch value.Kind {
	case workorcheventpayload.ValueArray:
		for _, item := range value.Array {
			if candidate := PreferredContinuationCandidate(item); candidate != "" {
				return candidate
			}
		}
	case workorcheventpayload.ValueObject:
		object := workorcheventpayload.Payload{Fields: value.Object}
		for _, key := range []string{"value", "id", "mode", "target", "name"} {
			if nested, ok := object.Field(key); ok {
				if candidate := PreferredContinuationCandidate(nested); candidate != "" {
					return candidate
				}
			}
		}
		for _, field := range value.Object {
			if candidate := PreferredContinuationCandidate(field.Value); candidate != "" {
				return candidate
			}
		}
	case workorcheventpayload.ValueString:
		if IsContinuationCandidate(value.String) {
			return strings.TrimSpace(value.String)
		}
	}
	return ""
}

func IsContinuationCandidate(value string) bool {
	normalized := NormalizeRepairResolutionText(value)
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"init", "initialize", "bootstrap", "fromscratch", "newscratch", "newproject",
		"currentworkspace", "currentdirectory",
		"初始化", "从零", "新项目", "当前工作区", "当前目录",
	} {
		if strings.Contains(normalized, NormalizeRepairResolutionText(marker)) {
			return true
		}
	}
	return false
}
