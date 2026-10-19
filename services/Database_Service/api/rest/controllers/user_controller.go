package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
	"strconv"
)

type UserController struct {
	userService *services.UserService
}

func NewUserController(userService *services.UserService) *UserController {
	return &UserController{userService: userService}
}

func (uc *UserController) CreateUser(c *gin.Context) {
	var user models.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := uc.userService.CreateUser(&user)
	if err != nil {
		respondError(c, err)
		return
	}

	user.PasswordHash = ""
	c.JSON(http.StatusCreated, gin.H{"user": user})
}

func (uc *UserController) GetUsers(c *gin.Context) {
	users, err := uc.userService.GetAllUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}
	for i := range users {
		users[i].PasswordHash = ""
	}
	c.JSON(http.StatusOK, users)
}

func (uc *UserController) GetUser(c *gin.Context) {
	idParam := c.Query("id")
	username := c.Query("username")
	email := c.Query("email")

	if idParam == "" && username == "" && email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one of id, username, or email must be provided"})
		return
	}

	var id int
	if idParam != "" {
		var err error
		id, err = strconv.Atoi(idParam)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "id must be positive"})
			return
		}
	}

	user := models.User{ID: id, Username: username, Email: email}
	user, err := uc.userService.GetUser(user)
	if err != nil {
		respondError(c, err)

		return
	}
	user.PasswordHash = ""
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (uc *UserController) UpdateUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	var user models.User
	if err = c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user.ID = id

	err = uc.userService.UpdateUser(user)
	if err != nil {
		respondError(c, err)
		return
	}
	user, err = uc.userService.GetUser(models.User{ID: id})
	if err != nil {
		respondError(c, err)
		return
	}
	user.PasswordHash = ""
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (uc *UserController) DeleteUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	err = uc.userService.DeleteUser(id)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": nil})
}
