package orchestrator

import (
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func fileListOperationResponse(result magicevent.Result) (orchestratorevents.ExecuteFileListOperationResult, error) {
	if result == nil {
		return orchestratorevents.ExecuteFileListOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return orchestratorevents.ExecuteFileListOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return orchestratorevents.ExecuteFileListOperationResult{}, err
	}
	response, ok := data.(orchestratorevents.ExecuteFileListOperationResult)
	if !ok {
		return orchestratorevents.ExecuteFileListOperationResult{}, fmt.Errorf("runtime file list returned invalid result type %T", data)
	}
	return response, nil
}
func fileReadOperationResponse(result magicevent.Result) (orchestratorevents.ExecuteFileReadOperationResult, error) {
	if result == nil {
		return orchestratorevents.ExecuteFileReadOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return orchestratorevents.ExecuteFileReadOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return orchestratorevents.ExecuteFileReadOperationResult{}, err
	}
	response, ok := data.(orchestratorevents.ExecuteFileReadOperationResult)
	if !ok {
		return orchestratorevents.ExecuteFileReadOperationResult{}, fmt.Errorf("runtime file read returned invalid result type %T", data)
	}
	return response, nil
}
func sshOperationResponse(result magicevent.Result) (orchestratorevents.ExecuteSSHOperationResult, error) {
	if result == nil {
		return orchestratorevents.ExecuteSSHOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return orchestratorevents.ExecuteSSHOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return orchestratorevents.ExecuteSSHOperationResult{}, err
	}
	response, ok := data.(orchestratorevents.ExecuteSSHOperationResult)
	if !ok {
		return orchestratorevents.ExecuteSSHOperationResult{}, fmt.Errorf("runtime ssh returned invalid result type %T", data)
	}
	return response, nil
}
func webOperationResponse(result magicevent.Result) (orchestratorevents.ExecuteWebOperationResult, error) {
	if result == nil {
		return orchestratorevents.ExecuteWebOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return orchestratorevents.ExecuteWebOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return orchestratorevents.ExecuteWebOperationResult{}, err
	}
	response, ok := data.(orchestratorevents.ExecuteWebOperationResult)
	if !ok {
		return orchestratorevents.ExecuteWebOperationResult{}, fmt.Errorf("runtime web returned invalid result type %T", data)
	}
	return response, nil
}
func mcpOperationResponse(result magicevent.Result) (orchestratorevents.ExecuteMCPOperationResult, error) {
	if result == nil {
		return orchestratorevents.ExecuteMCPOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return orchestratorevents.ExecuteMCPOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return orchestratorevents.ExecuteMCPOperationResult{}, err
	}
	response, ok := data.(orchestratorevents.ExecuteMCPOperationResult)
	if !ok {
		return orchestratorevents.ExecuteMCPOperationResult{}, fmt.Errorf("runtime mcp returned invalid result type %T", data)
	}
	return response, nil
}
func gitOperationResponse(result magicevent.Result) (orchestratorevents.ExecuteGitOperationResult, error) {
	if result == nil {
		return orchestratorevents.ExecuteGitOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return orchestratorevents.ExecuteGitOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return orchestratorevents.ExecuteGitOperationResult{}, err
	}
	response, ok := data.(orchestratorevents.ExecuteGitOperationResult)
	if !ok {
		return orchestratorevents.ExecuteGitOperationResult{}, fmt.Errorf("runtime git returned invalid result type %T", data)
	}
	return response, nil
}
func shellOperationResponse(result magicevent.Result) (orchestratorevents.ExecuteShellOperationResult, error) {
	if result == nil {
		return orchestratorevents.ExecuteShellOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return orchestratorevents.ExecuteShellOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return orchestratorevents.ExecuteShellOperationResult{}, err
	}
	response, ok := data.(orchestratorevents.ExecuteShellOperationResult)
	if !ok {
		return orchestratorevents.ExecuteShellOperationResult{}, fmt.Errorf("runtime shell returned invalid result type %T", data)
	}
	return response, nil
}
func fileEditOperationResponse(result magicevent.Result) (orchestratorevents.ExecuteFileEditOperationResult, error) {
	if result == nil {
		return orchestratorevents.ExecuteFileEditOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return orchestratorevents.ExecuteFileEditOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return orchestratorevents.ExecuteFileEditOperationResult{}, err
	}
	response, ok := data.(orchestratorevents.ExecuteFileEditOperationResult)
	if !ok {
		return orchestratorevents.ExecuteFileEditOperationResult{}, fmt.Errorf("runtime file edit returned invalid result type %T", data)
	}
	return response, nil
}
func patchApplyOperationResponse(result magicevent.Result) (orchestratorevents.ExecutePatchApplyOperationResult, error) {
	if result == nil {
		return orchestratorevents.ExecutePatchApplyOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return orchestratorevents.ExecutePatchApplyOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return orchestratorevents.ExecutePatchApplyOperationResult{}, err
	}
	response, ok := data.(orchestratorevents.ExecutePatchApplyOperationResult)
	if !ok {
		return orchestratorevents.ExecutePatchApplyOperationResult{}, fmt.Errorf("runtime patch apply returned invalid result type %T", data)
	}
	return response, nil
}
func patchPreviewOperationResponse(result magicevent.Result) (orchestratorevents.ExecutePatchPreviewOperationResult, error) {
	if result == nil {
		return orchestratorevents.ExecutePatchPreviewOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return orchestratorevents.ExecutePatchPreviewOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return orchestratorevents.ExecutePatchPreviewOperationResult{}, err
	}
	response, ok := data.(orchestratorevents.ExecutePatchPreviewOperationResult)
	if !ok {
		return orchestratorevents.ExecutePatchPreviewOperationResult{}, fmt.Errorf("runtime patch preview returned invalid result type %T", data)
	}
	return response, nil
}

func runtimeOperationResultError(result magicevent.Result) error {
	return runtimeOperationMagicError(result.Error())
}

func runtimeOperationMagicError(magicErr *cd.Error) error {
	if magicErr == nil {
		return nil
	}
	if magicErr.Cause != nil {
		return magicErr.Cause
	}
	return magicErr
}
