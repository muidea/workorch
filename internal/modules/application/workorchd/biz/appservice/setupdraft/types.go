package setupdraft

import "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"

type Workspace struct {
	Title                string `json:"title,omitempty"`
	Description          string `json:"description,omitempty"`
	DeliverableDirectory string `json:"deliverableDirectory,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Draft struct {
	GoalText             string   `json:"goalText,omitempty"`
	DoDItems             []string `json:"doDItems,omitempty"`
	Assumptions          []string `json:"assumptions,omitempty"`
	ClarifyingQuestions  []string `json:"clarifyingQuestions,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
	Confidence           string   `json:"confidence,omitempty"`
	NeedsClarification   bool     `json:"needsClarification"`
	GenerationSummary    string   `json:"generationSummary,omitempty"`
	TemplateContextUsed  bool     `json:"templateContextUsed"`
	PreviousDraftApplied bool     `json:"previousDraftApplied"`
}

type GenerationCommand struct {
	TemplateID     string    `json:"templateId,omitempty"`
	Workspace      Workspace `json:"workspace,omitempty"`
	Messages       []Message `json:"messages,omitempty"`
	CurrentDraft   Draft     `json:"currentDraft,omitempty"`
	DialogLanguage string    `json:"dialogLanguage,omitempty"`
	Regenerate     bool      `json:"regenerate,omitempty"`
}

type GenerationRequest struct {
	Template       workorchmanagementcontract.WorkspaceTemplate `json:"template,omitempty"`
	Souls          []Soul                                       `json:"souls,omitempty"`
	Workspace      Workspace                                    `json:"workspace,omitempty"`
	Messages       []Message                                    `json:"messages,omitempty"`
	CurrentDraft   Draft                                        `json:"currentDraft,omitempty"`
	DialogLanguage string                                       `json:"dialogLanguage,omitempty"`
	Regenerate     bool                                         `json:"regenerate,omitempty"`
}

type Soul struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	SoulMd      string `json:"soulMd,omitempty"`
}
