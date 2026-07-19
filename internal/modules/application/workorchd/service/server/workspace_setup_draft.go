package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	engine "github.com/muidea/magicEngine/http"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/pkg/common"
)

const workspaceSetupDraftSessionPrefix = "wsdraft"

type workspaceSetupDraftSession struct {
	ID             string                                  `json:"id"`
	TemplateID     string                                  `json:"templateId,omitempty"`
	Workspace      appservice.WorkspaceSetupDraftWorkspace `json:"workspace,omitempty"`
	Messages       []appservice.WorkspaceSetupDraftMessage `json:"messages,omitempty"`
	Draft          appservice.WorkspaceSetupDraft          `json:"draft,omitempty"`
	State          string                                  `json:"state"`
	DialogLanguage string                                  `json:"dialogLanguage,omitempty"`
	CreatedAt      time.Time                               `json:"createdAt"`
	UpdatedAt      time.Time                               `json:"updatedAt"`
}

func registerWorkspaceSetupDraftRoutes(router engine.RouteRegistry, state *serverState) {
	router.AddHandler("/api/v1/workorch/workspace-setup-draft-sessions", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		var req createWorkspaceSetupDraftSessionRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		session, err := state.createWorkspaceSetupDraftSession(r.Context(), req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, workspaceSetupDraftSessionResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			DraftSession: session,
		})
	})
	router.AddHandler("/api/v1/workorch/workspace-setup-draft-sessions/:draftSessionId/messages", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		sessionID, action, ok := workspaceSetupDraftSessionActionFromPath(r.URL.Path)
		if !ok || action != "messages" {
			http.NotFound(w, r)
			return
		}
		var req appendWorkspaceSetupDraftMessageRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		session, err := state.appendWorkspaceSetupDraftMessage(r.Context(), sessionID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceSetupDraftSessionResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			DraftSession: session,
		})
	})
	router.AddHandler("/api/v1/workorch/workspace-setup-draft-sessions/:draftSessionId/regenerate", http.MethodPost, func(_ context.Context, w http.ResponseWriter, r *http.Request) {
		sessionID, action, ok := workspaceSetupDraftSessionActionFromPath(r.URL.Path)
		if !ok || action != "regenerate" {
			http.NotFound(w, r)
			return
		}
		var req appendWorkspaceSetupDraftMessageRequest
		if err := decodeOptionalJSON(r, &req); err != nil {
			writeBusinessError(w, err)
			return
		}
		session, err := state.regenerateWorkspaceSetupDraft(r.Context(), sessionID, req)
		if err != nil {
			writeBusinessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, workspaceSetupDraftSessionResponse{
			Service:      "workorch",
			Backend:      state.config.Backend,
			DataRoot:     state.config.DataRoot,
			DraftSession: session,
		})
	})
}

func defaultGenerateWorkspaceSetupDraft(ctx context.Context, backend string, dataRoot string, cmd appservice.WorkspaceSetupDraftGenerationCommand) (appservice.WorkspaceSetupDraft, error) {
	return appservice.GenerateWorkspaceSetupDraftWithBackend(ctx, backend, dataRoot, cmd)
}

func (s *serverState) createWorkspaceSetupDraftSession(ctx context.Context, req createWorkspaceSetupDraftSessionRequest) (workspaceSetupDraftSession, error) {
	if s == nil {
		return workspaceSetupDraftSession{}, common.ErrInvalidArgument
	}
	workspace := appservice.WorkspaceSetupDraftWorkspace{
		Title:                strings.TrimSpace(req.Title),
		Description:          strings.TrimSpace(req.Description),
		DeliverableDirectory: strings.TrimSpace(req.DeliverableDirectory),
	}
	if workspace.Title == "" && workspace.Description == "" {
		return workspaceSetupDraftSession{}, common.ErrInvalidArgument
	}
	templateID := strings.TrimSpace(req.TemplateID)
	draft := appservice.WorkspaceSetupDraft{}
	if templateID != "" {
		template, err := s.findWorkspaceTemplate(ctx, templateID)
		if err != nil {
			return workspaceSetupDraftSession{}, err
		}
		draft.GoalText = template.GoalText
		draft.DoDItems = template.DoDItems
		draft.TemplateContextUsed = true
	}
	draft = appservice.NormalizeWorkspaceSetupDraft(draft)
	now := time.Now().UTC()
	session := workspaceSetupDraftSession{
		ID:             generateWorkspaceSetupDraftSessionID(),
		TemplateID:     templateID,
		Workspace:      workspace,
		Draft:          draft,
		State:          workspaceSetupDraftState(draft),
		DialogLanguage: strings.TrimSpace(req.DialogLanguage),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.workspaceSetupDraftMu.Lock()
	if s.workspaceSetupDraftSessions == nil {
		s.workspaceSetupDraftSessions = map[string]*workspaceSetupDraftSession{}
	}
	s.workspaceSetupDraftSessions[session.ID] = cloneWorkspaceSetupDraftSessionPtr(session)
	s.workspaceSetupDraftMu.Unlock()
	return session, nil
}

func (s *serverState) appendWorkspaceSetupDraftMessage(ctx context.Context, sessionID string, req appendWorkspaceSetupDraftMessageRequest) (workspaceSetupDraftSession, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return workspaceSetupDraftSession{}, common.ErrInvalidArgument
	}
	session, err := s.workspaceSetupDraftSession(sessionID)
	if err != nil {
		return workspaceSetupDraftSession{}, err
	}
	session, err = s.applyWorkspaceSetupDraftRequestContext(ctx, session, req)
	if err != nil {
		return workspaceSetupDraftSession{}, err
	}
	session.Messages = append(session.Messages, appservice.WorkspaceSetupDraftMessage{Role: "user", Content: content})
	if req.DialogLanguage != "" {
		session.DialogLanguage = strings.TrimSpace(req.DialogLanguage)
	}
	if hasWorkspaceSetupDraftContent(req.Draft) {
		session.Draft = appservice.MergeWorkspaceSetupDraft(session.Draft, req.Draft)
	}
	session.Draft = appservice.WorkspaceSetupDraftRevisionBaseline(session.Draft)
	return s.generateAndStoreWorkspaceSetupDraft(ctx, session, false)
}

func (s *serverState) regenerateWorkspaceSetupDraft(ctx context.Context, sessionID string, req appendWorkspaceSetupDraftMessageRequest) (workspaceSetupDraftSession, error) {
	session, err := s.workspaceSetupDraftSession(sessionID)
	if err != nil {
		return workspaceSetupDraftSession{}, err
	}
	session, err = s.applyWorkspaceSetupDraftRequestContext(ctx, session, req)
	if err != nil {
		return workspaceSetupDraftSession{}, err
	}
	if req.DialogLanguage != "" {
		session.DialogLanguage = strings.TrimSpace(req.DialogLanguage)
	}
	if hasWorkspaceSetupDraftContent(req.Draft) {
		session.Draft = appservice.MergeWorkspaceSetupDraft(session.Draft, req.Draft)
	}
	if content := strings.TrimSpace(req.Content); content != "" {
		session.Messages = append(session.Messages, appservice.WorkspaceSetupDraftMessage{Role: "user", Content: content})
	}
	return s.generateAndStoreWorkspaceSetupDraft(ctx, session, true)
}

func (s *serverState) applyWorkspaceSetupDraftRequestContext(ctx context.Context, session workspaceSetupDraftSession, req appendWorkspaceSetupDraftMessageRequest) (workspaceSetupDraftSession, error) {
	templateID := strings.TrimSpace(req.TemplateID)
	if templateID != "" && templateID != session.TemplateID {
		template, err := s.findWorkspaceTemplate(ctx, templateID)
		if err != nil {
			return workspaceSetupDraftSession{}, err
		}
		session.TemplateID = templateID
		templateDraft := appservice.NormalizeWorkspaceSetupDraft(appservice.WorkspaceSetupDraft{
			GoalText:            template.GoalText,
			DoDItems:            template.DoDItems,
			TemplateContextUsed: true,
		})
		if hasWorkspaceSetupDraftContent(session.Draft) {
			session.Draft = appservice.MergeWorkspaceSetupDraft(templateDraft, session.Draft)
		} else {
			session.Draft = templateDraft
		}
	}
	if title := strings.TrimSpace(req.Title); title != "" {
		session.Workspace.Title = title
	}
	if description := strings.TrimSpace(req.Description); description != "" {
		session.Workspace.Description = description
	}
	if deliverableDirectory := strings.TrimSpace(req.DeliverableDirectory); deliverableDirectory != "" {
		session.Workspace.DeliverableDirectory = deliverableDirectory
	}
	return session, nil
}

func (s *serverState) generateAndStoreWorkspaceSetupDraft(ctx context.Context, session workspaceSetupDraftSession, regenerate bool) (workspaceSetupDraftSession, error) {
	if s == nil || s.generateWorkspaceSetupDraft == nil {
		return workspaceSetupDraftSession{}, common.ErrInvalidArgument
	}
	draft, err := s.generateWorkspaceSetupDraft(ctx, s.config.Backend, s.config.DataRoot, appservice.WorkspaceSetupDraftGenerationCommand{
		TemplateID:     session.TemplateID,
		Workspace:      session.Workspace,
		Messages:       session.Messages,
		CurrentDraft:   session.Draft,
		DialogLanguage: session.DialogLanguage,
		Regenerate:     regenerate,
	})
	if err != nil {
		return workspaceSetupDraftSession{}, err
	}
	previousDraftApplied := hasWorkspaceSetupDraftContent(session.Draft)
	draft.PreviousDraftApplied = previousDraftApplied
	session.Draft = appservice.MergeWorkspaceSetupDraft(session.Draft, draft)
	session.Draft.PreviousDraftApplied = session.Draft.PreviousDraftApplied || previousDraftApplied
	session.Messages = append(session.Messages, appservice.WorkspaceSetupDraftMessage{Role: "assistant", Content: workspaceSetupDraftAssistantMessage(session.Draft)})
	session.State = workspaceSetupDraftState(session.Draft)
	session.UpdatedAt = time.Now().UTC()
	s.workspaceSetupDraftMu.Lock()
	if s.workspaceSetupDraftSessions == nil {
		s.workspaceSetupDraftSessions = map[string]*workspaceSetupDraftSession{}
	}
	s.workspaceSetupDraftSessions[session.ID] = cloneWorkspaceSetupDraftSessionPtr(session)
	s.workspaceSetupDraftMu.Unlock()
	return session, nil
}

func (s *serverState) workspaceSetupDraftSession(sessionID string) (workspaceSetupDraftSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return workspaceSetupDraftSession{}, common.ErrInvalidArgument
	}
	s.workspaceSetupDraftMu.Lock()
	defer s.workspaceSetupDraftMu.Unlock()
	if s.workspaceSetupDraftSessions == nil {
		return workspaceSetupDraftSession{}, common.ErrNotFound
	}
	item := s.workspaceSetupDraftSessions[sessionID]
	if item == nil {
		return workspaceSetupDraftSession{}, common.ErrNotFound
	}
	return cloneWorkspaceSetupDraftSession(*item), nil
}

func (s *serverState) findWorkspaceTemplate(ctx context.Context, templateID string) (appservice.WorkspaceTemplate, error) {
	items, err := s.listWorkspaceTemplates(ctx, s.config.Backend, s.config.DataRoot)
	if err != nil {
		return appservice.WorkspaceTemplate{}, err
	}
	for _, item := range items {
		if item.ID == templateID {
			return item, nil
		}
	}
	return appservice.WorkspaceTemplate{}, common.ErrNotFound
}

func workspaceSetupDraftSessionActionFromPath(p string) (sessionID string, action string, ok bool) {
	const prefix = "/api/v1/workorch/workspace-setup-draft-sessions/"
	if !strings.HasPrefix(p, prefix) {
		return "", "", false
	}
	rest := strings.Trim(strings.TrimPrefix(p, prefix), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", "", false
	}
	sessionID = parts[0]
	if len(parts) > 1 {
		action = parts[1]
	}
	return sessionID, action, true
}

func workspaceSetupDraftState(draft appservice.WorkspaceSetupDraft) string {
	if len(draft.ClarifyingQuestions) > 0 || draft.GoalText == "" || len(draft.DoDItems) == 0 {
		return "clarifying"
	}
	return "draft_ready"
}

func workspaceSetupDraftAssistantMessage(draft appservice.WorkspaceSetupDraft) string {
	parts := []string{}
	if draft.GenerationSummary != "" {
		parts = append(parts, draft.GenerationSummary)
	}
	if draft.GoalText != "" {
		parts = append(parts, "Goal: "+draft.GoalText)
	}
	if len(draft.DoDItems) > 0 {
		items := make([]string, 0, len(draft.DoDItems))
		for _, item := range draft.DoDItems {
			item = strings.TrimSpace(item)
			if item != "" {
				items = append(items, "- "+item)
			}
		}
		if len(items) > 0 {
			parts = append(parts, "DoD:\n"+strings.Join(items, "\n"))
		}
	}
	if len(draft.ClarifyingQuestions) > 0 {
		parts = append(parts, "Clarifying questions: "+strings.Join(draft.ClarifyingQuestions, "; "))
	}
	if len(parts) == 0 {
		return "Draft updated."
	}
	return strings.Join(parts, "\n")
}

func hasWorkspaceSetupDraftContent(draft appservice.WorkspaceSetupDraft) bool {
	return strings.TrimSpace(draft.GoalText) != "" ||
		len(draft.DoDItems) > 0 ||
		len(draft.Assumptions) > 0 ||
		len(draft.ClarifyingQuestions) > 0 ||
		len(draft.Warnings) > 0
}

func cloneWorkspaceSetupDraftSessionPtr(session workspaceSetupDraftSession) *workspaceSetupDraftSession {
	cloned := cloneWorkspaceSetupDraftSession(session)
	return &cloned
}

func cloneWorkspaceSetupDraftSession(session workspaceSetupDraftSession) workspaceSetupDraftSession {
	session.Messages = append([]appservice.WorkspaceSetupDraftMessage(nil), session.Messages...)
	session.Draft.DoDItems = append([]string(nil), session.Draft.DoDItems...)
	session.Draft.Assumptions = append([]string(nil), session.Draft.Assumptions...)
	session.Draft.ClarifyingQuestions = append([]string(nil), session.Draft.ClarifyingQuestions...)
	session.Draft.Warnings = append([]string(nil), session.Draft.Warnings...)
	return session
}

func generateWorkspaceSetupDraftSessionID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return workspaceSetupDraftSessionPrefix + "-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s-%d", workspaceSetupDraftSessionPrefix, time.Now().UTC().UnixNano())
}
