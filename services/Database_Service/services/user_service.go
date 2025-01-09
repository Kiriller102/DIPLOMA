package services

import (
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/repository"
)

type UserService struct {
	userRepo repository.UserRepository
	db       *sqlx.DB
}

func NewUserService(userRepo repository.UserRepository, db *sqlx.DB) *UserService {
	if db == nil {
		log.Printf("database is nil")
		return nil
	}
	if db.Ping() != nil {
		log.Printf("error conecction to database")
		return nil
	}

	return &UserService{
		userRepo: userRepo,
		db:       db,
	}
}

//func (s *UserService) CreateUser(user models.User) (models.User, error) {
//	if user.Email == "" || user.PasswordHash == "" {
//		return models.User{}, errors.New("email or password is empty")
//	}
//
//	if u, err := s.userRepo.GetUserByEmail(s.db, user.Email); err != nil {
//		log.Printf("error get user by email")
//		return models.User{}, err
//	} else if u != models.User{} {
//
//	}
//}
