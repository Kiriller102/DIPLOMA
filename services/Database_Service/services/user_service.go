package services

import (
	"errors"
	"project/services/Database_Service/models"
	"project/services/Database_Service/repository"
)

type UserService struct {
	userRepo repository.UserRepository
}

func NewUserService(userRepo repository.UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
}

func (s *UserService) CreateUser(user models.User) (models.User, error) {
	if user.Email == "" || user.PasswordHash == "" {
		return models.User{}, errors.New("email or password is empty")
	}

	if u, er := s.userRepo.GetUserByEmail(user.Email); er != nil || u != models.User {

	}
}
