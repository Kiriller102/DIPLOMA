package rest

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"project/models"
	"project/services"
	"strconv"
)

type ChatController struct {
	chatService *services.ChatService
}

func NewChatController(chatService *services.ChatService) *ChatController {
	return &ChatController{chatService: chatService}
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

	err := cc.chatService.CreateChat(chat)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	if chatID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat id is required"})
		return
	}

	chat, err := cc.chatService.GetChatByID(chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	err = cc.chatService.UpdateChat(chat)
}

func (cc *ChatController) DeleteChat(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	if id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat id is required"})
	}

	err = cc.chatService.DeleteChatByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"chat": nil})
}
