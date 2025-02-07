package repository

import (
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"log"
	"project/models"
)

type UserRepositoryInterface interface {
	CreateUser(db *sqlx.DB, user models.User) error
	GetAllUsers(db *sqlx.DB) ([]models.User, error)
	GetUserByID(db *sqlx.DB, id uint) (models.User, error)
	GetUserByUsername(db *sqlx.DB, username string) (models.User, error)
	GetUserByEmail(db *sqlx.DB, email string) (models.User, error)
	UpdateUser(db *sqlx.DB, user models.User) error
	DeleteUserByID(db *sqlx.DB, user models.User) error
}

type UserRepository struct{}

func (r *UserRepository) CreateUser(db *sqlx.DB, user models.User) error {
	query := `
		INSERT INTO users (username, email, first_name, last_name,  password_hash, created_at, updated_at, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := db.Exec(query, user.Username, user.Email, user.FirstName, user.LastName, user.PasswordHash, user.CreatedAt, user.UpdatedAt, user.IsActive)
	if err != nil {
		log.Printf("error inserting user: %v", err)
		return err
	}
	return nil
}

func (r *UserRepository) GetAllUsers(db *sqlx.DB) ([]models.User, error) {
	var users []models.User
	query := `
		SELECT id, username, email, first_name, last_name, password_hash, created_at, updated_at, is_active FROM users
	`
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("error getting users: %v", err)
		return users, err
	}
	defer func(rows *sql.Rows) {
		_ = rows.Close()
	}(rows)

	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Email,
			&user.FirstName,
			&user.LastName,
			&user.PasswordHash,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.IsActive)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return users, nil
			}
			log.Fatalf("error getting users: %v", err)
			return []models.User{}, err
		}
		users = append(users, user)
	}

	return users, nil
}

func (r *UserRepository) GetUserByID(db *sqlx.DB, id int) (models.User, error) {
	query := `
       SELECT id, username, email, first_name, last_name, password_hash, created_at, updated_at, is_active
       FROM users
       WHERE id = $1
   `

	user := models.User{}
	err := db.QueryRow(query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.FirstName,
		&user.LastName,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.IsActive,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("No user found with ID %d", id)
			return models.User{}, nil
		}
		log.Printf("Error scanning row: %v", err)
		return models.User{}, err
	}

	return user, nil
}

func (r *UserRepository) GetUserByUsername(db *sqlx.DB, username string) (models.User, error) {
	query := `
       SELECT id, username, email, first_name, last_name, password_hash, created_at, updated_at, is_active
       FROM users
       WHERE username = $1
   `

	user := models.User{}
	err := db.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.FirstName,
		&user.LastName,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.IsActive,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("No user found with username %d", username)
			return models.User{}, nil
		}
		log.Printf("Error scanning row: %v", err)
		return models.User{}, err
	}

	return user, nil
}

func (r *UserRepository) GetUserByEmail(db *sqlx.DB, email string) (models.User, error) {
	query := `
       SELECT id, username, email, first_name, last_name, password_hash, created_at, updated_at, is_active
       FROM users
       WHERE email = $1
   `

	user := models.User{}
	err := db.QueryRow(query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.FirstName,
		&user.LastName,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.IsActive,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("No user found with email %d", email)
			return models.User{}, nil
		}
		log.Printf("Error scanning row: %v", err)
		return models.User{}, err
	}

	return user, nil
}

func (r *UserRepository) UpdateUser(db *sqlx.DB, user models.User) error {
	query := `
		UPDATE users
		SET 
		    username = $1,
		    email = $2,
		    first_name = $3,
		    last_name = $4,
		    password_hash = $5,
			updated_at = $6,
			is_active = $7
		WHERE 
		    id = $8
	`

	_, err := db.Exec(
		query,
		user.Username,
		user.Email,
		user.FirstName,
		user.LastName,
		user.PasswordHash,
		user.UpdatedAt,
		user.ID,
	)

	if err != nil {
		log.Printf("error updating user (ID:%d): %v", user.ID, err)
		return err
	}

	return nil
}

func (r *UserRepository) DeleteUserByID(db *sqlx.DB, id int) error {
	query := `
		DELETE FROM users 
		WHERE id = $1
	`

	result, err := db.Exec(query, id)
	if err != nil {
		log.Printf("error deleting user (ID:%d): %v", id, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("failed to check rows affected for user deletion (ID:%d): %v", id, err)
		return err
	}

	if rowsAffected == 0 {
		log.Printf("user (ID:%d) not found", id)
		return errors.New("user not found")
	}

	return nil
}
