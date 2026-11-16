package controllers

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"project/services/Database_Service/auth"
	"project/services/Database_Service/models"
	"project/services/Database_Service/services"
	"strconv"
)

type UserController struct {
	access      *auth.Service
	userService *services.UserService
}

func NewUserController(userService *services.UserService, access *auth.Service) *UserController {
	return &UserController{userService: userService, access: access}
}

// CreateUser is retained for clients of the old CRUD API. Use /auth/register.
func (uc *UserController) CreateUser(c *gin.Context) { respondError(c, auth.ErrForbidden) }

func (uc *UserController) createUserLegacy(c *gin.Context) {
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
		users[i].Email = ""
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
	if err := uc.access.RequireSelf(c.Request.Context(), user.ID); err != nil {
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

	if err := uc.access.RequireSelf(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	var user models.User
	if err = c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if user.PasswordHash != "" || user.Email != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email and password cannot be changed via profile"})
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

	if err := uc.access.RequireSelf(c.Request.Context(), id); err != nil {
		respondError(c, err)
		return
	}
	err = uc.userService.DeleteUser(id)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": nil})
}
