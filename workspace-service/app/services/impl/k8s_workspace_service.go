package impl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
	workspaceconfig "workspace-service/app/config"
	"workspace-service/app/models/dto"
	"workspace-service/app/services"
	"workspace-service/app/utils"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type K8sWorkspaceService struct {
	services.WorkspaceService
	clientset              *kubernetes.Clientset
	workspaceServiceConfig *workspaceconfig.WorkspaceServiceConfig
	k8sControllerClient    client.Client
	logger                 *zap.Logger
}

func (ws K8sWorkspaceService) CreateWorkspace(workspaceId string, backendTemplate string, frontendTemplate *string, remoteURL string, gitnessUser string, gitnessToken string) (*dto.WorkspaceDetails, error) {
	err := ws.checkAndCreateWorkspaceFromTemplate(workspaceId, backendTemplate, frontendTemplate, remoteURL, gitnessUser, gitnessToken)
	if err != nil {
		ws.logger.Error("Failed to check and create workspace from template", zap.Error(err))
		return nil, err
	}

	err = ws.checkAndCreateWorkspacePVC(workspaceId)
	if err != nil {
		ws.logger.Error("Failed to check and create workspace PVC", zap.Error(err))
		return nil, err
	}

	workspaceHost := ws.workspaceServiceConfig.WorkspaceHostName()
	workspaceUrl := fmt.Sprintf("https://%s.%s/?folder=/workspaces/%s", workspaceId, workspaceHost, workspaceId)
	frontendUrl := fmt.Sprintf("https://fe-%s.%s", workspaceId, workspaceHost)
	backendUrl := fmt.Sprintf("https://be-%s.%s", workspaceId, workspaceHost)

	response := &dto.WorkspaceDetails{
		WorkspaceId:      workspaceId,
		BackendTemplate:  &backendTemplate,
		FrontendTemplate: frontendTemplate,
		WorkspaceUrl:     &workspaceUrl,
		BackendUrl:       &backendUrl,
		FrontendUrl:      &frontendUrl,
	}

	exists := ws.checkIfWorkspaceExists(workspaceId)

	if exists {
		return response, nil
	}

	u := &unstructured.Unstructured{}
	u.Object = map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      workspaceId,
			"namespace": "argocd",
			"finalizers": []string{
				"resources-finalizer.argocd.argoproj.io",
			},
		},
		"spec": map[string]interface{}{
			"project": ws.workspaceServiceConfig.WorkspaceProject(),
			"source": map[string]interface{}{
				"repoURL":        ws.workspaceServiceConfig.ArgoRepoUrl(),
				"targetRevision": "HEAD",
				"path":           "supercoder/workspace",
				"helm": map[string]interface{}{
					"valueFiles": []string{
						ws.workspaceServiceConfig.WorkspaceValuesFileName(),
					},
				},
			},
			"destination": map[string]interface{}{
				"server":    "https://kubernetes.default.svc",
				"namespace": ws.workspaceServiceConfig.WorkspaceNamespace(),
			},
			"syncPolicy": map[string]interface{}{
				"syncOptions": []string{"PruneLast=true"},
				"automated": map[string]interface{}{
					"prune":      true,
					"selfHeal":   true,
					"allowEmpty": true,
				},
			},
		},
	}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Kind:    "Application",
		Version: "v1alpha1",
	})
	err = ws.k8sControllerClient.Create(context.Background(), u)
	if err != nil {
		_ = ws.k8sControllerClient.Delete(context.Background(), u, &client.DeleteOptions{})
		ws.logger.Error("Failed to create workspace", zap.Error(err))
		return nil, err
	}

	return response, nil
}

func (ws K8sWorkspaceService) CreateFrontendWorkspace(workspaceId string, frontendTemplate string, remoteUrl string) (*dto.WorkspaceDetails, error) {
	err := ws.checkAndCreateFrontendWorkspaceFromTemplate(workspaceId, frontendTemplate, remoteUrl)
	if err != nil {
		ws.logger.Error("Failed to check and create workspace from template", zap.Error(err))
		return nil, err
	}

	err = ws.checkAndCreateWorkspacePVC(workspaceId)
	if err != nil {
		ws.logger.Error("Failed to check and create workspace PVC", zap.Error(err))
		return nil, err
	}

	workspaceHost := ws.workspaceServiceConfig.WorkspaceHostName()
	workspaceUrl := fmt.Sprintf("https://%s.%s/?folder=/workspaces/stories/%s", workspaceId, workspaceHost, workspaceId)
	frontendUrl := fmt.Sprintf("https://fe-%s.%s", workspaceId, workspaceHost)

	response := &dto.WorkspaceDetails{
		WorkspaceId:      workspaceId,
		BackendTemplate:  nil,
		FrontendTemplate: &frontendTemplate,
		WorkspaceUrl:     &workspaceUrl,
		BackendUrl:       nil,
		FrontendUrl:      &frontendUrl,
	}

	return response, nil
}

func (ws K8sWorkspaceService) checkAndCreateWorkspacePVC(workspaceId string) (err error) {
	pvc, err := ws.clientset.CoreV1().PersistentVolumeClaims(ws.workspaceServiceConfig.WorkspaceNamespace()).Get(context.Background(), workspaceId, v12.GetOptions{})
	if err != nil || pvc == nil {
		err = ws.createWorkspacePVC(workspaceId)
		if err != nil {
			ws.logger.Error("Failed to create PVC", zap.Error(err))
			return err
		}
	}
	return nil
}

func (ws K8sWorkspaceService) createWorkspacePVC(workspaceId string) (err error) {
	storageClass := "workspaces-efs-dynamic-sc"
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: v12.ObjectMeta{
			Name: workspaceId,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteMany,
			},
			StorageClassName: &storageClass,
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("5Gi"),
				},
			},
		},
	}
	_, err = ws.clientset.CoreV1().PersistentVolumeClaims(ws.workspaceServiceConfig.WorkspaceNamespace()).Create(context.Background(), pvc, v12.CreateOptions{})
	if err != nil {
		ws.logger.Error("Failed to create PVC", zap.Error(err))
		return err
	}
	return nil
}

func (ws K8sWorkspaceService) checkIfWorkspaceExists(workspaceId string) bool {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Kind:    "Application",
		Version: "v1alpha1",
	})

	//check if application exists and skip if it does
	err := ws.k8sControllerClient.Get(context.Background(), client.ObjectKey{
		Namespace: "argocd",
		Name:      workspaceId,
	}, u)

	if err != nil {
		ws.logger.Error("Failed to get workspace", zap.Error(err))
		return false
	}

	return true
}

func (ws K8sWorkspaceService) checkAndCreateWorkspaceFromTemplate(workspaceId string, backendTemplate string, frontendTemplate *string, remoteURL string, gitnessUser string, gitnessToken string) error {
	exists, err := utils.CheckIfWorkspaceExists(workspaceId)
	if err != nil {
		ws.logger.Error("Failed to check if workspace exists", zap.Error(err))
		return err
	}

	//copying backend template in the root dir
	if !exists {
		err = utils.RsyncFolders("/templates/"+backendTemplate+"/", "/workspaces/"+workspaceId)
		if err != nil {
			ws.logger.Error("Failed to rsync folders", zap.Error(err))
			return err
		}
	}
	if frontendTemplate != nil {
		//creating a frontend folder in the directory
		frontendPath := "/workspaces/" + workspaceId + "/frontend"
		err = os.MkdirAll(frontendPath, os.ModePerm)
		if err != nil {
			fmt.Println("Error creating directory:", err)
			return err
		}
		//copying frontend template in the /frontend folder
		err = utils.SudoRsyncFolders("/templates/"+*frontendTemplate+"/", "/workspaces/"+workspaceId+"/frontend")
		if err != nil {
			ws.logger.Error("Failed to rsync folders", zap.Error(err))
			return err
		}
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
				Username: gitnessUser,
				Password: gitnessToken,
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

	err = utils.ChownRWorkspace("1000", "1000", workspacePath)
	if err != nil {
		ws.logger.Error("Failed to chown workspace", zap.Error(err))
		return err
	}

	return nil
}

func (ws K8sWorkspaceService) checkAndCreateFrontendWorkspaceFromTemplate(storyHashId string, workspaceId string, frontendTemplate string) error {
	exists, err := utils.CheckIfFrontendWorkspaceExists(storyHashId, workspaceId)
	if err != nil {
		ws.logger.Error("Failed to check if workspace exists", zap.Error(err))
		return err
	}
	if !exists {
		frontendPath := "/workspaces/stories/" + workspaceId + "/" + storyHashId
		err = os.MkdirAll(frontendPath, os.ModePerm)
		if err != nil {
			fmt.Println("Error creating directory:", err)
			return err
		}
		err = utils.RsyncFolders("/templates/"+frontendTemplate+"/", "/workspaces/stories/"+workspaceId+"/"+storyHashId)
		if err != nil {
			ws.logger.Error("Failed to rsync folders", zap.Error(err))
			return err
		}
	}
	workspacePath := "/workspaces/stories/" + workspaceId + "/" + storyHashId
	err = utils.ChownRWorkspace("1000", "1000", workspacePath)
	if err != nil {
		ws.logger.Error("Failed to chown workspace", zap.Error(err))
		return err
	}

	return nil
}

func (ws K8sWorkspaceService) DeleteWorkspace(workspaceId string) error {
	workspaceExists := ws.checkIfWorkspaceExists(workspaceId)
	if !workspaceExists {
		ws.logger.Info("Workspace does not exist", zap.String("workspaceId", workspaceId))
		return nil
	}
	u := &unstructured.Unstructured{}
	u.Object = map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      workspaceId,
			"namespace": "argocd",
		},
	}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Kind:    "Application",
		Version: "v1alpha1",
	})
	err := ws.k8sControllerClient.Delete(context.Background(), u, &client.DeleteOptions{})
	if err != nil {
		ws.logger.Error("Failed to delete workspace", zap.Error(err))
		return err
	}
	return nil
}

func NewK8sWorkspaceService(
	k8sClient client.Client,
	clientset *kubernetes.Clientset,
	workspaceServiceConfig *workspaceconfig.WorkspaceServiceConfig,
	logger *zap.Logger,
) services.WorkspaceService {
	return &K8sWorkspaceService{
		k8sControllerClient:    k8sClient,
		clientset:              clientset,
		workspaceServiceConfig: workspaceServiceConfig,
		logger:                 logger.Named("K8sWorkspaceService"),
	}
}
