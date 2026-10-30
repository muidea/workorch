package biz

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/modules/application/workorchd/service/server"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	decisionowner "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	fileowner "github.com/muidea/workorch/internal/modules/blocks/file/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	mcpowner "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/biz"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memoryowner "github.com/muidea/workorch/internal/modules/blocks/memory/biz"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runowner "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	skillowner "github.com/muidea/workorch/internal/modules/blocks/skill/biz"
	skillmodel "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/models"
	soulowner "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspaceowner "github.com/muidea/workorch/internal/modules/kernel/workspace/biz"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/testkit"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

// 使用正式 HTTP ports 和真实持久 owners；仅 provider 与任务排队由测试控制。
func TestAuthoringHTTPFileBackedCompilationConfirmationAndRecovery(t *testing.T) {
	for _, scenario := range []string{"pass", "review_fail", "review_unknown", "review_missing_required", "resource_drift", "invalid_derivation", "invalid_proposal", "invalid_review"} {
		t.Run(scenario, func(t *testing.T) { testAuthoringHTTPFileBackedScenario(t, scenario) })
	}
}

func TestStartupPreparationHTTPFileOwnersReadOnlyAndReopen(t *testing.T) {
	testAuthoringHTTPFileBackedScenario(t, "step04_read_only")
}

func TestPreparationWorkflowHTTPFileOwnersBlockedAndReopen(t *testing.T) {
	for _, scenario := range []string{"step04_workflow", "step04_workflow_empty_selection", "step04_workflow_mcp_selected", "step04_workflow_mcp_missing", "step04_workflow_mcp_disabled", "step04_workflow_skill_refs_missing", "step04_workflow_final_skill_drift", "step04_workflow_final_skill_unavailable"} {
		t.Run(scenario, func(t *testing.T) { testAuthoringHTTPFileBackedScenario(t, scenario) })
	}
}

type preparationHTTPSoulRepository struct {
	repository.SoulRepository
	reads *atomic.Int32
}

type preparationHTTPSkillRepository struct {
	repository.SkillRepository
	reads       *atomic.Int32
	finalChange string
}

type preparationHTTPMCPRepository struct {
	repository.MCPRegistryRepository
	reads *atomic.Int32
}

func (r *preparationHTTPMCPRepository) Load(ctx context.Context) (*mcpmodel.RegistrySnapshot, error) {
	r.reads.Add(1)
	return r.MCPRegistryRepository.Load(ctx)
}

func (r *preparationHTTPSkillRepository) List(ctx context.Context) ([]*skillmodel.Skill, error) {
	if r.reads.Add(1) >= 3 {
		if r.finalChange == "step04_workflow_final_skill_unavailable" {
			return nil, common.ErrNotFound
		}
		if r.finalChange == "step04_workflow_final_skill_drift" {
			skills, err := r.SkillRepository.List(ctx)
			if err != nil || len(skills) != 1 {
				return nil, common.ErrConflict
			}
			skills[0].Instruction = "真实最终指令变化"
			skills[0].Provenance.ContentHash = workorchskillcontract.StableContentHash(skills[0].Content())
			if err := r.SkillRepository.Update(ctx, skills[0]); err != nil {
				return nil, err
			}
		}
	}
	return r.SkillRepository.List(ctx)
}

func (r *preparationHTTPSoulRepository) Get(ctx context.Context, id common.SoulID) (*soulmodel.Soul, error) {
	r.reads.Add(1)
	return r.SoulRepository.Get(ctx, id)
}

func TestAuthoringHTTPCreationCutpointRecovery(t *testing.T) {
	for _, scenario := range []string{"creation_binding", "creation_binding_empty", "creation_index", "creation_directory"} {
		t.Run(scenario, func(t *testing.T) { testAuthoringHTTPFileBackedScenario(t, scenario) })
	}
}

func TestAuthoringHTTPPreparationFailureFileRecovery(t *testing.T) {
	for _, scenario := range []string{"resource_missing", "config_failure", "proposal_config_failure", "review_preparation_failure", "review_fail_preparation_failure", "review_unknown_preparation_failure", "render_failure", "request_budget", "attempt_budget"} {
		t.Run(scenario, func(t *testing.T) { testAuthoringHTTPFileBackedScenario(t, scenario) })
	}
}

func TestAuthoringHTTPRealBackgroundRoutineFileRecovery(t *testing.T) {
	testAuthoringHTTPFileBackedScenario(t, "real_background")
}

func TestAuthoringHTTPConcurrentCompileUsesOnePhysicalDerivation(t *testing.T) {
	testAuthoringHTTPFileBackedScenario(t, "concurrent_compile")
}

type authoringConcurrentRoutine struct {
	task.BackgroundRoutine
	queued []task.Task
}

func (r *authoringConcurrentRoutine) AsyncTaskContext(_ context.Context, work task.Task) error {
	r.queued = append(r.queued, work)
	return nil
}

func TestAuthoringHTTPCompileTasksCompeteForFreshPermit(t *testing.T) {
	testAuthoringHTTPFileBackedScenario(t, "reserve_compete")
}

func TestAuthoringHTTPStep02ExactInputBeforeReserve(t *testing.T) {
	for _, scenario := range []string{"step02_input_guard", "step02_template_guard"} {
		t.Run(scenario, func(t *testing.T) { testAuthoringHTTPFileBackedScenario(t, scenario) })
	}
}

func TestAuthoringHTTPStep02CandidateReadbackWithoutConfirmation(t *testing.T) {
	testAuthoringHTTPFileBackedScenario(t, "step02_candidate")
}

func TestAuthoringHTTPStep03TicketDeliveryRecovery(t *testing.T) {
	for _, scenario := range []string{"step03_decision_unavailable", "step03_ticket_notification"} {
		t.Run(scenario, func(t *testing.T) { testAuthoringHTTPFileBackedScenario(t, scenario) })
	}
}

func testAuthoringHTTPFileBackedScenario(t *testing.T, scenario string) {
	ctx := context.Background()
	preparationWorkflow := strings.HasPrefix(scenario, "step04_workflow")
	emptySelection := scenario == "step04_workflow_empty_selection"
	finalSkillUnavailable := scenario == "step04_workflow_final_skill_unavailable"
	finalSkillChanged := scenario == "step04_workflow_final_skill_drift" || finalSkillUnavailable
	wantSkillReads := int32(4)
	if finalSkillUnavailable {
		wantSkillReads = 3
	}
	missingSkillRef := scenario == "step04_workflow_skill_refs_missing"
	referenceState := "observed_checks_pending"
	if missingSkillRef {
		referenceState = "unavailable"
	}
	skillStatusState := "status_blocked"
	if missingSkillRef || emptySelection {
		skillStatusState = "observed_freshness_pending"
	} else if scenario == "step04_workflow_mcp_selected" {
		skillStatusState = "version_conflict"
	}
	mcpFixture := strings.HasPrefix(scenario, "step04_workflow_mcp_")
	mcpFailure := map[string]string{"step04_workflow_mcp_missing": "capability_missing", "step04_workflow_mcp_disabled": "server_disabled"}[scenario]
	mcpReason := "mcp_selection_empty_policy_pending"
	if mcpFixture {
		mcpReason = "mcp_selection_compiled_connection_pending"
	}
	if mcpFailure != "" {
		mcpReason = "mcp_selection_" + mcpFailure
	}
	if emptySelection {
		mcpReason = workspace.PreparationEmptyMCPReason
	}
	storage, directory := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("可验证的资源\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if scenario == "render_failure" {
		// 模板和资源各自合法，但组装后的请求超过 render 文档边界。
		if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte(strings.Repeat("r", 30*1024)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if scenario == "resource_missing" {
		if err := os.Remove(filepath.Join(directory, "requirements.txt")); err != nil {
			t.Fatal(err)
		}
	}
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	hints, err := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"generic": map[string]any{"evidenceRequirements": []any{map[string]any{"id": "*", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	template := workspace.WorkspaceTemplate{ID: "http-template", Name: "模板", DoDItems: []string{"模板验收"}, ContextHints: hints}
	if emptySelection {
		// Explicit, reviewed fixture policy, never production defaults or a
		// post-confirmation repair of an incomplete original template.
		values := template.ContextHints.Map()
		values["acceptanceSlicePolicy"] = map[string]any{"schemaVersion": "workorch.acceptance_slice_policy", "maxWorkItems": 4, "maxFocuses": 64, "maxCandidateContextItems": 4, "completionPolicy": "all_required_items_accepted", "resultPolicy": "slice_result_allowed"}
		declaration := values["profileContracts"].(map[string]any)["generic"].(map[string]any)["evidenceRequirements"].([]any)[0].(map[string]any)
		fact := map[string]any{"ownerModuleId": runtimecontract.RuntimeModuleShell, "factKind": string(runtimecontract.RuntimeOwnerFactSelectableValidationSurface), "operationKind": runtimecontract.RuntimeBlockKindManagedValidationSession}
		declaration["requiredOwnerFacts"] = []any{fact}
		declaration["ownerFactConvergences"] = []any{map[string]any{"ownerModuleId": fact["ownerModuleId"], "factKind": fact["factKind"], "operationKind": fact["operationKind"], "requirePreexisting": true}}
		template.ContextHints, err = workorcheventpayload.FromMap(values)
		if err != nil {
			t.Fatal(err)
		}
	}
	preparationInstalledRoot := ""
	installedRaw := []byte("---\r\nname: HTTP 原始安装材料\r\nversion: http-installed-v1\r\ntriggers: [交付目标]\r\nrequired_tools: [shell]\r\nreference_refs: [references/exact.bin]\r\n---\r\n指令正文\x00\r\n")
	if preparationWorkflow {
		preparationInstalledRoot = t.TempDir()
		t.Setenv("CODEX_SKILLS_DIR", preparationInstalledRoot)
		template.DefaultSoulID = "http-soul"
		skill := &skillmodel.Skill{ID: "http-skill", Name: "HTTP Skill", Version: "v1", Instruction: "HTTP Skill 原文", TriggerRules: []string{"http"}}
		skill.Provenance = workorchskillcontract.Provenance{Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal, ContentHash: workorchskillcontract.StableContentHash(skill.Content())}
		if err := filerepository.NewSkillRepository(filepath.Join(storage, "system", "skills.json")).Create(ctx, skill); err != nil {
			t.Fatal(err)
		}
		s := &soulmodel.Soul{ID: "http-soul", Revision: 1, Name: "HTTP Soul", SoulMD: "HTTP Soul 原文\n", SourceProvenance: soulmodel.SoulSourceUserManaged}
		if emptySelection {
			// A real, exact Soul policy selects no matching skills, while the
			// entire nonempty registry and installed catalog are still observed.
			s.AllowedSkillRefs = []string{"not-selected-by-this-bounded-profile"}
		}
		s.ContentHash = soulmodel.StableSoulContentHash(s)
		if err := filerepository.NewSoulRepository(filepath.Join(storage, "system", "souls.json")).Create(ctx, s); err != nil {
			t.Fatal(err)
		}
		configs, err := workorcheventpayload.FromMap(map[string]any{"managedWorkspaceConfigs": []any{map[string]any{"path": "mcp/config.json", "content": map[string]any{"fixture": "original-managed-config"}}}})
		if err != nil {
			t.Fatal(err)
		}
		template.ContextHints = template.ContextHints.Merge(configs)
		if mcpFixture {
			installedRaw = []byte("---\r\nname: HTTP 原始安装材料\r\nversion: http-installed-v1\r\ntriggers: [交付目标]\r\nrequired_tools: [shell]\r\nrequired_mcp_server_refs: [http-mcp]\r\nrequired_capability_refs: [http-capability]\r\nreference_refs: [references/exact.bin]\r\n---\r\n指令正文\x00\r\n")
			// Fixture metadata only: the production owner derives the complete
			// content revision. No MCP transport or executable is started.
			snapshot := &mcpmodel.RegistrySnapshot{Revision: 1,
				Servers:      []*mcpmodel.Server{{ID: "http-mcp", Name: "HTTP MCP", Transport: "stdio", Command: "never-start", Enabled: true, AllowedTools: []string{"http-tool"}}},
				Capabilities: []*mcpmodel.Capability{{ID: "http-capability", ServerID: "http-mcp", Kind: "tool", Name: "http-tool", DisplayName: "HTTP tool", Description: "MCP 原文\r\n第二行", Enabled: true, Revision: 1, Source: "user", SourceRef: "user:http-capability"}},
			}
			if mcpFailure == "capability_missing" {
				snapshot.Capabilities = nil
			}
			if mcpFailure == "server_disabled" {
				snapshot.Servers[0].Enabled = false
				// Match the Registry owner's existing metadata invariant: an
				// enabled capability cannot belong to a disabled server.
				snapshot.Capabilities[0].Enabled = false
			}
			if err := filerepository.NewMCPRegistryRepository(filepath.Join(storage, "system", "mcp-registry.json")).Commit(ctx, 0, snapshot); err != nil {
				t.Fatal(err)
			}
		}
	}
	if strings.HasPrefix(scenario, "step03_") {
		values := hints.Map()
		values["profileContracts"].(map[string]any)["generic"].(map[string]any)["deliverableExpectations"] = []any{map[string]any{"id": "required-delivery", "required": true}}
		template.ContextHints, err = workorcheventpayload.FromMap(values)
		if err != nil {
			t.Fatal(err)
		}
	}
	if scenario == "render_failure" {
		template.DoDItems = append(template.DoDItems, strings.Repeat("模板原文", 20*1024))
	}
	if err := repositories.WorkspaceTemplates.Create(ctx, &template); err != nil {
		t.Fatal(err)
	}
	var preparationRepaired atomic.Bool
	var decisionRestored atomic.Bool
	configurationCause := def.NewError(def.Unexpected, "mock 配置读取失败\r\n原文 sk_test_marker\n")
	var physicalCalls atomic.Int32
	var preparationCacheReads atomic.Int32
	var preparationMemoryModeReads atomic.Int32
	var preparationMemoryModeEnabled atomic.Bool
	var preparationSoulReads atomic.Int32
	var preparationSkillReads atomic.Int32
	var preparationMCPReads atomic.Int32
	var preparationStatusCalls atomic.Int32
	var missingReviewPayload []byte
	var missingRequirementID string
	realWorker := scenario == "real_background" || scenario == "concurrent_compile"
	deriveStarted, releaseDerive := make(chan struct{}), make(chan struct{})
	var concurrentRoutine *authoringConcurrentRoutine
	var startedOnce, releaseOnce sync.Once
	if scenario == "concurrent_compile" {
		t.Cleanup(func() { releaseOnce.Do(func() { close(releaseDerive) }) })
	}
	open := func() (string, *authoringTestRoutine, func()) {
		t.Helper()
		hub := magicevent.NewHub(32)
		owners := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
		owner, err := workspaceowner.New(hub, nil, workspaceowner.Dependencies{OwnerRepositories: owners.WorkspaceOwner, EventHub: hub})
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		decision, err := decisionowner.New(hub, nil, decisionowner.Dependencies{OwnerRepositories: owners.DecisionOwner, EventHub: hub})
		if err != nil {
			t.Fatal(err)
		}
		if scenario != "step03_decision_unavailable" || decisionRestored.Load() {
			if err := decision.Start(); err != nil {
				t.Fatal(err)
			}
		}
		file, err := fileowner.New(hub, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Start(); err != nil {
			t.Fatal(err)
		}
		provider := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
		var preparationRuns *runowner.Biz
		if preparationWorkflow {
			preparationRuns = runowner.New()
			if err := preparationRuns.Configure(runowner.Dependencies{OwnerRepositories: owners.RunOwner, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			if err := preparationRuns.Start(); err != nil {
				t.Fatal(err)
			}
			if err := provider.Subscribe(llmevents.TopicObserveProviderCatalogCache, func(ev magicevent.Event, result magicevent.Result) {
				preparationCacheReads.Add(1)
				c, ok := ev.Data().(llmevents.ObserveProviderCatalogCacheCommand)
				if !ok {
					t.Error("wrong cache-only DTO")
					return
				}
				observed := llmevents.ObserveProviderCatalogCacheResult{WorkspaceID: c.WorkspaceID, OwnerEpoch: "llm-preparation-epoch:ABCDEFGHIJKLMNOPQRSTUVWXYZ", ObservedAt: time.Now().UTC(), Catalog: llmmodel.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", State: llmmodel.ProviderCatalogStateUnavailable, ErrorCode: "cache_miss"}}
				if emptySelection {
					observed.Configuration = &llmevents.PreparationProviderConfiguration{Version: llmevents.PreparationProviderConfigurationVersion, DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ModelID: "chat"}}
				}
				result.Set(observed, nil)
			}); err != nil {
				t.Fatal(err)
			}
		}
		target := runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "mock"}
		if err := provider.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, result magicevent.Result) {
			if preparationWorkflow && preparationMemoryModeEnabled.Load() {
				preparationMemoryModeReads.Add(1)
				result.Set(llmevents.GetActiveConfigurationResult{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ConfigurationRevision: "http-memory-mode", DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: runtimecontract.BuiltinAIProxyProviderID, ModelID: "chat"}}, nil)
				return
			}
			fail := !preparationRepaired.Load() && (scenario == "config_failure" || scenario == "proposal_config_failure" && physicalCalls.Load() >= 1 || (scenario == "review_preparation_failure" || scenario == "review_fail_preparation_failure" || scenario == "review_unknown_preparation_failure") && physicalCalls.Load() >= 3)
			if fail {
				result.Set(nil, configurationCause)
				return
			}
			result.Set(llmevents.GetActiveConfigurationResult{ConfigurationRevision: "http-provider", DefaultChat: target}, nil)
		}); err != nil {
			t.Fatal(err)
		}
		if err := provider.Subscribe(llmevents.TopicLLMCommandCompleteProviderDocument, func(ev magicevent.Event, result magicevent.Result) {
			command := ev.Data().(llmevents.CompleteProviderDocumentCommand)
			all, err := owners.Workspaces.List(ctx)
			if err != nil || len(all) != 1 {
				t.Errorf("provider workspace: %v", err)
				return
			}
			record := all[0].DefinitionAuthoring
			if record == nil {
				t.Error("provider called before persisted input")
				return
			}
			var payload []byte
			switch command.Request.Kind {
			case "workspace/specification/derive":
				if scenario == "concurrent_compile" {
					startedOnce.Do(func() { close(deriveStarted) })
					<-releaseDerive
				}
				if len(record.DerivationRequests) != 1 || len(record.DerivationRequests[0].Resources) != 1 {
					t.Error("provider called before File snapshot and permit")
					return
				}
				payload, err = json.Marshal(workspace.DefinitionDerivedAcceptance{AuthoringInputHash: record.InputHash, TemplateSnapshotHash: record.TemplateSnapshotHash, AdditionalItems: []string{"派生验收"}})
				if scenario == "invalid_derivation" {
					payload = []byte("  {\"providerControlled\": true}\n")
				}
			case "workspace/specification/proposal":
				if scenario == "proposal_config_failure" || scenario == "review_preparation_failure" || scenario == "review_fail_preparation_failure" || scenario == "review_unknown_preparation_failure" {
					if len(record.SpecificationRequests) < 1 {
						t.Error("proposal permit not durable")
						return
					}
				} else if len(record.SpecificationRequests) != 1 {
					t.Error("proposal permit not durable")
					return
				}
				mapping := authoringHTTPMapping(t, record)
				payload, err = json.Marshal(struct {
					Mapping workspace.DefinitionAuthoringMapping `json:"mapping"`
				}{mapping})
				if scenario == "invalid_proposal" {
					payload = []byte("  {\"providerControlled\": true}\n")
				}
			default:
				kind := strings.TrimPrefix(command.Request.Kind, "workspace/specification/review/")
				proposal := record.SpecificationRequests[0].Proposal
				if proposal == nil || len(record.SpecificationRequests) != int(physicalCalls.Load()) {
					t.Error("critic permit not durable")
					return
				}
				if strings.Contains(command.Request.PromptContext.Sections[1].Content, `"Reviews":[{`) {
					t.Error("critic received earlier reviews")
				}
				review := workspace.GoalSpecificationCompletenessReview{Kind: kind, Status: workspace.GoalReviewPass, SpecificationHash: proposal.ContentHash(), Reason: "独立覆盖", EvidenceRefs: []string{workspace.DefinitionAuthoringGoalItemRef}, RequirementIDs: authoringHTTPRequiredReviewRequirementIDs(t, *proposal, kind)}
				if scenario == "review_missing_required" && kind == workspace.GoalReviewVerifierFeasibility {
					goalRequirement := authoringHTTPGoalRequirementID(t, *proposal)
					missingRequirementID = goalRequirement
					review.RequirementIDs = review.RequirementIDs[:0]
					for _, id := range authoringHTTPRequiredReviewRequirementIDs(t, *proposal, kind) {
						if id != goalRequirement {
							review.RequirementIDs = append(review.RequirementIDs, id)
						}
					}
				}
				if physicalCalls.Load() == 2 {
					switch scenario {
					case "review_fail_preparation_failure":
						review.Status = workspace.GoalReviewFail
					case "review_unknown_preparation_failure":
						review.Status = workspace.GoalReviewUnknown
					}
				}
				if kind == workspace.GoalReviewVerifierFeasibility {
					switch scenario {
					case "review_fail":
						review.Status = workspace.GoalReviewFail
					case "review_unknown":
						review.Status = workspace.GoalReviewUnknown
					}
				}
				payload, err = json.Marshal(struct {
					Review workspace.GoalSpecificationCompletenessReview `json:"review"`
				}{review})
				if scenario == "review_missing_required" && kind == workspace.GoalReviewVerifierFeasibility {
					missingReviewPayload = testkit.OwnerBoundFixtureDocument(payload)
				}
				if scenario == "invalid_review" {
					payload = []byte("  {\"未知审查字段\":\"sk_test_marker\"}\r\n")
				}
			}
			if err != nil {
				t.Error(err)
				return
			}
			physicalCalls.Add(1)
			result.Set(authoringTestDocumentResult(t, command, payload), nil)
		}); err != nil {
			t.Fatal(err)
		}
		routine := &authoringTestRoutine{}
		var background task.BackgroundRoutine = routine
		if scenario == "reserve_compete" {
			concurrentRoutine = &authoringConcurrentRoutine{}
			background = concurrentRoutine
		}
		if realWorker {
			background = task.NewBackgroundRoutine(2)
		}
		application := &Biz{Base: basebiz.New(ModuleID, hub, background), authoringTasks: newAuthoringTaskLifecycle()}
		var component *runtimeComponent
		var preparationSouls *soulowner.Biz
		var preparationSkills *skillowner.Biz
		var preparationMemory *memoryowner.Biz
		var preparationMCP *mcpowner.Biz
		if preparationWorkflow {
			preparationMCP, err = mcpowner.New(hub, nil, mcpowner.Dependencies{Registry: &preparationHTTPMCPRepository{MCPRegistryRepository: filerepository.NewMCPRegistryRepository(filepath.Join(storage, "system", "mcp-registry.json")), reads: &preparationMCPReads}, EventHub: hub})
			if err != nil {
				t.Fatal(err)
			}
			if err := preparationMCP.Start(); err != nil {
				t.Fatal(err)
			}
			preparationMemory, err = memoryowner.New(hub, nil, memoryowner.Dependencies{EventHub: hub, OwnerRepositories: repositories.MemoryOwner})
			if err != nil {
				t.Fatal(err)
			}
			if err := preparationMemory.Start(); err != nil {
				t.Fatal(err)
			}
			preparationSkills, err = skillowner.New(hub, nil, skillowner.Dependencies{Skills: &preparationHTTPSkillRepository{SkillRepository: filerepository.NewSkillRepository(filepath.Join(storage, "system", "skills.json")), reads: &preparationSkillReads, finalChange: scenario}})
			if err != nil {
				t.Fatal(err)
			}
			if err := preparationSkills.Start(); err != nil {
				t.Fatal(err)
			}
			preparationSouls, err = soulowner.New(hub, nil, soulowner.Dependencies{Souls: &preparationHTTPSoulRepository{SoulRepository: filerepository.NewSoulRepository(filepath.Join(storage, "system", "souls.json")), reads: &preparationSoulReads}})
			if err != nil {
				t.Fatal(err)
			}
			if err := preparationSouls.Start(); err != nil {
				t.Fatal(err)
			}
			// Same production port adapter; only its process configuration and
			// provider are local fixtures, never the user's live configuration.
			session, err := runtimecore.NewSessionFromExports(bootstrap.Exports{Infrastructure: bootstrap.InfrastructureExports{CommandHub: hub, BackgroundRoutine: background, LoadConfig: func() (workorchconfig.Config, error) {
				cfg := workorchconfig.Config{Runtime: workorchconfig.RuntimeConfig{DefaultSoulID: "unused-system-soul"}}
				if emptySelection {
					cfg.Runtime.MaxRuntimeRepairAttempts = 10
					cfg.Runtime.LLMOutput.MaxDocumentRecoveryAttempts = 3
				}
				return cfg, nil
			}}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			component = newRuntimeComponent(session)
		} else {
			component, err = newRuntimeComponentFromCommandHub(hub, background)
			if err != nil {
				t.Fatal(err)
			}
		}
		component.session.SetRepositoryConfig("file", storage)
		// No test may invoke the machine's configured SkillHub/service.
		component.ports = newApplicationPortsWithPreparationRunner(component.withSession, nil, component.authoringTasks)
		if preparationWorkflow {
			runner := preparationStatusRunnerFunc(func(callCtx context.Context, directory string, global bool) (contextregistryports.SkillHubPreparationOutput, error) {
				items, err := repositories.Workspaces.List(callCtx)
				if err != nil || len(items) != 1 || len(items[0].PreparationSkillHubEffects) != 1 || items[0].PreparationSkillHubEffects[0].Summary() != workspace.PreparationSkillHubReserved {
					t.Error("status preceded durable reservation")
					return contextregistryports.SkillHubPreparationOutput{}, common.ErrConflict
				}
				plan := items[0].PreparationSkillHubEffects[0]
				call := preparationStatusCalls.Add(1)
				if call > 2 || global != (call == 1) || (!global && directory != plan.ProjectDirectory) || (global && directory != "") {
					t.Error("wrong/repeated status target")
					return contextregistryports.SkillHubPreparationOutput{}, common.ErrConflict
				}
				status := contextregistrycore.SkillHubStatus{Scope: "global", SkillCount: 1, Items: []contextregistrycore.SkillHubStatusItem{{SkillID: "http-installed", Agent: "codex", TargetPath: filepath.Join(preparationInstalledRoot, "http-installed")}}}
				if !global {
					status = contextregistrycore.SkillHubStatus{Scope: "project", ProjectPath: directory, SkillCount: 1, Items: []contextregistrycore.SkillHubStatusItem{{SkillID: "http-installed", LocalPath: filepath.Join(directory, ".agents", "skills", "http-installed", "SKILL.md")}}}
				}
				status.Items[0].Status, status.Items[0].LocalVersion = "modified", "http-installed-v1"
				if skillStatusState == "observed_freshness_pending" || skillStatusState == "version_conflict" {
					status.Items[0].Status = "synced"
				}
				if skillStatusState == "version_conflict" {
					status.Items[0].LocalVersion = "http-status-other-v1"
				}
				raw, _ := json.Marshal(status)
				return contextregistryports.SkillHubPreparationOutput{Bytes: raw, TotalBytes: uint64(len(raw)), CommandStarted: true, ExitCodeKnown: true}, nil
			})
			component.ports = newApplicationPortsWithPreparationRunner(component.withSession, runner, component.authoringTasks)
		}
		ports := component.applicationPorts()
		application.configureAuthoringBackgroundPorts(&ports)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address := listener.Addr().String()
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
		transport := server.NewHTTPServerWithApplicationPorts(server.Config{Addr: address, Backend: "file", DataRoot: storage}, ports, background)
		finished := make(chan error, 1)
		go func() { finished <- transport.Start() }()
		baseURL := "http://" + address
		client := &http.Client{Timeout: time.Second}
		deadline := time.Now().Add(5 * time.Second)
		for {
			response, err := client.Get(baseURL + "/healthz")
			if err == nil {
				response.Body.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("HTTP listener did not start")
			}
			time.Sleep(10 * time.Millisecond)
		}
		return baseURL, routine, func() {
			shutdown, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := transport.Shutdown(shutdown); err != nil {
				t.Error(err)
			}
			application.authoringTasks.stop()
			if realWorker && !background.Shutdown(shutdown) {
				t.Error("真实后台编译 worker 未停止")
			}
			if err := <-finished; err != nil {
				t.Error(err)
			}
			if err := component.close(shutdown); err != nil {
				t.Error(err)
			}
			provider.Unsubscribe(llmevents.TopicLLMCommandCompleteProviderDocument)
			provider.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration)
			if preparationRuns != nil {
				provider.Unsubscribe(llmevents.TopicObserveProviderCatalogCache)
				preparationRuns.Stop(ctx)
			}
			if preparationSouls != nil {
				preparationSouls.Teardown(ctx)
			}
			if preparationSkills != nil {
				preparationSkills.Teardown(ctx)
			}
			if preparationMemory != nil {
				preparationMemory.Teardown(ctx)
			}
			if preparationMCP != nil {
				preparationMCP.Teardown(ctx)
			}
			file.Teardown(ctx)
			decision.Teardown(ctx)
			owner.Teardown(ctx)
			hub.Terminate(ctx)
		}
	}
	baseURL, routine, closeOwners := open()
	defer func() {
		if scenario == "concurrent_compile" {
			releaseOnce.Do(func() { close(releaseDerive) })
		}
		closeOwners()
	}()
	request := management.DefinitionAuthoringCreate{RequestID: "http-file-recovery", Title: "HTTP 创作", TemplateID: "http-template", DeliverableDirectory: directory, Input: management.DefinitionAuthoringInput{GoalText: "交付目标", AcceptanceRequirements: []string{"用户验收"}, ResourceRefs: []string{"requirements.txt"}}}
	assertNoRuns := func() {
		t.Helper()
		runs, err := repositories.Runs.List(ctx)
		if err != nil || len(runs) != 0 {
			t.Fatalf("创作链路自动创建 Run: %v %v", runs, err)
		}
	}
	if strings.HasPrefix(scenario, "creation_") {
		id := workspace.DefinitionAuthoringWorkspaceID(request.RequestID)
		obstacle := filepath.Join(storage, "internal", "workspaces", string(id), "config")
		if scenario == "creation_binding_empty" {
			obstacle = filepath.Join(obstacle, "soul_bindings.json")
		}
		if scenario == "creation_index" {
			obstacle = filepath.Join(storage, "system", "workspace_index.json")
		}
		if scenario == "creation_directory" {
			obstacle = filepath.Join(directory, "blocked")
			request.DeliverableDirectory = filepath.Join(obstacle, "project")
		}
		if scenario == "creation_index" {
			if err := os.MkdirAll(obstacle, 0o755); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(obstacle), 0o755); err != nil {
				t.Fatal(err)
			}
			content := []byte("创建恢复测试障碍")
			if scenario == "creation_binding_empty" {
				content = nil // The real repository returns ErrInvalidArgument after metadata is saved.
			}
			if err := os.WriteFile(obstacle, content, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		payload, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.Post(baseURL+"/api/v1/workorch/workspace-authoring", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		var recovery struct {
			Code     string                                          `json:"code"`
			Recovery *management.DefinitionAuthoringCreationRecovery `json:"creationRecovery"`
		}
		err = json.NewDecoder(response.Body).Decode(&recovery)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusServiceUnavailable || recovery.Code != "workspace_creation_incomplete" || recovery.Recovery == nil || recovery.Recovery.WorkspaceID != string(id) || recovery.Recovery.RequestID != request.RequestID || !recovery.Recovery.RetrySameRequest {
			t.Fatalf("missing public creation recovery: %d %+v %v", response.StatusCode, recovery, err)
		}
		expectedPersistence, expectedPhase := "unknown", "owner_persistence"
		if scenario == "creation_directory" {
			expectedPersistence, expectedPhase = "persisted", "directory_preparation"
		}
		if recovery.Recovery.Persistence != expectedPersistence || recovery.Recovery.Phase != expectedPhase {
			t.Fatalf("fabricated committed stage: %+v", recovery.Recovery)
		}
		if scenario == "creation_binding_empty" {
			raw, err := os.ReadFile(filepath.Join(storage, "internal", "workspaces", string(id), "workspace.json"))
			if err != nil || !bytes.Contains(raw, []byte(request.RequestID)) {
				t.Fatalf("regression must reach durable metadata before invalid bindings: %s %v", raw, err)
			}
		}
		assertNoRuns()
		closeOwners()
		// 仅移开测试故障障碍，保全内容；生产恢复没有删除动作。
		if err := os.Rename(obstacle, obstacle+".preserved"); err != nil {
			t.Fatal(err)
		}
		baseURL, routine, closeOwners = open()
		created := authoringHTTPRequest(t, baseURL, http.MethodPost, "/api/v1/workorch/workspace-authoring", request, http.StatusCreated)
		if created.WorkspaceID != string(id) {
			t.Fatal("recovery created different identity")
		}
		items, err := repositories.Workspaces.List(ctx)
		if err != nil || len(items) != 1 {
			t.Fatalf("public recovery missing index: %+v %v", items, err)
		}
		assertNoRuns()
		return
	}
	created := authoringHTTPRequest(t, baseURL, http.MethodPost, "/api/v1/workorch/workspace-authoring", request, http.StatusCreated)
	assertNoRuns()
	if scenario == "step04_read_only" {
		// Authoring creation is fixture setup, not a preparation effect. Take
		// all baselines only after its owner receipts have returned.
		snapshotFiles := func() map[string][32]byte {
			t.Helper()
			files := make(map[string][32]byte)
			for _, root := range []string{storage, directory} {
				if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if entry.IsDir() {
						return nil
					}
					data, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					files[path] = sha256.Sum256(data)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			return files
		}
		for reopen := 0; reopen < 2; reopen++ {
			before := snapshotFiles()
			stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
			if err != nil {
				t.Fatal(err)
			}
			baselineCalls := physicalCalls.Load()
			for read := 0; read < 3; read++ {
				client := &http.Client{Timeout: 2 * time.Second}
				response, err := client.Get(baseURL + "/api/v1/workorch/workspaces/" + created.WorkspaceID + "/activation-preparation")
				if err != nil {
					t.Fatal(err)
				}
				var body struct {
					Preparation management.StartupPreparationView `json:"preparation"`
				}
				decodeErr := json.NewDecoder(response.Body).Decode(&body)
				response.Body.Close()
				if decodeErr != nil || response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" || body.Preparation.ValidateFor(created.WorkspaceID) != nil || body.Preparation.WorkspaceRevision != stored.Revision || body.Preparation.Record != nil || body.Preparation.OperationState != "not_checked" {
					t.Fatalf("real HTTP projection: %+v status=%d err=%v", body, response.StatusCode, decodeErr)
				}
			}
			if !reflect.DeepEqual(before, snapshotFiles()) || physicalCalls.Load() != baselineCalls {
				t.Fatal("GET wrote resource/owner state or called provider")
			}
			assertNoRuns()
			if reopen == 0 {
				closeOwners()
				baseURL, routine, closeOwners = open()
			}
		}
		return
	}
	path := "/api/v1/workorch/workspaces/" + created.WorkspaceID + "/authoring"
	replay := authoringHTTPRequest(t, baseURL, http.MethodPost, "/api/v1/workorch/workspace-authoring", request, http.StatusCreated)
	if replay.InputHash != created.InputHash || replay.WorkspaceRevision != created.WorkspaceRevision {
		t.Fatal("create replay changed identity")
	}
	save := management.DefinitionAuthoringSave{RequestID: "save-http", TemplateID: request.TemplateID, Input: request.Input, ExpectedWorkspaceRevision: created.WorkspaceRevision, ExpectedInputHash: created.InputHash}
	save.Input.GoalText = "修订后的交付目标"
	updated := authoringHTTPRequest(t, baseURL, http.MethodPut, path, save, http.StatusOK)
	if updated.InputRevision != 2 || updated.InputHash == created.InputHash {
		t.Fatal("修订未形成新的输入身份")
	}
	closeOwners()
	baseURL, routine, closeOwners = open()
	saveReplay := authoringHTTPRequest(t, baseURL, http.MethodPut, path, save, http.StatusOK)
	if saveReplay.InputHash != updated.InputHash || saveReplay.WorkspaceRevision != updated.WorkspaceRevision {
		t.Fatal("保存响应丢失重放再次写入 revision")
	}
	conflictingSave := save
	conflictingSave.Input.GoalText = "过期不同输入"
	authoringHTTPRequest(t, baseURL, http.MethodPut, path, conflictingSave, http.StatusConflict)
	assertNoRuns()
	if scenario == "request_budget" || scenario == "attempt_budget" {
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil {
			t.Fatal(err)
		}
		record := stored.DefinitionAuthoring.Clone()
		count := workspace.DefinitionDerivationRequestLimit
		if scenario == "attempt_budget" {
			count = workspace.DefinitionDerivationAttemptLimit
		}
		content := "可验证的资源\n"
		sum := sha256.Sum256([]byte(content))
		resources := []workspace.DefinitionAuthoringResourceSnapshot{{Ref: "requirements.txt", Content: content, ContentHash: "sha256:" + hex.EncodeToString(sum[:])}}
		configuration := llmevents.GetActiveConfigurationResult{ConfigurationRevision: "http-provider", DefaultChat: runtimecontract.ProviderModelTarget{ProviderID: "mock", ModelID: "mock"}}
		for n := range count {
			input := record.Input.Clone()
			if scenario == "request_budget" {
				input.Revision = 1
				input.GoalText = fmt.Sprintf("历史目标-%d", n)
			}
			request := workspace.DefinitionDerivationRequest{RequestID: fmt.Sprintf("historical-%d", n), Input: input, Template: record.TemplateSnapshot.Clone(), Resources: resources, ResourceSnapshotHash: workspace.DefinitionAuthoringResourcesHash(resources), ProviderBindingHash: definitionProviderBindingHash(configuration), FailureCode: "provider_failed"}
			request.PermitID = request.ExpectedPermitID()
			record.DerivationRequests = append(record.DerivationRequests, request)
		}
		if err := record.Validate(); err != nil {
			t.Fatal(err)
		}
		stored.DefinitionAuthoring = &record
		if err := repositories.Workspaces.CompareAndSwap(ctx, stored, stored.Revision); err != nil {
			t.Fatal(err)
		}
	}
	compileRequest := management.DefinitionAuthoringCompile{ExpectedInputHash: updated.InputHash, ExpectedTemplateSnapshotHash: updated.TemplateSnapshotHash}
	if strings.HasPrefix(scenario, "step02_") {
		// Missing/partial conditions never reach a task, reserve or provider.
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", nil, http.StatusBadRequest)
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", management.DefinitionAuthoringCompile{ExpectedInputHash: updated.InputHash}, http.StatusBadRequest)
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", management.DefinitionAuthoringCompile{ExpectedInputHash: "stale", ExpectedTemplateSnapshotHash: updated.TemplateSnapshotHash}, http.StatusConflict)
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", management.DefinitionAuthoringCompile{ExpectedInputHash: updated.InputHash, ExpectedTemplateSnapshotHash: "stale"}, http.StatusConflict)
		if physicalCalls.Load() != 0 {
			t.Fatal("rejected compile performed a provider call")
		}
	}
	accepted := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusAccepted)
	if !accepted.CompilationAccepted || accepted.State != "draft" || (!realWorker && physicalCalls.Load() != 0) {
		t.Fatal("acceptance fabricated owner progress")
	}
	if scenario == "step02_input_guard" || scenario == "step02_template_guard" {
		if scenario == "step02_input_guard" {
			// A -> B -> A keeps the exact original text but must not reuse identity.
			current := updated
			for index, goal := range []string{"其他会话的新目标", updated.Input.GoalText} {
				input := *updated.Input
				input.GoalText = goal
				current = authoringHTTPRequest(t, baseURL, http.MethodPut, path, management.DefinitionAuthoringSave{RequestID: fmt.Sprintf("step02-revise-%d", index), TemplateID: current.TemplateID, Input: input, ExpectedWorkspaceRevision: current.WorkspaceRevision, ExpectedInputHash: current.InputHash}, http.StatusOK)
			}
			if current.InputHash == updated.InputHash || current.InputRevision != updated.InputRevision+2 {
				t.Fatal("A-B-A reused old input identity")
			}
		} else {
			changed := template.Clone()
			changed.DoDItems = append(changed.DoDItems, "新增模板必需承诺")
			if err := repositories.WorkspaceTemplates.Update(ctx, &changed); err != nil {
				t.Fatal(err)
			}
		}
		routine.queued.Run() // Delayed accepted task must not select the new input.
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusConflict)
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || physicalCalls.Load() != 0 || len(stored.DefinitionAuthoring.DerivationRequests) != 0 || len(stored.DefinitionAuthoring.SpecificationRequests) != 0 {
			t.Fatalf("stale task reached reserve/provider: %v calls=%d", err, physicalCalls.Load())
		}
		assertNoRuns()
		return
	}
	if scenario == "concurrent_compile" {
		select {
		case <-deriveStarted:
		case <-time.After(5 * time.Second):
			t.Fatal("并发编译派生调用未进入 barrier")
		}
		// 派生物理调用仍在途时，重复请求只能复用 owner permit。
		for range 4 {
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusOK)
		}
		releaseOnce.Do(func() { close(releaseDerive) })
	}
	if scenario == "reserve_compete" {
		for range 3 {
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusAccepted)
		}
		if len(concurrentRoutine.queued) != 4 {
			t.Fatalf("竞争夹具任务数=%d，want 4", len(concurrentRoutine.queued))
		}
		start := make(chan struct{})
		var workers sync.WaitGroup
		for _, work := range concurrentRoutine.queued {
			workers.Add(1)
			go func(work task.Task) {
				defer workers.Done()
				<-start
				work.Run()
			}(work)
		}
		close(start)
		workers.Wait()
	} else if !realWorker {
		routine.queued.Run()
	}
	if scenario == "resource_missing" || scenario == "config_failure" || scenario == "proposal_config_failure" || scenario == "review_preparation_failure" || scenario == "review_fail_preparation_failure" || scenario == "review_unknown_preparation_failure" || scenario == "render_failure" || scenario == "request_budget" || scenario == "attempt_budget" {
		failed := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		stage, phase := "derivation", "config"
		wantCalls := int32(0)
		switch scenario {
		case "resource_missing":
			phase = "resources"
		case "proposal_config_failure":
			stage, wantCalls = "proposal", 1
		case "review_preparation_failure", "review_fail_preparation_failure", "review_unknown_preparation_failure":
			stage, wantCalls = "review", 3
		case "render_failure":
			phase = "render"
		case "request_budget", "attempt_budget":
			phase = scenario
		}
		if failed.State != "preparation_failed" || len(failed.PreparationDiagnostics) != 1 || physicalCalls.Load() != wantCalls {
			t.Fatalf("pre-reserve failure hidden or permit issued: %+v calls=%d", failed, physicalCalls.Load())
		}
		diagnostic := failed.PreparationDiagnostics[0]
		if diagnostic.Stage != stage || diagnostic.PhaseCode != phase || !diagnostic.Current || !diagnostic.CauseSaved || diagnostic.CauseBytes == 0 || diagnostic.CauseSHA256 == "" || (scenario == "resource_missing" && diagnostic.ResourceRef != "requirements.txt") {
			t.Fatalf("preparation metadata incomplete: %+v", diagnostic)
		}
		var envelope struct {
			Diagnostic management.DefinitionAuthoringPreparationDiagnosticView `json:"diagnostic"`
		}
		response, err := http.Get(baseURL + path + "/preparation-diagnostics/" + diagnostic.DiagnosticID)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
			response.Body.Close()
			t.Fatalf("preparation raw query status=%d headers=%v", response.StatusCode, response.Header)
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		raw, err := base64.StdEncoding.DecodeString(envelope.Diagnostic.CauseBase64)
		if err != nil || uint64(len(raw)) != diagnostic.CauseBytes || envelope.Diagnostic.CauseSHA256 != diagnostic.CauseSHA256 {
			t.Fatalf("preparation raw integrity: %+v err=%v", envelope.Diagnostic, err)
		}
		if phase == "config" && !bytes.Equal(raw, []byte(configurationCause.Error())) {
			t.Fatalf("configuration original cause rewritten: %q", raw)
		}
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || stored.DefinitionAuthoring == nil || len(stored.DefinitionAuthoring.PreparationDiagnostics) != 1 || !bytes.Equal(stored.DefinitionAuthoring.PreparationDiagnostics[0].Cause, raw) {
			t.Fatalf("owner original cause mismatch: %+v %v", stored, err)
		}
		if len(stored.DefinitionAuthoring.SpecificationRequests) != int(max(0, wantCalls-1)) {
			t.Fatal("preparation failure fabricated specification permit")
		}
		assertNoRuns()
		// 相同显式失败幂等，但仍可接受任务；诊断身份不含
		// 已推进到当前值的 observed revision。
		again := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusAccepted)
		if !again.CompilationAccepted || again.State != "preparation_failed" {
			t.Fatal("explicit preparation retry was not accepted")
		}
		routine.queued.Run()
		duplicate := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		if duplicate.WorkspaceRevision != failed.WorkspaceRevision || len(duplicate.PreparationDiagnostics) != 1 || duplicate.PreparationDiagnostics[0].DiagnosticID != diagnostic.DiagnosticID || physicalCalls.Load() != wantCalls {
			t.Fatalf("duplicate preparation cause advanced owner revision: %+v", duplicate)
		}
		closeOwners()
		baseURL, routine, closeOwners = open()
		recovered := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		if recovered.State != "preparation_failed" || recovered.WorkspaceRevision != failed.WorkspaceRevision || recovered.PreparationDiagnostics[0].DiagnosticID != diagnostic.DiagnosticID {
			t.Fatal("preparation history lost across file-owner restart")
		}
		if scenario == "render_failure" || scenario == "request_budget" || scenario == "attempt_budget" {
			return
		}
		if scenario == "resource_missing" {
			if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("可验证的资源\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		preparationRepaired.Store(true)
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusAccepted)
		routine.queued.Run()
		ready := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		wantState := "ready_to_confirm"
		if scenario == "review_fail_preparation_failure" || scenario == "review_unknown_preparation_failure" {
			wantState = "review_blocked"
		}
		if ready.State != wantState || ready.InputHash != failed.InputHash || ready.Proposal == nil || len(ready.Proposal.Reviews) != 6 || physicalCalls.Load() != 8 || ready.PreparationDiagnostics[0].Current {
			t.Fatalf("explicit same-input recovery failed or repeated completed slots: %+v calls=%d", ready, physicalCalls.Load())
		}
		if wantState == "review_blocked" {
			confirmation := management.DefinitionAuthoringConfirm{InputHash: ready.InputHash, DerivationPermitID: ready.DerivationPermitID, ProposalHash: ready.Proposal.ProposalHash, SpecificationHash: ready.Proposal.SpecificationHash, ExpectedWorkspaceRevision: ready.WorkspaceRevision}
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusConflict)
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusOK)
			if physicalCalls.Load() != 8 {
				t.Fatal("完整阻塞审查重复编译重发了旧 critics")
			}
		}
		assertNoRuns()
		return
	}
	if scenario == "invalid_derivation" || scenario == "invalid_proposal" || scenario == "invalid_review" || scenario == "review_missing_required" {
		failed := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		requestIndex := len(failed.Requests) - 1
		failureCode := scenario
		if scenario == "review_missing_required" {
			failureCode = "invalid_review"
		}
		if scenario == "invalid_derivation" {
			failureCode = "invalid_proposal"
			if physicalCalls.Load() != 1 || failed.DerivationPermitID == "" || len(failed.Requests) != 1 || failed.Requests[0].Phase != "derivation" || failed.Requests[0].PermitID != failed.DerivationPermitID {
				t.Fatalf("派生失败继续调用或丢失诊断身份: %+v calls=%d", failed, physicalCalls.Load())
			}
		}
		if failed.State != "failed" || requestIndex < 0 || failed.Requests[requestIndex].FailureCode != failureCode || failed.Requests[requestIndex].Diagnostic == nil || failed.Requests[requestIndex].Diagnostic.ParseError != "" || failed.Requests[requestIndex].Diagnostic.ParseErrorBytes == 0 || !failed.Requests[requestIndex].Diagnostic.ParseErrorSaved {
			t.Fatalf("invalid proposal was not durably diagnosed: %+v", failed)
		}
		var envelope struct {
			Diagnostic management.DefinitionAuthoringDiagnosticView `json:"diagnostic"`
		}
		response, err := http.Get(baseURL + path + "/diagnostics/" + failed.Requests[requestIndex].PermitID)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("diagnostic status %d: %s", response.StatusCode, body)
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		raw := []byte("  {\"providerControlled\": true}\n")
		if scenario == "invalid_review" {
			raw = []byte("  {\"未知审查字段\":\"sk_test_marker\"}\r\n")
		}
		if scenario == "review_missing_required" {
			raw = missingReviewPayload
			if len(raw) == 0 {
				t.Fatal("missing-required review payload was not captured")
			}
		}
		if envelope.Diagnostic.ParseError == "" || envelope.Diagnostic.ParseErrorSHA256 != failed.Requests[requestIndex].Diagnostic.ParseErrorSHA256 || envelope.Diagnostic.ParseErrorBytes != failed.Requests[requestIndex].Diagnostic.ParseErrorBytes || envelope.Diagnostic.PayloadBase64 != base64.StdEncoding.EncodeToString(raw) || envelope.Diagnostic.PayloadBytes != uint64(len(raw)) || !envelope.Diagnostic.PayloadSaved {
			t.Fatalf("raw diagnostic changed: %+v", envelope.Diagnostic)
		}
		before := physicalCalls.Load()
		replayed := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusOK)
		if physicalCalls.Load() != before || replayed.CompilationAccepted || replayed.WorkspaceRevision != failed.WorkspaceRevision {
			t.Fatal("known failure automatically retried provider or changed its receipt")
		}
		assertNoRuns()
		closeOwners()
		baseURL, routine, closeOwners = open()
		recovered := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusOK)
		if recovered.State != "failed" || recovered.CompilationAccepted || recovered.WorkspaceRevision != failed.WorkspaceRevision || len(recovered.Requests) != len(failed.Requests) || physicalCalls.Load() != before {
			t.Fatalf("重启后失败请求身份被替换或重新调用 provider: %+v", recovered)
		}
		request := recovered.Requests[requestIndex]
		if request.PermitID != failed.Requests[requestIndex].PermitID || request.Diagnostic == nil || !reflect.DeepEqual(request.Diagnostic, failed.Requests[requestIndex].Diagnostic) {
			t.Fatal("重启重放丢失原 permit 或原始诊断元信息")
		}
		assertNoRuns()
		if scenario == "review_missing_required" {
			if failed.Proposal == nil || failed.State != "failed" || len(failed.Proposal.Reviews) != 5 {
				t.Fatalf("漏 required 的 PASS critic 未作为失败阻塞: %+v", failed)
			}
			if !strings.Contains(envelope.Diagnostic.ParseError, workspace.GoalReviewVerifierFeasibility) || missingRequirementID == "" || !strings.Contains(envelope.Diagnostic.ParseError, missingRequirementID) {
				t.Fatalf("漏审诊断未定位维度和 required ID: %s", envelope.Diagnostic.ParseError)
			}
			for _, review := range failed.Proposal.Reviews {
				if review.Status != workspace.GoalReviewPass {
					t.Fatalf("失败结果改写已完成 critic 原 status: %+v", review)
				}
			}
			if failed.Proposal.SpecificationHash != "" {
				t.Fatal("漏审候选获得可确认的 specification hash")
			}
			// 提供非空的自报身份，确保请求进入 owner 冲突门禁而不是仅因空字段被拒绝。
			confirmation := management.DefinitionAuthoringConfirm{InputHash: failed.InputHash, DerivationPermitID: failed.Proposal.DerivationPermitID, ProposalHash: failed.Proposal.ProposalHash, SpecificationHash: failed.Proposal.ProposalHash, ExpectedWorkspaceRevision: failed.WorkspaceRevision}
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusConflict)
			stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
			if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.DefinitionAuthoringConfirmation != nil || len(stored.DefinitionAuthoringHistory) != 0 {
				t.Fatalf("漏 required 的 PASS critic 产生正式 Goal/DoD 或确认身份: %+v %v", stored, err)
			}
			goals, goalErr := repositories.Goals.ListByWorkspaceID(ctx, common.WorkspaceID(created.WorkspaceID))
			dods, dodErr := repositories.DoDs.ListByWorkspaceID(ctx, common.WorkspaceID(created.WorkspaceID))
			if goalErr != nil || dodErr != nil || len(goals) != 0 || len(dods) != 0 {
				t.Fatalf("漏审确认留下正式定义: goals=%d dods=%d errors=%v/%v", len(goals), len(dods), goalErr, dodErr)
			}
			closeOwners()
			baseURL, routine, closeOwners = open()
			recovered := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
			if recovered.State != "failed" || recovered.WorkspaceRevision != failed.WorkspaceRevision || recovered.Proposal == nil || len(recovered.Proposal.Reviews) != 5 || physicalCalls.Load() != before {
				t.Fatalf("file 重开后无效 PASS 被重审、改判或移除: %+v calls=%d", recovered, physicalCalls.Load())
			}
			assertNoRuns()
			return
		}
		return
	}
	ready := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
	if realWorker {
		deadline := time.Now().Add(30 * time.Second)
		for ready.State != "ready_to_confirm" && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
			ready = authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		}
	}
	wantState := "ready_to_confirm"
	if scenario == "review_fail" || scenario == "review_unknown" {
		wantState = "review_blocked"
	}
	if ready.State != wantState || ready.Proposal == nil || len(ready.Proposal.Reviews) != 6 || len(ready.Proposal.Items) != 3 || physicalCalls.Load() != 8 {
		t.Fatalf("compile: %+v calls=%d", ready, physicalCalls.Load())
	}
	for _, review := range ready.Proposal.Reviews {
		if review.Status != "PASS" && wantState != "review_blocked" {
			t.Fatal("non-PASS review")
		}
	}
	assertNoRuns()
	assertSuccessAudit := func() {
		t.Helper()
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || stored.DefinitionAuthoring == nil {
			t.Fatalf("读取成功审计前像失败：%v", err)
		}
		request := stored.DefinitionAuthoring.SpecificationRequests[0]
		if request.SuccessAudit == nil || request.Proposal == nil {
			t.Fatal("已完成提案缺少 owner 原文审计")
		}
		type expectedAudit struct {
			permit, hash string
			raw          *workspace.DefinitionAuthoringSuccessByteAudit
		}
		expectedAudits := []expectedAudit{{request.PermitID, request.Proposal.ContentHash(), request.SuccessAudit}}
		for _, d := range stored.DefinitionAuthoring.DerivationRequests {
			if d.SuccessAudit != nil {
				expectedAudits = append(expectedAudits, expectedAudit{d.PermitID, d.Proposal.ContentHash(), d.SuccessAudit})
			}
		}
		for _, r := range stored.DefinitionAuthoring.SpecificationRequests {
			if r.ReviewAudit != nil {
				expectedAudits = append(expectedAudits, expectedAudit{r.PermitID, r.ProposalHash, r.ReviewAudit})
			}
		}
		if len(expectedAudits) != 8 {
			t.Fatal("missing v2 success audit")
		}
		before := stored
		readAudit := func(workspaceID, permit string, status int) management.DefinitionAuthoringSuccessAuditView {
			t.Helper()
			response, err := http.Get(baseURL + "/api/v1/workorch/workspaces/" + workspaceID + "/authoring/success-byte-audits/" + permit)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != status || response.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("受限审计响应：%d %v", response.StatusCode, response.Header)
			}
			if status != http.StatusOK {
				return management.DefinitionAuthoringSuccessAuditView{}
			}
			var envelope struct {
				Audit management.DefinitionAuthoringSuccessAuditView `json:"audit"`
			}
			if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			return envelope.Audit
		}
		for _, expected := range expectedAudits {
			for range 2 {
				audit := readAudit(created.WorkspaceID, expected.permit, http.StatusOK)
				if audit.WorkspaceID != created.WorkspaceID || audit.PermitID != expected.permit || audit.CanonicalHash != expected.hash {
					t.Fatal("受限审计身份变化")
				}
				for _, pair := range []struct {
					view management.DefinitionProviderObservationView
					raw  workspace.DefinitionProviderObservation
				}{{audit.Document, expected.raw.Document}, {audit.Wire, expected.raw.Wire}} {
					content, err := base64.StdEncoding.DecodeString(pair.view.ContentBase64)
					if err != nil || !bytes.Equal(content, pair.raw.Content) || !reflect.DeepEqual(pair.view.DefinitionProviderObservationMetadata, *authoringProviderObservationMetadata(&pair.raw)) {
						t.Fatal("受限审计未保全原 bytes 与 metadata")
					}
				}
			}
		}
		readAudit(created.WorkspaceID, "unknown-permit", http.StatusNotFound)
		readAudit("other-workspace", request.PermitID, http.StatusNotFound)
		after, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || !reflect.DeepEqual(before, after) || physicalCalls.Load() != 8 {
			t.Fatal("受限审计读取发生写入或重复 provider 调用")
		}
		assertNoRuns()
	}
	assertSuccessAudit()
	if scenario == "step02_candidate" {
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil {
			t.Fatal(err)
		}
		canonical := stored.DefinitionAuthoring.SpecificationRequests[0].Proposal
		p := ready.Proposal
		if p.Version != canonical.Version || p.GoalItemRef != canonical.GoalItemRef || p.GoalTextHash != canonical.GoalTextHash || len(p.Dimensions) != len(canonical.Dimensions) || len(p.Requirements) != len(canonical.Requirements) || len(p.Obligations) != len(canonical.Obligations) {
			t.Fatal("candidate structure projection lost canonical fields")
		}
		for index, requirement := range canonical.Requirements {
			got := p.Requirements[index]
			if got.ID != requirement.ID || got.DimensionID != requirement.DimensionID || got.Text != requirement.Text || got.Required != requirement.Required || got.SourceKind != requirement.SourceKind || !reflect.DeepEqual(got.SourceItemRefs, requirement.SourceItemRefs) || !reflect.DeepEqual(got.ResourceIDs, requirement.ResourceIDs) {
				t.Fatal("requirement source or original text changed")
			}
		}
		for index, obligation := range canonical.Obligations {
			got := p.Obligations[index]
			if got.ID != obligation.ID || got.RequirementID != obligation.RequirementID || got.Text != obligation.Text || !reflect.DeepEqual(got.DoDItemRefs, obligation.DoDItemRefs) {
				t.Fatal("all_of obligation mapping changed")
			}
		}
		for range 2 {
			replayed := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusOK)
			if !reflect.DeepEqual(replayed, ready) || physicalCalls.Load() != 8 {
				t.Fatal("completed replay changed candidate or called provider")
			}
		}
		closeOwners()
		baseURL, routine, closeOwners = open()
		recovered := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		if !reflect.DeepEqual(recovered, ready) || physicalCalls.Load() != 8 {
			t.Fatal("file-backed candidate/reviews not recovered")
		}
		stored, err = repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || stored.DefinitionAuthoringConfirmation != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil {
			t.Fatal("STEP02 created confirmation/admission")
		}
		goals, goalErr := repositories.Goals.ListByWorkspaceID(ctx, common.WorkspaceID(created.WorkspaceID))
		dods, dodErr := repositories.DoDs.ListByWorkspaceID(ctx, common.WorkspaceID(created.WorkspaceID))
		if goalErr != nil || dodErr != nil || len(goals) != 0 || len(dods) != 0 {
			t.Fatal("STEP02 materialized formal Goal/DoD")
		}
		assertNoRuns()
		return
	}
	confirmation := management.DefinitionAuthoringConfirm{InputHash: ready.InputHash, DerivationPermitID: ready.DerivationPermitID, ProposalHash: ready.Proposal.ProposalHash, SpecificationHash: ready.Proposal.SpecificationHash, ExpectedWorkspaceRevision: ready.WorkspaceRevision}
	if wantState == "review_blocked" {
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusConflict)
		assertNoRuns()
		closeOwners()
		baseURL, routine, closeOwners = open()
		recovered := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		if recovered.State != "review_blocked" || recovered.WorkspaceRevision != ready.WorkspaceRevision {
			t.Fatalf("阻塞审查重启后丢失: %+v", recovered)
		}
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusOK)
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.DefinitionAuthoringConfirmation != nil || physicalCalls.Load() != 8 {
			t.Fatalf("阻塞审查获得准入或重发 provider: %+v %v", stored, err)
		}
		assertNoRuns()
		return
	}
	if strings.HasPrefix(scenario, "step03_") {
		id := common.WorkspaceID(created.WorkspaceID)
		bound := common.ContextWithRuntimeWorkspaceID(ctx, id)
		if scenario == "step03_decision_unavailable" {
			authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusInternalServerError)
		} else {
			blocked := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusOK)
			if blocked.BlockingTicketID == "" {
				t.Fatal("missing initial ticket")
			}
			// Reconstruct the durable ticket-created / notification-not-marked cut.
			// Actual process-kill coverage for this cut is owned by Decision tests.
			ticket, err := repositories.Tickets.Get(bound, common.TicketID(blocked.BlockingTicketID))
			if err != nil {
				t.Fatal(err)
			}
			ticket.CreatedNotificationPublishedAt = ""
			if err := repositories.Tickets.Update(bound, ticket); err != nil {
				t.Fatal(err)
			}
		}
		before, err := repositories.Workspaces.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			observed := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
			if observed.State != "clarification_required" || observed.ConfirmationPhase != "materialized" || observed.BlockingTicketID != "" || observed.ConfirmationRecovery == nil || observed.CandidateHash == "" {
				t.Fatalf("missing exact blocked recovery: %+v", observed)
			}
		}
		after, err := repositories.Workspaces.Get(ctx, id)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("GET changed blocked Workspace")
		}
		closeOwners()
		decisionRestored.Store(true)
		baseURL, routine, closeOwners = open()
		observed := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
		if observed.BlockingTicketID != "" {
			t.Fatal("reopen GET repaired ticket delivery")
		}
		recovered := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", observed.ConfirmationRecovery, http.StatusOK)
		if recovered.State != "clarification_required" || recovered.BlockingTicketID == "" || recovered.StartupInputHash != "" || recovered.WorkspaceRevision != before.Revision {
			t.Fatalf("ticket recovery changed authority: %+v", recovered)
		}
		again := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusOK)
		if !reflect.DeepEqual(recovered, again) {
			t.Fatal("original confirmation replay changed ticket")
		}
		tickets, err := repositories.Tickets.ListByWorkspaceID(bound, id)
		if err != nil || len(tickets) != 1 || string(tickets[0].ID) != recovered.BlockingTicketID || tickets[0].CreatedNotificationPublishedAt == "" || tickets[0].RunID != "" {
			t.Fatalf("ticket delivery not unique/durable: %v", err)
		}
		after, err = repositories.Workspaces.Get(ctx, id)
		if err != nil || !reflect.DeepEqual(before, after) || after.StartupInput != nil || physicalCalls.Load() != 8 {
			t.Fatal("delivery changed Workspace/provider facts")
		}
		assertNoRuns()
		return
	}
	if scenario == "resource_drift" {
		if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("漂移资源\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusConflict)
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil || stored.DefinitionAuthoringConfirmation != nil {
			t.Fatalf("资源漂移绕过准入: %+v %v", stored, err)
		}
		assertNoRuns()
		if err := os.WriteFile(filepath.Join(directory, "requirements.txt"), []byte("可验证的资源\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stale := confirmation
	stale.InputHash = "stale"
	authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", stale, http.StatusConflict)
	admitted := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", confirmation, http.StatusOK)
	if admitted.State != "admitted" || admitted.ActivationState != "eligible" || admitted.StartupInputHash == "" {
		t.Fatalf("admission: %+v", admitted)
	}
	assertNoRuns()
	closeOwners()
	baseURL, routine, closeOwners = open()
	if preparationWorkflow {
		baselineCalls := physicalCalls.Load()
		current, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil {
			t.Fatal(err)
		}
		declarations, err := workorchmanagedconfig.CompilePreparationDeclarations(current.DefinitionAuthoring.TemplateSnapshot.ContextHints, current.ContextHints)
		if err != nil || len(declarations) == 0 {
			t.Fatal("missing managed fixture declarations", err)
		}
		var configPaths []string
		configRaw := []byte("{\r\n\"fixture\":\"original-managed-config\"}\n")
		for _, d := range declarations {
			root := current.InternalDirectory
			if d.Scope == workorchmanagedconfig.ScopeSystem {
				root = workorchmanagedconfig.SystemDirectory(root)
			}
			configPath := filepath.Join(root, d.Path)
			if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(configPath, configRaw, 0600); err != nil {
				t.Fatal(err)
			}
			configPaths = append(configPaths, configPath)
		}
		intent := management.PreparationCheckRequest{RequestID: "http-preparation-original", ExpectedWorkspaceRevision: admitted.WorkspaceRevision}
		if err := os.MkdirAll(filepath.Join(current.StartupInput.ResolvedProjectWorkspaceDirectory, ".agents", "skills"), 0700); err != nil {
			t.Fatal(err)
		}
		installedPaths := []string{filepath.Join(preparationInstalledRoot, "http-installed", "SKILL.md"), filepath.Join(current.StartupInput.ResolvedProjectWorkspaceDirectory, ".agents", "skills", "http-installed", "SKILL.md")}
		for _, path := range installedPaths {
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, installedRaw, 0600); err != nil {
				t.Fatal(err)
			}
			refPath := filepath.Join(filepath.Dir(path), "references", "exact.bin")
			if err := os.MkdirAll(filepath.Dir(refPath), 0700); err != nil {
				t.Fatal(err)
			}
			if !missingSkillRef {
				if err := os.WriteFile(refPath, []byte("HTTP 原始辅助文件\xff\x00\r\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		preparationPath := "/api/v1/workorch/workspaces/" + created.WorkspaceID + "/activation-preparation"
		memoryOriginal := &memorymodel.MemoryItem{ID: "http-memory", WorkspaceID: common.WorkspaceID(created.WorkspaceID), SourceType: "user", MemoryType: "project_context", Summary: "HTTP 原库存摘要", Content: "HTTP 私有库存正文\r\n", Tags: []string{"original"}, Vector: []float64{0.125, 0.25}}
		if err := repositories.MemoryItems.Create(ctx, memoryOriginal); err != nil {
			t.Fatal(err)
		}
		preparationMemoryModeEnabled.Store(true)
		first := preparationHTTPWorkflowRequest(t, baseURL, http.MethodPost, preparationPath+"/check", intent)
		if finalSkillChanged && first.Preparation.CurrentValidity != "stale" {
			t.Fatal("real final owner drift/unavailability did not stale current validity")
		}
		if first.ValidateFor(created.WorkspaceID, intent) != nil || first.Preparation.OperationState != "blocked" || len(first.Preparation.Record.Attempts[0].Checks) < 10 {
			t.Fatalf("public preparation did not persist full blocked checklist: %+v", first)
		}
		assessment := first.Preparation.Record.Attempts[0].Assessment
		definitionCompilation := first.Preparation.Record.Attempts[0].DefinitionCompilation
		if definitionCompilation == nil || definitionCompilation.ValidateFor(first.Preparation.Record.Attempts[0]) != nil || definitionCompilation.SourceInventoryHash == "" {
			t.Fatal("HTTP lost provider-independent original sealed-source compilation", definitionCompilation)
		}
		if assessment == nil || assessment.Outcome != "blocked" || assessment.ValidateFor(first.Preparation.Record.Attempts[0].Binding.ManifestHash, first.Preparation.Record.Attempts[0].Checks) != nil {
			t.Fatal("HTTP lost complete original all-of assessment", assessment)
		}
		if emptySelection && (len(assessment.Outstanding) != 1 || assessment.Outstanding[0].ID != "provider_cache" || assessment.PassedRequiredCount != assessment.RequiredCount-1) {
			t.Fatalf("bounded non-provider main path did not converge: %+v", assessment)
		}
		if emptySelection {
			if first.Compilation == nil || first.Compilation.FailureCode != "" || first.Compilation.BootstrapHash == "" {
				t.Fatalf("original full structure did not compile from cold-cache declaration: %+v", first.Compilation)
			}
			t.Logf("STEP04 bounded checklist: required=%d passed=%d outstanding=%s; M3 not issued", assessment.RequiredCount, assessment.PassedRequiredCount, assessment.Outstanding[0].ID)
			for _, change := range []string{"hash", "count", "snapshot", "profile", "version", "erase_seal"} {
				bad, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
				if err != nil {
					t.Fatal(err)
				}
				c := bad.StartupPreparation.Attempts[0].DefinitionCompilation
				switch change {
				case "hash":
					c.SourceInventoryHash = "caller"
				case "count":
					c.ObligationCount++
				case "snapshot":
					c.SnapshotHash = "other"
				case "profile":
					c.ProfileContractsHash = "other"
				case "version":
					c.CompilerVersion = "other"
				case "erase_seal":
					bad.PreparationSealEvidence = nil
				}
				if startuppreparation.ValidatePreparedDefinitionPersistence(bad) == nil || repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision) == nil {
					t.Fatal("forged pure compilation accepted", change)
				}
			}
			original, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
			if err != nil {
				t.Fatal(err)
			}
			clone := original.StartupPreparation.Clone()
			clone.Attempts[0].DefinitionCompilation.ObligationCount++
			if clone.Attempts[0].DefinitionCompilation.ObligationCount == original.StartupPreparation.Attempts[0].DefinitionCompilation.ObligationCount {
				t.Fatal("compilation clone aliases original")
			}
		}
		if emptySelection {
			for _, mutate := range []func(*workspace.Workspace){
				func(w *workspace.Workspace) { w.PreparationSkillRegistryEvidence = nil },
				func(w *workspace.Workspace) { w.PreparationMCPRegistryEvidence = nil },
				func(w *workspace.Workspace) { w.StartupPreparation.Attempts[0].EmptySelectionEvaluationVersion = "" },
				func(w *workspace.Workspace) { w.StartupPreparation.Attempts[0].Assessment.PassedRequiredCount++ },
				func(w *workspace.Workspace) {
					a := &w.StartupPreparation.Attempts[0]
					for i, c := range a.Checks {
						if c.ID == "provider_cache" {
							a.Checks = append(a.Checks[:i], a.Checks[i+1:]...)
							break
						}
					}
					requirements := make([]workspace.PreparationCheckRequirement, len(a.Checks))
					for i, c := range a.Checks {
						requirements[i] = workspace.PreparationCheckRequirement{ID: c.ID, Owner: c.Owner, Required: c.Required}
					}
					s, err := workspace.AssessPreparationChecks(a.Binding.ManifestHash, requirements, a.Checks)
					if err != nil {
						t.Fatal(err)
					}
					a.Assessment = &s
				},
			} {
				bad, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
				if err != nil {
					t.Fatal(err)
				}
				mutate(bad)
				if workspace.ValidatePreparationEvidencePersistence(bad) == nil {
					t.Fatal("forged aggregate or incomplete owner material accepted")
				}
			}
		}
		if first.Preparation.Record.Attempts[0].DirectoryEvidenceHash == "" {
			t.Fatal("HTTP explicit check lost actual File material reference")
		}
		if first.Preparation.Record.Attempts[0].RunHistoryEvidenceRef == "" {
			t.Fatal("HTTP explicit check lost original complete Run history reference")
		}
		if first.Preparation.Record.Attempts[0].SkillHubInstalledState != "observed_selection_pending" {
			t.Fatal("HTTP lost complete installed materials or claimed readiness")
		}
		if first.Preparation.Record.Attempts[0].SkillSelectionState != "compiled_checks_pending" {
			t.Fatal("HTTP lost sealed combination compilation")
		}
		if first.Preparation.Record.Attempts[0].SkillContextState != "compiled_checks_pending" {
			t.Fatal("HTTP lost pure context budget compilation")
		}
		if first.Preparation.Record.Attempts[0].SkillReferenceState != referenceState {
			t.Fatal("HTTP lost original auxiliary observation")
		}
		if first.Preparation.Record.Attempts[0].SkillStatusState != skillStatusState {
			t.Fatal("HTTP lost original selected status diagnosis")
		}
		public, _ := json.Marshal(first)
		if first.Preparation.Record.Attempts[0].MemoryInventoryEvidenceRef == "" || bytes.Contains(public, []byte("HTTP 私有库存正文")) || bytes.Contains(public, []byte("memory-preparation-epoch:")) {
			t.Fatalf("HTTP lost inventory reference or leaked private material: ref=%q checks=%+v", first.Preparation.Record.Attempts[0].MemoryInventoryEvidenceRef, first.Preparation.Record.Attempts[0].Checks)
		}
		if err := repositories.MemoryItems.Delete(ctx, memoryOriginal.ID); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(public, []byte("HTTP 原始安装材料")) || bytes.Contains(public, []byte("指令正文")) {
			t.Fatal("private installed bytes leaked")
		}
		if bytes.Contains(public, []byte("HTTP 原始辅助文件")) || bytes.Contains(public, []byte("references/exact.bin")) {
			t.Fatal("private auxiliary material leaked")
		}
		if first.Preparation.Record.Attempts[0].ResourceEvidenceHash == "" {
			t.Fatal("HTTP explicit check lost reviewed resource evidence reference")
		}
		if first.Preparation.Record.Attempts[0].ManagedConfigEvidenceRef == "" {
			t.Fatal("HTTP explicit check lost private managed config ref")
		}
		if first.Preparation.Record.Attempts[0].SoulEvidenceRef == "" || preparationSoulReads.Load() != 4 {
			t.Fatal("HTTP did not archive exact Soul before/after records")
		}
		if first.Preparation.Record.Attempts[0].SkillRegistryEvidenceRef == "" || preparationSkillReads.Load() != wantSkillReads {
			t.Fatalf("HTTP did not archive complete registry reads: reads=%d want=%d", preparationSkillReads.Load(), wantSkillReads)
		}
		for _, check := range first.Preparation.Record.Attempts[0].Checks {
			if check.ID == "mcp_selection" && check.Reason != mcpReason {
				t.Fatal("HTTP lost full registry observation or granted connection proof", check)
			}
			skillReason := "skill_selection_compiled_checks_pending"
			if emptySelection {
				skillReason = workspace.PreparationEmptySkillReason
			}
			if check.ID == "skill_selection" && check.Reason != skillReason {
				t.Fatal("HTTP lost registry classification")
			}
			if check.ID == "soul_selection" && check.Reason != "souls_observed_remaining_checks" {
				t.Fatal("HTTP lost complete Soul classification")
			}
			wantState := "blocked"
			if emptySelection && (check.ID == "skill_selection" || check.ID == "mcp_selection") {
				wantState = "passed"
			}
			if check.ID == "exact_seal" || check.ID == "persistence_integrity" || check.ID == "first_activation_no_run" || check.ID == "directory_boundary" || check.ID == "managed_configuration" || check.ID == "soul_selection" || check.ID == "memory_selection" || strings.HasPrefix(check.ID, "resource/") {
				wantState = "passed"
			}
			if check.ID == "exact_seal" && (check.Reason != "seal_observed_remaining_checks" || first.Preparation.Record.Attempts[0].SealEvidenceRef == "") || check.ID == "persistence_integrity" && (check.Reason != "persistence_binding_observed_remaining_checks" || first.Preparation.Record.Attempts[0].SealEvidenceRef == "") {
				t.Fatal("HTTP lost exact owner seal observation", check)
			}
			if check.State != wantState || check.ID == "provider_cache" && check.Reason != "provider_cache_miss" || check.ID == "first_activation_no_run" && (check.Reason != "run_history_empty_final_fence_observed" || first.Preparation.Record.Attempts[0].NoRunEvaluationVersion != "workorch-preparation-no-run-final-fence/v1") {
				t.Fatalf("HTTP lost actual observation blockers: %+v", check)
			}
			if check.ID == "directory_boundary" && check.Reason != "directory_observed_remaining_checks" {
				t.Fatalf("HTTP did not observe existing prepared directories: %+v", check)
			}
			if strings.HasPrefix(check.ID, "resource/") && check.Reason != "resource_observed_remaining_checks" {
				t.Fatalf("HTTP did not compare reviewed current resource bytes: %+v", check)
			}
			if check.ID == "managed_configuration" && check.Reason != "managed_configs_observed_remaining_checks" {
				t.Fatal("HTTP did not observe exact managed content", check.Reason)
			}
		}
		// Changing/removing an external file cannot cause GET/replay/recovery to
		// fabricate a fresh check or replace the immutable original evidence.
		for _, path := range installedPaths {
			if err := os.Rename(path, path+".preserved"); err != nil {
				t.Fatal(err)
			}
			refPath := filepath.Join(filepath.Dir(path), "references", "exact.bin")
			if !missingSkillRef {
				if err := os.Rename(refPath, refPath+".preserved"); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := filerepository.NewSoulRepository(filepath.Join(storage, "system", "souls.json")).Delete(ctx, "http-soul"); err != nil {
			t.Fatal(err)
		}
		// Only remove this isolated test fixture; a completed check must retain its
		// original history and must not requery the changed external registry.
		skillPath := filepath.Join(storage, "system", "skills.json")
		if err := os.Rename(skillPath, skillPath+".preserved"); err != nil {
			t.Fatal(err)
		}
		for _, configPath := range configPaths {
			if err := os.Remove(configPath); err != nil {
				t.Fatal(err)
			}
		}
		for reopen := 0; reopen < 2; reopen++ {
			for _, action := range []string{"", "/check", "/recover"} {
				method := http.MethodPost
				if action == "" {
					method = http.MethodGet
				}
				again := preparationHTTPWorkflowRequest(t, baseURL, method, preparationPath+action, intent)
				expected := first.Preparation
				if reopen > 0 {
					// Reopened File owner has a different process epoch. History is
					// immutable, but the independent current-validity axis must stale.
					expected.CurrentValidity = "stale"
					if first.Preparation.CurrentValidity != "stale" {
						expected.CurrentValidityReason = "file_epoch_changed_or_expired"
					}
				}
				if !reflect.DeepEqual(expected, again.Preparation) {
					t.Fatal("HTTP GET/replay/recovery/reopen rewrote original preparation")
				}
				if action == "" && again.Compilation != nil || emptySelection && action != "" && !reflect.DeepEqual(first.Compilation, again.Compilation) {
					t.Fatal("GET compiled or explicit original recovery changed complete compilation")
				}
			}
			if reopen == 0 {
				closeOwners()
				baseURL, routine, closeOwners = open()
			}
		}
		stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
		if err != nil || len(stored.PreparationSealEvidence) != 1 || stored.PreparationSealEvidence[0].FinalOwners == nil || stored.PreparationSealEvidence[0].FinalOwners.MatchesOriginal(stored, stored.StartupPreparation.Attempts[0]) == finalSkillChanged {
			t.Fatal("HTTP/replay/reopen lost complete exact final owner observations", err)
		}
		finalSkills := stored.PreparationSealEvidence[0].FinalOwners.Skills
		if finalSkillUnavailable && finalSkills != nil || scenario == "step04_workflow_final_skill_drift" && (finalSkills == nil || finalSkills.Catalog.Skills[0].Instruction != "真实最终指令变化") {
			t.Fatal("final owner failure/drift was replaced with original material")
		}
		if bytes.Contains(public, []byte("FinalOwners")) || bytes.Contains(public, []byte("skill-preparation-epoch:")) || bytes.Contains(public, []byte("HTTP Soul 原文")) || bytes.Contains(public, []byte("HTTP Skill 原文")) {
			t.Fatal("HTTP leaked private final owner observations")
		}
		for _, change := range []string{"remove_final", "rewrite_final", "remove_contract"} {
			bad, err := repositories.Workspaces.Get(ctx, stored.ID)
			if err != nil {
				t.Fatal(err)
			}
			e := &bad.PreparationSealEvidence[0]
			switch change {
			case "remove_final":
				e.FinalOwners = nil
			case "rewrite_final":
				e.FinalOwners.Souls.Souls[0].SoulMD += " changed"
			case "remove_contract":
				bad.StartupPreparation.Attempts[0].OwnerRevalidationVersion = ""
				e.FinalOwners = nil
			}
			e.ContentHash = e.Hash()
			if err := repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
				t.Fatal("completed final owner history rewritten after rehash", change)
			}
		}
		if err != nil || preparationStatusCalls.Load() != 2 || len(stored.PreparationSkillHubEffects) != 1 || stored.PreparationSkillHubEffects[0].Summary() != workspace.PreparationSkillHubRecorded || first.Preparation.Record.Attempts[0].SkillHubEffectState != "recorded" {
			t.Fatal("status history lost/replayed or falsely unresolved", err)
		}
		if len(stored.PreparationProviderCacheEvidence) != 1 || stored.PreparationProviderCacheEvidence[0].Ref != first.Preparation.Record.Attempts[0].ProviderCacheEvidenceRef || stored.PreparationProviderCacheEvidence[0].Facts.Observation.Catalog.ErrorCode != "cache_miss" || stored.PreparationProviderCacheEvidence[0].Facts.WorkspaceRevision != admitted.WorkspaceRevision+1 || preparationCacheReads.Load() != 1 {
			t.Fatal("HTTP/recovery/reopen lost cache-only owner history or repeated observation")
		}
		if len(stored.PreparationRunHistoryEvidence) != 1 || stored.PreparationRunHistoryEvidence[0].Ref != first.Preparation.Record.Attempts[0].RunHistoryEvidenceRef || stored.PreparationRunHistoryEvidence[0].Facts.Observation.RequestID != intent.RequestID || stored.PreparationRunHistoryEvidence[0].Facts.WorkspaceRevision != admitted.WorkspaceRevision+1 || stored.PreparationRunHistoryEvidence[0].Facts.BlockedReason() != "run_history_empty_fence_not_final" || stored.PreparationRunHistoryEvidence[0].Validate() != nil {
			t.Fatal("HTTP/recovery/reopen lost original complete Run history or granted no-Run credit")
		}
		if len(stored.PreparationMemoryInventoryEvidence) != 1 || stored.PreparationMemoryInventoryEvidence[0].Ref != first.Preparation.Record.Attempts[0].MemoryInventoryEvidenceRef || !reflect.DeepEqual(stored.PreparationMemoryInventoryEvidence[0].Facts.Observation.Items, []*memorymodel.MemoryItem{memoryOriginal}) {
			t.Fatal("HTTP/recovery/reopen lost original complete inventory")
		}
		memoryEvidence := stored.PreparationMemoryInventoryEvidence[0]
		if len(stored.PreparationMCPRegistryEvidence) != 1 || stored.PreparationMCPRegistryEvidence[0].Ref != first.Preparation.Record.Attempts[0].MCPRegistryEvidenceRef || stored.PreparationMCPRegistryEvidence[0].Facts.SnapshotHash != stored.StartupPreparation.Attempts[0].ObservationSnapshotHash || stored.PreparationMCPRegistryEvidence[0].Validate() != nil || preparationMCPReads.Load() != 4 {
			t.Fatal("HTTP/reopen lost complete original MCP registry history")
		}
		mcpSelection := stored.PreparationMCPRegistryEvidence[0].Selection
		mcpArchiveReason := mcpReason
		if emptySelection {
			mcpArchiveReason = "mcp_selection_empty_policy_pending"
		}
		if mcpSelection == nil || mcpSelection.FailureCode != mcpFailure || stored.PreparationMCPRegistryEvidence[0].SelectionEvaluationVersion != workspace.PreparationMCPSelectionEvaluationVersion || stored.PreparationMCPRegistryEvidence[0].BlockedReason() != mcpArchiveReason || mcpSelection.Version != workspace.PreparationMCPSelectionVersion || mcpSelection.SkillRegistryRef != stored.PreparationSkillRegistryEvidence[0].Ref || mcpSelection.SkillRegistryContentHash != stored.PreparationSkillRegistryEvidence[0].ContentHash || !reflect.DeepEqual(mcpSelection.Definition, workspace.PreparationMCPDefinition(stored)) {
			t.Fatal("HTTP/reopen lost owner-derived exact MCP selection")
		}
		if scenario == "step04_workflow_mcp_selected" {
			if !reflect.DeepEqual(mcpSelection.SelectedServerIDs, []string{"http-mcp"}) || !reflect.DeepEqual(mcpSelection.SelectedCapabilityIDs, []string{"http-capability"}) || len(mcpSelection.Selected.Servers) != 1 || mcpSelection.Selected.Servers[0].Command != "never-start" || len(mcpSelection.Selected.Capabilities) != 1 || mcpSelection.Selected.Capabilities[0].Description != "MCP 原文\r\n第二行" || !reflect.DeepEqual(mcpSelection.Selected.Servers, stored.PreparationMCPRegistryEvidence[0].Facts.Before.Servers) || !reflect.DeepEqual(mcpSelection.Selected.Capabilities, stored.PreparationMCPRegistryEvidence[0].Facts.Before.Capabilities) {
				t.Fatal("HTTP/reopen lost nonempty selection or exact owner values")
			}
		} else if !mcpFixture && (len(mcpSelection.Selected.Servers) != 0 || len(mcpSelection.Selected.Capabilities) != 0) {
			t.Fatal("empty registry invented MCP resources")
		}
		for _, mutation := range []string{"reason", "consumer", "source", "erase"} {
			bad, err := repositories.Workspaces.Get(ctx, stored.ID)
			if err != nil {
				t.Fatal(err)
			}
			e := &bad.PreparationMCPRegistryEvidence[0]
			switch mutation {
			case "reason":
				for n := range bad.StartupPreparation.Attempts[0].Checks {
					if bad.StartupPreparation.Attempts[0].Checks[n].ID == "mcp_selection" {
						bad.StartupPreparation.Attempts[0].Checks[n].Reason = "mcp_selection_source_binding_conflict"
					}
				}
			case "consumer":
				e.SelectionEvaluationVersion = ""
			case "source":
				e.Selection.SkillRegistryContentHash = "replacement"
			case "erase":
				bad.PreparationMCPRegistryEvidence = nil
				bad.StartupPreparation.Attempts[0].MCPRegistryEvidenceRef = ""
			}
			if len(bad.PreparationMCPRegistryEvidence) != 0 {
				e.ContentHash = e.Hash()
			}
			if workspace.ValidatePreparationEvidencePersistence(bad) == nil || repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision) == nil {
				t.Fatal("HTTP history accepted a rehashed selection consumer replacement", mutation)
			}
		}
		if memoryEvidence.Selection == nil || memoryEvidence.Selection.Mode != "list_only" || memoryEvidence.Selection.FailureCode != "" || memoryEvidence.Selection.Selection == nil || !reflect.DeepEqual(memoryEvidence.Selection.Selection.Selected, []*memorymodel.MemoryItem{memoryOriginal}) || memoryEvidence.Facts.SelectionSource == nil || memoryEvidence.Facts.SelectionSource.SnapshotHash != stored.StartupPreparation.Attempts[0].ObservationSnapshotHash || memoryEvidence.Facts.SelectionSource.Before.ConfigurationRevision != "http-memory-mode" {
			t.Fatal("HTTP/recovery/reopen lost original mode, selected bytes or exact accepted snapshot")
		}
		for _, change := range []string{"erase_provider", "provider_ref", "provider_epoch", "provider_binding"} {
			bad, _ := repositories.Workspaces.Get(ctx, stored.ID)
			switch change {
			case "erase_provider":
				bad.PreparationProviderCacheEvidence = nil
				bad.StartupPreparation.Attempts[0].ProviderCacheEvidenceRef = ""
			case "provider_ref":
				bad.StartupPreparation.Attempts[0].ProviderCacheEvidenceRef = ""
			case "provider_epoch":
				bad.PreparationProviderCacheEvidence[0].Facts.Observation.OwnerEpoch = "llm-preparation-epoch:BBBBBBBBBBBBBBBBBBBBBBBBBB"
			case "provider_binding":
				bad.PreparationProviderCacheEvidence[0].Binding.StartupInputHash = "replacement"
			}
			if len(bad.PreparationProviderCacheEvidence) != 0 {
				bad.PreparationProviderCacheEvidence[0].ContentHash = bad.PreparationProviderCacheEvidence[0].Hash()
			}
			if err := repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
				t.Fatal("HTTP history rewritten after rehash", change)
			}
		}
		if stored.PreparationSkillHubEffects[0].InstalledSummary() != "observed_selection_pending" {
			t.Fatal("reopen lost installed history")
		}
		for _, facts := range []*workspace.PreparationSkillHubStatusFacts{stored.PreparationSkillHubEffects[0].Global, stored.PreparationSkillHubEffects[0].Project} {
			if facts.Installed == nil || len(facts.Installed.First) != 1 || len(facts.Installed.Second) != 1 || !bytes.Equal(facts.Installed.First[0].Raw, installedRaw) || !bytes.Equal(facts.Installed.Second[0].Raw, installedRaw) {
				t.Fatal("recovery reread missing files or lost original rounds")
			}
			aux := facts.Installed.References
			if aux == nil || len(aux.First) != 1 || missingSkillRef && (aux.FailureCode == "" || aux.First[0].File.Complete) || !missingSkillRef && (!aux.CompleteFor(*facts.Installed) || !bytes.Equal(aux.Second[0].File.Raw, []byte("HTTP 原始辅助文件\xff\x00\r\n"))) {
				t.Fatal("reopen lost original reference reads or failure")
			}
		}
		if err != nil || len(stored.PreparationSoulEvidence) != 1 || preparationSoulReads.Load() != 4 || stored.PreparationSoulEvidence[0].Ref != first.Preparation.Record.Attempts[0].SoulEvidenceRef || stored.PreparationSoulEvidence[0].Facts.Souls[0].SoulMD != "HTTP Soul 原文\n" || !reflect.DeepEqual(stored.PreparationSoulEvidence[0].Facts.Souls, stored.PreparationSoulEvidence[0].Facts.ReobservedSouls) {
			t.Fatal("HTTP/reopen lost original Soul values or repeated observation", err)
		}
		if len(stored.PreparationSkillRegistryEvidence) != 1 || preparationSkillReads.Load() != wantSkillReads || stored.PreparationSkillRegistryEvidence[0].Ref != first.Preparation.Record.Attempts[0].SkillRegistryEvidenceRef || len(stored.PreparationSkillRegistryEvidence[0].Facts.Catalog.Skills) != 1 || stored.PreparationSkillRegistryEvidence[0].Facts.Catalog.Skills[0].Instruction != "HTTP Skill 原文" || !reflect.DeepEqual(stored.PreparationSkillRegistryEvidence[0].Facts.Catalog.Skills, stored.PreparationSkillRegistryEvidence[0].Facts.ReobservedCatalog.Skills) {
			t.Fatal("HTTP/reopen lost registry history or reobserved")
		}
		selection := stored.PreparationSkillRegistryEvidence[0].Selection
		if selection == nil || selection.References == nil || selection.References.State != referenceState {
			t.Fatal("reopen lost reference compilation")
		}
		selectedCount := 2
		if emptySelection {
			selectedCount = 0
		}
		if selection.Status == nil || selection.Status.State != skillStatusState || len(selection.Status.Entries) != selectedCount {
			t.Fatal("reopen lost original selected status diagnosis")
		}
		if selection == nil || selection.Summary() != "compiled_checks_pending" || len(selection.Catalog) != 3 || len(selection.Selected) != selectedCount || !emptySelection && (selection.Selected[0].Content.ID != "skillhub:workspace:"+created.WorkspaceID+":http-installed" || selection.Selected[0].Content.Instruction != string(installedRaw)) || selection.SoulRef != stored.PreparationSoulEvidence[0].Ref || selection.SkillHubEffectRef != stored.PreparationSkillHubEffects[0].Ref || selection.Definition.GoalText != stored.DefinitionAuthoringConfirmation.Goal.Text || !reflect.DeepEqual(selection.Definition.DoDItems, stored.DefinitionAuthoringConfirmation.DoD.Items) {
			t.Fatal("reopen lost complete selection, exact order, source material or owner identities")
		}
		// Rehashing a changed derived selection cannot replace exact source
		// recomputation; dropping it consistently still cannot rewrite history.
		if selection.Context == nil || !emptySelection && selection.Context.Budget == nil || len(selection.Context.Projection) != selectedCount {
			t.Fatal("reopen lost independently compiled context budget")
		}
		changes := []string{"score", "content", "allowlist", "source_ref", "context_projection", "context_budget", "context_diagnostic", "context_summary_missing", "context_erase", "references_path", "references_erase", "references_state", "status_raw", "status_reason", "status_state", "status_erase", "erase"}
		if emptySelection {
			changes = []string{"allowlist", "source_ref", "context_diagnostic", "context_summary_missing", "references_erase", "references_state", "status_state", "status_erase", "erase"}
		}
		for _, change := range changes {
			bad, _ := repositories.Workspaces.Get(ctx, stored.ID)
			e := &bad.PreparationSkillRegistryEvidence[0]
			switch change {
			case "status_raw":
				e.Selection.Status.Entries[0].ObservedStatus = "caller"
			case "status_reason":
				e.Selection.Status.Entries[0].Reason = "synced_observed_freshness_pending"
				if skillStatusState == "observed_freshness_pending" {
					e.Selection.Status.Entries[0].Reason = "status_not_synced"
				}
			case "status_state":
				bad.StartupPreparation.Attempts[0].SkillStatusState = "ready"
			case "status_erase":
				e.Selection.Status = nil
				bad.StartupPreparation.Attempts[0].SkillStatusState = ""
			case "references_path":
				e.Selection.References.Entries[0].Path = "/unrelated/identical-bytes"
			case "references_erase":
				e.Selection.References = nil
				bad.StartupPreparation.Attempts[0].SkillReferenceState = ""
			case "references_state":
				bad.StartupPreparation.Attempts[0].SkillReferenceState = "ready"
			case "erase":
				e.Selection = nil
				bad.StartupPreparation.Attempts[0].SkillSelectionState = ""
				bad.StartupPreparation.Attempts[0].SkillContextState = ""
				for n := range bad.StartupPreparation.Attempts[0].Checks {
					if bad.StartupPreparation.Attempts[0].Checks[n].ID == "skill_selection" {
						bad.StartupPreparation.Attempts[0].Checks[n].Reason = "skill_registry_observed_selection_pending"
					}
				}
			case "score":
				e.Selection.Selection.Matches[0].Score++
			case "content":
				e.Selection.Selected[0].Content.Instruction = "changed original instruction"
			case "allowlist":
				e.Selection.AllowedSkillRefs = []string{"unconfirmed"}
			case "source_ref":
				e.Selection.SoulRef = "other"
			case "context_projection":
				e.Selection.Context.Projection[0].Instruction = "changed projection"
			case "context_budget":
				e.Selection.Context.Budget.IncludedInstructionBytes++
			case "context_diagnostic":
				e.Selection.Context.Diagnostic = []byte("invented diagnostic")
			case "context_summary_missing":
				bad.StartupPreparation.Attempts[0].SkillContextState = ""
			case "context_erase":
				bad.StartupPreparation.Attempts[0].DefinitionCompilation = nil
				bad.StartupPreparation.Attempts[0].Assessment = nil
				bad.StartupPreparation.Attempts[0].EmptySelectionEvaluationVersion = ""
				e.Selection.Context = nil
				bad.StartupPreparation.Attempts[0].SkillContextState = ""
				// A historical snapshot predating context compilation also
				// predates its dependent MCP compilation. It remains readable,
				// but may never replace the completed current history by CAS.
				bad.PreparationMCPRegistryEvidence[0].Selection = nil
				bad.PreparationMCPRegistryEvidence[0].SelectionEvaluationVersion = ""
				bad.PreparationMCPRegistryEvidence[0].ContentHash = bad.PreparationMCPRegistryEvidence[0].Hash()
				for n := range bad.StartupPreparation.Attempts[0].Checks {
					if bad.StartupPreparation.Attempts[0].Checks[n].ID == "mcp_selection" {
						bad.StartupPreparation.Attempts[0].Checks[n].Reason = "mcp_registry_observed_connection_pending"
					}
				}
			}
			e.ContentHash = e.Hash()
			if change == "context_erase" && workspace.ValidatePreparationEvidencePersistence(bad) != nil {
				t.Fatal("legacy history without context projection no longer loads")
			}
			if change != "erase" && change != "context_erase" && workspace.ValidatePreparationEvidencePersistence(bad) == nil {
				t.Fatal("changed selection trusted after rehash")
			}
			if err := repositories.Workspaces.CompareAndSwap(ctx, bad, bad.Revision); err == nil {
				t.Fatal("completed selection rewritten")
			}
		}
		callerCopy, err := repositories.Workspaces.Get(ctx, stored.ID)
		if err != nil {
			t.Fatal(err)
		}
		callerCopy.StartupPreparation.Attempts[0].Assessment.Outstanding[0].Reason = "caller-mutated"
		callerCopy.PreparationSealEvidence[0].FinalOwners.Souls.Souls[0].SoulMD = "caller-mutated"
		if !emptySelection {
			callerCopy.PreparationSkillRegistryEvidence[0].Selection.Selected[0].Content.TriggerRules[0] = "caller-mutated"
		}
		fresh, err := repositories.Workspaces.Get(ctx, stored.ID)
		if err != nil || fresh.StartupPreparation.Attempts[0].Assessment.Outstanding[0].Reason == "caller-mutated" || fresh.PreparationSealEvidence[0].FinalOwners.Souls.Souls[0].SoulMD == "caller-mutated" || !emptySelection && fresh.PreparationSkillRegistryEvidence[0].Selection.Selected[0].Content.TriggerRules[0] != "交付目标" {
			t.Fatal("persisted selection aliases caller")
		}
		if err != nil || stored.Revision != admitted.WorkspaceRevision+2 || physicalCalls.Load() != baselineCalls || preparationCacheReads.Load() != 1 || preparationMemoryModeReads.Load() != 4 || len(stored.StartupPreparation.Attempts) != 1 || stored.StartupInput.StartupInputHash != admitted.StartupInputHash {
			t.Fatalf("preparation changed sealed/provider facts: %v", err)
		}
		if len(stored.PreparationDirectoryEvidence) != 1 || workspace.ValidatePreparationEvidencePersistence(stored) != nil || stored.PreparationDirectoryEvidence[0].ContentHash != first.Preparation.Record.Attempts[0].DirectoryEvidenceHash || !stored.PreparationDirectoryEvidence[0].Internal.Complete || !stored.PreparationDirectoryEvidence[0].Deliverable.Complete {
			t.Fatal("HTTP/reopen did not retain File owner original material")
		}
		if len(stored.PreparationResourceEvidence) != 1 || len(stored.PreparationResourceEvidence[0].Resources) != 1 || stored.PreparationResourceEvidence[0].ContentHash != first.Preparation.Record.Attempts[0].ResourceEvidenceHash || !bytes.Equal(stored.PreparationResourceEvidence[0].Resources[0].Observation.Content, []byte("可验证的资源\n")) {
			t.Fatal("HTTP/reopen lost exact reviewed resource original bytes")
		}
		if len(stored.PreparationManagedConfigEvidence) != 1 || len(stored.PreparationManagedConfigEvidence[0].Files) != len(declarations) || stored.PreparationManagedConfigEvidence[0].Ref != first.Preparation.Record.Attempts[0].ManagedConfigEvidenceRef {
			t.Fatal("HTTP/reopen lost original managed config evidence")
		}
		for i, file := range stored.PreparationManagedConfigEvidence[0].Files {
			if file.Scope != declarations[i].Scope || file.Path != declarations[i].Path || !bytes.Equal(file.Observation.Content, configRaw) {
				t.Fatal("HTTP/reopen changed original config scope/bytes")
			}
		}
		assertNoRuns()
		return
	}
	assertSuccessAudit()
	observed := authoringHTTPRequest(t, baseURL, http.MethodGet, path, nil, http.StatusOK)
	if observed.ConfirmationRecovery == nil || observed.ConfirmationRecovery.InputHash != confirmation.InputHash || observed.ConfirmationRecovery.DerivationPermitID != confirmation.DerivationPermitID || observed.ConfirmationRecovery.ProposalHash != confirmation.ProposalHash || observed.ConfirmationRecovery.SpecificationHash != confirmation.SpecificationHash || observed.ConfirmationRecovery.ExpectedWorkspaceRevision != observed.WorkspaceRevision {
		t.Fatalf("owner confirmation recovery identity lost: %+v", observed)
	}
	for _, changed := range []management.DefinitionAuthoringConfirm{
		{InputHash: "changed", DerivationPermitID: confirmation.DerivationPermitID, ProposalHash: confirmation.ProposalHash, SpecificationHash: confirmation.SpecificationHash, ExpectedWorkspaceRevision: observed.WorkspaceRevision},
		{InputHash: confirmation.InputHash, DerivationPermitID: "changed", ProposalHash: confirmation.ProposalHash, SpecificationHash: confirmation.SpecificationHash, ExpectedWorkspaceRevision: observed.WorkspaceRevision},
		{InputHash: confirmation.InputHash, DerivationPermitID: confirmation.DerivationPermitID, ProposalHash: "changed", SpecificationHash: confirmation.SpecificationHash, ExpectedWorkspaceRevision: observed.WorkspaceRevision},
		{InputHash: confirmation.InputHash, DerivationPermitID: confirmation.DerivationPermitID, ProposalHash: confirmation.ProposalHash, SpecificationHash: "changed", ExpectedWorkspaceRevision: observed.WorkspaceRevision},
	} {
		authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", changed, http.StatusConflict)
	}
	recovered := authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/confirm", observed.ConfirmationRecovery, http.StatusOK)
	if recovered.WorkspaceRevision != admitted.WorkspaceRevision || recovered.StartupInputHash != admitted.StartupInputHash {
		t.Fatal("exact confirmation replay changed persisted admission")
	}
	authoringHTTPRequest(t, baseURL, http.MethodPost, path+"/compile", compileRequest, http.StatusOK)
	if physicalCalls.Load() != 8 {
		t.Fatal("restart repeated provider calls")
	}
	stored, err := repositories.Workspaces.Get(ctx, common.WorkspaceID(created.WorkspaceID))
	if err != nil || stored.ActiveRunID != "" || stored.DefinitionAuthoringConfirmation == nil {
		t.Fatalf("persisted workspace: %+v %v", stored, err)
	}
	runs, err := repositories.Runs.List(ctx)
	if err != nil || len(runs) != 0 {
		t.Fatalf("authoring created Run: %v %v", runs, err)
	}
}

func authoringHTTPRequest(t *testing.T, baseURL, method, path string, body any, status int) management.DefinitionAuthoringView {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 180 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("%s %s permits persistent browser cache: %v", method, path, response.Header)
	}
	if response.StatusCode != status {
		var detail json.RawMessage
		_ = json.NewDecoder(response.Body).Decode(&detail)
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, response.StatusCode, status, detail)
	}
	if status != http.StatusOK && status != http.StatusCreated && status != http.StatusAccepted {
		return management.DefinitionAuthoringView{}
	}
	var envelope struct {
		Authoring management.DefinitionAuthoringView `json:"authoring"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Authoring
}

func authoringHTTPMapping(t *testing.T, record *workspace.DefinitionAuthoringRecord) workspace.DefinitionAuthoringMapping {
	t.Helper()
	mapping, err := testkit.Mapping(record.DerivationRequests[0], workspace.DefinitionAuthoringVerificationMapping{Declaration: "按模板验证", Status: workspace.GoalVerificationDeclared, ProfileID: "generic", EvidenceRequirementID: "*", Scope: "dodItem"})
	if err != nil {
		t.Fatal(err)
	}
	return mapping
}

func authoringHTTPProposal(t *testing.T, record *workspace.DefinitionAuthoringRecord) workspace.GoalSpecification {
	t.Helper()
	proposal, err := workspace.CompileDefinitionAuthoringMapping(record.DerivationRequests[0], authoringHTTPMapping(t, record))
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func authoringHTTPRequiredReviewRequirementIDs(t *testing.T, proposal workspace.GoalSpecification, kind string) []string {
	t.Helper()
	required, err := proposal.RequiredReviewRequirementIDs(kind)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(required))
	for _, id := range required {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		t.Fatalf("review kind %q has no required requirement IDs", kind)
	}
	return ids
}

func authoringHTTPGoalRequirementID(t *testing.T, proposal workspace.GoalSpecification) string {
	t.Helper()
	for _, requirement := range proposal.Requirements {
		if len(requirement.SourceItemRefs) == 1 && requirement.SourceItemRefs[0] == workspace.DefinitionAuthoringGoalItemRef {
			return requirement.ID
		}
	}
	t.Fatal("当前 compiler 未保留独立 Goal requirement")
	return ""
}
