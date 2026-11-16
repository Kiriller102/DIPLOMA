package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"project/services/Database_Service/auth"
)

const sessionCookie = "messenger_session"

type attempt struct {
	count int
	until time.Time
}

type AuthController struct {
	access   *auth.Service
	secure   bool
	mu       sync.Mutex
	attempts map[string]attempt
	workers  chan struct{}
}

func NewAuthController(access *auth.Service, secure bool) *AuthController {
	return &AuthController{access: access, secure: secure, attempts: make(map[string]attempt), workers: make(chan struct{}, 4)}
}

func RequestToken(r *http.Request) (string, error) {
	if r.Header.Get("Authorization") != "" {
		return auth.Bearer(r.Header.Get("Authorization"))
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", auth.ErrUnauthenticated
	}
	return cookie.Value, nil
}

func readAuthJSON(c *gin.Context, target interface{}) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request fields"})
		return false
	}
	if err := decoder.Decode(new(interface{})); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expected one JSON object"})
		return false
	}
	return true
}

// Bound expensive password hashing and repeated attempts without an external service.
func (a *AuthController) allow(c *gin.Context, email string) bool {
	key := c.ClientIP() + "|" + strings.ToLower(strings.TrimSpace(email))
	now := time.Now()
	a.mu.Lock()
	for k, v := range a.attempts {
		if !now.Before(v.until) {
			delete(a.attempts, k)
		}
	}
	entry := a.attempts[key]
	if entry.count >= 10 || (entry.count == 0 && len(a.attempts) >= 10000) {
		a.mu.Unlock()
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts; try again in a minute"})
		return false
	}
	if entry.count == 0 {
		entry.until = now.Add(time.Minute)
	}
	entry.count++
	a.attempts[key] = entry
	a.mu.Unlock()
	select {
	case a.workers <- struct{}{}:
		return true
	default:
		c.Header("Retry-After", "1")
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many authentication requests"})
		return false
	}
}

func (a *AuthController) setCookie(c *gin.Context, token string, expires time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", Expires: expires, MaxAge: int(auth.SessionLifetime.Seconds()), HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode})
}

func (a *AuthController) Register(c *gin.Context) {
	var input auth.Registration
	if !readAuthJSON(c, &input) || !a.allow(c, input.Email) {
		return
	}
	defer func() { <-a.workers }()
	session, err := a.access.Register(c.Request.Context(), input)
	if err != nil {
		respondError(c, err)
		return
	}
	// Replace any earlier session in this browser after successful authentication.
	if old, _ := RequestToken(c.Request); old != "" {
		if err := a.access.Logout(c.Request.Context(), old); err != nil {
			respondError(c, err)
			return
		}
	}
	a.setCookie(c, session.Token, session.ExpiresAt)
	c.JSON(http.StatusCreated, gin.H{"user": session.User, "expires_at": session.ExpiresAt})
}

func (a *AuthController) Login(c *gin.Context) { a.login(c, false) }
func (a *AuthController) Token(c *gin.Context) { a.login(c, true) }
func (a *AuthController) login(c *gin.Context, bearer bool) {
	var input auth.Credentials
	if !readAuthJSON(c, &input) || !a.allow(c, input.Email) {
		return
	}
	defer func() { <-a.workers }()
	session, err := a.access.Login(c.Request.Context(), input)
	if err != nil {
		respondError(c, err)
		return
	}
	if bearer {
		c.JSON(http.StatusOK, gin.H{"access_token": session.Token, "token_type": "Bearer", "expires_at": session.ExpiresAt, "user": session.User})
		return
	}
	if old, _ := RequestToken(c.Request); old != "" {
		if err := a.access.Logout(c.Request.Context(), old); err != nil {
			respondError(c, err)
			return
		}
	}
	a.setCookie(c, session.Token, session.ExpiresAt)
	c.JSON(http.StatusOK, gin.H{"user": session.User, "expires_at": session.ExpiresAt})
}

func (a *AuthController) Logout(c *gin.Context) {
	token, _ := RequestToken(c.Request)
	if err := a.access.Logout(c.Request.Context(), token); err != nil {
		respondError(c, err)
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", Expires: time.Unix(1, 0), MaxAge: -1, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode})
	c.Status(http.StatusNoContent)
}
