package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"project/models"
	"project/services"
	"strconv"
)

type ChatParticipantController struct {
	chatParticipantService *services.ChatParticipantService
}

func NewChatParticipantController(chatParticipantService *services.ChatParticipantService) *ChatParticipantController {
	return &ChatParticipantController{chatParticipantService: chatParticipantService}
}

func (cpc *ChatParticipantController) CreateChatParticipant(c *gin.Context) {
	var chatParticipant models.ChatParticipant
	if err := c.ShouldBindJSON(&chatParticipant); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := cpc.chatParticipantService.CreateChatParticipant(chatParticipant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"chatParticipant": chatParticipant})
}

func (cpc *ChatParticipantController) GetChatParticipantsByChatID(c *gin.Context) {
	chatID, err := strconv.Atoi(c.Param("chat_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	participants, err := cpc.chatParticipantService.GetAllChatParticipants(chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"chatParticipants": participants})
}

func (cpc *ChatParticipantController) DeleteChatParticipantByUserID(c *gin.Context) {
	chatID, err := strconv.Atoi(c.Param("chat_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if chatID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ChatID is required"})
		return
	}

	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if userID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UserID is required"})
		return
	}

}
