package orchestrator

// Runtime response decoding is owned by Biz.

import (
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func fileListOperationResponse(result magicevent.Result) (fileevents.ExecuteOperationResult, error) {
	if result == nil {
		return fileevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(fileevents.ExecuteOperationResult)
	if !ok {
		return fileevents.ExecuteOperationResult{}, fmt.Errorf("runtime file list returned invalid result type %T", data)
	}
	return response, nil
}
func fileReadOperationResponse(result magicevent.Result) (fileevents.ExecuteOperationResult, error) {
	if result == nil {
		return fileevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(fileevents.ExecuteOperationResult)
	if !ok {
		return fileevents.ExecuteOperationResult{}, fmt.Errorf("runtime file read returned invalid result type %T", data)
	}
	return response, nil
}
func sshOperationResponse(result magicevent.Result) (sshevents.ExecuteOperationResult, error) {
	if result == nil {
		return sshevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return sshevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return sshevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(sshevents.ExecuteOperationResult)
	if !ok {
		return sshevents.ExecuteOperationResult{}, fmt.Errorf("runtime ssh returned invalid result type %T", data)
	}
	return response, nil
}
func webOperationResponse(result magicevent.Result) (webevents.ExecuteOperationResult, error) {
	if result == nil {
		return webevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return webevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return webevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(webevents.ExecuteOperationResult)
	if !ok {
		return webevents.ExecuteOperationResult{}, fmt.Errorf("runtime web returned invalid result type %T", data)
	}
	return response, nil
}
func mcpOperationResponse(result magicevent.Result) (mcpevents.ExecuteOperationResult, error) {
	if result == nil {
		return mcpevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return mcpevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return mcpevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(mcpevents.ExecuteOperationResult)
	if !ok {
		return mcpevents.ExecuteOperationResult{}, fmt.Errorf("runtime mcp returned invalid result type %T", data)
	}
	return response, nil
}
func gitOperationResponse(result magicevent.Result) (gitevents.ExecuteOperationResult, error) {
	if result == nil {
		return gitevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return gitevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return gitevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(gitevents.ExecuteOperationResult)
	if !ok {
		return gitevents.ExecuteOperationResult{}, fmt.Errorf("runtime git returned invalid result type %T", data)
	}
	return response, nil
}
func shellOperationResponse(result magicevent.Result) (shellevents.ExecuteOperationResult, error) {
	if result == nil {
		return shellevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return shellevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return shellevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(shellevents.ExecuteOperationResult)
	if !ok {
		return shellevents.ExecuteOperationResult{}, fmt.Errorf("runtime shell returned invalid result type %T", data)
	}
	return response, nil
}
func fileEditOperationResponse(result magicevent.Result) (fileevents.ExecuteOperationResult, error) {
	if result == nil {
		return fileevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(fileevents.ExecuteOperationResult)
	if !ok {
		return fileevents.ExecuteOperationResult{}, fmt.Errorf("runtime file edit returned invalid result type %T", data)
	}
	return response, nil
}
func fileWriteOperationResponse(result magicevent.Result) (fileevents.ExecuteOperationResult, error) {
	if result == nil {
		return fileevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(fileevents.ExecuteOperationResult)
	if !ok {
		return fileevents.ExecuteOperationResult{}, fmt.Errorf("runtime file write returned invalid result type %T", data)
	}
	return response, nil
}
func patchApplyOperationResponse(result magicevent.Result) (fileevents.ExecuteOperationResult, error) {
	if result == nil {
		return fileevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(fileevents.ExecuteOperationResult)
	if !ok {
		return fileevents.ExecuteOperationResult{}, fmt.Errorf("runtime patch apply returned invalid result type %T", data)
	}
	return response, nil
}
func patchPreviewOperationResponse(result magicevent.Result) (fileevents.ExecuteOperationResult, error) {
	if result == nil {
		return fileevents.ExecuteOperationResult{}, common.ErrInvalidArgument
	}
	if err := runtimeOperationResultError(result); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	data, magicErr := result.Get()
	if err := runtimeOperationMagicError(magicErr); err != nil {
		return fileevents.ExecuteOperationResult{}, err
	}
	response, ok := data.(fileevents.ExecuteOperationResult)
	if !ok {
		return fileevents.ExecuteOperationResult{}, fmt.Errorf("runtime patch preview returned invalid result type %T", data)
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
