package repository

import (
	"github.com/jmoiron/sqlx"
	"log"
	"project/services/Database_Service/models"
)

func PostUser(db *sqlx.DB, user models.User) error {
	query := `
		INSERT INTO users (username, email, password_hash, created_at, updated_at, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := db.Exec(query, user.Username, user.Email, user.PasswordHash, user.CreatedAt, user.UpdatedAt, user.IsActive)
	if err != nil {
		log.Printf("Error inserting user: %v\n", err)
		return err
	}
	return nil
}

func GetUserByID(db *sqlx.DB, id uint) (models.User, error) {
	query := `
		SELECT * FROM users WHERE id = $1
	`
	u, err := db.Exec(query, id)

	if err != nil {
		log.Printf("Error getting user: %v\n", err)
		panic(err)
	}
	user := models.User{}
	u
}
