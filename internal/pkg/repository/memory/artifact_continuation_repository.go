package memory

import (
	"context"
	"sort"
	"sync"

	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ArtifactContinuationRepository struct {
	mu               sync.RWMutex
	items            map[common.ArtifactContinuationID]*orchestratormodel.ArtifactContinuation
	plans            map[common.ArtifactContinuationID]map[string]any
	chunks           map[common.ArtifactContinuationID]map[int]*orchestratormodel.ArtifactContinuationChunk
	chunkContent     map[common.ArtifactContinuationID]map[int]string
	assembledMeta    map[common.ArtifactContinuationID]*orchestratormodel.ArtifactContinuationAssembledMetadata
	assembledContent map[common.ArtifactContinuationID]string
}

func NewArtifactContinuationRepository() *ArtifactContinuationRepository {
	return &ArtifactContinuationRepository{
		items:            map[common.ArtifactContinuationID]*orchestratormodel.ArtifactContinuation{},
		plans:            map[common.ArtifactContinuationID]map[string]any{},
		chunks:           map[common.ArtifactContinuationID]map[int]*orchestratormodel.ArtifactContinuationChunk{},
		chunkContent:     map[common.ArtifactContinuationID]map[int]string{},
		assembledMeta:    map[common.ArtifactContinuationID]*orchestratormodel.ArtifactContinuationAssembledMetadata{},
		assembledContent: map[common.ArtifactContinuationID]string{},
	}
}

func (r *ArtifactContinuationRepository) Create(_ context.Context, item *orchestratormodel.ArtifactContinuation) error {
	if item == nil || item.ID == "" || item.WorkspaceID == "" || item.RunID == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; ok {
		return common.ErrConflict
	}
	r.items[item.ID] = cloneArtifactContinuation(item)
	return nil
}

func (r *ArtifactContinuationRepository) Get(_ context.Context, id common.ArtifactContinuationID) (*orchestratormodel.ArtifactContinuation, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneArtifactContinuation(item), nil
}

func (r *ArtifactContinuationRepository) ListByRun(_ context.Context, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ret := make([]*orchestratormodel.ArtifactContinuation, 0)
	for _, item := range r.items {
		if item.RunID == runID {
			ret = append(ret, cloneArtifactContinuation(item))
		}
	}
	sortArtifactContinuations(ret)
	return nilIfEmptyArtifactContinuations(ret), nil
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
	if _, ok := r.items[item.ID]; !ok {
		return common.ErrNotFound
	}
	r.items[item.ID] = cloneArtifactContinuation(item)
	return nil
}

func (r *ArtifactContinuationRepository) Delete(_ context.Context, id common.ArtifactContinuationID) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	delete(r.items, id)
	delete(r.plans, id)
	delete(r.chunks, id)
	delete(r.chunkContent, id)
	delete(r.assembledMeta, id)
	delete(r.assembledContent, id)
	return nil
}

func (r *ArtifactContinuationRepository) SavePlan(_ context.Context, id common.ArtifactContinuationID, plan map[string]any) error {
	if id == "" {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return common.ErrNotFound
	}
	r.plans[id] = cloneMap(plan)
	return nil
}

func (r *ArtifactContinuationRepository) GetPlan(_ context.Context, id common.ArtifactContinuationID) (map[string]any, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.items[id]; !ok {
		return nil, common.ErrNotFound
	}
	plan, ok := r.plans[id]
	if !ok {
		return nil, common.ErrNotFound
	}
	return cloneMap(plan), nil
}

func (r *ArtifactContinuationRepository) SaveChunk(_ context.Context, chunk *orchestratormodel.ArtifactContinuationChunk, content string) error {
	if chunk == nil || chunk.ContinuationID == "" || chunk.Sequence <= 0 {
		return common.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[chunk.ContinuationID]; !ok {
		return common.ErrNotFound
	}
	if r.chunks[chunk.ContinuationID] == nil {
		r.chunks[chunk.ContinuationID] = map[int]*orchestratormodel.ArtifactContinuationChunk{}
	}
	if r.chunkContent[chunk.ContinuationID] == nil {
		r.chunkContent[chunk.ContinuationID] = map[int]string{}
	}
	r.chunks[chunk.ContinuationID][chunk.Sequence] = cloneArtifactContinuationChunk(chunk)
	r.chunkContent[chunk.ContinuationID][chunk.Sequence] = content
	return nil
}

func (r *ArtifactContinuationRepository) GetChunk(_ context.Context, id common.ArtifactContinuationID, sequence int) (*orchestratormodel.ArtifactContinuationChunk, string, error) {
	if id == "" || sequence <= 0 {
		return nil, "", common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.items[id]; !ok {
		return nil, "", common.ErrNotFound
	}
	chunk, ok := r.chunks[id][sequence]
	if !ok {
		return nil, "", common.ErrNotFound
	}
	return cloneArtifactContinuationChunk(chunk), r.chunkContent[id][sequence], nil
}

func (r *ArtifactContinuationRepository) ListChunks(_ context.Context, id common.ArtifactContinuationID) ([]*orchestratormodel.ArtifactContinuationChunk, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.items[id]; !ok {
		return nil, common.ErrNotFound
	}
	ret := make([]*orchestratormodel.ArtifactContinuationChunk, 0, len(r.chunks[id]))
	for _, item := range r.chunks[id] {
		ret = append(ret, cloneArtifactContinuationChunk(item))
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
	if _, ok := r.items[item.ContinuationID]; !ok {
		return common.ErrNotFound
	}
	r.assembledMeta[item.ContinuationID] = cloneArtifactContinuationAssembledMetadata(item)
	r.assembledContent[item.ContinuationID] = content
	return nil
}

func (r *ArtifactContinuationRepository) GetAssembled(_ context.Context, id common.ArtifactContinuationID) (*orchestratormodel.ArtifactContinuationAssembledMetadata, string, error) {
	if id == "" {
		return nil, "", common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.items[id]; !ok {
		return nil, "", common.ErrNotFound
	}
	item, ok := r.assembledMeta[id]
	if !ok {
		return nil, "", common.ErrNotFound
	}
	return cloneArtifactContinuationAssembledMetadata(item), r.assembledContent[id], nil
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
	cloned.Metadata = cloneMap(item.Metadata)
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
	cloned.Validation = cloneMap(item.Validation)
	cloned.Metadata = cloneMap(item.Metadata)
	return &cloned
}

func cloneArtifactContinuationAssembledMetadata(item *orchestratormodel.ArtifactContinuationAssembledMetadata) *orchestratormodel.ArtifactContinuationAssembledMetadata {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.SourceChunkRefs = cloneStringSlice(item.SourceChunkRefs)
	cloned.ApplyEvidenceRefs = cloneStringSlice(item.ApplyEvidenceRefs)
	cloned.Metadata = cloneMap(item.Metadata)
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
