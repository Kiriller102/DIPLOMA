package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/models"
)

type UserRepositoryInterface interface {
	CreateUser(db *sqlx.DB, user *models.User) error
	GetAllUsers(ctx context.Context, db *sqlx.DB) ([]models.User, error)
	GetUserByID(ctx context.Context, db *sqlx.DB, id int) (models.User, error)
	GetUserByUsername(ctx context.Context, db *sqlx.DB, username string) (models.User, error)
	GetUserByEmail(ctx context.Context, db *sqlx.DB, email string) (models.User, error)
	UpdateUser(ctx context.Context, db *sqlx.DB, user models.User) error
	DeleteUserByID(ctx context.Context, db *sqlx.DB, id int) error
}

type UserRepository struct{}

func (r *UserRepository) CreateUser(db *sqlx.DB, user *models.User) error {
	query := `
		INSERT INTO users (username, email, first_name, last_name,  password_hash, created_at, updated_at, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id
	`
	err := db.QueryRowx(query, user.Username, user.Email, user.FirstName, user.LastName, user.PasswordHash, user.CreatedAt, user.UpdatedAt, user.IsActive).Scan(&user.ID)
	if err != nil {
		log.Printf("error inserting user: %v", err)
		return err
	}
	return nil
}

func (r *UserRepository) GetAllUsers(ctx context.Context, db *sqlx.DB) ([]models.User, error) {
	users := make([]models.User, 0)
	query := `
		SELECT id, username, email, first_name, last_name, password_hash, created_at, updated_at, is_active FROM users
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("error getting users: %v", err)
		return users, err
	}
	defer rows.Close()

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
				return users, rows.Err()
			}
			log.Printf("error getting users: %v", err)
			return []models.User{}, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

func (r *UserRepository) GetUserByID(ctx context.Context, db *sqlx.DB, id int) (models.User, error) {
	query := `
       SELECT id, username, email, first_name, last_name, password_hash, created_at, updated_at, is_active
       FROM users
       WHERE id = $1
   `

	user := models.User{}
	err := db.GetContext(ctx, &user, query, id)

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

func (r *UserRepository) GetUserByUsername(ctx context.Context, db *sqlx.DB, username string) (models.User, error) {
	query := `
       SELECT id, username, email, first_name, last_name, password_hash, created_at, updated_at, is_active
       FROM users
       WHERE username = $1
   `

	user := models.User{}
	err := db.GetContext(ctx, &user, query, username)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("No user found with username %s", username)
			return models.User{}, nil
		}
		log.Printf("Error scanning row: %v", err)
		return models.User{}, err
	}

	return user, nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, db *sqlx.DB, email string) (models.User, error) {
	query := `
       SELECT id, username, email, first_name, last_name, password_hash, created_at, updated_at, is_active
       FROM users
       WHERE email = $1
   `

	user := models.User{}
	err := db.GetContext(ctx, &user, query, email)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("No user found with email %s", email)
			return models.User{}, nil
		}
		log.Printf("Error scanning row: %v", err)
		return models.User{}, err
	}

	return user, nil
}

func (r *UserRepository) UpdateUser(ctx context.Context, db *sqlx.DB, user models.User) error {
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

	_, err := db.ExecContext(ctx,
		query,
		user.Username,
		user.Email,
		user.FirstName,
		user.LastName,
		user.PasswordHash,
		user.UpdatedAt,
		user.IsActive,
		user.ID,
	)

	if err != nil {
		log.Printf("error updating user (ID:%d): %v", user.ID, err)
		return err
	}

	return nil
}

func (r *UserRepository) DeleteUserByID(ctx context.Context, db *sqlx.DB, id int) error {
	query := `
		DELETE FROM users 
		WHERE id = $1
	`

	result, err := db.ExecContext(ctx, query, id)
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
