package impl

import (
	"errors"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"go.uber.org/zap"
	"time"
	workspaceconfig "workspace-service/app/config"
	"workspace-service/app/models/dto"
	"workspace-service/app/services"
	"workspace-service/app/utils"
)

type DockerWorkspaceService struct {
	services.WorkspaceService
	workspaceServiceConfig *workspaceconfig.WorkspaceServiceConfig
	logger                 *zap.Logger
}

func (ws DockerWorkspaceService) CreateWorkspace(workspaceId string, backendTemplate string, remoteURL string) (*dto.WorkspaceDetails, error) {
	err := ws.checkAndCreateWorkspaceFromTemplate(workspaceId, backendTemplate, remoteURL)
	if err != nil {
		ws.logger.Error("Failed to check and create workspace from template", zap.Error(err))
		return nil, err
	}

	workspaceUrl := "http://localhost:8081/?folder=/workspaces/" + workspaceId
	frontendUrl := "http://localhost:3000"
	backendUrl := "http://localhost:5000"

	return &dto.WorkspaceDetails{
		WorkspaceId:      workspaceId,
		BackendTemplate:  &backendTemplate,
		FrontendTemplate: nil,
		WorkspaceUrl:     &workspaceUrl,
		FrontendUrl:      &frontendUrl,
		BackendUrl:       &backendUrl,
	}, nil
}

func (ws DockerWorkspaceService) checkAndCreateWorkspaceFromTemplate(workspaceId string, backendTemplate string, remoteURL string) error {
	exists, err := utils.CheckIfWorkspaceExists(workspaceId)
	if err != nil {
		ws.logger.Error("Failed to check if workspace exists", zap.Error(err))
		return err
	}

	if exists {
		ws.logger.Info("Workspace already exists", zap.String("workspaceId", workspaceId))
		return nil
	}

	ws.logger.Info("Creating workspace from template", zap.String("workspaceId", workspaceId), zap.String("backendTemplate", backendTemplate))

	err = utils.SudoRsyncFolders("/templates/"+backendTemplate+"/", "/workspaces/"+workspaceId)
	if err != nil {
		ws.logger.Error("Failed to rsync folders", zap.Error(err))
		return err
	}

	workspacePath := "/workspaces/" + workspaceId
	repo, err := git.PlainOpen(workspacePath)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			ws.logger.Info("Git repository does not exist", zap.String("workspaceId", workspaceId))
			ws.logger.Info("Initializing and configuring Git repository", zap.String("workspaceId", workspaceId))

			// Initialize Git repository
			repo, err = git.PlainInit(workspacePath, false)
			if err != nil {
				ws.logger.Error("Failed to initialize Git repository", zap.Error(err))
				return err
			}

			// Checkout main branch (create if not exists)
			worktree, err := repo.Worktree()
			if err != nil {
				ws.logger.Error("Failed to get worktree", zap.Error(err))
				return err
			}

			// Stage all files
			err = worktree.AddGlob(".")
			if err != nil {
				ws.logger.Error("Failed to stage files", zap.Error(err))
				return err
			}

			// Commit staged files
			commit, err := worktree.Commit("Initial commit", &git.CommitOptions{
				Author: &object.Signature{
					Name:  "SuperCoder",
					Email: "supercoder@superagi.com",
					When:  time.Now(),
				},
			})
			if err != nil {
				ws.logger.Error("Failed to commit files", zap.Error(err))
				return err
			}

			// Create the main branch from the initial commit
			headRef, err := repo.Head()
			if err != nil {
				ws.logger.Error("Failed to get HEAD reference", zap.Error(err))
				return err
			}

			ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), headRef.Hash())
			err = repo.Storer.SetReference(ref)
			if err != nil {
				ws.logger.Error("Failed to create main branch", zap.Error(err))
				return err
			}

			// Set HEAD to point to the main branch
			err = repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, ref.Name()))
			if err != nil {
				ws.logger.Error("Failed to set HEAD to main branch", zap.Error(err))
				return err
			}

			ws.logger.Info("Git commit files output", zap.String("output", commit.String()))

			// Push to given URL of remote, in main branch
			ws.logger.Info("Pushing changes to remote repository", zap.String("remoteURL", remoteURL))

			// Add the remote
			_, err = repo.CreateRemote(&config.RemoteConfig{
				Name: "origin",
				URLs: []string{remoteURL},
			})
			if err != nil {
				ws.logger.Error("Failed to create remote", zap.Error(err))
				return err
			}

			// Push to the remote repository
			auth := &http.BasicAuth{
				Username: ws.workspaceServiceConfig.GitnessAuthUsername(),
				Password: ws.workspaceServiceConfig.GitnessAuthToken(),
			}

			err = repo.Push(&git.PushOptions{
				RemoteName: "origin",
				Auth:       auth,
				RefSpecs:   []config.RefSpec{"refs/heads/main:refs/heads/main"},
			})

			// Cleanup: Remove the remote after pushing
			err = repo.DeleteRemote("origin")
			if err != nil {
				ws.logger.Error("Failed to delete remote", zap.Error(err))
				return err
			}

			ws.logger.Info("Successfully pushed changes to remote repository")
		} else {
			ws.logger.Error("Failed to open Git repository", zap.Error(err))
			return err
		}
	}

	err = utils.ChownRWorkspace(workspaceId, "1000", "1000")
	if err != nil {
		ws.logger.Error("Failed to chown workspace", zap.Error(err))
		return err
	}
	return nil
}

func (ws DockerWorkspaceService) DeleteWorkspace(workspaceId string) error {
	return nil
}

func NewDockerWorkspaceService(
	logger *zap.Logger,
	workspaceServiceConfig *workspaceconfig.WorkspaceServiceConfig,
) services.WorkspaceService {
	return &DockerWorkspaceService{
		workspaceServiceConfig: workspaceServiceConfig,
		logger:                 logger.Named("DockerWorkspaceService"),
	}
}
