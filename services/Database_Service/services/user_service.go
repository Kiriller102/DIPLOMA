package services

import (
	"database/sql"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/models"
	"project/services/Database_Service/repository"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
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

func (s *UserService) CreateUser(user *models.User) error {
	if user != nil {
		user.Username = strings.TrimSpace(user.Username)
		user.Email = strings.TrimSpace(user.Email)
	}
	if user == nil || user.Username == "" || utf8.RuneCountInString(user.Username) > 50 || !isValidEmail(user.Email) || len(user.Email) > 100 || user.PasswordHash == "" {
		return fmt.Errorf("%w: username, valid email and password_hash are required", ErrInvalidInput)
	}
	if utf8.RuneCountInString(user.FirstName) > 50 || utf8.RuneCountInString(user.LastName) > 50 || len(user.PasswordHash) > 255 {
		return fmt.Errorf("%w: user field is too long", ErrInvalidInput)
	}
	user.ID = 0
	user.CreatedAt = time.Now().UTC()
	user.UpdatedAt = user.CreatedAt
	return s.userRepo.CreateUser(s.db, user)
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

	if user.ID <= 0 {
		log.Printf("error: user.ID is nil")
		return fmt.Errorf("%w: user id is required", ErrInvalidInput)
	}

	usr, err := s.userRepo.GetUserByID(s.db, user.ID)
	if err != nil {
		log.Printf("error get user by id: %s", err.Error())
		return err
	}

	if usr.ID == 0 {
		return sql.ErrNoRows
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
			return fmt.Errorf("%w: invalid email", ErrInvalidInput)
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
		return sql.ErrNoRows
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
		} else if u == (models.User{}) {
			log.Printf("error no users exist with email: %v", user.Email)
		}
	case user.Username != "":
		if u, err = s.userRepo.GetUserByUsername(s.db, user.Username); err != nil {
			log.Printf("error get user by username")
		} else if u == (models.User{}) {
			log.Printf("error no users exist with username: %v", user.Username)
		}
	default:
		if u, err = s.userRepo.GetUserByID(s.db, user.ID); err != nil {
			log.Printf("error get user by id")
		} else if u.ID != user.ID {
			log.Printf("error user not found: %v", user)
		}
	}

	if err != nil {
		return models.User{}, err
	}
	if u == (models.User{}) {
		return models.User{}, sql.ErrNoRows
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
