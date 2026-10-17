package workorchmanagementcontract

import "fmt"

// DefinitionAuthoringCreationRecovery 是公开恢复提示，不授予创建／执行权威。
// unknown 不能被当作尚未创建；仍以原请求重放，对账后才继续。
type DefinitionAuthoringCreationRecovery struct {
	WorkspaceID      string `json:"workspaceId"`
	RequestID        string `json:"requestId"`
	Persistence      string `json:"persistence"`
	Phase            string `json:"phase"`
	RetrySameRequest bool   `json:"retrySameRequest"`
}

type DefinitionAuthoringCreationError struct {
	Recovery DefinitionAuthoringCreationRecovery
	Cause    error
}

func (e *DefinitionAuthoringCreationError) Error() string {
	return fmt.Sprintf("Workspace 创建尚未完整完成（%s）；请以原请求对账并重试", e.Recovery.Phase)
}

func (e *DefinitionAuthoringCreationError) Unwrap() error { return e.Cause }

// DefinitionAuthoringCreationRejectedBeforeDispatchError is emitted only before
// sending the creation command to the owner. HTTP status alone is not proof
// that persistence has not started.
type DefinitionAuthoringCreationRejectedBeforeDispatchError struct{ Cause error }

func (e *DefinitionAuthoringCreationRejectedBeforeDispatchError) Error() string {
	return e.Cause.Error()
}
func (e *DefinitionAuthoringCreationRejectedBeforeDispatchError) Unwrap() error { return e.Cause }
