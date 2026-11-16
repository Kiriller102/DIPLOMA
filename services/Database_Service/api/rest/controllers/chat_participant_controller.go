package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
	"strconv"
)

type ChatParticipantController struct {
	access                 *auth.Service
	chatParticipantService *services.ChatParticipantService
}

func NewChatParticipantController(chatParticipantService *services.ChatParticipantService, access *auth.Service) *ChatParticipantController {
	return &ChatParticipantController{chatParticipantService: chatParticipantService, access: access}
}

func (cpc *ChatParticipantController) CreateChatParticipant(c *gin.Context) {
	var chatParticipant models.ChatParticipant
	if err := c.ShouldBindJSON(&chatParticipant); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := cpc.access.RequireChat(c.Request.Context(), chatParticipant.ChatID, true); err != nil {
		respondError(c, err)
		return
	}
	err := cpc.chatParticipantService.CreateChatParticipant(&chatParticipant)
	if err != nil {
		respondError(c, err)
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

	if err := cpc.access.RequireChat(c.Request.Context(), chatID, false); err != nil {
		respondError(c, err)
		return
	}
	participants, err := cpc.chatParticipantService.GetAllChatParticipants(chatID)
	if err != nil {
		respondError(c, err)
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
	if chatID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ChatID is required"})
		return
	}

	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UserID is required"})
		return
	}

	chatPart := models.ChatParticipant{ChatID: chatID, UserID: userID}
	if err := cpc.access.RequireRemoveParticipant(c.Request.Context(), chatID, userID); err != nil {
		respondError(c, err)
		return
	}
	err = cpc.chatParticipantService.DeleteParticipantByUserID(chatPart)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"chat_participant": nil})
}

func (cpc *ChatParticipantController) GetAllChatsByUserID(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userID is required"})
		return
	}

	if err := cpc.access.RequireSelf(c.Request.Context(), userID); err != nil {
		respondError(c, err)
		return
	}
	chats, err := cpc.chatParticipantService.GetChatsIdsByUserID(userID)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, chats)
}
