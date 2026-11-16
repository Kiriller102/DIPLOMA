package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
)

var (
	ErrUnauthenticated = errors.New("authentication required")
	ErrForbidden       = errors.New("permission denied")
)

const SessionLifetime = 24 * time.Hour
const passwordCost = 12

type Service struct {
	db        *sqlx.DB
	dummyHash []byte
}
type Principal struct {
	User      models.User
	ExpiresAt time.Time
	TokenHash []byte
}
type principalKey struct{}
type Registration struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}
type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type Session struct {
	Token     string
	User      models.User
	ExpiresAt time.Time
}

var usernamePattern = regexp.MustCompile(`^[\pL\pN_][\pL\pN_.-]{2,49}$`)

func New(db *sqlx.DB) (*Service, error) {
	if db == nil {
		return nil, errors.New("auth: database is required")
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("dummy-password-never-used"), passwordCost)
	if err != nil {
		return nil, err
	}
	return &Service{db: db, dummyHash: dummy}, nil
}

func (s *Service) Register(ctx context.Context, input Registration) (Session, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	email, err := mail.ParseAddress(input.Email)
	if err != nil || email.Address != input.Email || len(input.Email) > 100 || !usernamePattern.MatchString(input.Username) || utf8.RuneCountInString(input.FirstName) > 50 || utf8.RuneCountInString(input.LastName) > 50 {
		return Session{}, fmt.Errorf("%w: valid email, username (3–50 letters, digits, _, . or -) and names up to 50 characters are required", services.ErrInvalidInput)
	}
	if utf8.RuneCountInString(input.Password) < 12 || len(input.Password) > 72 || !utf8.ValidString(input.Password) {
		return Session{}, fmt.Errorf("%w: password must contain at least 12 characters and at most 72 UTF-8 bytes", services.ErrInvalidInput)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), passwordCost)
	if err != nil {
		return Session{}, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	var user models.User
	err = tx.GetContext(ctx, &user, `INSERT INTO users (username,email,first_name,last_name,password_hash)
  VALUES ($1,$2,$3,$4,$5) RETURNING id,username,email,first_name,last_name,created_at,updated_at,is_active`, input.Username, input.Email, input.FirstName, input.LastName, string(hash))
	if err != nil {
		return Session{}, err
	}
	session, err := createSession(ctx, tx, user)
	if err != nil {
		return Session{}, err
	}
	return session, tx.Commit()
}

func (s *Service) Login(ctx context.Context, input Credentials) (Session, error) {
	var user models.User
	err := s.db.GetContext(ctx, &user, `SELECT id,username,email,first_name,last_name,password_hash,created_at,updated_at,is_active FROM users WHERE lower(email)=$1`, strings.ToLower(strings.TrimSpace(input.Email)))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Session{}, err
	}
	hash := []byte(user.PasswordHash)
	// Existing demo records have no usable password; never reveal account existence.
	_, hashErr := bcrypt.Cost(hash)
	if hashErr != nil {
		hash = s.dummyHash
	}
	passwordErr := bcrypt.CompareHashAndPassword(hash, []byte(input.Password))
	if err != nil || hashErr != nil || passwordErr != nil || !user.IsActive || user.PasswordHash == "" || len(input.Password) > 72 {
		return Session{}, ErrUnauthenticated
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()
	// Recheck credentials under a row lock before issuing a session.
	var active bool
	if err = tx.GetContext(ctx, &active, `SELECT is_active FROM users WHERE id=$1 AND password_hash=$2 FOR SHARE`, user.ID, user.PasswordHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrUnauthenticated
		}
		return Session{}, err
	}
	if !active {
		return Session{}, ErrUnauthenticated
	}
	user.PasswordHash = ""
	session, err := createSession(ctx, tx, user)
	if err != nil {
		return Session{}, err
	}
	return session, tx.Commit()
}

func createSession(ctx context.Context, tx *sqlx.Tx, user models.User) (Session, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return Session{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(secret[:])
	digest := sha256.Sum256(secret[:])
	expires := time.Now().UTC().Add(SessionLifetime)
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_sessions WHERE expires_at<=NOW()`); err != nil {
		return Session{}, err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO user_sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)`, digest[:], user.ID, expires)
	return Session{Token: token, User: user, ExpiresAt: expires}, err
}

func tokenHash(token string) ([]byte, error) {
	if len(token) != 43 {
		return nil, ErrUnauthenticated
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, ErrUnauthenticated
	}
	digest := sha256.Sum256(raw)
	return digest[:], nil
}
func Bearer(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", ErrUnauthenticated
	}
	if _, err := tokenHash(parts[1]); err != nil {
		return "", err
	}
	return parts[1], nil
}
func (s *Service) Authenticate(ctx context.Context, token string) (context.Context, error) {
	digest, err := tokenHash(token)
	if err != nil {
		return nil, err
	}
	var record struct {
		models.User
		ExpiresAt time.Time `db:"expires_at"`
	}
	err = s.db.GetContext(ctx, &record, `SELECT u.id,u.username,u.email,u.first_name,u.last_name,u.created_at,u.updated_at,u.is_active,s.expires_at
  FROM user_sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>NOW()`, digest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	if !record.IsActive {
		return nil, ErrForbidden
	}
	return context.WithValue(ctx, principalKey{}, Principal{User: record.User, ExpiresAt: record.ExpiresAt, TokenHash: digest}), nil
}
func (s *Service) Logout(ctx context.Context, token string) error {
	digest, err := tokenHash(token)
	if err != nil {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE token_hash=$1`, digest)
	return err
}
func (s *Service) currentSession(ctx context.Context) (Principal, error) {
	p, err := Current(ctx)
	if err != nil {
		return Principal{}, err
	}
	var valid bool
	err = s.db.GetContext(ctx, &valid, `SELECT EXISTS(SELECT 1 FROM user_sessions s JOIN users u ON u.id=s.user_id WHERE token_hash=$1 AND u.id=$2 AND u.is_active AND expires_at>NOW())`, p.TokenHash, p.User.ID)
	if err != nil {
		return Principal{}, err
	}
	if !valid {
		return Principal{}, ErrUnauthenticated
	}
	return p, nil
}

func Current(ctx context.Context) (Principal, error) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	if !ok || p.User.ID <= 0 || !time.Now().Before(p.ExpiresAt) {
		return Principal{}, ErrUnauthenticated
	}
	return p, nil
}

func (s *Service) EffectiveUserID(ctx context.Context, supplied int) int {
	p, _ := Current(ctx)
	return p.User.ID
}

func (s *Service) RequireSelf(ctx context.Context, userID int) error {
	p, err := s.currentSession(ctx)
	if err != nil {
		return err
	}
	if p.User.ID != userID {
		return ErrForbidden
	}
	return nil
}

func (s *Service) RequireChat(ctx context.Context, chatID int, admin bool) error {
	p, err := s.currentSession(ctx)
	if err != nil {
		return err
	}
	var allowed bool
	err = s.db.GetContext(ctx, &allowed, `SELECT EXISTS (SELECT 1 FROM chat_participants p JOIN users u ON u.id = p.user_id
		WHERE p.chat_id = $1 AND p.user_id = $2 AND u.is_active AND (NOT $3 OR p.role = 'admin'))`, chatID, p.User.ID, admin)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *Service) RequireMessage(ctx context.Context, messageID int, author bool) error {
	p, err := s.currentSession(ctx)
	if err != nil {
		return err
	}
	var message models.Message
	if err = s.db.GetContext(ctx, &message, `SELECT chat_id, sender_id FROM messages WHERE id = $1`, messageID); err != nil {
		return err
	}
	if err = s.RequireChat(ctx, message.ChatID, false); err != nil {
		return err
	}
	if author && message.SenderID != p.User.ID {
		return ErrForbidden
	}
	return nil
}

func (s *Service) RequireRemoveParticipant(ctx context.Context, chatID, userID int) error {
	if err := s.RequireChat(ctx, chatID, true); err != nil {
		return err
	}
	var owner int
	if err := s.db.GetContext(ctx, &owner, `SELECT created_by FROM chats WHERE id = $1`, chatID); err != nil {
		return err
	}
	if owner == userID {
		return ErrForbidden
	}
	return nil
}
