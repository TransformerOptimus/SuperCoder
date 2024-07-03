package repositories

import (
	"ai-developer/app/models"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func (receiver UserRepository) GetUserByID(userID uint) (*models.User, error) {
	var user models.User
	err := receiver.db.First(&user, userID).Error
	if err != nil {
		return nil, err
	}
	return &user, nil

}

func (receiver UserRepository) GetUserByEmail(email string) (*models.User, error) {
	var user models.User
	err := receiver.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (receiver UserRepository) CreateUser(user *models.User) (*models.User, error) {
	err := receiver.db.Create(user).Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (receiver UserRepository) UpdateUserByEmail(email string, user *models.User) error {
	user.Email = email
	err := receiver.db.Save(user).Error
	if err != nil {
		return err
	}
	return nil
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{
		db: db,
	}
}

func (receiver UserRepository) FetchOrganisationIDByUserID(userID uint) (uint, error) {
	var organisationID uint
	err := receiver.db.Model(&models.User{}).Where("id = ?", userID).Select("organisation_id").Scan(&organisationID).Error
	if err != nil {
		return 0, err
	}
	return organisationID, nil
}
