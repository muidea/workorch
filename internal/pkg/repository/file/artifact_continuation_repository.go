package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	artifactContinuationMetadataFile = "metadata.json"
	artifactContinuationChunksDir    = "chunks"
	artifactContinuationAssembledDir = "assembled"
	artifactContinuationContentExt   = ".content"
)

type ArtifactContinuationRepository struct {
	mu   sync.RWMutex
	path string
}

func NewArtifactContinuationRepository(path string) *ArtifactContinuationRepository {
	return &ArtifactContinuationRepository{path: path}
}

func (r *ArtifactContinuationRepository) Create(_ context.Context, item *orchestratormodel.ArtifactContinuation) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	dir, err := r.continuationDir(item.ID)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, artifactContinuationMetadataFile)
	if _, err := os.Stat(path); err == nil {
		return common.ErrConflict
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return writeJSONFileAtomic(path, cloneArtifactContinuation(item))
}

func (r *ArtifactContinuationRepository) Get(_ context.Context, id common.ArtifactContinuationID) (*orchestratormodel.ArtifactContinuation, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	path, err := r.metadataPath(id)
	if err != nil {
		return nil, err
	}
	item, err := readJSONItemFile[orchestratormodel.ArtifactContinuation](path)
	if err != nil {
		return nil, err
	}
	return cloneArtifactContinuation(&item), nil
}

func (r *ArtifactContinuationRepository) ListByRun(_ context.Context, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.list(func(item orchestratormodel.ArtifactContinuation) bool {
		return item.RunID == runID
	})
}

func (r *ArtifactContinuationRepository) ListOpenByRun(ctx context.Context, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	items, err := r.ListByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	ret := items[:0]
	for _, item := range items {
		if !artifactContinuationTerminal(item.State) {
			ret = append(ret, item)
		}
	}
	return nilIfEmptyArtifactContinuations(ret), nil
}

func (r *ArtifactContinuationRepository) Update(_ context.Context, item *orchestratormodel.ArtifactContinuation) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	path, err := r.metadataPath(item.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return writeJSONFileAtomic(path, cloneArtifactContinuation(item))
}

func (r *ArtifactContinuationRepository) Delete(_ context.Context, id common.ArtifactContinuationID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	dir, err := r.continuationDir(id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, artifactContinuationMetadataFile)); os.IsNotExist(err) {
		return common.ErrNotFound
	} else if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (r *ArtifactContinuationRepository) SaveChunk(_ context.Context, chunk *orchestratormodel.ArtifactContinuationChunk, content string) error {
	if chunk == nil || chunk.ContinuationID == "" || chunk.Sequence <= 0 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.metadataPathExisting(chunk.ContinuationID); err != nil {
		return err
	}
	metaPath, contentPath, err := r.chunkPaths(chunk.ContinuationID, chunk.Sequence)
	if err != nil {
		return err
	}
	if err := writeJSONFileAtomic(metaPath, cloneArtifactContinuationChunk(chunk)); err != nil {
		return err
	}
	return writeFileAtomic(contentPath, []byte(content), 0o644)
}

func (r *ArtifactContinuationRepository) GetChunk(_ context.Context, id common.ArtifactContinuationID, sequence int) (*orchestratormodel.ArtifactContinuationChunk, string, error) {
	if id == "" || sequence <= 0 {
		return nil, "", common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, err := r.metadataPathExisting(id); err != nil {
		return nil, "", err
	}
	metaPath, contentPath, err := r.chunkPaths(id, sequence)
	if err != nil {
		return nil, "", err
	}
	chunk, err := readJSONItemFile[orchestratormodel.ArtifactContinuationChunk](metaPath)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(contentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", common.ErrNotFound
		}
		return nil, "", err
	}
	return cloneArtifactContinuationChunk(&chunk), string(data), nil
}

func (r *ArtifactContinuationRepository) ListChunks(_ context.Context, id common.ArtifactContinuationID) ([]*orchestratormodel.ArtifactContinuationChunk, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, err := r.metadataPathExisting(id); err != nil {
		return nil, err
	}
	dir, err := r.chunksDir(id)
	if err != nil {
		return nil, err
	}
	paths, err := repositoryItemPaths(dir)
	if err != nil {
		return nil, err
	}
	ret := make([]*orchestratormodel.ArtifactContinuationChunk, 0, len(paths))
	for _, path := range paths {
		item, err := readJSONItemFile[orchestratormodel.ArtifactContinuationChunk](path)
		if err != nil {
			return nil, err
		}
		ret = append(ret, cloneArtifactContinuationChunk(&item))
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Sequence < ret[j].Sequence })
	if len(ret) == 0 {
		return nil, nil
	}
	return ret, nil
}

func (r *ArtifactContinuationRepository) SaveAssembled(_ context.Context, item *orchestratormodel.ArtifactContinuationAssembledMetadata, content string) error {
	if item == nil || item.ContinuationID == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.metadataPathExisting(item.ContinuationID); err != nil {
		return err
	}
	metaPath, contentPath, err := r.assembledPaths(item.ContinuationID)
	if err != nil {
		return err
	}
	if err := writeJSONFileAtomic(metaPath, cloneArtifactContinuationAssembledMetadata(item)); err != nil {
		return err
	}
	return writeFileAtomic(contentPath, []byte(content), 0o644)
}

func (r *ArtifactContinuationRepository) GetAssembled(_ context.Context, id common.ArtifactContinuationID) (*orchestratormodel.ArtifactContinuationAssembledMetadata, string, error) {
	if id == "" {
		return nil, "", common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, err := r.metadataPathExisting(id); err != nil {
		return nil, "", err
	}
	metaPath, contentPath, err := r.assembledPaths(id)
	if err != nil {
		return nil, "", err
	}
	item, err := readJSONItemFile[orchestratormodel.ArtifactContinuationAssembledMetadata](metaPath)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(contentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", common.ErrNotFound
		}
		return nil, "", err
	}
	return cloneArtifactContinuationAssembledMetadata(&item), string(data), nil
}

func (r *ArtifactContinuationRepository) root() string {
	return repositoryItemRoot(r.path)
}

func (r *ArtifactContinuationRepository) continuationDir(id common.ArtifactContinuationID) (string, error) {
	value := strings.TrimSpace(string(id))
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return "", common.ErrInvalidArgument
	}
	return filepath.Join(r.root(), safeRepositoryItemFileName(value)), nil
}

func (r *ArtifactContinuationRepository) metadataPath(id common.ArtifactContinuationID) (string, error) {
	dir, err := r.continuationDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, artifactContinuationMetadataFile), nil
}

func (r *ArtifactContinuationRepository) metadataPathExisting(id common.ArtifactContinuationID) (string, error) {
	path, err := r.metadataPath(id)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", common.ErrNotFound
	} else if err != nil {
		return "", err
	}
	return path, nil
}

func (r *ArtifactContinuationRepository) chunksDir(id common.ArtifactContinuationID) (string, error) {
	dir, err := r.continuationDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, artifactContinuationChunksDir), nil
}

func (r *ArtifactContinuationRepository) chunkPaths(id common.ArtifactContinuationID, sequence int) (string, string, error) {
	if sequence <= 0 {
		return "", "", common.ErrInvalidArgument
	}
	dir, err := r.chunksDir(id)
	if err != nil {
		return "", "", err
	}
	name := fmt.Sprintf("%03d", sequence)
	return filepath.Join(dir, name+".json"), filepath.Join(dir, name+artifactContinuationContentExt), nil
}

func (r *ArtifactContinuationRepository) assembledPaths(id common.ArtifactContinuationID) (string, string, error) {
	dir, err := r.continuationDir(id)
	if err != nil {
		return "", "", err
	}
	assembledDir := filepath.Join(dir, artifactContinuationAssembledDir)
	return filepath.Join(assembledDir, artifactContinuationMetadataFile), filepath.Join(assembledDir, "content"), nil
}

func (r *ArtifactContinuationRepository) list(matches func(orchestratormodel.ArtifactContinuation) bool) ([]*orchestratormodel.ArtifactContinuation, error) {
	entries, err := os.ReadDir(r.root())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	ret := make([]*orchestratormodel.ArtifactContinuation, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			return nil, fmt.Errorf("%w: artifact continuation root contains a non-directory entry", common.ErrInvalidArgument)
		}
		item, err := readJSONItemFile[orchestratormodel.ArtifactContinuation](filepath.Join(r.root(), entry.Name(), artifactContinuationMetadataFile))
		if err != nil {
			return nil, err
		}
		if matches(item) {
			ret = append(ret, cloneArtifactContinuation(&item))
		}
	}
	sortArtifactContinuations(ret)
	return nilIfEmptyArtifactContinuations(ret), nil
}

func artifactContinuationTerminal(state common.ArtifactContinuationState) bool {
	return state == common.ArtifactContinuationStateApplied || state == common.ArtifactContinuationStateFailed
}

func nilIfEmptyArtifactContinuations(items []*orchestratormodel.ArtifactContinuation) []*orchestratormodel.ArtifactContinuation {
	if len(items) == 0 {
		return nil
	}
	return items
}

func sortArtifactContinuations(items []*orchestratormodel.ArtifactContinuation) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].RunID != items[j].RunID {
			return string(items[i].RunID) < string(items[j].RunID)
		}
		return string(items[i].ID) < string(items[j].ID)
	})
}

func cloneArtifactContinuation(item *orchestratormodel.ArtifactContinuation) *orchestratormodel.ArtifactContinuation {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.CoverageTargets = cloneStringSlice(item.CoverageTargets)
	cloned.ReceivedSequences = cloneIntSlice(item.ReceivedSequences)
	cloned.ReceivedChunkHashes = cloneIntStringMap(item.ReceivedChunkHashes)
	cloned.RetryCounters = cloneStringIntMap(item.RetryCounters)
	cloned.EvidenceRefs = cloneStringSlice(item.EvidenceRefs)
	cloned.Metadata = item.Metadata.Clone()
	return &cloned
}

func cloneArtifactContinuationChunk(item *orchestratormodel.ArtifactContinuationChunk) *orchestratormodel.ArtifactContinuationChunk {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.LLMCallRefs = cloneStringSlice(item.LLMCallRefs)
	if item.StepRefs != nil {
		cloned.StepRefs = append([]common.StepID(nil), item.StepRefs...)
	}
	cloned.Validation = item.Validation.Clone()
	cloned.Metadata = item.Metadata.Clone()
	return &cloned
}

func cloneArtifactContinuationAssembledMetadata(item *orchestratormodel.ArtifactContinuationAssembledMetadata) *orchestratormodel.ArtifactContinuationAssembledMetadata {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.SourceChunkRefs = cloneStringSlice(item.SourceChunkRefs)
	cloned.ApplyEvidenceRefs = cloneStringSlice(item.ApplyEvidenceRefs)
	cloned.Metadata = item.Metadata.Clone()
	return &cloned
}

func cloneIntSlice(input []int) []int {
	if input == nil {
		return nil
	}
	return append([]int(nil), input...)
}

func cloneIntStringMap(input map[int]string) map[int]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[int]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneStringIntMap(input map[string]int) map[string]int {
	if input == nil {
		return nil
	}
	out := make(map[string]int, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
