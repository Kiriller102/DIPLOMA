package controllers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/internal/logging"
	"project/services/Database_Service/services"
)

func respondError(c *gin.Context, err error) {
	logging.Capture(c.Request.Context(), err, "response")
	if c.Request.Context().Err() != nil {
		return
	}
	code, message := http.StatusInternalServerError, "internal server error"
	var pg *pq.Error
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		code, message = http.StatusUnauthorized, "authentication required"
		c.Header("WWW-Authenticate", "Bearer")
	case errors.Is(err, auth.ErrForbidden):
		code, message = http.StatusForbidden, "permission denied"
	case errors.Is(err, sql.ErrNoRows):
		code, message = http.StatusNotFound, "not found"
	case errors.Is(err, services.ErrInvalidInput):
		code, message = http.StatusBadRequest, err.Error()
	case errors.As(err, &pg):
		switch pg.Code {
		case "23505":
			code, message = http.StatusConflict, "record already exists"
		case "23503", "22P02", "22001", "23502":
			code, message = http.StatusBadRequest, "invalid fields or referenced record does not exist"
		}
	}
	if code == http.StatusInternalServerError && logging.FromContext(c.Request.Context()) == nil {
		log.Printf("request failed: %v", err)
	}
	c.JSON(code, gin.H{"error": message})
}
