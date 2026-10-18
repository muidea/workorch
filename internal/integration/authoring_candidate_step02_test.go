package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

// Only the external provider HTTP response is scripted. The public application,
// LLM adapter, EventHub, Workspace/File owners and file persistence are real.
// STEP02 stops at readback; no confirmation or Runtime activation is submitted.
func TestAuthoringCandidateStep02ProductionOwners(t *testing.T) {
	ctx := context.Background()
	storage, directory := t.TempDir(), t.TempDir()
	repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
	provider := &emptyDirectoryProvider{repos: repos}
	proxy := httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(proxy.Close)
	base, client := implementationWorkFullflowStart(t, ctx, storage, proxy.URL, repos)
	type response struct {
		Authoring management.DefinitionAuthoringView `json:"authoring"`
	}
	// The exact source commitment inventory is fixed before any generation.
	goal := "交付一个可本地运行的非敏感文本工具。"
	criteria := []string{"正常输入返回预期结果，空输入给出明确错误。", "保留原始输入顺序，不静默去重或截断。"}
	var created response
	productionJSON(t, client, http.MethodPost, base+"/api/v1/workorch/workspace-authoring", management.DefinitionAuthoringCreate{RequestID: "step02-production-candidate", Title: "STEP02 候选验证", TemplateID: "software-dev", DeliverableDirectory: directory, Input: management.DefinitionAuthoringInput{GoalText: goal, AcceptanceRequirements: criteria}}, &created)
	path := base + "/api/v1/workorch/workspaces/" + created.Authoring.WorkspaceID + "/authoring"
	for range 2 {
		var observed response
		productionJSON(t, client, http.MethodGet, path, nil, &observed)
		if !reflect.DeepEqual(created, observed) || len(provider.snapshot()) != 0 {
			t.Fatal("query changed authoring or contacted provider")
		}
	}
	request := management.DefinitionAuthoringCompile{ExpectedInputHash: created.Authoring.InputHash, ExpectedTemplateSnapshotHash: created.Authoring.TemplateSnapshotHash}
	productionJSON(t, client, http.MethodPost, path+"/compile", request, nil)
	var ready response
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		productionJSON(t, client, http.MethodGet, path, nil, &ready)
		if ready.Authoring.State == "ready_to_confirm" {
			break
		}
		if ready.Authoring.State == "failed" || ready.Authoring.State == "preparation_failed" || ready.Authoring.State == "review_blocked" {
			t.Fatalf("success path blocked: %+v", ready.Authoring)
		}
		time.Sleep(20 * time.Millisecond)
	}
	p := ready.Authoring.Proposal
	if ready.Authoring.State != "ready_to_confirm" || p == nil || p.GoalText != goal || len(p.Reviews) != 6 || len(p.Dimensions) == 0 || len(p.Requirements) == 0 || len(p.Obligations) == 0 {
		t.Fatalf("incomplete production candidate: %+v", ready.Authoring)
	}
	for _, text := range append([]string{goal}, criteria...) {
		found := false
		for _, requirement := range p.Requirements {
			if requirement.Text == text && requirement.Required {
				found = true
			}
		}
		if !found {
			t.Fatalf("original required commitment missing: %q", text)
		}
	}
	for _, v := range p.Verifications {
		if v.Status != workspace.GoalVerificationDeclared || v.ObligationID == "" || v.EvidenceRequirementID == "" || v.ProfileID == "" {
			t.Fatalf("invalid declaration: %+v", v)
		}
	}
	beforeCalls := provider.snapshot()
	if len(beforeCalls) != 8 {
		t.Fatalf("expected derive/proposal/six distinct review slots: %v", beforeCalls)
	}
	for _, review := range p.Reviews {
		if review.Status != workspace.GoalReviewPass || review.SpecificationHash != p.ProposalHash || len(review.RequirementIDs) == 0 || len(review.EvidenceRefs) == 0 {
			t.Fatalf("review binding: %+v", review)
		}
	}
	var replay response
	productionJSON(t, client, http.MethodPost, path+"/compile", request, &replay)
	if !reflect.DeepEqual(ready, replay) || !reflect.DeepEqual(beforeCalls, provider.snapshot()) {
		t.Fatal("replay changed owner facts or physically called provider")
	}
	// Independently reopen the file repository, not the page-memory projection.
	stored, err := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage).Workspaces.Get(ctx, common.WorkspaceID(ready.Authoring.WorkspaceID))
	if err != nil || stored.DefinitionAuthoring == nil || stored.DefinitionAuthoringConfirmation != nil || stored.StartupInput != nil || stored.ActivationAdmission != nil {
		t.Fatalf("STEP02 crossed confirmation boundary: %v", err)
	}
	if len(stored.DefinitionAuthoring.DerivationRequests) != 1 || len(stored.DefinitionAuthoring.SpecificationRequests) != 7 {
		t.Fatal("durable permit ledger mismatch")
	}
	// Independently check each source against the actual default Profile, not
	// merely the fixture's self-reported DECLARED status or the Web projection.
	record := stored.DefinitionAuthoring
	bindings, err := definitionconfiguration.Compile(&workspace.Workspace{}, &record.TemplateSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	derivation := record.DerivationRequests[0]
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		t.Fatal(err)
	}
	proposal := record.SpecificationRequests[0].Proposal
	if proposal == nil || len(proposal.Verifications) != len(items) {
		t.Fatal("missing exact per-source declarations")
	}
	for _, verification := range proposal.Verifications {
		if !definitionadmission.SpecificationVerificationApplies(*proposal, verification, items, bindings.ProfileContracts) {
			t.Fatalf("default Profile declaration does not survive source-policy merge: %s", verification.ID)
		}
	}
	assertNoRun(t, repos)
	assertEmptyDeliverable(t, directory)
	t.Logf("STEP02 input=%s template=%s proposal=%s specification=%s slots=%v", ready.Authoring.InputHash, ready.Authoring.TemplateSnapshotHash, p.ProposalHash, p.SpecificationHash, beforeCalls)
}

func TestAuthoringCandidateStep02DeclarationFailuresPreserveOwnerFacts(t *testing.T) {
	for _, tc := range []struct {
		name, declaration, status, profile, state string
		calls                                     int
	}{
		{"empty_declared", "", workspace.GoalVerificationDeclared, "software-dev", "failed", 2},
		{"blank_unverified", " \t\n\u3000", workspace.GoalVerificationUnverified, "", "review_blocked", 8},
		{"honest_gap", "缺少可证明适用性的 Profile 声明，保留该缺口。", workspace.GoalVerificationUnverified, "", "review_blocked", 8},
		{"invented_profile", "尚未执行，仅声明能力。", workspace.GoalVerificationDeclared, "invented-profile", "failed", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			storage, directory := t.TempDir(), t.TempDir()
			repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage)
			provider := &emptyDirectoryProvider{repos: repos, mappingProposal: func(record *workspace.DefinitionAuthoringRecord) (workspace.DefinitionAuthoringMapping, error) {
				mapping, err := productionMapping(record)
				if err != nil {
					return mapping, err
				}
				v := &mapping.Obligations[0].Verification
				v.Declaration, v.Status, v.ProfileID = tc.declaration, tc.status, tc.profile
				if tc.status == workspace.GoalVerificationUnverified {
					v.EvidenceRequirementID, v.Scope = "", ""
				}
				return mapping, nil
			}}
			proxy := httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
			t.Cleanup(proxy.Close)
			base, client := implementationWorkFullflowStart(t, ctx, storage, proxy.URL, repos)
			type response struct {
				Authoring management.DefinitionAuthoringView `json:"authoring"`
			}
			var created response
			productionJSON(t, client, http.MethodPost, base+"/api/v1/workorch/workspace-authoring", management.DefinitionAuthoringCreate{RequestID: "step02-declaration-" + tc.name, Title: "声明负例", TemplateID: "software-dev", DeliverableDirectory: directory, Input: management.DefinitionAuthoringInput{GoalText: "交付本地文本工具。", AcceptanceRequirements: []string{"保留输入顺序。"}}}, &created)
			path := base + "/api/v1/workorch/workspaces/" + created.Authoring.WorkspaceID + "/authoring"
			request := management.DefinitionAuthoringCompile{ExpectedInputHash: created.Authoring.InputHash, ExpectedTemplateSnapshotHash: created.Authoring.TemplateSnapshotHash}
			productionJSON(t, client, http.MethodPost, path+"/compile", request, nil)
			var observed response
			for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
				productionJSON(t, client, http.MethodGet, path, nil, &observed)
				if observed.Authoring.State == tc.state {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if observed.Authoring.State != tc.state || len(provider.snapshot()) != tc.calls {
				t.Fatalf("unexpected state/calls: %s/%v", observed.Authoring.State, provider.snapshot())
			}
			if tc.state == "review_blocked" {
				if observed.Authoring.Proposal == nil || len(observed.Authoring.Proposal.Reviews) != 6 {
					t.Fatal("honest gap lost proposal or six independent reviews")
				}
				for _, review := range observed.Authoring.Proposal.Reviews {
					if review.Status != workspace.GoalReviewPass {
						t.Fatal("fixture should demonstrate six PASS cannot override gap")
					}
				}
			}
			id := common.WorkspaceID(created.Authoring.WorkspaceID)
			before, err := repos.Workspaces.Get(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			var replay response
			productionJSON(t, client, http.MethodPost, path+"/compile", request, &replay)
			after, err := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage).Workspaces.Get(ctx, id)
			if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(observed, replay) || len(provider.snapshot()) != tc.calls {
				t.Fatalf("replay rewrote failure/gap or repeated model call: %v", err)
			}
			if after.DefinitionAuthoringConfirmation != nil || after.StartupInput != nil || after.ActivationAdmission != nil {
				t.Fatal("negative candidate crossed STEP02 boundary")
			}
			assertNoRun(t, repos)
			assertEmptyDeliverable(t, directory)
		})
	}
}
