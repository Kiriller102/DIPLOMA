package rest

import (
	"context"
	"errors"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"os"
	"os/signal"
	"project/api/rest/controllers"
	"sync"
	"time"
)

type HTTPServer struct {
	router *gin.Engine
	server *http.Server
}

func NewHTTPServer(userController *controllers.UserController,
	messageController *controllers.MessageController,
	chatController *controllers.ChatController,
	ChatParticipantController *controllers.ChatParticipantController) *HTTPServer {
	router := gin.Default()

	router.Use(cors.Default()) //TODO change before publish

	userRoutes := router.Group("/users")
	{
		userRoutes.POST("", userController.CreateUser)
		userRoutes.GET("", userController.GetUser)
		userRoutes.GET("/all", userController.GetUsers)
		userRoutes.PUT("/:id", userController.UpdateUser)
		userRoutes.DELETE("/:id", userController.DeleteUser)
	}

	messageRoutes := router.Group("/messages")
	{
		messageRoutes.POST("", messageController.CreateMessage)
		messageRoutes.GET("longpoll/:chat_id", messageController.LongPolling)
		messageRoutes.GET("/:chat_id", messageController.GetMessagesByChatID)
		messageRoutes.PUT("/:id", messageController.UpdateMessage)
	}

	chatRoutes := router.Group("/chat")
	{
		chatRoutes.POST("", chatController.CreateChat)
		chatRoutes.GET("/:id", chatController.GetChat)
		chatRoutes.PUT("/:id", chatController.UpdateChat)
		chatRoutes.DELETE("/:id", chatController.DeleteChat)
	}

	chatParticipantRoutes := router.Group("/chat_participant")
	{
		chatParticipantRoutes.POST("", ChatParticipantController.CreateChatParticipant)
		chatParticipantRoutes.GET("/:chat_id", ChatParticipantController.GetChatParticipantsByChatID)
		chatParticipantRoutes.GET("/chats/:user_id", ChatParticipantController.GetAllChatsByUserID)
		chatParticipantRoutes.DELETE("/:chat_id/:user_id", ChatParticipantController.DeleteChatParticipantByUserID)
	}

	return &HTTPServer{
		router: router,
		server: &http.Server{
			Addr:    ":8080",
			Handler: router,
		},
	}
}

func (s *HTTPServer) Run(wg *sync.WaitGroup) {
	defer wg.Done()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		log.Println("Starting HTTP server on :8080")
		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("error starting server: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down HTTP server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}
