package services

import (
	"ai-developer/app/config"
	"ai-developer/app/services/git_providers"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"go.uber.org/zap"
	"os"
	"os/exec"
)

type CodeDownloadService struct {
	projectService      *ProjectService
	gitnessService      *git_providers.GitnessService
	organisationService *OrganisationService
	logger              *zap.Logger
}

func (cds CodeDownloadService) GetZipFile(projectId uint) (zipFile *string, err error) {
	tempDir, err := os.MkdirTemp(os.TempDir(), fmt.Sprintf("project-%d", projectId))
	if err != nil {
		return
	}

	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tempDir)

	project, err := cds.projectService.GetProjectById(projectId)
	if err != nil {
		return
	}

	org, err := cds.organisationService.GetOrganisationByID(project.OrganisationID)
	if err != nil {
		return
	}

	spaceOrProjectName := cds.gitnessService.GetSpaceOrProjectName(org)

	origin := fmt.Sprintf("https://%s/git/%s/%s.git", config.GitnessHost(), spaceOrProjectName, project.Name)
	_, err = git.PlainClone(tempDir, false, &git.CloneOptions{
		URL: origin,
		Auth: &http.BasicAuth{
			Username: config.GitnessUser(),
			Password: config.GitnessToken(),
		},
	})

	if err != nil {
		return
	}

	zipFilePath := fmt.Sprintf("%s-%s.zip", tempDir, "repo.zip")
	err = zipDir(tempDir, zipFilePath)
	if err != nil {
		return
	}

	return &zipFilePath, nil
}

func zipDir(source, target string) error {
	cmd := exec.Command("zip", "-r", target, ".")
	cmd.Dir = source
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("zip error: %v", err)
	}
	return nil
}

func NewCodeDownloadService(
	logger *zap.Logger,
	projectService *ProjectService,
	gitnessService *git_providers.GitnessService,
	organisationService *OrganisationService,
) *CodeDownloadService {
	return &CodeDownloadService{
		projectService:      projectService,
		gitnessService:      gitnessService,
		organisationService: organisationService,
		logger:              logger.Named("CodeDownloadService"),
	}
}
