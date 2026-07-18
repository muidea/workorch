package runtimev2

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

func runtimeV2FileWriteParameters(content string) []runtimecontract.OperationParameter {
	value := content
	return []runtimecontract.OperationParameter{{Name: "content", String: &value}}
}
