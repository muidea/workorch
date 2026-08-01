package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

func runtimeFileWriteParameters(content string) []runtimecontract.OperationParameter {
	value := content
	return []runtimecontract.OperationParameter{{Name: "content", String: &value}}
}
