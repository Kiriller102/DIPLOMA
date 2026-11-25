package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/internal/logging"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
	"strconv"
	"time"
)

type MessageController struct {
	access         *auth.Service
	messageService *services.MessageService
}

func NewMessageController(messageService *services.MessageService, access *auth.Service) *MessageController {
	return &MessageController{messageService: messageService, access: access}
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

	message.SenderID = mc.access.EffectiveUserID(c.Request.Context(), message.SenderID)
	if err := mc.access.RequireChat(c.Request.Context(), message.ChatID, false); err != nil {
		respondError(c, err)
		return
	}
	err := mc.messageService.CreateMessage(c.Request.Context(), &message)
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
		if err := mc.access.RequireChat(c.Request.Context(), chatID, false); err != nil {
			respondError(c, err)
			return
		}
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
		logging.Capture(c.Request.Context(), services.ErrInvalidInput, "validation")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if chatID <= 0 {
		logging.Capture(c.Request.Context(), services.ErrInvalidInput, "validation")
		c.JSON(http.StatusBadRequest, gin.H{"error": "ChatID is empty"})
		return
	}

	var beforeID *int
	if values, exists := c.Request.URL.Query()["before_id"]; exists {
		if len(values) != 1 {
			logging.Capture(c.Request.Context(), services.ErrInvalidInput, "validation")
			c.JSON(http.StatusBadRequest, gin.H{"error": "before_id must be specified once"})
			return
		}
		id, err := strconv.Atoi(values[0])
		if err != nil || id <= 0 {
			logging.Capture(c.Request.Context(), services.ErrInvalidInput, "validation")
			c.JSON(http.StatusBadRequest, gin.H{"error": "before_id must be a positive integer"})
			return
		}
		beforeID = &id
	}

	ctx := c.Request.Context()
	if err := mc.access.RequireChat(ctx, chatID, false); err != nil {
		logging.Capture(ctx, err, "access")
		if ctx.Err() != nil {
			return
		}
		respondError(c, err)
		return
	}
	page, err := mc.messageService.GetMessagesPageByChatID(ctx, chatID, beforeID)

	if err != nil {
		logging.Capture(ctx, err, "service")
		if ctx.Err() != nil {
			return
		}
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, page)
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

	if err := mc.access.RequireMessage(c.Request.Context(), id, true); err != nil {
		respondError(c, err)
		return
	}
	err = mc.messageService.UpdateMessage(c.Request.Context(), message)
	if err != nil {
		respondError(c, err)
		return
	}

	message, err = mc.messageService.GetMessageByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": message})
}

func (mc *MessageController) EncryptLegacyMessage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "positive message id required"})
		return
	}
	if err = mc.access.RequireMessage(c.Request.Context(), id, false); err != nil {
		respondError(c, err)
		return
	}
	var input struct {
		Content      string `json:"content"`
		PreviousHash string `json:"previous_hash"`
	}
	if err = c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid encrypted message"})
		return
	}
	p, err := auth.Current(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	message, err := mc.messageService.EncryptLegacyMessage(c.Request.Context(), id, p.User.ID, input.Content, input.PreviousHash)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": message})
}
