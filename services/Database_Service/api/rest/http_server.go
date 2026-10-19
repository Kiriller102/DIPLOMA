package rest

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"project/services/Database_Service/api/rest/controllers"
)

type HTTPServer struct {
	router      *gin.Engine
	server      *http.Server
	cancelPolls context.CancelFunc
}

func NewHTTPServer(userController *controllers.UserController,
	messageController *controllers.MessageController,
	chatController *controllers.ChatController,
	ChatParticipantController *controllers.ChatParticipantController, db *sqlx.DB) *HTTPServer {
	router := gin.Default()

	router.SetTrustedProxies(nil)
	pollCtx, cancelPolls := context.WithCancel(context.Background())
	router.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		c.Next()
	})
	router.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

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
		messageRoutes.GET("/longpoll/:chat_id", messageController.LongPolling)
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
		router:      router,
		cancelPolls: cancelPolls,
		server: &http.Server{
			Addr:              ":8080",
			Handler:           router,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return pollCtx },
		},
	}
}

func (s *HTTPServer) Serve(listener net.Listener) error  { return s.server.Serve(listener) }
func (s *HTTPServer) Handler() http.Handler              { return s.router }
func (s *HTTPServer) CancelPolls()                       { s.cancelPolls() }
func (s *HTTPServer) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }
