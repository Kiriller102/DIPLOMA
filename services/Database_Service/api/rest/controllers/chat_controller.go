package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
	"strconv"
)

type ChatController struct {
	access      *auth.Service
	chatService *services.ChatService
}

func NewChatController(chatService *services.ChatService, access *auth.Service) *ChatController {
	return &ChatController{chatService: chatService, access: access}
}

func (cc *ChatController) CreateChat(c *gin.Context) {
	var chat models.Chat
	if err := c.ShouldBindJSON(&chat); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if chat.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat name is required"})
		return
	}

	chat.CreatedBy = cc.access.EffectiveUserID(c.Request.Context(), chat.CreatedBy)
	err := cc.chatService.CreateChat(c.Request.Context(), &chat)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"chat": chat})
}

func (cc *ChatController) GetChat(c *gin.Context) {
	chatID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if chatID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat id is required"})
		return
	}

	if err := cc.access.RequireChat(c.Request.Context(), chatID, false); err != nil {
		respondError(c, err)
		return
	}
	chat, err := cc.chatService.GetChatByID(c.Request.Context(), chatID)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"chat": chat})
}

func (cc *ChatController) UpdateChat(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var chat models.Chat
	if err = c.ShouldBindJSON(&chat); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	chat.ID = id

	if err := cc.access.RequireChat(c.Request.Context(), id, true); err != nil {
		respondError(c, err)
		return
	}
	err = cc.chatService.UpdateChat(c.Request.Context(), chat)
	if err != nil {
		respondError(c, err)
		return
	}
	chat, err = cc.chatService.GetChatByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"chat": chat})
}

func (cc *ChatController) DeleteChat(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat id is required"})
		return
	}

	if err := cc.access.RequireChat(c.Request.Context(), id, true); err != nil {
		respondError(c, err)
		return
	}
	err = cc.chatService.DeleteChatByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"chat": nil})
}
