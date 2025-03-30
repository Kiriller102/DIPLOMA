package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"project/models"
	"project/services"
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

	err := mc.messageService.CreateMessage(message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": message})
}

func (mc *MessageController) LongPolling(c *gin.Context) {
	chatID, err := strconv.Atoi(c.Param("chat_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat_id required"})
		return
	}
	lastIDStr := c.Query("last_id")
	lastID, err := strconv.Atoi(lastIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Параметр last_id обязателен"})
		return
	}

	timeout := time.After(30 * time.Second)          // максимальное время ожидания
	ticker := time.NewTicker(100 * time.Millisecond) // интервал проверки новых сообщений
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			// Если за время ожидания не пришли новые сообщения,
			// отправляем пустой ответ или статус, что обновлений нет.
			c.JSON(http.StatusNoContent, gin.H{})
			return
		case <-ticker.C:
			// Получаем все сообщения чата
			messages, err := mc.messageService.GetMessagesByChatID(chatID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting messages"})
				return
			}
			// Если сообщения существуют и последнее сообщение новое (его ID больше, чем lastID клиента)
			if len(messages) > 0 && messages[len(messages)-1].ID != lastID {
				c.JSON(http.StatusOK, messages)
				return
			}
		}
	}
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
