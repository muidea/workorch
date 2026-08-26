package workorchrepo

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type decisionWorkspaceDeletionRepository struct {
	dataRoot     string
	ownerBinding repository.OwnerRepositoryBinding
}

type orchestratorWorkspaceDeletionRepository struct{ dataRoot string }

func newOrchestratorWorkspaceDeletionRepository(dataRoot string) *orchestratorWorkspaceDeletionRepository {
	return &orchestratorWorkspaceDeletionRepository{dataRoot: dataRoot}
}

func (r *orchestratorWorkspaceDeletionRepository) Create(_ context.Context, item *orchestratormodel.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "orchestrator", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeWorkspaceDeletionFile(path, orchestratormodel.CloneWorkspaceDeletion(item))
}

func (r *orchestratorWorkspaceDeletionRepository) Get(_ context.Context, workspaceID common.WorkspaceID) (*orchestratormodel.WorkspaceDeletion, error) {
	path, err := workspaceDeletionFilePath(r.dataRoot, "orchestrator", workspaceID)
	if err != nil {
		return nil, err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	item, err := readWorkspaceDeletionFile[orchestratormodel.WorkspaceDeletion](path)
	if err != nil {
		return nil, err
	}
	if item.WorkspaceID != workspaceID || item.Validate() != nil {
		return nil, common.ErrConflict
	}
	return orchestratormodel.CloneWorkspaceDeletion(item), nil
}

func (r *orchestratorWorkspaceDeletionRepository) List(_ context.Context) ([]*orchestratormodel.WorkspaceDeletion, error) {
	if r == nil || strings.TrimSpace(r.dataRoot) == "" {
		return nil, common.ErrInvalidArgument
	}
	directory := filepath.Join(r.dataRoot, "system", "workspace-deletions", "orchestrator")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ret := make([]*orchestratormodel.WorkspaceDeletion, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return nil, common.ErrConflict
		}
		workspaceID := common.WorkspaceID(strings.TrimSuffix(entry.Name(), ".json"))
		item, err := r.Get(context.Background(), workspaceID)
		if err != nil {
			return nil, err
		}
		ret = append(ret, item)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].WorkspaceID < ret[j].WorkspaceID })
	return ret, nil
}

func (r *orchestratorWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *orchestratormodel.WorkspaceDeletion, expected uint64) error {
	if r == nil || item == nil || item.Validate() != nil || expected == 0 || item.Revision != expected+1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "orchestrator", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	current, err := readWorkspaceDeletionFile[orchestratormodel.WorkspaceDeletion](path)
	if err != nil {
		return err
	}
	if current.Validate() != nil || current.Revision != expected || current.QuiescenceIntent() != item.QuiescenceIntent() {
		return common.ErrConflict
	}
	return writeWorkspaceDeletionFile(path, orchestratormodel.CloneWorkspaceDeletion(item))
}

func (r *orchestratorWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, workspaceID common.WorkspaceID, operation func(context.Context) error) error {
	return withWorkspaceDeletionActiveFence(ctx, r.dataRoot, "orchestrator", workspaceID, operation)
}

func (r *orchestratorWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, scope deletioncontract.Scope, operation func(context.Context) error) error {
	return withWorkspaceDeletionPurgeFence(ctx, r.dataRoot, "orchestrator", scope, operation)
}

func newDecisionWorkspaceDeletionRepository(root string, binding repository.OwnerRepositoryBinding) *decisionWorkspaceDeletionRepository {
	return &decisionWorkspaceDeletionRepository{dataRoot: root, ownerBinding: binding}
}
func (r *decisionWorkspaceDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}
func (r *decisionWorkspaceDeletionRepository) Create(_ context.Context, item *decisionmodel.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "decision", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeWorkspaceDeletionFile(path, decisionmodel.CloneWorkspaceDeletion(item))
}
func (r *decisionWorkspaceDeletionRepository) Get(_ context.Context, id common.WorkspaceID) (*decisionmodel.WorkspaceDeletion, error) {
	path, err := workspaceDeletionFilePath(r.dataRoot, "decision", id)
	if err != nil {
		return nil, err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	item, err := readWorkspaceDeletionFile[decisionmodel.WorkspaceDeletion](path)
	if err != nil {
		return nil, err
	}
	if item.Validate() != nil || item.WorkspaceID != id {
		return nil, common.ErrConflict
	}
	return decisionmodel.CloneWorkspaceDeletion(item), nil
}
func (r *decisionWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *decisionmodel.WorkspaceDeletion, expected uint64) error {
	if r == nil || item == nil || item.Validate() != nil || expected == 0 || item.Revision != expected+1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "decision", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	current, err := readWorkspaceDeletionFile[decisionmodel.WorkspaceDeletion](path)
	if err != nil {
		return err
	}
	if current.Validate() != nil || current.Revision != expected {
		return common.ErrConflict
	}
	return writeWorkspaceDeletionFile(path, decisionmodel.CloneWorkspaceDeletion(item))
}
func (r *decisionWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, id common.WorkspaceID, fn func(context.Context) error) error {
	return withWorkspaceDeletionActiveFence(ctx, r.dataRoot, "decision", id, fn)
}
func (r *decisionWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, scope deletioncontract.Scope, fn func(context.Context) error) error {
	return withWorkspaceDeletionPurgeFence(ctx, r.dataRoot, "decision", scope, fn)
}

type approvalWorkspaceDeletionRepository struct {
	dataRoot     string
	ownerBinding repository.OwnerRepositoryBinding
}

func newApprovalWorkspaceDeletionRepository(root string, binding repository.OwnerRepositoryBinding) *approvalWorkspaceDeletionRepository {
	return &approvalWorkspaceDeletionRepository{dataRoot: root, ownerBinding: binding}
}
func (r *approvalWorkspaceDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}
func (r *approvalWorkspaceDeletionRepository) Create(_ context.Context, item *approvalmodel.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "approval", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeWorkspaceDeletionFile(path, approvalmodel.CloneWorkspaceDeletion(item))
}
func (r *approvalWorkspaceDeletionRepository) Get(_ context.Context, id common.WorkspaceID) (*approvalmodel.WorkspaceDeletion, error) {
	path, err := workspaceDeletionFilePath(r.dataRoot, "approval", id)
	if err != nil {
		return nil, err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	item, err := readWorkspaceDeletionFile[approvalmodel.WorkspaceDeletion](path)
	if err != nil {
		return nil, err
	}
	if item.Validate() != nil || item.WorkspaceID != id {
		return nil, common.ErrConflict
	}
	return approvalmodel.CloneWorkspaceDeletion(item), nil
}
func (r *approvalWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *approvalmodel.WorkspaceDeletion, expected uint64) error {
	if r == nil || item == nil || item.Validate() != nil || expected == 0 || item.Revision != expected+1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "approval", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	current, err := readWorkspaceDeletionFile[approvalmodel.WorkspaceDeletion](path)
	if err != nil {
		return err
	}
	if current.Validate() != nil || current.Revision != expected {
		return common.ErrConflict
	}
	return writeWorkspaceDeletionFile(path, approvalmodel.CloneWorkspaceDeletion(item))
}
func (r *approvalWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, id common.WorkspaceID, fn func(context.Context) error) error {
	return withWorkspaceDeletionActiveFence(ctx, r.dataRoot, "approval", id, fn)
}
func (r *approvalWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, scope deletioncontract.Scope, fn func(context.Context) error) error {
	return withWorkspaceDeletionPurgeFence(ctx, r.dataRoot, "approval", scope, fn)
}

var workspaceDeletionFileLocks sync.Map

func workspaceDeletionFileLock(path string) *sync.Mutex {
	key, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		key = filepath.Clean(path)
	}
	value, _ := workspaceDeletionFileLocks.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func workspaceDeletionFilePath(dataRoot, owner string, workspaceID common.WorkspaceID) (string, error) {
	identity := strings.TrimSpace(string(workspaceID))
	if strings.TrimSpace(dataRoot) == "" || strings.TrimSpace(owner) == "" || identity == "" || filepath.Base(identity) != identity || strings.ContainsAny(identity, `/\\`) {
		return "", common.ErrInvalidArgument
	}
	return filepath.Join(dataRoot, "system", "workspace-deletions", owner, identity+".json"), nil
}

func readWorkspaceDeletionFile[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, common.ErrNotFound
		}
		return nil, err
	}
	var item T
	if err := decodeCurrentJSON(data, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func writeWorkspaceDeletionFile(path string, item any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".workspace-deletion-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	keep = true
	return nil
}

type memoryWorkspaceDeletionRepository struct {
	dataRoot     string
	ownerBinding repository.OwnerRepositoryBinding
}

func newMemoryWorkspaceDeletionRepository(dataRoot string, binding repository.OwnerRepositoryBinding) *memoryWorkspaceDeletionRepository {
	return &memoryWorkspaceDeletionRepository{dataRoot: dataRoot, ownerBinding: binding}
}
func (r *memoryWorkspaceDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *memoryWorkspaceDeletionRepository) Create(_ context.Context, item *memorymodel.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "memory", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeWorkspaceDeletionFile(path, memorymodel.CloneWorkspaceDeletion(item))
}

func (r *memoryWorkspaceDeletionRepository) Get(_ context.Context, workspaceID common.WorkspaceID) (*memorymodel.WorkspaceDeletion, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "memory", workspaceID)
	if err != nil {
		return nil, err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	item, err := readWorkspaceDeletionFile[memorymodel.WorkspaceDeletion](path)
	if err != nil {
		return nil, err
	}
	if item.WorkspaceID != workspaceID || item.Validate() != nil {
		return nil, common.ErrConflict
	}
	return memorymodel.CloneWorkspaceDeletion(item), nil
}

func (r *memoryWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *memorymodel.WorkspaceDeletion, expected uint64) error {
	if r == nil || item == nil || item.Validate() != nil || expected == 0 || item.Revision != expected+1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "memory", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	current, err := readWorkspaceDeletionFile[memorymodel.WorkspaceDeletion](path)
	if err != nil {
		return err
	}
	if current.Validate() != nil || current.Revision != expected {
		return common.ErrConflict
	}
	return writeWorkspaceDeletionFile(path, memorymodel.CloneWorkspaceDeletion(item))
}

func (r *memoryWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, workspaceID common.WorkspaceID, operation func(context.Context) error) error {
	return withWorkspaceDeletionActiveFence(ctx, r.dataRoot, "memory", workspaceID, operation)
}
func (r *memoryWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, scope deletioncontract.Scope, operation func(context.Context) error) error {
	return withWorkspaceDeletionPurgeFence(ctx, r.dataRoot, "memory", scope, operation)
}

type resultWorkspaceDeletionRepository struct {
	dataRoot     string
	ownerBinding repository.OwnerRepositoryBinding
}

func newResultWorkspaceDeletionRepository(dataRoot string, binding repository.OwnerRepositoryBinding) *resultWorkspaceDeletionRepository {
	return &resultWorkspaceDeletionRepository{dataRoot: dataRoot, ownerBinding: binding}
}
func (r *resultWorkspaceDeletionRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	if r == nil {
		return repository.OwnerRepositoryBinding{}
	}
	return r.ownerBinding
}

func (r *resultWorkspaceDeletionRepository) Create(_ context.Context, item *resultmodel.WorkspaceDeletion) error {
	if r == nil || item == nil || item.Validate() != nil || item.Revision != 1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "result", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeWorkspaceDeletionFile(path, resultmodel.CloneWorkspaceDeletion(item))
}

func (r *resultWorkspaceDeletionRepository) Get(_ context.Context, workspaceID common.WorkspaceID) (*resultmodel.WorkspaceDeletion, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "result", workspaceID)
	if err != nil {
		return nil, err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	item, err := readWorkspaceDeletionFile[resultmodel.WorkspaceDeletion](path)
	if err != nil {
		return nil, err
	}
	if item.WorkspaceID != workspaceID || item.Validate() != nil {
		return nil, common.ErrConflict
	}
	return resultmodel.CloneWorkspaceDeletion(item), nil
}

func (r *resultWorkspaceDeletionRepository) CompareAndSwap(_ context.Context, item *resultmodel.WorkspaceDeletion, expected uint64) error {
	if r == nil || item == nil || item.Validate() != nil || expected == 0 || item.Revision != expected+1 {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(r.dataRoot, "result", item.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	current, err := readWorkspaceDeletionFile[resultmodel.WorkspaceDeletion](path)
	if err != nil {
		return err
	}
	if current.Validate() != nil || current.Revision != expected {
		return common.ErrConflict
	}
	return writeWorkspaceDeletionFile(path, resultmodel.CloneWorkspaceDeletion(item))
}

func (r *resultWorkspaceDeletionRepository) WithWorkspaceActiveFence(ctx context.Context, workspaceID common.WorkspaceID, operation func(context.Context) error) error {
	return withWorkspaceDeletionActiveFence(ctx, r.dataRoot, "result", workspaceID, operation)
}
func (r *resultWorkspaceDeletionRepository) WithWorkspacePurgeFence(ctx context.Context, scope deletioncontract.Scope, operation func(context.Context) error) error {
	return withWorkspaceDeletionPurgeFence(ctx, r.dataRoot, "result", scope, operation)
}

type workspaceDeletionMutationContextKey struct{}

type workspaceDeletionMutationContext struct {
	owner       string
	workspaceID common.WorkspaceID
}

// withWorkspaceDeletionMutation serializes every owner mutation with the
// workspace-scoped deletion tombstone. The context marker prevents a wrapper
// (for example artifact storage) and its workspace-scoped base repository from
// trying to acquire the same non-reentrant lock twice.
func withWorkspaceDeletionMutation(
	ctx context.Context,
	dataRoot string,
	owner string,
	workspaceID common.WorkspaceID,
	mutate func(context.Context) error,
) error {
	if mutate == nil {
		return common.ErrInvalidArgument
	}
	if marker, ok := ctx.Value(workspaceDeletionMutationContextKey{}).(workspaceDeletionMutationContext); ok && marker.owner == owner && marker.workspaceID == workspaceID {
		return mutate(ctx)
	}
	path, err := workspaceDeletionFilePath(dataRoot, owner, workspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()

	_, fenced, err := workspaceDeletionMode(path, owner)
	if err != nil {
		return err
	}
	if fenced {
		return common.ErrNotFound
	}
	guarded := context.WithValue(ctx, workspaceDeletionMutationContextKey{}, workspaceDeletionMutationContext{owner: owner, workspaceID: workspaceID})
	return mutate(guarded)
}

func withWorkspaceDeletionPurgeFence(ctx context.Context, dataRoot, owner string, scope deletioncontract.Scope, operation func(context.Context) error) error {
	scope = scope.Normalize()
	if scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge || operation == nil {
		return common.ErrInvalidArgument
	}
	path, err := workspaceDeletionFilePath(dataRoot, owner, scope.WorkspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	current, err := workspaceDeletionScope(path, owner)
	if err != nil {
		return err
	}
	if current != scope {
		return common.ErrConflict
	}
	guarded := context.WithValue(ctx, workspaceDeletionMutationContextKey{}, workspaceDeletionMutationContext{owner: owner, workspaceID: scope.WorkspaceID})
	return operation(guarded)
}

func workspaceDeletionScope(path, owner string) (deletioncontract.Scope, error) {
	switch owner {
	case "orchestrator":
		item, err := readWorkspaceDeletionFile[orchestratormodel.WorkspaceDeletion](path)
		if err != nil || item.Validate() != nil {
			if err != nil {
				return deletioncontract.Scope{}, err
			}
			return deletioncontract.Scope{}, common.ErrConflict
		}
		if item.Phase != orchestratormodel.WorkspaceDeletionPhaseFenced && item.Phase != orchestratormodel.WorkspaceDeletionPhasePurged {
			return deletioncontract.Scope{}, common.ErrConflict
		}
		return item.Scope(), nil
	case "memory":
		item, err := readWorkspaceDeletionFile[memorymodel.WorkspaceDeletion](path)
		if err != nil || item.Validate() != nil {
			if err != nil {
				return deletioncontract.Scope{}, err
			}
			return deletioncontract.Scope{}, common.ErrConflict
		}
		return item.Scope(), nil
	case "result":
		item, err := readWorkspaceDeletionFile[resultmodel.WorkspaceDeletion](path)
		if err != nil || item.Validate() != nil {
			if err != nil {
				return deletioncontract.Scope{}, err
			}
			return deletioncontract.Scope{}, common.ErrConflict
		}
		return item.Scope(), nil
	case "decision":
		item, err := readWorkspaceDeletionFile[decisionmodel.WorkspaceDeletion](path)
		if err != nil || item.Validate() != nil {
			if err != nil {
				return deletioncontract.Scope{}, err
			}
			return deletioncontract.Scope{}, common.ErrConflict
		}
		return item.Scope(), nil
	case "approval":
		item, err := readWorkspaceDeletionFile[approvalmodel.WorkspaceDeletion](path)
		if err != nil || item.Validate() != nil {
			if err != nil {
				return deletioncontract.Scope{}, err
			}
			return deletioncontract.Scope{}, common.ErrConflict
		}
		return item.Scope(), nil
	case "eventstream":
		item, err := readWorkspaceDeletionFile[eventstreammodel.WorkspaceDeletion](path)
		if err != nil || item.Validate() != nil {
			if err != nil {
				return deletioncontract.Scope{}, err
			}
			return deletioncontract.Scope{}, common.ErrConflict
		}
		return item.Scope(), nil
	default:
		return deletioncontract.Scope{}, common.ErrInvalidArgument
	}
}

func workspaceDeletionMode(path, owner string) (string, bool, error) {
	switch owner {
	case "orchestrator":
		item, err := readWorkspaceDeletionFile[orchestratormodel.WorkspaceDeletion](path)
		if errors.Is(err, common.ErrNotFound) {
			return "", false, nil
		}
		if err != nil || item.Validate() != nil {
			if err != nil {
				return "", false, err
			}
			return "", false, common.ErrConflict
		}
		return string(item.Mode), true, nil
	case "memory":
		item, err := readWorkspaceDeletionFile[memorymodel.WorkspaceDeletion](path)
		if errors.Is(err, common.ErrNotFound) {
			return "", false, nil
		}
		if err != nil || item.Validate() != nil {
			if err != nil {
				return "", false, err
			}
			return "", false, common.ErrConflict
		}
		return string(item.Mode), true, nil
	case "result":
		item, err := readWorkspaceDeletionFile[resultmodel.WorkspaceDeletion](path)
		if errors.Is(err, common.ErrNotFound) {
			return "", false, nil
		}
		if err != nil || item.Validate() != nil {
			if err != nil {
				return "", false, err
			}
			return "", false, common.ErrConflict
		}
		return string(item.Mode), true, nil
	case "decision":
		item, err := readWorkspaceDeletionFile[decisionmodel.WorkspaceDeletion](path)
		if errors.Is(err, common.ErrNotFound) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if item.Validate() != nil {
			return "", false, common.ErrConflict
		}
		return string(item.Mode), true, nil
	case "approval":
		item, err := readWorkspaceDeletionFile[approvalmodel.WorkspaceDeletion](path)
		if errors.Is(err, common.ErrNotFound) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if item.Validate() != nil {
			return "", false, common.ErrConflict
		}
		return string(item.Mode), true, nil
	case "eventstream":
		item, err := readWorkspaceDeletionFile[eventstreammodel.WorkspaceDeletion](path)
		if errors.Is(err, common.ErrNotFound) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if item.Validate() != nil {
			return "", false, common.ErrConflict
		}
		return string(item.Mode), true, nil
	default:
		return "", false, common.ErrInvalidArgument
	}
}

func workspaceDeletionExists(dataRoot, owner string, workspaceID common.WorkspaceID) (bool, error) {
	path, err := workspaceDeletionFilePath(dataRoot, owner, workspaceID)
	if err != nil {
		return false, err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	_, exists, err := workspaceDeletionMode(path, owner)
	return exists, err
}

func withWorkspaceDeletionActiveFence(ctx context.Context, dataRoot, owner string, workspaceID common.WorkspaceID, operation func(context.Context) error) error {
	if operation == nil {
		return common.ErrInvalidArgument
	}
	if marker, ok := ctx.Value(workspaceDeletionMutationContextKey{}).(workspaceDeletionMutationContext); ok && marker.owner == owner && marker.workspaceID == workspaceID {
		return operation(ctx)
	}
	path, err := workspaceDeletionFilePath(dataRoot, owner, workspaceID)
	if err != nil {
		return err
	}
	lock := workspaceDeletionFileLock(path)
	lock.Lock()
	defer lock.Unlock()
	_, fenced, err := workspaceDeletionMode(path, owner)
	if err != nil {
		return err
	}
	if fenced {
		return common.ErrNotFound
	}
	guarded := context.WithValue(ctx, workspaceDeletionMutationContextKey{}, workspaceDeletionMutationContext{owner: owner, workspaceID: workspaceID})
	return operation(guarded)
}
