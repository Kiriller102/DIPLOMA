package controllers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/e2ee"
	"project/services/Database_Service/services"
)

type CryptoController struct {
	crypto *services.CryptoService
	access *auth.Service
}

func NewCryptoController(db *sqlx.DB, access *auth.Service) *CryptoController {
	return &CryptoController{crypto: services.NewCryptoService(db), access: access}
}

func (cc *CryptoController) Identity(c *gin.Context) {
	p, err := auth.Current(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	identity, err := cc.crypto.Identity(c.Request.Context(), p.User.ID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, identity)
}

func (cc *CryptoController) CreateIdentity(c *gin.Context) {
	var identity e2ee.Identity
	if err := c.ShouldBindJSON(&identity); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid encryption identity"})
		return
	}
	p, err := auth.Current(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	identity.UserID = p.User.ID
	if err = cc.crypto.CreateIdentity(c.Request.Context(), identity); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "ok"})
}

func (cc *CryptoController) PublicKeys(c *gin.Context) {
	keys, err := cc.crypto.PublicKeys(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, keys)
}

func (cc *CryptoController) ChatKeys(c *gin.Context) {
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil || chatID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "positive chat id required"})
		return
	}
	if err = cc.access.RequireChat(c.Request.Context(), chatID, false); err != nil {
		respondError(c, err)
		return
	}
	keys, err := cc.crypto.ChatKeys(c.Request.Context(), chatID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, keys)
}
