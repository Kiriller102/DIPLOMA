package services

import (
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"project/models"
	"project/repository"
	"regexp"
	"time"
)

type UserService struct {
	userRepo repository.UserRepository
	db       *sqlx.DB
}

func NewUserService(db *sqlx.DB) (*UserService, error) {
	if db == nil {
		log.Printf("database is nil")
		return nil, fmt.Errorf("database is nil")
	}
	if err := db.Ping(); err != nil {
		log.Printf("error conecction to database")
		return nil, fmt.Errorf("error conecction to database")
	}

	return &UserService{
		userRepo: repository.UserRepository{},
		db:       db,
	}, nil
}

func (s *UserService) CreateUser(user models.User) error {
	if user.Email == "" || user.PasswordHash == "" {
		return errors.New("email or password is empty")
	}

	if !isValidEmail(user.Email) {
		log.Printf("error incorrect email: %v", user.Email)
		return errors.New("invalid email")
	}

	if u, err := s.userRepo.GetUserByEmail(s.db, user.Email); err != nil {
		log.Printf("error get user by email")
		return err
	} else if u.ID != 0 {
		log.Printf("error user already exists with email: %v", user.Email)
		return errors.New("user already exists with email: " + user.Email)
	}

	if u, err := s.userRepo.GetUserByUsername(s.db, user.Username); err != nil {
		log.Printf("error get user by username")
		return err
	} else if u.ID != 0 {
		log.Printf("error user already exists with username: %v", user.Username)
		return errors.New("user already exists with username: " + user.Username)
	}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	err := s.userRepo.CreateUser(s.db, user)
	if err != nil {
		return err
	}

	return nil
}

func (s *UserService) UpdateUser(user models.User) error {
	//user.UpdatedAt = time.Now()
	//if user.Email == "" || user.PasswordHash == "" {
	//	return models.User{}, errors.New("email or password is empty")
	//}
	//
	//if !isValidEmail(user.Email) {
	//	log.Printf("error incorrect email: %v", user.Email)
	//	return models.User{}, errors.New("invalid email")
	//}
	//
	//if u, err := s.userRepo.GetUserByEmail(s.db, user.Email); err != nil {
	//	log.Printf("error get user by email")
	//	return models.User{}, err
	//} else if u.ID != user.ID {
	//	log.Printf("error user already exists with email: %v", user.Email)
	//}
	//
	//if u, err := s.userRepo.GetUserByUsername(s.db, user.Username); err != nil {
	//	log.Printf("error get user by username")
	//	return models.User{}, err
	//} else if u.ID != user.ID {
	//	log.Printf("error user already exists with username: %v", user.Username)
	//	return models.User{}, errors.New("user already exists with username: " + user.Username)
	//}
	//
	//err := s.userRepo.UpdateUser(s.db, user)
	//if err != nil {
	//	return models.User{}, err
	//}
	//
	//return user, nil

	if user.ID == 0 {
		log.Printf("error: user.ID is nil")
		return errors.New("user id is required")
	}

	usr, err := s.userRepo.GetUserByID(s.db, user.ID)
	if err != nil {
		log.Printf("error get user by id: %s", err.Error())
		return err
	}

	if user.Username != "" {
		usr.Username = user.Username
	}

	if user.PasswordHash != "" {
		usr.PasswordHash = user.PasswordHash
	}

	if user.Email != "" {
		if isValidEmail(user.Email) {
			usr.Email = user.Email
		} else {
			log.Printf("error incorrect email: %v", user.Email)
			return errors.New("invalid email")
		}
	}

	if user.FirstName != "" {
		usr.FirstName = user.FirstName
	}

	if user.LastName != "" {
		usr.LastName = user.LastName
	}

	usr.UpdatedAt = time.Now()

	err = s.userRepo.UpdateUser(s.db, usr)
	if err != nil {
		log.Printf("error update user: %s", err.Error())
		return err
	}

	return nil
}

func (s *UserService) DeleteUser(id int) error {
	if u, err := s.userRepo.GetUserByID(s.db, id); err != nil {
		log.Printf("error get user by id")
		return err
	} else if u.ID != id {
		log.Printf("error user not found: %v", id)
		return errors.New("user not found")
	}

	err := s.userRepo.DeleteUserByID(s.db, id)
	if err != nil {
		log.Printf("error delete user")
		return err
	}

	return nil
}

func (s *UserService) GetUser(user models.User) (models.User, error) {

	u := models.User{}
	var err error
	switch {
	case user.Email != "":
		if u, err = s.userRepo.GetUserByEmail(s.db, user.Email); err != nil {
			log.Printf("error get user by email")
		} else if u != (models.User{}) {
			log.Printf("error no users exist with email: %v", user.Email)
		}
	case user.Username != "":
		if u, err = s.userRepo.GetUserByUsername(s.db, user.Username); err != nil {
			log.Printf("error get user by username")
		} else if u != (models.User{}) {
			log.Printf("error no users exist with username: %v", user.Username)
		}
	default:
		if u, err = s.userRepo.GetUserByID(s.db, user.ID); err != nil {
			log.Printf("error get user by id")
		} else if u.ID != user.ID {
			log.Printf("error user not found: %v", user)
		}
	}

	if u == (models.User{}) {
		return models.User{}, errors.New("user not found")
	}
	return u, err
}

func (s *UserService) GetAllUsers() ([]models.User, error) {
	users, err := s.userRepo.GetAllUsers(s.db)
	if err != nil {
		log.Printf("error get all users")
		return nil, err
	}

	return users, nil
}

func isValidEmail(email string) bool {
	re := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	r, err := regexp.Compile(re)
	if err != nil {
		fmt.Println("error in regular expression:", err)
		return false
	}
	return r.MatchString(email)
}
