package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (m *Biz) handleRuntimeValidationReadDependencies(event magicevent.Event, result magicevent.Result) {
	command, ok := shellRuntimeIngressCommand[shellevents.QueryRuntimeValidationReadDependenciesCommand](event, result)
	if !ok {
		return
	}
	if m == nil || m.runtimeAdapter == nil || command.Validate() != nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	receipt, found, err := m.queryRuntimeOperationReceiptWithSource(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution, &command.RuntimeOperationExecutionContext)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	if !found || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != command.SourceReceiptHash ||
		(receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted && receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected) ||
		!reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, &command.OriginalDependencies) || !reflect.DeepEqual(receipt.Evidence.ApplicabilityDependencies, command.ApplicabilitySource) ||
		!reflect.DeepEqual(receipt.TargetRefs, command.Operation.TargetRefs) {
		setCapabilityError(result, fmt.Errorf("%w: Shell copied-input source receipt mismatch", common.ErrConflict))
		return
	}
	// Caller directories cannot authorize reads. Query the current Workspace owner.
	workspaceCommand := workspaceevents.GetWorkspaceCommand{WorkspaceID: command.WorkspaceID}
	query := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandGet, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), event.Context(), workspaceCommand)
	query.BindLaneKey(eventcontract.QueryLaneKey(workspaceevents.TopicWorkspaceCommandGet, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	response := m.SendEvent(query)
	if response == nil {
		setCapabilityError(result, fmt.Errorf("Shell dependency workspace query returned no result"))
		return
	}
	data, queryErr := response.Get()
	if queryErr != nil {
		setCapabilityError(result, queryErr)
		return
	}
	workspace, ok := data.(workspaceevents.WorkspaceResult)
	if !ok || workspace.ValidateFor(workspaceCommand) != nil {
		setCapabilityError(result, common.ErrConflict)
		return
	}
	inputs, missing, err := observeShellValidationCopiedInputs(event.Context(), workspace.Workspace.DeliverableDirectory, command.OriginalDependencies)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	output := shellevents.QueryRuntimeValidationReadDependenciesResult{RequestID: command.RequestID, IssuedAtStateRevision: command.IssuedAtStateRevision, SourceReceiptHash: command.SourceReceiptHash, Missing: missing}
	if !missing {
		set, err := shellValidationOwnerDependencies(command.Transaction.DefinitionRevision, RuntimeCommandRequest{WorkspaceID: command.WorkspaceID, ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true}}, inputs)
		if err != nil {
			setCapabilityError(result, err)
			return
		}
		output.CurrentDependencies = &set
	}
	if _, _, err := output.ValidateFor(command); err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(output, nil)
}

// Re-read only the exact originally copied selector. This is not a new tree
// inventory, an atomic cross-file snapshot, or tracing all child-process reads.
func observeShellValidationCopiedInputs(ctx context.Context, directory string, original runtimecontract.RuntimeOwnerRevisionSet) ([]shellValidationCopiedInput, bool, error) {
	if directory == "" || !filepath.IsAbs(directory) || len(original.Entries) == 0 || len(original.Entries) > maxObservedWorkspaceFiles {
		return nil, false, common.ErrInvalidArgument
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, false, err
	}
	defer root.Close()
	inputs := make([]shellValidationCopiedInput, 0, len(original.Entries))
	var total int64
	for _, entry := range original.Entries {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		ref := entry.ResourceID
		if !filepath.IsLocal(ref) || filepath.ToSlash(filepath.Clean(ref)) != ref || managedValidationSandboxPathExcluded(ref) {
			return nil, false, common.ErrInvalidArgument
		}
		// Reject symlink/special selectors; Root also prevents concurrent links
		// from escaping the owner-supplied deliverable tree during open.
		parts := strings.Split(ref, "/")
		for index := range parts {
			info, err := root.Lstat(filepath.Join(parts[:index+1]...))
			if os.IsNotExist(err) {
				return nil, true, nil
			}
			if err != nil {
				return nil, false, err
			}
			if info.Mode()&os.ModeSymlink != 0 || (index < len(parts)-1 && !info.IsDir()) || (index == len(parts)-1 && !info.Mode().IsRegular()) {
				return nil, false, common.ErrInvalidArgument
			}
		}
		file, err := openShellDependencySelector(ctx, root, ref)
		if os.IsNotExist(err) {
			return nil, true, nil
		}
		if err != nil {
			return nil, false, err
		}
		input, readErr := observeShellValidationCopiedFile(ctx, file, ref, maxObservedWorkspaceBytes-total)
		closeErr := file.Close()
		if readErr != nil {
			return nil, false, readErr
		}
		if closeErr != nil {
			return nil, false, closeErr
		}
		total += input.Size
		inputs = append(inputs, input)
	}
	return inputs, false, nil
}

func observeShellValidationCopiedFile(ctx context.Context, file *os.File, ref string, remaining int64) (shellValidationCopiedInput, error) {
	info, err := file.Stat()
	if err != nil {
		return shellValidationCopiedInput{}, err
	}
	if !info.Mode().IsRegular() {
		return shellValidationCopiedInput{}, common.ErrInvalidArgument
	}
	digest := sha256.New()
	reader := io.LimitReader(file, remaining+1)
	buffer := make([]byte, 32*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return shellValidationCopiedInput{}, err
		}
		n, err := reader.Read(buffer)
		if n > 0 {
			_, _ = digest.Write(buffer[:n])
			size += int64(n)
		}
		if size > remaining {
			return shellValidationCopiedInput{}, fmt.Errorf("Shell copied-input observation exceeds bounded bytes")
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return shellValidationCopiedInput{}, err
		}
	}
	return shellValidationCopiedInput{TargetRef: ref, ContentHash: hex.EncodeToString(digest.Sum(nil)), Size: size, Mode: uint32(info.Mode().Perm() & 0777)}, nil
}
