package controllers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"project/services/Database_Service/services"
)

func respondError(c *gin.Context, err error) {
	code, message := http.StatusInternalServerError, "internal server error"
	var pg *pq.Error
	switch {
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
	if code == http.StatusInternalServerError {
		log.Printf("request failed: %v", err)
	}
	c.JSON(code, gin.H{"error": message})
}
