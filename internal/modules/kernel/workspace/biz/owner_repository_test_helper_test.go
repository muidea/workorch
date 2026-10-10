package workspace

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type workspaceOwnerTestBackend struct {
	binding    repository.OwnerRepositoryBinding
	workspaces repository.WorkspaceRepository
	cleaner    repository.WorkspaceDeletionMaterialCleaner
	goals      repository.GoalRepository
	dods       repository.DoDRepository
	templates  repository.WorkspaceTemplateRepository
}

func (b workspaceOwnerTestBackend) WorkspaceOwnerBinding() repository.OwnerRepositoryBinding {
	return b.binding
}
func (b workspaceOwnerTestBackend) WorkspaceOwnerWorkspaces() repository.WorkspaceRepository {
	return b.workspaces
}
func (b workspaceOwnerTestBackend) WorkspaceOwnerDeletionMaterialCleaner() repository.WorkspaceDeletionMaterialCleaner {
	return b.cleaner
}
func (b workspaceOwnerTestBackend) WorkspaceOwnerGoals() repository.GoalRepository { return b.goals }
func (b workspaceOwnerTestBackend) WorkspaceOwnerDoDs() repository.DoDRepository   { return b.dods }
func (b workspaceOwnerTestBackend) WorkspaceOwnerTemplates() repository.WorkspaceTemplateRepository {
	return b.templates
}

type workspaceOwnerTestRepository struct {
	repository.WorkspaceRepository
	binding repository.OwnerRepositoryBinding
}

func (r workspaceOwnerTestRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type workspaceOwnerTestCleaner struct {
	repository.WorkspaceDeletionMaterialCleaner
	binding repository.OwnerRepositoryBinding
}

func (r workspaceOwnerTestCleaner) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type workspaceOwnerTestGoalRepository struct {
	repository.GoalRepository
	binding repository.OwnerRepositoryBinding
}

func (r workspaceOwnerTestGoalRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type workspaceOwnerTestDoDRepository struct {
	repository.DoDRepository
	binding repository.OwnerRepositoryBinding
}

func (r workspaceOwnerTestDoDRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

type workspaceOwnerTestTemplateRepository struct {
	repository.WorkspaceTemplateRepository
	binding repository.OwnerRepositoryBinding
}

func (r workspaceOwnerTestTemplateRepository) OwnerRepositoryBinding() repository.OwnerRepositoryBinding {
	return r.binding
}

func newWorkspaceOwnerTestRepositories(
	workspaces repository.WorkspaceRepository,
	cleaner repository.WorkspaceDeletionMaterialCleaner,
	goals repository.GoalRepository,
	dods repository.DoDRepository,
	templates repository.WorkspaceTemplateRepository,
) repository.WorkspaceOwnerRepositories {
	binding := repository.NewWorkspaceOwnerRepositoryBinding()
	owner, err := repository.NewWorkspaceOwnerRepositories(workspaceOwnerTestBackend{
		binding:    binding,
		workspaces: workspaceOwnerTestRepository{WorkspaceRepository: workspaces, binding: binding},
		cleaner:    workspaceOwnerTestCleaner{WorkspaceDeletionMaterialCleaner: cleaner, binding: binding},
		goals:      workspaceOwnerTestGoalRepository{GoalRepository: goals, binding: binding},
		dods:       workspaceOwnerTestDoDRepository{DoDRepository: dods, binding: binding},
		templates:  workspaceOwnerTestTemplateRepository{WorkspaceTemplateRepository: templates, binding: binding},
	})
	if err != nil {
		panic(err)
	}
	return owner
}

func workspaceOwnerTestDependencies(
	hub magicevent.Hub,
	background task.BackgroundRoutine,
	workspaces repository.WorkspaceRepository,
	cleaner repository.WorkspaceDeletionMaterialCleaner,
	goals repository.GoalRepository,
	dods repository.DoDRepository,
	templates repository.WorkspaceTemplateRepository,
) Dependencies {
	return Dependencies{
		OwnerRepositories: newWorkspaceOwnerTestRepositories(workspaces, cleaner, goals, dods, templates),
		EventHub:          hub,
		BackgroundRoutine: background,
	}
}
