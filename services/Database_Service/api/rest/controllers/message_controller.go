package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
	"strconv"
	"time"
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

	err := mc.messageService.CreateMessage(&message)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": message})
}

func (mc *MessageController) LongPolling(c *gin.Context) {
	chatID, err := strconv.Atoi(c.Param("chat_id"))
	if err != nil || chatID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat_id must be positive"})
		return
	}
	lastID, err := strconv.Atoi(c.DefaultQuery("last_id", "0"))
	if err != nil || lastID < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "last_id must be non-negative"})
		return
	}
	timeout := time.NewTimer(25 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		messages, err := mc.messageService.GetMessagesAfter(c.Request.Context(), chatID, lastID)
		if err != nil {
			if c.Request.Context().Err() == nil {
				respondError(c, err)
			}
			return
		}
		if len(messages) > 0 {
			c.JSON(http.StatusOK, messages)
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-timeout.C:
			c.Status(http.StatusNoContent)
			return
		case <-ticker.C:
		}
	}
}

func (mc *MessageController) GetMessagesByChatID(c *gin.Context) {
	chatID, err := strconv.Atoi(c.Param("chat_id"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if chatID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ChatID is empty"})
		return
	}

	messages, err := mc.messageService.GetMessagesByChatID(chatID)

	if err != nil {
		respondError(c, err)
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
	if id <= 0 {
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
		respondError(c, err)
		return
	}

	message, err = mc.messageService.GetMessageByID(id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": message})
}
