package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"project/models"
	"project/services"
	"strconv"
)

type MessageController struct {
	messageService *services.MessageService
}

func NewMessageController(messageService *services.MessageService) *MessageController {
	return &MessageController{messageService: messageService}
}

func (mc *MessageController) CreateMessage(c *gin.Context) {
	var message models.Message
	if err := c.ShouldBindJSON(&message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if message.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Content is empty"})
		return
	}

	err := mc.messageService.CreateMessage(message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": message})
}

func (mc *MessageController) GetMessagesByChatID(c *gin.Context) {
	chatID, err := strconv.Atoi(c.Param("chat_id"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if chatID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ChatID is empty"})
		return
	}

	messages, err := mc.messageService.GetMessagesByChatID(chatID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func (mc *MessageController) UpdateMessage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	var message models.Message
	if err = c.ShouldBindJSON(&message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if message.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Content is empty"})
		return
	}
	message.ID = id

	err = mc.messageService.UpdateMessage(message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": message})
}
