package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"project/services/Database_Service/api/grpc"
	"project/services/Database_Service/api/rest"
	"project/services/Database_Service/api/rest/controllers"
	"project/services/Database_Service/migrations"
	"project/services/Database_Service/repository"
	"project/services/Database_Service/services"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	for _, path := range []string{".env", "services/Database_Service/configs/secrets.env"} {
		if err := godotenv.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("load %s: %w", path, err)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := repository.NewPostgresDB(repository.Config{
		Host: env("DB_HOST", "localhost"), Port: env("DB_PORT", "5432"),
		Username: env("DB_USER", "messenger"), Password: env("DB_PASSWORD", "messenger-local"),
		DBName: env("DB_NAME", "messenger"), SSLMode: env("DB_SSLMODE", "disable"),
	})
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := migrations.Apply(initCtx, db); err != nil {
		return err
	}
	if env("DEMO_MODE", "false") == "true" {
		log.Println("Demo mode: user selection does not authenticate requests")
		if err := migrations.SeedDemo(initCtx, db); err != nil {
			return err
		}
	}
	users, err := services.NewUserService(db)
	if err != nil {
		return err
	}
	messages, err := services.NewMessageService(db)
	if err != nil {
		return err
	}
	chats, err := services.NewChatService(db)
	if err != nil {
		return err
	}
	participants, err := services.NewChatParticipantService(db)
	if err != nil {
		return err
	}
	httpServer := rest.NewHTTPServer(controllers.NewUserController(users), controllers.NewMessageController(messages),
		controllers.NewChatController(chats), controllers.NewChatParticipantController(participants), db)
	grpcServer := grpc.NewGRPCServer()
	grpcServer.RegisterServices(users, messages, chats, participants)
	httpListener, err := net.Listen("tcp", ":"+env("HTTP_PORT", "8080"))
	if err != nil {
		return err
	}
	defer httpListener.Close()
	grpcListener, err := net.Listen("tcp", ":"+env("GRPC_PORT", "5050"))
	if err != nil {
		return err
	}
	defer grpcListener.Close()
	errs := make(chan error, 2)
	go func() { errs <- httpServer.Serve(httpListener) }()
	go func() { errs <- grpcServer.Serve(grpcListener) }()
	log.Printf("HTTP listening on %s; gRPC on %s", httpListener.Addr(), grpcListener.Addr())
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errs:
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	// Cancel active long polls before draining the HTTP server.
	httpServer.CancelPolls()
	grpcServer.Shutdown(shutdownCtx)
	if err := httpServer.Shutdown(shutdownCtx); err != nil && serveErr == nil {
		serveErr = err
	}
	return serveErr
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
