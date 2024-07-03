package repositories

import (
	"ai-developer/app/models"
	"ai-developer/app/types/request"
	"gorm.io/gorm"
)

type ProjectRepository struct {
	db *gorm.DB
}

func (receiver ProjectRepository) GetAllProjectsByOrganisationId(organisationId int) ([]models.Project, error) {
	var projects []models.Project
	err := receiver.db.Where("organisation_id = ?", organisationId).Find(&projects).Error
	if err != nil {
		return nil, err
	}
	return projects, nil
}

func (receiver ProjectRepository) CreateProject(project *models.Project) (*models.Project, error) {
	err := receiver.db.Create(project).Error
	if err != nil {
		return nil, err
	}
	return project, nil
}

func (receiver ProjectRepository) GetProjectById(projectId int) (*models.Project, error) {
	var project models.Project
	err := receiver.db.First(&project, projectId).Error
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (receiver ProjectRepository) UpdateProject(project *models.Project, updateData request.UpdateProjectRequest) (*models.Project, error) {
	project.Name = updateData.Name
	project.Description = updateData.Description
	err := receiver.db.Save(project).Error
	if err != nil {
		return nil, err
	}
	return project, nil
}

func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{
		db: db,
	}
}
