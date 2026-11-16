package rest

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"project/services/Database_Service/auth"
	"strings"
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
	ChatParticipantController *controllers.ChatParticipantController, db *sqlx.DB, access *auth.Service, secureCookies bool) *HTTPServer {
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

	authController := controllers.NewAuthController(access, secureCookies)
	router.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
			origin := c.GetHeader("Origin")
			validOrigin := true
			if origin != "" {
				parsed, err := url.Parse(origin)
				validOrigin = err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && strings.EqualFold(parsed.Host, c.Request.Host)
			}
			if c.GetHeader("X-Messenger-Request") != "1" || !validOrigin {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "cross-origin request rejected"})
				return
			}
		}
		c.Next()
	})
	router.POST("/auth/register", authController.Register)
	router.POST("/auth/login", authController.Login)
	router.POST("/auth/token", authController.Token)
	router.POST("/auth/logout", authController.Logout)

	router.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		token, err := controllers.RequestToken(c.Request)
		if err == nil {
			var ctx context.Context
			ctx, err = access.Authenticate(c.Request.Context(), token)
			if err == nil {
				c.Request = c.Request.WithContext(ctx)
				c.Next()
				return
			}
		}
		code, message := http.StatusUnauthorized, "authentication required"
		if errors.Is(err, auth.ErrForbidden) {
			code, message = http.StatusForbidden, "permission denied"
		}
		if !errors.Is(err, auth.ErrUnauthenticated) && !errors.Is(err, auth.ErrForbidden) {
			log.Printf("authenticate request: %v", err)
			code, message = http.StatusServiceUnavailable, "authentication temporarily unavailable"
		}
		c.Header("WWW-Authenticate", "Bearer")
		c.AbortWithStatusJSON(code, gin.H{"error": message})
	})
	router.GET("/auth/me", func(c *gin.Context) {
		p, err := auth.Current(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user": p.User})
	})
	userRoutes := router.Group("/users")
	{
		userRoutes.POST("", userController.CreateUser)
		userRoutes.GET("", userController.GetUser)
		userRoutes.GET("/all", userController.GetUsers)
		userRoutes.PUT("/:id", userController.UpdateUser)
		userRoutes.DELETE("/:id", userController.DeleteUser)
	}
	cryptoController := controllers.NewCryptoController(db, access)
	router.GET("/crypto/identity", cryptoController.Identity)
	router.POST("/crypto/identity", cryptoController.CreateIdentity)
	router.GET("/crypto/keys", cryptoController.PublicKeys)
	router.GET("/crypto/chat/:id", cryptoController.ChatKeys)

	messageRoutes := router.Group("/messages")
	{
		messageRoutes.POST("", messageController.CreateMessage)
		messageRoutes.GET("/longpoll/:chat_id", messageController.LongPolling)
		messageRoutes.GET("/:chat_id", messageController.GetMessagesByChatID)
		messageRoutes.PUT("/:id", messageController.UpdateMessage)
		messageRoutes.POST("/:id/encrypt", messageController.EncryptLegacyMessage)
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
